package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// rawRequest sends a JSON request and returns the status code and parsed body.
// Unlike mustJSONRequest, this takes raw JSON bytes to avoid double-marshal.
func rawRequest(t *testing.T, method, url, token string, body []byte) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	result := map[string]any{"status_code": float64(resp.StatusCode)}
	if resp.ContentLength != 0 && resp.StatusCode != http.StatusNoContent {
		var body any
		if json.NewDecoder(resp.Body).Decode(&body) == nil {
			result["body"] = body
		}
	}
	return resp.StatusCode, result
}

func TestBulkImport_InvalidPayload(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, nil)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestBulkImport_CreateUpstream(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	payload := map[string]any{
		"upstreams": []config.Upstream{
			{
				ID:        "up-test-1",
				Name:      "test-upstream",
				Algorithm: "round_robin",
				Targets:   []config.UpstreamTarget{{ID: "t1", Address: "http://localhost:3000", Weight: 1}},
			},
		},
	}
	body, _ := json.Marshal(payload)
	status, resp := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, resp = %v", status, resp)
	}
}

func TestBulkImport_UpsertMode(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	up := config.Upstream{
		ID:        "up-upsert-1",
		Name:      "upstream-1",
		Algorithm: "round_robin",
		Targets:   []config.UpstreamTarget{{ID: "t1", Address: "http://localhost:3000", Weight: 1}},
	}
	// Create first
	body1, _ := json.Marshal(map[string]any{"mode": "upsert", "upstreams": []config.Upstream{up}})
	status, _ := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body1)
	if status != http.StatusOK {
		t.Fatalf("first upsert: status = %d", status)
	}

	// Upsert same ID with new name
	up.Name = "upstream-1-updated"
	body2, _ := json.Marshal(map[string]any{"mode": "upsert", "upstreams": []config.Upstream{up}})
	status, resp := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body2)
	if status != http.StatusOK {
		t.Fatalf("second upsert: status = %d, resp = %v", status, resp)
	}
}

func TestBulkImport_UpstreamValidation(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"upstreams": []config.Upstream{
			{ID: "up-bad-1", Name: "", Targets: nil},
		},
	})
	status, resp := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, resp = %v", status, resp)
	}
}

func TestBulkImport_CreateMode_Skipped(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	up := config.Upstream{
		ID:        "up-create-1",
		Name:      "test-up",
		Algorithm: "round_robin",
		Targets:   []config.UpstreamTarget{{ID: "t1", Address: "http://localhost:3000", Weight: 1}},
	}
	body, _ := json.Marshal(map[string]any{"mode": "create", "upstreams": []config.Upstream{up}})

	// First create — should succeed
	status, _ := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body)
	if status != http.StatusOK {
		t.Fatalf("first create: status = %d", status)
	}

	// Second create with same ID — should skip
	status, resp := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body)
	if status != http.StatusOK {
		t.Fatalf("second create: status = %d, resp = %v", status, resp)
	}
}

func TestBulkImport_InvalidAlgorithm(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"upstreams": []config.Upstream{
			{
				ID:        "up-alg-1",
				Name:      "test-up",
				Algorithm: "invalid_algo",
				Targets:   []config.UpstreamTarget{{ID: "t1", Address: "http://localhost:3000", Weight: 1}},
			},
		},
	})
	status, resp := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, resp = %v", status, resp)
	}
}

func TestBulkImport_NoAuth(t *testing.T) {
	t.Parallel()
	baseURL, _, _, _ := newAdminTestServer(t)
	body, _ := json.Marshal(map[string]any{"upstreams": []any{}})
	status, _ := rawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", "", body)
	if status == http.StatusOK {
		t.Error("expected non-200 without auth")
	}
}
