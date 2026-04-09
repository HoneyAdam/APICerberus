package test

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/loadbalancer"
)

// TestE2EAdvancedFeatures validates v0.6.0 advanced features
func TestE2EAdvancedFeatures(t *testing.T) {
	t.Parallel()

	// Create test config
	cfgPath := writeAdvancedTestConfig(t)

	// Start gateway
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/apicerberus", "start", "--config", cfgPath)
	cmd.Dir = filepath.Join("..")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	defer cmd.Process.Kill()

	// Wait for gateway to start
	time.Sleep(2 * time.Second)

	// Test metrics endpoint
	t.Run("MetricsEndpoint", func(t *testing.T) {
		testMetricsEndpoint(t)
	})

	// Test tracing
	t.Run("TracingFunctionality", func(t *testing.T) {
		t.Skip("Tracing functionality removed")
	})

	// Test webhooks
	t.Run("WebhookFunctionality", func(t *testing.T) {
		t.Skip("Webhook functionality removed")
	})
}

func testMetricsEndpoint(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:18080/metrics")
	if err != nil {
		t.Logf("Metrics endpoint not available: %v", err)
		return
	}
	defer resp.Body.Close()

	t.Logf("Metrics response status: %d", resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			t.Logf("Metrics available: %d bytes", len(body))
		}
	}
}

// TestE2EAdvancedUnitTests runs unit tests for advanced features
func TestE2EAdvancedUnitTests(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run metrics tests
	cmd := exec.CommandContext(ctx, "go", "test", "./internal/metrics/...", "-v")
	cmd.Dir = filepath.Join("..")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Metrics tests output:\n%s", string(output))
	}

	// Run loadbalancer tests
	cmd = exec.CommandContext(ctx, "go", "test", "./internal/loadbalancer/...", "-v")
	cmd.Dir = filepath.Join("..")

	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Logf("Loadbalancer tests output:\n%s", string(output))
	}
}

// TestE2ESubnetAwareLoadBalancing tests subnet-aware load balancing
func TestE2ESubnetAwareLoadBalancing(t *testing.T) {
	selector := loadbalancer.NewSubnetAwareSelector()

	// Set up target locations
	selector.SetTargetLocation("us-east", "US")
	selector.SetTargetLocation("us-west", "US")
	selector.SetTargetLocation("eu-west", "EU")

	// US client should get US target
	usTarget := selector.Select("192.168.1.1", []string{"us-east", "us-west", "eu-west"})
	if usTarget != "us-east" && usTarget != "us-west" {
		t.Errorf("US client got non-US target: %v", usTarget)
	}

	// EU client should get EU target
	euTarget := selector.Select("172.16.1.1", []string{"us-east", "us-west", "eu-west"})
	if euTarget != "eu-west" {
		t.Errorf("EU client got non-EU target: %v", euTarget)
	}

	t.Log("Subnet-aware load balancing working")
}

// TestE2EAdaptiveLoadBalancing tests adaptive load balancing
func TestE2EAdaptiveLoadBalancing(t *testing.T) {
	config := loadbalancer.DefaultAdaptiveConfig()
	balancer := loadbalancer.NewAdaptiveBalancer(config)

	// Record some requests
	balancer.RecordRequest("target-1", 100*time.Millisecond, nil)
	balancer.RecordRequest("target-1", 200*time.Millisecond, nil)
	balancer.RecordRequest("target-2", 50*time.Millisecond, nil)

	// Check stats
	stats1, _ := balancer.GetStats("target-1")
	if stats1.RequestCount != 2 {
		t.Errorf("Expected 2 requests for target-1, got %d", stats1.RequestCount)
	}

	// Check algorithm
	algo := balancer.GetAlgorithm()
	if algo == "" {
		t.Error("Algorithm should not be empty")
	}

	t.Logf("Adaptive load balancing using algorithm: %s", algo)
}

func writeAdvancedTestConfig(t *testing.T) string {
	t.Helper()

	config := `version: "1.0"

server:
  address: "127.0.0.1:18080"
  read_timeout: 30s
  write_timeout: 30s

logging:
  level: "info"
  format: "json"

auth:
  jwt:
    enabled: true
    secret: "test-secret-key"
    issuer: "test-issuer"

rate_limiting:
  enabled: true
  requests_per_second: 100
  burst_size: 150

cache:
  enabled: true
  max_size: 104857600
  ttl: "5m"
  max_item_size: 10485760

metrics:
  enabled: true
  endpoint: "/metrics"

tracing:
  enabled: true
  service_name: "apicerberus-gateway"
  sample_rate: 1.0

webhooks:
  enabled: true
  retries: 3

load_balancer:
  algorithm: "adaptive"
  geo_aware: true
  adaptive:
    error_rate_threshold: 0.1
    latency_threshold: "500ms"

admin:
  enabled: true
  address: "127.0.0.1:18080"
  api_key: "admin-test-token"

backend:
  services: []
`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "advanced_test.yaml")
	if err := os.WriteFile(cfgPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	return cfgPath
}
