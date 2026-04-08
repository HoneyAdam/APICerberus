package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

func TestE2EAdminConfigureAndProxy(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("e2e-ok"))
	}))
	defer upstream.Close()

	upHost := mustHost(t, upstream.URL)
	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			Addr:        adminAddr,
			APIKey:      "secret-e2e",
			TokenSecret: "secret-e2e-token",
			TokenTTL:    1 * time.Hour,
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	adminHandler, err := admin.NewServer(cfg, gw)
	if err != nil {
		t.Fatalf("admin.NewServer error: %v", err)
	}
	adminHTTP := &http.Server{
		Addr:           adminAddr,
		Handler:        adminHandler,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
		IdleTimeout:    10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gwErrCh := make(chan error, 1)
	go func() { gwErrCh <- gw.Start(ctx) }()

	adminErrCh := make(chan error, 1)
	go func() {
		err := adminHTTP.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		adminErrCh <- err
	}()

	waitForTCPReady(t, adminAddr)
	adminToken := getAdminBearerToken(t, adminAddr, "secret-e2e")
	waitForHTTPReady(t, "http://"+adminAddr+"/admin/api/v1/status", map[string]string{"Authorization": "Bearer " + adminToken})

	mustAdminCall(t, "http://"+adminAddr+"/admin/api/v1/upstreams", "secret-e2e", http.MethodPost, map[string]any{
		"id":        "up-e2e",
		"name":      "up-e2e",
		"algorithm": "round_robin",
		"targets": []map[string]any{
			{"id": "up-e2e-t1", "address": upHost, "weight": 1},
		},
		"health_check": map[string]any{
			"active": map[string]any{
				"path":                "/health",
				"interval":            int64(time.Second),
				"timeout":             int64(time.Second),
				"healthy_threshold":   1,
				"unhealthy_threshold": 1,
			},
		},
	}, http.StatusCreated)

	mustAdminCall(t, "http://"+adminAddr+"/admin/api/v1/services", "secret-e2e", http.MethodPost, map[string]any{
		"id":       "svc-e2e",
		"name":     "svc-e2e",
		"protocol": "http",
		"upstream": "up-e2e",
	}, http.StatusCreated)

	mustAdminCall(t, "http://"+adminAddr+"/admin/api/v1/routes", "secret-e2e", http.MethodPost, map[string]any{
		"id":      "route-e2e",
		"name":    "route-e2e",
		"service": "svc-e2e",
		"paths":   []string{"/e2e"},
		"methods": []string{"GET"},
	}, http.StatusCreated)

	waitForHTTPReady(t, "http://"+gwAddr+"/e2e", nil)
	resp, err := http.Get("http://" + gwAddr + "/e2e")
	if err != nil {
		t.Fatalf("gateway request error: %v", err)
	}
	body := readAllAndClose(t, resp.Body)
	if resp.StatusCode != http.StatusOK || body != "e2e-ok" {
		t.Fatalf("unexpected gateway response: status=%d body=%q", resp.StatusCode, body)
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = adminHTTP.Shutdown(shutdownCtx)

	if err := <-gwErrCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
	if err := <-adminErrCh; err != nil {
		t.Fatalf("admin runtime error: %v", err)
	}
}

func TestE2EHotReloadWithConfigWatch(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hot-reload-ok"))
	}))
	defer upstream.Close()

	upHost := mustHost(t, upstream.URL)
	gwAddr := freeAddr(t)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "apicerberus.yaml")

	writeConfigFile(t, cfgPath, gwAddr, upHost, "/old")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("initial config load: %v", err)
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	waitForHTTPReady(t, "http://"+gwAddr+"/old", nil)
	resp, err := http.Get("http://" + gwAddr + "/old")
	if err != nil {
		t.Fatalf("old route request failed: %v", err)
	}
	if body := readAllAndClose(t, resp.Body); resp.StatusCode != http.StatusOK || body != "hot-reload-ok" {
		t.Fatalf("old route unexpected response status=%d body=%q", resp.StatusCode, body)
	}

	reloaded := make(chan struct{}, 1)
	stopWatch, err := config.Watch(cfgPath, func(next *config.Config, loadErr error) {
		if loadErr != nil {
			return
		}
		if reloadErr := gw.Reload(next); reloadErr == nil {
			select {
			case reloaded <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		t.Fatalf("config watch error: %v", err)
	}
	defer stopWatch()

	writeConfigFile(t, cfgPath, gwAddr, upHost, "/new")

	select {
	case <-reloaded:
	case <-time.After(8 * time.Second):
		t.Fatalf("hot reload did not trigger")
	}

	resp, err = http.Get("http://" + gwAddr + "/old")
	if err != nil {
		t.Fatalf("old route request after reload failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected old route to return 404 after reload, got %d", resp.StatusCode)
	}

	resp, err = http.Get("http://" + gwAddr + "/new")
	if err != nil {
		t.Fatalf("new route request after reload failed: %v", err)
	}
	if body := readAllAndClose(t, resp.Body); resp.StatusCode != http.StatusOK || body != "hot-reload-ok" {
		t.Fatalf("new route unexpected response status=%d body=%q", resp.StatusCode, body)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
}

func mustAdminCall(t *testing.T, rawURL, key, method string, payload any, expectedStatus int) {
	t.Helper()
	var body []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("json marshal: %v", err)
		}
		body = b
	}
	req, err := http.NewRequest(method, rawURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if key != "" {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse url: %v", err)
		}
		token := getAdminBearerToken(t, u.Host, key)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin call failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected admin status for %s %s: got %d want %d body=%s", method, rawURL, resp.StatusCode, expectedStatus, string(b))
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func waitForHTTPReady(t *testing.T, rawURL string, headers map[string]string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("service not ready: %s", rawURL)
}

func readAllAndClose(t *testing.T, rc io.ReadCloser) string {
	t.Helper()
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u.Host
}

func writeConfigFile(t *testing.T, path, gwAddr, upHost, routePath string) {
	t.Helper()
	content := fmt.Sprintf(`
gateway:
  http_addr: "%s"
  read_timeout: "2s"
  write_timeout: "2s"
  idle_timeout: "5s"
  max_header_bytes: 1048576
  max_body_bytes: 1048576
admin:
  addr: "127.0.0.1:0"
  api_key: "x"
  ui_enabled: false
  ui_path: "/dashboard"
logging:
  level: "info"
  format: "json"
  output: "stdout"
services:
  - id: "svc-hot"
    name: "svc-hot"
    protocol: "http"
    upstream: "up-hot"
routes:
  - id: "route-hot"
    name: "route-hot"
    service: "svc-hot"
    paths:
      - "%s"
    methods:
      - "GET"
upstreams:
  - id: "up-hot"
    name: "up-hot"
    algorithm: "round_robin"
    targets:
      - id: "up-hot-t1"
        address: "%s"
        weight: 1
    health_check:
      active:
        path: "/health"
        interval: "1s"
        timeout: "1s"
        healthy_threshold: 1
        unhealthy_threshold: 1
`, gwAddr, routePath, upHost)

	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}
