package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// EncodeSegment encodes bytes to base64url without padding.
func EncodeSegment(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// SignHS256 creates a JWT signature for signingInput using HMAC-SHA256.
func SignHS256(signingInput string, secret []byte) []byte {
	if len(secret) == 0 {
		return nil
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

// VerifyHS256 validates JWT signature using HMAC-SHA256.
func VerifyHS256(signingInput string, signature []byte, secret []byte) bool {
	if len(secret) == 0 {
		return false
	}
	expected := SignHS256(signingInput, secret)
	return subtle.ConstantTimeCompare(expected, signature) == 1
}
