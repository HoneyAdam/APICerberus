package gateway

import (
	"hash/crc32"
	"hash/fnv"
	randv2 "math/rand/v2"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/loadbalancer"
)

// LeastConn selects the target with the fewest active in-flight requests.
type LeastConn struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
	active  map[string]int64
	counter atomic.Uint64
}

func NewLeastConn(targets []config.UpstreamTarget) *LeastConn {
	lc := &LeastConn{}
	lc.UpdateTargets(targets)
	return lc
}

func (lc *LeastConn) Next(_ *RequestContext) (*config.UpstreamTarget, error) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	var (
		minActive int64 = 1<<63 - 1
		cands           = make([]config.UpstreamTarget, 0, len(lc.targets))
	)
	for _, t := range lc.targets {
		key := targetKey(t)
		if healthy, ok := lc.health[key]; ok && !healthy {
			continue
		}
		current := lc.active[key]
		if current < minActive {
			minActive = current
			cands = cands[:0]
			cands = append(cands, t)
			continue
		}
		if current == minActive {
			cands = append(cands, t)
		}
	}
	if len(cands) == 0 {
		return nil, ErrNoHealthyTargets
	}

	idx := int(lc.counter.Add(1)-1) % len(cands) // #nosec G115 -- len(cands) is guaranteed > 0 and fits in int.
	selected := cands[idx]
	lc.active[targetKey(selected)]++
	return &selected, nil
}

func (lc *LeastConn) UpdateTargets(targets []config.UpstreamTarget) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.targets = cloneTargets(targets)
	if lc.health == nil {
		lc.health = make(map[string]bool, len(targets))
	}
	if lc.active == nil {
		lc.active = make(map[string]int64, len(targets))
	}

	newHealth := make(map[string]bool, len(targets))
	newActive := make(map[string]int64, len(targets))
	for _, t := range lc.targets {
		key := targetKey(t)
		healthy, ok := lc.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
		newActive[key] = lc.active[key]
	}
	lc.health = newHealth
	lc.active = newActive
}

func (lc *LeastConn) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.health == nil {
		lc.health = make(map[string]bool)
	}
	lc.health[targetID] = healthy
}

func (lc *LeastConn) Done(targetID string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.active == nil {
		return
	}
	if lc.active[targetID] > 0 {
		lc.active[targetID]--
	}
}

// IPHash hashes client IP and keeps sticky target selection.
type IPHash struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
	counter atomic.Uint64
}

func NewIPHash(targets []config.UpstreamTarget) *IPHash {
	ih := &IPHash{}
	ih.UpdateTargets(targets)
	return ih
}

func (ih *IPHash) Next(ctx *RequestContext) (*config.UpstreamTarget, error) {
	healthy := ih.healthyTargets()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyTargets
	}

	key := affinityKey(ctx)
	if key == "" {
		idx := int(ih.counter.Add(1)-1) % len(healthy) // #nosec G115 -- len(healthy) is guaranteed > 0 and fits in int.
		selected := healthy[idx]
		return &selected, nil
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := int(h.Sum32() % uint32(len(healthy))) // #nosec G115 -- len(healthy) is guaranteed > 0 here.
	selected := healthy[idx]
	return &selected, nil
}

func (ih *IPHash) UpdateTargets(targets []config.UpstreamTarget) {
	ih.mu.Lock()
	defer ih.mu.Unlock()

	ih.targets = cloneTargets(targets)
	if ih.health == nil {
		ih.health = make(map[string]bool, len(targets))
	}
	newHealth := make(map[string]bool, len(targets))
	for _, t := range ih.targets {
		key := targetKey(t)
		healthy, ok := ih.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
	}
	ih.health = newHealth
}

func (ih *IPHash) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	if ih.health == nil {
		ih.health = make(map[string]bool)
	}
	ih.health[targetID] = healthy
}

func (ih *IPHash) Done(_ string) {}

func (ih *IPHash) healthyTargets() []config.UpstreamTarget {
	ih.mu.RLock()
	defer ih.mu.RUnlock()

	out := make([]config.UpstreamTarget, 0, len(ih.targets))
	for _, t := range ih.targets {
		key := targetKey(t)
		if healthy, ok := ih.health[key]; ok && !healthy {
			continue
		}
		out = append(out, t)
	}
	return out
}

// RandomBalancer picks a healthy target at random.
type RandomBalancer struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
}

