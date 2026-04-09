package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestLeastConnBalancesByActiveConnections(t *testing.T) {
	t.Parallel()

	lc := NewLeastConn([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	})

	t1, _ := lc.Next(nil)
	t2, _ := lc.Next(nil)
	t3, _ := lc.Next(nil)

	if t1.ID != "a" || t2.ID != "b" || t3.ID != "a" {
		t.Fatalf("unexpected least-conn sequence: %q, %q, %q", t1.ID, t2.ID, t3.ID)
	}

	lc.Done("a")
	lc.Done("a")
	lc.Done("b")

	t4, _ := lc.Next(nil)
	if t4.ID != "a" && t4.ID != "b" {
		t.Fatalf("expected a valid target after done() decrements, got %q", t4.ID)
	}
}

func TestIPHashStickySelection(t *testing.T) {
	t.Parallel()

	ih := NewIPHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
		{ID: "c", Address: "10.0.0.3:8080"},
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/x", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	ctx := &RequestContext{Request: req}

	first, err := ih.Next(ctx)
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	for i := 0; i < 30; i++ {
		next, err := ih.Next(ctx)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if next.ID != first.ID {
			t.Fatalf("expected sticky selection %q got %q", first.ID, next.ID)
		}
	}
}

func TestRandomBalancerSkipsUnhealthy(t *testing.T) {
	t.Parallel()

	r := NewRandomBalancer([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	})
	r.ReportHealth("a", false, 0)

	for i := 0; i < 50; i++ {
		target, err := r.Next(nil)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if target.ID != "b" {
			t.Fatalf("expected only healthy target b, got %q", target.ID)
		}
	}
}

func TestConsistentHashSkipsUnhealthy(t *testing.T) {
	t.Parallel()

	ch := NewConsistentHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users/42", nil)
	req.RemoteAddr = "203.0.113.42:9876"
	ctx := &RequestContext{Request: req}

	first, err := ch.Next(ctx)
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}

	ch.ReportHealth(first.ID, false, 0)
	next, err := ch.Next(ctx)
	if err != nil {
		t.Fatalf("Next error after health update: %v", err)
	}
	if next.ID == first.ID {
		t.Fatalf("expected unhealthy target to be skipped")
	}
}

func TestLeastLatencyPrefersLowerLatency(t *testing.T) {
	t.Parallel()

	ll := NewLeastLatency([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	})
	ll.ReportHealth("a", true, 220*time.Millisecond)
	ll.ReportHealth("b", true, 35*time.Millisecond)

	for i := 0; i < 10; i++ {
		target, err := ll.Next(nil)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if target.ID != "b" {
			t.Fatalf("expected low-latency target b, got %q", target.ID)
		}
	}
}

func TestAdaptiveBasicSelection(t *testing.T) {
	t.Parallel()

	a := NewAdaptive([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	})

	for i := 0; i < 40; i++ {
		a.ReportHealth("a", false, 25*time.Millisecond)
	}
	target, err := a.Next(nil)
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if target == nil {
		t.Fatalf("expected a target")
	}
}

func TestSubnetAwarePlaceholderRoundRobin(t *testing.T) {
	t.Parallel()

	g := NewSubnetAware([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
		{ID: "b", Address: "10.0.0.2:8080"},
	})
	counts := map[string]int{"a": 0, "b": 0}
	for i := 0; i < 100; i++ {
		target, err := g.Next(nil)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		counts[target.ID]++
	}
	if counts["a"] != 50 || counts["b"] != 50 {
		t.Fatalf("unexpected distribution: %#v", counts)
	}
}

func TestHealthWeightedDistributionAndHealth(t *testing.T) {
	t.Parallel()

	hw := NewHealthWeighted([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "b", Address: "10.0.0.2:8080", Weight: 3},
	})

	counts := map[string]int{"a": 0, "b": 0}
	for i := 0; i < 500; i++ {
		target, err := hw.Next(nil)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		counts[target.ID]++
	}
	if counts["b"] <= counts["a"] {
		t.Fatalf("expected weighted preference for b, got %#v", counts)
	}

	hw.ReportHealth("b", false, 0)
	for i := 0; i < 40; i++ {
		target, err := hw.Next(nil)
		if err != nil {
			t.Fatalf("Next error after health update: %v", err)
		}
		if target.ID != "a" {
			t.Fatalf("expected unhealthy target b to be skipped")
		}
	}
}

// TestIPHash_Done tests the Done method
func TestIPHash_Done(t *testing.T) {
	targets := []config.UpstreamTarget{
		{ID: "target-1", Address: "localhost:8081"},
		{ID: "target-2", Address: "localhost:8082"},
	}
	ih := NewIPHash(targets)

	// Done should be callable without panic (it's a no-op for IPHash)
	ih.Done("target-1")
	ih.Done("")
}

// TestRandomBalancer_Done tests the Done method
func TestRandomBalancer_Done(t *testing.T) {
	targets := []config.UpstreamTarget{
		{ID: "target-1", Address: "localhost:8081"},
		{ID: "target-2", Address: "localhost:8082"},
	}
	rb := NewRandomBalancer(targets)

	// Done should be callable without panic (it's a no-op for RandomBalancer)
	rb.Done("target-1")
	rb.Done("")
}

// TestConsistentHash_Done tests the Done method
func TestConsistentHash_Done(t *testing.T) {
	targets := []config.UpstreamTarget{
		{ID: "target-1", Address: "localhost:8081"},
		{ID: "target-2", Address: "localhost:8082"},
	}
	ch := NewConsistentHash(targets)

	// Done should be callable without panic (it's a no-op for ConsistentHash)
	ch.Done("target-1")
	ch.Done("")
}

// TestLeastLatency_Done tests the Done method
func TestLeastLatency_Done(t *testing.T) {
	targets := []config.UpstreamTarget{
		{ID: "target-1", Address: "localhost:8081"},
		{ID: "target-2", Address: "localhost:8082"},
	}
	ll := NewLeastLatency(targets)

	// Done should be callable without panic (it's a no-op for LeastLatency)
	ll.Done("target-1")
	ll.Done("")
}

// TestHealthWeighted_Done tests the Done method
func TestHealthWeighted_Done(t *testing.T) {
	targets := []config.UpstreamTarget{
		{ID: "target-1", Address: "localhost:8081"},
		{ID: "target-2", Address: "localhost:8082"},
	}
	hw := NewHealthWeighted(targets)

	// Done should be callable without panic (it's a no-op for HealthWeighted)
	hw.Done("target-1")
	hw.Done("")
}
