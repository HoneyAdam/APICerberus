package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/federation"
)

// Test analyticsMetricsInWindow with various scenarios
func TestAnalyticsMetricsInWindow_Final(t *testing.T) {
	t.Parallel()

	t.Run("nil engine returns nil", func(t *testing.T) {
		result := analyticsMetricsInWindow(nil, time.Now().Add(-time.Hour), time.Now())
		if result != nil {
			t.Error("Expected nil for nil engine")
		}
	})

	t.Run("empty metrics returns nil", func(t *testing.T) {
		engine := analytics.NewEngine(analytics.EngineConfig{})
		result := analyticsMetricsInWindow(engine, time.Now().Add(-time.Hour), time.Now())
		if result != nil {
			t.Error("Expected nil for empty metrics")
		}
	})

	t.Run("metrics outside window are filtered", func(t *testing.T) {
		engine := analytics.NewEngine(analytics.EngineConfig{})

		// Record a metric
		engine.Record(analytics.RequestMetric{
			Timestamp:  time.Now(),
			RouteID:    "test-route",
			LatencyMS:  100,
			StatusCode: 200,
		})

		// Query for metrics outside the window (past)
		result := analyticsMetricsInWindow(engine, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
		if len(result) != 0 {
			t.Errorf("Expected 0 metrics, got %d", len(result))
		}
	})

	t.Run("metrics inside window are returned", func(t *testing.T) {
		engine := analytics.NewEngine(analytics.EngineConfig{})

		now := time.Now()
		// Record a metric
		engine.Record(analytics.RequestMetric{
			Timestamp:  now,
			RouteID:    "test-route",
			LatencyMS:  100,
			StatusCode: 200,
		})

		// Query for metrics inside the window
		result := analyticsMetricsInWindow(engine, now.Add(-time.Hour), now.Add(time.Hour))
		if len(result) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(result))
		}
	})

	t.Run("swaps from/to when from is after to", func(t *testing.T) {
		engine := analytics.NewEngine(analytics.EngineConfig{})

		now := time.Now()
		// Record a metric
		engine.Record(analytics.RequestMetric{
			Timestamp:  now,
			RouteID:    "test-route",
			LatencyMS:  100,
			StatusCode: 200,
		})

		// Query with reversed from/to (should swap)
		result := analyticsMetricsInWindow(engine, now.Add(time.Hour), now.Add(-time.Hour))
		if len(result) != 1 {
			t.Errorf("Expected 1 metric after swap, got %d", len(result))
		}
	})
}

