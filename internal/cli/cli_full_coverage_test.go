package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunDB_ErrorPaths tests runDB and runDBMigrate error paths
func TestRunDB_ErrorPaths(t *testing.T) {
	t.Run("missing db subcommand", func(t *testing.T) {
		err := runDB([]string{})
		if err == nil {
			t.Error("expected error for missing db subcommand")
		}
		if !strings.Contains(err.Error(), "missing db subcommand") {
			t.Errorf("error should mention missing db subcommand, got: %v", err)
		}
	})

	t.Run("unknown db subcommand", func(t *testing.T) {
		err := runDB([]string{"unknown"})
		if err == nil {
			t.Error("expected error for unknown db subcommand")
		}
		if !strings.Contains(err.Error(), `unknown db subcommand "unknown"`) {
			t.Errorf("error should mention unknown subcommand, got: %v", err)
		}
	})

	t.Run("missing migrate subcommand", func(t *testing.T) {
		err := runDBMigrate([]string{})
		if err == nil {
			t.Error("expected error for missing migrate subcommand")
		}
		if !strings.Contains(err.Error(), "missing migrate subcommand") {
			t.Errorf("error should mention missing migrate subcommand, got: %v", err)
		}
	})

	t.Run("unknown migrate subcommand", func(t *testing.T) {
		err := runDBMigrate([]string{"unknown"})
		if err == nil {
			t.Error("expected error for unknown migrate subcommand")
		}
		if !strings.Contains(err.Error(), `unknown migrate subcommand "unknown"`) {
			t.Errorf("error should mention unknown subcommand, got: %v", err)
		}
	})

	t.Run("migrate status missing config", func(t *testing.T) {
		err := runDBMigrateStatus([]string{"--config", "/nonexistent/config.yaml"})
		if err == nil {
			t.Error("expected error for missing config file")
		}
		if !strings.Contains(err.Error(), "load config") {
			t.Errorf("error should mention load config, got: %v", err)
		}
	})

	t.Run("migrate apply missing config", func(t *testing.T) {
		err := runDBMigrateApply([]string{"--config", "/nonexistent/config.yaml"})
		if err == nil {
			t.Error("expected error for missing config file")
		}
		if !strings.Contains(err.Error(), "load config") {
			t.Errorf("error should mention load config, got: %v", err)
		}
	})
}

