package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunUser_MissingSubcommand(t *testing.T) {
	err := runUser([]string{})
	if err == nil {
		t.Error("runUser should return error for missing subcommand")
	}
	if !strings.Contains(err.Error(), "missing user subcommand") {
		t.Errorf("Error should mention missing subcommand, got: %v", err)
	}
}

func TestRunUser_UnknownSubcommand(t *testing.T) {
	err := runUser([]string{"unknown"})
	if err == nil {
		t.Error("runUser should return error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown user subcommand") {
		t.Errorf("Error should mention unknown subcommand, got: %v", err)
	}
}

func TestRunUserList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/users" {
			t.Errorf("Expected path /admin/api/v1/users, got %s", r.URL.Path)
		}

		response := map[string]any{
			"users": []map[string]any{
				{
					"id":             "user-1",
					"email":          "user1@example.com",
					"name":           "User One",
					"role":           "user",
					"status":         "active",
					"credit_balance": 100,
				},
			},
			"total": 1,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserList([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runUserList error: %v", err)
	}
}

func TestRunUserList_WithFilters(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("search") != "test" {
			t.Errorf("Expected search=test, got %s", query.Get("search"))
		}
		if query.Get("status") != "active" {
			t.Errorf("Expected status=active, got %s", query.Get("status"))
		}
		if query.Get("role") != "admin" {
			t.Errorf("Expected role=admin, got %s", query.Get("role"))
		}
		if query.Get("sort") != "email" {
			t.Errorf("Expected sort=email, got %s", query.Get("sort"))
		}
		if query.Get("desc") != "true" {
			t.Errorf("Expected desc=true, got %s", query.Get("desc"))
		}
		if query.Get("limit") != "25" {
			t.Errorf("Expected limit=25, got %s", query.Get("limit"))
		}
		if query.Get("offset") != "10" {
			t.Errorf("Expected offset=10, got %s", query.Get("offset"))
		}

		json.NewEncoder(w).Encode(map[string]any{"users": []map[string]any{}, "total": 0})
	}))
	defer upstream.Close()

	err := runUserList([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--search", "test",
		"--status", "active",
		"--role", "admin",
		"--sort", "email",
		"--desc",
		"--limit", "25",
		"--offset", "10",
	})
	if err != nil {
		t.Errorf("runUserList error: %v", err)
	}
}

func TestRunUserList_EmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"users": []map[string]any{}, "total": 0})
	}))
	defer upstream.Close()

	err := runUserList([]string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runUserList error: %v", err)
	}
}

func TestRunUserCreate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/users" {
			t.Errorf("Expected path /admin/api/v1/users, got %s", r.URL.Path)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["email"] != "newuser@example.com" {
			t.Errorf("Expected email=newuser@example.com, got %v", payload["email"])
		}

		response := map[string]any{
			"id":             "user-new",
			"email":          "newuser@example.com",
			"name":           "New User",
			"role":           "user",
			"status":         "active",
			"credit_balance": 100,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserCreate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--email", "newuser@example.com",
		"--name", "New User",
		"--company", "Test Co",
		"--role", "user",
		"--status", "active",
		"--credits", "100",
	})
	if err != nil {
		t.Errorf("runUserCreate error: %v", err)
	}
}

func TestRunUserCreate_WithPassword(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["password"] != "secret123" {
			t.Errorf("Expected password to be set, got %v", payload["password"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "user-1"})
	}))
	defer upstream.Close()

	err := runUserCreate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--email", "user@example.com",
		"--name", "Test User",
		"--password", "secret123",
	})
	if err != nil {
		t.Errorf("runUserCreate error: %v", err)
	}
}

func TestRunUserCreate_MissingEmail(t *testing.T) {
	err := runUserCreate([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--name", "Test User",
	})
	if err == nil {
		t.Error("runUserCreate should return error for missing email")
	}
}

func TestRunUserCreate_MissingName(t *testing.T) {
	err := runUserCreate([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--email", "user@example.com",
	})
	if err == nil {
		t.Error("runUserCreate should return error for missing name")
	}
}

