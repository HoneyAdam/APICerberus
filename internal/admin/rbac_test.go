package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupPermissionForEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		expected string
	}{
		// Exact matches
		{"list services", "GET", "/admin/api/v1/services", PermServicesRead},
		{"create service", "POST", "/admin/api/v1/services", PermServicesWrite},
		{"delete service", "DELETE", "/admin/api/v1/services/svc-123", PermServicesWrite},

		// Prefix matches
		{"get service by id", "GET", "/admin/api/v1/services/svc-abc", PermServicesRead},
		{"update route by id", "PUT", "/admin/api/v1/routes/route-xyz", PermRoutesWrite},
		{"get user by id", "GET", "/admin/api/v1/users/usr-123", PermUsersRead},
		{"get user credits", "GET", "/admin/api/v1/users/usr-123/credits", PermUsersRead},
		{"get user api keys", "GET", "/admin/api/v1/users/usr-123/api-keys", PermUsersRead},

		// Credits (top-level)
		{"list credits overview", "GET", "/admin/api/v1/credits/overview", PermCreditsRead},
		{"adjust credits", "POST", "/admin/api/v1/users/usr-1/credits/topup", PermUsersWrite},

		// Config
		{"export config", "GET", "/admin/api/v1/config/export", PermConfigRead},
		{"reload config", "POST", "/admin/api/v1/config/reload", PermConfigWrite},

		// Audit
		{"search audit", "GET", "/admin/api/v1/audit-logs", PermAuditRead},
		{"export audit", "GET", "/admin/api/v1/audit-logs/export", PermAuditRead},
		{"cleanup audit", "DELETE", "/admin/api/v1/audit-logs/cleanup", PermConfigWrite},

		// Analytics
		{"overview", "GET", "/admin/api/v1/analytics/overview", PermAnalyticsRead},
		{"top routes", "GET", "/admin/api/v1/analytics/top-routes", PermAnalyticsRead},

		// Alerts
		{"list alerts", "GET", "/admin/api/v1/alerts", PermAlertsRead},
		{"create alert", "POST", "/admin/api/v1/alerts", PermAlertsWrite},
		{"delete alert", "DELETE", "/admin/api/v1/alerts/alert-1", PermAlertsWrite},

		// Subgraphs (cluster)
		{"list subgraphs", "GET", "/admin/api/v1/subgraphs", PermClusterRead},
		{"add subgraph", "POST", "/admin/api/v1/subgraphs", PermClusterWrite},

		// Status / info
		{"status", "GET", "/admin/api/v1/status", PermConfigRead},
		{"info", "GET", "/admin/api/v1/info", PermConfigRead},

		// Unknown endpoints — no permission required (new endpoints)
		{"unknown endpoint", "GET", "/admin/api/v1/unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lookupPermissionForEndpoint(tt.method, tt.path)
			if got != tt.expected {
				t.Errorf("lookupPermissionForEndpoint(%q, %q) = %q, want %q",
					tt.method, tt.path, got, tt.expected)
			}
		})
	}
}

func TestRolePermissions_Consistency(t *testing.T) {
	t.Parallel()

	// Admin should have all permissions
	allPerms := make(map[string]bool)
	for _, p := range RolePermissions[RoleAdmin] {
		allPerms[p] = true
	}
	// Admin should have all defined permission constants
	for _, perm := range []string{
		PermServicesRead, PermServicesWrite, PermRoutesRead, PermRoutesWrite,
		PermUpstreamsRead, PermUpstreamsWrite, PermPluginsRead, PermPluginsWrite,
		PermUsersRead, PermUsersWrite, PermUsersImpersonate,
		PermCreditsRead, PermCreditsWrite, PermConfigRead, PermConfigWrite,
		PermAuditRead, PermAnalyticsRead, PermClusterRead, PermClusterWrite,
		PermAlertsRead, PermAlertsWrite,
	} {
		if !allPerms[perm] {
			t.Errorf("admin role missing permission %q", perm)
		}
	}

	// Manager should NOT have cluster or config write
	mgrPerms := make(map[string]bool)
	for _, p := range RolePermissions[RoleManager] {
		mgrPerms[p] = true
	}
	for _, forbidden := range []string{PermClusterRead, PermClusterWrite, PermConfigWrite} {
		if mgrPerms[forbidden] {
			t.Errorf("manager role should not have permission %q", forbidden)
		}
	}

	// Viewer should only have read permissions
	for _, p := range RolePermissions[RoleViewer] {
		if len(p) > 5 && p[len(p)-5:] == "write" {
			t.Errorf("viewer role should not have write permission %q", p)
		}
	}
}

func TestValidRoles_MatchFrontend(t *testing.T) {
	t.Parallel()

	expected := []string{"admin", "manager", "user", "viewer"}
	if len(ValidRoles) != len(expected) {
		t.Fatalf("expected %d roles, got %d", len(expected), len(ValidRoles))
	}
	for i, role := range expected {
		if ValidRoles[i] != role {
			t.Errorf("role[%d] = %q, want %q", i, ValidRoles[i], role)
		}
	}
}