// TestRunDB_WithRealConfig tests migration with an actual config file
func TestRunDB_WithRealConfig(t *testing.T) {
	// Create a temporary directory with a minimal config and SQLite
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	cfg := `
admin:
  api_key: test-admin-key-that-is-at-least-32-chars
  token_secret: test-token-secret-that-is-at-least-32-characters
store:
  path: ` + dbPath + `
  busy_timeout: 5s
  journal_mode: WAL
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Run("migrate status with valid config", func(t *testing.T) {
		err := runDBMigrateStatus([]string{"--config", cfgPath})
		if err != nil {
			t.Errorf("runDBMigrateStatus error: %v", err)
		}
	})

	t.Run("migrate apply with valid config", func(t *testing.T) {
		err := runDBMigrateApply([]string{"--config", cfgPath})
		if err != nil {
			t.Errorf("runDBMigrateApply error: %v", err)
		}
	})

	t.Run("migrate apply again - already applied", func(t *testing.T) {
		err := runDBMigrateApply([]string{"--config", cfgPath})
		if err != nil {
			t.Errorf("runDBMigrateApply (already applied) error: %v", err)
		}
	})
}

// TestRunCredit_WithMockServer tests credit commands with mock admin server
func TestRunCredit_WithMockServer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/credits/overview"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_distributed": "100000",
				"total_consumed":    "50000",
				"top_consumers": []map[string]any{
					{"user_id": "user-1", "email": "a@b.com", "name": "User A", "consumed": "1000"},
				},
			})
		case strings.Contains(r.URL.Path, "/credits/balance"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id":        "user-1",
				"credit_balance": "500",
			})
		case strings.Contains(r.URL.Path, "/credits/topup"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id":        "user-1",
				"credit_balance": "600",
				"amount":         100,
			})
		case strings.Contains(r.URL.Path, "/credits/deduct"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id":        "user-1",
				"credit_balance": "400",
				"amount":         100,
			})
		case strings.Contains(r.URL.Path, "/credits/transactions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transactions": []map[string]any{
					{"id": "txn-1", "type": "topup", "amount": 100, "balance_after": 500, "description": "Test", "created_at": "2024-01-01"},
				},
				"total": 1,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	t.Run("credit overview", func(t *testing.T) {
		err := runCredit([]string{"overview", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("credit overview error: %v", err)
		}
	})

	t.Run("credit overview json output", func(t *testing.T) {
		err := runCredit([]string{"overview", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("credit overview json error: %v", err)
		}
	})

	t.Run("credit balance", func(t *testing.T) {
		err := runCredit([]string{"balance", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("credit balance error: %v", err)
		}
	})

	t.Run("credit balance json", func(t *testing.T) {
		err := runCredit([]string{"balance", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--output", "json"})
		if err != nil {
			t.Errorf("credit balance json error: %v", err)
		}
	})

	t.Run("credit topup", func(t *testing.T) {
		err := runCredit([]string{"topup", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--amount", "100", "--reason", "test"})
		if err != nil {
			t.Errorf("credit topup error: %v", err)
		}
	})

	t.Run("credit topup json", func(t *testing.T) {
		err := runCredit([]string{"topup", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--amount", "100", "--output", "json"})
		if err != nil {
			t.Errorf("credit topup json error: %v", err)
		}
	})

	t.Run("credit deduct", func(t *testing.T) {
		err := runCredit([]string{"deduct", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--amount", "100", "--reason", "test"})
		if err != nil {
			t.Errorf("credit deduct error: %v", err)
		}
	})

	t.Run("credit transactions", func(t *testing.T) {
		err := runCredit([]string{"transactions", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
		if err != nil {
			t.Errorf("credit transactions error: %v", err)
		}
	})

	t.Run("credit transactions json", func(t *testing.T) {
		err := runCredit([]string{"transactions", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--output", "json"})
		if err != nil {
			t.Errorf("credit transactions json error: %v", err)
		}
	})

	t.Run("credit transactions with type filter", func(t *testing.T) {
		err := runCredit([]string{"transactions", "--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1", "--type", "topup", "--limit", "10"})
		if err != nil {
			t.Errorf("credit transactions with filter error: %v", err)
		}
	})

	t.Run("credit balance missing user", func(t *testing.T) {
		err := runCredit([]string{"balance", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err == nil {
			t.Error("expected error for missing user")
		}
	})
}

// TestRunEntity_Upstream tests upstream entity commands
func TestRunEntity_Upstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/upstreams"):
			if strings.HasSuffix(r.URL.Path, "/upstreams") {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "up-1", "name": "Test Upstream", "algorithm": "round_robin"},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "up-1", "name": "Test Upstream"})
			}
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "up-new"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "up-1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	t.Run("upstream list", func(t *testing.T) {
		err := runUpstream([]string{"list", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("upstream list error: %v", err)
		}
	})

	t.Run("upstream list json", func(t *testing.T) {
		err := runUpstream([]string{"list", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("upstream list json error: %v", err)
		}
	})

	t.Run("upstream get", func(t *testing.T) {
		err := runUpstream([]string{"get", "--admin-url", upstream.URL, "--admin-key", "test-key", "up-1"})
		if err != nil {
			t.Errorf("upstream get error: %v", err)
		}
	})

	t.Run("upstream add", func(t *testing.T) {
		err := runUpstream([]string{"add", "--admin-url", upstream.URL, "--admin-key", "test-key", "--body", `{"name":"new-up","algorithm":"round_robin"}`})
		if err != nil {
			t.Errorf("upstream add error: %v", err)
		}
	})

	t.Run("upstream update", func(t *testing.T) {
		err := runUpstream([]string{"update", "--admin-url", upstream.URL, "--admin-key", "test-key", "--id", "up-1", "--body", `{"name":"updated"}`})
		if err != nil {
			t.Errorf("upstream update error: %v", err)
		}
	})

	t.Run("upstream delete", func(t *testing.T) {
		err := runUpstream([]string{"delete", "--admin-url", upstream.URL, "--admin-key", "test-key", "up-1"})
		if err != nil {
			t.Errorf("upstream delete error: %v", err)
		}
	})
}

// TestRunEntity_Route tests route entity commands
func TestRunEntity_Route(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/routes"):
			if strings.HasSuffix(r.URL.Path, "/routes") {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "route-1", "path": "/api/*", "service": "svc-1"},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "route-1", "path": "/api/*"})
			}
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "route-new"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "route-1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	t.Run("route list", func(t *testing.T) {
		err := runRoute([]string{"list", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("route list error: %v", err)
		}
	})

	t.Run("route list empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		}))
		defer srv.Close()
		err := runRoute([]string{"list", "--admin-url", srv.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("route list empty error: %v", err)
		}
	})

	t.Run("route get json", func(t *testing.T) {
		err := runRoute([]string{"get", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json", "route-1"})
		if err != nil {
			t.Errorf("route get json error: %v", err)
		}
	})

	t.Run("route add", func(t *testing.T) {
		err := runRoute([]string{"add", "--admin-url", upstream.URL, "--admin-key", "test-key", "--body", `{"name":"new-route","path":"/api/*"}`})
		if err != nil {
			t.Errorf("route add error: %v", err)
		}
	})

	t.Run("route delete", func(t *testing.T) {
		err := runRoute([]string{"delete", "--admin-url", upstream.URL, "--admin-key", "test-key", "route-1"})
		if err != nil {
			t.Errorf("route delete error: %v", err)
		}
	})
}

// TestRunEntityCommand_ErrorPaths tests error paths for entity commands
func TestRunEntityCommand_ErrorPaths(t *testing.T) {
	t.Run("missing subcommand", func(t *testing.T) {
		err := runEntityCommand("service", "/admin/api/v1/services", []string{})
		if err == nil {
			t.Error("expected error for missing subcommand")
		}
		if !strings.Contains(err.Error(), "missing service subcommand") {
			t.Errorf("error should mention missing subcommand, got: %v", err)
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		err := runEntityCommand("service", "/admin/api/v1/services", []string{"bogus"})
		if err == nil {
			t.Error("expected error for unknown subcommand")
		}
		if !strings.Contains(err.Error(), `unknown service subcommand "bogus"`) {
			t.Errorf("error should mention unknown subcommand, got: %v", err)
		}
	})

	t.Run("list missing admin connection", func(t *testing.T) {
		err := runEntityList("service", "/admin/api/v1/services", []string{})
		if err == nil {
			t.Error("expected error for missing admin connection")
		}
	})

	t.Run("get missing id", func(t *testing.T) {
		err := runEntityGet("service", "/admin/api/v1/services", []string{
			"--admin-url", "http://localhost:9876", "--admin-key", "test-key",
		})
		if err == nil {
			t.Error("expected error for missing id")
		}
	})

	t.Run("add missing body", func(t *testing.T) {
		err := runEntityAdd("service", "/admin/api/v1/services", []string{
			"--admin-url", "http://localhost:9876", "--admin-key", "test-key",
		})
		if err == nil {
			t.Error("expected error for missing body")
		}
	})

	t.Run("update missing id", func(t *testing.T) {
		err := runEntityUpdate("service", "/admin/api/v1/services", []string{
			"--admin-url", "http://localhost:9876", "--admin-key", "test-key",
			"--body", `{"name":"test"}`,
		})
		if err == nil {
			t.Error("expected error for missing id")
		}
	})

	t.Run("delete missing id", func(t *testing.T) {
		err := runEntityDelete("service", "/admin/api/v1/services", []string{
			"--admin-url", "http://localhost:9876", "--admin-key", "test-key",
		})
		if err == nil {
			t.Error("expected error for missing id")
		}
	})
}

// TestRunAnalytics_Subcommands tests analytics subcommands with mock server
func TestRunAnalytics_Subcommands(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/analytics/overview"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_requests": 1000,
				"avg_latency_ms": 45,
				"error_rate":     "2%",
				"top_routes":     []map[string]any{},
			})
		case strings.Contains(r.URL.Path, "/analytics/timeseries"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"timestamp": "2024-01-01T00:00:00Z", "requests": 100, "errors": 2, "avg_latency_ms": 10},
				},
			})
		case strings.Contains(r.URL.Path, "/analytics/latency"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"p50_ms": 10, "p95_ms": 50, "p99_ms": 100, "avg_ms": 25,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	t.Run("analytics overview", func(t *testing.T) {
		err := runAnalytics([]string{"overview", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("analytics overview error: %v", err)
		}
	})

	t.Run("analytics overview json", func(t *testing.T) {
		err := runAnalytics([]string{"overview", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("analytics overview json error: %v", err)
		}
	})

	t.Run("analytics requests", func(t *testing.T) {
		err := runAnalytics([]string{"requests", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("analytics requests error: %v", err)
		}
	})

	t.Run("analytics requests json", func(t *testing.T) {
		err := runAnalytics([]string{"requests", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("analytics requests json error: %v", err)
		}
	})

	t.Run("analytics requests with window", func(t *testing.T) {
		err := runAnalytics([]string{"requests", "--admin-url", upstream.URL, "--admin-key", "test-key", "--window", "1h", "--granularity", "5m"})
		if err != nil {
			t.Errorf("analytics requests with window error: %v", err)
		}
	})

	t.Run("analytics requests missing admin connection", func(t *testing.T) {
		err := runAnalytics([]string{"requests"})
		if err == nil {
			t.Error("expected error for missing admin connection")
		}
	})
}

// TestRunAudit_Subcommands with mock server
func TestRunAudit_Subcommands(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/audit-logs"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"audit_logs": []map[string]any{
					{"id": "log-1", "method": "GET", "path": "/api/v1", "status_code": 200, "latency_ms": 10, "user_id": "user-1", "route_id": "route-1"},
				},
				"total": 1,
			})
		case strings.Contains(r.URL.Path, "/audit/stats"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_requests": 1000, "error_requests": 50, "error_rate": "5%", "avg_latency_ms": 45.5,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	t.Run("audit search", func(t *testing.T) {
		err := runAudit([]string{"search", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("audit search error: %v", err)
		}
	})

	t.Run("audit search json", func(t *testing.T) {
		err := runAudit([]string{"search", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("audit search json error: %v", err)
		}
	})

	t.Run("audit detail", func(t *testing.T) {
		err := runAudit([]string{"detail", "--admin-url", upstream.URL, "--admin-key", "test-key", "log-1"})
		if err != nil {
			t.Errorf("audit detail error: %v", err)
		}
	})

	t.Run("audit stats", func(t *testing.T) {
		err := runAudit([]string{"stats", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("audit stats error: %v", err)
		}
	})

	t.Run("audit stats json", func(t *testing.T) {
		err := runAudit([]string{"stats", "--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
		if err != nil {
			t.Errorf("audit stats json error: %v", err)
		}
	})

	t.Run("audit search with filters", func(t *testing.T) {
		err := runAudit([]string{
			"search", "--admin-url", upstream.URL, "--admin-key", "test-key",
			"--user-id", "user-1", "--route", "route-1",
			"--status-min", "200", "--method", "GET", "--limit", "10",
		})
		if err != nil {
			t.Errorf("audit search with filters error: %v", err)
		}
	})

	t.Run("audit detail missing id", func(t *testing.T) {
		err := runAudit([]string{"detail", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err == nil {
			t.Error("expected error for missing log id")
		}
	})
}

// TestRunCredit_ErrorPaths tests credit command error paths
func TestRunCredit_ErrorPaths(t *testing.T) {
	t.Run("missing subcommand", func(t *testing.T) {
		err := runCredit([]string{})
		if err == nil {
			t.Error("expected error for missing credit subcommand")
		}
		if !strings.Contains(err.Error(), "missing credit subcommand") {
			t.Errorf("error should mention missing credit subcommand, got: %v", err)
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		err := runCredit([]string{"unknown"})
		if err == nil {
			t.Error("expected error for unknown credit subcommand")
		}
		if !strings.Contains(err.Error(), `unknown credit subcommand "unknown"`) {
			t.Errorf("error should mention unknown subcommand, got: %v", err)
		}
	})

	t.Run("topup missing user", func(t *testing.T) {
		err := runCredit([]string{"topup", "--admin-url", "http://localhost:9876", "--admin-key", "test-key", "--amount", "100"})
		if err == nil {
			t.Error("expected error for missing user")
		}
	})

	t.Run("topup missing amount", func(t *testing.T) {
		err := runCredit([]string{"topup", "--admin-url", "http://localhost:9876", "--admin-key", "test-key", "--user", "user-1"})
		if err == nil {
			t.Error("expected error for missing amount")
		}
	})

	t.Run("deduct missing user", func(t *testing.T) {
		err := runCredit([]string{"deduct", "--admin-url", "http://localhost:9876", "--admin-key", "test-key", "--amount", "100"})
		if err == nil {
			t.Error("expected error for missing user")
		}
	})

	t.Run("balance missing user", func(t *testing.T) {
		err := runCredit([]string{"balance", "--admin-url", "http://localhost:9876", "--admin-key", "test-key"})
		if err == nil {
			t.Error("expected error for missing user")
		}
	})

	t.Run("transactions missing user", func(t *testing.T) {
		err := runCredit([]string{"transactions", "--admin-url", "http://localhost:9876", "--admin-key", "test-key"})
		if err == nil {
			t.Error("expected error for missing user")
		}
	})
}
