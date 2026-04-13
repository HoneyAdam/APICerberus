package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
)

var (
	errAdminTokenExpired = errors.New("admin token expired")
	errAdminTokenInvalid = errors.New("admin token invalid")
)

const adminSessionCookieName = "apicerberus_admin_session"

// extractAdminTokenFromCookie reads the admin JWT from the HttpOnly session cookie.
func extractAdminTokenFromCookie(r *http.Request) string {
	if c, err := r.Cookie(adminSessionCookieName); err == nil && c != nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

// issueAdminToken generates a scoped HS256 admin JWT with optional role and permissions.
func issueAdminToken(secret string, ttl time.Duration, role string, permissions []string) (string, error) {
	if secret == "" {
		return "", errors.New("admin token secret is not configured")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	now := time.Now().UTC()
	// Generate a unique token ID for revocation/correlation (M1)
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		// Fall back to time-based JTI if crypto/rand is unavailable
		jtiBytes = []byte(fmt.Sprintf("%x-%x", now.UnixNano(), now.Unix()))
	}
	jti := fmt.Sprintf("%x", jtiBytes)
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	payload := map[string]any{
		"sub": "admin",
		"jti": jti,
		"iss": "apicerberus-admin",
		"aud": "apicerberus",
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	if role != "" {
		payload["role"] = role
	}
	if len(permissions) > 0 {
		payload["permissions"] = permissions
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	signingInput := jwt.EncodeSegment(headerBytes) + "." + jwt.EncodeSegment(payloadBytes)
	signature, err := jwt.SignHS256(signingInput, []byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	token := signingInput + "." + jwt.EncodeSegment(signature)
	return token, nil
}

// verifyAdminToken parses and validates an admin JWT.
func verifyAdminToken(tokenString, secret string) error {
	if secret == "" {
		return errors.New("admin token secret is not configured")
	}
	tok, err := jwt.Parse(tokenString)
	if err != nil {
		return errAdminTokenInvalid
	}
	alg, _ := tok.HeaderString("alg")
	if alg != "HS256" {
		return errAdminTokenInvalid
	}
	if !jwt.VerifyHS256(tok.SigningInput, tok.Signature, []byte(secret)) {
		return errAdminTokenInvalid
	}
	exp, ok := tok.ClaimUnix("exp")
	if !ok || time.Now().UTC().Unix() > exp {
		return errAdminTokenExpired
	}
	// Validate iat (issued-at) — reject tokens with future iat (clock skew tolerance: 60s)
	if iat, ok := tok.ClaimUnix("iat"); ok {
		now := time.Now().UTC().Unix()
		if iat > now+60 {
			return errors.New("admin token issued in the future")
		}
	}
	// Validate nbf (not-before) if present
	if nbf, ok := tok.ClaimUnix("nbf"); ok {
		if time.Now().UTC().Unix() < nbf {
			return errors.New("admin token not yet valid")
		}
	}
	return nil
}

// extractBearerToken extracts the token from an Authorization: Bearer <token> header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}

// withAdminBearerAuth restricts endpoints to valid Bearer tokens only,
// then chains to RBAC for permission checking.
func (s *Server) withAdminBearerAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractClientIP(r)

		s.mu.RLock()
		cfg := s.cfg.Admin
		s.mu.RUnlock()

		// IP allow-list check (enforced before auth)
		if !isAllowedIP(clientIP, cfg.AllowedIPs) {
			writeError(w, http.StatusForbidden, "ip_not_allowed", "Client IP is not in the admin allow-list")
			return
		}

		// Rate limiting check
		if s.isRateLimited(clientIP) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many failed authentication attempts. Please try again later.")
			return
		}

		token := extractBearerToken(r)
		if token == "" {
			token = extractAdminTokenFromCookie(r)
		}
		if token == "" {
			s.recordFailedAuth(clientIP)
			writeError(w, http.StatusUnauthorized, "admin_unauthorized", "Missing Bearer token")
			return
		}
		if err := verifyAdminToken(token, cfg.TokenSecret); err != nil {
			s.recordFailedAuth(clientIP)
			writeError(w, http.StatusUnauthorized, "admin_unauthorized", "Invalid or expired token")
			return
		}
		s.clearFailedAuth(clientIP)

		// Extract role and permissions from the verified JWT
		role, perms := extractRoleFromJWT(token)
		ctx := r.Context()
		if role != "" {
			ctx = context.WithValue(ctx, ctxUserRole, role)
			ctx = context.WithValue(ctx, ctxUserPerms, perms)
		}

		// Chain to RBAC middleware
		s.withRBAC(next)(w, r.WithContext(ctx))
	}
}

// withAdminStaticAuth restricts endpoints to the static API key (bootstrap only).
func (s *Server) withAdminStaticAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractClientIP(r)

		s.mu.RLock()
		cfg := s.cfg.Admin
		s.mu.RUnlock()

		// IP allow-list check (enforced before auth)
		if !isAllowedIP(clientIP, cfg.AllowedIPs) {
			writeError(w, http.StatusForbidden, "ip_not_allowed", "Client IP is not in the admin allow-list")
			return
		}

		// Rate limiting check
		if s.isRateLimited(clientIP) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many failed authentication attempts. Please try again later.")
			return
		}

		provided := r.Header.Get("X-Admin-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.APIKey)) != 1 {
			s.recordFailedAuth(clientIP)
			writeError(w, http.StatusUnauthorized, "admin_unauthorized", "Invalid admin key")
			return
		}
		s.clearFailedAuth(clientIP)
		next(w, r)
	}
}

