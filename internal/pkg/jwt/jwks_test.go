package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewJWKSClient(t *testing.T) {
	t.Parallel()

	c := NewJWKSClient("https://example.com/.well-known/jwks.json", 0)
	if c == nil {
		t.Fatal("NewJWKSClient returned nil")
	}
	if c.ttl != time.Hour {
		t.Errorf("expected default TTL of 1h, got %v", c.ttl)
	}

	c2 := NewJWKSClient("https://example.com/.well-known/jwks.json", 30*time.Minute)
	if c2.ttl != 30*time.Minute {
		t.Errorf("expected TTL of 30m, got %v", c2.ttl)
	}
}

func TestJWKSClient_NilReceiver(t *testing.T) {
	t.Parallel()

	var c *JWKSClient
	_, err := c.GetRSAKey(context.Background(), "kid1")
	if err == nil {
		t.Fatal("expected error for nil JWKSClient")
	}
	_, err = c.GetECDSAKey(context.Background(), "kid1")
	if err == nil {
		t.Fatal("expected error for nil JWKSClient")
	}
}

func TestJWKSClient_EmptyURL(t *testing.T) {
	t.Parallel()

	c := NewJWKSClient("", 0)
	c.now = func() time.Time { return time.Now() }
	_, err := c.GetRSAKey(context.Background(), "kid1")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestJWKSClient_GetRSAKey_FromHTTP(t *testing.T) {
	t.Parallel()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nBytes := privKey.N.Bytes()
		eBytes := big.NewInt(int64(privKey.E)).Bytes()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"test-rsa","n":"` + base64.RawURLEncoding.EncodeToString(nBytes) + `","e":"` + base64.RawURLEncoding.EncodeToString(eBytes) + `"}]}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	key, err := c.GetRSAKey(context.Background(), "test-rsa")
	if err != nil {
		t.Fatalf("GetRSAKey error: %v", err)
	}
	if key == nil {
		t.Fatal("GetRSAKey returned nil key")
	}
	if key.E != privKey.E {
		t.Errorf("expected E=%d, got %d", privKey.E, key.E)
	}
}

func TestJWKSClient_GetECDSAKey_FromHTTP(t *testing.T) {
	t.Parallel()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}

	xBytes := privKey.X.Bytes()
	yBytes := privKey.Y.Bytes()
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"EC","kid":"test-ec","crv":"P-256","x":"` + base64.RawURLEncoding.EncodeToString(xBytes) + `","y":"` + base64.RawURLEncoding.EncodeToString(yBytes) + `"}]}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	key, err := c.GetECDSAKey(context.Background(), "test-ec")
	if err != nil {
		t.Fatalf("GetECDSAKey error: %v", err)
	}
	if key == nil {
		t.Fatal("GetECDSAKey returned nil key")
	}
	if key.X.Cmp(privKey.X) != 0 || key.Y.Cmp(privKey.Y) != 0 {
		t.Error("ECDSA key mismatch")
	}
}

func TestJWKSClient_LookupRSA_NoKid_Fallback(t *testing.T) {
	t.Parallel()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	nBytes := privKey.N.Bytes()
	eBytes := big.NewInt(int64(privKey.E)).Bytes()
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","n":"` + base64.RawURLEncoding.EncodeToString(nBytes) + `","e":"` + base64.RawURLEncoding.EncodeToString(eBytes) + `"}]}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	// When there's only one key and no kid, it should still return it
	key, err := c.GetRSAKey(context.Background(), "")
	if err != nil {
		t.Fatalf("GetRSAKey with empty kid error: %v", err)
	}
	if key == nil {
		t.Fatal("GetRSAKey returned nil for single-key set")
	}
}

func TestJWKSClient_LookupECDSA_NoKid_Fallback(t *testing.T) {
	t.Parallel()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}

	xBytes := privKey.X.Bytes()
	yBytes := privKey.Y.Bytes()
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"EC","crv":"P-256","x":"` + base64.RawURLEncoding.EncodeToString(xBytes) + `","y":"` + base64.RawURLEncoding.EncodeToString(yBytes) + `"}]}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	key, err := c.GetECDSAKey(context.Background(), "")
	if err != nil {
		t.Fatalf("GetECDSAKey with empty kid error: %v", err)
	}
	if key == nil {
		t.Fatal("GetECDSAKey returned nil for single-key set")
	}
}

func TestJWKSClient_KeyNotFound(t *testing.T) {
	t.Parallel()

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	_, err := c.GetRSAKey(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for empty JWKS")
	}
}

func TestJWKSClient_CacheHit(t *testing.T) {
	t.Parallel()

	fetchCount := 0
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	nBytes := privKey.N.Bytes()
	eBytes := big.NewInt(int64(privKey.E)).Bytes()

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"cached-key","n":"` + base64.RawURLEncoding.EncodeToString(nBytes) + `","e":"` + base64.RawURLEncoding.EncodeToString(eBytes) + `"}]}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 1*time.Hour)
	c.now = func() time.Time { return time.Now() }

	// First call fetches
	_, err := c.GetRSAKey(context.Background(), "cached-key")
	if err != nil {
		t.Fatalf("first GetRSAKey error: %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("expected 1 fetch, got %d", fetchCount)
	}

	// Second call should use cache
	_, err = c.GetRSAKey(context.Background(), "cached-key")
	if err != nil {
		t.Fatalf("second GetRSAKey error: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch (cache hit), got %d", fetchCount)
	}
}

