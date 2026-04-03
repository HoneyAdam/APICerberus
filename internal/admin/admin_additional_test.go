package admin

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/logging"
)

// Test WebSocketHub basic operations
func TestWebSocketHub_New(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	if hub == nil {
		t.Fatal("NewWebSocketHub returned nil")
	}
	if hub.connections == nil {
		t.Error("connections map not initialized")
	}
	if hub.subscribers == nil {
		t.Error("subscribers map not initialized")
	}
}

func TestWebSocketHub_RegisterAfterStop(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	hub.Stop()

	// Create a mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Register should close connection and return nil
	wsConn := hub.Register(server, []string{"test-topic"})
	if wsConn != nil {
		t.Error("Register should return nil after Stop")
	}
}

func TestWebSocketPoolManager(t *testing.T) {
	pm := NewWebSocketPoolManager()

	// Get pool for topic
	pool := pm.GetPool("test-topic")
	if pool == nil {
		t.Fatal("GetPool returned nil")
	}

	// Get buffer from pool
	buf := pm.GetBuffer("test-topic")
	if buf == nil {
		t.Error("GetBuffer returned nil")
	}

	// Return buffer to pool
	pm.PutBuffer("test-topic", buf)

	// Verify pool exists
	pool2 := pm.GetPool("test-topic")
	if pool2 != pool {
		t.Error("GetPool returned different pool for same topic")
	}
}

func TestWebSocketConn_Send(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	wsConn := &WebSocketConn{
		ID:        "test-conn",
		Conn:      server,
		Topics:    make(map[string]bool),
		CreatedAt: time.Now(),
		LastPing:  time.Now(),
		writeCh:   make(chan []byte, 64),
		hub:       hub,
	}

	// Test Send
	err := wsConn.Send([]byte("test message"))
	if err != nil {
		t.Errorf("Send error: %v", err)
	}

	// Close
	wsConn.close()
}

func TestWebSocketHub_GetMetrics(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	// Get metrics - just verify it doesn't panic
	metrics := hub.GetMetrics()
	if metrics.TotalConnections.Load() != 0 {
		t.Errorf("Initial TotalConnections = %d, want 0", metrics.TotalConnections.Load())
	}
}

// Test handler functions - focus on edge cases that work with existing test server
func TestGetService_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/services/nonexistent-service-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestGetRoute_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/routes/nonexistent-route-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestGetUpstream_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/upstreams/nonexistent-upstream-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestDeleteService_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/services/nonexistent-service-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestDeleteRoute_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/routes/nonexistent-route-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestDeleteUpstream_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/nonexistent-upstream-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestGetUpstreamHealth_NotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/upstreams/nonexistent-upstream-12345/health", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestDeleteUpstreamTarget_UpstreamNotFound(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	// Try to delete a target from non-existent upstream
	resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/upstreams/nonexistent-upstream/targets/target-1", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

// Helper functions tests
func TestAsString(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"float64", 3.14, "3.14"},
		{"bool", true, "true"},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asString(tt.value)
			if got != tt.expected {
				t.Errorf("asString(%v) = %q, want %q", tt.value, got, tt.expected)
			}
		})
	}
}

func TestGenerateConnID(t *testing.T) {
	id := generateConnID()
	if id == "" {
		t.Error("generateConnID returned empty string")
	}
	// Length should be reasonable (timestamp + hyphen + 8 chars = 15 + 1 + 8 = 24)
	if len(id) < 10 {
		t.Errorf("generateConnID returned ID of length %d, want at least 10", len(id))
	}
}

func TestRandomString(t *testing.T) {
	str := randomString(10)
	if len(str) != 10 {
		t.Errorf("randomString(10) returned string of length %d, want 10", len(str))
	}
}

// Test helper functions from server.go
func TestAsInt64(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		fallback int64
		expected int64
	}{
		{"int", 42, 0, 42},
		{"int64", int64(42), 0, 42},
		{"int32", int32(42), 0, 42},
		{"float64", float64(42.5), 0, 42},
		{"string valid", "42", 0, 42},
		{"string invalid", "abc", 99, 99},
		{"bool", true, 0, 0},
		{"nil", nil, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asInt64(tt.value, tt.fallback)
			if got != tt.expected {
				t.Errorf("asInt64(%v, %d) = %d, want %d", tt.value, tt.fallback, got, tt.expected)
			}
		})
	}
}

