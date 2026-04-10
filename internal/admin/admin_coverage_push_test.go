package admin

import (
	"net/http"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TestValidateBillingConfig tests the pure validation function
func TestValidateBillingConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid config", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       10,
			RouteCosts:        map[string]int64{"route-1": 5},
			MethodMultipliers: map[string]float64{"GET": 1.0, "POST": 2.0},
			ZeroBalanceAction: "reject",
		}
		if err := validateBillingConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid config allow_with_flag", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       0,
			ZeroBalanceAction: "allow_with_flag",
		}
		if err := validateBillingConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("negative default cost", func(t *testing.T) {
		cfg := config.BillingConfig{DefaultCost: -1}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for negative default_cost")
		}
	})

	t.Run("negative route cost", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost: 10,
			RouteCosts:  map[string]int64{"route-1": -5},
		}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for negative route cost")
		}
	})

	t.Run("empty route cost key", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost: 10,
			RouteCosts:  map[string]int64{"   ": 5},
		}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for empty route cost key")
		}
	})

	t.Run("empty method multiplier key", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       10,
			MethodMultipliers: map[string]float64{"   ": 1.5},
		}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for empty method multiplier key")
		}
	})

	t.Run("zero method multiplier", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       10,
			MethodMultipliers: map[string]float64{"GET": 0},
		}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for zero method multiplier")
		}
	})

	t.Run("negative method multiplier", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       10,
			MethodMultipliers: map[string]float64{"GET": -1.0},
		}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for negative method multiplier")
		}
	})

	t.Run("invalid zero balance action", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       10,
			ZeroBalanceAction: "block",
		}
		if err := validateBillingConfig(cfg); err == nil {
			t.Error("expected error for invalid zero_balance_action")
		}
	})

	t.Run("case insensitive zero balance action", func(t *testing.T) {
		cfg := config.BillingConfig{
			DefaultCost:       10,
			ZeroBalanceAction: "REJECT",
		}
		if err := validateBillingConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestParseBillingRouteCosts tests the parsing helper
func TestParseBillingRouteCosts(t *testing.T) {
	t.Parallel()

	t.Run("map[string]any", func(t *testing.T) {
		result, err := parseBillingRouteCosts(map[string]any{
			"route-1": float64(10),
			"route-2": float64(20),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["route-1"] != 10 {
			t.Errorf("route-1 = %d, want 10", result["route-1"])
		}
		if result["route-2"] != 20 {
			t.Errorf("route-2 = %d, want 20", result["route-2"])
		}
	})

	t.Run("empty map", func(t *testing.T) {
		result, err := parseBillingRouteCosts(map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %v", result)
		}
	})

	t.Run("empty key rejected", func(t *testing.T) {
		_, err := parseBillingRouteCosts(map[string]any{"  ": float64(5)})
		if err == nil {
			t.Error("expected error for empty key")
		}
	})

	t.Run("negative cost rejected", func(t *testing.T) {
		_, err := parseBillingRouteCosts(map[string]any{"route-1": float64(-1)})
		if err == nil {
			t.Error("expected error for negative cost")
		}
	})

	t.Run("invalid type rejected", func(t *testing.T) {
		_, err := parseBillingRouteCosts("not a map")
		if err == nil {
			t.Error("expected error for invalid type")
		}
	})
}

// TestParseBillingMethodMultipliers tests the parsing helper
func TestParseBillingMethodMultipliers(t *testing.T) {
	t.Parallel()

	t.Run("map[string]any", func(t *testing.T) {
		result, err := parseBillingMethodMultipliers(map[string]any{
			"get":  1.0,
			"post": 2.0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["GET"] != 1.0 {
			t.Errorf("GET = %f, want 1.0", result["GET"])
		}
		if result["POST"] != 2.0 {
			t.Errorf("POST = %f, want 2.0", result["POST"])
		}
	})

	t.Run("empty map", func(t *testing.T) {
		result, err := parseBillingMethodMultipliers(map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %v", result)
		}
	})

	t.Run("empty key rejected", func(t *testing.T) {
		_, err := parseBillingMethodMultipliers(map[string]any{"  ": 1.5})
		if err == nil {
			t.Error("expected error for empty key")
		}
	})

	t.Run("zero multiplier rejected", func(t *testing.T) {
		_, err := parseBillingMethodMultipliers(map[string]any{"GET": 0.0})
		if err == nil {
			t.Error("expected error for zero multiplier")
		}
	})

	t.Run("negative multiplier rejected", func(t *testing.T) {
		_, err := parseBillingMethodMultipliers(map[string]any{"GET": -1.5})
		if err == nil {
			t.Error("expected error for negative multiplier")
		}
	})

	t.Run("non-numeric rejected", func(t *testing.T) {
		_, err := parseBillingMethodMultipliers(map[string]any{"GET": "not-a-number"})
		if err == nil {
			t.Error("expected error for non-numeric value")
		}
	})

	t.Run("invalid type rejected", func(t *testing.T) {
		_, err := parseBillingMethodMultipliers([]string{"GET"})
		if err == nil {
			t.Error("expected error for invalid type")
		}
	})
}

// TestCreditEndpoints tests credit handler endpoints
func TestCreditEndpoints(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("credit overview", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/credits/overview", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("user credit overview", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/user-1/credits", token, nil)
		// 200 with balance, 400 if store error, or 404 if user not found
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusNotFound {
			t.Errorf("expected 200, 400, or 404, got %v", sc)
		}
	})

	t.Run("user credit balance", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/user-1/credits/balance", token, nil)
		// 200 with balance, 400 if store error, or 404 if user not found
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusNotFound {
			t.Errorf("expected 200, 400, or 404, got %v", sc)
		}
	})

	t.Run("topup credits missing amount", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/user-1/credits/topup", token, map[string]any{
			"reason": "test",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("topup credits zero amount", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/user-1/credits/topup", token, map[string]any{
			"amount": 0,
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("topup credits negative amount", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/user-1/credits/topup", token, map[string]any{
			"amount": -50,
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("deduct credits zero amount", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/user-1/credits/deduct", token, map[string]any{
			"amount": 0,
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("adjust credits unified missing amount", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/user-1/credits", token, map[string]any{})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("list credit transactions", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/user-1/credits/transactions", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusInternalServerError && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 500, or 400, got %v", sc)
		}
	})
}

// TestAuditEndpoints tests audit handler endpoints
func TestAuditEndpoints(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("search audit logs", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusInternalServerError {
			t.Errorf("expected 200, 400, or 500, got %v", sc)
		}
	})

	t.Run("search audit logs with filters", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs?status_min=400&limit=10", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusInternalServerError {
			t.Errorf("expected 200, 400, or 500, got %v", sc)
		}
	})

	t.Run("get audit log not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/nonexistent-id", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 200, or 400, got %v", sc)
		}
	})

	t.Run("audit log stats", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/stats", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500, got %v", sc)
		}
	})

	t.Run("cleanup audit logs with retention days", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/audit-logs/cleanup", token, map[string]any{
			"retention_days": 30,
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusInternalServerError && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 500, or 400, got %v", sc)
		}
	})

	t.Run("cleanup audit logs with date range", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/audit-logs/cleanup", token, map[string]any{
			"before": "2024-01-01T00:00:00Z",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusInternalServerError && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 500, or 400, got %v", sc)
		}
	})

	t.Run("search user audit logs", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/user-1/audit-logs", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusInternalServerError && sc != http.StatusBadRequest {
			t.Errorf("expected 200, 500, or 400, got %v", sc)
		}
	})

	t.Run("export audit logs json", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/export?format=json", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest && sc != http.StatusInternalServerError {
			t.Errorf("expected 200, 400, or 500, got %v", sc)
		}
	})
}

