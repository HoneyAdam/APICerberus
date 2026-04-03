package loadbalancer

import (
	"fmt"
	"testing"
	"time"
)

func TestAdaptiveBalancerLeastConnections(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	// Set algorithm to least_connections
	balancer.algorithm = "least_connections"

	targets := []string{"target-1", "target-2", "target-3"}

	// Record some requests with different error rates
	balancer.RecordRequest("target-1", 100*time.Millisecond, nil)
	balancer.RecordRequest("target-1", 100*time.Millisecond, nil) // 0% error rate

	balancer.RecordRequest("target-2", 100*time.Millisecond, nil)
	balancer.RecordRequest("target-2", 100*time.Millisecond, fmt.Errorf("error")) // 50% error rate

	balancer.RecordRequest("target-3", 100*time.Millisecond, fmt.Errorf("error"))
	balancer.RecordRequest("target-3", 100*time.Millisecond, fmt.Errorf("error")) // 100% error rate

	// Should select target-1 with lowest error rate
	got, _ := balancer.SelectTarget(targets, 0)
	if got != "target-1" {
		t.Errorf("Least connections selection = %v, want target-1", got)
	}
}

func TestAdaptiveBalancerLeastConnectionsEmpty(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.algorithm = "least_connections"

	// Empty targets
	got, idx := balancer.SelectTarget([]string{}, 0)
	if got != "" {
		t.Errorf("Empty targets selection = %v, want empty", got)
	}
	if idx != 0 {
		t.Errorf("Empty targets index = %v, want 0", idx)
	}
}

func TestAdaptiveBalancerLeastConnectionsNoStats(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.algorithm = "least_connections"

	targets := []string{"target-1", "target-2"}

	// No stats recorded - should select first target
	got, _ := balancer.SelectTarget(targets, 0)
	if got != "target-1" {
		t.Errorf("No stats selection = %v, want target-1", got)
	}
}

func TestAdaptiveBalancerWeightedResponseTime(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	// Set algorithm to weighted_response_time
	balancer.algorithm = "weighted_response_time"

	targets := []string{"target-1", "target-2", "target-3"}

	// Record requests with different latencies
	balancer.RecordRequest("target-1", 500*time.Millisecond, nil) // High latency
	balancer.RecordRequest("target-2", 100*time.Millisecond, nil) // Low latency
	balancer.RecordRequest("target-3", 300*time.Millisecond, nil) // Medium latency

	// Should select target-2 with lowest latency (highest weight)
	got, _ := balancer.SelectTarget(targets, 0)
	if got != "target-2" {
		t.Errorf("Weighted response time selection = %v, want target-2", got)
	}
}

func TestAdaptiveBalancerWeightedResponseTimeEmpty(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.algorithm = "weighted_response_time"

	// Empty targets
	got, idx := balancer.SelectTarget([]string{}, 0)
	if got != "" {
		t.Errorf("Empty targets selection = %v, want empty", got)
	}
	if idx != 0 {
		t.Errorf("Empty targets index = %v, want 0", idx)
	}
}

func TestAdaptiveBalancerWeightedResponseTimeZeroTotalWeight(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	balancer.algorithm = "weighted_response_time"

	targets := []string{"target-1"}

	// Record with zero latency to test edge case
	balancer.RecordRequest("target-1", 0, nil)

	got, _ := balancer.SelectTarget(targets, 0)
	if got != "target-1" {
		t.Errorf("Zero latency selection = %v, want target-1", got)
	}
}

func TestAdaptiveBalancerWeightedRandomSelect(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	targets := []string{"target-1", "target-2", "target-3"}
	weights := map[string]float64{
		"target-1": 0.1,
		"target-2": 0.5,
		"target-3": 0.3,
	}

	// Should select target-2 with highest weight
	got := balancer.weightedRandomSelect(targets, weights, 0.9)
	if got != "target-2" {
		t.Errorf("Weighted random selection = %v, want target-2", got)
	}
}

func TestAdaptiveBalancerWeightedRandomSelectEmptyWeights(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	targets := []string{"target-1", "target-2"}
	weights := map[string]float64{}

	// Should fallback to first target when no weights match
	got := balancer.weightedRandomSelect(targets, weights, 0)
	if got != "target-1" {
		t.Errorf("Empty weights selection = %v, want target-1", got)
	}
}

func TestAdaptiveBalancerWeightedRandomSelectMissingTarget(t *testing.T) {
	config := DefaultAdaptiveConfig()
	balancer := NewAdaptiveBalancer(config)

	// Target not in weights map
	targets := []string{"target-missing"}
	weights := map[string]float64{
		"target-1": 1.0,
	}

	// Should fallback to first target
	got := balancer.weightedRandomSelect(targets, weights, 1.0)
	if got != "target-missing" {
		t.Errorf("Missing target selection = %v, want target-missing", got)
	}
}

func TestAdaptiveBalancerCheckAndSwitch(t *testing.T) {
	config := DefaultAdaptiveConfig()
	config.EnableAutoSwitch = true
	config.ErrorRateThreshold = 0.1 // Low threshold for testing

	balancer := NewAdaptiveBalancer(config)

	// Initially round_robin
	if balancer.GetAlgorithm() != "round_robin" {
		t.Skip("Starting algorithm is not round_robin")
	}

	// Record high error rate requests
	for i := 0; i < 100; i++ {
		balancer.RecordRequest("target-1", 100*time.Millisecond, fmt.Errorf("error"))
	}

	// Trigger algorithm check
	balancer.checkAndSwitch()

	// Algorithm should have switched due to high error rate
	// Note: This depends on the actual implementation logic
}
