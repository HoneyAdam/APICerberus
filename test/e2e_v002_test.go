package test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

func TestE2EAPIKeyRateLimitAndCORS(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("v002-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Services: []config.Service{
			{ID: "svc-1", Name: "svc-1", Protocol: "http", Upstream: "up-1"},
		},
		Routes: []config.Route{
			{
				ID:      "route-1",
				Name:    "route-1",
				Service: "svc-1",
				Paths:   []string{"/v002"},
				Methods: []string{http.MethodGet, http.MethodOptions},
				Plugins: []config.PluginConfig{
					{
						Name: "rate-limit",
						Config: map[string]any{
							"algorithm": "fixed_window",
							"scope":     "consumer",
							"limit":     1,
							"window":    "1s",
						},
					},
				},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-1",
				Name:      "up-1",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "t1", Address: mustHost(t, upstream.URL), Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           1 * time.Second,
						Timeout:            1 * time.Second,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
		Consumers: []config.Consumer{
			{
				ID:   "consumer-a",
				Name: "consumer-a",
				APIKeys: []config.ConsumerAPIKey{
					{ID: "k1", Key: "ck_live_v002"},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{
				Name: "cors",
				Config: map[string]any{
					"allowed_origins": []any{"https://app.example.com"},
					"allowed_methods": []any{"GET", "OPTIONS"},
				},
			},
			{
				Name: "auth-apikey",
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	waitForHTTPReady(t, "http://"+gwAddr+"/v002", nil)

	preflightReq, _ := http.NewRequest(http.MethodOptions, "http://"+gwAddr+"/v002", nil)
	preflightReq.Header.Set("Origin", "https://app.example.com")
	preflightReq.Header.Set("Access-Control-Request-Method", "GET")
	preflightResp, err := http.DefaultClient.Do(preflightReq)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	_ = preflightResp.Body.Close()
	if preflightResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected preflight 204 got %d", preflightResp.StatusCode)
	}
	if preflightResp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("expected CORS headers on preflight")
	}

	req1, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+"/v002", nil)
	req1.Header.Set("X-API-Key", "ck_live_v002")
	req1.Header.Set("Origin", "https://app.example.com")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	body1 := readAllAndClose(t, resp1.Body)
	if resp1.StatusCode != http.StatusOK || body1 != "v002-ok" {
		t.Fatalf("unexpected first response status=%d body=%q", resp1.StatusCode, body1)
	}
	if resp1.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("expected CORS headers on actual request")
	}

	req2, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+"/v002", nil)
	req2.Header.Set("X-API-Key", "ck_live_v002")
	req2.Header.Set("Origin", "https://app.example.com")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected second response 429 got %d", resp2.StatusCode)
	}
	if resp2.Header.Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
}

func TestE2EJWTAuthRS256RateLimitPerConsumer(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey error: %v", err)
	}

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": "kid-e2e",
					"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
				},
			},
		})
	}))
	defer jwksServer.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("jwt-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Services: []config.Service{
			{ID: "svc-jwt", Name: "svc-jwt", Protocol: "http", Upstream: "up-jwt"},
		},
		Routes: []config.Route{
			{
				ID:      "route-jwt",
				Name:    "route-jwt",
				Service: "svc-jwt",
				Paths:   []string{"/jwt"},
				Methods: []string{http.MethodGet},
				Plugins: []config.PluginConfig{
					{
						Name: "rate-limit",
						Config: map[string]any{
							"algorithm": "fixed_window",
							"scope":     "consumer",
							"limit":     1,
							"window":    "1s",
						},
					},
				},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-jwt",
				Name:      "up-jwt",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "t-jwt", Address: mustHost(t, upstream.URL), Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           1 * time.Second,
						Timeout:            1 * time.Second,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{
				Name: "auth-jwt",
				Config: map[string]any{
					"jwks_url":        jwksServer.URL,
					"jwks_ttl":        "1h",
					"required_claims": []any{"sub"},
				},
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	waitForHTTPReady(t, "http://"+gwAddr+"/jwt", nil)

	tokenA := mustBuildRS256JWT(t, privateKey, "kid-e2e", "consumer-a", time.Now().Add(10*time.Minute))
	tokenB := mustBuildRS256JWT(t, privateKey, "kid-e2e", "consumer-b", time.Now().Add(10*time.Minute))

	call := func(token string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+"/jwt", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("jwt request failed: %v", err)
		}
		return resp
	}

	resp1 := call(tokenA)
	if body := readAllAndClose(t, resp1.Body); resp1.StatusCode != http.StatusOK || body != "jwt-ok" {
		t.Fatalf("consumer A first request unexpected status=%d body=%q", resp1.StatusCode, body)
	}

	resp2 := call(tokenA)
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("consumer A second request expected 429 got %d", resp2.StatusCode)
	}

	resp3 := call(tokenB)
	if body := readAllAndClose(t, resp3.Body); resp3.StatusCode != http.StatusOK || body != "jwt-ok" {
		t.Fatalf("consumer B first request unexpected status=%d body=%q", resp3.StatusCode, body)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
}

func mustBuildRS256JWT(t *testing.T, privateKey *rsa.PrivateKey, kid, sub string, exp time.Time) string {
	t.Helper()
	header := map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	}
	payload := map[string]any{
		"sub": sub,
		"exp": exp.Unix(),
	}
	headerSeg := mustJSONSegment(t, header)
	payloadSeg := mustJSONSegment(t, payload)
	signingInput := headerSeg + "." + payloadSeg
	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15 error: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func mustJSONSegment(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
