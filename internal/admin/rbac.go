package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
)

// RBAC context key
type ctxKey int

const (
	ctxUserRole ctxKey = iota
	ctxUserPerms
)

// UserRole is a recognized admin role.
type UserRole string

const (
	RoleAdmin   UserRole = "admin"
	RoleManager UserRole = "manager"
	RoleUser    UserRole = "user"
	RoleViewer  UserRole = "viewer"
)

// ValidRoles contains all recognized roles.
var ValidRoles = []string{string(RoleAdmin), string(RoleManager), string(RoleUser), string(RoleViewer)}

// Permission IDs — must match web/src/components/users/UserRoleManager.tsx
const (
	PermServicesRead     = "services:read"
	PermServicesWrite    = "services:write"
	PermRoutesRead       = "routes:read"
	PermRoutesWrite      = "routes:write"
	PermUpstreamsRead    = "upstreams:read"
	PermUpstreamsWrite   = "upstreams:write"
	PermPluginsRead      = "plugins:read"
	PermPluginsWrite     = "plugins:write"
	PermUsersRead        = "users:read"
	PermUsersWrite       = "users:write"
	PermUsersImpersonate = "users:impersonate"
	PermCreditsRead      = "credits:read"
	PermCreditsWrite     = "credits:write"
	PermConfigRead       = "config:read"
	PermConfigWrite      = "config:write"
	PermAuditRead        = "audit:read"
	PermAnalyticsRead    = "analytics:read"
	PermClusterRead      = "cluster:read"
	PermClusterWrite     = "cluster:write"
	PermAlertsRead       = "alerts:read"
	PermAlertsWrite      = "alerts:write"
)

// RolePermissions maps each role to its permission IDs.
var RolePermissions = map[UserRole][]string{
	RoleAdmin: {
		PermServicesRead, PermServicesWrite, PermRoutesRead, PermRoutesWrite,
		PermUpstreamsRead, PermUpstreamsWrite, PermPluginsRead, PermPluginsWrite,
		PermUsersRead, PermUsersWrite, PermUsersImpersonate,
		PermCreditsRead, PermCreditsWrite,
		PermConfigRead, PermConfigWrite, PermAuditRead, PermAnalyticsRead,
		PermClusterRead, PermClusterWrite, PermAlertsRead, PermAlertsWrite,
	},
	RoleManager: {
		PermServicesRead, PermServicesWrite, PermRoutesRead, PermRoutesWrite,
		PermUpstreamsRead, PermUpstreamsWrite, PermPluginsRead, PermPluginsWrite,
		PermUsersRead, PermUsersWrite,
		PermCreditsRead, PermCreditsWrite,
		PermConfigRead, PermAuditRead, PermAnalyticsRead,
		PermAlertsRead, PermAlertsWrite,
	},
	RoleUser: {
		PermServicesRead, PermRoutesRead, PermUpstreamsRead, PermPluginsRead,
		PermCreditsRead, PermAnalyticsRead,
	},
	RoleViewer: {
		PermServicesRead, PermRoutesRead, PermUpstreamsRead, PermPluginsRead,
		PermAnalyticsRead,
	},
}

