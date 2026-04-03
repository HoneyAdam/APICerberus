package analytics

import (
	"testing"
	"time"
)

func TestRingBufferSnapshotOrderAndLimit(t *testing.T) {
	t.Parallel()

	ring := NewRingBuffer[int](3)
	ring.Push(1)
	ring.Push(2)
	ring.Push(3)
	ring.Push(4)

	all := ring.Snapshot(0)
	if len(all) != 3 {
		t.Fatalf("expected 3 entries got %d", len(all))
	}
	if all[0] != 4 || all[1] != 3 || all[2] != 2 {
		t.Fatalf("unexpected snapshot order: %#v", all)
	}

	limited := ring.Snapshot(2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 entries got %d", len(limited))
	}
	if limited[0] != 4 || limited[1] != 3 {
		t.Fatalf("unexpected limited snapshot order: %#v", limited)
	}
}

func TestTimeSeriesStoreBucketsAndCleanup(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Minute)
	store := NewTimeSeriesStore(2 * time.Hour)
	store.Record(RequestMetric{
		Timestamp:  now.Add(-3 * time.Hour),
		StatusCode: 200,
		LatencyMS:  10,
	})
	store.Record(RequestMetric{
		Timestamp:  now,
		StatusCode: 200,
		LatencyMS:  10,
		BytesIn:    10,
		BytesOut:   20,
	})
	store.Record(RequestMetric{
		Timestamp:  now,
		StatusCode: 200,
		LatencyMS:  30,
		BytesIn:    5,
		BytesOut:   7,
	})
	store.Record(RequestMetric{
		Timestamp:  now,
		StatusCode: 503,
		LatencyMS:  50,
		Error:      true,
	})

	buckets := store.Buckets(now.Add(-time.Hour), now.Add(time.Hour))
	if len(buckets) != 1 {
		t.Fatalf("expected 1 active bucket got %d", len(buckets))
	}

	b := buckets[0]
	if b.Requests != 3 {
		t.Fatalf("expected requests=3 got %d", b.Requests)
	}
	if b.Errors != 1 {
		t.Fatalf("expected errors=1 got %d", b.Errors)
	}
	if b.P50LatencyMS != 30 || b.P95LatencyMS != 50 || b.P99LatencyMS != 50 {
		t.Fatalf("unexpected percentiles: p50=%d p95=%d p99=%d", b.P50LatencyMS, b.P95LatencyMS, b.P99LatencyMS)
	}
	if b.BytesIn != 15 || b.BytesOut != 27 {
		t.Fatalf("unexpected bytes counters: in=%d out=%d", b.BytesIn, b.BytesOut)
	}
	if b.StatusCodes[200] != 2 || b.StatusCodes[503] != 1 {
		t.Fatalf("unexpected status code distribution: %#v", b.StatusCodes)
	}
}

func TestEngineRecordAndOverview(t *testing.T) {
	t.Parallel()

	engine := NewEngine(EngineConfig{
		RingBufferSize:  8,
		BucketRetention: 4 * time.Hour,
	})
	engine.IncActiveConns()
	engine.IncActiveConns()
	engine.DecActiveConns()

	engine.Record(RequestMetric{StatusCode: 200, RouteID: "r1", LatencyMS: 10})
	engine.Record(RequestMetric{StatusCode: 502, RouteID: "r2", LatencyMS: 20})
	engine.Record(RequestMetric{StatusCode: 200, RouteID: "r3", LatencyMS: 30})

	overview := engine.Overview()
	if overview.TotalRequests != 3 {
		t.Fatalf("expected total_requests=3 got %d", overview.TotalRequests)
	}
	if overview.TotalErrors != 1 {
		t.Fatalf("expected total_errors=1 got %d", overview.TotalErrors)
	}
	if overview.ActiveConns != 1 {
		t.Fatalf("expected active_conns=1 got %d", overview.ActiveConns)
	}
	if overview.ErrorRate < 0.3333 || overview.ErrorRate > 0.3334 {
		t.Fatalf("unexpected error_rate=%f", overview.ErrorRate)
	}

	latest := engine.Latest(2)
	if len(latest) != 2 {
		t.Fatalf("expected 2 latest items got %d", len(latest))
	}
	if latest[0].RouteID != "r3" || latest[1].RouteID != "r2" {
		t.Fatalf("unexpected latest order: %#v", latest)
	}
}

// Test TimeSeries function
func TestEngine_TimeSeries(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Minute)
	engine := NewEngine(EngineConfig{
		RingBufferSize:  100,
		BucketRetention: 4 * time.Hour,
	})

	// Record some metrics in different time buckets
	engine.Record(RequestMetric{
		Timestamp:  now.Add(-30 * time.Minute),
		StatusCode: 200,
		RouteID:    "r1",
		LatencyMS:  10,
	})
	engine.Record(RequestMetric{
		Timestamp:  now.Add(-20 * time.Minute),
		StatusCode: 200,
		RouteID:    "r2",
		LatencyMS:  20,
	})
	engine.Record(RequestMetric{
		Timestamp:  now.Add(-10 * time.Minute),
		StatusCode: 500,
		RouteID:    "r3",
		LatencyMS:  30,
		Error:      true,
	})

	// Test TimeSeries with different time ranges
	t.Run("1 hour range", func(t *testing.T) {
		buckets := engine.TimeSeries(now.Add(-1*time.Hour), now)
		if len(buckets) == 0 {
			t.Error("expected some buckets, got none")
		}
	})

	t.Run("2 hour range", func(t *testing.T) {
		buckets := engine.TimeSeries(now.Add(-2*time.Hour), now)
		if len(buckets) == 0 {
			t.Error("expected some buckets, got none")
		}
	})

	// Test with nil engine
	var nilEngine *Engine
	buckets := nilEngine.TimeSeries(now.Add(-1*time.Hour), now)
	if buckets != nil {
		t.Error("expected nil buckets with nil engine")
	}
}

// Test RingBuffer Len function
func TestRingBuffer_Len(t *testing.T) {
	t.Parallel()

	ring := NewRingBuffer[int](5)

	// Test empty buffer
	if ring.Len() != 0 {
		t.Errorf("expected empty buffer len=0, got %d", ring.Len())
	}

	// Add some items
	ring.Push(1)
	ring.Push(2)
	ring.Push(3)

	if ring.Len() != 3 {
		t.Errorf("expected buffer len=3, got %d", ring.Len())
	}

	// Fill buffer
	ring.Push(4)
	ring.Push(5)
	ring.Push(6) // Should overwrite first item

	if ring.Len() != 5 {
		t.Errorf("expected buffer len=5 (capacity), got %d", ring.Len())
	}
}
