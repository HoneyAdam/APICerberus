package loadtest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestReport_Percentile(t *testing.T) {
	t.Parallel()

	r := &Report{
		Latencies: []time.Duration{
			1 * time.Millisecond,
			2 * time.Millisecond,
			3 * time.Millisecond,
			4 * time.Millisecond,
			5 * time.Millisecond,
			10 * time.Millisecond,
			20 * time.Millisecond,
			50 * time.Millisecond,
			100 * time.Millisecond,
			200 * time.Millisecond,
		},
	}

	tests := []struct {
		name string
		p    int
		want time.Duration
	}{
		{"p50", 50, 5 * time.Millisecond},
		{"p95", 95, 200 * time.Millisecond},
		{"p99", 99, 200 * time.Millisecond},
		{"p10", 10, 1 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := r.Percentile(tt.p)
			// Allow some tolerance since percentile calculation varies
			if got < tt.want/2 || got > tt.want*2 {
				t.Errorf("Percentile(%d) = %v, want roughly %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestReport_PercentileEmpty(t *testing.T) {
	t.Parallel()
	r := &Report{}
	if got := r.Percentile(50); got != 0 {
		t.Errorf("Percentile on empty report = %v, want 0", got)
	}
}

func TestReport_Mean(t *testing.T) {
	t.Parallel()
	r := &Report{
		Latencies: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond},
	}
	if got := r.Mean(); got != 20*time.Millisecond {
		t.Errorf("Mean() = %v, want 20ms", got)
	}
}

func TestReport_MeanEmpty(t *testing.T) {
	t.Parallel()
	r := &Report{}
	if got := r.Mean(); got != 0 {
		t.Errorf("Mean on empty report = %v, want 0", got)
	}
}

func TestReport_FailureRate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		total    int64
		failures int64
		want     float64
	}{
		{"zero total", 0, 0, 0},
		{"all success", 100, 0, 0},
		{"half fail", 100, 50, 0.5},
		{"all fail", 100, 100, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &Report{Total: tt.total, Failures: tt.failures}
			if got := r.FailureRate(); got != tt.want {
				t.Errorf("FailureRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstantRate(t *testing.T) {
	t.Parallel()

	var count int64
	report := ConstantRate(func(ctx context.Context) Result {
		atomic.AddInt64(&count, 1)
		return Result{
			Latency:    5 * time.Millisecond,
			StatusCode: 200,
		}
	}, 100, 1*time.Second)

	// Should have sent roughly 100 requests (allow 20% variance for timing)
	if report.Total < 80 || report.Total > 120 {
		t.Errorf("expected ~100 requests, got %d", report.Total)
	}
	if report.Successes != report.Total {
		t.Errorf("all requests should succeed, got %d failures", report.Failures)
	}
	if report.RequestsPerSec < 50 {
		t.Errorf("throughput too low: %.1f req/s", report.RequestsPerSec)
	}
}

func TestRampUp(t *testing.T) {
	t.Parallel()

	report := RampUp(func(ctx context.Context) Result {
		return Result{
			Latency:    10 * time.Millisecond,
			StatusCode: 200,
		}
	}, 10, 50, 2*time.Second, 4)

	if report.Total == 0 {
		t.Error("expected some requests in ramp-up test")
	}
	if report.FailureRate() > 0.1 {
		t.Errorf("failure rate too high: %.2f", report.FailureRate())
	}
}