func TestRunUserGet(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/user-1") {
			t.Errorf("Expected path to end with /user-1, got %s", r.URL.Path)
		}

		response := map[string]any{
			"id":     "user-1",
			"email":  "user1@example.com",
			"name":   "User One",
			"role":   "user",
			"status": "active",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserGet([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--id", "user-1"})
	if err != nil {
		t.Errorf("runUserGet error: %v", err)
	}
}

func TestRunUserGet_PositionalArg(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/user-2") {
			t.Errorf("Expected path to end with /user-2, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "user-2"})
	}))
	defer upstream.Close()

	err := runUserGet([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "user-2"})
	if err != nil {
		t.Errorf("runUserGet error: %v", err)
	}
}

func TestRunUserGet_MissingID(t *testing.T) {
	err := runUserGet([]string{"--admin-url", "http://localhost:9876", "--admin-key", "test-key"})
	if err == nil {
		t.Error("runUserGet should return error for missing ID")
	}
}

func TestRunUserUpdate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/user-1") {
			t.Errorf("Expected path to end with /user-1, got %s", r.URL.Path)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["name"] != "Updated Name" {
			t.Errorf("Expected name=Updated Name, got %v", payload["name"])
		}

		response := map[string]any{
			"id":     "user-1",
			"email":  "user1@example.com",
			"name":   "Updated Name",
			"role":   "user",
			"status": "active",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserUpdate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "user-1",
		"--name", "Updated Name",
	})
	if err != nil {
		t.Errorf("runUserUpdate error: %v", err)
	}
}

func TestRunUserUpdate_WithCredits(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["credit_balance"] != float64(500) {
			t.Errorf("Expected credit_balance=500, got %v", payload["credit_balance"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "user-1", "credit_balance": 500})
	}))
	defer upstream.Close()

	err := runUserUpdate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "user-1",
		"--credits", "500",
	})
	if err != nil {
		t.Errorf("runUserUpdate error: %v", err)
	}
}

func TestRunUserUpdate_WithRateLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		rateLimits, ok := payload["rate_limits"].(map[string]any)
		if !ok {
			t.Errorf("Expected rate_limits in payload, got %v", payload["rate_limits"])
		}
		if rateLimits["requests_per_second"] != float64(100) {
			t.Errorf("Expected requests_per_second=100, got %v", rateLimits["requests_per_second"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "user-1"})
	}))
	defer upstream.Close()

	err := runUserUpdate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "user-1",
		"--rate-limit-rps", "100",
	})
	if err != nil {
		t.Errorf("runUserUpdate error: %v", err)
	}
}

func TestRunUserUpdate_NoFields(t *testing.T) {
	err := runUserUpdate([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--id", "user-1",
	})
	if err == nil {
		t.Error("runUserUpdate should return error when no update fields provided")
	}
}

func TestRunUserStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/suspend") {
			t.Errorf("Expected suspend path, got %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "user-1", "status": "suspended"})
	}))
	defer upstream.Close()

	err := runUserStatus([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--id", "user-1"}, "suspend")
	if err != nil {
		t.Errorf("runUserStatus error: %v", err)
	}
}

func TestRunUserStatus_Activate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/user-1/activate") {
			t.Errorf("Expected activate path, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "user-1", "status": "active"})
	}))
	defer upstream.Close()

	err := runUserStatus([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--id", "user-1"}, "activate")
	if err != nil {
		t.Errorf("runUserStatus error: %v", err)
	}
}

func TestRunUserAPIKey_MissingSubcommand(t *testing.T) {
	err := runUserAPIKey([]string{})
	if err == nil {
		t.Error("runUserAPIKey should return error for missing subcommand")
	}
}

func TestRunUserAPIKey_UnknownSubcommand(t *testing.T) {
	err := runUserAPIKey([]string{"unknown"})
	if err == nil {
		t.Error("runUserAPIKey should return error for unknown subcommand")
	}
}

func TestRunUserAPIKeyList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/api-keys") {
			t.Errorf("Expected api-keys path, got %s", r.URL.Path)
		}

		response := []map[string]any{
			{
				"id":         "key-1",
				"key_prefix": "ck_live_abc",
				"name":       "Production Key",
				"status":     "active",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserAPIKeyList([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
	if err != nil {
		t.Errorf("runUserAPIKeyList error: %v", err)
	}
}

func TestRunUserAPIKeyCreate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["name"] != "My API Key" {
			t.Errorf("Expected name='My API Key', got %v", payload["name"])
		}

		response := map[string]any{
			"key":     "ck_live_full_key_here",
			"api_key": map[string]any{"id": "key-1", "name": "My API Key"},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserAPIKeyCreate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--name", "My API Key",
		"--mode", "live",
	})
	if err != nil {
		t.Errorf("runUserAPIKeyCreate error: %v", err)
	}
}

func TestRunUserAPIKeyRevoke(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/api-keys/key-1") {
			t.Errorf("Expected revoke path, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	err := runUserAPIKeyRevoke([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--key", "key-1",
	})
	if err != nil {
		t.Errorf("runUserAPIKeyRevoke error: %v", err)
	}
}

