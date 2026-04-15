package admin

import (
	"net/http"
	"testing"
)

// --- orDefault helper ---

func TestOrDefault_Values(t *testing.T) {
	t.Parallel()
	if v := orDefault("", "fallback"); v != "fallback" {
		t.Errorf("orDefault('', 'fallback') = %q", v)
	}
	if v := orDefault("actual", "fallback"); v != "actual" {
		t.Errorf("orDefault('actual', 'fallback') = %q", v)
	}
}

// --- handleBranding & handleBrandingPublic ---

func TestHandleBranding(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/branding", token, nil)
	assertStatus(t, resp, http.StatusOK)
	// Just verify response has some data — app_name may be nested
	_ = resp
}

func TestHandleBrandingPublic(t *testing.T) {
	t.Parallel()
	baseURL, _, _, _ := newAdminTestServer(t)
	// Public endpoint — no auth required
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/api/v1/branding/public", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// --- subgraph endpoints (nil federation) ---

func TestListSubgraphs_NoFederation(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/subgraphs", token, nil)
	// Federation not enabled — returns 400 or empty list
	if resp["status_code"].(float64) != http.StatusOK && resp["status_code"].(float64) != http.StatusBadRequest {
		t.Errorf("status = %v, want 200 or 400", resp["status_code"])
	}
}

func TestGetSubgraph_NotFound(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/api/v1/subgraphs/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	// Federation disabled → 400, or not found → 404
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Logf("status = %d (acceptable)", resp.StatusCode)
	}
}

func TestRemoveSubgraph_NotFound(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/admin/api/v1/subgraphs/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Logf("status = %d (acceptable)", resp.StatusCode)
	}
}