func TestAsFloat64(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		wantVal  float64
		wantOk   bool
	}{
		{"float64", float64(3.14), 3.14, true},
		{"int", 42, 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"string valid", "3.14", 3.14, true},
		{"string invalid", "abc", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asFloat64(tt.value)
			if ok != tt.wantOk {
				t.Errorf("asFloat64(%v) ok = %v, want %v", tt.value, ok, tt.wantOk)
				return
			}
			if ok && got != tt.wantVal {
				t.Errorf("asFloat64(%v) = %f, want %f", tt.value, got, tt.wantVal)
			}
		})
	}
}

func TestParseBoolString(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantVal bool
		wantErr bool
	}{
		{"true lowercase", "true", true, false},
		{"true uppercase", "TRUE", true, false},
		{"true mixed", "True", true, false},
		{"1", "1", true, false},
		{"yes", "yes", true, false},
		{"false", "false", false, false},
		{"0", "0", false, false},
		{"no", "no", false, false},
		{"empty", "", false, true},
		{"random", "random", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBoolString(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBoolString(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if got != tt.wantVal {
				t.Errorf("parseBoolString(%q) = %v, want %v", tt.value, got, tt.wantVal)
			}
		})
	}
}

// Test WebSocketPoolManager
func TestWebSocketPoolManager_GetBuffer(t *testing.T) {
	pm := NewWebSocketPoolManager()

	// Get buffer for topic
	buf := pm.GetBuffer("test-topic")
	if buf == nil {
		t.Error("GetBuffer returned nil")
	}

	// Put buffer back
	pm.PutBuffer("test-topic", buf)

	// Get pool for topic should return same pool
	pool1 := pm.GetPool("test-topic")
	pool2 := pm.GetPool("test-topic")
	if pool1 != pool2 {
		t.Error("GetPool returned different pools for same topic")
	}
}

// Test WebSocketHub basic operations
func TestWebSocketHub_SubscribeUnsubscribe(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	// Create a mock connection with proper initialization
	server, _ := net.Pipe()
	hub.mu.Lock()
	wsConn := &WebSocketConn{
		ID:     "test-conn-1",
		Conn:   server,
		Topics: make(map[string]bool),
		hub:    hub,
		writeCh: make(chan []byte, 64),
	}
	hub.connections["test-conn-1"] = wsConn
	hub.mu.Unlock()

	// Subscribe
	hub.Subscribe("test-conn-1", "test-topic")

	// Check subscription
	hub.mu.RLock()
	subs, ok := hub.subscribers["test-topic"]["test-conn-1"]
	hub.mu.RUnlock()
	if !ok || !subs {
		t.Error("Subscription not recorded")
	}

	// Unsubscribe
	hub.Unsubscribe("test-conn-1", "test-topic")

	// Check unsubscription
	hub.mu.RLock()
	_, exists := hub.subscribers["test-topic"]["test-conn-1"]
	hub.mu.RUnlock()
	if exists {
		t.Error("Unsubscription not recorded")
	}
}

func TestWebSocketHub_Broadcast(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	// Create a simple event
	event := realtimeEvent{
		Type:    "test",
		Payload: map[string]any{"key": "value"},
	}

	// Broadcast should not panic
	hub.Broadcast("test-topic", event)

	// BroadcastExcept should not panic
	hub.BroadcastExcept("test-topic", event, "excluded-conn")
}

func TestWebSocketHub_Unregister(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	// Unregister should not block even if connection doesn't exist
	hub.Unregister("non-existent-conn")
}

func TestWebSocketHub_Stop(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)

	// Stop should not panic
	hub.Stop()

	// Stop again should be safe
	hub.Stop()
}

