package admin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestAdminAuthMiddleware(t *testing.T) {
	t.Parallel()

	serverURL, _, _ := newAdminTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, serverURL+"/admin/api/v1/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", resp.StatusCode)
	}
}

func TestAdminDashboardSPAFallback(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)
	status, body, _ := mustRawRequest(t, http.MethodGet, baseURL+"/", "")
	if status != http.StatusOK {
		t.Fatalf("expected GET / to return 200, got %d body=%q", status, body)
	}
	if !strings.Contains(body, "<div id=\"app\"></div>") {
		t.Fatalf("expected dashboard index shell, got body=%q", body)
	}

	status, body, _ = mustRawRequest(t, http.MethodGet, baseURL+"/services", "")
	if status != http.StatusOK {
		t.Fatalf("expected SPA fallback to return 200, got %d body=%q", status, body)
	}
	if !strings.Contains(body, "<div id=\"app\"></div>") {
		t.Fatalf("expected SPA fallback body, got %q", body)
	}
}

func TestAdminRealtimeWebSocketEndpoint(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	statusLine, conn, reader := performWebSocketHandshake(t, baseURL, "secret-admin")
	defer conn.Close()

	if !strings.Contains(statusLine, "101") {
		t.Fatalf("expected websocket upgrade status 101, got %q", strings.TrimSpace(statusLine))
	}

	frame, err := readWebSocketFramePayload(reader)
	if err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(frame, &event); err != nil {
		t.Fatalf("unmarshal websocket frame: %v payload=%q", err, string(frame))
	}
	if got := strings.TrimSpace(asString(event["type"])); got != "connected" {
		t.Fatalf("expected first websocket event type=connected got=%q payload=%s", got, string(frame))
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	foundLiveEvent := false
	for i := 0; i < 6; i++ {
		nextFrame, err := readWebSocketFramePayload(reader)
		if err != nil {
			break
		}
		var nextEvent map[string]any
		if err := json.Unmarshal(nextFrame, &nextEvent); err != nil {
			t.Fatalf("unmarshal websocket frame: %v payload=%q", err, string(nextFrame))
		}
		eventType := strings.TrimSpace(asString(nextEvent["type"]))
		if eventType == "health_change" || eventType == "request_metric" {
			foundLiveEvent = true
			break
		}
	}
	if !foundLiveEvent {
		t.Fatalf("expected websocket to emit realtime event after connected frame")
	}
}

func TestAdminRealtimeWebSocketRejectsUnauthorized(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)
	statusLine, conn, _ := performWebSocketHandshake(t, baseURL, "")
	defer conn.Close()

	if !strings.Contains(statusLine, "401") {
		t.Fatalf("expected websocket unauthorized status 401, got %q", strings.TrimSpace(statusLine))
	}
}

func TestAdminAlertsCRUD(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	createPayload := map[string]any{
		"name":      "error spike",
		"enabled":   true,
		"type":      "error_rate",
		"threshold": 10,
		"window":    "5m",
		"cooldown":  "1m",
		"action": map[string]any{
			"type": "log",
		},
	}
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/alerts", "secret-admin", createPayload)
	assertStatus(t, resp, http.StatusCreated)
	ruleID := jsonObjectField(t, resp, "id")
	if strings.TrimSpace(ruleID) == "" {
		t.Fatalf("expected created alert id")
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/alerts", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "rules")
	assertHasJSONField(t, resp, "history")

	updatePayload := map[string]any{
		"name":      "error spike",
		"enabled":   false,
		"type":      "error_rate",
		"threshold": 12,
		"window":    "5m",
		"cooldown":  "1m",
		"action": map[string]any{
			"type": "log",
		},
	}
	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/alerts/"+ruleID, "secret-admin", updatePayload)
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "enabled", false)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/alerts/"+ruleID, "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)
}