// endpointPermissions maps admin API route prefixes to the minimum required
// permission. The lookup function matches by longest prefix.
var endpointPermissions = map[string]string{
	// Services
	"GET /admin/api/v1/services":    PermServicesRead,
	"POST /admin/api/v1/services":   PermServicesWrite,
	"PUT /admin/api/v1/services":    PermServicesWrite,
	"DELETE /admin/api/v1/services": PermServicesWrite,

	// Routes
	"GET /admin/api/v1/routes":    PermRoutesRead,
	"POST /admin/api/v1/routes":   PermRoutesWrite,
	"PUT /admin/api/v1/routes":    PermRoutesWrite,
	"DELETE /admin/api/v1/routes": PermRoutesWrite,

	// Upstreams
	"GET /admin/api/v1/upstreams":    PermUpstreamsRead,
	"POST /admin/api/v1/upstreams":   PermUpstreamsWrite,
	"PUT /admin/api/v1/upstreams":    PermUpstreamsWrite,
	"DELETE /admin/api/v1/upstreams": PermUpstreamsWrite,

	// Users (general)
	"GET /admin/api/v1/users":    PermUsersRead,
	"POST /admin/api/v1/users":   PermUsersWrite,
	"PUT /admin/api/v1/users":    PermUsersWrite,
	"DELETE /admin/api/v1/users": PermUsersWrite,

	// Credits
	"GET /admin/api/v1/credits":  PermCreditsRead,
	"POST /admin/api/v1/credits": PermCreditsWrite,

	// Config
	"GET /admin/api/v1/config":  PermConfigRead,
	"POST /admin/api/v1/config": PermConfigWrite,
	"PUT /admin/api/v1/config":  PermConfigWrite,

	// Audit
	"GET /admin/api/v1/audit-logs":        PermAuditRead,
	"DELETE /admin/api/v1/audit-logs":     PermConfigWrite,
	"GET /admin/api/v1/audit-logs/export": PermAuditRead,

	// Analytics
	"GET /admin/api/v1/analytics": PermAnalyticsRead,

	// Alerts
	"GET /admin/api/v1/alerts":    PermAlertsRead,
	"POST /admin/api/v1/alerts":   PermAlertsWrite,
	"PUT /admin/api/v1/alerts":    PermAlertsWrite,
	"DELETE /admin/api/v1/alerts": PermAlertsWrite,

	// Cluster (via subgraphs / federation)
	"GET /admin/api/v1/subgraphs":    PermClusterRead,
	"POST /admin/api/v1/subgraphs":   PermClusterWrite,
	"DELETE /admin/api/v1/subgraphs": PermClusterWrite,

	// Status / info (read-only)
	"GET /admin/api/v1/status": PermConfigRead,
	"GET /admin/api/v1/info":   PermConfigRead,

	// Auth (token management)
	"POST /admin/api/v1/auth/token":          PermConfigWrite,
	"POST /admin/api/v1/auth/logout":         PermConfigWrite,
	"POST /admin/api/v1/auth/sso/logout":     PermConfigWrite,
	"GET /admin/api/v1/auth/sso/status":      PermConfigRead,

	// Branding
	"GET /admin/api/v1/branding": PermConfigRead,

	// Billing
	"GET /admin/api/v1/billing": PermCreditsRead,
	"PUT /admin/api/v1/billing": PermCreditsWrite,

	// Bulk operations
	"POST /admin/api/v1/bulk/services": PermServicesWrite,
	"POST /admin/api/v1/bulk/routes":  PermRoutesWrite,
	"POST /admin/api/v1/bulk/delete":  PermConfigWrite,
	"POST /admin/api/v1/bulk/plugins": PermPluginsWrite,
	"POST /admin/api/v1/bulk/import":  PermConfigWrite,

	// GraphQL (uses read permissions for queries)
	"POST /admin/graphql": PermServicesRead,
	"GET /admin/graphql":  PermServicesRead,

	// Webhooks (related to alerts)
	"GET /admin/api/v1/webhooks":    PermAlertsRead,
	"POST /admin/api/v1/webhooks":   PermAlertsWrite,
	"GET /admin/api/v1/webhooks/events":             PermAlertsRead,
	"PUT /admin/api/v1/webhooks/{id}":              PermAlertsWrite,
	"DELETE /admin/api/v1/webhooks/{id}":           PermAlertsWrite,
	"GET /admin/api/v1/webhooks/{id}/deliveries":   PermAlertsRead,
	"POST /admin/api/v1/webhooks/{id}/test":        PermAlertsWrite,
	"POST /admin/api/v1/webhooks/{id}/rotate-secret": PermAlertsWrite,
}

// lookupPermissionForEndpoint returns the required permission for a given
// method + path prefix, or empty string if no mapping exists (allow by default).
func lookupPermissionForEndpoint(method, path string) string {
	// Exact match first
	if perm, ok := endpointPermissions[method+" "+path]; ok {
		return perm
	}

	// Try matching with normalized path (replace IDs with {id} placeholder)
	// This handles paths like /admin/api/v1/webhooks/wh_abc123 matching /admin/api/v1/webhooks/{id}
	normalized := normalizePathWithIDPlaceholder(path)
	if perm, ok := endpointPermissions[method+" "+normalized]; ok {
		return perm
	}

	// Try removing the last path segment and checking if that matches.
	// This handles paths like /admin/api/v1/webhooks/nonexistent where "nonexistent" is not
	// recognized as an ID but /admin/api/v1/webhooks/{id} should still apply.
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 4 {
		// Try with last segment replaced to {id}
		normalizedParts := make([]string, len(parts))
		copy(normalizedParts, parts)
		normalizedParts[len(parts)-1] = "{id}"
		normalizedWithID := "/" + strings.Join(normalizedParts, "/")
		if perm, ok := endpointPermissions[method+" "+normalizedWithID]; ok {
			return perm
		}
	}

	// Prefix match: walk from longest prefix to shortest.
	// E.g. "GET /admin/api/v1/services/svc-123" →
	// try "GET /admin/api/v1/services/svc-123", then
	// "GET /admin/api/v1/services", etc.
	for i := len(parts); i >= 3; i-- {
		prefix := "/" + strings.Join(parts[:i], "/")
		if perm, ok := endpointPermissions[method+" "+prefix]; ok {
			return perm
		}
	}
	return ""
}

