package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestGatewayHandleHealth_HealthEndpoint(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: ":0",
		},
		Admin: config.AdminConfig{
			Addr:        ":0",
			APIKey:      "test-admin-api-key-at-least-32-chars!!",
			TokenSecret: "test-admin-token-secret-at-least-32-chars",
		},
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	defer g.Shutdown(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	g.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if _, ok := resp["uptime"]; !ok {
		t.Error("expected uptime in response")
	}
}

func TestGatewayHandleHealth_ReadyEndpoint(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: ":0",
		},
		Admin: config.AdminConfig{
			Addr:        ":0",
			APIKey:      "test-admin-api-key-at-least-32-chars!!",
			TokenSecret: "test-admin-token-secret-at-least-32-chars",
		},
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	defer g.Shutdown(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	g.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
}

func TestGatewayHandleHealth_UnknownPath(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{HTTPAddr: ":0"},
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
		},
	}

	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	defer g.Shutdown(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/not-health", nil)
	rec := httptest.NewRecorder()

	handled := g.handleHealth(rec, req)
	if handled {
		t.Fatal("expected handleHealth to return false for non-health path")
	}
	if rec.Body.Len() > 0 {
		t.Errorf("expected no response body, got: %s", rec.Body.String())
	}
}
