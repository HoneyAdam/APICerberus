package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunCredit_MissingSubcommand(t *testing.T) {
	err := runCredit([]string{})
	if err == nil {
		t.Error("runCredit should return error for missing subcommand")
	}
	if !strings.Contains(err.Error(), "missing credit subcommand") {
		t.Errorf("Error should mention missing subcommand, got: %v", err)
	}
}

func TestRunCredit_UnknownSubcommand(t *testing.T) {
	err := runCredit([]string{"unknown"})
	if err == nil {
		t.Error("runCredit should return error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown credit subcommand") {
		t.Errorf("Error should mention unknown subcommand, got: %v", err)
	}
}

func TestRunCreditOverview(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/credits/overview" {
			t.Errorf("Expected path /admin/api/v1/credits/overview, got %s", r.URL.Path)
		}

		response := map[string]any{
			"total_distributed": 10000,
			"total_consumed":    7500,
			"top_consumers": []map[string]any{
				{
					"user_id":  "user-1",
					"email":    "user1@example.com",
					"name":     "User One",
					"consumed": 5000,
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditOverview([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runCreditOverview error: %v", err)
	}
}

func TestRunCreditOverview_JSONOutput(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"total_distributed": 10000,
			"total_consumed":    7500,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditOverview([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--output", "json"})
	if err != nil {
		t.Errorf("runCreditOverview error: %v", err)
	}
}

func TestRunCreditOverview_EmptyConsumers(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"total_distributed": 0,
			"total_consumed":    0,
			"top_consumers":     []map[string]any{},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditOverview([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runCreditOverview error: %v", err)
	}
}

func TestRunCreditBalance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/user-1/credits/balance") {
			t.Errorf("Expected balance path, got %s", r.URL.Path)
		}

		response := map[string]any{
			"user_id":        "user-1",
			"credit_balance": 500,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditBalance([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
	if err != nil {
		t.Errorf("runCreditBalance error: %v", err)
	}
}

func TestRunCreditBalance_MissingUser(t *testing.T) {
	err := runCreditBalance([]string{"--admin-url", "http://localhost:9876", "--admin-key", "test-key"})
	if err == nil {
		t.Error("runCreditBalance should return error for missing user")
	}
}

func TestRunCreditTopup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/user-1/credits/topup") {
			t.Errorf("Expected topup path, got %s", r.URL.Path)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["amount"] != float64(100) {
			t.Errorf("Expected amount=100, got %v", payload["amount"])
		}
		if payload["reason"] != "Bonus credits" {
			t.Errorf("Expected reason='Bonus credits', got %v", payload["reason"])
		}

		response := map[string]any{
			"user_id":        "user-1",
			"credit_balance": 600,
			"amount":         100,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditAdjust([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--amount", "100",
		"--reason", "Bonus credits",
	}, true)
	if err != nil {
		t.Errorf("runCreditAdjust (topup) error: %v", err)
	}
}

func TestRunCreditDeduct(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/user-1/credits/deduct") {
			t.Errorf("Expected deduct path, got %s", r.URL.Path)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["amount"] != float64(50) {
			t.Errorf("Expected amount=50, got %v", payload["amount"])
		}

		response := map[string]any{
			"user_id":        "user-1",
			"credit_balance": 450,
			"amount":         -50,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditAdjust([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--amount", "50",
		"--reason", "Usage charge",
	}, false)
	if err != nil {
		t.Errorf("runCreditAdjust (deduct) error: %v", err)
	}
}

func TestRunCreditAdjust_MissingUser(t *testing.T) {
	err := runCreditAdjust([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--amount", "100",
	}, true)
	if err == nil {
		t.Error("runCreditAdjust should return error for missing user")
	}
}

func TestRunCreditAdjust_InvalidAmount(t *testing.T) {
	err := runCreditAdjust([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
		"--amount", "0",
	}, true)
	if err == nil {
		t.Error("runCreditAdjust should return error for zero amount")
	}
}

func TestRunCreditTransactions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/user-1/credits/transactions") {
			t.Errorf("Expected transactions path, got %s", r.URL.Path)
		}

		query := r.URL.Query()
		if query.Get("type") != "topup" {
			t.Errorf("Expected type=topup, got %s", query.Get("type"))
		}
		if query.Get("limit") != "25" {
			t.Errorf("Expected limit=25, got %s", query.Get("limit"))
		}

		response := map[string]any{
			"transactions": []map[string]any{
				{
					"id":            "txn-1",
					"type":          "topup",
					"amount":        100,
					"balance_after": 500,
					"description":   "Initial topup",
					"created_at":    "2024-01-01T00:00:00Z",
				},
			},
			"total": 1,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditTransactions([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--type", "topup",
		"--limit", "25",
	})
	if err != nil {
		t.Errorf("runCreditTransactions error: %v", err)
	}
}

func TestRunCreditTransactions_EmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"transactions": []map[string]any{},
			"total":        0,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runCreditTransactions([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err != nil {
		t.Errorf("runCreditTransactions error: %v", err)
	}
}

func TestRunCreditTransactions_MissingUser(t *testing.T) {
	err := runCreditTransactions([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runCreditTransactions should return error for missing user")
	}
}
