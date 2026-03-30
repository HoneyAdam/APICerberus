package portal

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

type contextKey string

const (
	contextUserKey    contextKey = "portal_user"
	contextSessionKey contextKey = "portal_session"
)

// Server hosts user-scoped portal API endpoints.
type Server struct {
	mu sync.RWMutex

	cfg        *config.Config
	store      *store.Store
	mux        *http.ServeMux
	uiFS       fs.FS
	pathPrefix string
	apiPrefix  string
}

func NewServer(cfg *config.Config, st *store.Store) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	if st == nil {
		return nil, errors.New("store is nil")
	}
	if st.Users() == nil || st.Sessions() == nil {
		return nil, errors.New("store repositories are not initialized")
	}

	pathPrefix := normalizePortalPathPrefix(cfg.Portal.PathPrefix)
	s := &Server{
		cfg:        cfg,
		store:      st,
		mux:        http.NewServeMux(),
		pathPrefix: pathPrefix,
		apiPrefix:  pathPrefix + "/api/v1",
	}
	if s.apiPrefix == "/api/v1" && pathPrefix == "" {
		s.apiPrefix = "/api/v1"
	}

	portalFS, err := embeddedPortalFS()
	if err != nil {
		return nil, err
	}
	s.uiFS = portalFS
	s.registerRoutes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST "+s.apiPrefix+"/auth/login", s.login)
	s.mux.HandleFunc("POST "+s.apiPrefix+"/auth/logout", s.withSession(s.logout))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/auth/me", s.withSession(s.me))
	s.mux.HandleFunc("PUT "+s.apiPrefix+"/auth/password", s.withSession(s.changePassword))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/api-keys", s.withSession(s.listMyAPIKeys))
	s.mux.HandleFunc("POST "+s.apiPrefix+"/api-keys", s.withSession(s.createMyAPIKey))
	s.mux.HandleFunc("PUT "+s.apiPrefix+"/api-keys/{id}", s.withSession(s.renameMyAPIKey))
	s.mux.HandleFunc("DELETE "+s.apiPrefix+"/api-keys/{id}", s.withSession(s.revokeMyAPIKey))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/apis", s.withSession(s.listMyAPIs))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/apis/{routeId}", s.withSession(s.getMyAPIDetail))

	s.mux.HandleFunc("POST "+s.apiPrefix+"/playground/send", s.withSession(s.playgroundSend))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/playground/templates", s.withSession(s.listTemplates))
	s.mux.HandleFunc("POST "+s.apiPrefix+"/playground/templates", s.withSession(s.saveTemplate))
	s.mux.HandleFunc("DELETE "+s.apiPrefix+"/playground/templates/{id}", s.withSession(s.deleteTemplate))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/usage/overview", s.withSession(s.usageOverview))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/usage/timeseries", s.withSession(s.usageTimeSeries))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/usage/top-endpoints", s.withSession(s.usageTopEndpoints))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/usage/errors", s.withSession(s.usageErrors))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/logs", s.withSession(s.listMyLogs))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/logs/{id}", s.withSession(s.getMyLogDetail))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/logs/export", s.withSession(s.exportMyLogs))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/credits/balance", s.withSession(s.myBalance))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/credits/transactions", s.withSession(s.myTransactions))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/credits/forecast", s.withSession(s.myForecast))
	s.mux.HandleFunc("POST "+s.apiPrefix+"/credits/purchase", s.withSession(s.purchaseCredits))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/security/ip-whitelist", s.withSession(s.listMyIPs))
	s.mux.HandleFunc("POST "+s.apiPrefix+"/security/ip-whitelist", s.withSession(s.addMyIP))
	s.mux.HandleFunc("DELETE "+s.apiPrefix+"/security/ip-whitelist/{ip}", s.withSession(s.removeMyIP))
	s.mux.HandleFunc("GET "+s.apiPrefix+"/security/activity", s.withSession(s.myActivity))

	s.mux.HandleFunc("GET "+s.apiPrefix+"/settings/profile", s.withSession(s.getProfile))
	s.mux.HandleFunc("PUT "+s.apiPrefix+"/settings/profile", s.withSession(s.updateProfile))
	s.mux.HandleFunc("PUT "+s.apiPrefix+"/settings/notifications", s.withSession(s.updateNotifications))

	if s.uiFS != nil {
		s.mux.Handle("/", s.newPortalUIHandler())
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	email := strings.TrimSpace(strings.ToLower(in.Email))
	password := strings.TrimSpace(in.Password)
	if email == "" || password == "" {
		writeError(w, http.StatusBadRequest, "invalid_credentials", "email and password are required")
		return
	}

	users := s.store.Users()
	user, err := users.FindByEmail(email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_lookup_failed", "failed to lookup user")
		return
	}
	if user == nil || !store.VerifyPassword(user.PasswordHash, password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if !isUserActive(user) {
		writeError(w, http.StatusForbidden, "user_inactive", "user account is inactive")
		return
	}

	token, err := store.GenerateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_token_failed", "failed to create session token")
		return
	}
	now := time.Now().UTC()
	maxAge := s.sessionMaxAge()
	expiresAt := now.Add(maxAge)
	session := &store.Session{
		UserID:    user.ID,
		TokenHash: store.HashSessionToken(token),
		UserAgent: strings.TrimSpace(r.UserAgent()),
		ClientIP:  clientIP(r),
		ExpiresAt: expiresAt,
		LastSeen:  now,
	}
	if err := s.store.Sessions().Create(session); err != nil {
		writeError(w, http.StatusInternalServerError, "session_create_failed", "failed to create session")
		return
	}

	setSessionCookie(w, sessionCookieConfig{
		Name:     s.sessionCookieName(),
		Path:     s.sessionCookiePath(),
		Value:    token,
		Expires:  expiresAt,
		MaxAge:   maxAge,
		Secure:   s.sessionSecure(),
		HTTPOnly: true,
	})

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user": sanitizeUser(user),
		"session": map[string]any{
			"id":         session.ID,
			"expires_at": expiresAt,
		},
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if session != nil {
		_ = s.store.Sessions().DeleteByID(session.ID)
	} else {
		if cookie, err := r.Cookie(s.sessionCookieName()); err == nil {
			_ = s.store.Sessions().DeleteByTokenHash(store.HashSessionToken(cookie.Value))
		}
	}

	clearSessionCookie(w, sessionCookieConfig{
		Name:     s.sessionCookieName(),
		Path:     s.sessionCookiePath(),
		Secure:   s.sessionSecure(),
		HTTPOnly: true,
	})
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"logged_out": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user": sanitizeUser(user),
	})
}

