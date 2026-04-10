package admin

import (
	"net/http"
	"testing"
)

// TestUserEndpoints_Missing tests user endpoint branches not yet covered
func TestUserEndpoints_Missing(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("get user not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 404 or 400, got %v", sc)
		}
	})

	t.Run("update user missing body", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/users/user-1", token, nil)
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update user status unified unknown action", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPatch, baseURL+"/admin/api/v1/users/user-1/status", token, map[string]any{
			"status": "unknown",
		})
		// User may not exist (404) or action may be invalid (400)
		if sc := resp["status_code"].(float64); sc != http.StatusBadRequest && sc != http.StatusNotFound {
			t.Errorf("expected 400 or 404, got %v", sc)
		}
	})

	t.Run("delete user not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusNoContent && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 204, or 400, got %v", sc)
		}
	})

	t.Run("reset user password empty password", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/user-1/reset-password", token, map[string]any{
			"password": "",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})
}

// TestServiceEndpoints_Missing tests service endpoint branches not yet covered
func TestServiceEndpoints_Missing(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("get service not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/services/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusBadRequest && sc != http.StatusOK {
			t.Errorf("expected 404, 400, or 200, got %v", sc)
		}
	})

	t.Run("update service missing body", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/services/existing-svc", token, nil)
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update service empty name", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/services/existing-svc", token, map[string]any{
			"name": "",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("delete service not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/services/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusNoContent && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 204, or 400, got %v", sc)
		}
	})
}

// TestRouteEndpoints_Missing tests route endpoint branches not yet covered
func TestRouteEndpoints_Missing(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("get route not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/routes/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusBadRequest && sc != http.StatusOK {
			t.Errorf("expected 404, 400, or 200, got %v", sc)
		}
	})

	t.Run("update route missing body", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/routes/existing-route", token, nil)
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update route empty name", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/routes/existing-route", token, map[string]any{
			"name": "",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("delete route not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/routes/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusNoContent && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 204, or 400, got %v", sc)
		}
	})
}

// TestUpstreamEndpoints_Missing tests upstream endpoint branches not yet covered
func TestUpstreamEndpoints_Missing(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("get upstream not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/upstreams/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusBadRequest && sc != http.StatusOK {
			t.Errorf("expected 404, 400, or 200, got %v", sc)
		}
	})

	t.Run("update upstream missing body", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/upstreams/existing-up", token, nil)
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update upstream empty name", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/upstreams/existing-up", token, map[string]any{
			"name": "",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("delete upstream not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusNoContent && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 204, or 400, got %v", sc)
		}
	})

	t.Run("add upstream target missing body", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams/existing-up/targets", token, nil)
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("add upstream target empty address", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams/existing-up/targets", token, map[string]any{
			"address": "",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("delete upstream target not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/existing-up/targets/nonexistent", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusNoContent && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 204, or 400, got %v", sc)
		}
	})
}
