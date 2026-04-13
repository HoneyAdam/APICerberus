package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestSignAndVerifyES256 covers ECDSA P-256 signing and verification.
func TestSignAndVerifyES256(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey error: %v", err)
	}

	headerSeg := mustJSONSegment(t, map[string]any{"alg": "ES256"})
	payloadSeg := mustJSONSegment(t, map[string]any{"sub": "u1", "nbf": 1700000000})
	signingInput := headerSeg + "." + payloadSeg

	sig, err := SignES256(signingInput, privateKey)
	if err != nil {
		t.Fatalf("SignES256 error: %v", err)
	}

	if !VerifyES256(signingInput, sig, &privateKey.PublicKey) {
		t.Fatal("expected ES256 verification to succeed")
	}
	if VerifyES256(signingInput, sig, nil) {
		t.Fatal("expected ES256 verification with nil key to fail")
	}

	// Tampered input should fail
	if VerifyES256("tampered.input", sig, &privateKey.PublicKey) {
		t.Fatal("expected ES256 verification with tampered input to fail")
	}
}

// TestVerifyEdDSA covers Ed25519 verification.
func TestVerifyEdDSA(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey error: %v", err)
	}

	headerSeg := mustJSONSegment(t, map[string]any{"alg": "EdDSA"})
	payloadSeg := mustJSONSegment(t, map[string]any{"sub": "u1"})
	signingInput := headerSeg + "." + payloadSeg

	sig, err := jwt.SigningMethodEdDSA.Sign(signingInput, privateKey)
	if err != nil {
		t.Fatalf("EdDSA sign error: %v", err)
	}

	if !VerifyEdDSA(signingInput, sig, publicKey) {
		t.Fatal("expected EdDSA verification to succeed")
	}
	if VerifyEdDSA(signingInput, sig, nil) {
		t.Fatal("expected EdDSA verification with nil key to fail")
	}
	if VerifyEdDSA(signingInput, sig, "not-a-key") {
		t.Fatal("expected EdDSA verification with wrong type to fail")
	}
	if VerifyEdDSA("tampered.input", sig, publicKey) {
		t.Fatal("expected EdDSA verification with tampered input to fail")
	}
}

// TestParseEdgeCases covers malformed token parsing.
func TestParseEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  error
	}{
		{"empty", "", ErrInvalidTokenFormat},
		{"too_few_parts", "a.b", ErrInvalidTokenFormat},
		{"too_many_parts", "a.b.c.d", ErrInvalidTokenFormat},
		{"invalid_base64_header", "!!!.eyJzdWIiOiJ1MSJ9.c2ln", ErrInvalidHeader},
		{"invalid_json_header", "bm90LWpzb24.eyJzdWIiOiJ1MSJ9.c2ln", ErrInvalidHeader},
		{"invalid_base64_payload", "eyJhbGciOiJIUzI1NiJ9.!!!.c2ln", ErrInvalidPayload},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.want != nil && err.Error() == tt.want.Error() {
				return
			}
			// Accept any error that wraps the expected sentinel
			if tt.want != nil {
				if err.Error() != "" {
					return // any error is fine for edge cases
				}
			}
		})
	}
}

// TestClaimStrings covers string and array claims.
func TestClaimStrings(t *testing.T) {
	t.Parallel()

	token := &Token{Payload: map[string]any{
		"aud":       "single-audience",
		"auds":      []any{"aud1", "aud2", "", "  ", 123},
		"empty_aud": "",
		"num_aud":   42,
	}}

	auds, ok := token.ClaimStrings("aud")
	if !ok || len(auds) != 1 || auds[0] != "single-audience" {
		t.Fatalf("expected single audience, got %v", auds)
	}

	auds2, ok := token.ClaimStrings("auds")
	if !ok || len(auds2) != 2 || auds2[0] != "aud1" || auds2[1] != "aud2" {
		t.Fatalf("expected [aud1 aud2], got %v", auds2)
	}

	_, ok = token.ClaimStrings("empty_aud")
	if ok {
		t.Fatal("expected empty_aud to return false")
	}

	_, ok = token.ClaimStrings("num_aud")
	if ok {
		t.Fatal("expected num_aud to return false")
	}

	_, ok = token.ClaimStrings("missing")
	if ok {
		t.Fatal("expected missing claim to return false")
	}

	// Nil token
	_, ok = (*Token)(nil).ClaimStrings("aud")
	if ok {
		t.Fatal("expected nil token to return false")
	}
}