// Test asInt function
func TestAsInt(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		fallback int
		expected int
	}{
		{"int", 42, 0, 42},
		{"int64", int64(42), 0, 42},
		{"int32", int32(42), 0, 42},
		{"float64", float64(42.5), 0, 42},
		{"string valid", "42", 0, 42},
		{"string invalid", "abc", 99, 99},
		{"json.Number", json.Number("42"), 99, 99}, // json.Number not handled, returns fallback
		{"bool", true, 0, 0},
		{"nil", nil, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asInt(tt.value, tt.fallback)
			if got != tt.expected {
				t.Errorf("asInt(%v, %d) = %d, want %d", tt.value, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Test parseAuditTime function
func TestParseAuditTime(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantZero bool
	}{
		{"empty string", "", true},
		{"now", "now", true},        // "now" is not a valid RFC3339 format
		{"1h ago", "1h", true},      // relative time not supported
		{"1d ago", "1d", true},      // relative time not supported
		{"RFC3339 timestamp", "2024-01-15T10:30:00Z", false},
		{"invalid", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := parseAuditTime(tt.value)
			if tt.wantZero && !got.IsZero() {
				t.Errorf("parseAuditTime(%q) = %v, want zero time", tt.value, got)
			}
			if !tt.wantZero && got.IsZero() {
				t.Errorf("parseAuditTime(%q) = zero time, want non-zero", tt.value)
			}
		})
	}
}

// Test Analytics Endpoints
func TestAnalyticsTopRoutes(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)

	// Test with custom limit
	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?limit=5", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsTopRoutes_InvalidLimit(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test with invalid limit
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?limit=invalid", "secret-admin", nil)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestAnalyticsErrors(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/errors", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsTopConsumers(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-consumers", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsLatency(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/latency", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsThroughput(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/throughput", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsStatusCodes(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/status-codes", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsOverview(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/overview", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestAnalyticsTimeSeries(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test without time range - should use defaults
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/timeseries", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

// Test update handlers - focus on error cases
func TestUpdateService_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test with wrong HTTP method - API returns 404 for non-existent service
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/services/test-service", "secret-admin", nil)
	// Router returns 404 for non-existent routes with wrong method
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 404 or 405, got %v", statusCode)
	}
}

func TestUpdateRoute_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/routes/test-route", "secret-admin", nil)
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 404 or 405, got %v", statusCode)
	}
}

func TestUpdateUpstream_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams/test-upstream", "secret-admin", nil)
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 404 or 405, got %v", statusCode)
	}
}

func TestUpdateService_NotFound(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	body := map[string]any{
		"name":     "updated-service",
		"upstream": "test-upstream",
	}

	resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/services/nonexistent-service-12345", "secret-admin", body)
	// API validates body first, returns 400 if invalid, or 404 if not found
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400, got %v", statusCode)
	}
}

func TestUpdateRoute_NotFound(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	body := map[string]any{
		"name":    "updated-route",
		"service": "test-service",
	}

	resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/routes/nonexistent-route-12345", "secret-admin", body)
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400, got %v", statusCode)
	}
}

func TestUpdateUpstream_NotFound(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	body := map[string]any{
		"name":    "updated-upstream",
		"targets": []map[string]any{{"address": "localhost:8080"}},
	}

	resp := mustJSONRequest(t, http.MethodPut, baseURL+"/admin/api/v1/upstreams/nonexistent-upstream-12345", "secret-admin", body)
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400, got %v", statusCode)
	}
}

// Test delete alert
func TestDeleteAlert_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Test with wrong HTTP method
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/alerts/alert-123", "secret-admin", nil)
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 404 or 405, got %v", statusCode)
	}
}

