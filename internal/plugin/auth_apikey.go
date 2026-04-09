package plugin

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// AuthError represents an authentication failure.
type AuthError struct {
	Code    string
	Message string
	Status  int
}

func (e *AuthError) Error() string { return e.Message }

var (
	ErrMissingAPIKey = &AuthError{
		Code:    "missing_api_key",
		Message: "API key is required",
		Status:  http.StatusUnauthorized,
	}
	ErrInvalidAPIKey = &AuthError{
		Code:    "invalid_api_key",
		Message: "API key is invalid",
		Status:  http.StatusUnauthorized,
	}
	ErrExpiredAPIKey = &AuthError{
		Code:    "expired_api_key",
		Message: "API key is expired",
		Status:  http.StatusUnauthorized,
	}
)

// AuthAPIKey authenticates requests using consumer API keys.
type AuthAPIKey struct {
	keyNames    []string
	queryNames  []string
	cookieNames []string

	consumers []config.Consumer
	entries   []apiKeyEntry
	buckets   map[[32]byte][]*apiKeyEntry
	lookup    APIKeyLookupFunc
	backoff   *AuthBackoff
}

type AuthAPIKeyOptions struct {
	KeyNames    []string // header names, default: X-API-Key + Authorization(Bearer)
	QueryNames  []string // default: apikey, api_key
	CookieNames []string // default: apikey
	Lookup      APIKeyLookupFunc
	Backoff     *AuthBackoff // optional per-IP auth failure backoff
}

// APIKeyLookupFunc resolves a raw API key from an external source.
type APIKeyLookupFunc func(rawKey string, req *http.Request) (*config.Consumer, error)

type apiKeyEntry struct {
	consumer  *config.Consumer
	rawKey    string
	hash      [32]byte
	expiresAt *time.Time
}

func NewAuthAPIKey(consumers []config.Consumer, opts AuthAPIKeyOptions) *AuthAPIKey {
	if len(opts.KeyNames) == 0 {
		opts.KeyNames = []string{"X-API-Key", "Authorization"}
	}
	if len(opts.QueryNames) == 0 {
		opts.QueryNames = []string{"apikey", "api_key"}
	}
	if len(opts.CookieNames) == 0 {
		opts.CookieNames = []string{"apikey"}
	}

	copiedConsumers := append([]config.Consumer(nil), consumers...)
	entries := make([]apiKeyEntry, 0)
	buckets := make(map[[32]byte][]*apiKeyEntry)

	for i := range copiedConsumers {
		consumer := &copiedConsumers[i]
		for _, key := range consumer.APIKeys {
			raw := strings.TrimSpace(key.Key)
			if raw == "" {
				continue
			}

			var expiresAt *time.Time
			if ts := strings.TrimSpace(key.ExpiresAt); ts != "" {
				if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
					expiresAt = &parsed
				}
			}

			entry := apiKeyEntry{
				consumer:  consumer,
				rawKey:    raw,
				hash:      sha256.Sum256([]byte(raw)),
				expiresAt: expiresAt,
			}
			entries = append(entries, entry)
		}
	}

	for i := range entries {
		entry := &entries[i]
		hash := entry.hash
		buckets[hash] = append(buckets[hash], entry)
	}

	return &AuthAPIKey{
		keyNames:    append([]string(nil), opts.KeyNames...),
		queryNames:  append([]string(nil), opts.QueryNames...),
		cookieNames: append([]string(nil), opts.CookieNames...),
		consumers:   copiedConsumers,
		entries:     entries,
		buckets:     buckets,
		lookup:      opts.Lookup,
		backoff:     opts.Backoff,
	}
}

func (a *AuthAPIKey) Name() string  { return "auth-apikey" }
func (a *AuthAPIKey) Phase() Phase  { return PhaseAuth }
func (a *AuthAPIKey) Priority() int { return 10 }

func (a *AuthAPIKey) Authenticate(req *http.Request) (*config.Consumer, error) {
	if a.backoff != nil {
		if delay := a.backoff.Check(req); delay > 0 {
			return nil, &AuthError{
				Code:    "auth_rate_limited",
				Message: "Too many failed attempts. Please try again later.",
				Status:  http.StatusTooManyRequests,
			}
		}
	}

	key := strings.TrimSpace(a.extractKey(req))
	if key == "" {
		return nil, ErrMissingAPIKey // Don't rate-limit missing keys (not brute force)
	}
	consumer, err := a.lookupWithRequest(key, req)
	if err != nil && a.backoff != nil {
		// Only record backoff for invalid key errors, not expired keys.
		if authErr, ok := err.(*AuthError); ok && authErr.Code == "invalid_api_key" {
			a.backoff.RecordFailure(req)
		}
	} else if err == nil && a.backoff != nil {
		a.backoff.RecordSuccess(req)
	}
	return consumer, err
}

func (a *AuthAPIKey) Lookup(key string) (*config.Consumer, error) {
	return a.lookupWithRequest(key, nil)
}

func (a *AuthAPIKey) lookupWithRequest(key string, req *http.Request) (*config.Consumer, error) {
	provided := strings.TrimSpace(key)
	if provided == "" {
		return nil, ErrMissingAPIKey
	}
	if a == nil {
		return nil, ErrInvalidAPIKey
	}
	if a.lookup != nil {
		return a.lookup(provided, req)
	}

	hash := sha256.Sum256([]byte(provided))
	candidates := a.buckets[hash]

	// Hash bucket first for performance.
	for _, entry := range candidates {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(entry.rawKey)) == 1 {
			if isExpired(entry.expiresAt) {
				return nil, ErrExpiredAPIKey
			}
			return entry.consumer, nil
		}
	}

	// Fallback linear scan preserves correctness on unlikely hash-collision scenarios.
	for i := range a.entries {
		entry := &a.entries[i]
		if subtle.ConstantTimeCompare([]byte(provided), []byte(entry.rawKey)) == 1 {
			if isExpired(entry.expiresAt) {
				return nil, ErrExpiredAPIKey
			}
			return entry.consumer, nil
		}
	}

	return nil, ErrInvalidAPIKey
}

func (a *AuthAPIKey) extractKey(req *http.Request) string {
	if req == nil {
		return ""
	}

	for _, header := range a.keyNames {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}

		if strings.EqualFold(header, "Authorization") {
			auth := strings.TrimSpace(req.Header.Get("Authorization"))
			if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
				if key := strings.TrimSpace(auth[7:]); key != "" {
					return key
				}
			}
			continue
		}

		if key := strings.TrimSpace(req.Header.Get(header)); key != "" {
			return key
		}
	}

	for _, q := range a.queryNames {
		if key := strings.TrimSpace(req.URL.Query().Get(q)); key != "" {
			return key
		}
	}

	for _, c := range a.cookieNames {
		cookie, err := req.Cookie(c)
		if err != nil {
			continue
		}
		if key := strings.TrimSpace(cookie.Value); key != "" {
			return key
		}
	}

	return ""
}

func isExpired(exp *time.Time) bool {
	if exp == nil {
		return false
	}
	return time.Now().After(*exp)
}

func (a *AuthAPIKey) DebugSummary() string {
	return fmt.Sprintf("consumers=%d keys=%d", len(a.consumers), len(a.entries))
}
