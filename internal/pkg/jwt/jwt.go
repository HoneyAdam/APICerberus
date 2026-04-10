package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidTokenFormat = errors.New("invalid jwt format")
	ErrInvalidHeader      = errors.New("invalid jwt header")
	ErrInvalidPayload     = errors.New("invalid jwt payload")
	ErrInvalidSignature   = errors.New("invalid jwt signature")
)

// Token is a parsed JWT token with decoded JSON header/payload and raw signature.
type Token struct {
	Raw          string
	Header       map[string]any
	Payload      map[string]any
	Signature    []byte
	SigningInput string
}

// Parse splits and decodes a compact JWT token.
func Parse(raw string) (*Token, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidTokenFormat
	}

	headerBytes, err := DecodeSegment(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHeader, err)
	}
	payloadBytes, err := DecodeSegment(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	signature, err := DecodeSegment(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}

	header := make(map[string]any)
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHeader, err)
	}
	payload := make(map[string]any)
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	return &Token{
		Raw:          raw,
		Header:       header,
		Payload:      payload,
		Signature:    signature,
		SigningInput: parts[0] + "." + parts[1],
	}, nil
}

// DecodeSegment decodes base64url segment without padding.
func DecodeSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}

// HeaderString returns a string-typed header value.
func (t *Token) HeaderString(name string) (string, bool) {
	if t == nil {
		return "", false
	}
	raw, ok := t.Header[name]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// ClaimString returns audience-like claim values as string slice.
func (t *Token) ClaimStrings(name string) ([]string, bool) {
	if t == nil {
		return nil, false
	}
	raw, ok := t.Payload[name]
	if !ok {
		return nil, false
	}

	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, false
		}
		return []string{trimmed}, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

// ClaimString returns a string-typed payload claim.
func (t *Token) ClaimString(name string) (string, bool) {
	if t == nil {
		return "", false
	}
	raw, ok := t.Payload[name]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// ClaimUnix returns unix timestamp claim (exp/iat/nbf).
func (t *Token) ClaimUnix(name string) (int64, bool) {
	if t == nil {
		return 0, false
	}
	raw, ok := t.Payload[name]
	if !ok {
		return 0, false
	}
	return claimUnix(raw)
}

func claimUnix(raw any) (int64, bool) {
	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case uint64:
		// uint64 timestamps > 2^63 are invalid; reject rather than overflow
		if v > 1<<63-1 {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return i, true
		}
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return int64(f), true
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, false
		}
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// EncodeSegment encodes data using base64url without padding.
func EncodeSegment(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// SignHS256 signs the input with HMAC-SHA256 using golang-jwt/jwt/v5.
func SignHS256(signingInput string, secret []byte) ([]byte, error) {
	if len(secret) < minHS256SecretLength {
		return nil, fmt.Errorf("%w: secret length %d is below minimum %d bytes", ErrWeakHS256Secret, len(secret), minHS256SecretLength)
	}
	method := jwt.SigningMethodHS256
	sig, err := method.Sign(signingInput, secret)
	if err != nil {
		return nil, fmt.Errorf("sign HS256: %w", err)
	}
	return sig, nil
}

// VerifyHS256 verifies the HMAC-SHA256 signature using golang-jwt/jwt/v5.
func VerifyHS256(signingInput string, signature []byte, secret []byte) bool {
	if len(secret) < minHS256SecretLength {
		return false
	}
	method := jwt.SigningMethodHS256
	return method.Verify(signingInput, signature, secret) == nil
}

// VerifyRS256 verifies the RSA-SHA256 signature using golang-jwt/jwt/v5.
func VerifyRS256(signingInput string, signature []byte, publicKey *rsa.PublicKey) bool {
	if publicKey == nil {
		return false
	}
	method := jwt.SigningMethodRS256
	return method.Verify(signingInput, signature, publicKey) == nil
}

// SignES256 signs the input with ECDSA P-256 using golang-jwt/jwt/v5.
func SignES256(signingInput string, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	if privateKey == nil {
		return nil, errors.New("private key is nil")
	}
	method := jwt.SigningMethodES256
	sig, err := method.Sign(signingInput, privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign ES256: %w", err)
	}
	return sig, nil
}

// VerifyES256 verifies the ECDSA P-256 signature using golang-jwt/jwt/v5.
func VerifyES256(signingInput string, signature []byte, publicKey *ecdsa.PublicKey) bool {
	if publicKey == nil {
		return false
	}
	method := jwt.SigningMethodES256
	return method.Verify(signingInput, signature, publicKey) == nil
}

// VerifyEdDSA verifies the Ed25519 signature using golang-jwt/jwt/v5.
func VerifyEdDSA(signingInput string, signature []byte, publicKey any) bool {
	edKey, ok := publicKey.(ed25519.PublicKey)
	if !ok {
		return false
	}
	method := jwt.SigningMethodEdDSA
	return method.Verify(signingInput, signature, edKey) == nil
}
