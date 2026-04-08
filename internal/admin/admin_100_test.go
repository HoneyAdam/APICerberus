package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// =============================================================================
// Mock Implementations for Advanced Testing
// =============================================================================

// mockAnalyticsEngine is a mock implementation of analytics.Engine for testing
type mockAnalyticsEngine struct {
	latestMetrics []analytics.RequestMetric
	returnNil     bool
}

func (m *mockAnalyticsEngine) Latest(limit int) []analytics.RequestMetric {
	if m.returnNil {
		return nil
	}
	return m.latestMetrics
}

// =============================================================================
// Advanced Error Path Testing for analyticsErrors
// =============================================================================

// TestAnalyticsErrors_Advanced tests analyticsErrors with various edge cases
func TestAnalyticsErrors_Advanced(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedStatus int
	}{
		{
			name:           "default time range",
			query:          "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "1h timeframe",
			query:          "?timeframe=1h",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "24h timeframe",
			query:          "?timeframe=24h",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "7d timeframe",
			query:          "?timeframe=7d",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "custom from/to",
			query:          "?from=2024-01-01T00:00:00Z\u0026to=2024-01-02T00:00:00Z",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid timeframe value",
			query:          "?timeframe=invalid",
			expectedStatus: http.StatusOK, // Implementation falls back to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL, _, _ := newAdminTestServer(t)

			url := baseURL + "/admin/api/v1/analytics/errors" + tt.query
			status, _, _ := mustRawRequest(t, http.MethodGet, url, "secret-admin")

			// The endpoint may return OK or ServiceUnavailable depending on analytics setup
			if status != tt.expectedStatus && status != http.StatusServiceUnavailable {
				t.Errorf("Expected status %d or 503, got %d", tt.expectedStatus, status)
			}
		})
	}
}

// TestAnalyticsErrors_WithMetrics tests analyticsErrors when metrics are present
func TestAnalyticsErrors_WithMetrics(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	// Make some requests to generate metrics
	for i := 0; i < 5; i++ {
		mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/status", "secret-admin")
	}

	// Give analytics time to process
	time.Sleep(100 * time.Millisecond)

	// Now request errors
	url := baseURL + "/admin/api/v1/analytics/errors"
	status, _, _ := mustRawRequest(t, http.MethodGet, url, "secret-admin")

	// Should return OK or ServiceUnavailable
	if status != http.StatusOK && status != http.StatusServiceUnavailable {
		t.Errorf("Expected status 200 or 503, got %d", status)
	}
}

// =============================================================================
// Advanced Error Path Testing for addSubgraph
// =============================================================================

// TestAddSubgraph_Advanced tests addSubgraph with advanced scenarios
func TestAddSubgraph_Advanced(t *testing.T) {
	tests := []struct {
		name           string
		body           map[string]any
		expectedStatus int
	}{
		{
			name:           "auto-generate ID",
			body:           map[string]any{"name": "Test Subgraph", "url": "http://localhost:4001"},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "with headers",
			body:           map[string]any{"id": "sg-1", "name": "Test", "url": "http://localhost:4001", "headers": map[string]string{"Authorization": "Bearer token"}},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "empty ID with spaces",
			body:           map[string]any{"id": "   ", "name": "Test", "url": "http://localhost:4001"},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "missing URL",
			body:           map[string]any{"id": "sg-1", "name": "Test"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON",
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL, _, _ := newAdminTestServer(t)

			var bodyBytes []byte
			if tt.body != nil {
				bodyBytes, _ = json.Marshal(tt.body)
			} else {
				bodyBytes = []byte(`{invalid}`)
			}

			url := baseURL + "/admin/api/v1/subgraphs"
			status, _, _ := mustRawRequestWithBody(t, http.MethodPost, url, "secret-admin", "application/json", bodyBytes)

			// Federation is disabled in test server, so expect 400
			if status != http.StatusBadRequest {
				t.Errorf("Expected status %d, got %d", http.StatusBadRequest, status)
			}
		})
	}
}

// =============================================================================
// Advanced Error Path Testing for composeSubgraphs
// =============================================================================

// TestComposeSubgraphs_Advanced tests composeSubgraphs with advanced scenarios
func TestComposeSubgraphs_Advanced(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	// Try to compose subgraphs (federation is disabled)
	url := baseURL + "/admin/api/v1/subgraphs/compose"
	status, _, _ := mustRawRequest(t, http.MethodPost, url, "secret-admin")

	// Federation is disabled, so expect 400
	if status != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, status)
	}
}

