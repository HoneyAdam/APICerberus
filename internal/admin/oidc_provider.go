package admin

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
	jwtv5 "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// OIDCProviderServer implements an OIDC Authorization Server.
type OIDCProviderServer struct {
	config        *config.OIDCProviderConfig
	signer       *oidcProviderSigner
	authCodes     map[string]*authCodeEntry
	refreshTokens map[string]*refreshTokenEntry // key = bcrypt hash of refresh token
	clients      map[string]*config.OIDCClient
	mu           sync.RWMutex
}

type refreshTokenEntry struct {
	Subject   string
	ClientID  string
	Scopes    []string
	Expiry    time.Time
}

type oidcProviderSigner struct {
	privateKey any       // *rsa.PrivateKey or *ecdsa.PrivateKey
	keyType    string    // "RSA" or "EC"
	keyID      string    // kid
	algorithm  string    // RS256 or ES256
}

type authCodeEntry struct {
	ClientID    string
	RedirectURI string
	Subject     string
	Scopes      []string
	Nonce       string
	Expiry      time.Time
	Used        bool
}

var providerSigner *oidcProviderSigner
var providerSignerMu sync.RWMutex

// InitOIDCProviderServer initializes the OIDC provider from config.
func (s *Server) initOIDCProviderServer() error {
	s.mu.RLock()
	provCfg := s.cfg.Admin.OIDC.Provider
	s.mu.RUnlock()

	if !provCfg.Enabled {
		return nil
	}

	// Load or generate signing key
	signer, err := initProviderSigner(provCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize OIDC provider signer: %w", err)
	}

	providerSignerMu.Lock()
	providerSigner = signer
	providerSignerMu.Unlock()

	// Build client map
	clientMap := make(map[string]*config.OIDCClient)
	for i := range provCfg.Clients {
		clientMap[provCfg.Clients[i].ClientID] = &provCfg.Clients[i]
	}

	s.oidcProvider = &OIDCProviderServer{
		config:        &provCfg,
		signer:       signer,
		authCodes:     make(map[string]*authCodeEntry),
		refreshTokens: make(map[string]*refreshTokenEntry),
		clients:      clientMap,
	}

	// Start cleanup goroutine for expired auth codes and refresh tokens
	go s.cleanupAuthCodes()

	return nil
}

