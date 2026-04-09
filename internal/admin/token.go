package admin

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
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

// issueAdminToken generates a scoped HS256 admin JWT.
func issueAdminToken(secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("admin token secret is not configured")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	now := time.Now().UTC()
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	payload := map[string]any{
		"sub": "admin",
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
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
	signature := jwt.SignHS256(signingInput, []byte(secret))
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

// withAdminBearerAuth restricts endpoints to valid Bearer tokens only.
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
		next(w, r)
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
func (s *Server) handleTokenIssue(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg.Admin
	gwHTTPS := s.cfg.Gateway.HTTPSAddr != ""
	s.mu.RUnlock()

	token, err := issueAdminToken(cfg.TokenSecret, cfg.TokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed", err.Error())
		return
	}

	// Set HttpOnly cookie for XSS-safe authentication transport.
	cookie := &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(cfg.TokenTTL.Seconds()),
	}
	if gwHTTPS || r.URL.Scheme == "https" {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"token_type": "Bearer",
		"expires_in": int(cfg.TokenTTL.Seconds()),
	})
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
