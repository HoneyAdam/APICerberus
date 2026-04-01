package loadbalancer

import (
	"fmt"
	"testing"
	"time"
)

func TestNewGeoIPResolver(t *testing.T) {
	resolver := NewGeoIPResolver()
	if resolver == nil {
		t.Fatal("NewGeoIPResolver() returned nil")
	}
}

func TestGeoIPResolverResolve(t *testing.T) {
	resolver := NewGeoIPResolver()

	tests := []struct {
		ip      string
		want    string
	}{
		{"192.168.1.1", "US"},
		{"10.0.0.1", "US"},
		{"172.16.0.1", "EU"},
		{"127.0.0.1", "LOCAL"},
		{"8.8.8.8", "UNKNOWN"},
		{"invalid", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := resolver.Resolve(tt.ip)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestNewGeoAwareSelector(t *testing.T) {
	selector := NewGeoAwareSelector()
	if selector == nil {
		t.Fatal("NewGeoAwareSelector() returned nil")
	}
}

func TestGeoAwareSelectorSelect(t *testing.T) {
	selector := NewGeoAwareSelector()
	selector.SetTargetLocation("target-1", "US")
	selector.SetTargetLocation("target-2", "EU")

	// US client should get US target
	got := selector.Select("192.168.1.1", []string{"target-1", "target-2"})
	if got != "target-1" {
		t.Errorf("Select for US client = %v, want target-1", got)
	}

	// EU client should get EU target
	got = selector.Select("172.16.0.1", []string{"target-1", "target-2"})
	if got != "target-2" {
		t.Errorf("Select for EU client = %v, want target-2", got)
	}
}

func TestGeoAwareSelectorSelectFallback(t *testing.T) {
	selector := NewGeoAwareSelector()
	// No locations set

	got := selector.Select("192.168.1.1", []string{"target-1", "target-2"})
	if got != "target-1" {
		t.Errorf("Should fallback to first target, got %v", got)
	}
}

func TestNewAdaptiveBalancer(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	if balancer == nil {
		t.Fatal("NewAdaptiveBalancer() returned nil")
	}

	if balancer.GetAlgorithm() != "round_robin" {
		t.Errorf("Default algorithm = %v, want round_robin", balancer.GetAlgorithm())
	}
}

func TestAdaptiveBalancerRecordRequest(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.RecordRequest("target-1", 100*time.Millisecond, nil)
	balancer.RecordRequest("target-1", 200*time.Millisecond, nil)

	stats, ok := balancer.GetStats("target-1")
	if !ok {
		t.Fatal("Expected stats for target-1")
	}

	if stats.RequestCount != 2 {
		t.Errorf("RequestCount = %v, want 2", stats.RequestCount)
	}

	if stats.AvgLatency != 150*time.Millisecond {
		t.Errorf("AvgLatency = %v, want 150ms", stats.AvgLatency)
	}
}

func TestAdaptiveBalancerRecordRequestWithError(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.RecordRequest("target-1", 100*time.Millisecond, nil)
	balancer.RecordRequest("target-1", 100*time.Millisecond, fmt.Errorf("error"))

	stats, _ := balancer.GetStats("target-1")

	if stats.ErrorCount != 1 {
		t.Errorf("ErrorCount = %v, want 1", stats.ErrorCount)
	}

	if stats.ErrorRate != 0.5 {
		t.Errorf("ErrorRate = %v, want 0.5", stats.ErrorRate)
	}
}

func TestAdaptiveBalancerRoundRobin(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	targets := []string{"target-1", "target-2", "target-3"}

	got, idx := balancer.SelectTarget(targets, 0)
	if got != "target-2" {
		t.Errorf("First selection = %v, want target-2", got)
	}

	got, _ = balancer.SelectTarget(targets, idx)
	if got != "target-3" {
		t.Errorf("Second selection = %v, want target-3", got)
	}
}

func TestAdaptiveBalancerCalculateHealthScore(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	// Unknown target should have perfect score
	score := balancer.CalculateHealthScore("target-unknown")
	if score != 100 {
		t.Errorf("Unknown target score = %v, want 100", score)
	}

	// Target with 0% error rate should have high score
	balancer.RecordRequest("target-1", 100*time.Millisecond, nil)
	score = balancer.CalculateHealthScore("target-1")
	if score < 90 {
		t.Errorf("Healthy target score = %v, expected > 90", score)
	}
}

func TestAdaptiveBalancerResetStats(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.RecordRequest("target-1", 100*time.Millisecond, nil)
	balancer.ResetStats()

	stats := balancer.GetAllStats()
	if len(stats) != 0 {
		t.Errorf("Expected 0 stats after reset, got %d", len(stats))
	}
}

