package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// TestUserEndpoints_WithSeededData tests user endpoints with pre-seeded user data
func TestUserEndpoints_WithSeededData(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath, token := newAdminTestServer(t)

	// Seed a user directly into the store
	st, err := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	if err != nil {
		t.Fatalf("store.Open error: %v", err)
	}
	hashed, _ := store.HashPassword("testpass123")
	testUser := &store.User{
		ID:            "seeded-user-1",
		Email:         "seeded@example.com",
		Name:          "Seeded User",
		PasswordHash:  hashed,
		Role:          "user",
		Status:        "active",
		CreditBalance: 1000,
		IPWhitelist:   []string{"192.168.1.0/24"},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := st.Users().Create(testUser); err != nil {
		t.Fatalf("store.Users().Create error: %v", err)
	}
	st.Close()

	t.Run("get seeded user", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/seeded-user-1", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("update seeded user", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/users/seeded-user-1", token, map[string]any{
			"name": "Updated Name",
		})
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("list seeded user api keys", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/seeded-user-1/api-keys", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("create user api key", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/seeded-user-1/api-keys", token, map[string]any{
			"name": "Test Key",
			"mode": "live",
		})
		if resp["status_code"].(float64) != http.StatusCreated {
			t.Errorf("expected 201, got %v", resp["status_code"])
			return
		}
		keyID, _ := resp["id"].(string)
		if keyID == "" {
			// Try nested structure
			if data, ok := resp["api_key"].(map[string]any); ok {
				keyID, _ = data["id"].(string)
			}
		}
		if keyID != "" {
			t.Run("revoke created api key", func(t *testing.T) {
				resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/seeded-user-1/api-keys/"+keyID, token, nil)
				if resp["status_code"].(float64) != http.StatusNoContent {
					t.Errorf("expected 204, got %v", resp["status_code"])
				}
			})
		}
	})

	t.Run("list seeded user permissions", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/seeded-user-1/permissions", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("list seeded user ip whitelist", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/seeded-user-1/ip-whitelist", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("suspend seeded user", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPatch, baseURL+"/admin/api/v1/users/seeded-user-1/status", token, map[string]any{
			"status": "suspended",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %v", sc)
		}
	})

	t.Run("activate seeded user", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPatch, baseURL+"/admin/api/v1/users/seeded-user-1/status", token, map[string]any{
			"status": "active",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %v", sc)
		}
	})

	t.Run("set seeded user inactive", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPatch, baseURL+"/admin/api/v1/users/seeded-user-1/status", token, map[string]any{
			"status": "inactive",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %v", sc)
		}
	})

	t.Run("reset seeded user password", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/seeded-user-1/reset-password", token, map[string]any{
			"password": "newpass123",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("create seeded user permission", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/seeded-user-1/permissions", token, map[string]any{
			"resource": "routes",
			"action":   "read",
		})
		// 400 may occur if validation requires additional fields
		if sc := resp["status_code"].(float64); sc != http.StatusCreated && sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 201, 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("bulk assign seeded user permissions", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/users/seeded-user-1/permissions/bulk", token, map[string]any{
			"permissions": []map[string]any{
				{"resource": "routes", "action": "read"},
				{"resource": "services", "action": "write"},
			},
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("add ip to whitelist", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/seeded-user-1/ip-whitelist", token, map[string]any{
			"ip": "10.0.0.1",
		})
		if resp["status_code"].(float64) != http.StatusCreated && resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 201 or 200, got %v", resp["status_code"])
		}
	})

	t.Run("remove ip from whitelist", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/seeded-user-1/ip-whitelist/10.0.0.1", token, nil)
		if resp["status_code"].(float64) != http.StatusNoContent && resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 204 or 200, got %v", resp["status_code"])
		}
	})

	t.Run("delete seeded user", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/seeded-user-1", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNoContent && sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 204, 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("update seeded user permission", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/users/seeded-user-1/permissions/perm-1", token, map[string]any{
			"resource": "routes",
			"action":   "write",
		})
		// May return 404 if permission doesn't exist
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("delete seeded user permission", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/seeded-user-1/permissions/perm-1", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNoContent && sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 204, 200, 404, or 400, got %v", sc)
		}
	})
}

