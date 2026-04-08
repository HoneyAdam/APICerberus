package gateway

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

var ErrNoHealthyTargets = errors.New("no healthy upstream target available")

// Balancer selects a target from an upstream pool.
type Balancer interface {
	Next(ctx *RequestContext) (*config.UpstreamTarget, error)
	UpdateTargets(targets []config.UpstreamTarget)
	ReportHealth(targetID string, healthy bool, _ time.Duration)
	Done(targetID string)
}

// NewBalancer creates a balancer for the requested algorithm.
func NewBalancer(algorithm string, targets []config.UpstreamTarget) Balancer {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "least_conn":
		return NewLeastConn(targets)
	case "ip_hash":
		return NewIPHash(targets)
	case "random":
		return NewRandomBalancer(targets)
	case "consistent_hash":
		return NewConsistentHash(targets)
	case "least_latency":
		return NewLeastLatency(targets)
	case "adaptive":
		return NewAdaptive(targets)
	case "geo_aware":
		return NewGeoAware(targets)
	case "health_weighted":
		return NewHealthWeighted(targets)
	case "weighted_round_robin":
		return NewWeightedRoundRobin(targets)
	case "round_robin", "":
		fallthrough
	default:
		return NewRoundRobin(targets)
	}
}

// RoundRobin uses atomic cursor modulo healthy target count.
type RoundRobin struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
	counter atomic.Uint64
}

func NewRoundRobin(targets []config.UpstreamTarget) *RoundRobin {
	rr := &RoundRobin{}
	rr.UpdateTargets(targets)
	return rr
}

func (rr *RoundRobin) Next(_ *RequestContext) (*config.UpstreamTarget, error) {
	healthy := rr.healthyTargets()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyTargets
	}
	idx := int(rr.counter.Add(1)-1) % len(healthy) // #nosec G115 -- len(healthy) is guaranteed > 0 and fits in int.
	selected := healthy[idx]
	return &selected, nil
}

func (rr *RoundRobin) UpdateTargets(targets []config.UpstreamTarget) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	rr.targets = cloneTargets(targets)
	if rr.health == nil {
		rr.health = make(map[string]bool, len(targets))
	}

	newHealth := make(map[string]bool, len(targets))
	for _, t := range rr.targets {
		key := targetKey(t)
		healthy, ok := rr.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
	}
	rr.health = newHealth
}

func (rr *RoundRobin) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if rr.health == nil {
		rr.health = make(map[string]bool)
	}
	rr.health[targetID] = healthy
}

func (rr *RoundRobin) Done(_ string) {}

func (rr *RoundRobin) healthyTargets() []config.UpstreamTarget {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	out := make([]config.UpstreamTarget, 0, len(rr.targets))
	for _, t := range rr.targets {
		key := targetKey(t)
		healthy, ok := rr.health[key]
		if !ok || healthy {
			out = append(out, t)
		}
	}
	return out
}

// WeightedRoundRobin expands healthy targets by weight and rotates atomically.
type WeightedRoundRobin struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
	counter atomic.Uint64
}

func NewWeightedRoundRobin(targets []config.UpstreamTarget) *WeightedRoundRobin {
	w := &WeightedRoundRobin{}
	w.UpdateTargets(targets)
	return w
}

func (w *WeightedRoundRobin) Next(_ *RequestContext) (*config.UpstreamTarget, error) {
	expanded := w.expandedHealthyTargets()
	if len(expanded) == 0 {
		return nil, ErrNoHealthyTargets
	}
	idx := int(w.counter.Add(1)-1) % len(expanded) // #nosec G115 -- len(expanded) is guaranteed > 0 and fits in int.
	selected := expanded[idx]
	return &selected, nil
}

func (w *WeightedRoundRobin) UpdateTargets(targets []config.UpstreamTarget) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.targets = cloneTargets(targets)
	if w.health == nil {
		w.health = make(map[string]bool, len(targets))
	}

	newHealth := make(map[string]bool, len(targets))
	for _, t := range w.targets {
		key := targetKey(t)
		healthy, ok := w.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
	}
	w.health = newHealth
}

func (w *WeightedRoundRobin) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.health == nil {
		w.health = make(map[string]bool)
	}
	w.health[targetID] = healthy
}

func (w *WeightedRoundRobin) Done(_ string) {}

func (w *WeightedRoundRobin) expandedHealthyTargets() []config.UpstreamTarget {
	w.mu.RLock()
	defer w.mu.RUnlock()

	expanded := make([]config.UpstreamTarget, 0)
	for _, t := range w.targets {
		key := targetKey(t)
		healthy, ok := w.health[key]
		if ok && !healthy {
			continue
		}

		weight := t.Weight
		if weight <= 0 {
			weight = 1
		}
		for i := 0; i < weight; i++ {
			expanded = append(expanded, t)
		}
	}
	return expanded
}

// UpstreamPool groups targets, health state and balancing strategy.
type UpstreamPool struct {
	mu       sync.RWMutex
	upstream config.Upstream
	balancer Balancer
	health   map[string]bool
}

func NewUpstreamPool(upstream config.Upstream) *UpstreamPool {
	targets := cloneTargets(upstream.Targets)
	pool := &UpstreamPool{
		upstream: upstream,
		balancer: NewBalancer(upstream.Algorithm, targets),
		health:   make(map[string]bool, len(targets)),
	}
	for _, t := range targets {
		pool.health[targetKey(t)] = true
	}
	return pool
}

func (p *UpstreamPool) Next(ctx *RequestContext) (*config.UpstreamTarget, error) {
	p.mu.RLock()
	b := p.balancer
	p.mu.RUnlock()
	return b.Next(ctx)
}

func (p *UpstreamPool) Done(targetID string) {
	p.mu.RLock()
	b := p.balancer
	p.mu.RUnlock()
	if b != nil {
		b.Done(targetID)
	}
}

func (p *UpstreamPool) Name() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if strings.TrimSpace(p.upstream.Name) != "" {
		return p.upstream.Name
	}
	return strings.TrimSpace(p.upstream.ID)
}

func (p *UpstreamPool) UpdateTargets(targets []config.UpstreamTarget) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.upstream.Targets = cloneTargets(targets)
	p.balancer.UpdateTargets(targets)

	newHealth := make(map[string]bool, len(targets))
	for _, t := range targets {
		key := targetKey(t)
		healthy, ok := p.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
		p.balancer.ReportHealth(key, healthy, 0)
	}
	p.health = newHealth
}

func (p *UpstreamPool) ReportHealth(targetID string, healthy bool, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.health[targetID] = healthy
	p.balancer.ReportHealth(targetID, healthy, latency)
}

func (p *UpstreamPool) IsHealthy(targetID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	healthy, ok := p.health[targetID]
	if !ok {
		return false
	}
	return healthy
}

func cloneTargets(in []config.UpstreamTarget) []config.UpstreamTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.UpstreamTarget, len(in))
	copy(out, in)
	return out
}

func targetKey(target config.UpstreamTarget) string {
	if strings.TrimSpace(target.ID) != "" {
		return target.ID
	}
	return target.Address
}