func TestAdminEndpointsIntegration(t *testing.T) {
	t.Parallel()

	baseURL, upstreamURL, storePath := newAdminTestServer(t)

	// status
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/status", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "status", "ok")

	// info
	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/info", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "version")

	// services list
	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/services", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertJSONArrayLenAtLeast(t, resp, 1)

	// create service
	servicePayload := map[string]any{
		"id":       "svc-orders",
		"name":     "svc-orders",
		"protocol": "http",
		"upstream": "up-users",
	}
	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/services", "secret-admin", servicePayload)
	assertStatus(t, resp, http.StatusCreated)

	// get/update/delete service
	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/services/svc-orders", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	servicePayload["name"] = "svc-orders-v2"
	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/services/svc-orders", "secret-admin", servicePayload)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/services/svc-orders", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)

	// routes CRUD
	routePayload := map[string]any{
		"id":      "route-extra",
		"name":    "route-extra",
		"service": "svc-users",
		"paths":   []string{"/extra"},
		"methods": []string{"GET"},
	}
	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/routes", "secret-admin", routePayload)
	assertStatus(t, resp, http.StatusCreated)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/routes/route-extra", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	routePayload["paths"] = []string{"/extra-v2"}
	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/routes/route-extra", "secret-admin", routePayload)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/routes/route-extra", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)

	// upstream CRUD
	upstreamHost := mustHost(t, upstreamURL)
	upstreamPayload := map[string]any{
		"id":        "up-extra",
		"name":      "up-extra",
		"algorithm": "round_robin",
		"targets": []map[string]any{
			{
				"id":      "up-extra-t1",
				"address": upstreamHost,
				"weight":  1,
			},
		},
		"health_check": map[string]any{
			"active": map[string]any{
				"path":                "/health",
				"interval":            int64(time.Second),
				"timeout":             int64(time.Second),
				"healthy_threshold":   1,
				"unhealthy_threshold": 1,
			},
		},
	}
	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams", "secret-admin", upstreamPayload)
	assertStatus(t, resp, http.StatusCreated)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/upstreams/up-extra", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	upstreamPayload["algorithm"] = "weighted_round_robin"
	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/upstreams/up-extra", "secret-admin", upstreamPayload)
	assertStatus(t, resp, http.StatusOK)

	// target management
	targetPayload := map[string]any{
		"id":      "up-extra-t2",
		"address": upstreamHost,
		"weight":  2,
	}
	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams/up-extra/targets", "secret-admin", targetPayload)
	assertStatus(t, resp, http.StatusCreated)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/upstreams/up-extra/health", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "targets")

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/up-extra/targets/up-extra-t2", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/up-extra", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)

	// reload endpoint
	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/config/reload", "secret-admin", map[string]any{})
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "reloaded", true)

	// config export/import endpoints
	status, exportedConfig, headers := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/config/export", "secret-admin")
	if status != http.StatusOK {
		t.Fatalf("expected config export status 200 got %d body=%q", status, exportedConfig)
	}
	if !strings.Contains(headers.Get("Content-Type"), "application/x-yaml") {
		t.Fatalf("unexpected config export content type: %s", headers.Get("Content-Type"))
	}
	if !strings.Contains(exportedConfig, "services:") {
		t.Fatalf("expected exported config to include services section")
	}

	importedConfig := strings.Join([]string{
		"gateway:",
		"  http_addr: 127.0.0.1:0",
		"admin:",
		"  api_key: secret-admin",
		"store:",
		"  path: " + storePath,
		"services:",
		"  -",
		"    id: svc-users",
		"    name: svc-users",
		"    protocol: http",
		"    upstream: up-users",
		"routes:",
		"  -",
		"    id: route-users",
		"    name: route-users",
		"    service: svc-users",
		"    paths:",
		"      - /users-imported",
		"    methods:",
		"      - GET",
		"upstreams:",
		"  -",
		"    id: up-users",
		"    name: up-users",
		"    algorithm: round_robin",
		"    targets:",
		"      -",
		"        id: up-users-t1",
		"        address: " + mustHost(t, upstreamURL),
		"        weight: 1",
		"",
	}, "\n")
	status, importBody, _ := mustRawRequestWithBody(t, http.MethodPost, baseURL+"/admin/api/v1/config/import", "secret-admin", "application/x-yaml", []byte(importedConfig))
	if status != http.StatusOK {
		t.Fatalf("expected config import status 200 got %d body=%q", status, importBody)
	}
	if !strings.Contains(importBody, "\"imported\":true") {
		t.Fatalf("expected import response to contain imported=true, got %q", importBody)
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/routes/route-users", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	routeBody, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("route response body is not object: %#v", resp)
	}
	paths, ok := routeBody["paths"].([]any)
	if !ok || len(paths) == 0 {
		t.Fatalf("route response paths missing: %#v", routeBody)
	}
	if got := strings.TrimSpace(asString(paths[0])); got != "/users-imported" {
		t.Fatalf("expected imported route path /users-imported got %q", got)
	}

	// user and credit endpoints
	userPayload := map[string]any{
		"email":           "user-one@example.com",
		"name":            "User One",
		"password":        "user-one-pass",
		"initial_credits": 100,
	}
	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users", "secret-admin", userPayload)
	assertStatus(t, resp, http.StatusCreated)
	userID := jsonObjectField(t, resp, "id")
	if userID == "" {
		t.Fatalf("expected created user id in response")
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "Users")
	assertHasJSONField(t, resp, "Total")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID, "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/users/"+userID, "secret-admin", map[string]any{
		"name": "User One Updated",
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/suspend", "secret-admin", map[string]any{})
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "status", "suspended")

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/activate", "secret-admin", map[string]any{})
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "status", "active")

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/reset-password", "secret-admin", map[string]any{
		"password": "user-one-pass-new",
	})
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "password_reset", true)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/api-keys", "secret-admin", map[string]any{
		"name": "integration-key",
		"mode": "test",
	})
	assertStatus(t, resp, http.StatusCreated)
	apiKeyID := nestedObjectField(t, resp, "key", "id")
	if apiKeyID == "" {
		t.Fatalf("expected API key id in response")
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID+"/api-keys", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/"+userID+"/api-keys/"+apiKeyID, "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/permissions", "secret-admin", map[string]any{
		"route_id": "route-users",
		"methods":  []string{"GET"},
		"allowed":  true,
	})
	assertStatus(t, resp, http.StatusCreated)
	permissionID := jsonObjectField(t, resp, "id")
	if permissionID == "" {
		t.Fatalf("expected permission id in response")
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID+"/permissions", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/users/"+userID+"/permissions/"+permissionID, "secret-admin", map[string]any{
		"route_id": "route-users",
		"methods":  []string{"GET", "POST"},
		"allowed":  true,
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/permissions/bulk", "secret-admin", map[string]any{
		"permissions": []map[string]any{
			{
				"route_id": "route-users",
				"methods":  []string{"GET"},
				"allowed":  true,
			},
		},
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/"+userID+"/permissions/"+permissionID, "secret-admin", nil)
	assertStatus(t, resp, http.StatusNoContent)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID+"/ip-whitelist", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/ip-whitelist", "secret-admin", map[string]any{
		"ips": []string{"203.0.113.10"},
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/users/"+userID+"/ip-whitelist/203.0.113.10", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/credits/overview", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/credits/topup", "secret-admin", map[string]any{
		"amount": 25,
		"reason": "test topup",
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users/"+userID+"/credits/deduct", "secret-admin", map[string]any{
		"amount": 10,
		"reason": "test deduct",
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID+"/credits/balance", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "balance", float64(115))

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID+"/credits/transactions", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "Transactions")

	seedStore, err := store.Open(&config.Config{
		Store: config.StoreConfig{
			Path:        storePath,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	})
	if err != nil {
		t.Fatalf("open seed store for audit logs: %v", err)
	}
	oldCreatedAt := time.Now().UTC().Add(-72 * time.Hour)
	newCreatedAt := time.Now().UTC().Add(-2 * time.Minute)
	if err := seedStore.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:           "audit-old-1",
			RequestID:    "req-old-1",
			RouteID:      "route-users",
			RouteName:    "route-users",
			ServiceName:  "svc-users",
			UserID:       userID,
			ConsumerName: "User One Updated",
			Method:       "GET",
			Path:         "/users",
			StatusCode:   500,
			LatencyMS:    120,
			ClientIP:     "203.0.113.10",
			Blocked:      true,
			BlockReason:  "rate_limit",
			RequestBody:  `{"query":"old"}`,
			ResponseBody: `{"error":"old timeout"}`,
			CreatedAt:    oldCreatedAt,
		},
		{
			ID:           "audit-new-1",
			RequestID:    "req-new-1",
			RouteID:      "route-users",
			RouteName:    "route-users",
			ServiceName:  "svc-users",
			UserID:       userID,
			ConsumerName: "User One Updated",
			Method:       "POST",
			Path:         "/users",
			StatusCode:   200,
			LatencyMS:    30,
			ClientIP:     "203.0.113.10",
			RequestBody:  `{"query":"new"}`,
			ResponseBody: `{"ok":true}`,
			CreatedAt:    newCreatedAt,
		},
	}); err != nil {
		_ = seedStore.Close()
		t.Fatalf("seed audit logs: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs?route=route-users&status_min=200", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "entries")
	auditID := firstAuditEntryID(t, resp)
	if auditID == "" {
		t.Fatalf("expected at least one audit entry id")
	}

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/"+auditID, "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "request_id")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/"+userID+"/audit-logs", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "entries")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/stats?route=route-users", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "total_requests")

	status, body, headers := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/audit-logs/export?format=jsonl&route=route-users", "secret-admin")
	if status != http.StatusOK {
		t.Fatalf("expected export status 200 got %d body=%q", status, body)
	}
	if !strings.Contains(body, "\"request_id\":\"req-") {
		t.Fatalf("expected jsonl export payload to include request ids, got %q", body)
	}
	if !strings.Contains(headers.Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("unexpected export content type: %s", headers.Get("Content-Type"))
	}

	resp = mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/audit-logs/cleanup?older_than_days=1&batch_size=10", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "deleted", float64(1))

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/overview?window=1h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "total_requests")
	assertHasJSONField(t, resp, "active_conns")
	assertHasJSONField(t, resp, "credits_consumed")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/timeseries?window=1h&granularity=1m", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "items")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?window=1h&limit=5", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "routes")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-consumers?window=1h&limit=5", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "consumers")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/errors?window=1h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "breakdown")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/latency?window=1h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "p95_latency_ms")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/throughput?window=1h&granularity=1m", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "items")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/status-codes?window=1h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "status_codes")

	// billing endpoints
	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/billing/config", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "default_cost")

	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/config", "secret-admin", map[string]any{
		"enabled":             true,
		"default_cost":        3,
		"zero_balance_action": "reject",
		"method_multipliers": map[string]any{
			"POST": 2.0,
		},
	})
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "enabled", true)
	assertJSONField(t, resp, "default_cost", float64(3))

	resp = mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/billing/route-costs", "secret-admin", map[string]any{
		"route_costs": map[string]any{
			"route-users": 7,
		},
	})
	assertStatus(t, resp, http.StatusOK)

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/billing/route-costs", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertNestedJSONField(t, resp, "route_costs", "route-users", float64(7))
}