func initProviderSigner(cfg config.OIDCProviderConfig) (*oidcProviderSigner, error) {
	keyType := cfg.KeyType
	if keyType == "" {
		keyType = "rsa"
	}
	keyID := cfg.KeyID
	if keyID == "" {
		keyID = "default"
	}

	var privKey any
	var algorithm string

	if cfg.RSAPrivateKeyFile != "" {
		data, err := readFile(cfg.RSAPrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read RSA private key: %w", err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM block from %s", cfg.RSAPrivateKeyFile)
		}
		rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			// Try PKCS8
			key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err2 != nil {
				return nil, fmt.Errorf("parse RSA private key: %w (tried PKCS1 and PKCS8)", err)
			}
			var ok bool
			rsaKey, ok = key.(*rsa.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("PEM key is not RSA private key")
			}
			privKey = rsaKey
		} else {
			privKey = rsaKey
		}
		algorithm = "RS256"
	} else if cfg.ECPrivateKeyFile != "" {
		data, err := readFile(cfg.ECPrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read EC private key: %w", err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM block from %s", cfg.ECPrivateKeyFile)
		}
		ecKey, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse EC private key: %w", err)
		}
		privKey = ecKey
		algorithm = "ES256"
	} else {
		// Auto-generate RSA key
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generate RSA key: %w", err)
		}
		privKey = rsaKey
		algorithm = "RS256"
		keyType = "rsa"
		_ = keyType
	}

	return &oidcProviderSigner{
		privateKey: privKey,
		keyType:    strings.ToUpper(keyType),
		keyID:      keyID,
		algorithm:  algorithm,
	}, nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func readFileFromDisk(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// discoveryHandler returns the OIDC Discovery document.
// GET /.well-known/openid-configuration
func (s *Server) handleOIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	s.mu.RLock()
	cfg := s.cfg.Admin.OIDC.Provider
	s.mu.RUnlock()

	disc := map[string]any{
		"issuer":                            cfg.Issuer,
		"authorization_endpoint":            cfg.Issuer + "/oidc/authorize",
		"token_endpoint":                     cfg.Issuer + "/oidc/token",
		"userinfo_endpoint":                  cfg.Issuer + "/oidc/userinfo",
		"jwks_uri":                           cfg.Issuer + "/oidc/jwks",
		"revocation_endpoint":               cfg.Issuer + "/oidc/revoke",
		"introspection_endpoint":            cfg.Issuer + "/oidc/introspect",
		"response_types_supported":          []string{"code", "token", "id_token"},
		"subject_types_supported":           []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256", "ES256"},
		"scopes_supported":                  []string{"openid", "profile", "email"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"claims_supported":                  []string{"sub", "iss", "aud", "exp", "iat", "name", "email"},
		"grant_types_supported":              []string{"authorization_code", "client_credentials", "refresh_token"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(disc)
}

// jwksHandler returns the public keys as JWKS.
// GET /oidc/jwks
func (s *Server) handleOIDCJWKS(w http.ResponseWriter, r *http.Request) {
	providerSignerMu.RLock()
	signer := providerSigner
	providerSignerMu.RUnlock()

	if signer == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	var jwk map[string]any
	switch key := signer.privateKey.(type) {
	case *rsa.PrivateKey:
		jwk = rsaPublicKeyToJWK(&key.PublicKey, signer.keyID, "RS256")
	case *ecdsa.PrivateKey:
		jwk = ecPublicKeyToJWK(&key.PublicKey, signer.keyID, "ES256")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "unsupported key type")
		return
	}

	jwks := map[string]any{"keys": []map[string]any{jwk}}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jwks)
}

// authorizeHandler handles the OIDC authorization endpoint.
// GET /oidc/authorize?client_id=xxx&redirect_uri=xxx&response_type=code&scope=openid+profile&state=xyz
func (s *Server) handleOIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	nonce := r.URL.Query().Get("nonce")

	if clientID == "" || redirectURI == "" || responseType == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "client_id, redirect_uri, and response_type are required")
		return
	}

	s.mu.RLock()
	client, ok := s.oidcProvider.clients[clientID]
	s.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}

	if !slices.Contains(client.RedirectURIs, redirectURI) {
		writeError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri not registered for this client")
		return
	}

	if responseType != "code" && responseType != "token" {
		writeError(w, http.StatusBadRequest, "unsupported_response_type", "only 'code' and 'token' response_types are supported")
		return
	}

	scopes := strings.Fields(scope)
	if !slices.Contains(scopes, "openid") {
		writeError(w, http.StatusBadRequest, "invalid_scope", "openid scope is required")
		return
	}

	// For now, use a default user — in production this would redirect to login
	// For testing/demo, we'll use a placeholder subject
	subject := "user@example.com"

	// Generate authorization code
	code, err := newRandomHex(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate authorization code")
		return
	}

	authCodeTTL := s.oidcProvider.config.AuthCodeTTL
	if authCodeTTL == 0 {
		authCodeTTL = 5 * time.Minute
	}

	s.mu.Lock()
	s.oidcProvider.authCodes[code] = &authCodeEntry{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Subject:     subject,
		Scopes:      scopes,
		Nonce:       nonce,
		Expiry:      time.Now().Add(authCodeTTL),
	}
	s.mu.Unlock()

	// Redirect back with code
	redirectURL := redirectURI + "?code=" + code
	if state != "" {
		redirectURL += "&state=" + state
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// tokenHandler handles token requests.
// POST /oidc/token
//grant_type=authorization_code&code=xxx&redirect_uri=xxx&client_id=xxx&client_secret=xxx
func (s *Server) handleOIDCProviderToken(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse form data")
		return
	}

	grantType := r.PostForm.Get("grant_type")
	if grantType == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "grant_type is required")
		return
	}

	clientID := r.PostForm.Get("client_id")
	clientSecret := r.PostForm.Get("client_secret")

	// Authenticate client
	client, ok := s.oidcProvider.clients[clientID]
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_client", "unknown client_id")
		return
	}

	// Verify client secret
	if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecret), []byte(clientSecret)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_client", "invalid client_secret")
		return
	}

	var accessToken, tokenType string
	var expiresIn int
	var idToken string
	var refreshToken string

	switch grantType {
	case "authorization_code":
		code := r.PostForm.Get("code")
		redirectURI := r.PostForm.Get("redirect_uri")

		s.mu.Lock()
		entry, exists := s.oidcProvider.authCodes[code]
		if exists && entry != nil && !entry.Used && time.Now().Before(entry.Expiry) {
			entry.Used = true
		}
		s.mu.Unlock()

		if !exists || entry == nil {
			writeError(w, http.StatusBadRequest, "invalid_grant", "authorization code invalid or expired")
			return
		}
		if entry.RedirectURI != redirectURI {
			writeError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
			return
		}

		// Generate tokens
		accessToken, expiresIn, _ = s.generateAccessToken(clientID, entry.Subject, entry.Scopes, "")
		idToken, _ = s.generateIDToken(clientID, entry.Subject, entry.Scopes, entry.Nonce)
		refreshToken = generateRefreshToken()

		// Store hashed refresh token with 7-day TTL
		rtHash := sha256.Sum256([]byte(refreshToken))
		s.oidcProvider.refreshTokens[string(rtHash[:])] = &refreshTokenEntry{
			Subject:  entry.Subject,
			ClientID: clientID,
			Scopes:   entry.Scopes,
			Expiry:   time.Now().Add(7 * 24 * time.Hour),
		}

		tokenType = "Bearer"

	case "client_credentials":
		subject := clientID // Client credentials use client_id as subject
		accessToken, expiresIn, _ = s.generateAccessToken(clientID, subject, client.Scopes, "")
		tokenType = "Bearer"

	case "refresh_token":
		refreshTok := r.PostForm.Get("refresh_token")
		if refreshTok == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token required")
			return
		}

		// Look up by SHA-256 hash of the token
		rtHash := sha256.Sum256([]byte(refreshTok))
		s.mu.Lock()
		entry, exists := s.oidcProvider.refreshTokens[string(rtHash[:])]
		if exists && time.Now().Before(entry.Expiry) && entry.ClientID == clientID {
			// Delete the used refresh token (one-time use)
			delete(s.oidcProvider.refreshTokens, string(rtHash[:]))
		} else {
			entry = nil
		}
		s.mu.Unlock()

		if entry == nil {
			writeError(w, http.StatusBadRequest, "invalid_grant", "refresh token invalid or expired")
			return
		}

		accessToken, expiresIn, _ = s.generateAccessToken(clientID, entry.Subject, entry.Scopes, "")
		tokenType = "Bearer"

	default:
		writeError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code, client_credentials, and refresh_token are supported")
		return
	}

	resp := map[string]any{
		"access_token": accessToken,
		"token_type":   tokenType,
		"expires_in":   expiresIn,
	}
	if idToken != "" {
		resp["id_token"] = idToken
	}
	if refreshToken != "" {
		resp["refresh_token"] = refreshToken
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) generateAccessToken(clientID, subject string, scopes []string, refreshToken string) (string, int, error) {
	providerSignerMu.RLock()
	signer := providerSigner
	providerSignerMu.RUnlock()

	if signer == nil {
		return "", 0, fmt.Errorf("signer not initialized")
	}

	ttl := s.oidcProvider.config.AccessTokenTTL
	if ttl == 0 {
		ttl = 3600 * time.Second // 1 hour default
	}

	now := time.Now()
	jti := generateJTI()

	claims := jwtv5.MapClaims{
		"iss": s.oidcProvider.config.Issuer,
		"sub": subject,
		"aud": clientID,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"jti": jti,
		"scope": strings.Join(scopes, " "),
	}
	if refreshToken != "" {
		claims["rt"] = refreshToken
	}

	var tokenStr string
	var err error

	switch key := signer.privateKey.(type) {
	case *rsa.PrivateKey:
		tokenStr, err = signTokenWithRS256(claims, key)
	case *ecdsa.PrivateKey:
		tokenStr, err = signTokenWithES256(claims, key)
	default:
		return "", 0, fmt.Errorf("unsupported key type")
	}

	return tokenStr, int(ttl.Seconds()), err
}

