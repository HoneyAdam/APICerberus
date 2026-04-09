//go:build e2e

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/ratelimit"
)

// TestChaosUpstreamPanicRecovery verifies that when an upstream panics,
// the gateway returns a 5xx and does not crash.
func TestChaosUpstreamPanicRecovery(t *testing.T) {
	t.Parallel()

	// Upstream that panics on every request
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("chaos: simulated upstream panic")
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
			APIKey:      "chaos-admin-key-123456789012345678",
			TokenSecret: "chaos-secret-token-1234567890123456",
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

	// Wait for admin
	adminToken := waitAndGetAdminToken(t, adminAddr, "chaos-admin-key-123456789012345678")
	waitForHTTPReady(t, "http://"+adminAddr+"/admin/api/v1/status", map[string]string{"Authorization": "Bearer " + adminToken})

	adminClient := &http.Client{Timeout: 10 * time.Second}

	mustAdminPostJSON(t, adminClient, adminAddr, adminToken, "/admin/api/v1/upstreams", map[string]any{
		"id":        "panic-up",
		"name":      "panic-up",
		"algorithm": "round_robin",
		"targets": []map[string]any{
			{"id": "panic-up-t1", "address": upHost, "weight": 1},
		},
	}, http.StatusCreated)

	mustAdminPostJSON(t, adminClient, adminAddr, adminToken, "/admin/api/v1/services", map[string]any{
		"id":       "panic-svc",
		"name":     "panic-svc",
		"protocol": "http",
		"upstream": "panic-up",
	}, http.StatusCreated)

	mustAdminPostJSON(t, adminClient, adminAddr, adminToken, "/admin/api/v1/routes", map[string]any{
		"id":      "panic-route",
		"name":    "panic-route",
		"service": "panic-svc",
		"paths":   []string{"/panic"},
		"methods": []string{"GET"},
	}, http.StatusCreated)

	time.Sleep(200 * time.Millisecond)

	// Send requests — should get 5xx, not gateway crash
	gwClient := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "http://"+gwAddr+"/panic", nil)
		resp, err := gwClient.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode < 500 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("request %d: expected 5xx, got %d body=%s", i, resp.StatusCode, string(body))
		}
		resp.Body.Close()
	}

	// Gateway should still be alive after panic
	time.Sleep(100 * time.Millisecond)
	req, _ := http.NewRequest("GET", "http://"+gwAddr+"/panic", nil)
	resp, err := gwClient.Do(req)
	if err != nil {
		t.Fatalf("post-panic request failed (gateway crashed): %v", err)
	}
	resp.Body.Close()
}

// TestChaosRateLimiterLocalFallback verifies that in-memory rate limiting
// continues to work correctly when Redis is unavailable.
func TestChaosRateLimiterLocalFallback(t *testing.T) {
	t.Parallel()

	// Create a Redis limiter pointing to an unreachable address.
	_, err := ratelimit.NewRedisLimiter(config.RedisConfig{
		Enabled:      true,
		Address:      "localhost:19999",
		DialTimeout:  200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
	})

	if err == nil {
		t.Fatal("expected error creating Redis limiter with unreachable Redis, got nil")
	}
	t.Logf("Redis limiter creation failed as expected: %v", err)

	// Verify that local (in-memory) rate limiting still works independently.
	local := ratelimit.NewTokenBucket(10, 10) // 10 rps, burst 10
	for i := 0; i < 10; i++ {
		allowed, _, _ := local.Allow("chaos-test-key")
		if !allowed {
			t.Fatalf("local limiter denied request %d (should allow within burst)", i)
		}
	}

	// 11th request should be denied (burst exhausted)
	allowed, _, _ := local.Allow("chaos-test-key")
	if allowed {
		t.Fatal("local limiter should deny after burst is exhausted")
	}
}

