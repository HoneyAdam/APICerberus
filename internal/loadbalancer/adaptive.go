package loadbalancer

import (
	"math"
	"sync"
	"time"
)

// AdaptiveBalancer adapts load balancing strategy based on performance metrics.
type AdaptiveBalancer struct {
	mu sync.RWMutex

	// Target statistics
	stats map[string]*TargetStats

	// Current algorithm
	algorithm string

	// Configuration
	config *AdaptiveConfig
}

// TargetStats tracks statistics for a target.
type TargetStats struct {
	ID           string
	RequestCount uint64
	ErrorCount   uint64
	TotalLatency time.Duration
	AvgLatency   time.Duration
	ErrorRate    float64
	LastUpdated  time.Time
}

// AdaptiveConfig holds adaptive balancer configuration.
type AdaptiveConfig struct {
	// Thresholds for switching algorithms
	ErrorRateThreshold float64       // Switch if error rate exceeds this
	LatencyThreshold   time.Duration // Switch if latency exceeds this
	SwitchCooldown     time.Duration // Minimum time between switches
	WindowSize         int           // Number of requests to consider
	EnableAutoSwitch   bool          // Enable automatic algorithm switching
}

// DefaultAdaptiveConfig returns default adaptive config.
func DefaultAdaptiveConfig() *AdaptiveConfig {
	return &AdaptiveConfig{
		ErrorRateThreshold: 0.1, // 10%
		LatencyThreshold:   500 * time.Millisecond,
		SwitchCooldown:     30 * time.Second,
		WindowSize:         100,
		EnableAutoSwitch:   true,
	}
}

// NewAdaptiveBalancer creates a new adaptive balancer.
func NewAdaptiveBalancer(config *AdaptiveConfig) *AdaptiveBalancer {
	if config == nil {
		config = DefaultAdaptiveConfig()
	}

	return &AdaptiveBalancer{
		stats:     make(map[string]*TargetStats),
		algorithm: "round_robin", // Default
		config:    config,
	}
}

// RecordRequest records a request result for a target.
func (ab *AdaptiveBalancer) RecordRequest(targetID string, latency time.Duration, err error) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	stats, ok := ab.stats[targetID]
	if !ok {
		stats = &TargetStats{
			ID: targetID,
		}
		ab.stats[targetID] = stats
	}

	stats.RequestCount++
	stats.TotalLatency += latency

	if err != nil {
		stats.ErrorCount++
	}

	// Calculate average latency
	if stats.RequestCount > 0 {
		stats.AvgLatency = stats.TotalLatency / time.Duration(stats.RequestCount) // #nosec G115 -- request count for a target won't overflow int64 in practice.
	}

	// Calculate error rate
	if stats.RequestCount > 0 {
		stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.RequestCount)
	}

	stats.LastUpdated = time.Now()

	// Check if we should switch algorithms
	if ab.config.EnableAutoSwitch {
		ab.checkAndSwitch()
	}
}

// checkAndSwitch checks if algorithm should be switched.
func (ab *AdaptiveBalancer) checkAndSwitch() {
	// Calculate aggregate statistics
	var totalRequests uint64
	var totalErrors uint64
	var totalLatency time.Duration
	var targetCount int

	for _, stats := range ab.stats {
		// Only consider recently updated targets
		if time.Since(stats.LastUpdated) < time.Minute {
			totalRequests += stats.RequestCount
			totalErrors += stats.ErrorCount
			totalLatency += stats.TotalLatency
			targetCount++
		}
	}

	if totalRequests == 0 {
		return
	}

	aggErrorRate := float64(totalErrors) / float64(totalRequests)
	aggAvgLatency := totalLatency / time.Duration(totalRequests)

	// Check thresholds
	shouldSwitch := false
	newAlgorithm := ab.algorithm

	if aggErrorRate > ab.config.ErrorRateThreshold {
		// High error rate - use least connections
		shouldSwitch = true
		newAlgorithm = "least_connections"
	} else if aggAvgLatency > ab.config.LatencyThreshold {
		// High latency - use weighted response time
		shouldSwitch = true
		newAlgorithm = "weighted_response_time"
	}

	if shouldSwitch && newAlgorithm != ab.algorithm {
		ab.algorithm = newAlgorithm
	}
}

// GetAlgorithm returns the current algorithm.
func (ab *AdaptiveBalancer) GetAlgorithm() string {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return ab.algorithm
}