// handleTokenIssue issues a new admin JWT when presented with the static key.
func (s *Server) handleTokenIssue(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	cfg := s.cfg.Admin
	s.mu.RUnlock()

	token, err := issueAdminToken(cfg.TokenSecret, cfg.TokenTTL, string(RoleAdmin), RolePermissions[RoleAdmin])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed", err.Error())
		return
	}

	// Set HttpOnly cookie for XSS-safe authentication transport.
	// Always set Secure flag to prevent token leakage over HTTP (CWE-614)
	cookie := &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(cfg.TokenTTL.Seconds()),
	}
	http.SetCookie(w, cookie)

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"token_type": "Bearer",
		"token":      token,
		"expires_in": int(cfg.TokenTTL.Seconds()),
		"message":    "Session cookie set successfully",
	})
}

// handleFormLogin accepts an admin key via HTML form POST, validates it against
// the static API key, and sets an HttpOnly session cookie. The key never
// enters JavaScript — it's submitted directly to the server.
func (s *Server) handleFormLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cfg := s.cfg.Admin
	s.mu.RUnlock()

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid form data")
		return
	}

	clientIP := extractClientIP(r)

	provided := r.FormValue("admin_key")
	if provided == "" {
		s.recordFailedAuth(clientIP)
		http.Redirect(w, r, "/dashboard?login=missing_key", http.StatusSeeOther)
		return
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.APIKey)) != 1 {
		s.recordFailedAuth(clientIP)
		http.Redirect(w, r, "/dashboard?login=invalid_key", http.StatusSeeOther)
		return
	}

	token, err := issueAdminToken(cfg.TokenSecret, cfg.TokenTTL, string(RoleAdmin), RolePermissions[RoleAdmin])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed", err.Error())
		return
	}

	cookie := &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cfg.TokenTTL.Seconds()),
	}
	http.SetCookie(w, cookie)

	http.Redirect(w, r, "/dashboard?login=success", http.StatusSeeOther)
}

// handleFormLogout clears the admin session cookie and redirects to login.
func (s *Server) handleFormLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/dashboard?logout=1", http.StatusSeeOther)
}

// handleTokenLogout clears the admin session cookie.
func (s *Server) handleTokenLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"logged_out": true})
}
