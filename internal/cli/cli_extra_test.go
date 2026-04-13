package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunUser_WithMockServer tests user commands with mock admin server
func TestRunUser_WithMockServer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/api-keys"):
			if r.Method == http.MethodPost {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "key-new", "key_prefix": "ck_live_abc", "name": "New Key", "status": "active",
				})
			} else if r.Method == http.MethodDelete {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "key-1"})
			} else {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "key-1", "key_prefix": "ck_live_abc", "name": "Production Key", "status": "active"},
				})
			}
		case strings.Contains(r.URL.Path, "/permissions"):
			if r.Method == http.MethodPost {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm-new"})
			} else if r.Method == http.MethodDelete {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm-1"})
			} else {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "perm-1", "route_id": "route-1", "methods": "GET,POST", "allowed": true},
				})
			}
		case strings.Contains(r.URL.Path, "/ip-whitelist"):
			if r.Method == http.MethodPost {
				_ = json.NewEncoder(w).Encode(map[string]any{"ip": "192.168.1.100"})
			} else if r.Method == http.MethodDelete {
				_ = json.NewEncoder(w).Encode(map[string]any{"ip": "192.168.1.100"})
			} else {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"ip": "192.168.1.1/24"},
				})
			}
		case strings.Contains(r.URL.Path, "/reset-password"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1"})
		case strings.Contains(r.URL.Path, "/suspend"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1", "status": "suspended"})
		case strings.Contains(r.URL.Path, "/activate"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1", "status": "active"})
		case strings.Contains(r.URL.Path, "/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1", "status": "active"})
		case strings.HasSuffix(r.URL.Path, "/users/user-1"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "user-1", "email": "user@example.com", "name": "Test User", "role": "user", "status": "active",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	t.Run("user apikey list", func(t *testing.T) {
		err := runUser([]string{"apikey", "list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("user apikey list error: %v", err)
		}
	})
	t.Run("user apikey list json", func(t *testing.T) {
		err := runUser([]string{"apikey", "list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--output", "json"})
		if err != nil {
			t.Errorf("user apikey list json error: %v", err)
		}
	})
	t.Run("user apikey create", func(t *testing.T) {
		err := runUser([]string{"apikey", "create", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--name", "Test Key"})
		if err != nil {
			t.Errorf("user apikey create error: %v", err)
		}
	})
	t.Run("user apikey revoke", func(t *testing.T) {
		err := runUser([]string{"apikey", "revoke", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--key", "key-1"})
		if err != nil {
			t.Errorf("user apikey revoke error: %v", err)
		}
	})
	t.Run("user permission list", func(t *testing.T) {
		err := runUser([]string{"permission", "list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("user permission list error: %v", err)
		}
	})
	t.Run("user permission list json", func(t *testing.T) {
		err := runUser([]string{"permission", "list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--output", "json"})
		if err != nil {
			t.Errorf("user permission list json error: %v", err)
		}
	})
	t.Run("user permission grant", func(t *testing.T) {
		err := runUser([]string{"permission", "grant", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--route", "route-1"})
		if err != nil {
			t.Errorf("user permission add error: %v", err)
		}
	})
	t.Run("user permission revoke", func(t *testing.T) {
		err := runUser([]string{"permission", "revoke", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--permission", "perm-1"})
		if err != nil {
			t.Errorf("user permission revoke error: %v", err)
		}
	})
	t.Run("user ip list", func(t *testing.T) {
		err := runUser([]string{"ip", "list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("user ip list error: %v", err)
		}
	})
	t.Run("user ip list json", func(t *testing.T) {
		err := runUser([]string{"ip", "list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--output", "json"})
		if err != nil {
			t.Errorf("user ip list json error: %v", err)
		}
	})
	t.Run("user ip add", func(t *testing.T) {
		err := runUser([]string{"ip", "add", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--ip", "192.168.1.100"})
		if err != nil {
			t.Errorf("user ip add error: %v", err)
		}
	})
	t.Run("user ip remove", func(t *testing.T) {
		err := runUser([]string{"ip", "remove", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--ip", "192.168.1.100"})
		if err != nil {
			t.Errorf("user ip remove error: %v", err)
		}
	})
	t.Run("user suspend", func(t *testing.T) {
		err := runUser([]string{"suspend", "--admin-url", upstream.URL, "--admin-key", "test-key", "user-1"})
		if err != nil {
			t.Errorf("user suspend error: %v", err)
		}
	})
	t.Run("user activate", func(t *testing.T) {
		err := runUser([]string{"activate", "--admin-url", upstream.URL, "--admin-key", "test-key", "user-1"})
		if err != nil {
			t.Errorf("user activate error: %v", err)
		}
	})
	t.Run("user reset-password unknown", func(t *testing.T) {
		err := runUser([]string{"reset-password", "--admin-url", upstream.URL, "--admin-key", "test-key", "user-1", "newpass123"})
		if err == nil {
			t.Error("expected error for unknown subcommand")
		}
	})
	t.Run("user get", func(t *testing.T) {
		err := runUser([]string{"get", "--admin-url", upstream.URL, "--admin-key", "test-key", "user-1"})
		if err != nil {
			t.Errorf("user get error: %v", err)
		}
	})
	t.Run("user apikey list empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		}))
		defer srv.Close()
		err := runUser([]string{"apikey", "list", "--admin-url", srv.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("user apikey list empty error: %v", err)
		}
	})
	t.Run("user permission list empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		}))
		defer srv.Close()
		err := runUser([]string{"permission", "list", "--admin-url", srv.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("user permission list empty error: %v", err)
		}
	})
	t.Run("user ip list empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		}))
		defer srv.Close()
		err := runUser([]string{"ip", "list", "--admin-url", srv.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("user ip list empty error: %v", err)
		}
	})
}

// TestRunAnalytics_LatencyWithServer tests analytics latency with mock server
func TestRunAnalytics_LatencyWithServer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"p50_ms": 10, "p95_ms": 50, "p99_ms": 100, "avg_ms": 25, "count": 1000,
		})
	}))
	defer upstream.Close()

	t.Run("latency basic", func(t *testing.T) {
		err := runAnalytics([]string{"latency", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("analytics latency error: %v", err)
		}
	})
	t.Run("latency json", func(t *testing.T) {
		err := runAnalytics([]string{"latency", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("analytics latency json error: %v", err)
		}
	})
	t.Run("latency with window", func(t *testing.T) {
		err := runAnalytics([]string{"latency", "--admin-url", upstream.URL, "--admin-key", "test-key", "--window", "1h"})
		if err != nil {
			t.Errorf("analytics latency with window error: %v", err)
		}
	})
	t.Run("latency with time range", func(t *testing.T) {
		err := runAnalytics([]string{"latency", "--admin-url", upstream.URL, "--admin-key", "test-key", "--from", "1h", "--to", "now"})
		if err != nil {
			t.Errorf("analytics latency with time range error: %v", err)
		}
	})
}