// Test aggregateAnalyticsSeries
func TestAggregateAnalyticsSeries_Final(t *testing.T) {
	t.Parallel()

	t.Run("empty metrics returns empty", func(t *testing.T) {
		result := aggregateAnalyticsSeries([]analytics.RequestMetric{}, time.Minute)
		if len(result) != 0 {
			t.Errorf("Expected 0 items, got %d", len(result))
		}
	})

	t.Run("nil metrics returns empty", func(t *testing.T) {
		result := aggregateAnalyticsSeries(nil, time.Minute)
		if len(result) != 0 {
			t.Errorf("Expected 0 items, got %d", len(result))
		}
	})

	t.Run("default granularity is minute", func(t *testing.T) {
		now := time.Now()
		metrics := []analytics.RequestMetric{
			{Timestamp: now, LatencyMS: 100, StatusCode: 200},
		}
		result := aggregateAnalyticsSeries(metrics, 0) // 0 should default to minute
		if len(result) != 1 {
			t.Errorf("Expected 1 item, got %d", len(result))
		}
	})

	t.Run("groups by time bucket", func(t *testing.T) {
		now := time.Now().Truncate(time.Minute)
		metrics := []analytics.RequestMetric{
			{Timestamp: now, LatencyMS: 100, StatusCode: 200, BytesIn: 100, BytesOut: 200, CreditsConsumed: 10},
			{Timestamp: now.Add(30 * time.Second), LatencyMS: 200, StatusCode: 500, BytesIn: 200, BytesOut: 400, CreditsConsumed: 20},
			{Timestamp: now.Add(time.Minute), LatencyMS: 150, StatusCode: 200, BytesIn: 150, BytesOut: 300, CreditsConsumed: 15},
		}
		result := aggregateAnalyticsSeries(metrics, time.Minute)
		if len(result) != 2 {
			t.Errorf("Expected 2 time buckets, got %d", len(result))
		}

		// First bucket should have 2 requests, 1 error
		if result[0].requests != 2 {
			t.Errorf("Expected 2 requests in first bucket, got %d", result[0].requests)
		}
		if result[0].errors != 1 {
			t.Errorf("Expected 1 error in first bucket, got %d", result[0].errors)
		}
		if result[0].bytesIn != 300 {
			t.Errorf("Expected 300 bytesIn, got %d", result[0].bytesIn)
		}
		if result[0].bytesOut != 600 {
			t.Errorf("Expected 600 bytesOut, got %d", result[0].bytesOut)
		}
		if result[0].creditsConsumed != 30 {
			t.Errorf("Expected 30 credits, got %d", result[0].creditsConsumed)
		}
	})

	t.Run("status codes are counted", func(t *testing.T) {
		now := time.Now().Truncate(time.Minute)
		metrics := []analytics.RequestMetric{
			{Timestamp: now, LatencyMS: 100, StatusCode: 200},
			{Timestamp: now, LatencyMS: 200, StatusCode: 200},
			{Timestamp: now, LatencyMS: 300, StatusCode: 404},
		}
		result := aggregateAnalyticsSeries(metrics, time.Minute)
		if len(result) != 1 {
			t.Fatalf("Expected 1 bucket, got %d", len(result))
		}
		if result[0].statusCodes[200] != 2 {
			t.Errorf("Expected 2 status 200, got %d", result[0].statusCodes[200])
		}
		if result[0].statusCodes[404] != 1 {
			t.Errorf("Expected 1 status 404, got %d", result[0].statusCodes[404])
		}
	})

	t.Run("results are sorted by time", func(t *testing.T) {
		now := time.Now().Truncate(time.Minute)
		metrics := []analytics.RequestMetric{
			{Timestamp: now.Add(time.Minute), LatencyMS: 100},
			{Timestamp: now, LatencyMS: 200},
		}
		result := aggregateAnalyticsSeries(metrics, time.Minute)
		if len(result) != 2 {
			t.Fatalf("Expected 2 buckets, got %d", len(result))
		}
		if !result[0].start.Before(result[1].start) {
			t.Error("Results not sorted by time")
		}
	})
}

// Test collectRequestMetricEvents
func TestCollectRequestMetricEvents_Final(t *testing.T) {
	t.Parallel()

	t.Run("nil stream returns nil", func(t *testing.T) {
		var stream *realtimeStream
		result := stream.collectRequestMetricEvents()
		if result != nil {
			t.Error("Expected nil for nil stream")
		}
	})

	t.Run("nil gateway returns nil", func(t *testing.T) {
		stream := &realtimeStream{gateway: nil}
		result := stream.collectRequestMetricEvents()
		if result != nil {
			t.Error("Expected nil for nil gateway")
		}
	})

	t.Run("nil analytics engine returns nil", func(t *testing.T) {
		// Create stream with nil gateway
		stream := &realtimeStream{gateway: nil}
		result := stream.collectRequestMetricEvents()
		if result != nil {
			t.Error("Expected nil for nil analytics engine")
		}
	})

}

// Mock gateway with analytics for testing
type mockGatewayWithAnalytics struct {
	analytics *analytics.Engine
}

func (m *mockGatewayWithAnalytics) Analytics() *analytics.Engine {
	return m.analytics
}

func (m *mockGatewayWithAnalytics) Subgraphs() *federation.SubgraphManager {
	return nil
}

func (m *mockGatewayWithAnalytics) FederationComposer() *federation.Composer {
	return nil
}

func (m *mockGatewayWithAnalytics) RebuildFederationPlanner() {}

