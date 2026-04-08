package raft

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Region represents a geographic region in a multi-region deployment.
type Region struct {
	ID        string           `json:"id" yaml:"id"`
	Name      string           `json:"name" yaml:"name"`
	Nodes     []string         `json:"nodes" yaml:"nodes"`                               // Node IDs in this region
	Priority  int              `json:"priority" yaml:"priority"`                         // Lower = preferred for leader
	LatencyMS map[string]int64 `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"` // Latency to other regions
}

// MultiRegionConfig holds configuration for multi-region Raft clustering.
type MultiRegionConfig struct {
	Enabled           bool           `json:"enabled" yaml:"enabled"`
	RegionID          string         `json:"region_id" yaml:"region_id"`
	Regions           []Region       `json:"regions" yaml:"regions"`
	LeaderPreference  string         `json:"leader_preference" yaml:"leader_preference"`   // "local", "lowest_latency", "priority"
	ReplicationMode   string         `json:"replication_mode" yaml:"replication_mode"`     // "sync", "async"
	WANTimeoutFactor  float64        `json:"wan_timeout_factor" yaml:"wan_timeout_factor"` // Multiplier for WAN timeouts
	MaxCrossRegionLag time.Duration  `json:"max_cross_region_lag" yaml:"max_cross_region_lag"`
	RegionWeights     map[string]int `json:"region_weights" yaml:"region_weights"`
}

// DefaultMultiRegionConfig returns default multi-region configuration.
func DefaultMultiRegionConfig() *MultiRegionConfig {
	return &MultiRegionConfig{
		Enabled:           false,
		LeaderPreference:  "priority",
		ReplicationMode:   "async",
		WANTimeoutFactor:  2.0,
		MaxCrossRegionLag: 30 * time.Second,
		RegionWeights:     make(map[string]int),
	}
}

// MultiRegionManager manages multi-region Raft clustering.
type MultiRegionManager struct {
	config      *MultiRegionConfig
	node        *Node
	localRegion *Region

	// Region latencies measured through heartbeat RTT
	regionLatencies map[string]time.Duration
	latencyMu       sync.RWMutex

	// Cross-region replication status
	regionReplicationStatus map[string]*RegionReplicationStatus
	statusMu                sync.RWMutex

	// Health check context
	healthCtx    context.Context
	healthCancel context.CancelFunc

}

// RegionReplicationStatus tracks replication status for a region.
type RegionReplicationStatus struct {
	RegionID       string        `json:"region_id"`
	LastContact    time.Time     `json:"last_contact"`
	ReplicationLag time.Duration `json:"replication_lag"`
	MatchIndex     uint64        `json:"match_index"`
	Status         RegionStatus  `json:"status"`
	NodesHealthy   int           `json:"nodes_healthy"`
	NodesTotal     int           `json:"nodes_total"`
}

// RegionStatus represents the health status of a region.
type RegionStatus int

const (
	RegionStatusHealthy RegionStatus = iota
	RegionStatusDegraded
	RegionStatusUnreachable
)

func (s RegionStatus) String() string {
	switch s {
	case RegionStatusHealthy:
		return "healthy"
	case RegionStatusDegraded:
		return "degraded"
	case RegionStatusUnreachable:
		return "unreachable"
	default:
		return "unknown"
	}
}

// NewMultiRegionManager creates a new multi-region manager.
func NewMultiRegionManager(config *MultiRegionConfig, node *Node) (*MultiRegionManager, error) {
	if config == nil {
		return nil, fmt.Errorf("multi-region config is required")
	}

	if !config.Enabled {
		return &MultiRegionManager{
			config: config,
			node:   node,
		}, nil
	}

	if config.RegionID == "" {
		return nil, fmt.Errorf("region_id is required when multi-region is enabled")
	}

	mgr := &MultiRegionManager{
		config:                  config,
		node:                    node,
		regionLatencies:         make(map[string]time.Duration),
		regionReplicationStatus: make(map[string]*RegionReplicationStatus),
	}

	// Find local region
	for i := range config.Regions {
		if config.Regions[i].ID == config.RegionID {
			mgr.localRegion = &config.Regions[i]
			break
		}
	}

	if mgr.localRegion == nil {
		return nil, fmt.Errorf("local region %s not found in regions list", config.RegionID)
	}

	mgr.healthCtx, mgr.healthCancel = context.WithCancel(context.Background())

	return mgr, nil
}

