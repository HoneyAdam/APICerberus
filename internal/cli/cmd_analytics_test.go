package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunAnalytics_MissingSubcommand(t *testing.T) {
	err := runAnalytics([]string{})
	if err == nil {
		t.Error("runAnalytics should return error for missing subcommand")
	}
	if !strings.Contains(err.Error(), "missing analytics subcommand") {
		t.Errorf("Error should mention missing subcommand, got: %v", err)
	}
}

func TestRunAnalytics_UnknownSubcommand(t *testing.T) {
	err := runAnalytics([]string{"unknown"})
	if err == nil {
		t.Error("runAnalytics should return error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown analytics subcommand") {
		t.Errorf("Error should mention unknown subcommand, got: %v", err)
	}
}

func TestRunAnalytics_Overview(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/analytics/overview" {
			t.Errorf("Expected path /admin/api/v1/analytics/overview, got %s", r.URL.Path)
		}
		response := map[string]any{
			"total_requests": 1000,
			"total_errors":   10,
			"avg_latency_ms": 45.5,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAnalyticsOverview([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAnalyticsOverview error: %v", err)
	}
}

func TestRunAnalytics_Overview_JSONOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{"total_requests": 500}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAnalyticsOverview([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
	if err != nil {
		t.Errorf("runAnalyticsOverview error: %v", err)
	}
}

func TestRunAnalytics_Requests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/api/v1/analytics/timeseries" {
			t.Errorf("Expected path /admin/api/v1/analytics/timeseries, got %s", r.URL.Path)
		}
		response := map[string]any{
			"items": []map[string]any{
				{
					"timestamp":      "2024-01-01T00:00:00Z",
					"requests":       100,
					"errors":         5,
					"avg_latency_ms": 50,
					"p95_latency_ms": 100,
					"credits_consumed": 500,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAnalyticsRequests([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAnalyticsRequests error: %v", err)
	}
}

func TestRunAnalytics_Requests_EmptyItems(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"items": []map[string]any{},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAnalyticsRequests([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAnalyticsRequests error: %v", err)
	}
}

func TestRunAnalytics_Requests_WithFlags(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("from") != "2024-01-01T00:00:00Z" {
			t.Errorf("Expected from parameter, got %s", query.Get("from"))
		}
		if query.Get("to") != "2024-01-02T00:00:00Z" {
			t.Errorf("Expected to parameter, got %s", query.Get("to"))
		}
		if query.Get("window") != "1h" {
			t.Errorf("Expected window parameter, got %s", query.Get("window"))
		}
		if query.Get("granularity") != "5m" {
			t.Errorf("Expected granularity parameter, got %s", query.Get("granularity"))
		}
		response := map[string]any{"items": []map[string]any{}}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAnalyticsRequests([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--from", "2024-01-01T00:00:00Z",
		"--to", "2024-01-02T00:00:00Z",
		"--window", "1h",
		"--granularity", "5m",
	})
	if err != nil {
		t.Errorf("runAnalyticsRequests error: %v", err)
	}
}

func TestRunAnalytics_Latency(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/api/v1/analytics/latency" {
			t.Errorf("Expected path /admin/api/v1/analytics/latency, got %s", r.URL.Path)
		}
		response := map[string]any{
			"p50": 25,
			"p95": 100,
			"p99": 200,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runAnalyticsLatency([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runAnalyticsLatency error: %v", err)
	}
}

func TestParseAnalyticsCommonFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantFrom string
		wantTo   string
		wantWindow string
	}{
		{
			name: "all flags",
			args: []string{"--from", "2024-01-01T00:00:00Z", "--to", "2024-01-02T00:00:00Z", "--window", "1h"},
			wantFrom: "2024-01-01T00:00:00Z",
			wantTo:   "2024-01-02T00:00:00Z",
			wantWindow: "1h",
		},
		{
			name: "only from",
			args: []string{"--from", "2024-01-01T00:00:00Z"},
			wantFrom: "2024-01-01T00:00:00Z",
		},
		{
			name: "empty values ignored",
			args: []string{"--from", "", "--to", "  "},
			wantFrom: "",
			wantTo:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, _, err := parseAnalyticsCommonFlags("test", tt.args)
			if err != nil {
				t.Fatalf("parseAnalyticsCommonFlags error: %v", err)
			}
			if query.Get("from") != tt.wantFrom {
				t.Errorf("from = %q, want %q", query.Get("from"), tt.wantFrom)
			}
			if query.Get("to") != tt.wantTo {
				t.Errorf("to = %q, want %q", query.Get("to"), tt.wantTo)
			}
			if query.Get("window") != tt.wantWindow {
				t.Errorf("window = %q, want %q", query.Get("window"), tt.wantWindow)
			}
		})
	}
}

func TestParseAnalyticsCommonFlags_InvalidFlag(t *testing.T) {
	_, _, err := parseAnalyticsCommonFlags("test", []string{"--invalid-flag"})
	if err == nil {
		t.Error("parseAnalyticsCommonFlags should return error for invalid flag")
	}
}