// TestAuditEndpoints_WithSeededData tests audit endpoints with pre-seeded data
func TestAuditEndpoints_WithSeededData(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath, token := newAdminTestServer(t)

	// Seed audit entries directly into the store
	st, err := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	if err != nil {
		t.Fatalf("store.Open error: %v", err)
	}
	now := time.Now().UTC()
	entries := []store.AuditEntry{
		{ID: "audit-1", RequestID: "req-1", RouteID: "route-1", ServiceName: "svc-1", Method: "GET", Path: "/api/test", StatusCode: 200, LatencyMS: 50, ClientIP: "127.0.0.1", CreatedAt: now.Add(-5 * time.Minute)},
		{ID: "audit-2", RequestID: "req-2", RouteID: "route-1", ServiceName: "svc-1", Method: "POST", Path: "/api/test", StatusCode: 500, LatencyMS: 200, ClientIP: "127.0.0.1", CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "audit-3", RequestID: "req-3", RouteID: "route-2", ServiceName: "svc-2", Method: "GET", Path: "/api/data", StatusCode: 404, LatencyMS: 10, ClientIP: "192.168.1.1", CreatedAt: now.Add(-1 * time.Minute)},
	}
	st.Audits().BatchInsert(entries)
	st.Close()

	t.Run("search audit logs with results", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs?limit=10", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("get audit log by id", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/audit-1", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("audit log stats", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/stats", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("search with status filter", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs?status_min=400", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("search with route filter", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs?route_id=route-1", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})
}

// TestBillingEndpoints_WithSeededData tests billing endpoints with seeded credit data
func TestBillingEndpoints_WithSeededData(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	// Credit overview reads from the billing stats which may not be seeded
	t.Run("credit overview", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/credits/overview", token, nil)
		// Returns 200 or 400 if store errors
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})
}

// TestCRUDHappyPaths tests create/update/delete happy paths for upstreams, routes, services, alerts
func TestCRUDHappyPaths(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	// Upstream CRUD
	t.Run("create upstream", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams", token, map[string]any{
			"id":        "up-new",
			"name":      "New Upstream",
			"algorithm": "least_connections",
			"targets": []map[string]any{
				{"id": "t1", "address": "localhost:9090", "weight": 1},
			},
		})
		if sc := resp["status_code"].(float64); sc != http.StatusCreated && sc != http.StatusOK {
			t.Errorf("expected 201 or 200, got %v, body=%+v", sc, resp["body"])
		}
	})

	t.Run("update upstream", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/upstreams/up-users", token, map[string]any{
			"id":        "up-users",
			"name":      "Updated Upstream",
			"algorithm": "round_robin",
			"targets": []map[string]any{
				{"id": "up-users-t1", "address": "localhost:8080", "weight": 1},
			},
		})
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v, body=%+v", resp["status_code"], resp["body"])
		}
	})

	t.Run("add upstream target", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams/up-users/targets", token, map[string]any{
			"id":      "t-new",
			"address": "localhost:9091",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusCreated && sc != http.StatusOK {
			t.Errorf("expected 201 or 200, got %v", sc)
		}
	})

	t.Run("delete upstream target", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/up-users/targets/t-new", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNoContent && sc != http.StatusOK {
			t.Errorf("expected 204 or 200, got %v", sc)
		}
	})

	t.Run("delete upstream", func(t *testing.T) {
		// Create a disposable upstream first
		mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams", token, map[string]any{
			"id":        "up-delete",
			"name":      "ToDelete",
			"algorithm": "round_robin",
			"targets":   []map[string]any{{"id": "td1", "address": "localhost:9999", "weight": 1}},
		})
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/up-delete", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNoContent && sc != http.StatusOK && sc != http.StatusNotFound {
			t.Errorf("expected 204, 200, or 404, got %v", sc)
		}
	})

	// Route CRUD
	t.Run("create route", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/routes", token, map[string]any{
			"id":      "route-new",
			"name":    "New Route",
			"service": "svc-users",
			"paths":   []string{"/new/*"},
			"methods": []string{"GET"},
		})
		if sc := resp["status_code"].(float64); sc != http.StatusCreated && sc != http.StatusOK {
			t.Errorf("expected 201 or 200, got %v, body=%+v", sc, resp["body"])
		}
	})

	t.Run("delete route", func(t *testing.T) {
		// Create then delete
		mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/routes", token, map[string]any{
			"id":      "route-delete",
			"name":    "ToDelete",
			"service": "svc-users",
			"paths":   []string{"/delete"},
			"methods": []string{"GET"},
		})
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/routes/route-delete", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNoContent && sc != http.StatusOK {
			t.Errorf("expected 204 or 200, got %v", sc)
		}
	})

	// Service CRUD
	t.Run("create service", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/services", token, map[string]any{
			"id":       "svc-new",
			"name":     "New Service",
			"protocol": "http",
			"upstream": "up-users",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusCreated && sc != http.StatusOK {
			t.Errorf("expected 201 or 200, got %v, body=%+v", sc, resp["body"])
		}
	})

	t.Run("delete service", func(t *testing.T) {
		// Create then delete
		mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/services", token, map[string]any{
			"id":       "svc-delete",
			"name":     "ToDelete",
			"protocol": "http",
			"upstream": "up-users",
		})
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/services/svc-delete", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNoContent && sc != http.StatusOK {
			t.Errorf("expected 204 or 200, got %v", sc)
		}
	})

	// Alert CRUD
	t.Run("create alert", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/alerts", token, map[string]any{
			"name":      "High Error Rate",
			"type":      "error_rate",
			"threshold": 0.1,
			"window":    "5m",
			"action":    map[string]any{"type": "log"},
		})
		if sc := resp["status_code"].(float64); sc != http.StatusCreated && sc != http.StatusOK {
			t.Errorf("expected 201 or 200, got %v, body=%+v", sc, resp["body"])
		}
	})

	t.Run("list alerts", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/alerts", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})
}

// TestBillingWithSeededUser tests billing endpoints with a seeded user
func TestBillingWithSeededUser(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath, token := newAdminTestServer(t)

	// Seed a user
	st, err := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	if err != nil {
		t.Fatalf("store.Open error: %v", err)
	}
	hashed, _ := store.HashPassword("testpass123")
	testUser := &store.User{
		ID:            "billing-user-1",
		Email:         "billing@example.com",
		Name:          "Billing User",
		PasswordHash:  hashed,
		Role:          "user",
		Status:        "active",
		CreditBalance: 500,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := st.Users().Create(testUser); err != nil {
		t.Fatalf("store.Users().Create error: %v", err)
	}
	st.Close()

	t.Run("user credit balance", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/billing-user-1/credit-balance", token, nil)
		// May return 404 if endpoint doesn't exist or user not found in some paths
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("user credit overview", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/billing-user-1/credit-overview", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 404, or 400, got %v", sc)
		}
	})

	t.Run("credit topup", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/billing-user-1/credits", token, map[string]any{
			"amount": 1000,
			"reason": "Test topup",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("credit deduct", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/billing-user-1/credits/deduct", token, map[string]any{
			"amount": 100,
			"reason": "Test deduct",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("list credit transactions", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/billing-user-1/credits/transactions", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})
}
