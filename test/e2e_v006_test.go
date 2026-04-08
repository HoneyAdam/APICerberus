package test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestE2EAuditLoggingCapturesMaskedData(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"token":"upstream-secret"}`))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v006-audit"
	routePath := "/v006/audit"
	cfg := v006Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)
	waitForGatewayListener(t, gwAddr)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v006-audit@example.com",
		"name":            "V006 Audit",
		"password":        "pass-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "v006-audit-key",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "full_key")
	if liveKey == "" {
		t.Fatalf("expected full_key from API key create response")
	}

	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	req, err := http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, bytes.NewBufferString(`{"password":"client-secret","token":"client-token"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", liveKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("unexpected gateway response status=%d body=%q", resp.StatusCode, body)
	}

	entry := waitForAuditEntry(t, adminAddr, cfg.Admin.APIKey, routeID)
	requestBody := anyString(entry, "request_body")
	responseBody := anyString(entry, "response_body")
	if strings.Contains(requestBody, "client-secret") || strings.Contains(requestBody, "client-token") {
		t.Fatalf("request body should be masked, got %q", requestBody)
	}
	if strings.Contains(responseBody, "upstream-secret") {
		t.Fatalf("response body should be masked, got %q", responseBody)
	}
	if !strings.Contains(requestBody, "***") || !strings.Contains(responseBody, "***") {
		t.Fatalf("expected masked values in request/response bodies, got req=%q resp=%q", requestBody, responseBody)
	}

	headers, ok := entry["request_headers"].(map[string]any)
	if !ok {
		t.Fatalf("request_headers is not an object: %#v", entry["request_headers"])
	}
	apiKeyValue := ""
	for key, raw := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "X-API-Key") {
			apiKeyValue = strings.TrimSpace(anyString(map[string]any{"value": raw}, "value"))
			break
		}
	}
	if apiKeyValue != "***" {
		t.Fatalf("expected masked X-API-Key header, got %q headers=%#v", apiKeyValue, headers)
	}
}

func TestE2ERetentionCleanupDeletesOldLogs(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte("v006-retention-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v006-retention"
	routePath := "/v006/retention"
	cfg := v006Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)

	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store for seed: %v", err)
	}
	oldCreatedAt := time.Now().UTC().Add(-72 * time.Hour)
	newCreatedAt := time.Now().UTC().Add(-10 * time.Minute)
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:         "v006-old-1",
			RouteID:    routeID,
			Method:     http.MethodGet,
			Path:       routePath,
			StatusCode: 500,
			CreatedAt:  oldCreatedAt,
		},
		{
			ID:         "v006-new-1",
			RouteID:    routeID,
			Method:     http.MethodGet,
			Path:       routePath,
			StatusCode: 200,
			CreatedAt:  newCreatedAt,
		},
	}); err != nil {
		_ = st.Close()
		t.Fatalf("seed audit logs: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	cleanup := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodDelete, "/admin/api/v1/audit-logs/cleanup?older_than_days=1&batch_size=10", nil, http.StatusOK))
	if deleted, ok := anyInt64(cleanup["deleted"]); !ok || deleted != 1 {
		t.Fatalf("expected deleted=1, got %#v", cleanup["deleted"])
	}

	search := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodGet, "/admin/api/v1/audit-logs?route="+routeID+"&limit=10", nil, http.StatusOK))
	entries, ok := search["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("expected exactly one audit entry after cleanup, got %#v", search["entries"])
	}
	row, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected audit entry payload: %#v", entries[0])
	}
	if anyString(row, "id") != "v006-new-1" {
		t.Fatalf("expected remaining entry id v006-new-1, got %#v", row["id"])
	}
}

func TestE2EAnalyticsTimeSeriesReturnsAggregatedData(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("v006-analytics-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v006-analytics"
	routePath := "/v006/analytics"
	cfg := v006Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)
	waitForGatewayListener(t, gwAddr)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v006-analytics@example.com",
		"name":            "V006 Analytics",
		"password":        "pass-123",
		"initial_credits": 50,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "v006-analytics-key",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "full_key")
	if liveKey == "" {
		t.Fatalf("expected full_key from API key create response")
	}

	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	for i := 0; i < 3; i++ {
		status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, liveKey)
		if status != http.StatusOK || body != "v006-analytics-ok" {
			t.Fatalf("unexpected gateway response status=%d body=%q", status, body)
		}
	}

	timeseries := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodGet, "/admin/api/v1/analytics/timeseries?window=10m&granularity=1m", nil, http.StatusOK))
	items, ok := timeseries["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty analytics timeseries, got %#v", timeseries["items"])
	}

	var totalRequests int64
	var totalCredits int64
	var totalStatus200 int64
	for _, itemRaw := range items {
		item, ok := itemRaw.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := anyInt64(item["requests"]); ok {
			totalRequests += value
		}
		if value, ok := anyInt64(item["credits_consumed"]); ok {
			totalCredits += value
		}
		statusCodes, ok := item["status_codes"].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := anyInt64(statusCodes["200"]); ok {
			totalStatus200 += value
		}
	}

	if totalRequests != 3 {
		t.Fatalf("expected total requests=3 got %d", totalRequests)
	}
	if totalCredits != 3 {
		t.Fatalf("expected total credits_consumed=3 got %d", totalCredits)
	}
	if totalStatus200 != 3 {
		t.Fatalf("expected total status 200 count=3 got %d", totalStatus200)
	}
}

func waitForAuditEntry(t *testing.T, adminAddr, adminKey, routeID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := asObject(t, adminJSONRequest(t, adminAddr, adminKey, http.MethodGet, "/admin/api/v1/audit-logs?route="+routeID+"&limit=10", nil, http.StatusOK))
		entries, ok := resp["entries"].([]any)
		if ok && len(entries) > 0 {
			row, ok := entries[0].(map[string]any)
			if ok {
				return row
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for audit entry for route %s", routeID)
	return nil
}

func v006Config(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	t.Helper()
	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			Addr:        adminAddr,
			APIKey:      "secret-v006",
			TokenSecret: "secret-v006-token",
			TokenTTL:    1 * time.Hour,
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/e2e-v006.db",
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Billing: config.BillingConfig{
			Enabled:           true,
			DefaultCost:       1,
			ZeroBalanceAction: "reject",
			TestModeEnabled:   true,
		},
		Audit: config.AuditConfig{
			Enabled:              true,
			BufferSize:           256,
			BatchSize:            1,
			FlushInterval:        10 * time.Millisecond,
			RetentionDays:        30,
			CleanupInterval:      time.Hour,
			CleanupBatchSize:     100,
			StoreRequestBody:     true,
			StoreResponseBody:    true,
			MaxRequestBodyBytes:  4096,
			MaxResponseBodyBytes: 4096,
			MaskHeaders:          []string{"X-API-Key", "Authorization"},
			MaskBodyFields:       []string{"password", "token"},
			MaskReplacement:      "***",
		},
		Services: []config.Service{
			{ID: "svc-v006", Name: "svc-v006", Protocol: "http", Upstream: "up-v006"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-v006",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet, http.MethodPost},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-v006",
				Name:      "up-v006",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-v006-t1", Address: upstreamHost, Weight: 1},
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
			{Name: "auth-apikey"},
		},
	}
}
