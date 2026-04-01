package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Registry holds all metrics for the gateway.
type Registry struct {
	mu sync.RWMutex

	// Counters
	counters map[string]*Counter

	// Gauges
	gauges map[string]*Gauge

	// Histograms
	histograms map[string]*Histogram
}

// Counter is a monotonically increasing counter.
type Counter struct {
	Name   string
	Help   string
	Labels []string
	value  float64
	mu     sync.RWMutex
}

// Gauge is a value that can go up and down.
type Gauge struct {
	Name   string
	Help   string
	Labels []string
	value  float64
	mu     sync.RWMutex
}

// Histogram tracks the distribution of values.
type Histogram struct {
	Name    string
	Help    string
	Labels  []string
	Buckets []float64
	counts  map[float64]uint64
	sum     float64
	count   uint64
	mu      sync.RWMutex
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// NewCounter creates or returns an existing counter.
func (r *Registry) NewCounter(name, help string, labels []string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c, ok := r.counters[name]; ok {
		return c
	}

	c := &Counter{
		Name:   name,
		Help:   help,
		Labels: labels,
		value:  0,
	}
	r.counters[name] = c
	return c
}

// NewGauge creates or returns an existing gauge.
func (r *Registry) NewGauge(name, help string, labels []string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()

	if g, ok := r.gauges[name]; ok {
		return g
	}

	g := &Gauge{
		Name:   name,
		Help:   help,
		Labels: labels,
		value:  0,
	}
	r.gauges[name] = g
	return g
}

// NewHistogram creates or returns an existing histogram.
func (r *Registry) NewHistogram(name, help string, labels []string, buckets []float64) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()

	if h, ok := r.histograms[name]; ok {
		return h
	}

	h := &Histogram{
		Name:    name,
		Help:    help,
		Labels:  labels,
		Buckets: buckets,
		counts:  make(map[float64]uint64),
	}
	r.histograms[name] = h
	return h
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

// Add adds the given value to the counter.
func (c *Counter) Add(v float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += v
}

// Value returns the current value.
func (c *Counter) Value() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Set sets the gauge value.
func (g *Gauge) Set(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = v
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value++
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value--
}

// Add adds the given value to the gauge.
func (g *Gauge) Add(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += v
}

// Sub subtracts the given value from the gauge.
func (g *Gauge) Sub(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value -= v
}

// Value returns the current value.
func (g *Gauge) Value() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
}

// Observe adds a value to the histogram.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Find the bucket
	for _, bucket := range h.Buckets {
		if v <= bucket {
			h.counts[bucket]++
			break
		}
	}

	h.sum += v
	h.count++
}

// PrometheusHandler returns an HTTP handler for Prometheus metrics export.
func (r *Registry) PrometheusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		r.mu.RLock()
		defer r.mu.RUnlock()

		// Write counters
		for _, c := range r.counters {
			fmt.Fprintf(w, "# HELP %s %s\n", c.Name, c.Help)
			fmt.Fprintf(w, "# TYPE %s counter\n", c.Name)
			fmt.Fprintf(w, "%s %f\n\n", c.Name, c.Value())
		}

		// Write gauges
		for _, g := range r.gauges {
			fmt.Fprintf(w, "# HELP %s %s\n", g.Name, g.Help)
			fmt.Fprintf(w, "# TYPE %s gauge\n", g.Name)
			fmt.Fprintf(w, "%s %f\n\n", g.Name, g.Value())
		}

		// Write histograms
		for _, h := range r.histograms {
			fmt.Fprintf(w, "# HELP %s %s\n", h.Name, h.Help)
			fmt.Fprintf(w, "# TYPE %s histogram\n", h.Name)

			h.mu.RLock()
			cumulative := uint64(0)
			for _, bucket := range h.Buckets {
				cumulative += h.counts[bucket]
				fmt.Fprintf(w, "%s_bucket{le=\"%f\"} %d\n", h.Name, bucket, cumulative)
			}
			fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.Name, h.count)
			fmt.Fprintf(w, "%s_sum %f\n", h.Name, h.sum)
			fmt.Fprintf(w, "%s_count %d\n\n", h.Name, h.count)
			h.mu.RUnlock()
		}
	})
}

// DefaultRegistry is the default global registry.
var DefaultRegistry = NewRegistry()