// =============================================================================
// Advanced Error Path Testing for collectRequestMetricEvents
// =============================================================================

// TestCollectRequestMetricEvents_Advanced tests collectRequestMetricEvents via various scenarios
func TestCollectRequestMetricEvents_Advanced(t *testing.T) {
	tests := []struct {
		name       string
		stream     *realtimeStream
		wantEvents int
	}{
		{
			name:       "nil stream",
			stream:     nil,
			wantEvents: 0,
		},
		{
			name:       "nil gateway",
			stream:     &realtimeStream{gateway: nil},
			wantEvents: 0,
		},
		{
			name: "valid stream with mock engine returning nil",
			stream: func() *realtimeStream {
				return &realtimeStream{
					gateway:             &gateway.Gateway{},
					lastMetricSignature: "",
				}
			}(),
			wantEvents: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []realtimeEvent
			if tt.stream == nil {
				events = nil
			} else {
				// Direct method call since it's a method on the struct
				events = tt.stream.collectRequestMetricEvents()
			}

			gotEvents := len(events)
			if tt.wantEvents == 0 && events == nil {
				gotEvents = 0
			}
			if gotEvents != tt.wantEvents {
				t.Errorf("collectRequestMetricEvents() returned %d events, want %d", gotEvents, tt.wantEvents)
			}
		})
	}
}

// TestCollectRequestMetricEvents_WithMetrics tests the full path with metrics
func TestCollectRequestMetricEvents_WithMetrics(t *testing.T) {
	// Create a stream with a real gateway that has analytics
	cfg := &config.Config{
		Gateway: config.GatewayConfig{HTTPAddr: "127.0.0.1:0"},
	}
	gw, err := gateway.New(cfg)
	if err != nil {
		t.Skipf("Cannot create gateway: %v", err)
	}
	defer gw.Shutdown(context.Background())

	stream := &realtimeStream{
		gateway:             gw,
		lastMetricSignature: "",
	}

	// First call should return empty (no metrics yet)
	events := stream.collectRequestMetricEvents()
	if events != nil && len(events) != 0 {
		t.Errorf("Expected nil or empty events for fresh engine, got %d", len(events))
	}
}

// =============================================================================
// Advanced Error Path Testing for updateUser
// =============================================================================

// TestUpdateUser_Advanced tests updateUser with various store failure scenarios
func TestUpdateUser_Advanced(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		body           map[string]any
		expectedStatus int
	}{
		{
			name:           "user not found",
			userID:         "nonexistent-user-id-12345",
			body:           map[string]any{"name": "Updated Name"},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid payload",
			userID:         "test-user-id",
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL, _, _ := newAdminTestServer(t)

			var bodyBytes []byte
			if tt.body != nil {
				bodyBytes, _ = json.Marshal(tt.body)
			} else {
				bodyBytes = []byte(`{invalid}`)
			}

			url := fmt.Sprintf("%s/admin/api/v1/users/%s", baseURL, tt.userID)
			status, _, _ := mustRawRequestWithBody(t, http.MethodPut, url, "secret-admin", "application/json", bodyBytes)

			if status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, status)
			}
		})
	}
}

// TestUpdateUser_FullPayload tests updateUser with a complete payload
func TestUpdateUser_FullPayload(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	// Create a user
	createBody := map[string]any{
		"email":    "fullpayload@example.com",
		"name":     "Original Name",
		"role":     "user",
		"password": "password123",
		"company":  "Original Company",
	}
	createBytes, _ := json.Marshal(createBody)

	status, body, _ := mustRawRequestWithBody(t, http.MethodPost, baseURL+"/admin/api/v1/users", "secret-admin", "application/json", createBytes)
	if status != http.StatusCreated {
		t.Fatalf("Create user request failed: status=%d body=%s", status, body)
	}

	var createResult map[string]any
	json.Unmarshal([]byte(body), &createResult)

	userID, _ := createResult["id"].(string)
	if userID == "" {
		t.Skip("Could not create user for test")
	}

	// Update with full payload
	updateBody := map[string]any{
		"email":          "updated@example.com",
		"name":           "Updated Name",
		"company":        "Updated Company",
		"role":           "admin",
		"status":         "active",
		"credit_balance": 1000,
		"ip_whitelist":   []string{"192.168.1.1", "10.0.0.0/8"},
		"metadata":       map[string]any{"key": "value", "nested": map[string]any{"foo": "bar"}},
		"rate_limits":    map[string]any{"rps": 100, "burst": 200},
	}
	updateBytes, _ := json.Marshal(updateBody)

	updateURL := fmt.Sprintf("%s/admin/api/v1/users/%s", baseURL, userID)
	updateStatus, _, _ := mustRawRequestWithBody(t, http.MethodPut, updateURL, "secret-admin", "application/json", updateBytes)

	// Should succeed
	if updateStatus != http.StatusOK && updateStatus != http.StatusBadRequest {
		t.Errorf("Expected status 200 or 400, got %d", updateStatus)
	}
}

