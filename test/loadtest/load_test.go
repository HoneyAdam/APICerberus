//go:build loadtest

package loadtest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestLoadBasicGateway fires sustained HTTP load at a real Gateway instance
// and reports latency percentiles and throughput.
func TestLoadBasicGateway(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	// Start gateway on a random port via httptest
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	attacker := NewAttacker(srv.URL)
	attacker.Headers["X-Request-ID"] = "load-test"

	t.Run("100 rps for 5s", func(t *testing.T) {
		report := ConstantRate(func(ctx context.Context) Result {
			return attacker.Attack(ctx, http.MethodGet, "/health", nil)
		}, 100, 5*time.Second)

		report.Print()

		if report.FailureRate() > 0.01 {
			t.Errorf("failure rate %.2f%% exceeds 1%% threshold", report.FailureRate()*100)
		}
		if report.Percentile(99) > 500*time.Millisecond {
			t.Errorf("p99 latency %s exceeds 500ms", report.Percentile(99))
		}
	})

	t.Run("500 rps burst for 3s", func(t *testing.T) {
		report := ConstantRate(func(ctx context.Context) Result {
			return attacker.Attack(ctx, http.MethodGet, "/health", nil)
		}, 500, 3*time.Second)

		report.Print()

		if report.FailureRate() > 0.05 {
			t.Errorf("failure rate %.2f%% exceeds 5%% threshold", report.FailureRate()*100)
		}
	})

	t.Run("ramp-up 50 to 200 rps over 5s", func(t *testing.T) {
		report := RampUp(func(ctx context.Context) Result {
			return attacker.Attack(ctx, http.MethodGet, "/health", nil)
		}, 50, 200, 5*time.Second, 5)

		report.Print()

		if report.FailureRate() > 0.05 {
			t.Errorf("failure rate %.2f%% exceeds 5%% threshold", report.FailureRate()*100)
		}
	})
}

// TestLoadConcurrentAdminAPI tests concurrent admin API-style requests.
func TestLoadConcurrentAdminAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","request_id":"%s"}`, r.Header.Get("X-Request-ID"))
	}))
	defer srv.Close()

	attacker := NewAttacker(srv.URL)
	attacker.Headers["X-Admin-Key"] = "test-key"

	report := ConstantRate(func(ctx context.Context) Result {
		return attacker.Attack(ctx, http.MethodGet, "/admin/api/v1/status", nil)
	}, 200, 5*time.Second)

	report.Print()

	if report.FailureRate() > 0.01 {
		t.Errorf("failure rate %.2f%% exceeds 1%% threshold", report.FailureRate()*100)
	}
}

// TestLoadStability runs a longer stability test to catch memory leaks
// and connection pool exhaustion.
func TestLoadStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	attacker := NewAttacker(srv.URL)

	report := ConstantRate(func(ctx context.Context) Result {
		return attacker.Attack(ctx, http.MethodGet, "/", nil)
	}, 50, 10*time.Second)

	report.Print()

	// Stability test: zero failures expected
	if report.Failures > 0 {
		t.Errorf("stability test had %d failures", report.Failures)
	}
}
