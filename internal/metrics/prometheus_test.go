package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestCounter(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("test_counter", "Test counter", []string{})

	// Test initial value
	if c.Value() != 0 {
		t.Errorf("Initial value = %v, want 0", c.Value())
	}

	// Test Inc
	c.Inc()
	if c.Value() != 1 {
		t.Errorf("After Inc, value = %v, want 1", c.Value())
	}

	// Test Add
	c.Add(5)
	if c.Value() != 6 {
		t.Errorf("After Add(5), value = %v, want 6", c.Value())
	}
}

func TestGauge(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("test_gauge", "Test gauge", []string{})

	// Test Set
	g.Set(10)
	if g.Value() != 10 {
		t.Errorf("After Set(10), value = %v, want 10", g.Value())
	}

	// Test Inc
	g.Inc()
	if g.Value() != 11 {
		t.Errorf("After Inc, value = %v, want 11", g.Value())
	}

	// Test Dec
	g.Dec()
	if g.Value() != 10 {
		t.Errorf("After Dec, value = %v, want 10", g.Value())
	}

	// Test Add
	g.Add(5)
	if g.Value() != 15 {
		t.Errorf("After Add(5), value = %v, want 15", g.Value())
	}

	// Test Sub
	g.Sub(3)
	if g.Value() != 12 {
		t.Errorf("After Sub(3), value = %v, want 12", g.Value())
	}
}

func TestHistogram(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogram("test_histogram", "Test histogram", []string{},
		[]float64{0.1, 0.5, 1.0, 2.0, 5.0})

	// Test Observe
	h.Observe(0.05)
	h.Observe(0.3)
	h.Observe(1.5)
	h.Observe(3.0)

	// Just verify it doesn't panic - full histogram testing would be complex
}

func TestPrometheusHandler(t *testing.T) {
	r := NewRegistry()

	// Create some metrics
	c := r.NewCounter("requests_total", "Total requests", []string{})
	c.Inc()
	c.Inc()

	g := r.NewGauge("active_connections", "Active connections", []string{})
	g.Set(10)

	// Create handler
	handler := r.PrometheusHandler()

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check status
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %v, want %v", rec.Code, http.StatusOK)
	}

	// Check content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %v, want text/plain; version=0.0.4", contentType)
	}

	// Check body contains metrics
	body := rec.Body.String()
	if !contains(body, "requests_total") {
		t.Error("Body should contain requests_total")
	}
	if !contains(body, "active_connections") {
		t.Error("Body should contain active_connections")
	}
	if !contains(body, "# TYPE requests_total counter") {
		t.Error("Body should contain TYPE annotation for counter")
	}
	if !contains(body, "# TYPE active_connections gauge") {
		t.Error("Body should contain TYPE annotation for gauge")
	}
}

func TestPrometheusHandlerMethodNotAllowed(t *testing.T) {
	r := NewRegistry()
	handler := r.PrometheusHandler()

	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %v, want %v", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestGatewayMetrics(t *testing.T) {
	r := NewRegistry()
	m := NewGatewayMetrics(r)

	if m.RequestsTotal == nil {
		t.Error("RequestsTotal should not be nil")
	}
	if m.RequestDuration == nil {
		t.Error("RequestDuration should not be nil")
	}
	if m.ActiveConnections == nil {
		t.Error("ActiveConnections should not be nil")
	}
	if m.CacheHits == nil {
		t.Error("CacheHits should not be nil")
	}
	if m.AuthSuccess == nil {
		t.Error("AuthSuccess should not be nil")
	}
}

func TestGatewayMetricsRecordRequest(t *testing.T) {
	r := NewRegistry()
	m := NewGatewayMetrics(r)

	// Record a request
	m.RecordRequest("GET", "200", 100*time.Millisecond, 100, 1000)

	// Just verify it doesn't panic
}

func TestGatewayMetricsRecordBackendRequest(t *testing.T) {
	r := NewRegistry()
	m := NewGatewayMetrics(r)

	// Record successful backend request
	m.RecordBackendRequest("service-1", "target-1", 50*time.Millisecond, nil)

	// Record failed backend request
	m.RecordBackendRequest("service-1", "target-1", 100*time.Millisecond, http.ErrServerClosed)

	// Just verify it doesn't panic
}

func TestGatewayMetricsCache(t *testing.T) {
	r := NewRegistry()
	m := NewGatewayMetrics(r)

	// Record cache hit
	m.RecordCacheHit()
	if m.CacheHits.Value() != 1 {
		t.Errorf("CacheHits = %v, want 1", m.CacheHits.Value())
	}

	// Record cache miss
	m.RecordCacheMiss()
	if m.CacheMisses.Value() != 1 {
		t.Errorf("CacheMisses = %v, want 1", m.CacheMisses.Value())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
