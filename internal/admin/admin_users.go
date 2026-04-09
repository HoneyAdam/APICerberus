package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	query := r.URL.Query()
	limit, _ := strconv.Atoi(strings.TrimSpace(query.Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(query.Get("offset")))
	sortDesc := strings.EqualFold(strings.TrimSpace(query.Get("sort_desc")), "true")

	result, err := st.Users().List(store.UserListOptions{
		Search:   strings.TrimSpace(query.Get("search")),
		Status:   strings.TrimSpace(query.Get("status")),
		Role:     strings.TrimSpace(query.Get("role")),
		SortBy:   strings.TrimSpace(query.Get("sort_by")),
		SortDesc: sortDesc,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_users_failed", err.Error())
		return
	}
	// Sanitize users before returning
	users := make([]map[string]any, 0, len(result.Users))
	for i := range result.Users {
		users = append(users, sanitizedUser(&result.Users[i]))
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"users": users,
		"total": result.Total,
	})
}

// sanitizedUser returns a user map with sensitive fields removed.
func sanitizedUser(u *store.User) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"id":             u.ID,
		"email":          u.Email,
		"name":           u.Name,
		"company":        u.Company,
		"role":           u.Role,
		"status":         u.Status,
		"credit_balance": u.CreditBalance,
		"ip_whitelist":   u.IPWhitelist,
		"metadata":       u.Metadata,
		"rate_limits":    u.RateLimits,
		"created_at":     u.CreatedAt,
		"updated_at":     u.UpdatedAt,
	}
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	password := strings.TrimSpace(asString(payload["password"]))
	if password == "" {
		writeError(w, http.StatusBadRequest, "invalid_payload", "password is required")
		return
	}
	if len(password) < 8 {
		writeError(w, http.StatusBadRequest, "invalid_password", "password must be at least 8 characters")
		return
	}
	passwordHash, err := store.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user", err.Error())
		return
	}

	user := &store.User{
		Email:         strings.TrimSpace(asString(payload["email"])),
		Name:          strings.TrimSpace(asString(payload["name"])),
		Company:       strings.TrimSpace(asString(payload["company"])),
		PasswordHash:  passwordHash,
		Role:          normalizeDefault(asString(payload["role"]), "user"),
		Status:        normalizeDefault(asString(payload["status"]), "active"),
		CreditBalance: int64(asInt(payload["credit_balance"], asInt(payload["initial_credits"], 0))),
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Users().Create(user); err != nil {
		writeError(w, http.StatusBadRequest, "create_user_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, sanitizedUser(user))
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "get_user_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, sanitizedUser(user))
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_user_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	if value := strings.TrimSpace(asString(payload["email"])); value != "" {
		user.Email = value
	}
	if value := strings.TrimSpace(asString(payload["name"])); value != "" {
		user.Name = value
	}
	if value := strings.TrimSpace(asString(payload["company"])); value != "" {
		user.Company = value
	}
	if value := strings.TrimSpace(asString(payload["role"])); value != "" {
		user.Role = value
	}
	if value := strings.TrimSpace(asString(payload["status"])); value != "" {
		user.Status = value
	}
	if _, ok := payload["credit_balance"]; ok {
		user.CreditBalance = int64(asInt(payload["credit_balance"], int(user.CreditBalance)))
	}
	if password := strings.TrimSpace(asString(payload["password"])); password != "" {
		hash, err := store.HashPassword(password)
		if err != nil {
			writeError(w, http.StatusBadRequest, "update_user_failed", err.Error())
			return
		}
		user.PasswordHash = hash
	}
	if value, ok := payload["ip_whitelist"]; ok {
		user.IPWhitelist = asStringSlice(value)
	}
	if value, ok := payload["metadata"].(map[string]any); ok {
		user.Metadata = value
	}
	if value, ok := payload["rate_limits"].(map[string]any); ok {
		user.RateLimits = value
	}

	if err := st.Users().Update(user); err != nil {
		if errors.Is(err, store.ErrInsufficientCredits) {
			writeError(w, http.StatusPaymentRequired, "insufficient_credits", err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_user_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, sanitizedUser(user))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Users().Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "delete_user_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) suspendUser(w http.ResponseWriter, r *http.Request) {
	s.updateUserStatus(w, r, "suspended")
}

func (s *Server) activateUser(w http.ResponseWriter, r *http.Request) {
	s.updateUserStatus(w, r, "active")
}

// updateUserStatusUnified handles PUT /users/{id}/status — reads status from request body.
func (s *Server) updateUserStatusUnified(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	status := strings.TrimSpace(asString(payload["status"]))
	if status == "" {
		writeError(w, http.StatusBadRequest, "invalid_status", "status is required")
		return
	}
	switch strings.ToLower(status) {
	case "active", "suspended", "inactive":
	default:
		writeError(w, http.StatusBadRequest, "invalid_status", "status must be one of: active, suspended, inactive")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_user_status_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	if err := st.Users().UpdateStatus(id, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_user_status_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status})
}

func (s *Server) updateUserStatus(w http.ResponseWriter, r *http.Request, status string) {
	id := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Users().UpdateStatus(id, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_user_status_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status})
}

func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	password := strings.TrimSpace(asString(payload["password"]))
	if password == "" {
		writeError(w, http.StatusBadRequest, "invalid_password", "password is required")
		return
	}
	if len(password) < 8 {
		writeError(w, http.StatusBadRequest, "invalid_password", "password must be at least 8 characters")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reset_password_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	hash, err := store.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reset_password_failed", err.Error())
		return
	}
	user.PasswordHash = hash
	if err := st.Users().Update(user); err != nil {
		writeError(w, http.StatusBadRequest, "reset_password_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "password_reset": true})
}

func (s *Server) listUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	keys, err := st.APIKeys().ListByUser(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_api_keys_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, keys)
}

func (s *Server) createUserAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	name := strings.TrimSpace(asString(payload["name"]))
	mode := strings.TrimSpace(asString(payload["mode"]))
	if mode == "" {
		mode = "live"
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	fullKey, key, err := st.APIKeys().Create(userID, name, mode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "create_api_key_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"full_key": fullKey,
		"key":      key,
	})
}

func (s *Server) revokeUserAPIKey(w http.ResponseWriter, r *http.Request) {
	keyID := strings.TrimSpace(r.PathValue("keyId"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.APIKeys().Revoke(keyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "api_key_not_found", "API key not found")
			return
		}
		writeError(w, http.StatusBadRequest, "revoke_api_key_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listUserPermissions(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	permissions, err := st.Permissions().ListByUser(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_permissions_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, permissions)
}

func (s *Server) createUserPermission(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	permission, err := decodePermissionPayload(payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_permission", err.Error())
		return
	}
	permission.UserID = userID

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Permissions().Create(permission); err != nil {
		writeError(w, http.StatusBadRequest, "create_permission_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, permission)
}

func (s *Server) updateUserPermission(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	permissionID := strings.TrimSpace(r.PathValue("pid"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	permission, err := decodePermissionPayload(payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_permission", err.Error())
		return
	}
	permission.ID = permissionID
	permission.UserID = userID

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Permissions().Update(permission); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "permission_not_found", "Permission not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_permission_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, permission)
}

func (s *Server) deleteUserPermission(w http.ResponseWriter, r *http.Request) {
	permissionID := strings.TrimSpace(r.PathValue("pid"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	if err := st.Permissions().Delete(permissionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "permission_not_found", "Permission not found")
			return
		}
		writeError(w, http.StatusBadRequest, "delete_permission_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) bulkAssignUserPermissions(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	rawPermissions, ok := payload["permissions"].([]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_permissions", "permissions array is required")
		return
	}
	permissions := make([]store.EndpointPermission, 0, len(rawPermissions))
	for _, raw := range rawPermissions {
		item, ok := raw.(map[string]any)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_permissions", "permission item must be object")
			return
		}
		permission, err := decodePermissionPayload(item)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_permissions", err.Error())
			return
		}
		permissions = append(permissions, *permission)
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	if err := st.Permissions().BulkAssign(userID, permissions); err != nil {
		writeError(w, http.StatusBadRequest, "bulk_assign_permissions_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"assigned": len(permissions)})
}

func (s *Server) listUserIPWhitelist(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_ip_whitelist_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"ip_whitelist": user.IPWhitelist})
}

func (s *Server) addUserIPWhitelist(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	ips := asStringSlice(payload["ips"])
	if len(ips) == 0 {
		if value := strings.TrimSpace(asString(payload["ip"])); value != "" {
			ips = []string{value}
		}
	}
	if len(ips) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_ip", "ip or ips is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_ip_whitelist_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(user.IPWhitelist)+len(ips))
	for _, item := range user.IPWhitelist {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	for _, item := range ips {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	user.IPWhitelist = merged
	if err := st.Users().Update(user); err != nil {
		writeError(w, http.StatusBadRequest, "update_ip_whitelist_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"ip_whitelist": user.IPWhitelist})
}

func (s *Server) deleteUserIPWhitelist(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	ipValue := strings.TrimSpace(r.PathValue("ip"))
	if decoded, err := url.PathUnescape(ipValue); err == nil {
		ipValue = strings.TrimSpace(decoded)
	}
	if ipValue == "" {
		writeError(w, http.StatusBadRequest, "invalid_ip", "ip is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "delete_ip_whitelist_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	next := make([]string, 0, len(user.IPWhitelist))
	for _, item := range user.IPWhitelist {
		if strings.TrimSpace(item) == ipValue {
			continue
		}
		next = append(next, item)
	}
	user.IPWhitelist = next
	if err := st.Users().Update(user); err != nil {
		writeError(w, http.StatusBadRequest, "delete_ip_whitelist_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"ip_whitelist": user.IPWhitelist})
}

func decodePermissionPayload(payload map[string]any) (*store.EndpointPermission, error) {
	if payload == nil {
		return nil, errors.New("permission payload is required")
	}
	permission := &store.EndpointPermission{
		ID:           strings.TrimSpace(asString(payload["id"])),
		RouteID:      strings.TrimSpace(asString(payload["route_id"])),
		Methods:      asStringSlice(payload["methods"]),
		Allowed:      asBool(payload["allowed"], true),
		RateLimits:   asAnyMap(payload["rate_limits"]),
		AllowedDays:  asIntSlice(payload["allowed_days"]),
		AllowedHours: asStringSlice(payload["allowed_hours"]),
	}
	if permission.RouteID == "" {
		return nil, errors.New("route_id is required")
	}
	if value, ok := payload["credit_cost"]; ok {
		raw := strings.TrimSpace(asString(value))
		if raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, errors.New("credit_cost must be numeric")
			}
			permission.CreditCost = &parsed
		}
	}
	if value := strings.TrimSpace(asString(payload["valid_from"])); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, errors.New("valid_from must be RFC3339")
			}
		}
		permission.ValidFrom = &parsed
	}
	if value := strings.TrimSpace(asString(payload["valid_until"])); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, errors.New("valid_until must be RFC3339")
			}
		}
		permission.ValidUntil = &parsed
	}
	return permission, nil
}