// Start starts the multi-region manager.
func (m *MultiRegionManager) Start() error {
	if !m.config.Enabled {
		return nil
	}

	// Start region health monitoring
	go m.monitorRegionHealth()

	// Start latency monitoring
	go m.monitorLatencies()

	return nil
}

// Stop stops the multi-region manager.
func (m *MultiRegionManager) Stop() {
	if m.healthCancel != nil {
		m.healthCancel()
	}
}

// IsEnabled returns true if multi-region mode is enabled.
func (m *MultiRegionManager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// GetLocalRegion returns the local region ID.
func (m *MultiRegionManager) GetLocalRegion() string {
	if m.config == nil {
		return ""
	}
	return m.config.RegionID
}

// GetRegionForNode returns the region ID for a given node ID.
func (m *MultiRegionManager) GetRegionForNode(nodeID string) string {
	if !m.IsEnabled() {
		return m.config.RegionID
	}

	for _, region := range m.config.Regions {
		for _, id := range region.Nodes {
			if id == nodeID {
				return region.ID
			}
		}
	}
	return ""
}

// IsLocalNode checks if a node is in the same region.
func (m *MultiRegionManager) IsLocalNode(nodeID string) bool {
	return m.GetRegionForNode(nodeID) == m.config.RegionID
}

// IsCrossRegion checks if communicating with a node in another region.
func (m *MultiRegionManager) IsCrossRegion(nodeID string) bool {
	if !m.IsEnabled() {
		return false
	}
	return !m.IsLocalNode(nodeID)
}

// GetLatencyToRegion returns the measured latency to a region.
func (m *MultiRegionManager) GetLatencyToRegion(regionID string) time.Duration {
	m.latencyMu.RLock()
	defer m.latencyMu.RUnlock()
	return m.regionLatencies[regionID]
}

// RecordLatency records a latency measurement to a node.
func (m *MultiRegionManager) RecordLatency(nodeID string, latency time.Duration) {
	if !m.IsEnabled() {
		return
	}

	regionID := m.GetRegionForNode(nodeID)
	if regionID == "" {
		return
	}

	m.latencyMu.Lock()
	defer m.latencyMu.Unlock()

	// Use exponential moving average
	const alpha = 0.3
	current := m.regionLatencies[regionID]
	if current == 0 {
		m.regionLatencies[regionID] = latency
	} else {
		m.regionLatencies[regionID] = time.Duration(float64(current)*(1-alpha) + float64(latency)*alpha)
	}
}

// ShouldPreferLocalLeader returns true if leader should preferably be local.
func (m *MultiRegionManager) ShouldPreferLocalLeader() bool {
	return m.config.LeaderPreference == "local"
}

// GetLeaderPriorityScore returns a priority score for a candidate leader.
// Lower scores are better. Considers region priority and latency.
func (m *MultiRegionManager) GetLeaderPriorityScore(nodeID string) int {
	if !m.IsEnabled() {
		return 0
	}

	regionID := m.GetRegionForNode(nodeID)
	if regionID == "" {
		return 1000
	}

	// Local region gets priority boost
	if regionID == m.config.RegionID {
		return -100
	}

	// Find region config
	for _, region := range m.config.Regions {
		if region.ID == regionID {
			return region.Priority
		}
	}

	return 100
}

// GetReplicationTimeout returns the appropriate timeout for replication to a node.
func (m *MultiRegionManager) GetReplicationTimeout(nodeID string, baseTimeout time.Duration) time.Duration {
	if !m.IsEnabled() || !m.IsCrossRegion(nodeID) {
		return baseTimeout
	}

	// Apply WAN timeout factor for cross-region replication
	timeout := time.Duration(float64(baseTimeout) * m.config.WANTimeoutFactor)

	// Add latency-based padding
	regionID := m.GetRegionForNode(nodeID)
	if latency := m.GetLatencyToRegion(regionID); latency > 0 {
		timeout += latency * 3 // 3x latency for RTT + processing
	}

	return timeout
}

// GetQuorumRegions returns the set of regions needed for quorum.
func (m *MultiRegionManager) GetQuorumRegions() []string {
	if !m.IsEnabled() {
		return []string{m.config.RegionID}
	}

	// Collect healthy regions
	var healthyRegions []string
	m.statusMu.RLock()
	for regionID, status := range m.regionReplicationStatus {
		if status.Status != RegionStatusUnreachable {
			healthyRegions = append(healthyRegions, regionID)
		}
	}
	m.statusMu.RUnlock()

	// If no status yet, use all configured regions
	if len(healthyRegions) == 0 {
		for _, region := range m.config.Regions {
			healthyRegions = append(healthyRegions, region.ID)
		}
	}

	return healthyRegions
}

// UpdateReplicationStatus updates the replication status for a region.
func (m *MultiRegionManager) UpdateReplicationStatus(regionID string, matchIndex uint64) {
	if !m.IsEnabled() {
		return
	}

	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	status, exists := m.regionReplicationStatus[regionID]
	if !exists {
		status = &RegionReplicationStatus{
			RegionID: regionID,
		}
		m.regionReplicationStatus[regionID] = status
	}

	// Calculate lag
	if m.node != nil {
		status.ReplicationLag = time.Duration(int64(m.node.lastLogIndex()-matchIndex)) * time.Millisecond // #nosec G115 -- log index delta is bounded by Raft cluster size.
	}

	status.LastContact = time.Now()
	status.MatchIndex = matchIndex

	// Determine health status
	if status.ReplicationLag > m.config.MaxCrossRegionLag {
		status.Status = RegionStatusDegraded
	} else {
		status.Status = RegionStatusHealthy
	}
}

// GetRegionReplicationStatus returns replication status for all regions.
func (m *MultiRegionManager) GetRegionReplicationStatus() map[string]*RegionReplicationStatus {
	if !m.IsEnabled() {
		return nil
	}

	m.statusMu.RLock()
	defer m.statusMu.RUnlock()

	result := make(map[string]*RegionReplicationStatus, len(m.regionReplicationStatus))
	for k, v := range m.regionReplicationStatus {
		result[k] = v
	}
	return result
}

// ShouldReplicateToRegion checks if replication should proceed to a region.
func (m *MultiRegionManager) ShouldReplicateToRegion(regionID string) bool {
	if !m.IsEnabled() {
		return true
	}

	m.statusMu.RLock()
	status, exists := m.regionReplicationStatus[regionID]
	m.statusMu.RUnlock()

	if !exists {
		return true // Allow if no status yet
	}

	return status.Status != RegionStatusUnreachable
}

// monitorRegionHealth monitors health of remote regions.
func (m *MultiRegionManager) monitorRegionHealth() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.healthCtx.Done():
			return
		case <-ticker.C:
			m.checkRegionHealth()
		}
	}
}

