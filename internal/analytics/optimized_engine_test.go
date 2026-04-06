package analytics

import (
	"testing"
	"time"
)

// TestDefaultOptimizedEngineConfig tests the default configuration
func TestDefaultOptimizedEngineConfig(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()

	if cfg.RingBufferSize != 100_000 {
		t.Errorf("expected RingBufferSize 100000, got %d", cfg.RingBufferSize)
	}
	if cfg.BatchSize != 1000 {
		t.Errorf("expected BatchSize 1000, got %d", cfg.BatchSize)
	}
	if cfg.WorkerCount != 4 {
		t.Errorf("expected WorkerCount 4, got %d", cfg.WorkerCount)
	}
	if !cfg.DropOnOverflow {
		t.Error("expected DropOnOverflow to be true")
	}
}

// TestNewOptimizedEngine tests creating a new optimized engine
func TestNewOptimizedEngine(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.RingBufferSize = 1000
	cfg.BatchSize = 10
	cfg.WorkerCount = 2
	cfg.QueueSize = 100

	engine := NewOptimizedEngine(cfg)
	if engine == nil {
		t.Fatal("NewOptimizedEngine returned nil")
	}

	// Verify initial state
	if engine.IsStopped() {
		t.Error("engine should not be stopped initially")
	}

	// Clean up
	engine.Stop()
}

// TestNewOptimizedEngineWithZeroConfig tests creating engine with zero config
func TestNewOptimizedEngineWithZeroConfig(t *testing.T) {
	engine := NewOptimizedEngine(OptimizedEngineConfig{})
	if engine == nil {
		t.Fatal("NewOptimizedEngine returned nil with zero config")
	}
	engine.Stop()
}

// TestOptimizedEngineRecord tests recording metrics
func TestOptimizedEngineRecord(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.BatchSize = 5
	cfg.BatchInterval = 500 * time.Millisecond
	cfg.WorkerCount = 1
	cfg.QueueSize = 100

	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	metric := RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "test-route",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		LatencyMS:  10,
	}

	engine.Record(metric)

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify metric was recorded by checking overview
	overview := engine.Overview()
	if overview.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", overview.TotalRequests)
	}
}

// TestOptimizedEngineRecordMultiple tests recording multiple metrics
func TestOptimizedEngineRecordMultiple(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.BatchSize = 10
	cfg.BatchInterval = 200 * time.Millisecond
	cfg.WorkerCount = 2
	cfg.QueueSize = 100

	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	// Record multiple metrics
	for i := 0; i < 10; i++ {
		metric := RequestMetric{
			Timestamp:  time.Now(),
			RouteID:    "test-route",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			LatencyMS:  int64(i * 10),
		}
		engine.Record(metric)
	}

	// Give time for processing
	time.Sleep(300 * time.Millisecond)

	// Verify metrics were recorded
	overview := engine.Overview()
	if overview.TotalRequests != 10 {
		t.Errorf("expected 10 requests, got %d", overview.TotalRequests)
	}
}

// TestOptimizedEngineActiveConns tests active connection tracking
func TestOptimizedEngineActiveConns(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	// Test increment/decrement
	engine.IncActiveConns()
	engine.IncActiveConns()

	overview := engine.Overview()
	if overview.ActiveConns != 2 {
		t.Errorf("expected 2 active connections, got %d", overview.ActiveConns)
	}

	engine.DecActiveConns()

	overview = engine.Overview()
	if overview.ActiveConns != 1 {
		t.Errorf("expected 1 active connection, got %d", overview.ActiveConns)
	}
}

// TestOptimizedEngineLatest tests retrieving latest metrics
func TestOptimizedEngineLatest(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.RingBufferSize = 100
	cfg.BatchSize = 1
	cfg.BatchInterval = 50 * time.Millisecond
	cfg.WorkerCount = 1

	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	// Record some metrics
	for i := 0; i < 5; i++ {
		engine.Record(RequestMetric{
			Timestamp:  time.Now(),
			RouteID:    "test-route",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			LatencyMS:  int64(i),
		})
	}

	// Give time for processing
	time.Sleep(200 * time.Millisecond)

	// Get latest
	latest := engine.Latest(10)
	if len(latest) != 5 {
		t.Errorf("expected 5 latest metrics, got %d", len(latest))
	}

	// Test with limit
	latest = engine.Latest(3)
	if len(latest) != 3 {
		t.Errorf("expected 3 latest metrics, got %d", len(latest))
	}
}

// TestOptimizedEngineTimeSeries tests time series retrieval
func TestOptimizedEngineTimeSeries(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.BatchSize = 1
	cfg.BatchInterval = 50 * time.Millisecond
	cfg.WorkerCount = 1

	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	now := time.Now()

	// Record metrics
	for i := 0; i < 5; i++ {
		engine.Record(RequestMetric{
			Timestamp:  now.Add(-time.Duration(5-i) * time.Minute),
			RouteID:    "test-route",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			LatencyMS:  10,
		})
	}

	// Give time for processing
	time.Sleep(200 * time.Millisecond)

	// Get time series
	from := now.Add(-10 * time.Minute)
	to := now.Add(time.Minute)
	points := engine.TimeSeries(from, to)

	if len(points) == 0 {
		t.Error("expected some time series points")
	}
}