func TestRunUserPermission_MissingSubcommand(t *testing.T) {
	err := runUserPermission([]string{})
	if err == nil {
		t.Error("runUserPermission should return error for missing subcommand")
	}
}

func TestRunUserPermission_UnknownSubcommand(t *testing.T) {
	err := runUserPermission([]string{"unknown"})
	if err == nil {
		t.Error("runUserPermission should return error for unknown subcommand")
	}
}

func TestRunUserPermissionList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/permissions") {
			t.Errorf("Expected permissions path, got %s", r.URL.Path)
		}

		response := []map[string]any{
			{
				"id":          "perm-1",
				"route_id":    "route-1",
				"methods":     "GET,POST",
				"allowed":     true,
				"credit_cost": 10,
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserPermissionList([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
	if err != nil {
		t.Errorf("runUserPermissionList error: %v", err)
	}
}

func TestRunUserPermissionGrant(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["route_id"] != "route-1" {
			t.Errorf("Expected route_id=route-1, got %v", payload["route_id"])
		}
		if payload["allowed"] != true {
			t.Errorf("Expected allowed=true, got %v", payload["allowed"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "perm-1"})
	}))
	defer upstream.Close()

	err := runUserPermissionGrant([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--route", "route-1",
		"--methods", "GET,POST",
		"--allow",
		"--credit-cost", "10",
	})
	if err != nil {
		t.Errorf("runUserPermissionGrant error: %v", err)
	}
}

func TestRunUserPermissionRevoke(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/permissions/perm-1") {
			t.Errorf("Expected revoke path, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	err := runUserPermissionRevoke([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--permission", "perm-1",
	})
	if err != nil {
		t.Errorf("runUserPermissionRevoke error: %v", err)
	}
}

func TestRunUserIP_MissingSubcommand(t *testing.T) {
	err := runUserIP([]string{})
	if err == nil {
		t.Error("runUserIP should return error for missing subcommand")
	}
}

func TestRunUserIP_UnknownSubcommand(t *testing.T) {
	err := runUserIP([]string{"unknown"})
	if err == nil {
		t.Error("runUserIP should return error for unknown subcommand")
	}
}

func TestRunUserIPList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/ip-whitelist") {
			t.Errorf("Expected ip-whitelist path, got %s", r.URL.Path)
		}

		response := map[string]any{
			"ip_whitelist": []string{"192.168.1.1/24", "10.0.0.1"},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runUserIPList([]string{"--admin-url", upstream.URL, "--admin-key", "test-key", "--user", "user-1"})
	if err != nil {
		t.Errorf("runUserIPList error: %v", err)
	}
}

func TestRunUserIPAdd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["ip"] != "192.168.1.1" {
			t.Errorf("Expected ip=192.168.1.1, got %v", payload["ip"])
		}

		json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer upstream.Close()

	err := runUserIPAdd([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--ip", "192.168.1.1",
	})
	if err != nil {
		t.Errorf("runUserIPAdd error: %v", err)
	}
}

func TestRunUserIPRemove(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/user-1/ip-whitelist/192.168.1.1") {
			t.Errorf("Expected remove path, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	err := runUserIPRemove([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--ip", "192.168.1.1",
	})
	if err != nil {
		t.Errorf("runUserIPRemove error: %v", err)
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "a,b,c",
			expected: []string{"a", "b", "c"},
		},
		{
			input:    "  a  ,  b  ,  c  ",
			expected: []string{"a", "b", "c"},
		},
		{
			input:    "a,,c",
			expected: []string{"a", "c"},
		},
		{
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		result := splitCSV(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}