func NewRandomBalancer(targets []config.UpstreamTarget) *RandomBalancer {
	r := &RandomBalancer{}
	r.UpdateTargets(targets)
	return r
}

func (r *RandomBalancer) Next(_ *RequestContext) (*config.UpstreamTarget, error) {
	healthy := r.healthyTargets()
	if len(healthy) == 0 {
		return nil, ErrNoHealthyTargets
	}
	idx := randv2.IntN(len(healthy)) // #nosec G404 -- math/rand/v2 is acceptable for load-balancing random selection.
	selected := healthy[idx]
	return &selected, nil
}

func (r *RandomBalancer) UpdateTargets(targets []config.UpstreamTarget) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.targets = cloneTargets(targets)
	if r.health == nil {
		r.health = make(map[string]bool, len(targets))
	}
	newHealth := make(map[string]bool, len(targets))
	for _, t := range r.targets {
		key := targetKey(t)
		healthy, ok := r.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
	}
	r.health = newHealth
}

func (r *RandomBalancer) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.health == nil {
		r.health = make(map[string]bool)
	}
	r.health[targetID] = healthy
}

func (r *RandomBalancer) Done(_ string) {}

func (r *RandomBalancer) healthyTargets() []config.UpstreamTarget {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]config.UpstreamTarget, 0, len(r.targets))
	for _, t := range r.targets {
		key := targetKey(t)
		if healthy, ok := r.health[key]; ok && !healthy {
			continue
		}
		out = append(out, t)
	}
	return out
}

type consistentHashEntry struct {
	hash   uint32
	target config.UpstreamTarget
}

// ConsistentHash uses virtual-node ring for affinity routing.
type ConsistentHash struct {
	mu       sync.RWMutex
	targets  []config.UpstreamTarget
	health   map[string]bool
	ring     []consistentHashEntry
	replicas int
}

func NewConsistentHash(targets []config.UpstreamTarget) *ConsistentHash {
	ch := &ConsistentHash{replicas: 120}
	ch.UpdateTargets(targets)
	return ch
}

func (ch *ConsistentHash) Next(ctx *RequestContext) (*config.UpstreamTarget, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return nil, ErrNoHealthyTargets
	}
	key := affinityKey(ctx)
	if key == "" && ctx != nil && ctx.Request != nil && strings.TrimSpace(ctx.Request.URL.Path) != "" {
		key = strings.TrimSpace(ctx.Request.URL.Path)
	}
	if key == "" {
		key = "default"
	}
	hash := crc32.ChecksumIEEE([]byte(key))
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i].hash >= hash
	})
	if idx >= len(ch.ring) {
		idx = 0
	}

	for i := 0; i < len(ch.ring); i++ {
		entry := ch.ring[(idx+i)%len(ch.ring)]
		targetID := targetKey(entry.target)
		if healthy, ok := ch.health[targetID]; ok && !healthy {
			continue
		}
		selected := entry.target
		return &selected, nil
	}
	return nil, ErrNoHealthyTargets
}

func (ch *ConsistentHash) UpdateTargets(targets []config.UpstreamTarget) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.targets = cloneTargets(targets)
	if ch.health == nil {
		ch.health = make(map[string]bool, len(targets))
	}
	newHealth := make(map[string]bool, len(targets))
	for _, t := range ch.targets {
		key := targetKey(t)
		healthy, ok := ch.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
	}
	ch.health = newHealth
	ch.rebuildRingLocked()
}

func (ch *ConsistentHash) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.health == nil {
		ch.health = make(map[string]bool)
	}
	ch.health[targetID] = healthy
}

func (ch *ConsistentHash) Done(_ string) {}

func (ch *ConsistentHash) rebuildRingLocked() {
	if ch.replicas <= 0 {
		ch.replicas = 120
	}
	ring := make([]consistentHashEntry, 0, len(ch.targets)*ch.replicas)
	for _, target := range ch.targets {
		key := targetKey(target)
		for i := 0; i < ch.replicas; i++ {
			token := key + "#" + strconv.Itoa(i)
			ring = append(ring, consistentHashEntry{
				hash:   crc32.ChecksumIEEE([]byte(token)),
				target: target,
			})
		}
	}
	sort.Slice(ring, func(i, j int) bool {
		return ring[i].hash < ring[j].hash
	})
	ch.ring = ring
}