// checkRegionHealth checks health of all regions.
func (m *MultiRegionManager) checkRegionHealth() {
	if m.node == nil {
		return
	}

	// Check each known region
	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	for regionID, status := range m.regionReplicationStatus {
		if regionID == m.config.RegionID {
			continue // Skip local region
		}

		// Check if region is responding
		timeSinceContact := time.Since(status.LastContact)

		switch {
		case timeSinceContact > 30*time.Second:
			status.Status = RegionStatusUnreachable
		case timeSinceContact > 10*time.Second:
			status.Status = RegionStatusDegraded
		default:
			if status.ReplicationLag <= m.config.MaxCrossRegionLag {
				status.Status = RegionStatusHealthy
			}
		}
	}
}

// monitorLatencies monitors network latencies to other regions.
func (m *MultiRegionManager) monitorLatencies() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.healthCtx.Done():
			return
		case <-ticker.C:
			m.measureLatencies()
		}
	}
}

// measureLatencies measures latency to nodes in other regions.
func (m *MultiRegionManager) measureLatencies() {
	if m.node == nil {
		return
	}

	// Get all peers
	m.node.mu.RLock()
	peers := make(map[string]string, len(m.node.Peers))
	for id, addr := range m.node.Peers {
		peers[id] = addr
	}
	m.node.mu.RUnlock()

	// Measure latency to each peer
	for peerID, addr := range peers {
		if m.IsLocalNode(peerID) {
			continue
		}

		go m.measureLatencyToNode(peerID, addr)
	}
}

