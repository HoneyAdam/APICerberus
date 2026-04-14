package analytics

import (
	"context"
	"testing"
	"time"
)

func TestNewEngine_Defaults(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestEngine_IncDecActiveConns(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	e.IncActiveConns()
	e.IncActiveConns()
	ov := e.Overview()
	if ov.ActiveConns != 2 {
		t.Errorf("ActiveConns = %d, want 2", ov.ActiveConns)
	}
	e.DecActiveConns()
	ov = e.Overview()
	if ov.ActiveConns != 1 {
		t.Errorf("ActiveConns = %d, want 1", ov.ActiveConns)
	}
}

func TestEngine_DecActiveConns_NotNegative(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	e.DecActiveConns()
	e.DecActiveConns()
	ov := e.Overview()
	if ov.ActiveConns != 0 {
		t.Errorf("ActiveConns = %d, want 0 (not negative)", ov.ActiveConns)
	}
}

func TestEngine_NilReceivers(t *testing.T) {
	t.Parallel()
	var e *Engine
	e.IncActiveConns()
	e.DecActiveConns()
	if ov := e.Overview(); ov != (Overview{}) {
		t.Errorf("nil Overview = %+v, want zero", ov)
	}
	if latest := e.Latest(10); latest != nil {
		t.Errorf("nil Latest = %v, want nil", latest)
	}
	if ts := e.TimeSeries(time.Now(), time.Now()); ts != nil {
		t.Errorf("nil TimeSeries = %v, want nil", ts)
	}
	e.Record(RequestMetric{})
	e.Shutdown(context.Background())
}

func TestEngine_Record(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	now := time.Now()
	e.Record(RequestMetric{
		Timestamp:  now,
		RouteID:    "route-1",
		StatusCode: 200,
		LatencyMS:  50,
	})
	e.Record(RequestMetric{
		Timestamp:  now,
		RouteID:    "route-2",
		StatusCode: 500,
		LatencyMS:  100,
		Error:      true,
	})
	ov := e.Overview()
	if ov.TotalRequests != 2 {
		t.Errorf("TotalRequests = %d, want 2", ov.TotalRequests)
	}
	if ov.TotalErrors != 1 {
		t.Errorf("TotalErrors = %d, want 1", ov.TotalErrors)
	}
}

func TestEngine_Record_ZeroTimestamp(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	e.Record(RequestMetric{StatusCode: 200, LatencyMS: 10})
	ov := e.Overview()
	if ov.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", ov.TotalRequests)
	}
}

func TestEngine_Latest(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	e.Record(RequestMetric{RouteID: "r1", StatusCode: 200})
	e.Record(RequestMetric{RouteID: "r2", StatusCode: 200})
	latest := e.Latest(1)
	if len(latest) != 1 {
		t.Fatalf("Latest(1) = %d items, want 1", len(latest))
	}
	if latest[0].RouteID != "r2" {
		t.Errorf("Latest(1) RouteID = %q, want r2 (most recent)", latest[0].RouteID)
	}
}

func TestEngine_Latest_All(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	e.Record(RequestMetric{RouteID: "r1", StatusCode: 200})
	e.Record(RequestMetric{RouteID: "r2", StatusCode: 200})
	latest := e.Latest(0) // 0 means no limit
	if len(latest) != 2 {
		t.Errorf("Latest(0) = %d items, want 2", len(latest))
	}
}

func TestEngine_TimeSeries(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	e := NewEngine(EngineConfig{BucketRetention: time.Hour})
	e.Record(RequestMetric{Timestamp: now, StatusCode: 200, LatencyMS: 50})
	buckets := e.TimeSeries(now.Add(-time.Hour), now.Add(time.Minute))
	if len(buckets) != 1 {
		t.Fatalf("Buckets = %d, want 1", len(buckets))
	}
	if buckets[0].Requests != 1 {
		t.Errorf("Requests = %d, want 1", buckets[0].Requests)
	}
	if buckets[0].AvgLatencyMS != 50 {
		t.Errorf("AvgLatencyMS = %f, want 50", buckets[0].AvgLatencyMS)
	}
}

func TestEngine_Shutdown(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	e.Shutdown(context.Background())
}

func TestNewRingBuffer(t *testing.T) {
	t.Parallel()
	rb := NewRingBuffer[int](5)
	if rb == nil {
		t.Fatal("expected non-nil ring buffer")
	}
	if rb.size != 5 {
		t.Errorf("size = %d, want 5", rb.size)
	}
}

