package jwt

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestToken_ClaimStrings(t *testing.T) {
	t.Run("string claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": "audience1",
			},
		}
		auds, ok := token.ClaimStrings("aud")
		if !ok {
			t.Error("ClaimStrings should return true for string claim")
		}
		if len(auds) != 1 || auds[0] != "audience1" {
			t.Errorf("ClaimStrings = %v, want [audience1]", auds)
		}
	})

	t.Run("string slice claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": []any{"audience1", "audience2", "audience3"},
			},
		}
		auds, ok := token.ClaimStrings("aud")
		if !ok {
			t.Error("ClaimStrings should return true for string slice claim")
		}
		if len(auds) != 3 {
			t.Errorf("ClaimStrings length = %d, want 3", len(auds))
		}
		if auds[0] != "audience1" || auds[1] != "audience2" || auds[2] != "audience3" {
			t.Errorf("ClaimStrings = %v, want [audience1, audience2, audience3]", auds)
		}
	})

	t.Run("empty string claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": "   ",
			},
		}
		_, ok := token.ClaimStrings("aud")
		if ok {
			t.Error("ClaimStrings should return false for empty string claim")
		}
	})

	t.Run("empty slice claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": []any{},
			},
		}
		_, ok := token.ClaimStrings("aud")
		if ok {
			t.Error("ClaimStrings should return false for empty slice claim")
		}
	})

	t.Run("slice with non-strings", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": []any{123, 456, 789},
			},
		}
		_, ok := token.ClaimStrings("aud")
		if ok {
			t.Error("ClaimStrings should return false for slice with non-strings")
		}
	})

	t.Run("slice with mixed types", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": []any{"audience1", 123, "audience2", 456},
			},
		}
		auds, ok := token.ClaimStrings("aud")
		if !ok {
			t.Error("ClaimStrings should return true for slice with some strings")
		}
		if len(auds) != 2 {
			t.Errorf("ClaimStrings length = %d, want 2", len(auds))
		}
	})

	t.Run("missing claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{},
		}
		_, ok := token.ClaimStrings("aud")
		if ok {
			t.Error("ClaimStrings should return false for missing claim")
		}
	})

	t.Run("nil token", func(t *testing.T) {
		var token *Token
		_, ok := token.ClaimStrings("aud")
		if ok {
			t.Error("ClaimStrings should return false for nil token")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"aud": 123,
			},
		}
		_, ok := token.ClaimStrings("aud")
		if ok {
			t.Error("ClaimStrings should return false for unsupported type")
		}
	})
}

func TestToken_ClaimUnix(t *testing.T) {
	now := time.Now().Unix()

	t.Run("float64 claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": float64(now + 3600),
			},
		}
		exp, ok := token.ClaimUnix("exp")
		if !ok {
			t.Error("ClaimUnix should return true for float64 claim")
		}
		if exp != now+3600 {
			t.Errorf("ClaimUnix = %d, want %d", exp, now+3600)
		}
	})

	t.Run("int64 claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": int64(now + 3600),
			},
		}
		exp, ok := token.ClaimUnix("exp")
		if !ok {
			t.Error("ClaimUnix should return true for int64 claim")
		}
		if exp != now+3600 {
			t.Errorf("ClaimUnix = %d, want %d", exp, now+3600)
		}
	})

	t.Run("int claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": int(now + 3600),
			},
		}
		exp, ok := token.ClaimUnix("exp")
		if !ok {
			t.Error("ClaimUnix should return true for int claim")
		}
		if exp != now+3600 {
			t.Errorf("ClaimUnix = %d, want %d", exp, now+3600)
		}
	})

	t.Run("json.Number claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": json.Number("1234567890"),
			},
		}
		exp, ok := token.ClaimUnix("exp")
		if !ok {
			t.Error("ClaimUnix should return true for json.Number claim")
		}
		if exp != 1234567890 {
			t.Errorf("ClaimUnix = %d, want 1234567890", exp)
		}
	})

	t.Run("string claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": "1234567890",
			},
		}
		exp, ok := token.ClaimUnix("exp")
		if !ok {
			t.Error("ClaimUnix should return true for string claim")
		}
		if exp != 1234567890 {
			t.Errorf("ClaimUnix = %d, want 1234567890", exp)
		}
	})

	t.Run("invalid string claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": "not-a-number",
			},
		}
		_, ok := token.ClaimUnix("exp")
		if ok {
			t.Error("ClaimUnix should return false for invalid string claim")
		}
	})

	t.Run("empty string claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": "   ",
			},
		}
		_, ok := token.ClaimUnix("exp")
		if ok {
			t.Error("ClaimUnix should return false for empty string claim")
		}
	})

	t.Run("missing claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{},
		}
		_, ok := token.ClaimUnix("exp")
		if ok {
			t.Error("ClaimUnix should return false for missing claim")
		}
	})

	t.Run("nil token", func(t *testing.T) {
		var token *Token
		_, ok := token.ClaimUnix("exp")
		if ok {
			t.Error("ClaimUnix should return false for nil token")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"exp": true,
			},
		}
		_, ok := token.ClaimUnix("exp")
		if ok {
			t.Error("ClaimUnix should return false for unsupported type")
		}
	})
}