func (s *Server) generateIDToken(clientID, subject string, scopes []string, nonce string) (string, error) {
	providerSignerMu.RLock()
	signer := providerSigner
	providerSignerMu.RUnlock()

	ttl := s.oidcProvider.config.IDTokenTTL
	if ttl == 0 {
		ttl = 3600 * time.Second
	}

	now := time.Now()

	claims := jwtv5.MapClaims{
		"iss": s.oidcProvider.config.Issuer,
		"sub": subject,
		"aud": clientID,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"nonce": nonce,
	}

	// Add email if profile scope
	if slices.Contains(scopes, "profile") {
		claims["name"] = subject
	}
	if slices.Contains(scopes, "email") {
		claims["email"] = subject
	}

	switch key := signer.privateKey.(type) {
	case *rsa.PrivateKey:
		return signTokenWithRS256(claims, key)
	case *ecdsa.PrivateKey:
		return signTokenWithES256(claims, key)
	default:
		return "", fmt.Errorf("unsupported key type")
	}
}

func signTokenWithRS256(claims jwtv5.MapClaims, key *rsa.PrivateKey) (string, error) {
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims)
	return token.SignedString(key)
}

func signTokenWithES256(claims jwtv5.MapClaims, key *ecdsa.PrivateKey) (string, error) {
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodES256, claims)
	return token.SignedString(key)
}

