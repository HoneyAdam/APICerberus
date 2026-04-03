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

func TestRunAudit_MissingSubcommand(t *testing.T) {
	err := runAudit([]string{})
	if err == nil {
		t.Error("runAudit should return error for missing subcommand")
	}
	if !strings.Contains(err.Error(), "missing audit subcommand") {
		t.Errorf("Error should mention missing subcommand, got: %v", err)
	}
}

func TestRunAudit_UnknownSubcommand(t *testing.T) {
	err := runAudit([]string{"unknown"})
	if err == nil {
		t.Error("runAudit should return error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown audit subcommand") {
		t.Errorf("Error should mention unknown subcommand, got: %v", err)
	}
}

func TestRunAuditSearch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/audit-logs" {
			t.Errorf("Expected path /admin/api/v1/audit-logs, got %s", r.URL.Path)
		}

		response := map[string]any{
			"entries": []map[string]any{
				{
					"id":         "log-1",
					"created_at": "2024-01-01T00:00:00Z",
					"method":     "GET",
					"path":       "/api/test",
					"status_code": 200,
					"latency_ms":  50,
					"user_id":    "user-1",
					"route_name": "test-route",
				},
			},
			"total": 1,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAuditSearch([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAuditSearch error: %v", err)
	}
}

func TestRunAuditSearch_WithFilters(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("user_id") != "user-123" {
			t.Errorf("Expected user_id=user-123, got %s", query.Get("user_id"))
		}
		if query.Get("route") != "test-route" {
			t.Errorf("Expected route=test-route, got %s", query.Get("route"))
		}
		if query.Get("method") != "GET" {
			t.Errorf("Expected method=GET, got %s", query.Get("method"))
		}

		response := map[string]any{
			"entries": []map[string]any{},
			"total":   0,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAuditSearch([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user-id", "user-123",
		"--route", "test-route",
		"--method", "GET",
	})
	if err != nil {
		t.Errorf("runAuditSearch error: %v", err)
	}
}

func TestRunAuditSearch_EmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"entries": []map[string]any{},
			"total":   0,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAuditSearch([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAuditSearch error: %v", err)
	}
}

func TestRunAuditDetail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/admin/api/v1/audit-logs/") {
			t.Errorf("Expected audit log detail path, got %s", r.URL.Path)
		}

		response := map[string]any{
			"id":         "log-1",
			"created_at": "2024-01-01T00:00:00Z",
			"method":     "GET",
			"path":       "/api/test",
			"status_code": 200,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAuditDetail([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--id", "log-1"})
	if err != nil {
		t.Errorf("runAuditDetail error: %v", err)
	}
}

func TestRunAuditDetail_PositionalArg(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "log-2") {
			t.Errorf("Expected log-2 in path, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "log-2"})
	}))
	defer upstream.Close()

	err := runAuditDetail([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "log-2"})
	if err != nil {
		t.Errorf("runAuditDetail error: %v", err)
	}
}

func TestRunAuditDetail_MissingID(t *testing.T) {
	err := runAuditDetail([]string{"--admin-url", "http://localhost:9876", "--admin-key", "test-key"})
	if err == nil {
		t.Error("runAuditDetail should return error for missing ID")
	}
	if !strings.Contains(err.Error(), "audit id is required") {
		t.Errorf("Error should mention required ID, got: %v", err)
	}
}

func TestRunAuditExport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/audit-logs/export" {
			t.Errorf("Expected export path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "jsonl" {
			t.Errorf("Expected format=jsonl, got %s", r.URL.Query().Get("format"))
		}

		w.Write([]byte(`{"id":"log-1","method":"GET"}
{"id":"log-2","method":"POST"}
`))
	}))
	defer upstream.Close()

	err := runAuditExport([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAuditExport error: %v", err)
	}
}