// normalizePathWithIDPlaceholder replaces path segments that look like IDs with {id}.
// This allows /admin/api/v1/webhooks/wh_abc123 to match /admin/api/v1/webhooks/{id}.
func normalizePathWithIDPlaceholder(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		// Skip first 3 segments (admin, api, v1) and empty parts
		if i < 3 || part == "" {
			continue
		}
		// Replace segments that look like IDs (UUIDs, prefixed IDs like wh_*, srv_*)
		// with {id} placeholder
		if isLikelyID(part) {
			parts[i] = "{id}"
		}
	}
	return "/" + strings.Join(parts, "/")
}

// isLikelyID returns true if the segment looks like an ID (UUID or prefix_randomid)
func isLikelyID(segment string) bool {
	if segment == "" || segment == "{id}" {
		return false
	}
	// UUID pattern (8-4-4-4-12 hex digits with dashes)
	if len(segment) == 36 {
		dashes := 0
		for i, c := range segment {
			if c == '-' {
				if !((i == 8 || i == 13 || i == 18 || i == 23)) {
					return false
				}
				dashes++
			} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		if dashes == 4 {
			return true
		}
	}
	// Prefix pattern: type_prefix (e.g., wh_abc123, srv_xyz) OR type-prefix (e.g., wh-test-123)
	// Common prefixes in this codebase - check both underscore and dash separators
	if strings.Contains(segment, "_") || strings.Contains(segment, "-") {
		// Get prefix before first separator
		var prefix string
		if idx := strings.Index(segment, "_"); idx != -1 {
			prefix = segment[:idx]
		} else if idx := strings.Index(segment, "-"); idx != -1 {
			prefix = segment[:idx]
		}
		if prefix == "wh" || prefix == "srv" || prefix == "up" || prefix == "route" ||
			prefix == "svc" || prefix == "key" || prefix == "user" || prefix == "consumer" ||
			prefix == "token" || prefix == "webhook" {
			return true
		}
	}
	return false
}

// withRBAC wraps a handler that has already passed authentication. It extracts
// the user's role/permissions from the JWT, checks that the role is valid,
// and verifies the user has permission for the requested endpoint.
func (s *Server) withRBAC(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(ctxUserRole).(string)
		if !ok || role == "" {
			// Static API key auth must not bypass RBAC — deny if no role is established.
			writeError(w, http.StatusForbidden, "permission_denied", "role not established in request context")
			return
		}

		perms, _ := r.Context().Value(ctxUserPerms).([]string)

		// Validate role
		if !slices.Contains(ValidRoles, role) {
			writeError(w, http.StatusForbidden, "invalid_role",
				fmt.Sprintf("Role %q is not a recognized role", role))
			return
		}

		// Check permission
		requiredPerm := lookupPermissionForEndpoint(r.Method, r.URL.Path)
		if requiredPerm == "" {
			// M-013 FIX: No explicit permission mapping — DEFAULT DENY for security.
			// Unmapped endpoints should not automatically allow access.
			// Configure the endpoint in endpointPermissions or deny by default.
			writeError(w, http.StatusForbidden, "permission_denied",
				"endpoint not classified for RBAC; access denied by default")
			return
		}

		if !slices.Contains(perms, requiredPerm) {
			writeError(w, http.StatusForbidden, "permission_denied",
				fmt.Sprintf("Role %q lacks permission %q", role, requiredPerm))
			return
		}

		next(w, r)
	}
}

// extractRoleFromJWT parses the role and permissions from the verified admin
// JWT payload. Must be called AFTER verifyAdminToken succeeds.
func extractRoleFromJWT(tokenString string) (role string, perms []string) {
	parts := strings.Split(strings.TrimSpace(tokenString), ".")
	if len(parts) != 3 {
		return "", nil
	}
	decoded, err := jwt.DecodeSegment(parts[1])
	if err != nil {
		return "", nil
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", nil
	}
	role, _ = payload["role"].(string)
	if raw, ok := payload["permissions"]; ok {
		if arr, ok := raw.([]any); ok {
			perms = make([]string, 0, len(arr))
			for _, p := range arr {
				if s, ok := p.(string); ok {
					perms = append(perms, s)
				}
			}
		}
	}
	return role, perms
}
