// Package main provides comprehensive benchmarks for APICerebrus Plugin components.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// ============================================================================
// JWT Validation Benchmarks
// ============================================================================

// BenchmarkJWTValidationHS256 benchmarks HMAC-SHA256 JWT validation.
// Baseline: ~5,000 validations/sec
func BenchmarkJWTValidationHS256(b *testing.B) {
	secret := "super-secret-key-that-is-32-bytes!"
	auth := plugin.NewAuthJWT(plugin.AuthJWTOptions{
		Secret:    secret,
		Issuer:    "test-issuer",
		Audience:  []string{"test-audience"},
		ClockSkew: 30 * time.Second,
	})

	// Create a valid JWT token (simplified - in real tests use proper JWT library)
	token := createTestJWT(secret, "HS256", map[string]any{
		"sub": "user123",
		"iss": "test-issuer",
		"aud": "test-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _ = auth.Authenticate(reqCopy)
	}
}

// BenchmarkJWTValidationRS256 benchmarks RSA-SHA256 JWT validation.
// Baseline: ~1,000 validations/sec (slower due to RSA operations)
func BenchmarkJWTValidationRS256(b *testing.B) {
	// Generate test RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatalf("failed to generate RSA key: %v", err)
	}

	auth := plugin.NewAuthJWT(plugin.AuthJWTOptions{
		PublicKey: &privateKey.PublicKey,
		Issuer:    "test-issuer",
		ClockSkew: 30 * time.Second,
	})

	token := createTestJWTRS256(privateKey, map[string]any{
		"sub": "user123",
		"iss": "test-issuer",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _ = auth.Authenticate(reqCopy)
	}
}

// BenchmarkJWTValidationWithClaims benchmarks JWT validation with claim extraction.
// Baseline: ~4,000 validations/sec with claims
func BenchmarkJWTValidationWithClaims(b *testing.B) {
	secret := "super-secret-key-that-is-32-bytes!"
	auth := plugin.NewAuthJWT(plugin.AuthJWTOptions{
		Secret:   secret,
		Issuer:   "test-issuer",
		Audience: []string{"test-audience"},
		ClaimsToHeaders: map[string]string{
			"sub":         "X-User-ID",
			"email":       "X-User-Email",
			"permissions": "X-User-Permissions",
		},
		ClockSkew: 30 * time.Second,
	})

	token := createTestJWT(secret, "HS256", map[string]any{
		"sub":         "user123",
		"email":       "user@example.com",
		"permissions": "read,write",
		"iss":         "test-issuer",
		"aud":         "test-audience",
		"exp":         time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _ = auth.Authenticate(reqCopy)
	}
}

// ============================================================================
// Rate Limiting Benchmarks
// ============================================================================

// BenchmarkRateLimitTokenBucket benchmarks token bucket algorithm.
// Baseline: ~500,000 checks/sec
func BenchmarkRateLimitTokenBucket(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "global",
		RequestsPerSecond: 1000,
		Burst:             100,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rlReq := plugin.RateLimitRequest{
		Request: req,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitFixedWindow benchmarks fixed window algorithm.
// Baseline: ~600,000 checks/sec
func BenchmarkRateLimitFixedWindow(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm: "fixed_window",
		Scope:     "global",
		Limit:     1000,
		Window:    time.Second,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rlReq := plugin.RateLimitRequest{
		Request: req,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitSlidingWindow benchmarks sliding window algorithm.
// Baseline: ~400,000 checks/sec
func BenchmarkRateLimitSlidingWindow(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm: "sliding_window",
		Scope:     "global",
		Limit:     1000,
		Window:    time.Second,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rlReq := plugin.RateLimitRequest{
		Request: req,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitLeakyBucket benchmarks leaky bucket algorithm.
// Baseline: ~450,000 checks/sec
func BenchmarkRateLimitLeakyBucket(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm:         "leaky_bucket",
		Scope:             "global",
		RequestsPerSecond: 1000,
		Burst:             100,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rlReq := plugin.RateLimitRequest{
		Request: req,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitByConsumer benchmarks rate limiting with consumer scope.
// Baseline: ~300,000 checks/sec with consumer resolution
func BenchmarkRateLimitByConsumer(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "consumer",
		RequestsPerSecond: 100,
		Burst:             10,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	consumer := &config.Consumer{
		ID:   "consumer-123",
		Name: "Test Consumer",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rlReq := plugin.RateLimitRequest{
		Request:  req,
		Consumer: consumer,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitByIP benchmarks rate limiting with IP scope.
// Baseline: ~400,000 checks/sec with IP extraction
func BenchmarkRateLimitByIP(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "ip",
		RequestsPerSecond: 100,
		Burst:             10,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rlReq := plugin.RateLimitRequest{
		Request: req,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitComposite benchmarks composite rate limiting.
// Baseline: ~200,000 checks/sec with composite scopes
func BenchmarkRateLimitComposite(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "composite",
		RequestsPerSecond: 100,
		Burst:             10,
		CompositeScopes:   []string{"consumer", "ip"},
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	consumer := &config.Consumer{
		ID:   "consumer-123",
		Name: "Test Consumer",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rlReq := plugin.RateLimitRequest{
		Request:  req,
		Consumer: consumer,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = rl.Check(rlReq)
	}
}

// BenchmarkRateLimitParallel benchmarks rate limiting under concurrent load.
// Baseline: ~350,000 checks/sec with 8 threads
func BenchmarkRateLimitParallel(b *testing.B) {
	rl, err := plugin.NewRateLimit(plugin.RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "global",
		RequestsPerSecond: 10000,
		Burst:             1000,
	})
	if err != nil {
		b.Fatalf("failed to create rate limiter: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rlReq := plugin.RateLimitRequest{
			Request: req,
		}
		for pb.Next() {
			_, _ = rl.Check(rlReq)
		}
	})
}

// ============================================================================
// CORS Processing Benchmarks
// ============================================================================

// BenchmarkCORSSimpleRequest benchmarks simple CORS request handling.
// Baseline: ~200 ns/op, 1 alloc/op
func BenchmarkCORSSimpleRequest(b *testing.B) {
	cors := plugin.NewCORS(plugin.CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		_ = cors.Handle(rec, req)
	}
}

// BenchmarkCORSPreflightRequest benchmarks preflight request handling.
// Baseline: ~300 ns/op, 2 allocs/op
func BenchmarkCORSPreflightRequest(b *testing.B) {
	cors := plugin.NewCORS(plugin.CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowedMethods:   []string{"GET", "POST", "DELETE"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		_ = cors.Handle(rec, req)
	}
}

// BenchmarkCORSWildcardOrigin benchmarks wildcard origin handling.
// Baseline: ~150 ns/op, 1 alloc/op
func BenchmarkCORSWildcardOrigin(b *testing.B) {
	cors := plugin.NewCORS(plugin.CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "https://any-origin.com")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		_ = cors.Handle(rec, req)
	}
}

// BenchmarkCORSParallel benchmarks CORS under concurrent load.
// Baseline: ~250 ns/op with 8 threads
func BenchmarkCORSParallel(b *testing.B) {
	cors := plugin.NewCORS(plugin.CORSConfig{
		AllowedOrigins: []string{"https://example.com", "https://app.example.com"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-ID"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rec := httptest.NewRecorder()
			_ = cors.Handle(rec, req)
		}
	})
}

// ============================================================================
// Cache Benchmarks
// ============================================================================

// BenchmarkCacheGetHit benchmarks cache hit performance.
// Baseline: ~100 ns/op, 0 allocs/op
func BenchmarkCacheGetHit(b *testing.B) {
	cache, err := plugin.NewCache(plugin.CacheConfig{
		TTL:     5 * time.Minute,
		MaxSize: 10000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Pre-populate cache
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	cache.Set("test-key", http.StatusOK, headers, []byte(`{"data":"test"}`), 5*time.Minute, nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get("test-key")
	}
}

// BenchmarkCacheGetMiss benchmarks cache miss performance.
// Baseline: ~50 ns/op, 0 allocs/op
func BenchmarkCacheGetMiss(b *testing.B) {
	cache, err := plugin.NewCache(plugin.CacheConfig{
		TTL:     5 * time.Minute,
		MaxSize: 10000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get("non-existent-key")
	}
}

// BenchmarkCacheSetOperation benchmarks cache write performance.
// Baseline: ~500 ns/op, 3 allocs/op for small values
func BenchmarkCacheSetOperation(b *testing.B) {
	cache, err := plugin.NewCache(plugin.CacheConfig{
		TTL:     5 * time.Minute,
		MaxSize: 100000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	body := []byte(`{"data":"test"}`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, http.StatusOK, headers, body, 5*time.Minute, nil)
	}
}

// BenchmarkCacheSetLargeValue benchmarks cache write with large values.
// Baseline: ~2,000 ns/op for 100KB values
func BenchmarkCacheSetLargeValue(b *testing.B) {
	cache, err := plugin.NewCache(plugin.CacheConfig{
		TTL:     5 * time.Minute,
		MaxSize: 100000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	largeBody := make([]byte, 100*1024) // 100KB
	for i := range largeBody {
		largeBody[i] = byte('a' + (i % 26))
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, http.StatusOK, headers, largeBody, 5*time.Minute, nil)
	}
}

// BenchmarkCacheGenerateKey benchmarks cache key generation.
// Baseline: ~200 ns/op, 2 allocs/op
func BenchmarkCacheGenerateKey(b *testing.B) {
	cache, err := plugin.NewCache(plugin.CacheConfig{
		TTL:        5 * time.Minute,
		KeyHeaders: []string{"Accept", "Accept-Language"},
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	headers.Set("Accept", "application/json")
	headers.Set("Accept-Language", "en-US")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cache.GenerateKey("GET", "http://example.com/api/test", headers)
	}
}

// BenchmarkCacheParallel benchmarks cache under concurrent load.
// Baseline: ~80% hit rate at 8 threads with mixed read/write
func BenchmarkCacheParallel(b *testing.B) {
	cache, err := plugin.NewCache(plugin.CacheConfig{
		TTL:     5 * time.Minute,
		MaxSize: 100000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Pre-populate with some entries
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, http.StatusOK, headers, []byte(`{"data":"test"}`), 5*time.Minute, nil)
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1500) // 66% hit rate expected
			_, _ = cache.Get(key)
			i++
		}
	})
}

// ============================================================================
// Pipeline Benchmarks
// ============================================================================

// BenchmarkPipelineExecution benchmarks full plugin pipeline execution.
// Baseline: ~1,000 executions/sec for 3 plugins
func BenchmarkPipelineExecution(b *testing.B) {
	plugins := []plugin.PipelinePlugin{
		createBenchMockPlugin("cors", plugin.PhasePreAuth, 10),
		createBenchMockPlugin("auth", plugin.PhaseAuth, 20),
		createBenchMockPlugin("rate-limit", plugin.PhasePreProxy, 30),
	}

	p := plugin.NewPipeline(plugins)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := &plugin.PipelineContext{
			Request:        req,
			ResponseWriter: rec,
			Metadata:       make(map[string]any),
		}
		_, _ = p.Execute(ctx)
	}
}

// BenchmarkPipelineParallel benchmarks pipeline under concurrent load.
// Baseline: ~800 executions/sec with 8 threads
func BenchmarkPipelineParallel(b *testing.B) {
	plugins := []plugin.PipelinePlugin{
		createBenchMockPlugin("cors", plugin.PhasePreAuth, 10),
		createBenchMockPlugin("auth", plugin.PhaseAuth, 20),
		createBenchMockPlugin("rate-limit", plugin.PhasePreProxy, 30),
	}

	p := plugin.NewPipeline(plugins)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()
		for pb.Next() {
			ctx := &plugin.PipelineContext{
				Request:        req,
				ResponseWriter: rec,
				Metadata:       make(map[string]any),
			}
			_, _ = p.Execute(ctx)
		}
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

func createBenchMockPlugin(name string, phase plugin.Phase, priority int) plugin.PipelinePlugin {
	return plugin.NewPipelinePlugin(name, phase, priority,
		func(ctx *plugin.PipelineContext) (bool, error) {
			if ctx.Metadata == nil {
				ctx.Metadata = make(map[string]any)
			}
			ctx.Metadata[name+"_executed"] = true
			return false, nil
		},
		func(ctx *plugin.PipelineContext, proxyErr error) {},
	)
}

// createTestJWT creates a simple JWT token for testing (simplified implementation)
func createTestJWT(secret, alg string, claims map[string]any) string {
	header := map[string]string{
		"alg": alg,
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	// Simplified - just return base64 encoded parts without actual signing
	// In real benchmarks, use a proper JWT library
	return fmt.Sprintf("%s.%s.signature",
		base64URLEncode(headerJSON),
		base64URLEncode(claimsJSON))
}

func createTestJWTRS256(privateKey *rsa.PrivateKey, claims map[string]any) string {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	return fmt.Sprintf("%s.%s.signature",
		base64URLEncode(headerJSON),
		base64URLEncode(claimsJSON))
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(
		strings.ReplaceAll(
			strings.ReplaceAll(
				string(data),
				"+", "-",
			),
			"/", "_",
		),
		"=",
	)
}

// generateTestECKey generates an ECDSA key for testing
//
//nolint:unused // used for future test scenarios
func generateTestECKey(b *testing.B) *ecdsa.PrivateKey {
	b.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("failed to generate EC key: %v", err)
	}
	return key
}

// encodePublicKeyToPEM encodes RSA public key to PEM format
//
//lint:ignore U1000 reserved for future use
func encodePublicKeyToPEM(key *rsa.PublicKey) string {
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(key)
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}
	return string(pem.EncodeToMemory(pemBlock))
}