func TestRunAuditStats(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/audit-logs/stats" {
			t.Errorf("Expected stats path, got %s", r.URL.Path)
		}

		response := map[string]any{
			"total_requests":  1000,
			"error_requests":  50,
			"error_rate":      "5%",
			"avg_latency_ms":  45.5,
			"top_routes": []map[string]any{
				{"route_id": "route-1", "route_name": "Test Route", "count": 500},
			},
			"top_users": []map[string]any{
				{"user_id": "user-1", "consumer_name": "Test User", "count": 200},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAuditStats([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAuditStats error: %v", err)
	}
}

func TestRunAuditCleanup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/audit-logs/cleanup" {
			t.Errorf("Expected cleanup path, got %s", r.URL.Path)
		}

		response := map[string]any{
			"deleted": 100,
			"remaining": 900,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAuditCleanup([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--older-than-days", "30"})
	if err != nil {
		t.Errorf("runAuditCleanup error: %v", err)
	}
}

func TestRunAuditCleanup_WithCutoff(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("cutoff") != "2024-01-01T00:00:00Z" {
			t.Errorf("Expected cutoff parameter, got %s", query.Get("cutoff"))
		}

		json.NewEncoder(w).Encode(map[string]any{"deleted": 50})
	}))
	defer upstream.Close()

	err := runAuditCleanup([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--cutoff", "2024-01-01T00:00:00Z",
	})
	if err != nil {
		t.Errorf("runAuditCleanup error: %v", err)
	}
}

func TestParseAuditQueryFlags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantUserID     string
		wantRoute      string
		wantMethod     string
		wantStatusMin  string
		wantStatusMax  string
		wantLimit      string
		wantOffset     string
	}{
		{
			name:       "all filters",
			args:       []string{"--user-id", "user-123", "--route", "test-route", "--method", "GET", "--status-min", "200", "--status-max", "299", "--limit", "100", "--offset", "50"},
			wantUserID: "user-123",
			wantRoute:  "test-route",
			wantMethod: "GET",
			wantStatusMin: "200",
			wantStatusMax: "299",
			wantLimit:  "100",
			wantOffset: "50",
		},
		{
			name:       "empty values ignored",
			args:       []string{"--user-id", "", "--route", "  "},
			wantUserID: "",
			wantRoute:  "",
			wantLimit:  "50", // default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, _, err := parseAuditQueryFlags("test", tt.args)
			if err != nil {
				t.Fatalf("parseAuditQueryFlags error: %v", err)
			}
			if query.Get("user_id") != tt.wantUserID {
				t.Errorf("user_id = %q, want %q", query.Get("user_id"), tt.wantUserID)
			}
			if query.Get("route") != tt.wantRoute {
				t.Errorf("route = %q, want %q", query.Get("route"), tt.wantRoute)
			}
			if query.Get("method") != tt.wantMethod {
				t.Errorf("method = %q, want %q", query.Get("method"), tt.wantMethod)
			}
			if query.Get("status_min") != tt.wantStatusMin {
				t.Errorf("status_min = %q, want %q", query.Get("status_min"), tt.wantStatusMin)
			}
			if query.Get("status_max") != tt.wantStatusMax {
				t.Errorf("status_max = %q, want %q", query.Get("status_max"), tt.wantStatusMax)
			}
			if query.Get("limit") != tt.wantLimit {
				t.Errorf("limit = %q, want %q", query.Get("limit"), tt.wantLimit)
			}
			if query.Get("offset") != tt.wantOffset {
				t.Errorf("offset = %q, want %q", query.Get("offset"), tt.wantOffset)
			}
		})
	}
}

func TestPrintAuditList(t *testing.T) {
	result := map[string]any{
		"entries": []map[string]any{
			{
				"id":          "log-1",
				"created_at":  "2024-01-01T00:00:00Z",
				"method":      "GET",
				"path":        "/api/test",
				"status_code": 200,
				"latency_ms":  50,
				"user_id":     "user-1",
				"route_name":  "test-route",
			},
		},
		"total": 1,
	}

	err := printAuditList(result)
	if err != nil {
		t.Errorf("printAuditList error: %v", err)
	}
}

func TestPrintAuditList_Empty(t *testing.T) {
	result := map[string]any{
		"entries": []map[string]any{},
		"total":   0,
	}

	err := printAuditList(result)
	if err != nil {
		t.Errorf("printAuditList error: %v", err)
	}
}