func (s *Server) withSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.sessionCookieName())
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
			return
		}

		tokenHash := store.HashSessionToken(cookie.Value)
		session, err := s.store.Sessions().FindByTokenHash(tokenHash)
		if err != nil || session == nil {
			clearSessionCookie(w, sessionCookieConfig{
				Name:     s.sessionCookieName(),
				Path:     s.sessionCookiePath(),
				Secure:   s.sessionSecure(),
				HTTPOnly: true,
			})
			writeError(w, http.StatusUnauthorized, "session_expired", "session is invalid or expired")
			return
		}
		if session.ExpiresAt.Before(time.Now().UTC()) {
			_ = s.store.Sessions().DeleteByID(session.ID)
			clearSessionCookie(w, sessionCookieConfig{
				Name:     s.sessionCookieName(),
				Path:     s.sessionCookiePath(),
				Secure:   s.sessionSecure(),
				HTTPOnly: true,
			})
			writeError(w, http.StatusUnauthorized, "session_expired", "session is invalid or expired")
			return
		}

		user, err := s.store.Users().FindByID(session.UserID)
		if err != nil || user == nil || !isUserActive(user) {
			_ = s.store.Sessions().DeleteByID(session.ID)
			clearSessionCookie(w, sessionCookieConfig{
				Name:     s.sessionCookieName(),
				Path:     s.sessionCookiePath(),
				Secure:   s.sessionSecure(),
				HTTPOnly: true,
			})
			writeError(w, http.StatusUnauthorized, "session_expired", "session is invalid or expired")
			return
		}

		_ = s.store.Sessions().Touch(session.ID)
		ctx := context.WithValue(r.Context(), contextUserKey, user)
		ctx = context.WithValue(ctx, contextSessionKey, session)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) sessionCookieName() string {
	s.mu.RLock()
	name := strings.TrimSpace(s.cfg.Portal.Session.CookieName)
	s.mu.RUnlock()
	if name == "" {
		return "apicerberus_session"
	}
	return name
}

func (s *Server) sessionCookiePath() string {
	s.mu.RLock()
	pathPrefix := normalizePortalPathPrefix(s.cfg.Portal.PathPrefix)
	s.mu.RUnlock()
	if pathPrefix == "" {
		return "/"
	}
	return pathPrefix
}

func (s *Server) sessionMaxAge() time.Duration {
	s.mu.RLock()
	maxAge := s.cfg.Portal.Session.MaxAge
	s.mu.RUnlock()
	if maxAge <= 0 {
		return 24 * time.Hour
	}
	return maxAge
}

func (s *Server) sessionSecure() bool {
	s.mu.RLock()
	secure := s.cfg.Portal.Session.Secure
	s.mu.RUnlock()
	return secure
}

func userFromContext(ctx context.Context) *store.User {
	if ctx == nil {
		return nil
	}
	user, _ := ctx.Value(contextUserKey).(*store.User)
	return user
}

func sessionFromContext(ctx context.Context) *store.Session {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(contextSessionKey).(*store.Session)
	return session
}

func normalizePortalPathPrefix(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return ""
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return strings.TrimSuffix(raw, "/")
}

func isUserActive(user *store.User) bool {
	if user == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(user.Status))
	switch status {
	case "", "active":
		return true
	default:
		return false
	}
}

func sanitizeUser(user *store.User) map[string]any {
	if user == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"company":        user.Company,
		"role":           user.Role,
		"status":         user.Status,
		"credit_balance": user.CreditBalance,
		"rate_limits":    user.RateLimits,
		"ip_whitelist":   user.IPWhitelist,
		"metadata":       user.Metadata,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	}
}

type sessionCookieConfig struct {
	Name     string
	Path     string
	Value    string
	Expires  time.Time
	MaxAge   time.Duration
	Secure   bool
	HTTPOnly bool
}

func setSessionCookie(w http.ResponseWriter, cfg sessionCookieConfig) {
	maxAgeSeconds := int(cfg.MaxAge / time.Second)
	if maxAgeSeconds < 0 {
		maxAgeSeconds = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    cfg.Value,
		Path:     cfg.Path,
		Expires:  cfg.Expires.UTC(),
		MaxAge:   maxAgeSeconds,
		HttpOnly: cfg.HTTPOnly,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, cfg sessionCookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    "",
		Path:     cfg.Path,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		HttpOnly: cfg.HTTPOnly,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			if first := strings.TrimSpace(parts[0]); first != "" {
				return first
			}
		}
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote == "" {
		return ""
	}
	lastColon := strings.LastIndex(remote, ":")
	if lastColon <= 0 {
		return remote
	}
	return strings.TrimSpace(remote[:lastColon])
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	_ = jsonutil.WriteJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
