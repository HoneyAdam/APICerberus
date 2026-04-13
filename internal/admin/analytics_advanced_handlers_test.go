package admin

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// =============================================================================
// Advanced Analytics Handler Tests
// =============================================================================

func TestHandleAnalyticsForecast(t *testing.T) {
	t.Run("forecast with default parameters", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/forecast", token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertHasJSONField(t, resp, "forecast")
		assertHasJSONField(t, resp, "metric")
		assertHasJSONField(t, resp, "trend")
		assertHasJSONField(t, resp, "confidence")
	})

	t.Run("forecast with custom parameters", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		url := baseURL + "/admin/api/v1/analytics/forecast?metric=latency&route_id=route-users&horizon=48"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertJSONField(t, resp, "metric", "latency")
		assertJSONField(t, resp, "route_id", "route-users")
	})

	t.Run("forecast with invalid horizon", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// Invalid horizon should use default
		url := baseURL + "/admin/api/v1/analytics/forecast?horizon=invalid"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("forecast with horizon exceeding max", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// Horizon > 168 should be clamped
		url := baseURL + "/admin/api/v1/analytics/forecast?horizon=200"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})
}

func TestHandleAnalyticsAnomalies(t *testing.T) {
	t.Run("anomalies with default parameters", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/anomalies", token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertHasJSONField(t, resp, "anomalies")
		assertHasJSONField(t, resp, "threshold")
		assertHasJSONField(t, resp, "total_checked")
	})

	t.Run("anomalies with custom threshold", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/anomalies?metric=latency&threshold=1.5"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertJSONField(t, resp, "threshold", 1.5)
	})

	t.Run("anomalies with route filter", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/anomalies?route_id=route-users&metric=requests"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertJSONField(t, resp, "route_id", "route-users")
	})

	t.Run("anomalies with time range", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		startTime := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().UTC().Format(time.RFC3339)
		url := baseURL + "/admin/api/v1/analytics/anomalies?start_time=" + startTime + "&end_time=" + endTime
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})
}

func TestHandleAnalyticsCorrelations(t *testing.T) {
	t.Run("correlations with default metrics", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/correlations", token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertHasJSONField(t, resp, "correlations")
	})

	t.Run("correlations with custom metrics", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/correlations?metrics=requests,latency,error_rate"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
		assertHasJSONField(t, resp, "correlations")
	})

	t.Run("correlations with route filter", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/correlations?route_id=route-users"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})
}