// LeastLatency chooses the healthiest target with the best observed latency.
type LeastLatency struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
	latency map[string]time.Duration
	counter atomic.Uint64
	alpha   float64
}

func NewLeastLatency(targets []config.UpstreamTarget) *LeastLatency {
	ll := &LeastLatency{alpha: 0.30}
	ll.UpdateTargets(targets)
	return ll
}

func (ll *LeastLatency) Next(_ *RequestContext) (*config.UpstreamTarget, error) {
	ll.mu.RLock()
	defer ll.mu.RUnlock()

	healthy := make([]config.UpstreamTarget, 0, len(ll.targets))
	for _, t := range ll.targets {
		key := targetKey(t)
		if healthyState, ok := ll.health[key]; ok && !healthyState {
			continue
		}
		healthy = append(healthy, t)
	}
	if len(healthy) == 0 {
		return nil, ErrNoHealthyTargets
	}

	best := make([]config.UpstreamTarget, 0, len(healthy))
	var bestLat time.Duration
	hasBest := false
	for _, t := range healthy {
		lat, ok := ll.latency[targetKey(t)]
		if !ok || lat <= 0 {
			continue
		}
		if !hasBest || lat < bestLat {
			hasBest = true
			bestLat = lat
			best = best[:0]
			best = append(best, t)
			continue
		}
		if lat == bestLat {
			best = append(best, t)
		}
	}

	if !hasBest || len(best) == 0 {
		idx := int(ll.counter.Add(1)-1) % len(healthy) // #nosec G115 -- len(healthy) is guaranteed > 0 and fits in int.
		selected := healthy[idx]
		return &selected, nil
	}
	idx := int(ll.counter.Add(1)-1) % len(best) // #nosec G115 -- len(best) is guaranteed > 0 and fits in int.
	selected := best[idx]
	return &selected, nil
}

func (ll *LeastLatency) UpdateTargets(targets []config.UpstreamTarget) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	ll.targets = cloneTargets(targets)
	if ll.health == nil {
		ll.health = make(map[string]bool, len(targets))
	}
	if ll.latency == nil {
		ll.latency = make(map[string]time.Duration, len(targets))
	}

	newHealth := make(map[string]bool, len(targets))
	newLatency := make(map[string]time.Duration, len(targets))
	for _, t := range ll.targets {
		key := targetKey(t)
		healthy, ok := ll.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
		if l, ok := ll.latency[key]; ok {
			newLatency[key] = l
		}
	}
	ll.health = newHealth
	ll.latency = newLatency
}

func (ll *LeastLatency) ReportHealth(targetID string, healthy bool, latency time.Duration) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	if ll.health == nil {
		ll.health = make(map[string]bool)
	}
	if ll.latency == nil {
		ll.latency = make(map[string]time.Duration)
	}
	ll.health[targetID] = healthy

	if latency <= 0 {
		return
	}
	previous, ok := ll.latency[targetID]
	if !ok || previous <= 0 {
		ll.latency[targetID] = latency
		return
	}
	updated := time.Duration(ll.alpha*float64(latency) + (1.0-ll.alpha)*float64(previous))
	if updated <= 0 {
		updated = latency
	}
	ll.latency[targetID] = updated
}

func (ll *LeastLatency) Done(_ string) {}

// Adaptive switches strategy based on observed errors and latency.
type Adaptive struct {
	mu          sync.RWMutex
	rr          *RoundRobin
	leastConn   *LeastConn
	leastLat    *LeastLatency
	errorCount  uint64
	totalCount  uint64
	latencyEWMA time.Duration
	alpha       float64
}

func NewAdaptive(targets []config.UpstreamTarget) *Adaptive {
	return &Adaptive{
		rr:        NewRoundRobin(targets),
		leastConn: NewLeastConn(targets),
		leastLat:  NewLeastLatency(targets),
		alpha:     0.25,
	}
}

func (a *Adaptive) Next(ctx *RequestContext) (*config.UpstreamTarget, error) {
	switch a.mode() {
	case "least_conn":
		return a.leastConn.Next(ctx)
	case "least_latency":
		return a.leastLat.Next(ctx)
	default:
		return a.rr.Next(ctx)
	}
}

func (a *Adaptive) UpdateTargets(targets []config.UpstreamTarget) {
	a.rr.UpdateTargets(targets)
	a.leastConn.UpdateTargets(targets)
	a.leastLat.UpdateTargets(targets)
}

