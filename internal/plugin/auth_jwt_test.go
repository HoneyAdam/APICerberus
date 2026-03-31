package plugin

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthJWTValidHS256(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	secret := []byte("top-secret")
	token := buildHS256JWT(t,
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{
			"sub":  "consumer-1",
			"iss":  "apicerberus",
			"aud":  "public-api",
			"role": "gold",
			"exp":  now.Add(5 * time.Minute).Unix(),
		},
		secret,
	)

	auth := NewAuthJWT(AuthJWTOptions{
		Secret:         string(secret),
		Issuer:         "apicerberus",
		Audience:       []string{"public-api"},
		RequiredClaims: []string{"sub", "role"},
		ClaimsToHeaders: map[string]string{
			"sub": "X-Consumer-ID",
		},
		ClockSkew: 10 * time.Second,
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	claims, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}
	if claims["sub"] != "consumer-1" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if req.Header.Get("X-Consumer-ID") != "consumer-1" {
		t.Fatalf("expected mapped header")
	}
}

func TestAuthJWTValidRS256FromJWKS(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": "kid-1",
					"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
				},
			},
		})
	}))
	defer server.Close()

	now := time.Unix(1_700_000_000, 0).UTC()
	token := buildRS256JWT(t,
		map[string]any{"alg": "RS256", "kid": "kid-1"},
		map[string]any{
			"sub": "consumer-rsa",
			"exp": now.Add(5 * time.Minute).Unix(),
		},
		privateKey,
	)

	auth := NewAuthJWT(AuthJWTOptions{
		JWKSURL: server.URL,
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	claims, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}
	if claims["sub"] != "consumer-rsa" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestAuthJWTExpired(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	secret := []byte("top-secret")
	token := buildHS256JWT(t,
		map[string]any{"alg": "HS256"},
		map[string]any{
			"sub": "consumer-1",
			"exp": now.Add(-time.Minute).Unix(),
		},
		secret,
	)

	auth := NewAuthJWT(AuthJWTOptions{
		Secret:    string(secret),
		ClockSkew: 0,
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(req)
	if err != ErrExpiredJWT {
		t.Fatalf("expected ErrExpiredJWT got %v", err)
	}
}

func TestAuthJWTWrongIssuer(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	secret := []byte("top-secret")
	token := buildHS256JWT(t,
		map[string]any{"alg": "HS256"},
		map[string]any{
			"sub": "consumer-1",
			"iss": "issuer-a",
			"exp": now.Add(5 * time.Minute).Unix(),
		},
		secret,
	)

	auth := NewAuthJWT(AuthJWTOptions{
		Secret: string(secret),
		Issuer: "issuer-b",
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(req)
	assertJWTErrorCode(t, err, "invalid_jwt_claims")
}

func TestAuthJWTWrongAudience(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	secret := []byte("top-secret")
	token := buildHS256JWT(t,
		map[string]any{"alg": "HS256"},
		map[string]any{
			"sub": "consumer-1",
			"aud": []string{"private-api"},
			"exp": now.Add(5 * time.Minute).Unix(),
		},
		secret,
	)

	auth := NewAuthJWT(AuthJWTOptions{
		Secret:   string(secret),
		Audience: []string{"public-api"},
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(req)
	assertJWTErrorCode(t, err, "invalid_jwt_claims")
}

func TestAuthJWTMissingRequiredClaim(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	secret := []byte("top-secret")
	token := buildHS256JWT(t,
		map[string]any{"alg": "HS256"},
		map[string]any{
			"sub": "consumer-1",
			"exp": now.Add(5 * time.Minute).Unix(),
		},
		secret,
	)

	auth := NewAuthJWT(AuthJWTOptions{
		Secret:         string(secret),
		RequiredClaims: []string{"sub", "tenant_id"},
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(req)
	assertJWTErrorCode(t, err, "invalid_jwt_claims")
}

func assertJWTErrorCode(t *testing.T, err error, expectedCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error")
	}
	authErr, ok := err.(*JWTAuthError)
	if !ok {
		t.Fatalf("expected *JWTAuthError got %T", err)
	}
	if authErr.Code != expectedCode {
		t.Fatalf("expected error code %q got %q", expectedCode, authErr.Code)
	}
}

func TestAuthJWTNoneAlgorithmRejected(t *testing.T) {
	t.Parallel()

	// Build a JWT with "alg": "none" - this should be explicitly rejected
	// even though it's technically unsupported, to prevent algorithm confusion attacks
	now := time.Unix(1_700_000_000, 0).UTC()
	header := map[string]any{"alg": "none", "typ": "JWT"}
	payload := map[string]any{
		"sub": "attacker",
		"exp": now.Add(5 * time.Minute).Unix(),
	}
	headerSeg := mustJSONSegment(t, header)
	payloadSeg := mustJSONSegment(t, payload)
	// For "none" algorithm, signature should be empty
	token := headerSeg + "." + payloadSeg + "."

	auth := NewAuthJWT(AuthJWTOptions{
		Secret: "any-secret",
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(req)
	assertJWTErrorCode(t, err, "unsupported_jwt_algorithm")
}

func TestAuthJWTNoneAlgorithmUpperCaseRejected(t *testing.T) {
	t.Parallel()

	// Test that "NONE" (uppercase) is also rejected
	now := time.Unix(1_700_000_000, 0).UTC()
	header := map[string]any{"alg": "NONE", "typ": "JWT"}
	payload := map[string]any{
		"sub": "attacker",
		"exp": now.Add(5 * time.Minute).Unix(),
	}
	headerSeg := mustJSONSegment(t, header)
	payloadSeg := mustJSONSegment(t, payload)
	token := headerSeg + "." + payloadSeg + "."

	auth := NewAuthJWT(AuthJWTOptions{
		Secret: "any-secret",
	})
	auth.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := auth.Authenticate(req)
	assertJWTErrorCode(t, err, "unsupported_jwt_algorithm")
}

func buildHS256JWT(t *testing.T, header, payload map[string]any, secret []byte) string {
	t.Helper()
	headerSeg := mustJSONSegment(t, header)
	payloadSeg := mustJSONSegment(t, payload)
	signingInput := headerSeg + "." + payloadSeg
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func buildRS256JWT(t *testing.T, header, payload map[string]any, privateKey *rsa.PrivateKey) string {
	t.Helper()
	headerSeg := mustJSONSegment(t, header)
	payloadSeg := mustJSONSegment(t, payload)
	signingInput := headerSeg + "." + payloadSeg
	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15 error: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func mustJSONSegment(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