func TestHandleAnalyticsExports(t *testing.T) {
	t.Run("export json format", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		status, body, headers := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/exports?format=json", token)
		if status != http.StatusOK {
			t.Fatalf("expected status 200, got %d", status)
		}
		if !strings.Contains(headers.Get("Content-Type"), "application/json") {
			t.Errorf("expected JSON content type, got %s", headers.Get("Content-Type"))
		}
		if !strings.Contains(body, "request_id") {
			t.Errorf("expected exported data to contain request_id")
		}
	})

	t.Run("export csv format", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		status, body, headers := mustRawRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/exports?format=csv", token)
		if status != http.StatusOK {
			t.Fatalf("expected status 200, got %d", status)
		}
		if !strings.Contains(headers.Get("Content-Type"), "text/csv") {
			t.Errorf("expected CSV content type, got %s", headers.Get("Content-Type"))
		}
		if !strings.Contains(body, "request_id") {
			t.Errorf("expected CSV to contain request_id header")
		}
	})

	t.Run("export with invalid format", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/exports?format=xml"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("export with route and user filter", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/exports?format=json&route_id=route-users&user_id=user-1"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("export with limit", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		url := baseURL + "/admin/api/v1/analytics/exports?format=json&limit=5"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("export with limit exceeding max", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token

		// Seed some audit data
		seedAuditData(t, storePath)

		// Limit > 50000 should be clamped
		url := baseURL + "/admin/api/v1/analytics/exports?format=json&limit=100000"
		resp := mustJSONRequest(t, http.MethodGet, url, token, nil)
		assertStatus(t, resp, http.StatusOK)
	})
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestCalculateForecast(t *testing.T) {
	t.Run("forecast requests metric", func(t *testing.T) {
		points := calculateForecast(nil, "requests", "", 24)
		if len(points) != 24 {
			t.Errorf("expected 24 forecast points, got %d", len(points))
		}
		for i, p := range points {
			if p.Timestamp.IsZero() {
				t.Errorf("point %d has zero timestamp", i)
			}
			if p.Value <= 0 {
				t.Errorf("point %d has non-positive value", i)
			}
			if p.Lower >= p.Upper {
				t.Errorf("point %d has invalid bounds: lower=%v, upper=%v", i, p.Lower, p.Upper)
			}
		}
	})

	t.Run("forecast latency metric", func(t *testing.T) {
		points := calculateForecast(nil, "latency", "route-1", 12)
		if len(points) != 12 {
			t.Errorf("expected 12 forecast points, got %d", len(points))
		}
	})

	t.Run("forecast errors metric", func(t *testing.T) {
		points := calculateForecast(nil, "errors", "", 6)
		if len(points) != 6 {
			t.Errorf("expected 6 forecast points, got %d", len(points))
		}
	})
}

func TestDetermineTrend(t *testing.T) {
	tests := []struct {
		name     string
		forecast []ForecastPoint
		expected string
	}{
		{
			name:     "empty forecast",
			forecast: []ForecastPoint{},
			expected: "stable",
		},
		{
			name: "single point",
			forecast: []ForecastPoint{
				{Value: 100},
			},
			expected: "stable",
		},
		{
			name: "upward trend",
			forecast: []ForecastPoint{
				{Value: 100},
				{Value: 120},
			},
			expected: "up",
		},
		{
			name: "downward trend",
			forecast: []ForecastPoint{
				{Value: 100},
				{Value: 80},
			},
			expected: "down",
		},
		{
			name: "stable trend",
			forecast: []ForecastPoint{
				{Value: 100},
				{Value: 105},
			},
			expected: "stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineTrend(tt.forecast)
			if result != tt.expected {
				t.Errorf("expected trend %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectAnomalies(t *testing.T) {
	t.Run("empty entries", func(t *testing.T) {
		anomalies := detectAnomalies(nil, "requests", 2.5)
		if anomalies != nil {
			t.Errorf("expected nil for empty entries, got %v", anomalies)
		}
	})

	t.Run("no anomalies with uniform data", func(t *testing.T) {
		entries := []store.AuditEntry{
			{LatencyMS: 100, StatusCode: 200, CreatedAt: time.Now()},
			{LatencyMS: 100, StatusCode: 200, CreatedAt: time.Now()},
			{LatencyMS: 100, StatusCode: 200, CreatedAt: time.Now()},
		}
		anomalies := detectAnomalies(entries, "latency", 2.5)
		if len(anomalies) != 0 {
			t.Errorf("expected no anomalies for uniform data, got %d", len(anomalies))
		}
	})

	t.Run("detects anomalies with varying data", func(t *testing.T) {
		entries := []store.AuditEntry{
			{LatencyMS: 100, StatusCode: 200, CreatedAt: time.Now()},
			{LatencyMS: 100, StatusCode: 200, CreatedAt: time.Now()},
			{LatencyMS: 1000, StatusCode: 200, CreatedAt: time.Now()}, // anomaly
		}
		anomalies := detectAnomalies(entries, "latency", 1.0)
		if len(anomalies) == 0 {
			t.Errorf("expected anomalies, got none")
		}
		for _, a := range anomalies {
			if a.Severity == "" {
				t.Error("anomaly missing severity")
			}
			if a.ZScore == 0 {
				t.Error("anomaly missing z-score")
			}
		}
	})

	t.Run("error rate metric", func(t *testing.T) {
		entries := []store.AuditEntry{
			{StatusCode: 200, CreatedAt: time.Now()},
			{StatusCode: 200, CreatedAt: time.Now()},
			{StatusCode: 500, CreatedAt: time.Now()},
		}
		anomalies := detectAnomalies(entries, "error_rate", 1.0)
		// Should detect anomaly based on error rate
		_ = anomalies
	})
}

func TestCalculateCorrelations(t *testing.T) {
	t.Run("empty entries", func(t *testing.T) {
		correlations := calculateCorrelations(nil, []string{"requests", "latency"})
		if correlations != nil {
			t.Errorf("expected nil for empty entries, got %v", correlations)
		}
	})

	t.Run("single metric", func(t *testing.T) {
		entries := []store.AuditEntry{
			{LatencyMS: 100, BytesIn: 1000, BytesOut: 2000, StatusCode: 200},
		}
		correlations := calculateCorrelations(entries, []string{"requests"})
		if correlations != nil {
			t.Errorf("expected nil for single metric, got %v", correlations)
		}
	})

	t.Run("correlation between metrics", func(t *testing.T) {
		entries := []store.AuditEntry{
			{LatencyMS: 100, BytesIn: 1000, BytesOut: 2000, StatusCode: 200},
			{LatencyMS: 200, BytesIn: 2000, BytesOut: 4000, StatusCode: 200},
			{LatencyMS: 300, BytesIn: 3000, BytesOut: 6000, StatusCode: 200},
		}
		metrics := []string{"latency", "bytes_in", "bytes_out"}
		correlations := calculateCorrelations(entries, metrics)
		if len(correlations) == 0 {
			t.Fatal("expected correlations, got none")
		}
		for _, c := range correlations {
			if c.Metric1 == "" || c.Metric2 == "" {
				t.Error("correlation missing metric names")
			}
			if c.Strength == "" {
				t.Error("correlation missing strength")
			}
			if c.Direction == "" {
				t.Error("correlation missing direction")
			}
		}
	})
}

func TestExtractMetricValues(t *testing.T) {
	entries := []store.AuditEntry{
		{LatencyMS: 100, BytesIn: 1000, BytesOut: 2000, StatusCode: 200},
		{LatencyMS: 200, BytesIn: 2000, BytesOut: 4000, StatusCode: 500},
	}

	tests := []struct {
		name     string
		metric   string
		expected []float64
	}{
		{
			name:     "requests",
			metric:   "requests",
			expected: []float64{1, 1},
		},
		{
			name:     "latency",
			metric:   "latency",
			expected: []float64{100, 200},
		},
		{
			name:     "error_rate",
			metric:   "error_rate",
			expected: []float64{0, 1},
		},
		{
			name:     "bytes_in",
			metric:   "bytes_in",
			expected: []float64{1000, 2000},
		},
		{
			name:     "bytes_out",
			metric:   "bytes_out",
			expected: []float64{2000, 4000},
		},
		{
			name:     "unknown",
			metric:   "unknown",
			expected: []float64{0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := extractMetricValues(entries, tt.metric)
			if len(values) != len(tt.expected) {
				t.Errorf("expected %d values, got %d", len(tt.expected), len(values))
				return
			}
			for i, v := range values {
				if v != tt.expected[i] {
					t.Errorf("value %d: expected %v, got %v", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestPearsonCorrelation(t *testing.T) {
	tests := []struct {
		name     string
		x        []float64
		y        []float64
		expected float64
	}{
		{
			name:     "empty slices",
			x:        []float64{},
			y:        []float64{},
			expected: 0,
		},
		{
			name:     "different lengths",
			x:        []float64{1, 2, 3},
			y:        []float64{1, 2},
			expected: 0,
		},
		{
			name:     "perfect positive correlation",
			x:        []float64{1, 2, 3, 4, 5},
			y:        []float64{2, 4, 6, 8, 10},
			expected: 1,
		},
		{
			name:     "perfect negative correlation",
			x:        []float64{1, 2, 3, 4, 5},
			y:        []float64{10, 8, 6, 4, 2},
			expected: -1,
		},
		{
			name:     "no correlation",
			x:        []float64{1, 1, 1, 1, 1},
			y:        []float64{1, 2, 3, 4, 5},
			expected: 0, // division by zero case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pearsonCorrelation(tt.x, tt.y)
			// Use tolerance for floating point comparison
			tolerance := 0.0001
			if diff := result - tt.expected; diff < -tolerance || diff > tolerance {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestParseTimeRange(t *testing.T) {
	t.Run("default duration", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		start, end := parseTimeRange(req, 24*time.Hour)

		if start.IsZero() || end.IsZero() {
			t.Error("expected non-zero times")
		}

		duration := end.Sub(start)
		expectedDuration := 24 * time.Hour
		// Allow 1 second tolerance for execution time
		if duration < expectedDuration-time.Second || duration > expectedDuration+time.Second {
			t.Errorf("expected duration ~%v, got %v", expectedDuration, duration)
		}
	})

	t.Run("custom time range", func(t *testing.T) {
		startTime := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().UTC().Format(time.RFC3339)
		url := "/test?start_time=" + startTime + "&end_time=" + endTime
		req := httptest.NewRequest(http.MethodGet, url, nil)

		start, end := parseTimeRange(req, 24*time.Hour)

		// Parse expected times for comparison
		expectedStart, _ := time.Parse(time.RFC3339, startTime)
		expectedEnd, _ := time.Parse(time.RFC3339, endTime)

		if !start.Equal(expectedStart) {
			t.Errorf("expected start %v, got %v", expectedStart, start)
		}
		if !end.Equal(expectedEnd) {
			t.Errorf("expected end %v, got %v", expectedEnd, end)
		}
	})

	t.Run("invalid time format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?start_time=invalid", nil)
		start, end := parseTimeRange(req, 24*time.Hour)

		// Should use default when invalid
		if start.IsZero() || end.IsZero() {
			t.Error("expected fallback to default times")
		}
	})
}

func TestMax(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{1, 2, 2},
		{2, 1, 2},
		{1, 1, 1},
		{-1, 1, 1},
		{-2, -1, -1},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := max(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("max(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestExportJSON(t *testing.T) {
	entries := []store.AuditEntry{
		{
			ID:         "audit-1",
			RequestID:  "req-1",
			RouteID:    "route-1",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			LatencyMS:  100,
			ClientIP:   "127.0.0.1",
			CreatedAt:  time.Now(),
		},
	}

	w := httptest.NewRecorder()
	exportJSON(w, entries)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected content-type application/json, got %s", contentType)
	}

	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("expected attachment disposition, got %s", disposition)
	}

	var result []store.AuditEntry
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Errorf("failed to unmarshal exported JSON: %v", err)
	}
	if len(result) != len(entries) {
		t.Errorf("expected %d entries, got %d", len(entries), len(result))
	}
}

func TestExportCSV(t *testing.T) {
	entries := []store.AuditEntry{
		{
			ID:           "audit-1",
			RequestID:    "req-1",
			RouteID:      "route-1",
			RouteName:    "route-1",
			ServiceName:  "svc-1",
			UserID:       "user-1",
			ConsumerName: "test-user",
			Method:       "GET",
			Host:         "localhost",
			Path:         "/test",
			StatusCode:   200,
			LatencyMS:    100,
			BytesIn:      1000,
			BytesOut:     2000,
			ClientIP:     "127.0.0.1",
			Blocked:      false,
			CreatedAt:    time.Now(),
		},
	}

	w := httptest.NewRecorder()
	exportCSV(w, entries)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/csv" {
		t.Errorf("expected content-type text/csv, got %s", contentType)
	}

	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Errorf("failed to read CSV: %v", err)
	}

	// Should have header + 1 data row
	if len(records) != 2 {
		t.Errorf("expected 2 rows (header + data), got %d", len(records))
	}

	// Check header
	expectedHeaders := []string{"id", "request_id", "route_id", "route_name", "service_name",
		"user_id", "consumer_name", "method", "host", "path", "status_code", "latency_ms",
		"bytes_in", "bytes_out", "client_ip", "blocked", "created_at"}
	if len(records[0]) != len(expectedHeaders) {
		t.Errorf("expected %d columns, got %d", len(expectedHeaders), len(records[0]))
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

func seedAuditData(t *testing.T, storePath string) {
	t.Helper()

	// Use the existing store to seed data
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        storePath,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}

	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	entries := []store.AuditEntry{
		{
			ID:          "audit-1",
			RequestID:   "req-1",
			RouteID:     "route-users",
			RouteName:   "route-users",
			ServiceName: "svc-users",
			Method:      "GET",
			Path:        "/users",
			StatusCode:  200,
			LatencyMS:   50,
			BytesIn:     100,
			BytesOut:    1000,
			ClientIP:    "127.0.0.1",
			CreatedAt:   now.Add(-1 * time.Hour),
		},
		{
			ID:          "audit-2",
			RequestID:   "req-2",
			RouteID:     "route-users",
			RouteName:   "route-users",
			ServiceName: "svc-users",
			Method:      "POST",
			Path:        "/users",
			StatusCode:  201,
			LatencyMS:   100,
			BytesIn:     200,
			BytesOut:    500,
			ClientIP:    "127.0.0.1",
			CreatedAt:   now.Add(-30 * time.Minute),
		},
		{
			ID:          "audit-3",
			RequestID:   "req-3",
			RouteID:     "route-users",
			RouteName:   "route-users",
			ServiceName: "svc-users",
			Method:      "GET",
			Path:        "/users",
			StatusCode:  500,
			LatencyMS:   500,
			BytesIn:     100,
			BytesOut:    100,
			ClientIP:    "127.0.0.1",
			CreatedAt:   now,
		},
	}

	if err := st.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("failed to seed audit data: %v", err)
	}
}