// GatewayMetrics holds all gateway-specific metrics.
type GatewayMetrics struct {
	// Request metrics
	RequestsTotal      *Counter
	RequestDuration    *Histogram
	RequestSize        *Histogram
	ResponseSize       *Histogram

	// Connection metrics
	ActiveConnections  *Gauge
	TotalConnections   *Counter

	// Backend metrics
	BackendRequests    *Counter
	BackendErrors      *Counter
	BackendLatency     *Histogram

	// Cache metrics
	CacheHits          *Counter
	CacheMisses        *Counter

	// Rate limiting metrics
	RateLimitHits      *Counter
	RateLimitExceeds   *Counter

	// Auth metrics
	AuthSuccess        *Counter
	AuthFailures       *Counter

	// Federation metrics
	FederationRequests *Counter
	FederationErrors   *Counter
}

// NewGatewayMetrics creates a new gateway metrics instance.
func NewGatewayMetrics(r *Registry) *GatewayMetrics {
	return &GatewayMetrics{
		RequestsTotal: r.NewCounter(
			"gateway_requests_total",
			"Total number of HTTP requests",
			[]string{"method", "status"},
		),
		RequestDuration: r.NewHistogram(
			"gateway_request_duration_seconds",
			"HTTP request duration in seconds",
			[]string{"method", "route"},
			[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		),
		RequestSize: r.NewHistogram(
			"gateway_request_size_bytes",
			"HTTP request size in bytes",
			[]string{"method"},
			[]float64{100, 1000, 10000, 100000, 1000000},
		),
		ResponseSize: r.NewHistogram(
			"gateway_response_size_bytes",
			"HTTP response size in bytes",
			[]string{"method", "status"},
			[]float64{100, 1000, 10000, 100000, 1000000, 10000000},
		),
		ActiveConnections: r.NewGauge(
			"gateway_active_connections",
			"Number of active connections",
			[]string{},
		),
		TotalConnections: r.NewCounter(
			"gateway_connections_total",
			"Total number of connections",
			[]string{},
		),
		BackendRequests: r.NewCounter(
			"gateway_backend_requests_total",
			"Total number of backend requests",
			[]string{"service", "target"},
		),
		BackendErrors: r.NewCounter(
			"gateway_backend_errors_total",
			"Total number of backend errors",
			[]string{"service", "target"},
		),
		BackendLatency: r.NewHistogram(
			"gateway_backend_latency_seconds",
			"Backend request latency in seconds",
			[]string{"service"},
			[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		),
		CacheHits: r.NewCounter(
			"gateway_cache_hits_total",
			"Total number of cache hits",
			[]string{},
		),
		CacheMisses: r.NewCounter(
			"gateway_cache_misses_total",
			"Total number of cache misses",
			[]string{},
		),
		RateLimitHits: r.NewCounter(
			"gateway_rate_limit_hits_total",
			"Total number of rate limit checks",
			[]string{},
		),
		RateLimitExceeds: r.NewCounter(
			"gateway_rate_limit_exceeds_total",
			"Total number of rate limit exceeded",
			[]string{},
		),
		AuthSuccess: r.NewCounter(
			"gateway_auth_success_total",
			"Total number of successful authentications",
			[]string{},
		),
		AuthFailures: r.NewCounter(
			"gateway_auth_failures_total",
			"Total number of failed authentications",
			[]string{},
		),
		FederationRequests: r.NewCounter(
			"gateway_federation_requests_total",
			"Total number of federation requests",
			[]string{},
		),
		FederationErrors: r.NewCounter(
			"gateway_federation_errors_total",
			"Total number of federation errors",
			[]string{},
		),
	}
}

// RecordRequest records request metrics.
func (m *GatewayMetrics) RecordRequest(method, status string, duration time.Duration, reqSize, respSize int64) {
	m.RequestsTotal.Inc()
	m.RequestDuration.Observe(duration.Seconds())
	m.RequestSize.Observe(float64(reqSize))
	m.ResponseSize.Observe(float64(respSize))
}

// RecordBackendRequest records backend request metrics.
func (m *GatewayMetrics) RecordBackendRequest(service, target string, latency time.Duration, err error) {
	m.BackendRequests.Inc()
	m.BackendLatency.Observe(latency.Seconds())
	if err != nil {
		m.BackendErrors.Inc()
	}
}

// RecordCacheHit records a cache hit.
func (m *GatewayMetrics) RecordCacheHit() {
	m.CacheHits.Inc()
}

// RecordCacheMiss records a cache miss.
func (m *GatewayMetrics) RecordCacheMiss() {
	m.CacheMisses.Inc()
}