func TestClaimUnix_AllTypes(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		want     int64
		wantOk   bool
	}{
		{"float64", float64(1234.0), 1234, true},
		{"float32", float32(1234.0), 1234, true},
		{"int64", int64(1234), 1234, true},
		{"int", int(1234), 1234, true},
		{"int32", int32(1234), 1234, true},
		{"uint64", uint64(1234), 1234, true},
		{"json.Number valid", json.Number("1234"), 1234, true},
		{"json.Number float", json.Number("1234.56"), 1234, true},
		{"json.Number invalid", json.Number("abc"), 0, false},
		{"string valid", "1234", 1234, true},
		{"string empty", "", 0, false},
		{"string whitespace", "   ", 0, false},
		{"string invalid", "abc", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := claimUnix(tt.value)
			if ok != tt.wantOk {
				t.Errorf("claimUnix() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if ok && got != tt.want {
				t.Errorf("claimUnix() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestToken_HeaderString(t *testing.T) {
	t.Run("valid header", func(t *testing.T) {
		token := &Token{
			Header: map[string]any{
				"alg": "HS256",
			},
		}
		alg, ok := token.HeaderString("alg")
		if !ok {
			t.Error("HeaderString should return true for valid header")
		}
		if alg != "HS256" {
			t.Errorf("HeaderString = %q, want HS256", alg)
		}
	})

	t.Run("header with whitespace", func(t *testing.T) {
		token := &Token{
			Header: map[string]any{
				"alg": "  HS256  ",
			},
		}
		alg, ok := token.HeaderString("alg")
		if !ok {
			t.Error("HeaderString should return true after trimming whitespace")
		}
		if alg != "HS256" {
			t.Errorf("HeaderString = %q, want HS256", alg)
		}
	})

	t.Run("empty string after trim", func(t *testing.T) {
		token := &Token{
			Header: map[string]any{
				"alg": "   ",
			},
		}
		_, ok := token.HeaderString("alg")
		if ok {
			t.Error("HeaderString should return false for empty string after trim")
		}
	})

	t.Run("non-string header", func(t *testing.T) {
		token := &Token{
			Header: map[string]any{
				"alg": 123,
			},
		}
		_, ok := token.HeaderString("alg")
		if ok {
			t.Error("HeaderString should return false for non-string header")
		}
	})

	t.Run("missing header", func(t *testing.T) {
		token := &Token{
			Header: map[string]any{},
		}
		_, ok := token.HeaderString("alg")
		if ok {
			t.Error("HeaderString should return false for missing header")
		}
	})

	t.Run("nil token", func(t *testing.T) {
		var token *Token
		_, ok := token.HeaderString("alg")
		if ok {
			t.Error("HeaderString should return false for nil token")
		}
	})
}

func TestToken_ClaimString(t *testing.T) {
	t.Run("valid claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"sub": "user123",
			},
		}
		sub, ok := token.ClaimString("sub")
		if !ok {
			t.Error("ClaimString should return true for valid claim")
		}
		if sub != "user123" {
			t.Errorf("ClaimString = %q, want user123", sub)
		}
	})

	t.Run("claim with whitespace", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"sub": "  user123  ",
			},
		}
		sub, ok := token.ClaimString("sub")
		if !ok {
			t.Error("ClaimString should return true after trimming whitespace")
		}
		if sub != "user123" {
			t.Errorf("ClaimString = %q, want user123", sub)
		}
	})

	t.Run("empty string after trim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"sub": "   ",
			},
		}
		_, ok := token.ClaimString("sub")
		if ok {
			t.Error("ClaimString should return false for empty string after trim")
		}
	})

	t.Run("non-string claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{
				"sub": 123,
			},
		}
		_, ok := token.ClaimString("sub")
		if ok {
			t.Error("ClaimString should return false for non-string claim")
		}
	})

	t.Run("missing claim", func(t *testing.T) {
		token := &Token{
			Payload: map[string]any{},
		}
		_, ok := token.ClaimString("sub")
		if ok {
			t.Error("ClaimString should return false for missing claim")
		}
	})

	t.Run("nil token", func(t *testing.T) {
		var token *Token
		_, ok := token.ClaimString("sub")
		if ok {
			t.Error("ClaimString should return false for nil token")
		}
	})
}

func TestDecodeSegment(t *testing.T) {
	t.Run("valid base64url", func(t *testing.T) {
		data := []byte(`{"alg":"HS256"}`)
		encoded := base64.RawURLEncoding.EncodeToString(data)
		decoded, err := DecodeSegment(encoded)
		if err != nil {
			t.Errorf("DecodeSegment error: %v", err)
		}
		if string(decoded) != string(data) {
			t.Errorf("DecodeSegment = %q, want %q", string(decoded), string(data))
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		_, err := DecodeSegment("!!!invalid!!!")
		if err == nil {
			t.Error("DecodeSegment should return error for invalid base64")
		}
	})
}
