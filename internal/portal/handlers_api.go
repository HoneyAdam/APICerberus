package portal

import (
	"net/http"
	"strings"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	payload := map[string]any{}
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	oldPassword := strings.TrimSpace(asString(payload["old_password"]))
	newPassword := strings.TrimSpace(asString(payload["new_password"]))
	if oldPassword == "" || newPassword == "" {
		writeError(w, http.StatusBadRequest, "invalid_password", "old_password and new_password are required")
		return
	}
	if !store.VerifyPassword(user.PasswordHash, oldPassword) {
		writeError(w, http.StatusUnauthorized, "invalid_password", "old password is incorrect")
		return
	}
	hash, err := store.HashPassword(newPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	user.PasswordHash = hash
	if err := s.store.Users().Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, "password_update_failed", "failed to update password")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"password_changed": true})
}

func (s *Server) listMyAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	keys, err := s.store.APIKeys().ListByUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_api_keys_failed", "failed to list api keys")
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, map[string]any{
			"id":           key.ID,
			"name":         key.Name,
			"key_prefix":   key.KeyPrefix,
			"status":       key.Status,
			"expires_at":   key.ExpiresAt,
			"last_used_at": key.LastUsedAt,
			"last_used_ip": key.LastUsedIP,
			"created_at":   key.CreatedAt,
			"updated_at":   key.UpdatedAt,
		})
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func (s *Server) createMyAPIKey(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	payload := map[string]any{}
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	name := strings.TrimSpace(asString(payload["name"]))
	mode := strings.TrimSpace(asString(payload["mode"]))
	token, key, err := s.store.APIKeys().Create(user.ID, name, mode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_api_key_failed", "failed to create api key")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"key": map[string]any{
			"id":           key.ID,
			"name":         key.Name,
			"key_prefix":   key.KeyPrefix,
			"status":       key.Status,
			"expires_at":   key.ExpiresAt,
			"last_used_at": key.LastUsedAt,
			"last_used_ip": key.LastUsedIP,
			"created_at":   key.CreatedAt,
			"updated_at":   key.UpdatedAt,
		},
	})
}

func (s *Server) renameMyAPIKey(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_api_key", "api key id is required")
		return
	}
	payload := map[string]any{}
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	name := strings.TrimSpace(asString(payload["name"]))
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_api_key", "api key name is required")
		return
	}
	if err := s.store.APIKeys().RenameForUser(id, user.ID, name); err != nil {
		writeError(w, http.StatusBadRequest, "rename_api_key_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"name":    name,
		"renamed": true,
	})
}

func (s *Server) revokeMyAPIKey(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_api_key", "api key id is required")
		return
	}
	if err := s.store.APIKeys().RevokeForUser(id, user.ID); err != nil {
		writeError(w, http.StatusBadRequest, "revoke_api_key_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listMyAPIs(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	snapshot := s.configSnapshot()
	permissions, err := s.store.Permissions().ListByUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_permissions_failed", "failed to resolve user permissions")
		return
	}
	items := buildAPIList(snapshot, permissions)
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func (s *Server) getMyAPIDetail(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	routeID := strings.TrimSpace(r.PathValue("routeId"))
	if routeID == "" {
		writeError(w, http.StatusBadRequest, "invalid_route", "route id is required")
		return
	}
	snapshot := s.configSnapshot()
	permissions, err := s.store.Permissions().ListByUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_permissions_failed", "failed to resolve user permissions")
		return
	}
	route, service, permission := findAPIDetail(snapshot, permissions, routeID)
	if route == nil {
		writeError(w, http.StatusNotFound, "route_not_found", "route not found")
		return
	}
	if len(permissions) > 0 && (permission == nil || !permission.Allowed) {
		writeError(w, http.StatusForbidden, "route_forbidden", "route access is not allowed")
		return
	}

	creditCost := resolveRouteCreditCost(snapshot.Billing, route, permission)
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"route": map[string]any{
			"id":          route.ID,
			"name":        route.Name,
			"hosts":       route.Hosts,
			"paths":       route.Paths,
			"methods":     route.Methods,
			"strip_path":  route.StripPath,
			"priority":    route.Priority,
			"credit_cost": creditCost,
		},
		"service": map[string]any{
			"id":       service.ID,
			"name":     service.Name,
			"protocol": service.Protocol,
			"upstream": service.Upstream,
		},
		"permission": permission,
	})
}