func (a *Adaptive) ReportHealth(targetID string, healthy bool, latency time.Duration) {
	a.mu.Lock()
	a.totalCount++
	if !healthy {
		a.errorCount++
	}
	if latency > 0 {
		if a.latencyEWMA <= 0 {
			a.latencyEWMA = latency
		} else {
			a.latencyEWMA = time.Duration(a.alpha*float64(latency) + (1.0-a.alpha)*float64(a.latencyEWMA))
		}
	}
	if a.totalCount > 50_000 {
		a.totalCount /= 2
		a.errorCount /= 2
	}
	a.mu.Unlock()

	a.rr.ReportHealth(targetID, healthy, latency)
	a.leastConn.ReportHealth(targetID, healthy, latency)
	a.leastLat.ReportHealth(targetID, healthy, latency)
}

func (a *Adaptive) Done(targetID string) {
	a.leastConn.Done(targetID)
}

func (a *Adaptive) mode() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.totalCount >= 20 {
		errorRate := float64(a.errorCount) / float64(a.totalCount)
		if errorRate >= 0.25 {
			return "least_conn"
		}
	}
	if a.latencyEWMA >= 200*time.Millisecond {
		return "least_latency"
	}
	return "round_robin"
}

// GeoAware selects targets based on the client's geographic location.
// It resolves the client IP to a country using the GeoIPResolver, then
// picks a target in the same country via the GeoAwareSelector. When no
// geo match is found it falls back to round-robin selection.
type GeoAware struct {
	mu       sync.RWMutex
	targets  []config.UpstreamTarget
	health   map[string]bool
	rr       *RoundRobin
	resolver *loadbalancer.GeoIPResolver
	selector *loadbalancer.GeoAwareSelector
}

// NewGeoAware creates a geo-aware balancer. Target locations are derived
// from the first two octets of each target's address via the GeoIP resolver.
func NewGeoAware(targets []config.UpstreamTarget) *GeoAware {
	g := &GeoAware{
		rr:       NewRoundRobin(targets),
		resolver: loadbalancer.NewGeoIPResolver(),
		selector: loadbalancer.NewGeoAwareSelector(),
	}
	g.UpdateTargets(targets)
	return g
}

// registerTargetLocations resolves each target's address to a country code
// and registers the mapping with the GeoAwareSelector.
func (g *GeoAware) registerTargetLocations() {
	for _, t := range g.targets {
		key := targetKey(t)
		// Extract the host portion of the target address.
		host := t.Address
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		country := g.resolver.Resolve(host)
		g.selector.SetTargetLocation(key, country)
	}
}

func (g *GeoAware) Next(ctx *RequestContext) (*config.UpstreamTarget, error) {
	// Collect healthy target IDs.
	g.mu.RLock()
	healthy := make([]config.UpstreamTarget, 0, len(g.targets))
	for _, t := range g.targets {
		key := targetKey(t)
		if h, ok := g.health[key]; ok && !h {
			continue
		}
		healthy = append(healthy, t)
	}
	g.mu.RUnlock()

	if len(healthy) == 0 {
		return nil, ErrNoHealthyTargets
	}

	// Extract client IP from the request context.
	clientIP := extractClientIP(ctx)
	if clientIP != "" {
		ids := make([]string, len(healthy))
		idToTarget := make(map[string]config.UpstreamTarget, len(healthy))
		for i, t := range healthy {
			key := targetKey(t)
			ids[i] = key
			idToTarget[key] = t
		}

		selected := g.selector.Select(clientIP, ids)
		if target, ok := idToTarget[selected]; ok {
			// Only return the geo match if the selector actually found a
			// country match (not just the fallback first-element).
			clientCountry := g.resolver.Resolve(clientIP)
			g.mu.RLock()
			// Check: did the selector find a real geo match?
			for _, t := range healthy {
				key := targetKey(t)
				host := t.Address
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				targetCountry := g.resolver.Resolve(host)
				if key == selected && targetCountry == clientCountry && clientCountry != "UNKNOWN" {
					g.mu.RUnlock()
					return &target, nil
				}
			}
			g.mu.RUnlock()
		}
	}

	// Fall back to round-robin when geo resolution fails or no match.
	return g.rr.Next(ctx)
}