func TestJWKSClient_StaleCacheFallback(t *testing.T) {
	t.Parallel()

	fetchCount := 0
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	nBytes := privKey.N.Bytes()
	eBytes := big.NewInt(int64(privKey.E)).Bytes()

	// First server - returns valid key, then goes down
	firstURL := ""
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"fallback-key","n":"` + base64.RawURLEncoding.EncodeToString(nBytes) + `","e":"` + base64.RawURLEncoding.EncodeToString(eBytes) + `"}]}`))
	}))
	defer secondServer.Close()

	firstURL = secondServer.URL

	c := NewJWKSClient(firstURL, 1*time.Second)
	calledOnce := false
	c.now = func() time.Time {
		if calledOnce {
			return time.Now().Add(-2 * time.Hour) // make stale
		}
		calledOnce = true
		return time.Now()
	}

	// First fetch
	_, err := c.GetRSAKey(context.Background(), "fallback-key")
	if err != nil {
		t.Fatalf("first GetRSAKey error: %v", err)
	}

	// Stale cache, refresh should succeed again
	_, err = c.GetRSAKey(context.Background(), "fallback-key")
	if err != nil {
		t.Fatalf("stale cache GetRSAKey error: %v", err)
	}
}

func TestJWKSClient_HTTPError(t *testing.T) {
	t.Parallel()

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	_, err := c.GetRSAKey(context.Background(), "kid")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSignHS256_HappyPath(t *testing.T) {
	t.Parallel()

	signingInput := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0"
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}

	sig, err := SignHS256(signingInput, secret)
	if err != nil {
		t.Fatalf("SignHS256 error: %v", err)
	}
	if len(sig) == 0 {
		t.Fatal("SignHS256 returned empty signature")
	}

	if !VerifyHS256(signingInput, sig, secret) {
		t.Fatal("VerifyHS256 should return true for valid signature")
	}
}

func TestSignES256_HappyPath(t *testing.T) {
	t.Parallel()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}

	signingInput := "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0"

	sig, err := SignES256(signingInput, privKey)
	if err != nil {
		t.Fatalf("SignES256 error: %v", err)
	}
	if len(sig) == 0 {
		t.Fatal("SignES256 returned empty signature")
	}

	if !VerifyES256(signingInput, sig, &privKey.PublicKey) {
		t.Fatal("VerifyES256 should return true for valid signature")
	}
}

func TestSignES256_NilKey(t *testing.T) {
	t.Parallel()

	_, err := SignES256("test", nil)
	if err == nil {
		t.Fatal("expected error for nil private key")
	}
}

func TestVerifyRS256_NilKey(t *testing.T) {
	t.Parallel()

	if VerifyRS256("test", nil, nil) {
		t.Fatal("VerifyRS256 should return false for nil public key")
	}
}

func TestVerifyES256_NilKey(t *testing.T) {
	t.Parallel()

	if VerifyES256("test", nil, nil) {
		t.Fatal("VerifyES256 should return false for nil public key")
	}
}

func TestParseRSAPublicKeyFromJWK_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jwk     JWK
		wantErr bool
	}{
		{
			name:    "missing n",
			jwk:     JWK{Kty: "RSA", Kid: "test", E: "AQAB"},
			wantErr: true,
		},
		{
			name:    "missing e",
			jwk:     JWK{Kty: "RSA", Kid: "test", N: base64.RawURLEncoding.EncodeToString(make([]byte, 256))},
			wantErr: true,
		},
		{
			name:    "invalid base64 for n",
			jwk:     JWK{Kty: "RSA", Kid: "test", N: "!!!invalid!!!", E: "AQAB"},
			wantErr: true,
		},
		{
			name:    "invalid base64 for e",
			jwk:     JWK{Kty: "RSA", Kid: "test", N: base64.RawURLEncoding.EncodeToString(make([]byte, 256)), E: "!!!"},
			wantErr: true,
		},
		{
			name:    "empty n bytes",
			jwk:     JWK{Kty: "RSA", Kid: "test", N: "", E: "AQAB"},
			wantErr: true,
		},
		{
			name:    "invalid exponent (e=1)",
			jwk:     JWK{Kty: "RSA", Kid: "test", N: base64.RawURLEncoding.EncodeToString(make([]byte, 256)), E: "AQ"},
			wantErr: true,
		},
		{
			name: "unsupported kty",
			jwk: JWK{
				Kty: "oct",
				Kid: "test",
				N:   base64.RawURLEncoding.EncodeToString(make([]byte, 256)),
				E:   "AQAB",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseRSAPublicKeyFromJWK(tt.jwk)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRSAPublicKeyFromJWK error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestJWKSClient_RefreshWithInvalidJSON(t *testing.T) {
	t.Parallel()

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json}`))
	}))
	defer jwks.Close()

	c := NewJWKSClient(jwks.URL, 5*time.Minute)
	c.now = func() time.Time { return time.Now() }

	_, err := c.GetRSAKey(context.Background(), "kid")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
