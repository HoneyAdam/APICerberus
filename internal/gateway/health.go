package gateway

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TargetHealth keeps active health-check state for a target.
type TargetHealth struct {
	Healthy         bool
	ConsecutiveOK   int
	ConsecutiveFail int
	LastCheck       time.Time
	LastLatency     time.Duration
	Address         string
}

type upstreamHealth struct {
	config       config.HealthCheckConfig
	targets      map[string]*TargetHealth
	passive      passiveConfig
	passiveState map[string]*passiveTargetState
}

type passiveConfig struct {
	window           time.Duration
	errorThreshold   int
	successThreshold int
}

type passiveTargetState struct {
	errorTimes         []time.Time
	consecutiveSuccess int
}

// Checker runs active health checks and updates balancer health reports.
type Checker struct {
	mu        sync.RWMutex
	upstreams map[string]*upstreamHealth
	pools     map[string]*UpstreamPool
}

func NewChecker(upstreams []config.Upstream, pools map[string]*UpstreamPool) *Checker {
	c := &Checker{
		upstreams: make(map[string]*upstreamHealth, len(upstreams)),
		pools:     pools,
	}

	for _, up := range upstreams {
		passiveCfg := derivePassiveConfig(up.HealthCheck.Active)
		uh := &upstreamHealth{
			config:       up.HealthCheck,
			targets:      make(map[string]*TargetHealth, len(up.Targets)),
			passive:      passiveCfg,
			passiveState: make(map[string]*passiveTargetState, len(up.Targets)),
		}
		for _, t := range up.Targets {
			id := targetKey(t)
			uh.targets[id] = &TargetHealth{
				Healthy: true,
				Address: t.Address,
			}
			uh.passiveState[id] = &passiveTargetState{}
			if pool := pools[up.Name]; pool != nil {
				pool.ReportHealth(id, true, 0)
			}
		}
		c.upstreams[up.Name] = uh
	}
	return c
}

// Start launches active health check loops for configured upstreams.
func (c *Checker) Start(ctx context.Context) {
	c.mu.RLock()
	upstreams := make(map[string]*upstreamHealth, len(c.upstreams))
	for name, up := range c.upstreams {
		upstreams[name] = up
	}
	c.mu.RUnlock()

	for name, uh := range upstreams {
		interval := uh.config.Active.Interval
		if interval <= 0 {
			continue
		}
		go c.activeCheckLoop(ctx, name, uh, interval)
	}
}

func (c *Checker) activeCheckLoop(ctx context.Context, upstreamName string, uh *upstreamHealth, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAllTargets(ctx, upstreamName, uh)
		}
	}
}

func (c *Checker) checkAllTargets(ctx context.Context, upstreamName string, uh *upstreamHealth) {
	c.mu.RLock()
	targets := make(map[string]*TargetHealth, len(uh.targets))
	for id, t := range uh.targets {
		clone := *t
		targets[id] = &clone
	}
	checkCfg := uh.config.Active
	c.mu.RUnlock()

	client := &http.Client{Timeout: checkCfg.Timeout}
	for targetID, th := range targets {
		healthy, latency := runHealthCheck(ctx, client, th.Address, checkCfg.Path)
		c.applyHealthResult(upstreamName, targetID, healthy, latency, checkCfg)
	}
}

func runHealthCheck(ctx context.Context, client *http.Client, address, path string) (bool, time.Duration) {
	// SEC-PROXY-002: active health probes previously bypassed the SSRF gate
	// that the proxy path applies via validateUpstreamHost. An admin-lite
	// actor who could register an upstream target (or its DNS record was
	// rebind-flipped to a link-local IP after registration) turned the
	// health endpoint into a reflective oracle against cloud metadata
	// (169.254.169.254) and RFC1918 ranges. Apply the same gate here so
	// the healthy/unhealthy boolean and observed latency can no longer
	// leak information about internal network state.
	if err := validateUpstreamHost(strings.TrimSpace(address)); err != nil {
		return false, 0
	}

	start := time.Now()
	targetPath := normalizePath(path)
	url := "http://" + strings.TrimSpace(address) + targetPath
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return false, latency
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 400, latency
}

