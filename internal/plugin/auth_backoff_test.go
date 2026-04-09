package plugin

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestAuthBackoff_CheckNoFailures(t *testing.T) {
	ab := NewAuthBackoff()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	if delay := ab.Check(req); delay != 0 {
		t.Errorf("Expected no delay, got %v", delay)
	}
}

func TestAuthBackoff_RecordFailure(t *testing.T) {
	ab := NewAuthBackoff()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	delay := ab.RecordFailure(req)
	if delay != ab.initialDelay {
		t.Errorf("First failure delay = %v, want %v", delay, ab.initialDelay)
	}
}

func TestAuthBackoff_ExponentialBackoff(t *testing.T) {
	ab := NewAuthBackoff()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	// Failure 1: 100ms
	d1 := ab.RecordFailure(req)
	if d1 != 100*time.Millisecond {
		t.Errorf("Failure 1 delay = %v, want 100ms", d1)
	}

	// Failure 2: 200ms
	d2 := ab.RecordFailure(req)
	if d2 != 200*time.Millisecond {
		t.Errorf("Failure 2 delay = %v, want 200ms", d2)
	}

	// Failure 3: 400ms
	d3 := ab.RecordFailure(req)
	if d3 != 400*time.Millisecond {
		t.Errorf("Failure 3 delay = %v, want 400ms", d3)
	}

	// Failure 4: 800ms
	d4 := ab.RecordFailure(req)
	if d4 != 800*time.Millisecond {
		t.Errorf("Failure 4 delay = %v, want 800ms", d4)
	}
}

func TestAuthBackoff_CheckReturnsDelay(t *testing.T) {
	ab := NewAuthBackoff()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	ab.RecordFailure(req)

	// Check should return a delay since failure just happened
	delay := ab.Check(req)
	if delay <= 0 {
		t.Error("Expected positive delay after failure")
	}
}

func TestAuthBackoff_RecordSuccess_Clears(t *testing.T) {
	ab := NewAuthBackoff()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	ab.RecordFailure(req)
	ab.RecordFailure(req)

	if ab.Len() != 1 {
		t.Errorf("Expected 1 tracked IP, got %d", ab.Len())
	}

	ab.RecordSuccess(req)

	if ab.Len() != 0 {
		t.Errorf("Expected 0 tracked IPs after success, got %d", ab.Len())
	}
}

func TestAuthBackoff_DifferentIPs(t *testing.T) {
	ab := NewAuthBackoff()
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.2:1234"

	ab.RecordFailure(req1)
	ab.RecordFailure(req2)

	if ab.Len() != 2 {
		t.Errorf("Expected 2 tracked IPs, got %d", ab.Len())
	}

	// req2 should not be delayed by req1's failures
	delay := ab.Check(req2)
	if delay <= 0 {
		t.Error("Expected delay for req2 after its failure")
	}
}

func TestAuthBackoff_MaxDelay(t *testing.T) {
	ab := NewAuthBackoff()
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	// Record many failures to hit the max delay cap
	for i := 0; i < 20; i++ {
		ab.RecordFailure(req)
	}

	delay := ab.RecordFailure(req)
	if delay != ab.maxDelay {
		t.Errorf("Expected max delay %v, got %v", ab.maxDelay, delay)
	}
}

func TestAuthBackoff_NilRequest(t *testing.T) {
	ab := NewAuthBackoff()

	if delay := ab.Check(nil); delay != 0 {
		t.Errorf("Expected no delay for nil request, got %v", delay)
	}
	if delay := ab.RecordFailure(nil); delay != 0 {
		t.Errorf("Expected no delay for nil request, got %v", delay)
	}
	ab.RecordSuccess(nil) // should not panic
}

func TestAuthBackoff_IntegrationWithAuthAPIKey(t *testing.T) {
	ab := NewAuthBackoff()
	auth := NewAuthAPIKey([]config.Consumer{}, AuthAPIKeyOptions{
		Backoff: ab,
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-API-Key", "wrong-key")

	// First attempt should fail with invalid key
	_, err := auth.Authenticate(req)
	if err == nil {
		t.Error("Expected auth error for wrong key")
	}

	// Second attempt should be rate limited (backoff still active)
	delay := ab.Check(req)
	if delay <= 0 {
		t.Error("Expected backoff delay after failed auth")
	}
}

func TestAuthBackoff_ConfigConsumer(t *testing.T) {
	// Reuse the config.Consumer type from the actual import
	// This test just verifies the AuthAPIKey still works with the backoff option
	consumers := []config.Consumer{
		{
			Name: "test-consumer",
			APIKeys: []config.ConsumerAPIKey{
				{Key: "ck_live_valid-key-1234567890"},
			},
		},
	}

	ab := NewAuthBackoff()
	auth := NewAuthAPIKey(consumers, AuthAPIKeyOptions{
		Backoff: ab,
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-API-Key", "ck_live_valid-key-1234567890")

	consumer, err := auth.Authenticate(req)
	if err != nil {
		t.Errorf("Unexpected auth error: %v", err)
	}
	if consumer.Name != "test-consumer" {
		t.Errorf("Expected test-consumer, got %s", consumer.Name)
	}

	// Success should clear backoff
	if ab.Len() != 0 {
		t.Errorf("Expected 0 tracked IPs after success, got %d", ab.Len())
	}
}