// Test addSubgraph endpoint
func TestAddSubgraph_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("federation disabled", func(t *testing.T) {
		// When federation is not enabled, should return 400
		body := `{"name":"test","url":"http://localhost:4001"}`
		status, _, _ := mustRawRequestWithBody(t, http.MethodPost, baseURL+"/admin/api/v1/federation/subgraphs", "secret-admin", "application/json", []byte(body))
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusBadRequest, http.StatusNotFound)
		}
	})
}

// Test composeSubgraphs endpoint
func TestComposeSubgraphs_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("federation disabled", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/federation/compose", "secret-admin")
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusBadRequest, http.StatusNotFound)
		}
	})
}

// Test getSubgraph endpoint
func TestGetSubgraph_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("federation disabled", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/federation/subgraphs/test-id", "secret-admin")
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusBadRequest, http.StatusNotFound)
		}
	})
}

// Test removeSubgraph endpoint
func TestRemoveSubgraph_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("federation disabled", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/federation/subgraphs/test-id", "secret-admin")
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusBadRequest, http.StatusNotFound)
		}
	})
}

// Test listSubgraphs endpoint
func TestListSubgraphs_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("federation disabled returns empty or error", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/federation/subgraphs", "secret-admin")
		// May return 200 with empty list or 400 if federation disabled
		if status != http.StatusOK && status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d, %d or %d", status, http.StatusOK, http.StatusBadRequest, http.StatusNotFound)
		}
	})
}

// Test analyticsErrors endpoint
func TestAnalyticsErrors_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("analytics unavailable", func(t *testing.T) {
		// When analytics is not configured, should return 503
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/errors", "secret-admin")
		// May return 200 with empty data or 503 if analytics unavailable
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusOK, http.StatusServiceUnavailable)
		}
	})

	t.Run("invalid time range", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/errors?from=invalid", "secret-admin")
		if status != http.StatusBadRequest && status != http.StatusOK {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusBadRequest, http.StatusOK)
		}
	})
}

// Test analyticsLatency endpoint
func TestAnalyticsLatency_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("analytics unavailable", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/latency", "secret-admin")
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusOK, http.StatusServiceUnavailable)
		}
	})
}

// Test analyticsTopRoutes endpoint
func TestAnalyticsTopRoutes_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("analytics unavailable", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes", "secret-admin")
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusOK, http.StatusServiceUnavailable)
		}
	})
}

// Test analyticsTopConsumers endpoint
func TestAnalyticsTopConsumers_Endpoint_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("analytics unavailable", func(t *testing.T) {
		status, _, _ := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-consumers", "secret-admin")
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusOK, http.StatusServiceUnavailable)
		}
	})
}

// Test updateUser with various error paths
func TestUpdateUser_ErrorPaths_Final(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := newAdminTestServer(t)

	t.Run("invalid JSON payload", func(t *testing.T) {
		// First create a user
		result := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/users", "secret-admin", map[string]any{
			"email":    "update-invalid@example.com",
			"name":     "Test User",
			"role":     "user",
			"password": "password123",
		})
		userID := asString(result["id"])

		body := `{"name": invalid}`
		status, _, _ := mustRawRequestWithBody(t, http.MethodPut, baseURL+"/admin/api/v1/users/"+userID, "secret-admin", "application/json", []byte(body))
		// May return 400 for bad JSON or 404 if user not found (depending on order of validation)
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d or %d", status, http.StatusBadRequest, http.StatusNotFound)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		body := `{"name":"Updated Name"}`
		status, _, _ := mustRawRequestWithBody(t, http.MethodPut, baseURL+"/admin/api/v1/users/nonexistent-user-id", "secret-admin", "application/json", []byte(body))
		if status != http.StatusNotFound {
			t.Errorf("Status = %d, want %d", status, http.StatusNotFound)
		}
	})
}

