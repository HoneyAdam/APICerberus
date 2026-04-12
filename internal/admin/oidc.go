package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
	"github.com/APICerberus/APICerebrus/internal/store"
	"golang.org/x/oauth2"
)

// oidcState is an in-memory store for OIDC authorization flow state.
type oidcStateEntry struct {
	Provider   *oidc.Provider
	Verifier   *oidc.IDTokenVerifier
	Config     *oauth2.Config
	Expiry     time.Time
}

var (
	oidcProviderMu sync.RWMutex
	oidcProvider   *oidcStateEntry
)

// initOIDCProvider creates an OIDC provider and verifier from the current config.
// It must be called before handling any OIDC requests.
func (s *Server) initOIDCProvider() (*oidcStateEntry, error) {
	s.mu.RLock()
	cfg := s.cfg.Admin.OIDC
	s.mu.RUnlock()

	if !cfg.Enabled {
		return nil, fmt.Errorf("OIDC is not enabled")
	}

	// Check if we can reuse the cached provider
	oidcProviderMu.RLock()
	if oidcProvider != nil && time.Now().Before(oidcProvider.Expiry) {
		entry := oidcProvider
		oidcProviderMu.RUnlock()
		return entry, nil
	}
	oidcProviderMu.RUnlock()

	// Create new provider
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC provider discovery failed: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "email", "profile"}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	entry := &oidcStateEntry{
		Provider: provider,
		Verifier: verifier,
		Config:   oauth2Config,
		Expiry:   time.Now().Add(5 * time.Minute), // re-discover every 5 min
	}

	oidcProviderMu.Lock()
	oidcProvider = entry
	oidcProviderMu.Unlock()

	return entry, nil
}

// handleOIDCLogin initiates the OIDC authorization code flow.
// GET /admin/api/v1/auth/sso/login
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	entry, err := s.initOIDCProvider()
	if err != nil {
		writeError(w, http.StatusBadRequest, "oidc_not_configured", err.Error())
		return
	}

	// Generate state parameter to prevent CSRF
	state, err := generateRandomHex(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "state_generation_failed", err.Error())
		return
	}

	// Generate nonce to prevent replay attacks
	nonce, err := generateRandomHex(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "nonce_generation_failed", err.Error())
		return
	}

	// Store nonce in cookie for callback verification
	nonceCookie := &http.Cookie{
		Name:     "apicerberus_oidc_nonce",
		Value:    nonce,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(10 * time.Minute.Seconds()),
	}
	http.SetCookie(w, nonceCookie)

	// Store state in a cookie as well (OIDC state parameter)
	stateCookie := &http.Cookie{
		Name:     "apicerberus_oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(10 * time.Minute.Seconds()),
	}
	http.SetCookie(w, stateCookie)

	// Redirect to IdP authorization URL
	authURL := entry.Config.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback handles the OIDC callback (authorization code exchange).
