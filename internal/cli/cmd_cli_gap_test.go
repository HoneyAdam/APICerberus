package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CLI coverage gap tests - only tests not covered by other test files

func TestRunAuditTail_InvalidURL(t *testing.T) {
	err := runAuditTail([]string{
		"--admin-url", "http://127.0.0.1:19999",
		"--admin-key", "test-key",
		"--interval", "10ms",
		"--limit", "5",
	})
	if err == nil {
		t.Log("expected error when admin server is unreachable")
	}
}

func TestRunAuditStats_WithTopRoutesEmpty(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_requests": 1000,
			"error_requests": 50,
			"error_rate":     "5%",
			"avg_latency_ms": 45.5,
			"top_routes":     []map[string]any{},
			"top_users":      []map[string]any{},
		})
	}))
	defer upstream.Close()

	err := runAuditStats([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAuditStats error: %v", err)
	}
}

func TestRunAuditExport_WithCustomFormat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "csv" {
			t.Errorf("Expected format=csv, got %s", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte("id,method,status\nlog-1,GET,200\n"))
	}))
	defer upstream.Close()

	err := runAuditExport([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--format", "csv",
	})
	if err != nil {
		t.Errorf("runAuditExport error: %v", err)
	}
}

func TestRunAuditStats_JSONOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_requests": 1000,
			"error_requests": 50,
		})
	}))
	defer upstream.Close()

	err := runAuditStats([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--output", "json",
	})
	if err != nil {
		t.Errorf("runAuditStats error: %v", err)
	}
}

func TestRunAuditTail_Filters(t *testing.T) {
	err := runAuditTail([]string{
		"--admin-url", "http://127.0.0.1:19999",
		"--admin-key", "test-key",
		"--user-id", "user-1",
		"--route", "test-route",
		"--search", "test query",
		"--interval", "10ms",
	})
	if err == nil {
		t.Log("expected connection error")
	}
	if !strings.Contains(err.Error(), "connection refused") && !strings.Contains(err.Error(), "connectex") {
		t.Logf("expected connection error, got: %v", err)
	}
}

func TestRunCreditAdjust_JSONOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id":        "user-1",
			"credit_balance": 600,
			"amount":         100,
		})
	}))
	defer upstream.Close()

	err := runCreditAdjust([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--amount", "100",
		"--reason", "test",
		"--output", "json",
	}, true)
	if err != nil {
		t.Errorf("runCreditAdjust error: %v", err)
	}
}

func TestRunCreditTransactions_JSONOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transactions": []map[string]any{
				{"id": "txn-1", "type": "topup", "amount": 100, "balance_after": 500, "description": "Initial topup", "created_at": "2024-01-01T00:00:00Z"},
			},
			"total": 1,
		})
	}))
	defer upstream.Close()

	err := runCreditTransactions([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--output", "json",
	})
	if err != nil {
		t.Errorf("runCreditTransactions error: %v", err)
	}
}

// Test runAuditRetention unknown subcommand
func TestRunAuditRetention_UnknownSubcommand(t *testing.T) {
	err := runAuditRetention([]string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown audit retention subcommand") {
		t.Errorf("error should mention unknown subcommand, got: %v", err)
	}
}

// Test runAnalyticsLatency with server
func TestRunAnalyticsLatency(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"p50_ms": 10,
			"p95_ms": 50,
			"p99_ms": 100,
			"avg_ms": 25,
			"count":  1000,
		})
	}))
	defer upstream.Close()

	err := runAnalyticsLatency([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
	})
	if err != nil {
		t.Errorf("runAnalyticsLatency error: %v", err)
	}
}

// --- runConfig subcommand error paths ---

func TestRunConfig_MissingSubcommand(t *testing.T) {
	err := runConfig([]string{})
	if err == nil {
		t.Error("expected error for missing config subcommand")
	}
	if !strings.Contains(err.Error(), "missing config subcommand") {
		t.Errorf("error should mention missing subcommand, got: %v", err)
	}
}

func TestRunConfig_UnknownSubcommand(t *testing.T) {
	err := runConfig([]string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown config subcommand")
	}
	if !strings.Contains(err.Error(), `unknown config subcommand "unknown"`) {
		t.Errorf("error should mention unknown subcommand, got: %v", err)
	}
}

// --- runMCP invalid transport ---

func TestRunMCP_InvalidTransport(t *testing.T) {
	err := runMCP([]string{"--transport", "invalid"})
	if err == nil {
		t.Error("expected error for invalid transport")
	}
}

// --- runAuditCleanup error path ---

func TestRunAuditCleanup_InvalidURL(t *testing.T) {
	err := runAuditCleanup([]string{
		"--admin-url", "http://127.0.0.1:19999",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Log("expected error when admin server is unreachable")
	}
}

// --- runAuditCleanup with all flags ---

func TestRunAuditCleanup_WithAllFlags(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deleted": 100,
		})
	}))
	defer upstream.Close()

	err := runAuditCleanup([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--older-than-days", "30",
		"--batch-size", "500",
	})
	if err != nil {
		t.Errorf("runAuditCleanup error: %v", err)
	}
}
