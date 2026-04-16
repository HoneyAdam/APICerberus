package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestCheckerActiveHealthChecksAndBalancerIntegration(t *testing.T) {
	t.Parallel()

	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthySrv.Close()

	var status atomic.Int32
	status.Store(http.StatusInternalServerError)
	flappingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer flappingSrv.Close()

	upstream := config.Upstream{
		Name:      "users-upstream",
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "healthy", Address: mustHost(t, healthySrv.URL), Weight: 1},
			{ID: "flap", Address: mustHost(t, flappingSrv.URL), Weight: 1},
		},
		HealthCheck: config.HealthCheckConfig{
			Active: config.ActiveHealthCheckConfig{
				Path:               "/",
				Interval:           25 * time.Millisecond,
				Timeout:            100 * time.Millisecond,
				HealthyThreshold:   1,
				UnhealthyThreshold: 1,
			},
		},
	}

	pool := NewUpstreamPool(upstream)
	checker := NewChecker([]config.Upstream{upstream}, map[string]*UpstreamPool{
		upstream.Name: pool,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker.Start(ctx)

	waitFor(t, 2*time.Second, func() bool {
		return !checker.IsHealthy(upstream.Name, "flap")
	}, "flapping target should become unhealthy")

	for i := 0; i < 20; i++ {
		target, err := pool.Next(nil)
		if err != nil {
			t.Fatalf("pool.Next error: %v", err)
		}
		if target.ID != "healthy" {
			t.Fatalf("expected only healthy target, got %q", target.ID)
		}
	}

	status.Store(http.StatusOK)
	waitFor(t, 2*time.Second, func() bool {
		return checker.IsHealthy(upstream.Name, "flap")
	}, "flapping target should recover to healthy")
}

func TestCheckerPassiveErrorAndRecovery(t *testing.T) {
	t.Parallel()

	upstream := config.Upstream{
		Name:      "passive-upstream",
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		},
		HealthCheck: config.HealthCheckConfig{
			Active: config.ActiveHealthCheckConfig{
				Path:               "/health",
				Interval:           100 * time.Millisecond,
				Timeout:            50 * time.Millisecond,
				HealthyThreshold:   2,
				UnhealthyThreshold: 2,
			},
		},
	}

	pool := NewUpstreamPool(upstream)
	checker := NewChecker([]config.Upstream{upstream}, map[string]*UpstreamPool{
		upstream.Name: pool,
	})

	checker.ReportError(upstream.Name, "t1")
	if !checker.IsHealthy(upstream.Name, "t1") {
		t.Fatalf("target should remain healthy after first passive error")
	}

	checker.ReportError(upstream.Name, "t1")
	if checker.IsHealthy(upstream.Name, "t1") {
		t.Fatalf("target should become unhealthy after threshold")
	}

	if _, err := pool.Next(nil); !errors.Is(err, ErrNoHealthyTargets) {
		t.Fatalf("expected no healthy targets after passive mark, got %v", err)
	}

	checker.ReportSuccess(upstream.Name, "t1")
	if checker.IsHealthy(upstream.Name, "t1") {
		t.Fatalf("target should still be unhealthy until success threshold")
	}

	checker.ReportSuccess(upstream.Name, "t1")
	if !checker.IsHealthy(upstream.Name, "t1") {
		t.Fatalf("target should recover after consecutive successes")
	}
	if _, err := pool.Next(nil); err != nil {
		t.Fatalf("pool should serve recovered target: %v", err)
	}
}

func TestCheckerPassiveErrorWindowExpiry(t *testing.T) {
	t.Parallel()

	upstream := config.Upstream{
		Name:      "passive-window",
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		},
		HealthCheck: config.HealthCheckConfig{
			Active: config.ActiveHealthCheckConfig{
				Path:               "/health",
				Interval:           100 * time.Millisecond,
				Timeout:            50 * time.Millisecond,
				HealthyThreshold:   1,
				UnhealthyThreshold: 2,
			},
		},
	}

	pool := NewUpstreamPool(upstream)
	checker := NewChecker([]config.Upstream{upstream}, map[string]*UpstreamPool{
		upstream.Name: pool,
	})

	checker.ReportError(upstream.Name, "t1")
	time.Sleep(450 * time.Millisecond) // > derived passive window (~300ms)
	checker.ReportError(upstream.Name, "t1")

	if !checker.IsHealthy(upstream.Name, "t1") {
		t.Fatalf("old passive error should expire and not trigger unhealthy state")
	}
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u.Host
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", msg)
}

// TestRunHealthCheck_BlocksCloudMetadataSSRF verifies the SEC-PROXY-002 fix:
// active health probes must go through the same validateUpstreamHost gate
// the proxy path uses, so the healthy/unhealthy boolean and latency cannot
// be turned into a reflective oracle that reveals cloud-metadata or other
// link-local reachability. The probe must fail without issuing an HTTP
// request.
func TestRunHealthCheck_BlocksCloudMetadataSSRF(t *testing.T) {
	t.Parallel()

	blocked := []string{
		"169.254.169.254:80",   // AWS / GCP IMDS
		"169.254.169.254:8080", // same host, different port
		"0.0.0.0:80",           // unspecified
		"[::]:80",              // IPv6 unspecified
	}
	for _, addr := range blocked {
		t.Run(addr, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// Use a client with a very short timeout so if the SSRF gate
			// is missing and the dial is actually attempted, the test still
			// completes quickly — but the expected path is "rejected before
			// dial" with latency == 0.
			client := &http.Client{Timeout: 200 * time.Millisecond}
			healthy, latency := runHealthCheck(ctx, client, addr, "/health")
			if healthy {
				t.Fatalf("expected healthy=false for blocked address %q", addr)
			}
			if latency != 0 {
				t.Fatalf("expected latency=0 for rejected address %q (probe must not be issued), got %v",
					addr, latency)
			}
		})
	}
}

// TestRunHealthCheck_AllowsPublicHost verifies the SSRF gate does not
// accidentally block legitimate upstream addresses.
func TestRunHealthCheck_AllowsPublicHost(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// mustHost returns "127.0.0.1:<port>" for the test server. When
	// denyPrivateUpstreams is false (default), loopback is permitted.
	client := &http.Client{Timeout: time.Second}
	healthy, _ := runHealthCheck(ctx, client, mustHost(t, srv.URL), "/")
	if !healthy {
		t.Fatalf("expected healthy=true for legitimate test server")
	}
}