// GetStats returns statistics for a target.
func (ab *AdaptiveBalancer) GetStats(targetID string) (*TargetStats, bool) {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	stats, ok := ab.stats[targetID]
	return stats, ok
}

// GetAllStats returns all target statistics.
func (ab *AdaptiveBalancer) GetAllStats() map[string]*TargetStats {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	result := make(map[string]*TargetStats)
	for id, stats := range ab.stats {
		result[id] = stats
	}
	return result
}

// ResetStats resets statistics for all targets.
func (ab *AdaptiveBalancer) ResetStats() {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	ab.stats = make(map[string]*TargetStats)
}

// SelectTarget selects a target based on the current algorithm.
func (ab *AdaptiveBalancer) SelectTarget(targetIDs []string, lastIndex int) (string, int) {
	ab.mu.RLock()
	algorithm := ab.algorithm
	ab.mu.RUnlock()

	switch algorithm {
	case "round_robin":
		return ab.roundRobin(targetIDs, lastIndex)
	case "least_connections":
		return ab.leastConnections(targetIDs)
	case "weighted_response_time":
		return ab.weightedResponseTime(targetIDs)
	default:
		return ab.roundRobin(targetIDs, lastIndex)
	}
}

// roundRobin implements round-robin selection.
func (ab *AdaptiveBalancer) roundRobin(targetIDs []string, lastIndex int) (string, int) {
	if len(targetIDs) == 0 {
		return "", 0
	}

	nextIndex := (lastIndex + 1) % len(targetIDs)
	return targetIDs[nextIndex], nextIndex
}

// leastConnections selects the target with least connections (best error rate).
func (ab *AdaptiveBalancer) leastConnections(targetIDs []string) (string, int) {
	if len(targetIDs) == 0 {
		return "", 0
	}

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	var bestTarget string
	var bestErrorRate float64 = 1.0

	for _, id := range targetIDs {
		stats, ok := ab.stats[id]
		if !ok {
			// No stats yet, use this target
			return id, 0
		}

		if stats.ErrorRate < bestErrorRate {
			bestErrorRate = stats.ErrorRate
			bestTarget = id
		}
	}

	if bestTarget == "" {
		bestTarget = targetIDs[0]
	}

	return bestTarget, 0
}

// weightedResponseTime selects targets based on response time weighting.
func (ab *AdaptiveBalancer) weightedResponseTime(targetIDs []string) (string, int) {
	if len(targetIDs) == 0 {
		return "", 0
	}

	ab.mu.RLock()
	defer ab.mu.RUnlock()

	// Calculate weights (inverse of latency)
	weights := make(map[string]float64)
	var totalWeight float64

	for _, id := range targetIDs {
		stats, ok := ab.stats[id]
		if !ok {
			// No stats, assume average weight
			weights[id] = 1.0
			totalWeight += 1.0
			continue
		}

		// Weight = 1 / latency (higher weight for lower latency)
		latencyMs := float64(stats.AvgLatency.Milliseconds())
		if latencyMs == 0 {
			latencyMs = 1
		}
		weight := 1.0 / latencyMs
		weights[id] = weight
		totalWeight += weight
	}

	// Weighted random selection
	if totalWeight == 0 {
		return targetIDs[0], 0
	}

	target := ab.weightedRandomSelect(targetIDs, weights, totalWeight)
	return target, 0
}

// weightedRandomSelect selects a target using weighted random.
func (ab *AdaptiveBalancer) weightedRandomSelect(targetIDs []string, weights map[string]float64, totalWeight float64) string {
	// Pick the target with the highest weight
	var bestTarget string
	var bestWeight float64

	for _, id := range targetIDs {
		if w, ok := weights[id]; ok && w > bestWeight {
			bestWeight = w
			bestTarget = id
		}
	}

	if bestTarget == "" {
		return targetIDs[0]
	}

	return bestTarget
}

// CalculateHealthScore calculates a health score for a target (0-100).
func (ab *AdaptiveBalancer) CalculateHealthScore(targetID string) float64 {
	ab.mu.RLock()
	stats, ok := ab.stats[targetID]
	ab.mu.RUnlock()

	if !ok {
		return 100 // Unknown targets assumed healthy
	}

	// Score based on error rate and latency
	// Lower is worse
	errorScore := (1.0 - stats.ErrorRate) * 50

	latencyMs := float64(stats.AvgLatency.Milliseconds())
	latencyScore := math.Max(0, 50-(latencyMs/10)) // 0ms = 50pts, 500ms+ = 0pts

	return errorScore + latencyScore
}
