package jwt

import "errors"

const minHS256SecretLength = 32 // minimum 256 bits for HS256 (NIST SP 800-107)

var ErrWeakHS256Secret = errors.New("weak HS256 secret")

// L-005 NOTE: 32 bytes (256 bits) is the NIST-recommended minimum key length for HMAC-SHA256.
// This provides ~128 bits of effective security against classical brute-force attacks.
// While Grover's algorithm on quantum computers could theoretically reduce this to ~64 bits,
// no known quantum computer is capable of breaking 256-bit HMAC at scale. For post-quantum
// security, RS256/ES256 (asymmetric algorithms) should be used instead of HS256 (symmetric).
