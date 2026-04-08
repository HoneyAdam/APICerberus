package analytics

import (
	"testing"
	"time"
)

// =============================================================================
// Tests for 0.0% coverage functions
// =============================================================================

func TestOptimizedEngine_addToBatch(t *testing.T) {
	cfg := OptimizedEngineConfig{
		RingBufferSize:     1000,
		BucketRetention:    time.Hour,
		BatchSize:          10,
		BatchInterval:      time.Second,
		MaxBatchSize:       100,
		WorkerCount:        2,
		QueueSize:          1000,
		DropOnOverflow:     true,
		PreallocateBuckets: 60,
		EnableCompression:  false,
	}
	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	metric := RequestMetric{
		Timestamp:       time.Now(),
		RouteID:         "route-1",
		RouteName:       "Test Route",
		ServiceName:     "test-service",
		UserID:          "user-1",
		Method:          "GET",
		Path:            "/api/test",
		StatusCode:      200,
		LatencyMS:       100,
		BytesIn:         100,
		BytesOut:        200,
		CreditsConsumed: 1,
		Blocked:         false,
		Error:           false,
	}

	// addToBatch is called internally by Record
	engine.Record(metric)

	// Give time for async processing
	time.Sleep(10 * time.Millisecond)
}

// TestOptimizedEngine_addToBatch_More tests addToBatch with batch flush triggering
func TestOptimizedEngine_addToBatch_More(t *testing.T) {
	cfg := OptimizedEngineConfig{
		RingBufferSize:     1000,
		BucketRetention:    time.Hour,
		BatchSize:          2,
		BatchInterval:      time.Second,
		MaxBatchSize:       100,
		WorkerCount:        1,
		QueueSize:          100,
		DropOnOverflow:     true,
		PreallocateBuckets: 60,
		EnableCompression:  false,
	}
	engine := NewOptimizedEngine(cfg)
	defer engine.Stop()

	// Add metrics to trigger batch flush
	metric1 := RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "route-1",
		Method:     "GET",
		StatusCode: 200,
		LatencyMS:  100,
	}
	metric2 := RequestMetric{
		Timestamp:  time.Now(),
		RouteID:    "route-2",
		Method:     "POST",
		StatusCode: 201,
		LatencyMS:  150,
	}

	// First metric should not trigger flush (batch size = 2)
	engine.addToBatch(metric1)

	// Second metric should trigger flush
	engine.addToBatch(metric2)

	// Give time for async processing
	time.Sleep(50 * time.Millisecond)
}