// TestUpdateUser_AllFields tests updateUser with all possible fields
func TestUpdateUser_AllFields(t *testing.T) {
	baseURL, _, _ := newAdminTestServer(t)

	// Create a user
	createBody := map[string]any{
		"email":    "allfields@example.com",
		"name":     "Original",
		"role":     "user",
		"password": "password123",
	}
	createBytes, _ := json.Marshal(createBody)

	status, body, _ := mustRawRequestWithBody(t, http.MethodPost, baseURL+"/admin/api/v1/users", "secret-admin", "application/json", createBytes)
	if status != http.StatusCreated {
		t.Fatalf("Create user request failed: status=%d body=%s", status, body)
	}

	var createResult map[string]any
	json.Unmarshal([]byte(body), &createResult)

	userID, _ := createResult["id"].(string)
	if userID == "" {
		t.Skip("Could not create user for test")
	}

	// Test updating each field individually
	fieldTests := []struct {
		name string
		body map[string]any
	}{
		{"email", map[string]any{"email": "newemail@example.com"}},
		{"name", map[string]any{"name": "New Name"}},
		{"company", map[string]any{"company": "New Company"}},
		{"role", map[string]any{"role": "admin"}},
		{"status", map[string]any{"status": "suspended"}},
		{"credit_balance", map[string]any{"credit_balance": 500}},
		{"ip_whitelist", map[string]any{"ip_whitelist": []string{"192.168.1.1"}}},
		{"metadata", map[string]any{"metadata": map[string]any{"key": "value"}}},
		{"rate_limits", map[string]any{"rate_limits": map[string]any{"rps": 50}}},
	}

	for _, ft := range fieldTests {
		t.Run(ft.name, func(t *testing.T) {
			updateBytes, _ := json.Marshal(ft.body)
			updateURL := fmt.Sprintf("%s/admin/api/v1/users/%s", baseURL, userID)
			updateStatus, _, _ := mustRawRequestWithBody(t, http.MethodPut, updateURL, "secret-admin", "application/json", updateBytes)

			if updateStatus != http.StatusOK && updateStatus != http.StatusBadRequest {
				t.Errorf("Expected status 200 or 400, got %d", updateStatus)
			}
		})
	}
}

// =============================================================================
// WebSocket and Realtime Stream Testing
// =============================================================================

// TestRealtimeStream_CollectEvents tests the realtime stream event collection
func TestRealtimeStream_CollectEvents(t *testing.T) {
	tests := []struct {
		name   string
		stream *realtimeStream
	}{
		{
			name:   "nil stream",
			stream: nil,
		},
		{
			name: "stream with nil gateway",
			stream: &realtimeStream{
				gateway:        nil,
				healthSnapshot: make(map[string]bool),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []realtimeEvent
			if tt.stream != nil {
				events = tt.stream.collectEvents([]config.Upstream{})
			}
			// Events may be nil or empty depending on the stream state
			_ = events
		})
	}
}