func newAdminTestServer(t *testing.T) (adminBaseURL string, upstreamURL string, storePath string) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	storePath = t.TempDir() + "/admin-api-test.db"
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			APIKey:    "secret-admin",
			UIEnabled: true,
		},
		Store: config.StoreConfig{
			Path:        storePath,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Services: []config.Service{
			{
				ID:       "svc-users",
				Name:     "svc-users",
				Protocol: "http",
				Upstream: "up-users",
			},
		},
		Routes: []config.Route{
			{
				ID:      "route-users",
				Name:    "route-users",
				Service: "svc-users",
				Paths:   []string{"/users"},
				Methods: []string{http.MethodGet},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-users",
				Name:      "up-users",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{
						ID:      "up-users-t1",
						Address: mustHost(t, upstream.URL),
						Weight:  1,
					},
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
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}
	adminSrv, err := NewServer(cfg, gw)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(adminSrv)
	t.Cleanup(httpSrv.Close)
	t.Cleanup(func() {
		_ = gw.Shutdown(context.Background())
	})

	return httpSrv.URL, upstream.URL, storePath
}

func performWebSocketHandshake(t *testing.T, baseURL, apiKey string) (string, net.Conn, *bufio.Reader) {
	t.Helper()

	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	path := "/admin/api/v1/ws"
	if strings.TrimSpace(apiKey) != "" {
		query := url.Values{}
		query.Set("api_key", apiKey)
		path += "?" + query.Encode()
	}

	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatalf("dial websocket endpoint: %v", err)
	}
	reader := bufio.NewReader(conn)

	requestLines := []string{
		"GET " + path + " HTTP/1.1",
		"Host: " + parsed.Host,
		"Origin: http://" + parsed.Host,
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==",
		"Sec-WebSocket-Version: 13",
		"",
		"",
	}
	if _, err := conn.Write([]byte(strings.Join(requestLines, "\r\n"))); err != nil {
		_ = conn.Close()
		t.Fatalf("write websocket handshake request: %v", err)
	}

	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket status line: %v", err)
	}
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			_ = conn.Close()
			t.Fatalf("read websocket headers: %v", readErr)
		}
		if line == "\r\n" {
			break
		}
	}

	return statusLine, conn, reader
}