func (c *Checker) applyHealthResult(upstreamName, targetID string, ok bool, latency time.Duration, cfg config.ActiveHealthCheckConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	upstream, exists := c.upstreams[upstreamName]
	if !exists {
		return
	}
	target, exists := upstream.targets[targetID]
	if !exists {
		return
	}

	target.LastCheck = time.Now()
	target.LastLatency = latency

	previous := target.Healthy
	if ok {
		target.ConsecutiveOK++
		target.ConsecutiveFail = 0
		if !target.Healthy && target.ConsecutiveOK >= cfg.HealthyThreshold {
			target.Healthy = true
		}
	} else {
		target.ConsecutiveFail++
		target.ConsecutiveOK = 0
		if target.Healthy && target.ConsecutiveFail >= cfg.UnhealthyThreshold {
			target.Healthy = false
		}
	}

	if previous != target.Healthy {
		if pool := c.pools[upstreamName]; pool != nil {
			pool.ReportHealth(targetID, target.Healthy, latency)
		}
	}
}

// IsHealthy returns current target health state.
func (c *Checker) IsHealthy(upstreamName, targetID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	upstream, ok := c.upstreams[upstreamName]
	if !ok {
		return false
	}
	target, ok := upstream.targets[targetID]
	if !ok {
		return false
	}
	return target.Healthy
}

// Snapshot returns a copy of target health states for a given upstream.
func (c *Checker) Snapshot(upstreamName string) map[string]TargetHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	upstream, ok := c.upstreams[upstreamName]
	if !ok {
		return map[string]TargetHealth{}
	}

	out := make(map[string]TargetHealth, len(upstream.targets))
	for id, t := range upstream.targets {
		out[id] = *t
	}
	return out
}

// ReportError records a passive health-check error for target and can mark target unhealthy.
func (c *Checker) ReportError(upstreamName, targetID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	upstream, ok := c.upstreams[upstreamName]
	if !ok {
		return
	}
	target, ok := upstream.targets[targetID]
	if !ok {
		return
	}
	state, ok := upstream.passiveState[targetID]
	if !ok {
		state = &passiveTargetState{}
		upstream.passiveState[targetID] = state
	}

	now := time.Now()
	state.errorTimes = append(state.errorTimes, now)
	state.errorTimes = pruneOldErrors(state.errorTimes, now, upstream.passive.window)
	state.consecutiveSuccess = 0

	if len(state.errorTimes) >= upstream.passive.errorThreshold {
		if target.Healthy {
			target.Healthy = false
			target.ConsecutiveFail = len(state.errorTimes)
			target.ConsecutiveOK = 0
			target.LastCheck = now
			if pool := c.pools[upstreamName]; pool != nil {
				pool.ReportHealth(targetID, false, 0)
			}
		}
	}
}

// ReportSuccess records successful upstream response and can recover passive unhealthy target.
func (c *Checker) ReportSuccess(upstreamName, targetID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	upstream, ok := c.upstreams[upstreamName]
	if !ok {
		return
	}
	target, ok := upstream.targets[targetID]
	if !ok {
		return
	}
	state, ok := upstream.passiveState[targetID]
	if !ok {
		state = &passiveTargetState{}
		upstream.passiveState[targetID] = state
	}

	now := time.Now()
	state.errorTimes = pruneOldErrors(state.errorTimes, now, upstream.passive.window)
	state.consecutiveSuccess++

	if !target.Healthy && state.consecutiveSuccess >= upstream.passive.successThreshold {
		target.Healthy = true
		target.ConsecutiveOK = state.consecutiveSuccess
		target.ConsecutiveFail = 0
		target.LastCheck = now
		state.errorTimes = nil
		if pool := c.pools[upstreamName]; pool != nil {
			pool.ReportHealth(targetID, true, 0)
		}
	}
}

func derivePassiveConfig(active config.ActiveHealthCheckConfig) passiveConfig {
	errorThreshold := active.UnhealthyThreshold
	if errorThreshold <= 0 {
		errorThreshold = 3
	}
	successThreshold := active.HealthyThreshold
	if successThreshold <= 0 {
		successThreshold = 2
	}

	window := active.Interval * time.Duration(maxInt(errorThreshold, 3))
	if window <= 0 {
		window = 30 * time.Second
	}
	if window < 200*time.Millisecond {
		window = 200 * time.Millisecond
	}

	return passiveConfig{
		window:           window,
		errorThreshold:   errorThreshold,
		successThreshold: successThreshold,
	}
}

func pruneOldErrors(in []time.Time, now time.Time, window time.Duration) []time.Time {
	if len(in) == 0 {
		return in
	}
	if window <= 0 {
		return nil
	}
	threshold := now.Add(-window)
	keepFrom := 0
	for keepFrom < len(in) && in[keepFrom].Before(threshold) {
		keepFrom++
	}
	if keepFrom <= 0 {
		return in
	}
	out := make([]time.Time, len(in)-keepFrom)
	copy(out, in[keepFrom:])
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
