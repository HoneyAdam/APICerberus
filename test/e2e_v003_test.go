package test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

func TestE2ELeastLatencyWithHealthChecksAndCircuitBreaker(t *testing.T) {
	t.Parallel()

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fast-ok"))
	}))
	defer fast.Close()

	gwAddr := freeAddr(t)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Services: []config.Service{
			{ID: "svc-v003-lb", Name: "svc-v003-lb", Protocol: "http", Upstream: "up-v003-lb"},
		},
		Routes: []config.Route{
			{
				ID:      "route-v003-lb",
				Name:    "route-v003-lb",
				Service: "svc-v003-lb",
				Paths:   []string{"/v003/lb"},
				Methods: []string{http.MethodGet},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-v003-lb",
				Name:      "up-v003-lb",
				Algorithm: "least_latency",
				Targets: []config.UpstreamTarget{
					{ID: "t-bad", Address: "127.0.0.1:54321", Weight: 1},
					{ID: "t-fast", Address: mustHost(t, fast.URL), Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           2 * time.Second,
						Timeout:            100 * time.Millisecond,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{
				Name: "circuit-breaker",
				Config: map[string]any{
					"error_threshold":    1.0,
					"volume_threshold":   1,
					"sleep_window":       "200ms",
					"half_open_requests": 1,
					"window":             "2s",
				},
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	waitForGatewayListener(t, gwAddr)

	doRequest := func() *http.Response {
		req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+"/v003/lb", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp
	}

	resp1 := doRequest()
	_ = resp1.Body.Close()
	if resp1.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected first response 502 got %d", resp1.StatusCode)
	}

	resp2 := doRequest()
	body2 := readAllAndClose(t, resp2.Body)
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected second response 503 got %d body=%q", resp2.StatusCode, body2)
	}
	if !strings.Contains(body2, "circuit_open") {
		t.Fatalf("expected circuit_open payload, got %q", body2)
	}

	time.Sleep(250 * time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	var recovered bool
	for time.Now().Before(deadline) {
		resp := doRequest()
		body := readAllAndClose(t, resp.Body)
		if resp.StatusCode == http.StatusOK && body == "fast-ok" {
			recovered = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !recovered {
		t.Fatalf("expected recovery to healthy target after sleep window")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
}

func TestE2ERetryWithMultipleUpstreamTargets(t *testing.T) {
	t.Parallel()

	var status503Hits atomic.Int64
	status503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		status503Hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("upstream-503"))
	}))
	defer status503.Close()

	var okHits atomic.Int64
	okTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		okHits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("retry-final-ok"))
	}))
	defer okTarget.Close()

	gwAddr := freeAddr(t)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Services: []config.Service{
			{ID: "svc-v003-retry", Name: "svc-v003-retry", Protocol: "http", Upstream: "up-v003-retry"},
		},
		Routes: []config.Route{
			{
				ID:      "route-v003-retry",
				Name:    "route-v003-retry",
				Service: "svc-v003-retry",
				Paths:   []string{"/v003/retry"},
				Methods: []string{http.MethodGet},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-v003-retry",
				Name:      "up-v003-retry",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "t-closed-port", Address: "127.0.0.1:54321", Weight: 1},
					{ID: "t-503", Address: mustHost(t, status503.URL), Weight: 1},
					{ID: "t-ok", Address: mustHost(t, okTarget.URL), Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           2 * time.Second,
						Timeout:            100 * time.Millisecond,
						HealthyThreshold:   1,
						UnhealthyThreshold: 3,
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{
				Name: "retry",
				Config: map[string]any{
					"max_retries": 2,
					"base_delay":  "1ms",
					"max_delay":   "2ms",
					"jitter":      false,
				},
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	waitForGatewayListener(t, gwAddr)

	req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+"/v003/retry", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)
	if resp.StatusCode != http.StatusOK || body != "retry-final-ok" {
		t.Fatalf("unexpected retry response status=%d body=%q", resp.StatusCode, body)
	}

	if status503Hits.Load() != 1 {
		t.Fatalf("expected 503 target to be hit once, got %d", status503Hits.Load())
	}
	if okHits.Load() != 1 {
		t.Fatalf("expected final healthy target to be hit once, got %d", okHits.Load())
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
}

func waitForGatewayListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("gateway did not start listening on %s", addr)
}
