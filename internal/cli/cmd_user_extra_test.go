package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunUserStatus_MissingID tests missing ID error
func TestRunUserStatus_MissingID(t *testing.T) {
	err := runUserStatus([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	}, "suspend")
	if err == nil {
		t.Error("runUserStatus should return error for missing ID")
	}
	if !strings.Contains(err.Error(), "user id") {
		t.Errorf("Error should mention user id, got: %v", err)
	}
}

// TestRunUserStatus_APIError tests API error handling
func TestRunUserStatus_APIError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"error": "server error"})
	}))
	defer upstream.Close()

	err := runUserStatus([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "user-1",
	}, "suspend")
	if err == nil {
		t.Error("runUserStatus should return error for API failure")
	}
}

// TestRunUserAPIKeyList_MissingUser tests missing user error
func TestRunUserAPIKeyList_MissingUser(t *testing.T) {
	err := runUserAPIKeyList([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runUserAPIKeyList should return error for missing user")
	}
}

// TestRunUserAPIKeyList_EmptyResults tests empty results handling
func TestRunUserAPIKeyList_EmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer upstream.Close()

	err := runUserAPIKeyList([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err != nil {
		t.Errorf("runUserAPIKeyList error: %v", err)
	}
}

// TestRunUserAPIKeyCreate_MissingUser tests missing user error
func TestRunUserAPIKeyCreate_MissingUser(t *testing.T) {
	err := runUserAPIKeyCreate([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--name", "Test Key",
	})
	if err == nil {
		t.Error("runUserAPIKeyCreate should return error for missing user")
	}
}

// TestRunUserAPIKeyCreate_MissingName tests missing name error
func TestRunUserAPIKeyCreate_MissingName(t *testing.T) {
	err := runUserAPIKeyCreate([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err == nil {
		t.Error("runUserAPIKeyCreate should return error for missing name")
	}
}

// TestRunUserAPIKeyRevoke_MissingUser tests missing user error
func TestRunUserAPIKeyRevoke_MissingUser(t *testing.T) {
	err := runUserAPIKeyRevoke([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--key", "key-1",
	})
	if err == nil {
		t.Error("runUserAPIKeyRevoke should return error for missing user")
	}
}

// TestRunUserAPIKeyRevoke_MissingKey tests missing key error
func TestRunUserAPIKeyRevoke_MissingKey(t *testing.T) {
	err := runUserAPIKeyRevoke([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err == nil {
		t.Error("runUserAPIKeyRevoke should return error for missing key")
	}
}

// TestRunUserPermissionList_MissingUser tests missing user error
func TestRunUserPermissionList_MissingUser(t *testing.T) {
	err := runUserPermissionList([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runUserPermissionList should return error for missing user")
	}
}

// TestRunUserPermissionList_EmptyResults tests empty results handling
func TestRunUserPermissionList_EmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer upstream.Close()

	err := runUserPermissionList([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err != nil {
		t.Errorf("runUserPermissionList error: %v", err)
	}
}

// TestRunUserPermissionGrant_MissingUser tests missing user error
func TestRunUserPermissionGrant_MissingUser(t *testing.T) {
	err := runUserPermissionGrant([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--route", "route-1",
	})
	if err == nil {
		t.Error("runUserPermissionGrant should return error for missing user")
	}
}

// TestRunUserPermissionGrant_MissingRoute tests missing route error
func TestRunUserPermissionGrant_MissingRoute(t *testing.T) {
	err := runUserPermissionGrant([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err == nil {
		t.Error("runUserPermissionGrant should return error for missing route")
	}
}

// TestRunUserPermissionGrant_Deny tests deny permission
func TestRunUserPermissionGrant_Deny(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["allowed"] != false {
			t.Errorf("Expected allowed=false, got %v", payload["allowed"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "perm-1"})
	}))
	defer upstream.Close()

	// Use --allow=false to deny permission
	err := runUserPermissionGrant([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--user", "user-1",
		"--route", "route-1",
		"--allow=false",
	})
	if err != nil {
		t.Errorf("runUserPermissionGrant error: %v", err)
	}
}

// TestRunUserPermissionRevoke_MissingUser tests missing user error
func TestRunUserPermissionRevoke_MissingUser(t *testing.T) {
	err := runUserPermissionRevoke([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--permission", "perm-1",
	})
	if err == nil {
		t.Error("runUserPermissionRevoke should return error for missing user")
	}
}

// TestRunUserPermissionRevoke_MissingPermission tests missing permission error
func TestRunUserPermissionRevoke_MissingPermission(t *testing.T) {
	err := runUserPermissionRevoke([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err == nil {
		t.Error("runUserPermissionRevoke should return error for missing permission")
	}
}

// TestRunUserIPList_MissingUser tests missing user error
func TestRunUserIPList_MissingUser(t *testing.T) {
	err := runUserIPList([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runUserIPList should return error for missing user")
	}
}

// TestRunUserIPAdd_MissingUser tests missing user error
func TestRunUserIPAdd_MissingUser(t *testing.T) {
	err := runUserIPAdd([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--ip", "192.168.1.1",
	})
	if err == nil {
		t.Error("runUserIPAdd should return error for missing user")
	}
}

// TestRunUserIPAdd_MissingIP tests missing IP error
func TestRunUserIPAdd_MissingIP(t *testing.T) {
	err := runUserIPAdd([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err == nil {
		t.Error("runUserIPAdd should return error for missing IP")
	}
}

// TestRunUserIPRemove_MissingUser tests missing user error
func TestRunUserIPRemove_MissingUser(t *testing.T) {
	err := runUserIPRemove([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--ip", "192.168.1.1",
	})
	if err == nil {
		t.Error("runUserIPRemove should return error for missing user")
	}
}

// TestRunUserIPRemove_MissingIP tests missing IP error
func TestRunUserIPRemove_MissingIP(t *testing.T) {
	err := runUserIPRemove([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--user", "user-1",
	})
	if err == nil {
		t.Error("runUserIPRemove should return error for missing IP")
	}
}

// TestRequireArg_WithError tests requireArg with an error input
func TestRequireArg_WithError(t *testing.T) {
	// Simulate when a flag returns an error during parsing
	// This is covered by passing an empty value
	_, err := requireArg("", "test-arg")
	if err == nil {
		t.Error("requireArg should return error for empty value")
	}
}

// TestRunUserList_APIError tests API error handling
func TestRunUserList_APIError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"error": "server error"})
	}))
	defer upstream.Close()

	err := runUserList([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runUserList should return error for API failure")
	}
}

// TestRunUserCreate_APIError tests API error handling
func TestRunUserCreate_APIError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "invalid email"})
	}))
	defer upstream.Close()

	err := runUserCreate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--email", "invalid",
		"--name", "Test",
	})
	if err == nil {
		t.Error("runUserCreate should return error for API failure")
	}
}

// TestRunUserUpdate_MissingID tests missing ID error
func TestRunUserUpdate_MissingID(t *testing.T) {
	err := runUserUpdate([]string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--name", "New Name",
	})
	if err == nil {
		t.Error("runUserUpdate should return error for missing ID")
	}
}

// TestRunUserUpdate_APIError tests API error handling
func TestRunUserUpdate_APIError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "user not found"})
	}))
	defer upstream.Close()

	err := runUserUpdate([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "user-1",
		"--name", "New Name",
	})
	if err == nil {
		t.Error("runUserUpdate should return error for API failure")
	}
}

// TestRunUserGet_APIError tests API error handling
func TestRunUserGet_APIError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "user not found"})
	}))
	defer upstream.Close()

	err := runUserGet([]string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "user-1",
	})
	if err == nil {
		t.Error("runUserGet should return error for API failure")
	}
}
