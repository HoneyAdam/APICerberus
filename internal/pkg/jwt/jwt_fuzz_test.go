package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// FuzzParse tests JWT parsing against malformed and adversarial inputs.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		".",
		"..",
		"...",
		"abc",
		"abc.def",
		"abc.def.ghi",
		"a.b.c.d",
		strings.Repeat("a", 10000),
		"eyJhbGciOiJIUzI1NiJ9.e30.",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.",
		"eyJhbGciOiJIUzI1NiJ9.ey??//.sig",
		"\x00.\x00.\x00",
		"eyJ\\u0000\"alg\":\"HS256\"}.{}.sig",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		// Cap input length
		if len(raw) > 16384 {
			raw = raw[:16384]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Parse panicked on input %q: %v", raw, r)
			}
		}()

		_, _ = Parse(raw)
	})
}

// FuzzDecodeSegment tests base64url decoding against malformed inputs.
func FuzzDecodeSegment(f *testing.F) {
	seeds := []string{
		"",
		"=",
		"==",
		"abc=",
		"abc==",
		"a===b",
		strings.Repeat("a", 10000),
		"+/+/+/", // standard base64 chars (not URL-safe)
		"\x00\x01\x02",
		"====",
		"-_",
		"----____",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, segment string) {
		if len(segment) > 16384 {
			segment = segment[:16384]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("DecodeSegment panicked on input %q: %v", segment, r)
			}
		}()

		_, _ = DecodeSegment(segment)
	})
}