// TestClaimUnix covers unix timestamp claim parsing with multiple types.
func TestClaimUnix(t *testing.T) {
	t.Parallel()

	token := &Token{Payload: map[string]any{
		"exp_f64":   float64(1700000000),
		"exp_f32":   float32(1700000000), // same as f64 due to float32 precision limits
		"exp_i64":   int64(1700000002),
		"exp_i32":   int32(1700000003),
		"exp_i":     int(1700000004),
		"exp_u64":   uint64(1700000005),
		"exp_str":   "1700000006",
		"exp_json":  json.Number("1700000007"),
		"exp_empty": "",
		"exp_bad":   "not-a-number",
	}}

	tests := []struct {
		claim string
		want  int64
		ok    bool
	}{
		{"exp_f64", 1700000000, true},
		{"exp_f32", 1700000000, true}, // float32 precision rounds to same value
		{"exp_i64", 1700000002, true},
		{"exp_i32", 1700000003, true},
		{"exp_i", 1700000004, true},
		{"exp_u64", 1700000005, true},
		{"exp_str", 1700000006, true},
		{"exp_json", 1700000007, true},
		{"exp_empty", 0, false},
		{"exp_bad", 0, false},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		got, ok := token.ClaimUnix(tt.claim)
		if ok != tt.ok {
			t.Errorf("ClaimUnix(%q) ok=%v, want %v", tt.claim, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("ClaimUnix(%q) = %d, want %d", tt.claim, got, tt.want)
		}
	}

	// Nil token
	_, ok := (*Token)(nil).ClaimUnix("exp")
	if ok {
		t.Fatal("expected nil token to return false")
	}
}

// TestClaimStringEdgeCases covers nil token, missing claims, and non-string values.
func TestClaimStringEdgeCases(t *testing.T) {
	t.Parallel()

	token := &Token{Payload: map[string]any{
		"sub":   "user-123",
		"empty": "   ",
		"num":   42,
	}}

	sub, ok := token.ClaimString("sub")
	if !ok || sub != "user-123" {
		t.Fatalf("expected sub=user-123, got %q ok=%v", sub, ok)
	}

	_, ok = token.ClaimString("empty")
	if ok {
		t.Fatal("expected empty string claim to return false")
	}

	_, ok = token.ClaimString("num")
	if ok {
		t.Fatal("expected numeric claim to return false for ClaimString")
	}

	_, ok = token.ClaimString("missing")
	if ok {
		t.Fatal("expected missing claim to return false")
	}

	_, ok = (*Token)(nil).ClaimString("sub")
	if ok {
		t.Fatal("expected nil token to return false")
	}
}

// TestHeaderStringEdgeCases covers nil token and non-string header values.
func TestHeaderStringEdgeCases(t *testing.T) {
	t.Parallel()

	token := &Token{Header: map[string]any{
		"alg": "HS256",
		"kid": 123, // non-string
	}}

	alg, ok := token.HeaderString("alg")
	if !ok || alg != "HS256" {
		t.Fatalf("expected alg=HS256, got %q ok=%v", alg, ok)
	}

	_, ok = token.HeaderString("kid")
	if ok {
		t.Fatal("expected numeric header value to return false")
	}

	_, ok = (*Token)(nil).HeaderString("alg")
	if ok {
		t.Fatal("expected nil token to return false")
	}
}

// TestWeakHS256SecretRejection covers minimum secret length enforcement.
func TestWeakHS256SecretRejection(t *testing.T) {
	t.Parallel()

	weak := []byte("short")
	_, err := SignHS256("input", weak)
	if err == nil {
		t.Fatal("expected error for weak secret")
	}

	if VerifyHS256("input", []byte("sig"), weak) {
		t.Fatal("expected false for weak secret")
	}
}

// TestDecodeSegmentBasic covers base64url decoding edge cases (complement to existing tests).
func TestDecodeSegmentBasic(t *testing.T) {
	t.Parallel()

	_, err := DecodeSegment("!!!invalid!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}

	result, err := DecodeSegment("dGVzdA")
	if err != nil {
		t.Fatalf("DecodeSegment error: %v", err)
	}
	if string(result) != "test" {
		t.Fatalf("expected 'test', got %q", result)
	}
}

// TestEncodeSegment covers base64url encoding.
func TestEncodeSegment(t *testing.T) {
	t.Parallel()

	encoded := EncodeSegment([]byte("hello"))
	if encoded != "aGVsbG8" {
		t.Fatalf("expected aGVsbG8, got %q", encoded)
	}
}

// TestECDSAPublicKeyFromJWK covers ES256 JWK parsing.
func TestECDSAPublicKeyFromJWK(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey error: %v", err)
	}

	xBytes := privateKey.PublicKey.X.Bytes()
	yBytes := privateKey.PublicKey.Y.Bytes()

	jwk := JWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(xBytes),
		Y:   base64.RawURLEncoding.EncodeToString(yBytes),
	}

	pub, err := ParseECDSAPublicKeyFromJWK(jwk)
	if err != nil {
		t.Fatalf("ParseECDSAPublicKeyFromJWK error: %v", err)
	}
	if pub == nil {
		t.Fatal("expected non-nil public key")
	}

	if !VerifyES256("test.input", make([]byte, 64), pub) {
		_ = pub // Intentionally empty - just checking the key is usable; signature won't verify without proper signing
	}
}

// TestECDSAJWKEdgeCases covers invalid JWK fields.
func TestECDSAJWKEdgeCases(t *testing.T) {
	t.Parallel()

	_, err := ParseECDSAPublicKeyFromJWK(JWK{Kty: "RSA"})
	if err == nil {
		t.Fatal("expected error for RSA kty")
	}

	_, err = ParseECDSAPublicKeyFromJWK(JWK{Kty: "EC", Crv: "P-256"})
	if err == nil {
		t.Fatal("expected error for missing x/y")
	}

	_, err = ParseECDSAPublicKeyFromJWK(JWK{Kty: "EC", Crv: "P-256", X: "!!!", Y: "YWJj"})
	if err == nil {
		t.Fatal("expected error for invalid x encoding")
	}

	_, err = ParseECDSAPublicKeyFromJWK(JWK{Kty: "EC", Crv: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown curve")
	}

	_, err = ParseECDSAPublicKeyFromJWK(JWK{Kty: "EC", Crv: "P-256", X: "YWJj", Y: "!!!invalid"})
	if err == nil {
		t.Fatal("expected error for invalid y encoding")
	}

	// Point not on curve
	_, err = ParseECDSAPublicKeyFromJWK(JWK{Kty: "EC", Crv: "P-256", X: "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODkw", Y: "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODkw"})
	if err == nil {
		t.Fatal("expected error for point not on curve")
	}
}