// GET /admin/api/v1/auth/sso/callback
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	// Validate state
	stateCookie, err := r.Cookie("apicerberus_oidc_state")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_state", "Missing state cookie")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		writeError(w, http.StatusBadRequest, "missing_state", "No state parameter in callback")
		return
	}
	if !constantTimeEqual(state, stateCookie.Value) {
		writeError(w, http.StatusForbidden, "state_mismatch", "State parameter mismatch")
		return
	}

	// Check for IdP error
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		errDesc := r.URL.Query().Get("error_description")
		if errDesc == "" {
			errDesc = errCode
		}
		http.Redirect(w, r, "/dashboard?login=sso_error&error="+errCode, http.StatusSeeOther)
		return
	}

	// Validate nonce
	nonceCookie, err := r.Cookie("apicerberus_oidc_nonce")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_nonce", "Missing nonce cookie")
		return
	}

	entry, err := s.initOIDCProvider()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "oidc_error", err.Error())
		return
	}

	// Exchange authorization code for tokens
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	oauth2Token, err := entry.Config.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "token_exchange_failed", "Failed to exchange authorization code")
		return
	}

	// Extract and verify ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		writeError(w, http.StatusBadRequest, "no_id_token", "No ID token in response")
		return
	}

	idToken, err := entry.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_id_token", "Failed to verify ID token")
		return
	}

	// Verify nonce
	if idToken.Nonce != nonceCookie.Value {
		writeError(w, http.StatusUnauthorized, "nonce_mismatch", "Nonce mismatch in ID token")
		return
	}

	// Extract claims
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		writeError(w, http.StatusInternalServerError, "claim_extraction_failed", "Failed to parse ID token claims")
		return
	}

	email, _ := claims["email"].(string)
	if email == "" {
		writeError(w, http.StatusBadRequest, "missing_email", "Email claim not found in ID token")
		return
	}

	name := extractClaimName(claims)

	// Map role from IdP claims or use default
	role := mapOIDCRole(claims, s.cfg.Admin.OIDC)
	if role == "" {
		role = s.cfg.Admin.OIDC.DefaultRole
		if role == "" {
			role = string(RoleUser)
		}
	}

	// Auto-provision user if enabled and user doesn't exist
	if s.cfg.Admin.OIDC.AutoProvision {
		if err := s.ensureOIDCUser(email, name, role); err != nil {
			writeError(w, http.StatusInternalServerError, "user_provision_failed", err.Error())
			return
		}
	}

	// Verify user exists in the database
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	user, err := st.Users().FindByEmail(email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_lookup_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "user_not_found",
			"User not found. Please contact an administrator to create your account.")
		return
	}
	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "user_inactive",
			fmt.Sprintf("Account is %s. Please contact an administrator.", user.Status))
		return
	}

	// Use the user's actual role from the database (not from OIDC claims)
	role = user.Role
	if !slices.Contains(ValidRoles, role) {
		role = string(RoleUser)
	}
	perms := RolePermissions[UserRole(role)]

	// Issue local admin JWT with OIDC subject marker
	s.mu.RLock()
	tokenSecret := s.cfg.Admin.TokenSecret
	tokenTTL := s.cfg.Admin.TokenTTL
	s.mu.RUnlock()

	now := time.Now().UTC()
	payload := map[string]any{
		"sub":   "oidc:" + idToken.Subject,
		"email": email,
		"role":  role,
		"iat":   now.Unix(),
		"exp":   now.Add(tokenTTL).Unix(),
	}
	if len(perms) > 0 {
		payload["permissions"] = perms
	}

	token, err := issueAdminTokenWithPayload(tokenSecret, tokenTTL, payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed", err.Error())
		return
	}

	// Set HttpOnly cookie
	cookie := &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(tokenTTL.Seconds()),
	}
	http.SetCookie(w, cookie)

	// Clear OIDC cookies
	http.SetCookie(w, &http.Cookie{
		Name:   "apicerberus_oidc_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   "apicerberus_oidc_nonce",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// handleOIDCLogout redirects to the IdP's end session endpoint if available,
// otherwise performs local logout.
// POST /admin/api/v1/auth/sso/logout
func (s *Server) handleOIDCLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}

	s.mu.RLock()
	cfg := s.cfg.Admin
	s.mu.RUnlock()

	if !cfg.OIDC.Enabled {
		writeError(w, http.StatusBadRequest, "oidc_not_configured", "OIDC is not enabled")
		return
	}

	// Try RP-initiated logout via IdP end_session_endpoint
	redirectURL := "http://" + r.Host + "/dashboard?logout=1"

	entry, err := s.initOIDCProvider()
	if err == nil {
		// Try to get end_session_endpoint from discovery
		discoveryURL := strings.TrimSuffix(cfg.OIDC.IssuerURL, "/") + "/.well-known/openid-configuration"
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var disc struct {
				EndSessionEndpoint string `json:"end_session_endpoint"`
			}
			if json.NewDecoder(resp.Body).Decode(&disc) == nil && disc.EndSessionEndpoint != "" {
				// RP-initiated logout
				logoutURL := disc.EndSessionEndpoint +
					"?post_logout_redirect_uri=" + redirectURL +
					"&client_id=" + cfg.OIDC.ClientID
				http.Redirect(w, r, logoutURL, http.StatusFound)
				return
			}
		}
	}
	_ = entry

	// Fallback: local logout
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// handleOIDCStatus returns the current OIDC SSO configuration status.
// GET /admin/api/v1/auth/sso/status
func (s *Server) handleOIDCStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	cfg := s.cfg.Admin.OIDC
	s.mu.RUnlock()

	if !cfg.Enabled {
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
		})
		return
	}

	// Redact sensitive fields
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":        true,
		"issuer_url":     cfg.IssuerURL,
		"client_id":      cfg.ClientID,
		"redirect_url":   cfg.RedirectURL,
		"scopes":         cfg.Scopes,
		"auto_provision": cfg.AutoProvision,
		"default_role":   cfg.DefaultRole,
	})
}