func (g *GeoAware) UpdateTargets(targets []config.UpstreamTarget) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.targets = cloneTargets(targets)
	if g.health == nil {
		g.health = make(map[string]bool, len(targets))
	}
	newHealth := make(map[string]bool, len(targets))
	for _, t := range g.targets {
		key := targetKey(t)
		healthy, ok := g.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy
	}
	g.health = newHealth

	g.registerTargetLocations()
	g.rr.UpdateTargets(targets)
}

func (g *GeoAware) ReportHealth(targetID string, healthy bool, latency time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.health == nil {
		g.health = make(map[string]bool)
	}
	g.health[targetID] = healthy
	g.rr.ReportHealth(targetID, healthy, latency)
}

func (g *GeoAware) Done(targetID string) {
	g.rr.Done(targetID)
}

// extractClientIP returns the client IP from a RequestContext.
func extractClientIP(ctx *RequestContext) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	req := ctx.Request

	// Prefer X-Forwarded-For header.
	if xff := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if first != "" {
				return first
			}
		}
	}

	// Fall back to RemoteAddr.
	if host, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr)); err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(req.RemoteAddr)
}

// HealthWeighted chooses targets by score = health_score * configured weight.
type HealthWeighted struct {
	mu      sync.RWMutex
	targets []config.UpstreamTarget
	health  map[string]bool
	score   map[string]float64
}

func NewHealthWeighted(targets []config.UpstreamTarget) *HealthWeighted {
	hw := &HealthWeighted{}
	hw.UpdateTargets(targets)
	return hw
}

func (hw *HealthWeighted) Next(_ *RequestContext) (*config.UpstreamTarget, error) {
	hw.mu.RLock()
	defer hw.mu.RUnlock()

	type candidate struct {
		target config.UpstreamTarget
		weight float64
	}
	candidates := make([]candidate, 0, len(hw.targets))
	total := 0.0
	for _, t := range hw.targets {
		key := targetKey(t)
		if healthy, ok := hw.health[key]; ok && !healthy {
			continue
		}
		score := hw.score[key]
		if score <= 0 {
			continue
		}
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		weighted := float64(w) * score
		if weighted <= 0 {
			continue
		}
		candidates = append(candidates, candidate{target: t, weight: weighted})
		total += weighted
	}
	if len(candidates) == 0 || total <= 0 {
		return nil, ErrNoHealthyTargets
	}

	roll := randv2.Float64() * total // #nosec G404 -- math/rand/v2 is acceptable for load-balancing weighted random selection.
	acc := 0.0
	for _, c := range candidates {
		acc += c.weight
		if roll <= acc {
			selected := c.target
			return &selected, nil
		}
	}
	selected := candidates[len(candidates)-1].target
	return &selected, nil
}

func (hw *HealthWeighted) UpdateTargets(targets []config.UpstreamTarget) {
	hw.mu.Lock()
	defer hw.mu.Unlock()

	hw.targets = cloneTargets(targets)
	if hw.health == nil {
		hw.health = make(map[string]bool, len(targets))
	}
	if hw.score == nil {
		hw.score = make(map[string]float64, len(targets))
	}

	newHealth := make(map[string]bool, len(targets))
	newScore := make(map[string]float64, len(targets))
	for _, t := range hw.targets {
		key := targetKey(t)
		healthy, ok := hw.health[key]
		if !ok {
			healthy = true
		}
		newHealth[key] = healthy

		score, ok := hw.score[key]
		if !ok || score <= 0 {
			score = 1.0
		}
		newScore[key] = score
	}
	hw.health = newHealth
	hw.score = newScore
}

func (hw *HealthWeighted) ReportHealth(targetID string, healthy bool, _ time.Duration) {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	if hw.health == nil {
		hw.health = make(map[string]bool)
	}
	if hw.score == nil {
		hw.score = make(map[string]float64)
	}
	hw.health[targetID] = healthy

	score := hw.score[targetID]
	if score <= 0 {
		score = 1.0
	}
	if healthy {
		score += 0.15
		if score > 1.0 {
			score = 1.0
		}
	} else {
		score -= 0.40
		if score < 0 {
			score = 0
		}
	}
	hw.score[targetID] = score
}

func (hw *HealthWeighted) Done(_ string) {}

func affinityKey(ctx *RequestContext) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	req := ctx.Request

	if xff := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if first != "" {
				return first
			}
		}
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr)); err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(req.RemoteAddr)
}
