package portal

import (
	"net/http"
	"net/url"
	"strings"

	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func (s *Server) listMyIPs(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id": user.ID,
		"ips":     append([]string(nil), user.IPWhitelist...),
	})
}

func (s *Server) addMyIP(w http.ResponseWriter, r *http.Request) {
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
	additions := coerce.AsStringSlice(payload["ips"])
	if len(additions) == 0 {
		if single := strings.TrimSpace(coerce.AsString(payload["ip"])); single != "" {
			additions = []string{single}
		}
	}
	if len(additions) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_ip", "at least one IP value is required")
		return
	}
	current := append([]string(nil), user.IPWhitelist...)
	seen := map[string]struct{}{}
	for _, item := range current {
		seen[item] = struct{}{}
	}
	for _, item := range additions {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		current = append(current, item)
	}
	user.IPWhitelist = current
	if err := s.store.Users().Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, "update_ip_whitelist_failed", "failed to update ip whitelist")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id": user.ID,
		"ips":     current,
	})
}

func (s *Server) removeMyIP(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	ip, err := url.PathUnescape(strings.TrimSpace(r.PathValue("ip")))
	if err != nil || strings.TrimSpace(ip) == "" {
		writeError(w, http.StatusBadRequest, "invalid_ip", "ip is required")
		return
	}
	updated := make([]string, 0, len(user.IPWhitelist))
	for _, item := range user.IPWhitelist {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(ip)) {
			continue
		}
		updated = append(updated, item)
	}
	user.IPWhitelist = updated
	if err := s.store.Users().Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, "update_ip_whitelist_failed", "failed to update ip whitelist")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id": user.ID,
		"ips":     updated,
	})
}

func (s *Server) myActivity(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	session := sessionFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	result, err := s.store.Audits().Search(store.AuditSearchFilters{
		UserID: user.ID,
		Limit:  50,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "activity_failed", "failed to fetch activity")
		return
	}
	events := make([]map[string]any, 0, len(result.Entries)+1)
	if session != nil {
		events = append(events, map[string]any{
			"type":       "session",
			"timestamp":  session.LastSeen,
			"message":    "Portal session active",
			"client_ip":  session.ClientIP,
			"user_agent": session.UserAgent,
		})
	}
	for _, entry := range result.Entries {
		events = append(events, map[string]any{
			"type":        "request",
			"timestamp":   entry.CreatedAt,
			"method":      entry.Method,
			"path":        entry.Path,
			"status_code": entry.StatusCode,
			"latency_ms":  entry.LatencyMS,
			"client_ip":   entry.ClientIP,
		})
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"items": events, "total": len(events)})
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"user": sanitizeUser(user)})
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
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
	if name := strings.TrimSpace(coerce.AsString(payload["name"])); name != "" {
		user.Name = name
	}
	if company := strings.TrimSpace(coerce.AsString(payload["company"])); company != "" {
		user.Company = company
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok && metadata != nil {
		user.Metadata = metadata
	}
	if err := s.store.Users().Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, "update_profile_failed", "failed to update profile")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"user": sanitizeUser(user)})
}

func (s *Server) updateNotifications(w http.ResponseWriter, r *http.Request) {
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
	notifications, ok := payload["notifications"]
	if !ok {
		notifications = payload
	}
	if user.Metadata == nil {
		user.Metadata = map[string]any{}
	}
	user.Metadata["notifications"] = notifications
	if err := s.store.Users().Update(user); err != nil {
		writeError(w, http.StatusInternalServerError, "update_notifications_failed", "failed to update notifications")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"updated":       true,
		"notifications": notifications,
	})
}