// ensureOIDCUser creates a user in the database if one doesn't exist for the
// given email. This is the auto-provisioning step.
func (s *Server) ensureOIDCUser(email, name, role string) error {
	st, err := s.openStore()
	if err != nil {
		return err
	}
	existing, err := st.Users().FindByEmail(email)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // User already exists
	}

	// Generate random password (not used for OIDC login, but satisfies bcrypt requirement)
	randomPw, err := generateSecureRandomHex(32)
	if err != nil {
		return err
	}
	hashedPw, err := store.HashPassword(randomPw)
	if err != nil {
		return err
	}

	user := &store.User{
		Email:        email,
		Name:         name,
		PasswordHash: hashedPw,
		Role:         role,
		Status:       "active",
	}
	if err := st.Users().Create(user); err != nil {
		return fmt.Errorf("create OIDC user: %w", err)
	}
	return nil
}

// mapOIDCRole maps OIDC claims to an internal role based on claim_mapping config.
func mapOIDCRole(claims map[string]any, cfg config.OIDCConfig) string {
	// Check for groups claim and map to roles
	if groups, ok := claims["groups"].([]any); ok {
		groupNames := make([]string, len(groups))
		for i, g := range groups {
			groupNames[i], _ = g.(string)
		}
		// Check if any group matches known role patterns
		for _, g := range groupNames {
			switch strings.ToLower(g) {
			case "admin", "admins", "apicerberus-admin":
				return string(RoleAdmin)
			case "manager", "managers", "apicerberus-manager":
				return string(RoleManager)
			}
		}
	}

	// Check claim_mapping for explicit role assignment
	if cfg.ClaimMapping != nil {
		if roleClaim, ok := cfg.ClaimMapping["role"]; ok {
			if roleVal, ok := claims[roleClaim]; ok {
				if roleStr, ok := roleVal.(string); ok {
					if slices.Contains(ValidRoles, strings.ToLower(roleStr)) {
						return strings.ToLower(roleStr)
					}
				}
			}
		}
	}

	return ""
}

// extractClaimName extracts the user's name from OIDC claims.
func extractClaimName(claims map[string]any) string {
	if name, ok := claims["name"].(string); ok && name != "" {
		return name
	}
	if given, ok := claims["given_name"].(string); ok && given != "" {
		if family, ok := claims["family_name"].(string); ok {
			return given + " " + family
		}
		return given
	}
	if email, ok := claims["email"].(string); ok && email != "" {
		return email
	}
	return "SSO User"
}

// issueAdminTokenWithPayload issues a JWT with an arbitrary payload.
func issueAdminTokenWithPayload(secret string, ttl time.Duration, payload map[string]any) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("admin token secret is not configured")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if payload["iat"] == nil {
		payload["iat"] = time.Now().UTC().Unix()
	}
	if payload["exp"] == nil {
		payload["exp"] = time.Now().UTC().Add(ttl).Unix()
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
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
	return signingInput + "." + jwt.EncodeSegment(signature), nil
}

// Generate random hex string
func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateSecureRandomHex(n int) (string, error) {
	return generateRandomHex(n)
}

// constantTimeEqual compares two strings in constant time.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
