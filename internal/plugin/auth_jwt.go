package plugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
)

// JWTAuthError represents JWT authentication failure.
type JWTAuthError struct {
	PluginError
}

var (
	ErrMissingJWT = &JWTAuthError{
		PluginError: PluginError{
			Code:    "missing_jwt",
			Message: "JWT is required",
			Status:  http.StatusUnauthorized,
		},
	}
	ErrInvalidJWT = &JWTAuthError{
		PluginError: PluginError{
			Code:    "invalid_jwt",
			Message: "JWT is invalid",
			Status:  http.StatusUnauthorized,
		},
	}
	ErrInvalidJWTSignature = &JWTAuthError{
		PluginError: PluginError{
			Code:    "invalid_jwt_signature",
			Message: "JWT signature is invalid",
			Status:  http.StatusUnauthorized,
		},
	}
	ErrExpiredJWT = &JWTAuthError{
		PluginError: PluginError{
			Code:    "expired_jwt",
			Message: "JWT is expired",
			Status:  http.StatusUnauthorized,
		},
	}
	ErrInvalidJWTClaims = &JWTAuthError{
		PluginError: PluginError{
			Code:    "invalid_jwt_claims",
			Message: "JWT claims are invalid",
			Status:  http.StatusUnauthorized,
		},
	}
	ErrUnsupportedJWTAlgorithm = &JWTAuthError{
		PluginError: PluginError{
			Code:    "unsupported_jwt_algorithm",
			Message: "JWT algorithm is not supported",
			Status:  http.StatusUnauthorized,
		},
	}
)

// AuthJWTOptions configures AuthJWT plugin.
type AuthJWTOptions struct {
	Secret          string
	PublicKey       *rsa.PublicKey
	JWKSURL         string
	JWKSTTL         time.Duration
	Issuer          string
	Audience        []string
	RequiredClaims  []string
	ClaimsToHeaders map[string]string
	ClockSkew       time.Duration
	ECDSAPublicKey  *ecdsa.PublicKey
	EdDSAPublicKey  ed25519.PublicKey
	JTIReplayCache  *JTIReplayCache
}

// AuthJWT authenticates bearer JWT tokens.
type AuthJWT struct {
	secret          []byte
	publicKey       *rsa.PublicKey
	jwksClient      *jwt.JWKSClient
	issuer          string
	audience        map[string]struct{}
	requiredClaims  []string
	claimsToHeaders map[string]string
	clockSkew       time.Duration
	now             func() time.Time
	ecdsaPublicKey  *ecdsa.PublicKey
	ed25519Key      ed25519.PublicKey
	jtiReplayCache  *JTIReplayCache
}

func NewAuthJWT(opts AuthJWTOptions) *AuthJWT {
	clockSkew := opts.ClockSkew
	if clockSkew < 0 {
		clockSkew = 0
	}
	if clockSkew == 0 {
		clockSkew = 30 * time.Second
	}

	var jwksClient *jwt.JWKSClient
	if strings.TrimSpace(opts.JWKSURL) != "" {
		jwksClient = jwt.NewJWKSClient(opts.JWKSURL, opts.JWKSTTL)
	}

	aud := make(map[string]struct{}, len(opts.Audience))
	for _, value := range opts.Audience {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		aud[value] = struct{}{}
	}

	requiredClaims := make([]string, 0, len(opts.RequiredClaims))
	for _, claim := range opts.RequiredClaims {
		claim = strings.TrimSpace(claim)
		if claim == "" {
			continue
		}
		requiredClaims = append(requiredClaims, claim)
	}

	headers := make(map[string]string, len(opts.ClaimsToHeaders))
	for claim, header := range opts.ClaimsToHeaders {
		claim = strings.TrimSpace(claim)
		header = strings.TrimSpace(header)
		if claim == "" || header == "" {
			continue
		}
		headers[claim] = header
	}

	return &AuthJWT{
		secret:          []byte(opts.Secret),
		publicKey:       opts.PublicKey,
		jwksClient:      jwksClient,
		issuer:          strings.TrimSpace(opts.Issuer),
		audience:        aud,
		requiredClaims:  requiredClaims,
		claimsToHeaders: headers,
		clockSkew:       clockSkew,
		now:             time.Now,
		ecdsaPublicKey:  opts.ECDSAPublicKey,
		ed25519Key:      opts.EdDSAPublicKey,
		jtiReplayCache:  opts.JTIReplayCache,
	}
}