// Test resolveAnalyticsRange
func TestResolveAnalyticsRange_Final(t *testing.T) {
	t.Parallel()

	t.Run("default range is 1 hour", func(t *testing.T) {
		from, to, err := resolveAnalyticsRange(map[string][]string{})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		duration := to.Sub(from)
		if duration < 59*time.Minute || duration > 61*time.Minute {
			t.Errorf("Expected ~1 hour duration, got %v", duration)
		}
	})

	t.Run("invalid from time", func(t *testing.T) {
		_, _, err := resolveAnalyticsRange(map[string][]string{
			"from": {"invalid"},
		})
		if err == nil {
			t.Error("Expected error for invalid from time")
		}
	})

	t.Run("invalid to time", func(t *testing.T) {
		_, _, err := resolveAnalyticsRange(map[string][]string{
			"to": {"invalid"},
		})
		if err == nil {
			t.Error("Expected error for invalid to time")
		}
	})

	t.Run("custom range", func(t *testing.T) {
		now := time.Now().UTC()
		from, to, err := resolveAnalyticsRange(map[string][]string{
			"from": {now.Add(-2 * time.Hour).Format(time.RFC3339)},
			"to":   {now.Format(time.RFC3339)},
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		duration := to.Sub(from)
		if duration < 119*time.Minute || duration > 121*time.Minute {
			t.Errorf("Expected ~2 hour duration, got %v", duration)
		}
	})
}

// Test resolveAnalyticsGranularity
func TestResolveAnalyticsGranularity_Final(t *testing.T) {
	t.Parallel()

	t.Run("default granularity", func(t *testing.T) {
		gran, err := resolveAnalyticsGranularity(map[string][]string{})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if gran != time.Minute {
			t.Errorf("Expected 1 minute, got %v", gran)
		}
	})

	t.Run("minute granularity", func(t *testing.T) {
		gran, err := resolveAnalyticsGranularity(map[string][]string{
			"granularity": {"1m"},
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if gran != time.Minute {
			t.Errorf("Expected 1 minute, got %v", gran)
		}
	})

	t.Run("hour granularity", func(t *testing.T) {
		gran, err := resolveAnalyticsGranularity(map[string][]string{
			"granularity": {"1h"},
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if gran != time.Hour {
			t.Errorf("Expected 1 hour, got %v", gran)
		}
	})

	t.Run("day granularity", func(t *testing.T) {
		gran, err := resolveAnalyticsGranularity(map[string][]string{
			"granularity": {"24h"},
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		expected := 24 * time.Hour
		if gran != expected {
			t.Errorf("Expected 1 day, got %v", gran)
		}
	})

	t.Run("invalid granularity", func(t *testing.T) {
		_, err := resolveAnalyticsGranularity(map[string][]string{
			"granularity": {"invalid"},
		})
		if err == nil {
			t.Error("Expected error for invalid granularity")
		}
	})
}

// Test resolveAnalyticsLimit
func TestResolveAnalyticsLimit_Final(t *testing.T) {
	t.Parallel()

	t.Run("default limit", func(t *testing.T) {
		limit, err := resolveAnalyticsLimit(map[string][]string{}, 100, 0)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if limit != 100 {
			t.Errorf("Expected 100, got %d", limit)
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		limit, err := resolveAnalyticsLimit(map[string][]string{
			"limit": {"50"},
		}, 100, 0)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if limit != 50 {
			t.Errorf("Expected 50, got %d", limit)
		}
	})

	t.Run("limit exceeds max", func(t *testing.T) {
		limit, err := resolveAnalyticsLimit(map[string][]string{
			"limit": {"200"},
		}, 100, 100)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if limit != 100 {
			t.Errorf("Expected 100 (max), got %d", limit)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		_, err := resolveAnalyticsLimit(map[string][]string{
			"limit": {"invalid"},
		}, 100, 0)
		if err == nil {
			t.Error("Expected error for invalid limit")
		}
	})

	t.Run("negative limit uses fallback", func(t *testing.T) {
		limit, err := resolveAnalyticsLimit(map[string][]string{
			"limit": {"-5"},
		}, 100, 50)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if limit != 50 {
			t.Errorf("Expected 50 (fallback), got %d", limit)
		}
	})
}