func readWebSocketFramePayload(reader *bufio.Reader) ([]byte, error) {
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	opcode := firstByte & 0x0F
	if opcode != 0x1 {
		return nil, io.ErrUnexpectedEOF
	}

	secondByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	masked := secondByte&0x80 != 0
	length := uint64(secondByte & 0x7F)
	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return nil, err
		}
		length = uint64(extended[0])<<8 | uint64(extended[1])
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return nil, err
		}
		length = uint64(extended[0])<<56 |
			uint64(extended[1])<<48 |
			uint64(extended[2])<<40 |
			uint64(extended[3])<<32 |
			uint64(extended[4])<<24 |
			uint64(extended[5])<<16 |
			uint64(extended[6])<<8 |
			uint64(extended[7])
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(reader, maskKey[:]); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return payload, nil
}

func mustJSONRequest(t *testing.T, method, rawURL, adminKey string, payload any) map[string]any {
	t.Helper()

	var bodyBytes []byte
	if payload != nil {
		var err error
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("json marshal: %v", err)
		}
	}

	req, err := http.NewRequest(method, rawURL, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Admin-Key", adminKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	result := map[string]any{
		"status_code": float64(resp.StatusCode),
	}
	if resp.ContentLength == 0 || resp.StatusCode == http.StatusNoContent {
		return result
	}

	var body any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	result["body"] = body
	return result
}

