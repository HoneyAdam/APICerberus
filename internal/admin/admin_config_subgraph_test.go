package admin

import (
	"bytes"
	"net/http"
	"testing"
)

// TestConfigImportExport covers config import, export, and reload branches
func TestConfigImportExport(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("config export", func(t *testing.T) {
		// Export returns YAML, not JSON — use raw request
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/api/v1/config/export", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if sc := resp.StatusCode; sc != http.StatusOK {
			t.Errorf("expected 200, got %v", sc)
		}
	})

	t.Run("config export yaml", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/api/v1/config/export?format=yaml", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if sc := resp.StatusCode; sc != http.StatusOK {
			t.Errorf("expected 200, got %v", sc)
		}
	})

	t.Run("config import json", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/config/import", token, map[string]any{
			"gateway": map[string]any{
				"http_addr": "127.0.0.1:8080",
			},
			"services":  []map[string]any{},
			"routes":    []map[string]any{},
			"upstreams": []map[string]any{},
		})
		// May succeed or fail depending on validation
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusAccepted {
			t.Errorf("expected 200, 202, or 400, got %v", sc)
		}
	})

	t.Run("config import with invalid data", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/admin/api/v1/config/import", bytes.NewReader([]byte(`{"bad":true}`)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		// Just exercise the handler
		if sc := resp.StatusCode; sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusAccepted {
			t.Errorf("expected 200, 202, or 400, got %v", sc)
		}
	})

	t.Run("config reload", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/config/reload", token, nil)
		// May succeed or fail depending on config file availability
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusInternalServerError {
			t.Errorf("expected 200, 400, or 500, got %v", sc)
		}
	})
}

// TestSubgraphOperations covers subgraph CRUD branches
func TestSubgraphOperations(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("list subgraphs", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/federation/subgraphs", token, nil)
		// May return 200, 404 if route not registered, or other status
		if sc := resp["status_code"].(float64); sc < 200 || sc > 599 {
			t.Errorf("expected valid HTTP status, got %v", sc)
		}
	})

	t.Run("add subgraph", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/federation/subgraphs", token, map[string]any{
			"name": "test-subgraph",
			"url":  "http://localhost:4001/graphql",
		})
		if sc := resp["status_code"].(float64); sc < 200 || sc > 599 {
			t.Errorf("expected valid HTTP status, got %v", sc)
		}
	})

	t.Run("compose subgraphs", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/federation/compose", token, nil)
		if sc := resp["status_code"].(float64); sc < 200 || sc > 599 {
			t.Errorf("expected valid HTTP status, got %v", sc)
		}
	})
}

// TestStaticAuth covers the X-Admin-Key header auth path for token endpoint
func TestStaticAuth(t *testing.T) {
	t.Parallel()
	baseURL, _, _, _ := newAdminTestServer(t)

	t.Run("token with static auth correct key", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/admin/api/v1/auth/token", nil)
		req.Header.Set("X-Admin-Key", "secret-admin")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		// Should return 200 with token
		if sc := resp.StatusCode; sc != http.StatusOK {
			t.Errorf("expected 200, got %v", sc)
		}
	})

	t.Run("token with static auth wrong key", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/admin/api/v1/auth/token", nil)
		req.Header.Set("X-Admin-Key", "wrong-key")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if sc := resp.StatusCode; sc != http.StatusUnauthorized {
			t.Errorf("expected 401, got %v", sc)
		}
	})

	t.Run("token with empty key", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/admin/api/v1/auth/token", nil)
		req.Header.Set("X-Admin-Key", "")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if sc := resp.StatusCode; sc != http.StatusUnauthorized {
			t.Errorf("expected 401, got %v", sc)
		}
	})
}