// measureLatencyToNode measures latency to a specific node.
func (m *MultiRegionManager) measureLatencyToNode(nodeID, address string) {
	start := time.Now()

	// Try to connect
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	latency := time.Since(start)
	m.RecordLatency(nodeID, latency)
}

// GetSortedPeersByPriority returns peers sorted by replication priority.
// Local region peers first, then by latency.
func (m *MultiRegionManager) GetSortedPeersByPriority(peerIDs []string) []string {
	if !m.IsEnabled() {
		return peerIDs
	}

	type peerPriority struct {
		id       string
		isLocal  bool
		priority int
		latency  time.Duration
	}

	peers := make([]peerPriority, 0, len(peerIDs))
	for _, id := range peerIDs {
		pp := peerPriority{
			id:      id,
			isLocal: m.IsLocalNode(id),
		}

		// Get region priority
		regionID := m.GetRegionForNode(id)
		for _, region := range m.config.Regions {
			if region.ID == regionID {
				pp.priority = region.Priority
				break
			}
		}

		// Get latency
		pp.latency = m.GetLatencyToRegion(regionID)

		peers = append(peers, pp)
	}

	// Sort: local first, then by priority, then by latency
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].isLocal != peers[j].isLocal {
			return peers[i].isLocal
		}
		if peers[i].priority != peers[j].priority {
			return peers[i].priority < peers[j].priority
		}
		return peers[i].latency < peers[j].latency
	})

	result := make([]string, len(peers))
	for i, p := range peers {
		result[i] = p.id
	}
	return result
}

// GetRegionAwareTimeout returns a timeout adjusted for cross-region communication.
func (m *MultiRegionManager) GetRegionAwareTimeout(nodeID string, baseTimeout time.Duration) time.Duration {
	if !m.IsEnabled() || m.IsLocalNode(nodeID) {
		return baseTimeout
	}

	// Increase timeout for cross-region communication
	timeout := time.Duration(float64(baseTimeout) * m.config.WANTimeoutFactor)

	// Add latency-based padding if available
	regionID := m.GetRegionForNode(nodeID)
	if latency := m.GetLatencyToRegion(regionID); latency > 0 {
		timeout += latency * 2
	}

	return timeout
}

// ValidateConfig validates the multi-region configuration.
func (m *MultiRegionManager) ValidateConfig() error {
	if !m.config.Enabled {
		return nil
	}

	if m.config.RegionID == "" {
		return fmt.Errorf("region_id is required")
	}

	if len(m.config.Regions) == 0 {
		return fmt.Errorf("at least one region must be configured")
	}

	// Check local region exists
	foundLocal := false
	for _, region := range m.config.Regions {
		if region.ID == m.config.RegionID {
			foundLocal = true
			break
		}
	}
	if !foundLocal {
		return fmt.Errorf("local region %s not found in regions list", m.config.RegionID)
	}

	// Validate leader preference
	validPreferences := map[string]bool{
		"local":          true,
		"lowest_latency": true,
		"priority":       true,
	}
	if !validPreferences[m.config.LeaderPreference] {
		return fmt.Errorf("invalid leader_preference: %s", m.config.LeaderPreference)
	}

	// Validate replication mode
	validModes := map[string]bool{
		"sync":  true,
		"async": true,
	}
	if !validModes[m.config.ReplicationMode] {
		return fmt.Errorf("invalid replication_mode: %s", m.config.ReplicationMode)
	}

	return nil
}

// IsRegionAware returns true if this build supports multi-region clustering.
func IsRegionAware() bool {
	return true
}

// ParseRegionID extracts region from node ID (e.g., "node-us-east-1-01" -> "us-east-1").
func ParseRegionID(nodeID string) string {
	parts := strings.Split(nodeID, "-")
	if len(parts) >= 3 {
		// Look for region pattern (e.g., "us-east-1", "eu-west-2")
		for i := 0; i < len(parts)-1; i++ {
			if len(parts[i]) == 2 && len(parts[i+1]) >= 4 {
				return fmt.Sprintf("%s-%s", parts[i], parts[i+1])
			}
		}
	}
	return "default"
}