func mustRawRequest(t *testing.T, method, rawURL, adminKey string) (int, string, http.Header) {
	t.Helper()

	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Admin-Key", adminKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read raw response body: %v", err)
	}
	return resp.StatusCode, body.String(), resp.Header.Clone()
}

func mustRawRequestWithBody(t *testing.T, method, rawURL, adminKey, contentType string, payload []byte) (int, string, http.Header) {
	t.Helper()

	req, err := http.NewRequest(method, rawURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Admin-Key", adminKey)
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read raw response body: %v", err)
	}
	return resp.StatusCode, body.String(), resp.Header.Clone()
}

func firstAuditEntryID(t *testing.T, resp map[string]any) string {
	t.Helper()

	body, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("response body is not object: %#v", resp)
	}
	entries, ok := body["entries"].([]any)
	if !ok || len(entries) == 0 {
		return ""
	}
	first, ok := entries[0].(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := first["id"].(string); ok {
		return id
	}
	if id, ok := first["ID"].(string); ok {
		return id
	}
	return ""
}

func assertStatus(t *testing.T, resp map[string]any, want int) {
	t.Helper()
	got := int(resp["status_code"].(float64))
	if got != want {
		t.Fatalf("expected status %d got %d (resp=%#v)", want, got, resp)
	}
}

func assertJSONField(t *testing.T, resp map[string]any, key string, want any) {
	t.Helper()
	body, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("response body is not object: %#v", resp)
	}
	if body[key] != want {
		t.Fatalf("expected body[%q]=%v got %v (body=%#v)", key, want, body[key], body)
	}
}

func assertHasJSONField(t *testing.T, resp map[string]any, key string) {
	t.Helper()
	body, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("response body is not object: %#v", resp)
	}
	if _, exists := body[key]; !exists {
		t.Fatalf("expected field %q in body %#v", key, body)
	}
}

func assertNestedJSONField(t *testing.T, resp map[string]any, parentKey, childKey string, want any) {
	t.Helper()
	body, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("response body is not object: %#v", resp)
	}
	parent, ok := body[parentKey].(map[string]any)
	if !ok {
		t.Fatalf("body[%q] is not object: %#v", parentKey, body[parentKey])
	}
	if parent[childKey] != want {
		t.Fatalf("expected body[%q][%q]=%v got %v", parentKey, childKey, want, parent[childKey])
	}
}

func jsonObjectField(t *testing.T, resp map[string]any, key string) string {
	t.Helper()
	body, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("response body is not object: %#v", resp)
	}
	if value, ok := body[key].(string); ok && value != "" {
		return value
	}
	if key == "id" {
		if value, ok := body["ID"].(string); ok {
			return value
		}
	}
	return ""
}

func nestedObjectField(t *testing.T, resp map[string]any, parentKey, key string) string {
	t.Helper()
	body, ok := resp["body"].(map[string]any)
	if !ok {
		t.Fatalf("response body is not object: %#v", resp)
	}
	parent, ok := body[parentKey].(map[string]any)
	if !ok {
		t.Fatalf("response body[%q] is not object: %#v", parentKey, body[parentKey])
	}
	if value, ok := parent[key].(string); ok && value != "" {
		return value
	}
	if key == "id" {
		if value, ok := parent["ID"].(string); ok {
			return value
		}
	}
	return ""
}

func assertJSONArrayLenAtLeast(t *testing.T, resp map[string]any, min int) {
	t.Helper()
	body, ok := resp["body"].([]any)
	if !ok {
		t.Fatalf("response body is not array: %#v", resp)
	}
	if len(body) < min {
		t.Fatalf("expected array len >= %d got %d", min, len(body))
	}
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u.Host
}