func TestNewRingBuffer_ZeroSize(t *testing.T) {
	t.Parallel()
	rb := NewRingBuffer[int](0)
	if rb.size != 1 {
		t.Errorf("size = %d, want 1 (default)", rb.size)
	}
}

func TestRingBuffer_PushSnapshot(t *testing.T) {
	t.Parallel()
	rb := NewRingBuffer[string](3)
	rb.Push("a")
	rb.Push("b")
	rb.Push("c")
	snap := rb.Snapshot(10)
	if len(snap) != 3 {
		t.Fatalf("Snapshot(10) = %d items, want 3", len(snap))
	}
	// Most recent first
	if snap[0] != "c" {
		t.Errorf("snap[0] = %q, want c", snap[0])
	}
}

func TestRingBuffer_Overwrite(t *testing.T) {
	t.Parallel()
	rb := NewRingBuffer[int](2)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3) // overwrites 1
	snap := rb.Snapshot(10)
	if len(snap) != 2 {
		t.Fatalf("Snapshot = %d items, want 2", len(snap))
	}
	if snap[0] != 3 || snap[1] != 2 {
		t.Errorf("snap = %v, want [3 2]", snap)
	}
}

func TestRingBuffer_Nil(t *testing.T) {
	t.Parallel()
	var rb *RingBuffer[int]
	rb.Push(1)
	if snap := rb.Snapshot(10); snap != nil {
		t.Errorf("nil Snapshot = %v, want nil", snap)
	}
}

func TestRingBuffer_SnapshotEmpty(t *testing.T) {
	t.Parallel()
	rb := NewRingBuffer[int](5)
	if snap := rb.Snapshot(10); snap != nil {
		t.Errorf("empty Snapshot = %v, want nil", snap)
	}
}

func TestNewTimeSeriesStore(t *testing.T) {
	t.Parallel()
	ts := NewTimeSeriesStore(time.Hour)
	if ts == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestTimeSeriesStore_NilReceivers(t *testing.T) {
	t.Parallel()
	var ts *TimeSeriesStore
	ts.Record(RequestMetric{})
	if buckets := ts.Buckets(time.Now(), time.Now()); buckets != nil {
		t.Errorf("nil Buckets = %v, want nil", buckets)
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		values   []int64
		p        int
		expected int64
	}{
		{"empty", nil, 50, 0},
		{"single", []int64{100}, 50, 100},
		{"p50 even", []int64{10, 20, 30, 40}, 50, 20},
		{"p99", []int64{10, 20, 30, 40, 50}, 99, 50},
		{"p0 clamp", []int64{10, 20, 30}, 0, 10},
		{"p100", []int64{10, 20, 30}, 100, 30},
		{"p>100 clamp", []int64{10, 20, 30}, 200, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := percentile(tt.values, tt.p)
			if got != tt.expected {
				t.Errorf("percentile(%v, %d) = %d, want %d", tt.values, tt.p, got, tt.expected)
			}
		})
	}
}

func TestCloneStatusCodes(t *testing.T) {
	t.Parallel()
	original := map[int]int64{200: 50, 404: 10}
	cloned := cloneStatusCodes(original)
	if len(cloned) != 2 {
		t.Errorf("len = %d, want 2", len(cloned))
	}
	cloned[200] = 999
	if original[200] != 50 {
		t.Error("clone should not modify original")
	}
}

func TestCloneStatusCodes_Empty(t *testing.T) {
	t.Parallel()
	cloned := cloneStatusCodes(nil)
	if len(cloned) != 0 {
		t.Errorf("len = %d, want 0", len(cloned))
	}
}

func TestEngine_ErrorRate(t *testing.T) {
	t.Parallel()
	e := NewEngine(EngineConfig{})
	now := time.Now()
	e.Record(RequestMetric{Timestamp: now, StatusCode: 200})
	e.Record(RequestMetric{Timestamp: now, StatusCode: 200})
	e.Record(RequestMetric{Timestamp: now, StatusCode: 500, Error: true})
	ov := e.Overview()
	if ov.ErrorRate != 1.0/3.0 {
		t.Errorf("ErrorRate = %f, want %f", ov.ErrorRate, 1.0/3.0)
	}
}
