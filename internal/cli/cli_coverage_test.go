package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Additional Tests for CLI Low Coverage Functions
// =============================================================================

// TestRunStart_Advanced tests runStart function
func TestRunStart_Advanced(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode - starts real servers")
	}

	t.Run("start with missing config", func(t *testing.T) {
		err := runStart([]string{"--config", "/nonexistent/config.yaml"})
		if err == nil {
			t.Error("expected error for missing config file")
		}
	})

	t.Run("start with invalid config", func(t *testing.T) {
		// Create temp file with invalid content
		tmpFile, _ := os.CreateTemp("", "invalid-config-*.yaml")
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("invalid: yaml: content: [[[")
		tmpFile.Close()

		err := runStart([]string{"--config", tmpFile.Name()})
		if err == nil {
			t.Error("expected error for invalid config")
		}
	})
}

// TestRunAudit_More tests runAudit function
func TestRunAudit_More(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The audit API uses /admin/api/v1/audit-logs
		if strings.Contains(r.URL.Path, "/audit-logs") {
			json.NewEncoder(w).Encode(map[string]any{
				"logs": []map[string]any{
					{"id": "log-1", "message": "test"},
				},
				"total": 1,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	t.Run("audit search with query", func(t *testing.T) {
		// Use correct flag name: -search (not -query)
		err := runAudit([]string{"search", "-search", "test", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runAudit search error: %v", err)
		}
	})

	t.Run("audit search with user filter", func(t *testing.T) {
		// Use correct flag name: -user-id (not -user)
		err := runAudit([]string{"search", "-user-id", "user-1", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runAudit search with user error: %v", err)
		}
	})

	t.Run("audit search with time range", func(t *testing.T) {
		// Use RFC3339 timestamps for from/to
		now := time.Now().UTC().Format(time.RFC3339)
		oneHourAgo := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		err := runAudit([]string{"search", "-from", oneHourAgo, "-to", now, "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runAudit search with time range error: %v", err)
		}
	})
}

// TestRunCredit_More tests runCredit function
func TestRunCredit_More(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/credits") {
			json.NewEncoder(w).Encode(map[string]any{
				"user_id":  "user-1",
				"balance":  100,
				"currency": "credits",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	t.Run("credit overview", func(t *testing.T) {
		// Overview doesn't take user argument - it shows global overview
		err := runCredit([]string{"overview", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runCredit overview error: %v", err)
		}
	})

	t.Run("credit balance", func(t *testing.T) {
		// Balance uses --user flag, not positional arg
		err := runCredit([]string{"balance", "--user", "user-1", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runCredit balance error: %v", err)
		}
	})
}

// TestRunAnalytics_More tests runAnalytics function
func TestRunAnalytics_More(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/analytics/overview") {
			json.NewEncoder(w).Encode(map[string]any{
				"total_requests": 1000,
				"avg_latency_ms": 50.5,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/analytics/latency") {
			json.NewEncoder(w).Encode(map[string]any{
				"percentiles": map[string]any{
					"p50": 10.5,
					"p99": 100.2,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	t.Run("analytics overview", func(t *testing.T) {
		err := runAnalytics([]string{"overview", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runAnalytics overview error: %v", err)
		}
	})

	t.Run("analytics latency", func(t *testing.T) {
		err := runAnalytics([]string{"latency", "--admin-url", upstream.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runAnalytics latency error: %v", err)
		}
	})

	t.Run("analytics requests", func(t *testing.T) {
		upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/analytics/requests") || strings.Contains(r.URL.Path, "/analytics/timeseries") {
				json.NewEncoder(w).Encode(map[string]any{
					"series": []map[string]any{
						{"timestamp": "2024-01-01T00:00:00Z", "requests": 500},
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer upstream2.Close()

		err := runAnalytics([]string{"requests", "--admin-url", upstream2.URL, "--admin-key", "test-key"})
		if err != nil {
			t.Errorf("runAnalytics requests error: %v", err)
		}
	})
}

// TestResolveAdminConnection_More tests resolveAdminConnection function
func TestResolveAdminConnection_More(t *testing.T) {
	t.Run("from environment variables", func(t *testing.T) {
		t.Setenv("APICERBERUS_ADMIN_URL", "http://localhost:9876")
		t.Setenv("APICERBERUS_ADMIN_KEY", "env-key")

		url, key, err := resolveAdminConnection("", "", "")
		if err != nil {
			t.Errorf("resolveAdminConnection error: %v", err)
		}
		if url != "http://localhost:9876" {
			t.Errorf("url = %q, want http://localhost:9876", url)
		}
		if key != "env-key" {
			t.Errorf("key = %q, want env-key", key)
		}
	})

	t.Run("flags override environment", func(t *testing.T) {
		t.Setenv("APICERBERUS_ADMIN_URL", "http://env-host:9876")
		t.Setenv("APICERBERUS_ADMIN_KEY", "env-key")

		url, key, err := resolveAdminConnection("", "http://flag-host:9876", "flag-key")
		if err != nil {
			t.Errorf("resolveAdminConnection error: %v", err)
		}
		if url != "http://flag-host:9876" {
			t.Errorf("url = %q, want http://flag-host:9876", url)
		}
		if key != "flag-key" {
			t.Errorf("key = %q, want flag-key", key)
		}
	})

	t.Run("normalize URL without scheme", func(t *testing.T) {
		url, key, err := resolveAdminConnection("", "localhost:9876", "test-key")
		if err != nil {
			t.Errorf("resolveAdminConnection error: %v", err)
		}
		if url != "http://localhost:9876" {
			t.Errorf("url = %q, want http://localhost:9876", url)
		}
		if key != "test-key" {
			t.Errorf("key = %q, want test-key", key)
		}
	})

	t.Run("normalize URL with trailing slash", func(t *testing.T) {
		url, key, err := resolveAdminConnection("", "http://localhost:9876/", "test-key")
		if err != nil {
			t.Errorf("resolveAdminConnection error: %v", err)
		}
		if url != "http://localhost:9876" {
			t.Errorf("url = %q, want http://localhost:9876", url)
		}
		if key != "test-key" {
			t.Errorf("key = %q, want test-key", key)
		}
	})
}