func (a *AuthJWT) Name() string  { return "auth-jwt" }
func (a *AuthJWT) Phase() Phase  { return PhaseAuth }
func (a *AuthJWT) Priority() int { return 20 }

// Authenticate validates bearer JWT and injects mapped claims to request headers.
func (a *AuthJWT) Authenticate(req *http.Request) (map[string]any, error) {
	tokenRaw := strings.TrimSpace(extractBearerToken(req))
	if tokenRaw == "" {
		return nil, ErrMissingJWT
	}

	parsed, err := jwt.Parse(tokenRaw)
	if err != nil {
		return nil, ErrInvalidJWT
	}

	alg, ok := parsed.HeaderString("alg")
	if !ok {
		return nil, ErrUnsupportedJWTAlgorithm
	}
	alg = strings.ToUpper(alg)
	// Explicitly reject "none" algorithm to prevent signature bypass attacks
	if alg == "NONE" {
		return nil, ErrUnsupportedJWTAlgorithm
	}
	switch alg {
	case "HS256":
		if !jwt.VerifyHS256(parsed.SigningInput, parsed.Signature, a.secret) {
			return nil, ErrInvalidJWTSignature
		}
	case "RS256":
		pub, err := a.resolveRSAPublicKey(req.Context(), parsed)
		if err != nil {
			return nil, err
		}
		if !jwt.VerifyRS256(parsed.SigningInput, parsed.Signature, pub) {
			return nil, ErrInvalidJWTSignature
		}
	case "ES256":
		pub, err := a.resolveECDSAPublicKey(req.Context(), parsed)
		if err != nil {
			return nil, err
		}
		if !jwt.VerifyES256(parsed.SigningInput, parsed.Signature, pub) {
			return nil, ErrInvalidJWTSignature
		}
	case "EDDSA":
		if a.ed25519Key == nil {
			return nil, ErrUnsupportedJWTAlgorithm
		}
		if !jwt.VerifyEdDSA(parsed.SigningInput, parsed.Signature, a.ed25519Key) {
			return nil, ErrInvalidJWTSignature
		}
	default:
		return nil, ErrUnsupportedJWTAlgorithm
	}

	if err := a.validateClaims(parsed); err != nil {
		return nil, err
	}

	// Check jti replay cache if enabled.
	if err := a.checkJTIReplay(parsed); err != nil {
		return nil, err
	}

	a.applyClaimHeaders(req, parsed.Payload)
	return parsed.Payload, nil
}

func (a *AuthJWT) resolveRSAPublicKey(ctx context.Context, token *jwt.Token) (*rsa.PublicKey, error) {
	if a.publicKey != nil {
		return a.publicKey, nil
	}
	if a.jwksClient == nil {
		return nil, ErrInvalidJWTSignature
	}

	kid, _ := token.HeaderString("kid")
	pub, err := a.jwksClient.GetRSAKey(ctx, kid)
	if err != nil {
		return nil, ErrInvalidJWTSignature
	}
	return pub, nil
}

func (a *AuthJWT) resolveECDSAPublicKey(ctx context.Context, token *jwt.Token) (*ecdsa.PublicKey, error) {
	if a.ecdsaPublicKey != nil {
		return a.ecdsaPublicKey, nil
	}
	if a.jwksClient == nil {
		return nil, ErrInvalidJWTSignature
	}

	kid, _ := token.HeaderString("kid")
	pub, err := a.jwksClient.GetECDSAKey(ctx, kid)
	if err != nil {
		return nil, ErrInvalidJWTSignature
	}
	return pub, nil
}