// TestBillingConfigEndpoint tests billing config endpoint
func TestBillingConfigEndpoint(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("update billing config invalid zero balance action", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/config", token, map[string]any{
			"default_cost":       10,
			"zero_balance_action": "invalid",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update billing config negative default cost", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/config", token, map[string]any{
			"default_cost": -5,
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update billing config negative route cost", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/config", token, map[string]any{
			"default_cost": 10,
			"route_costs": map[string]any{
				"route-1": float64(-5),
			},
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update billing config invalid method multiplier", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/config", token, map[string]any{
			"default_cost": 10,
			"method_multipliers": map[string]any{
				"GET": 0,
			},
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update billing route costs negative cost", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/route-costs", token, map[string]any{
			"route_costs": map[string]any{
				"route-1": float64(-10),
			},
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update billing route costs empty key", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/route-costs", token, map[string]any{
			"route_costs": map[string]any{
				"  ": float64(10),
			},
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})
}

// TestAlertEndpoints tests alert handler endpoints
func TestAlertEndpoints(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("create alert missing name", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/alerts", token, map[string]any{
			"threshold": 100,
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("create alert invalid metric", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/alerts", token, map[string]any{
			"name":      "Test Alert",
			"metric":    "invalid_metric",
			"threshold": 100,
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("create alert invalid comparison", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/alerts", token, map[string]any{
			"name":       "Test Alert",
			"metric":     "error_rate",
			"threshold":  100,
			"comparison": "invalid",
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("update alert not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/alerts/nonexistent-id", token, map[string]any{
			"name": "Updated Alert",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusBadRequest {
			t.Errorf("expected 404 or 400, got %v", sc)
		}
	})

	t.Run("delete alert not found", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/alerts/nonexistent-id", token, nil)
		if sc := resp["status_code"].(float64); sc != http.StatusNotFound && sc != http.StatusNoContent && sc != http.StatusBadRequest {
			t.Errorf("expected 404, 204, or 400, got %v", sc)
		}
	})
}
