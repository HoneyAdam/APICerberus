package gateway

import (
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// Test NewBalancer with different algorithms
func TestNewBalancer(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		wantType  string
	}{
		{
			name:      "round robin default",
			algorithm: "",
			wantType:  "*gateway.RoundRobin",
		},
		{
			name:      "round robin explicit",
			algorithm: "round_robin",
			wantType:  "*gateway.RoundRobin",
		},
		{
			name:      "least_conn",
			algorithm: "least_conn",
			wantType:  "*gateway.LeastConn",
		},
		{
			name:      "ip_hash",
			algorithm: "ip_hash",
			wantType:  "*gateway.IPHash",
		},
		{
			name:      "random",
			algorithm: "random",
			wantType:  "*gateway.RandomBalancer",
		},
		{
			name:      "consistent_hash",
			algorithm: "consistent_hash",
			wantType:  "*gateway.ConsistentHash",
		},
		{
			name:      "weighted_round_robin",
			algorithm: "weighted_round_robin",
			wantType:  "*gateway.WeightedRoundRobin",
		},
		{
			name:      "least_latency",
			algorithm: "least_latency",
			wantType:  "*gateway.LeastLatency",
		},
		{
			name:      "adaptive",
			algorithm: "adaptive",
			wantType:  "*gateway.Adaptive",
		},
		{
			name:      "geo_aware",
			algorithm: "geo_aware",
			wantType:  "*gateway.GeoAware",
		},
		{
			name:      "health_weighted",
			algorithm: "health_weighted",
			wantType:  "*gateway.HealthWeighted",
		},
		{
			name:      "unknown algorithm defaults to round robin",
			algorithm: "unknown",
			wantType:  "*gateway.RoundRobin",
		},
		{
			name:      "case insensitive",
			algorithm: "ROUND_ROBIN",
			wantType:  "*gateway.RoundRobin",
		},
		{
			name:      "with whitespace",
			algorithm: "  round_robin  ",
			wantType:  "*gateway.RoundRobin",
		},
	}

	targets := []config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balancer := NewBalancer(tt.algorithm, targets)
			if balancer == nil {
				t.Fatal("NewBalancer returned nil")
			}

			// Verify the balancer can return targets
			target, err := balancer.Next(nil)
			if err != nil {
				t.Fatalf("Next() error = %v", err)
			}
			if target == nil {
				t.Fatal("Next() returned nil target")
			}
		})
	}
}

// Test NewBalancer with empty targets
func TestNewBalancer_EmptyTargets(t *testing.T) {
	balancer := NewBalancer("round_robin", []config.UpstreamTarget{})
	if balancer == nil {
		t.Fatal("NewBalancer returned nil for empty targets")
	}

	// Should return error when no targets available
	_, err := balancer.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Next() error = %v, want ErrNoHealthyTargets", err)
	}
}