func TestDeleteAlert_NotFound(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/alerts/nonexistent-alert-12345", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

// Test credit overview
func TestCreditOverviewEndpoint(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/credits/overview", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

// Test user audit logs endpoint
func TestSearchUserAuditLogsEndpoint(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// API returns empty results for non-existent users
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/users/nonexistent-user-12345/audit-logs", "secret-admin", nil)
	// May return 404 or 200 with empty results
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusOK && statusCode != http.StatusNotFound {
		t.Errorf("Expected 200 or 404, got %v", statusCode)
	}
}

// Test config import with invalid JSON
func TestConfigImport_InvalidJSON(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Send invalid JSON
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/admin/api/v1/config/import", strings.NewReader("not valid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "secret-admin")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

// Test WebSocketHub Register with closed hub
func TestWebSocketHub_Register_Closed(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)

	// Stop the hub
	hub.Stop()

	// Create mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Try to register after stop - should return nil
	wsConn := hub.Register(server, []string{"test-topic"})
	if wsConn != nil {
		t.Error("Register should return nil when hub is stopped")
	}
}

// Test dashboardAssetExists
func TestDashboardAssetExists(t *testing.T) {
	// Test with nil filesystem
	exists := dashboardAssetExists(nil, "/test.html")
	if exists {
		t.Error("dashboardAssetExists should return false for nil filesystem")
	}
}

// Test handleRealtimeWebSocket errors
func TestHandleRealtimeWebSocket_NotFound(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	// Try to access WebSocket endpoint without proper upgrade
	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/ws", "secret-admin", nil)
	// Will likely return 404 or 400 since it's not a WebSocket request
	statusCode := int(resp["status_code"].(float64))
	if statusCode != http.StatusNotFound && statusCode != http.StatusBadRequest && statusCode != http.StatusUpgradeRequired {
		t.Errorf("Expected 404, 400, or 426 for non-WebSocket request, got %d", statusCode)
	}
}

// Test metricSignature function
func TestMetricSignature(t *testing.T) {
	tests := []struct {
		name    string
		metric  analytics.RequestMetric
		wantSig string
	}{
		{
			name: "basic metric",
			metric: analytics.RequestMetric{
				Timestamp:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				RouteID:    "test-route",
				Path:       "/api/test",
				Method:     "GET",
				StatusCode: 200,
				LatencyMS:  100,
				BytesOut:   1024,
			},
			wantSig: "1704067200000000000|test-route|/api/test|GET|200|100|1024",
		},
		{
			name: "metric with spaces",
			metric: analytics.RequestMetric{
				Timestamp:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				RouteID:    "  test-route  ",
				Path:       "  /api/test  ",
				Method:     "  POST  ",
				StatusCode: 201,
				LatencyMS:  50,
				BytesOut:   512,
			},
			wantSig: "1704067200000000000|test-route|/api/test|POST|201|50|512",
		},
		{
			name: "empty metric",
			metric: analytics.RequestMetric{
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantSig: "1704067200000000000||||0|0|0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricSignature(tt.metric)
			if got != tt.wantSig {
				t.Errorf("metricSignature() = %v, want %v", got, tt.wantSig)
			}
		})
	}
}

// Test handleRegister and handleUnregister
func TestWebSocketHub_HandleRegisterUnregister(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	// Create a mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Register connection
	wsConn := hub.Register(server, []string{"topic1", "topic2"})
	if wsConn == nil {
		t.Fatal("Register returned nil")
	}

	// Wait for registration to be processed
	time.Sleep(50 * time.Millisecond)

	// Check connection is tracked
	hub.mu.RLock()
	if _, exists := hub.connections[wsConn.ID]; !exists {
		hub.mu.RUnlock()
		t.Error("Connection should be registered")
	} else {
		hub.mu.RUnlock()
	}

	// Check subscriptions
	hub.mu.RLock()
	if subs, exists := hub.subscribers["topic1"]; !exists || !subs[wsConn.ID] {
		hub.mu.RUnlock()
		t.Error("Should be subscribed to topic1")
	} else {
		hub.mu.RUnlock()
	}

	// Unregister connection
	hub.Unregister(wsConn.ID)

	// Wait for unregistration to be processed
	time.Sleep(50 * time.Millisecond)

	// Check connection is removed
	hub.mu.RLock()
	if _, exists := hub.connections[wsConn.ID]; exists {
		hub.mu.RUnlock()
		t.Error("Connection should be unregistered")
	} else {
		hub.mu.RUnlock()
	}
}

// Test handleUnregister for non-existent connection
func TestWebSocketHub_HandleUnregister_NonExistent(t *testing.T) {
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	// Should not panic when unregistering non-existent connection
	hub.Unregister("non-existent-id")

	// Wait for unregistration to be processed
	time.Sleep(50 * time.Millisecond)
}