// TestChaosCorruptedDatabase verifies that gateway.New returns an error
// when the database file is corrupted.
func TestChaosCorruptedDatabase(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/corrupted.db"
	if err := os.WriteFile(dbPath, []byte("this is not a sqlite database"), 0o644); err != nil {
		t.Fatalf("write corrupted db: %v", err)
	}

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
			APIKey:      "chaos-admin-key-123456789012345678",
			TokenSecret: "chaos-secret-token-1234567890123456",
			TokenTTL:    1 * time.Hour,
		},
		Store: config.StoreConfig{
			Path: dbPath,
		},
	}

	_, err := gateway.New(cfg)
	if err == nil {
		t.Fatal("expected gateway.New to fail with corrupted database, got nil")
	}
	t.Logf("gateway.New failed as expected with corrupted DB: %v", err)
}

// TestChaosUpstreamConnectionFailure verifies that when all upstream targets
// are unreachable, the gateway returns 502 Bad Gateway without crashing.
func TestChaosUpstreamConnectionFailure(t *testing.T) {
	t.Parallel()

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
			APIKey:      "chaos-admin-key-123456789012345678",
			TokenSecret: "chaos-secret-token-1234567890123456",
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

	adminToken := waitAndGetAdminToken(t, adminAddr, "chaos-admin-key-123456789012345678")
	waitForHTTPReady(t, "http://"+adminAddr+"/admin/api/v1/status", map[string]string{"Authorization": "Bearer " + adminToken})

	adminClient := &http.Client{Timeout: 10 * time.Second}

	// Point to an unreachable upstream
	mustAdminPostJSON(t, adminClient, adminAddr, adminToken, "/admin/api/v1/upstreams", map[string]any{
		"id":        "dead-up",
		"name":      "dead-up",
		"algorithm": "round_robin",
		"targets": []map[string]any{
			{"id": "dead-up-t1", "address": "127.0.0.1:59999", "weight": 1},
		},
	}, http.StatusCreated)

	mustAdminPostJSON(t, adminClient, adminAddr, adminToken, "/admin/api/v1/services", map[string]any{
		"id":       "dead-svc",
		"name":     "dead-svc",
		"protocol": "http",
		"upstream": "dead-up",
	}, http.StatusCreated)

	mustAdminPostJSON(t, adminClient, adminAddr, adminToken, "/admin/api/v1/routes", map[string]any{
		"id":      "dead-route",
		"name":    "dead-route",
		"service": "dead-svc",
		"paths":   []string{"/dead"},
		"methods": []string{"GET"},
	}, http.StatusCreated)

	time.Sleep(200 * time.Millisecond)

	// Request should fail with 5xx (gateway alive but upstream unreachable)
	gwClient := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "http://"+gwAddr+"/dead", nil)
	resp, err := gwClient.Do(req)
	if err != nil {
		t.Fatalf("request failed (gateway may have crashed): %v", err)
	}
	if resp.StatusCode < 500 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 5xx for unreachable upstream, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Gateway should still be alive
	time.Sleep(100 * time.Millisecond)
	req2, _ := http.NewRequest("GET", "http://"+gwAddr+"/dead", nil)
	resp2, err := gwClient.Do(req2)
	if err != nil {
		t.Fatalf("post-failure request failed (gateway crashed): %v", err)
	}
	resp2.Body.Close()
}

// --- helpers ---

func waitAndGetAdminToken(t *testing.T, adminAddr, key string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		token, err := tryGetAdminToken(adminAddr, key)
		if err == nil {
			return token
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("failed to get admin token after 15s")
	return ""
}

func tryGetAdminToken(adminAddr, key string) (string, error) {
	req, _ := http.NewRequest("POST", "http://"+adminAddr+"/admin/api/v1/auth/token", nil)
	req.Header.Set("X-Admin-Key", key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &http.ProtocolError{}
	}
	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.Token, nil
}

func mustAdminPostJSON(t *testing.T, client *http.Client, adminAddr, token, path string, payload any, expectedStatus int) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "http://"+adminAddr+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("admin POST %s error: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin POST %s: status = %d, want %d, body = %s", path, resp.StatusCode, expectedStatus, string(b))
	}
}
