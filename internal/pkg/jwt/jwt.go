package jwt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// ClaimStrings returns audience-like claim values as string slice.
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
		return int64(v), true // #nosec G115 -- JWT timestamps (exp/iat/nbf) always fit within int64.
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