// checkJTIReplay checks for JWT ID replay attacks when a replay cache is configured.
func (a *AuthJWT) checkJTIReplay(token *jwt.Token) error {
	if a.jtiReplayCache == nil {
		// M-002: WARNING - JTI replay protection is disabled because no replay cache is configured.
		// A JWT with a valid signature but replayed jti will be accepted without error.
		// This is a security risk in production. Configure jtiReplayCache with a Redis-backed store.
		fmt.Printf("WARN: JTI replay cache not configured, replay protection disabled for token\n")
		return nil
	}
	jti, ok := token.ClaimString("jti")
	if !ok {
		return nil // jti is optional — if not present, skip replay check
	}
	if a.jtiReplayCache.Seen(jti) {
		return &JWTAuthError{
			PluginError: PluginError{
				Code:    "jti_replay",
				Message: "jwt has already been used (possible replay)",
				Status:  http.StatusUnauthorized,
			},
		}
	}
	// Register the jti with the token's remaining TTL.
	expUnix, ok := token.ClaimUnix("exp")
	if !ok {
		return nil
	}
	ttl := time.Until(time.Unix(expUnix, 0))
	if ttl <= 0 {
		// Fallback: if exp is in the past from wall-clock perspective
		// (e.g., tests with fake time), use a conservative default.
		ttl = 5 * time.Minute
	}
	a.jtiReplayCache.Add(jti, ttl)
	return nil
}

func (a *AuthJWT) validateClaims(token *jwt.Token) error {
	expUnix, ok := token.ClaimUnix("exp")
	if !ok {
		return &JWTAuthError{
			PluginError: PluginError{
				Code:    ErrInvalidJWTClaims.Code,
				Message: "exp claim is required",
				Status:  ErrInvalidJWTClaims.Status,
			},
		}
	}

	now := a.now()
	exp := time.Unix(expUnix, 0)
	if now.After(exp.Add(a.clockSkew)) {
		return ErrExpiredJWT
	}

	// Validate iat (Issued At) claim if present.
	// Reject tokens with iat in the future (beyond acceptable clock skew).
	// This prevents tickets with future timestamps.
	if iatUnix, ok := token.ClaimUnix("iat"); ok {
		iat := time.Unix(iatUnix, 0)
		if now.Before(iat.Add(-a.clockSkew)) {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: "jwt iat claim is in the future",
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}
	}

	// Validate nbf (Not Before) claim if present.
	if nbfUnix, ok := token.ClaimUnix("nbf"); ok {
		nbf := time.Unix(nbfUnix, 0)
		if now.Before(nbf.Add(-a.clockSkew)) {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: "jwt is not yet valid (nbf)",
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}
	}

	if a.issuer != "" {
		iss, ok := token.ClaimString("iss")
		if !ok || iss != a.issuer {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: "issuer claim is invalid",
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}
	}

	if len(a.audience) > 0 {
		values, ok := token.ClaimStrings("aud")
		if !ok {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: "audience claim is missing",
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}

		matched := false
		for _, value := range values {
			if _, exists := a.audience[value]; exists {
				matched = true
				break
			}
		}
		if !matched {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: "audience claim is invalid",
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}
	}

	for _, claim := range a.requiredClaims {
		raw, exists := token.Payload[claim]
		if !exists {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: fmt.Sprintf("%s claim is required", claim),
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}
		if !hasClaimValue(raw) {
			return &JWTAuthError{
				PluginError: PluginError{
					Code:    ErrInvalidJWTClaims.Code,
					Message: fmt.Sprintf("%s claim is empty", claim),
					Status:  ErrInvalidJWTClaims.Status,
				},
			}
		}
	}
	return nil
}

func (a *AuthJWT) applyClaimHeaders(req *http.Request, claims map[string]any) {
	if req == nil || len(a.claimsToHeaders) == 0 {
		return
	}
	for claimName, headerName := range a.claimsToHeaders {
		raw, ok := claims[claimName]
		if !ok {
			continue
		}
		value, ok := claimValueToHeader(raw)
		if !ok {
			continue
		}
		req.Header.Set(headerName, value)
	}
}

func hasClaimValue(raw any) bool {
	switch v := raw.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	default:
		return true
	}
}

func claimValueToHeader(raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return "", false
		}
		return v, true
	case float64:
		return strconv.FormatInt(int64(v), 10), true
	case float32:
		return strconv.FormatInt(int64(v), 10), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue
			}
			out = append(out, fmt.Sprint(item))
		}
		if len(out) == 0 {
			return "", false
		}
		return strings.Join(out, ","), true
	default:
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			return "", false
		}
		return value, true
	}
}

func extractBearerToken(req *http.Request) string {
	if req == nil {
		return ""
	}
	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if len(auth) <= 7 {
		return ""
	}
	if !strings.EqualFold(auth[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[7:])
}