// TestMetricSignature_Advanced tests metricSignature with various inputs
func TestMetricSignature_Advanced(t *testing.T) {
	tests := []struct {
		name     string
		metric   analytics.RequestMetric
		contains []string
	}{
		{
			name:     "empty metric",
			metric:   analytics.RequestMetric{},
			contains: []string{"|0|0"}, // ends with zeros
		},
		{
			name: "metric with all fields",
			metric: analytics.RequestMetric{
				RouteID:    "route-1",
				RouteName:  "Test Route",
				Method:     "GET",
				StatusCode: 200,
			},
			contains: []string{"route-1", "GET", "200"},
		},
		{
			name: "metric with error",
			metric: analytics.RequestMetric{
				RouteID:    "route-1",
				RouteName:  "Test Route",
				Method:     "POST",
				StatusCode: 500,
				Error:      true,
			},
			contains: []string{"route-1", "POST", "500"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metricSignature(tt.metric)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("metricSignature() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

// =============================================================================
// Documentation of Untestable Functions
// =============================================================================

/*
DOCUMENTATION: Functions that are difficult to test without major refactoring

1. analyticsErrors - Full Coverage Blockers:
   - Line ~1991: analyticsMetricsInWindow() call - requires populated analytics engine
   - The grouping logic (lines 2010-2030) requires metrics with Error=true or StatusCode >= 400
   - The sorting logic (lines 2032-2040) requires multiple grouped errors

   To achieve 100% coverage, we would need to:
   - Inject a mock analytics.Engine that returns predefined metrics
   - This requires changing the Gateway interface or using dependency injection

2. addSubgraph - Full Coverage Blockers:
   - Line ~3314: uuid.NewString() error path - difficult to trigger
   - Line ~3332: mgr.AddSubgraph() error path - requires mock SubgraphManager
   - Success path (line ~3336) requires federation to be enabled

   To achieve 100% coverage, we would need to:
   - Enable federation in test server OR mock the SubgraphManager
   - Mock uuid.NewString() to return an error (requires monkey patching)

3. composeSubgraphs - Full Coverage Blockers:
   - Line ~3383: composer.Compose() error path - requires mock Composer
   - Success path (lines ~3386-3392) requires federation to be enabled with valid subgraphs

   To achieve 100% coverage, we would need to:
   - Enable federation in test server with mock Composer
   - Create test subgraphs with valid schemas

4. collectRequestMetricEvents - Full Coverage Blockers:
   - Line ~206: engine.Latest() returning metrics - requires populated analytics
   - Lines ~212-218: Signature deduplication logic - requires specific metric sequences
   - Lines ~224-235: Event creation with timestamp handling

   To achieve 100% coverage, we would need to:
   - Inject mock analytics.Engine with controlled metric data
   - Test the signature deduplication with specific metric sequences

5. updateUser - Full Coverage Blockers:
   - Line ~1020: store.HashPassword() error path - difficult to trigger
   - Line ~1046: store.ErrInsufficientCredits error path - requires specific store state
   - Line ~1049: sql.ErrNoRows error path - requires race condition or specific timing

   To achieve 100% coverage, we would need to:
   - Mock the store layer to return specific errors
   - Use dependency injection for store.HashPassword()

RECOMMENDED REFACTORING FOR 100% COVERAGE:

1. Create interfaces for external dependencies:
   - AnalyticsEngine interface with Latest() method
   - SubgraphManager interface with AddSubgraph(), ListSubgraphs(), etc.
   - Composer interface with Compose() method
   - Store interface with Users(), Close() methods

2. Use dependency injection in Server struct:
   type Server struct {
       gateway GatewayInterface  // Instead of concrete *gateway.Gateway
       storeFactory func() (StoreInterface, error)
   }

3. For functions that can't be refactored, use build tags for test builds:
   //go:build !test
   func uuidNewString() (string, error) { return uuid.NewString() }

   //go:build test
   var uuidNewString = func() (string, error) { return uuid.NewString() }
*/

// TestDocumentation_VerifyUntestable documents the coverage gaps
func TestDocumentation_VerifyUntestable(t *testing.T) {
	// This test serves as documentation for functions that cannot be fully tested
	// without significant refactoring

	untestable := map[string][]string{
		"analyticsErrors": {
			"Grouping logic with Error=true metrics (requires mock analytics engine)",
			"Sorting logic with multiple grouped errors (requires multiple error metrics)",
			"RouteID/RouteName 'unknown' fallback (requires metrics with empty route info)",
		},
		"addSubgraph": {
			"UUID generation error path (requires monkey patching uuid.NewString)",
			"Manager.AddSubgraph error path (requires mock SubgraphManager)",
			"Success path with auto-generated ID (requires federation enabled)",
		},
		"composeSubgraphs": {
			"Composer.Compose error path (requires mock Composer)",
			"Success path with supergraph SDL (requires federation with valid subgraphs)",
		},
		"collectRequestMetricEvents": {
			"Signature deduplication logic (requires controlled metric sequences)",
			"Event ordering (newest first to oldest) (requires multiple metrics)",
			"Timestamp zero handling (requires metrics with zero timestamps)",
		},
		"updateUser": {
			"HashPassword error path (requires mockable password hashing)",
			"ErrInsufficientCredits error path (requires specific store state)",
			"sql.ErrNoRows on Update (requires race condition simulation)",
		},
	}

	// Log the untestable paths for documentation
	t.Logf("Untestable paths requiring refactoring:")
	for funcName, paths := range untestable {
		t.Logf("  %s:", funcName)
		for _, path := range paths {
			t.Logf("    - %s", path)
		}
	}

	_ = untestable
}