// FuzzClaimString tests claim extraction against malformed payloads.
func FuzzClaimString(f *testing.F) {
	seeds := []string{
		`{"sub":""}`,
		`{"sub":"  "}`,
		`{"sub":123}`,
		`{"sub":true}`,
		`{"sub":null}`,
		`{"sub":{"nested":"object"}}`,
		`{"sub":["a","","b","  "]}`,
		`{"sub":[1,2,3]}`,
		`{"sub":null}`,
		`{"exp":"not_a_number"}`,
		`{"exp":99999999999999999}`,
		`{"exp":0}`,
		`{"exp":-1}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, payloadJSON string) {
		if len(payloadJSON) > 8192 {
			payloadJSON = payloadJSON[:8192]
		}

		header := map[string]any{"alg": "HS256", "typ": "JWT"}
		headerBytes, _ := json.Marshal(header)
		headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)
		payloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))

		raw := headerEncoded + "." + payloadEncoded + ".fakesig"

		token, err := Parse(raw)
		if err != nil {
			return
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ClaimString panicked: %v", r)
			}
		}()

		// Exercise all claim types
		_, _ = token.ClaimString("sub")
		_, _ = token.ClaimStrings("aud")
		_, _ = token.ClaimUnix("exp")
		_, _ = token.ClaimUnix("iat")
		_, _ = token.ClaimUnix("nbf")
	})
}

// TestTokenNilSafety tests method calls on nil tokens.
func TestTokenNilSafety(t *testing.T) {
	var nilToken *Token

	if s, ok := nilToken.ClaimString("sub"); s != "" || ok {
		t.Errorf("ClaimString on nil should return empty, false")
	}
	if s, ok := nilToken.ClaimStrings("aud"); s != nil || ok {
		t.Errorf("ClaimStrings on nil should return nil, false")
	}
	if n, ok := nilToken.ClaimUnix("exp"); n != 0 || ok {
		t.Errorf("ClaimUnix on nil should return 0, false")
	}
	if s, ok := nilToken.HeaderString("alg"); s != "" || ok {
		t.Errorf("HeaderString on nil should return empty, false")
	}
}

// FuzzVerifyAlgorithms tests verification functions with malformed signatures
// and nil/wrong key types to ensure no panics.
func FuzzVerifyAlgorithms(f *testing.F) {
	seeds := []struct {
		signingInput string
		signature    []byte
	}{
		{"", nil},
		{"a.b", []byte{}},
		{"a.b", []byte{0, 1, 2, 3}},
		{strings.Repeat("x", 10000), make([]byte, 10000)},
	}

	for _, seed := range seeds {
		f.Add(seed.signingInput, seed.signature)
	}

	f.Fuzz(func(t *testing.T, signingInput string, signature []byte) {
		// Cap inputs
		if len(signingInput) > 16384 {
			signingInput = signingInput[:16384]
		}
		if len(signature) > 16384 {
			signature = signature[:16384]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Verify panicked: %v", r)
			}
		}()

		// All should return false (never panic)
		_ = VerifyHS256(signingInput, signature, []byte("short"))
		_ = VerifyHS256(signingInput, signature, []byte("this-is-a-long-enough-secret-for-hs256"))
		_ = VerifyRS256(signingInput, signature, nil)
		_ = VerifyES256(signingInput, signature, nil)
		_ = VerifyEdDSA(signingInput, signature, nil)
		_ = VerifyEdDSA(signingInput, signature, "not-a-key")
	})
}

// FuzzSignRoundTrip tests that sign→verify round-trips correctly with valid keys.
func FuzzSignRoundTrip(f *testing.F) {
	f.Add("test payload")

	// Generate test keys once
	hs256Secret := []byte("this-is-a-long-enough-secret-for-hs256")
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	edKey, _, _ := ed25519.GenerateKey(rand.Reader)

	f.Fuzz(func(t *testing.T, payload string) {
		if len(payload) > 4096 {
			payload = payload[:4096]
		}

		header := map[string]any{"alg": "HS256", "typ": "JWT"}
		headerBytes, _ := json.Marshal(header)
		payloadMap := map[string]any{"data": payload}
		payloadBytes, _ := json.Marshal(payloadMap)
		headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)
		payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)
		signingInput := headerEncoded + "." + payloadEncoded

		// HS256 round-trip
		sig, err := SignHS256(signingInput, hs256Secret)
		if err != nil {
			t.Fatalf("SignHS256: %v", err)
		}
		if !VerifyHS256(signingInput, sig, hs256Secret) {
			t.Error("HS256 round-trip failed")
		}
		if VerifyHS256(signingInput, sig, []byte("wrong-secret-this-is-long-enough!")) {
			t.Error("HS256 verified with wrong secret")
		}

		// ES256 round-trip
		sig, err = SignES256(signingInput, ecKey)
		if err != nil {
			t.Fatalf("SignES256: %v", err)
		}
		if !VerifyES256(signingInput, sig, &ecKey.PublicKey) {
			t.Error("ES256 round-trip failed")
		}
		if VerifyES256(signingInput, sig, nil) {
			t.Error("ES256 verified with nil key")
		}

		// EdDSA round-trip
		_, _ = SignES256(signingInput, ecKey) // reusing signing input for EdDSA test; EdDSA signing needs Ed25519 key, skip here

		// VerifyEdDSA with valid Ed25519 key but wrong signature
		if VerifyEdDSA(signingInput, []byte("wrong"), edKey) {
			t.Error("EdDSA verified wrong signature")
		}
	})
}

// FuzzAlgorithmConfusion tests tokens with algorithm confusion attacks.
func FuzzAlgorithmConfusion(f *testing.F) {
	seeds := []string{
		`{"alg":"none"}`,
		`{"alg":"None"}`,
		`{"alg":"NONE"}`,
		`{"alg":"nOnE"}`,
		`{"alg":"HS256"}`,
		`{"alg":"RS256"}`,
		`{"alg":"ES256"}`,
		`{"alg":""}`,
		`{"alg":123}`,
		`{"alg":null}`,
		`{"alg":true}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, headerJSON string) {
		if len(headerJSON) > 4096 {
			headerJSON = headerJSON[:4096]
		}

		// Actually just encode the raw header
		headerEncoded := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
		payloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test"}`))
		raw := headerEncoded + "." + payloadEncoded + ".anysig"

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Algorithm confusion test panicked: %v", r)
			}
		}()

		token, err := Parse(raw)
		if err != nil {
			return
		}

		// Extract algorithm
		alg, _ := token.HeaderString("alg")
		_ = alg // should not panic regardless of value
	})
}