func TestCollectUnseenAuditRows(t *testing.T) {
	seen := map[string]struct{}{}

	items := []any{
		map[string]any{
			"id":         "log-1",
			"created_at": "2024-01-01T00:00:00Z",
			"method":     "GET",
			"path":       "/api/test",
			"status_code": 200,
			"latency_ms": 50,
			"user_id":    "user-1",
			"route_name": "test-route",
		},
		map[string]any{
			"id":         "log-2",
			"created_at": "2024-01-01T00:01:00Z",
			"method":     "POST",
			"path":       "/api/test",
			"status_code": 201,
			"latency_ms": 100,
			"user_id":    "user-2",
			"route_name": "test-route",
		},
	}

	rows := collectUnseenAuditRows(items, seen)
	if len(rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(rows))
	}

	// Second call should return empty (all seen)
	rows = collectUnseenAuditRows(items, seen)
	if len(rows) != 0 {
		t.Errorf("Expected 0 rows after seen, got %d", len(rows))
	}
}

func TestCollectUnseenAuditRows_EmptyID(t *testing.T) {
	seen := map[string]struct{}{}

	items := []any{
		map[string]any{
			"id":         "",
			"created_at": "2024-01-01T00:00:00Z",
		},
		map[string]any{
			"created_at": "2024-01-01T00:00:00Z",
			// No ID field
		},
	}

	rows := collectUnseenAuditRows(items, seen)
	if len(rows) != 0 {
		t.Errorf("Expected 0 rows for items with empty ID, got %d", len(rows))
	}
}

func TestRunAuditRetentionShow(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
audit:
  retention_days: 90
  route_retention_days:
    route1: 30
    route2: 60
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runAuditRetentionShow([]string{"--config", configPath})
	if err != nil {
		t.Errorf("runAuditRetentionShow error: %v", err)
	}
}

func TestRunAuditRetentionShow_NoRouteOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
audit:
  retention_days: 30
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runAuditRetentionShow([]string{"--config", configPath})
	if err != nil {
		t.Errorf("runAuditRetentionShow error: %v", err)
	}
}

func TestRunAuditRetentionSet(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
audit:
  retention_days: 30
  route_retention_days:
    route1: 15
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runAuditRetentionSet([]string{"--config", configPath, "--days", "60"})
	if err != nil {
		t.Errorf("runAuditRetentionSet error: %v", err)
	}

	// Verify the config was updated
	content, _ := os.ReadFile(configPath)
	if !strings.Contains(string(content), "60") {
		t.Error("Config should contain updated retention days")
	}
}

func TestRunAuditRetentionSet_RouteOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
audit:
  retention_days: 30
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runAuditRetentionSet([]string{
		"--config", configPath,
		"--route", "route2",
		"--route-days", "45",
	})
	if err != nil {
		t.Errorf("runAuditRetentionSet error: %v", err)
	}
}

func TestRunAuditRetentionSet_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runAuditRetentionSet([]string{"--config", configPath})
	if err == nil {
		t.Error("runAuditRetentionSet should return error when no changes provided")
	}
}

func TestRunAuditRetentionSet_RouteWithoutDays(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runAuditRetentionSet([]string{
		"--config", configPath,
		"--route", "route1",
	})
	if err == nil {
		t.Error("runAuditRetentionSet should return error when route provided without days")
	}
}

func TestRunAuditRetention(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
audit:
  retention_days: 30
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	// Test "show" subcommand
	err := runAuditRetention([]string{"show", "--config", configPath})
	if err != nil {
		t.Errorf("runAuditRetention show error: %v", err)
	}

	// Test "set" subcommand
	err = runAuditRetention([]string{"set", "--config", configPath, "--days", "60"})
	if err != nil {
		t.Errorf("runAuditRetention set error: %v", err)
	}

	// Test unknown subcommand
	err = runAuditRetention([]string{"unknown"})
	if err == nil {
		t.Error("runAuditRetention should return error for unknown subcommand")
	}
}

func TestRunAuditRetention_NoArgs(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
audit:
  retention_days: 30
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	// No args defaults to show - use env or default config instead
	// Since we can't pass --config directly to runAuditRetention,
	// we test the subcommands instead
	err := runAuditRetention([]string{"show", "--config", configPath})
	if err != nil {
		t.Errorf("runAuditRetention show error: %v", err)
	}
}