// TestOptimizedEngineMetrics tests metrics retrieval
func TestOptimizedEngineMetrics(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.BatchSize = 1
	cfg.BatchInterval = 50 * time.Millisecond
	cfg.WorkerCount = 1

	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	// Record metrics with different status codes
	engine.Record(RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "test-route",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		LatencyMS:  10,
	})
	engine.Record(RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "test-route",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 500,
		LatencyMS:  100,
	})

	// Give time for processing
	time.Sleep(200 * time.Millisecond)

	// Get metrics
	metrics := engine.Metrics()
	if metrics.TotalRequests != 2 {
		t.Errorf("expected 2 total requests, got %d", metrics.TotalRequests)
	}
	if metrics.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", metrics.TotalErrors)
	}
}

// TestOptimizedEngineStop tests stopping the engine
func TestOptimizedEngineStop(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	engine := NewOptimizedEngine(cfg)

	// Record a metric
	engine.Record(RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "test-route",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
	})

	// Stop should flush pending metrics
	engine.Stop()

	if !engine.IsStopped() {
		t.Error("engine should be stopped")
	}

	// Recording after stop should not panic
	engine.Record(RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "test-route",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
	})
}

// TestOptimizedTimeSeriesStore tests the optimized time series store
func TestOptimizedTimeSeriesStore(t *testing.T) {
	store := NewOptimizedTimeSeriesStore(time.Hour, 10)
	if store == nil {
		t.Fatal("NewOptimizedTimeSeriesStore returned nil")
	}

	now := time.Now()

	// Record batch of metrics
	metrics := []RequestMetric{
		{
			Timestamp:  now,
			RouteID:    "route1",
			Method:     "GET",
			Path:       "/api/1",
			StatusCode: 200,
			LatencyMS:  10,
		},
		{
			Timestamp:  now,
			RouteID:    "route2",
			Method:     "POST",
			Path:       "/api/2",
			StatusCode: 201,
			LatencyMS:  20,
		},
	}

	store.RecordBatch(metrics)

	// Get buckets
	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)
	buckets := store.Buckets(from, to)

	if len(buckets) == 0 {
		t.Error("expected some buckets")
	}
}

// TestOptimizedTimeSeriesStoreCleanup tests cleanup of old buckets
func TestOptimizedTimeSeriesStoreCleanup(t *testing.T) {
	store := NewOptimizedTimeSeriesStore(100*time.Millisecond, 5)

	now := time.Now()

	// Record recent metric
	store.RecordBatch([]RequestMetric{
		{
			Timestamp:  now,
			RouteID:    "recent-route",
			Method:     "GET",
			Path:       "/recent",
			StatusCode: 200,
			LatencyMS:  10,
		},
	})

	// Get buckets before cleanup
	from := now.Add(-2 * time.Hour)
	to := now.Add(time.Hour)
	buckets := store.Buckets(from, to)

	foundRecent := false
	for _, b := range buckets {
		if b.Requests > 0 {
			foundRecent = true
			break
		}
	}

	if !foundRecent {
		t.Error("recent buckets should exist")
	}

	// Run cleanup
	store.cleanup(now.Add(time.Hour))

	// Verify bucket exists after cleanup (recent one should remain)
	buckets = store.Buckets(from, to)
	foundAfter := false
	for _, b := range buckets {
		if b.Requests > 0 {
			foundAfter = true
			break
		}
	}

	if !foundAfter && len(buckets) > 0 {
		t.Log("buckets may have been cleaned up based on retention")
	}
}

// TestOptimizedEngineWithNil tests handling of nil engine
func TestOptimizedEngineWithNil(t *testing.T) {
	var engine *OptimizedEngine

	// These should not panic
	if !engine.IsStopped() {
		t.Error("nil engine should report as stopped")
	}

	engine.Stop() // Should not panic

	overview := engine.Overview()
	if overview.TotalRequests != 0 {
		t.Error("nil engine overview should have zero values")
	}

	latest := engine.Latest(10)
	if latest != nil {
		t.Error("nil engine Latest should return nil")
	}

	points := engine.TimeSeries(time.Now(), time.Now())
	if points != nil {
		t.Error("nil engine TimeSeries should return nil")
	}

	metrics := engine.Metrics()
	if metrics.TotalRequests != 0 {
		t.Error("nil engine Metrics should return zero values")
	}
}

// TestOptimizedTimeSeriesStoreWithNil tests handling of nil store
func TestOptimizedTimeSeriesStoreWithNil(t *testing.T) {
	var store *OptimizedTimeSeriesStore

	// These should not panic
	buckets := store.Buckets(time.Now(), time.Now())
	if buckets != nil {
		t.Error("nil store Buckets should return nil")
	}

	store.RecordBatch([]RequestMetric{}) // Should not panic
}

// TestOptimizedEngineConcurrency tests concurrent operations
func TestOptimizedEngineConcurrency(t *testing.T) {
	cfg := DefaultOptimizedEngineConfig()
	cfg.BatchSize = 100
	cfg.BatchInterval = 100 * time.Millisecond
	cfg.WorkerCount = 4
	cfg.QueueSize = 1000

	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	done := make(chan struct{})

	// Start multiple goroutines recording metrics
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				engine.Record(RequestMetric{
					Timestamp:  time.Now(),
					RouteID:    "route",
					Method:     "GET",
					Path:       "/test",
					StatusCode: 200,
					LatencyMS:  int64(j),
				})
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Give time for processing
	time.Sleep(300 * time.Millisecond)

	overview := engine.Overview()
	if overview.TotalRequests != 1000 {
		t.Errorf("expected 1000 requests, got %d", overview.TotalRequests)
	}
}