func TestExtractRoleFromJWT(t *testing.T) {
	t.Parallel()

	token, err := issueAdminToken("test-secret-at-least-32-chars-long!!", 0, "manager", []string{PermServicesRead, PermRoutesRead})
	if err != nil {
		t.Fatalf("issueAdminToken: %v", err)
	}

	role, perms := extractRoleFromJWT(token)
	if role != "manager" {
		t.Errorf("role = %q, want %q", role, "manager")
	}
	if len(perms) != 2 {
		t.Errorf("perms = %v, want 2 permissions", perms)
	}
	if perms[0] != PermServicesRead || perms[1] != PermRoutesRead {
		t.Errorf("perms = %v, want [services:read routes:read]", perms)
	}
}

func TestExtractRoleFromJWT_NoRole(t *testing.T) {
	t.Parallel()

	token, err := issueAdminToken("test-secret-at-least-32-chars-long!!", 0, "", nil)
	if err != nil {
		t.Fatalf("issueAdminToken: %v", err)
	}

	role, perms := extractRoleFromJWT(token)
	if role != "" {
		t.Errorf("expected empty role, got %q", role)
	}
	if perms != nil {
		t.Errorf("expected nil perms, got %v", perms)
	}
}

func TestExtractRoleFromJWT_InvalidToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no dots", "notajwt"},
		{"two parts", "abc.def"},
		{"four parts", "a.b.c.d"},
		{"invalid base64", "!!!.@@@.###"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			role, perms := extractRoleFromJWT(tt.token)
			if role != "" || perms != nil {
				t.Errorf("expected empty result for invalid token, got role=%q perms=%v", role, perms)
			}
		})
	}
}

// contextWithRole returns a context that has role and permissions set
// for testing the withRBAC middleware.
func contextWithRole(parent context.Context, role string, perms []string) context.Context {
	ctx := context.WithValue(parent, ctxUserRole, role)
	ctx = context.WithValue(ctx, ctxUserPerms, perms)
	return ctx
}

func TestWithRBAC_AllowsWithPermission(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	req := httptest.NewRequest("GET", "/admin/api/v1/services", nil)
	req = req.WithContext(contextWithRole(req.Context(), "viewer", []string{PermServicesRead}))

	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if !called {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWithRBAC_DeniesWithoutPermission(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	req := httptest.NewRequest("GET", "/admin/api/v1/services", nil)
	req = req.WithContext(contextWithRole(req.Context(), "viewer", []string{PermAnalyticsRead}))

	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if called {
		t.Error("expected handler NOT to be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("expected error object in response")
	}
	if errObj["code"] != "permission_denied" {
		t.Errorf("error code = %v, want permission_denied", errObj["code"])
	}
}

func TestWithRBAC_UnknownEndpointDenied(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	// M-013 FIX: Endpoint not in the permission map — should be DENIED by default (security)
	req := httptest.NewRequest("GET", "/admin/api/v1/unknown/thing", nil)
	req = req.WithContext(contextWithRole(req.Context(), "viewer", []string{PermAnalyticsRead}))

	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if called {
		t.Error("expected handler NOT to be called for unmapped endpoint (M-013 default-deny)")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestWithRBAC_AllowsNoRole(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	// No role in context — should be denied (H8 fix)
	req := httptest.NewRequest("DELETE", "/admin/api/v1/services/svc-1", nil)
	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for no role, got %d", rec.Code)
	}
	if called {
		t.Error("handler should not be called when role is missing")
	}
}

func TestWithRBAC_RejectsInvalidRole(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	req := httptest.NewRequest("GET", "/admin/api/v1/services", nil)
	req = req.WithContext(contextWithRole(req.Context(), "superadmin", []string{PermServicesRead}))

	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if called {
		t.Error("expected handler NOT to be called for invalid role")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestWithRBAC_AdminHasAllPermissions(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	req := httptest.NewRequest("DELETE", "/admin/api/v1/services/svc-1", nil)
	req = req.WithContext(contextWithRole(req.Context(), "admin", RolePermissions[RoleAdmin]))

	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if !called {
		t.Error("expected admin to have access to all endpoints")
	}
}

func TestWithRBAC_ManagerDenied(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	called := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := srv.withRBAC(handler)

	// Manager doesn't have cluster permissions
	req := httptest.NewRequest("POST", "/admin/api/v1/subgraphs", nil)
	req = req.WithContext(contextWithRole(req.Context(), "manager", RolePermissions[RoleManager]))

	rec := httptest.NewRecorder()
	wrapped(rec, req)

	if called {
		t.Error("expected manager to be denied cluster write")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestWithRBAC_UserCanOnlyRead(t *testing.T) {
	t.Parallel()

	srv := &Server{}

	tests := []struct {
		name    string
		method  string
		path    string
		allowed bool
	}{
		{"user read services", "GET", "/admin/api/v1/services", true},
		{"user write services", "POST", "/admin/api/v1/services", false},
		{"user read routes", "GET", "/admin/api/v1/routes", true},
		{"user write routes", "POST", "/admin/api/v1/routes", false},
		{"user read analytics", "GET", "/admin/api/v1/analytics/overview", true},
		{"user read audit", "GET", "/admin/api/v1/audit-logs", false},
		{"user write users", "POST", "/admin/api/v1/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			wrapped := srv.withRBAC(handler)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req = req.WithContext(contextWithRole(req.Context(), "user", RolePermissions[RoleUser]))

			rec := httptest.NewRecorder()
			wrapped(rec, req)

			if called != tt.allowed {
				t.Errorf("called=%v, want %v (status=%d)", called, tt.allowed, rec.Code)
			}
		})
	}
}
