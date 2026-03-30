package gateway

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestGatewayStartAndShutdownHTTPS(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("secure-ok"))
	}))
	defer upstream.Close()

	httpsAddr := freeAddr(t)
	certPath, keyPath := writeTestCertificatePair(t, t.TempDir(), "localhost", 72*time.Hour)

	cfg := gatewayTestConfig(t, "", mustHost(t, upstream.URL))
	cfg.Gateway.HTTPAddr = ""
	cfg.Gateway.HTTPSAddr = httpsAddr
	cfg.Gateway.TLS = config.TLSConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- gw.Start(ctx)
	}()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // test-only self-signed certificate
			},
		},
		Timeout: 2 * time.Second,
	}
	defer client.CloseIdleConnections()

	targetURL := "https://" + httpsAddr + "/api/users"
	waitForHTTPSReady(t, client, targetURL)

	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("https request through gateway failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.StatusCode)
	}
	if body != "secure-ok" {
		t.Fatalf("unexpected body: %q", body)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("gateway start returned error on shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("gateway did not shutdown after context cancellation")
	}
}

func waitForHTTPSReady(t *testing.T, client *http.Client, rawURL string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(rawURL)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("gateway did not become ready for %s", rawURL)
}