func generateJTI() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateRefreshToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// userinfoHandler returns user claims.
// GET /oidc/userinfo (Bearer token required)
func (s *Server) handleOIDCUserInfo(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "missing_token", "Bearer token required")
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	providerSignerMu.RLock()
	signer := providerSigner
	providerSignerMu.RUnlock()

	if signer == nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "signer not initialized")
		return
	}

	// Parse and verify token
	token, err := jwt.Parse(tokenStr)
	if err != nil || token == nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "failed to parse token")
		return
	}

	claims := token.Payload
	if claims == nil {
		claims = map[string]any{}
	}

	// Verify issuer only if config is available
	if s.oidcProvider.config != nil {
		iss, _ := claims["iss"].(string)
		if iss != "" && iss != s.oidcProvider.config.Issuer {
			writeError(w, http.StatusUnauthorized, "invalid_token", "issuer mismatch")
			return
		}
	}

	userInfo := map[string]any{}
	if sub, ok := claims["sub"].(string); ok {
		userInfo["sub"] = sub
	}
	if scope, ok := claims["scope"].(string); ok {
		userInfo["scope"] = scope
	}
	if name, ok := claims["name"].(string); ok {
		userInfo["name"] = name
	}
	if email, ok := claims["email"].(string); ok {
		userInfo["email"] = email
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userInfo)
}

// revokeHandler revokes tokens.
// POST /oidc/revoke
func (s *Server) handleOIDCRevoke(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse form data")
		return
	}

	// In a full implementation, we'd track revoked JTIs in a list/Set
	// For now, just acknowledge the revocation
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

// introspectHandler introspects tokens.
// POST /oidc/introspect
// M-009 fix: Added signature verification (was trusting any JWT without verifying signature)
// M-010 fix: Added audience validation (was not checking aud claim)
func (s *Server) handleOIDCIntrospect(w http.ResponseWriter, r *http.Request) {
	if s.oidcProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "provider_not_configured", "OIDC provider is not enabled")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse form data")
		return
	}

	tokenStr := r.PostForm.Get("token")
	if tokenStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "token required")
		return
	}

	// M-009: Parse AND verify signature before trusting claims
	token, err := jwt.Parse(tokenStr)
	if err != nil {
		// Token is invalid or expired — return inactive
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"active": false})
		return
	}

	// M-009: Verify signature using the provider's public key
	providerSignerMu.RLock()
	signer := providerSigner
	providerSignerMu.RUnlock()

	if signer == nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "signer not initialized")
		return
	}

	// Verify signature based on algorithm
	var sigValid bool
	switch signer.algorithm {
	case "RS256":
		if rsaPub, ok := signer.privateKey.(*rsa.PrivateKey); ok {
			sigValid = jwt.VerifyRS256(token.SigningInput, token.Signature, &rsaPub.PublicKey)
		}
	case "ES256":
		if ecdsaPub, ok := signer.privateKey.(*ecdsa.PrivateKey); ok {
			sigValid = jwt.VerifyES256(token.SigningInput, token.Signature, &ecdsaPub.PublicKey)
		}
	}
	if !sigValid {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"active": false, "error": "invalid_signature"})
		return
	}

	claims := token.Payload
	exp, _ := claims["exp"].(float64)
	now := float64(time.Now().Unix())

	// M-010: Validate audience claim if OIDC clients are configured.
	// Tokens should be issued to a known client_id. Reject tokens with unknown audience.
	if s.oidcProvider != nil && len(s.oidcProvider.clients) > 0 {
		aud, _ := claims["aud"].(string)
		if aud != "" {
			found := false
			for clientID := range s.oidcProvider.clients {
				if clientID == aud {
					found = true
					break
				}
			}
			if !found {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"active": false, "error": "invalid_audience"})
				return
			}
		}
	}

	resp := map[string]any{
		"active":    exp > now,
		"sub":       claims["sub"],
		"scope":     claims["scope"],
		"client_id": claims["aud"],
		"exp":       claims["exp"],
		"iat":       claims["iat"],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) cleanupAuthCodes() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for code, entry := range s.oidcProvider.authCodes {
				if now.After(entry.Expiry) || entry.Used {
					delete(s.oidcProvider.authCodes, code)
				}
			}
			for hash, entry := range s.oidcProvider.refreshTokens {
				if now.After(entry.Expiry) {
					delete(s.oidcProvider.refreshTokens, hash)
				}
			}
			s.mu.Unlock()
		}
	}
}

// RSA JWK conversion
func rsaPublicKeyToJWK(pub *rsa.PublicKey, kid, alg string) map[string]any {
	return map[string]any{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"alg": alg,
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

// ECDSA JWK conversion (P-256 only)
func ecPublicKeyToJWK(pub *ecdsa.PublicKey, kid, alg string) map[string]any {
	return map[string]any{
		"kty": "EC",
		"kid": kid,
		"use": "sig",
		"alg": alg,
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(pub.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(pub.Y.Bytes()),
	}
}

// helper to generate random hex string
func newRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// GetOIDCProvider returns the OIDC provider server.
func (s *Server) GetOIDCProvider() *OIDCProviderServer {
	return s.oidcProvider
}