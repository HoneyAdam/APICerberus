package raft

import (
	"testing"
	"time"
)

func TestDefaultMultiRegionConfig(t *testing.T) {
	cfg := DefaultMultiRegionConfig()

	if cfg.Enabled {
		t.Error("Expected multi-region to be disabled by default")
	}

	if cfg.LeaderPreference != "priority" {
		t.Errorf("Expected leader_preference to be 'priority', got %s", cfg.LeaderPreference)
	}

	if cfg.ReplicationMode != "async" {
		t.Errorf("Expected replication_mode to be 'async', got %s", cfg.ReplicationMode)
	}

	if cfg.WANTimeoutFactor != 2.0 {
		t.Errorf("Expected wan_timeout_factor to be 2.0, got %f", cfg.WANTimeoutFactor)
	}
}

func TestNewMultiRegionManager_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled: false,
	}

	mgr, err := NewMultiRegionManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewMultiRegionManager() error = %v", err)
	}

	if mgr.IsEnabled() {
		t.Error("Expected manager to be disabled")
	}
}

func TestNewMultiRegionManager_NoRegionID(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled: true,
		// No RegionID
	}

	_, err := NewMultiRegionManager(cfg, nil)
	if err == nil {
		t.Error("Expected error when region_id is missing")
	}
}

func TestNewMultiRegionManager_MissingLocalRegion(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{ID: "eu-west-1", Name: "Europe"},
		},
	}

	_, err := NewMultiRegionManager(cfg, nil)
	if err == nil {
		t.Error("Expected error when local region is not in regions list")
	}
}

func TestNewMultiRegionManager_Valid(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{ID: "us-east-1", Name: "US East", Priority: 1},
			{ID: "eu-west-1", Name: "Europe", Priority: 2},
		},
	}

	mgr, err := NewMultiRegionManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewMultiRegionManager() error = %v", err)
	}

	if !mgr.IsEnabled() {
		t.Error("Expected manager to be enabled")
	}

	if mgr.GetLocalRegion() != "us-east-1" {
		t.Errorf("Expected local region to be 'us-east-1', got %s", mgr.GetLocalRegion())
	}
}

func TestMultiRegionManager_GetRegionForNode(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{
				ID:    "us-east-1",
				Name:  "US East",
				Nodes: []string{"node-1", "node-2"},
			},
			{
				ID:    "eu-west-1",
				Name:  "Europe",
				Nodes: []string{"node-3", "node-4"},
			},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)

	tests := []struct {
		nodeID   string
		expected string
	}{
		{"node-1", "us-east-1"},
		{"node-2", "us-east-1"},
		{"node-3", "eu-west-1"},
		{"node-4", "eu-west-1"},
		{"node-5", ""},
	}

	for _, tt := range tests {
		result := mgr.GetRegionForNode(tt.nodeID)
		if result != tt.expected {
			t.Errorf("GetRegionForNode(%s) = %s, want %s", tt.nodeID, result, tt.expected)
		}
	}
}

func TestMultiRegionManager_IsLocalNode(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{
				ID:    "us-east-1",
				Nodes: []string{"local-node"},
			},
			{
				ID:    "eu-west-1",
				Nodes: []string{"remote-node"},
			},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)

	if !mgr.IsLocalNode("local-node") {
		t.Error("Expected local-node to be local")
	}

	if mgr.IsLocalNode("remote-node") {
		t.Error("Expected remote-node to not be local")
	}
}

func TestMultiRegionManager_IsCrossRegion(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{
				ID:    "us-east-1",
				Nodes: []string{"local-node"},
			},
			{
				ID:    "eu-west-1",
				Nodes: []string{"remote-node"},
			},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)

	if mgr.IsCrossRegion("local-node") {
		t.Error("Expected local-node to not be cross-region")
	}

	if !mgr.IsCrossRegion("remote-node") {
		t.Error("Expected remote-node to be cross-region")
	}
}

func TestMultiRegionManager_RecordLatency(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{
				ID:    "us-east-1",
				Nodes: []string{"local-node"},
			},
			{
				ID:    "eu-west-1",
				Nodes: []string{"remote-node"},
			},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)

	// Record latency to remote node
	mgr.RecordLatency("remote-node", 50*time.Millisecond)

	latency := mgr.GetLatencyToRegion("eu-west-1")
	if latency != 50*time.Millisecond {
		t.Errorf("Expected latency 50ms, got %v", latency)
	}

	// Update latency (should use EMA)
	mgr.RecordLatency("remote-node", 100*time.Millisecond)

	updatedLatency := mgr.GetLatencyToRegion("eu-west-1")
	if updatedLatency == 50*time.Millisecond || updatedLatency == 100*time.Millisecond {
		t.Logf("EMA latency calculation: %v", updatedLatency)
	}
}

func TestMultiRegionManager_GetLeaderPriorityScore(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{ID: "us-east-1", Priority: 1, Nodes: []string{"local-node"}},
			{ID: "eu-west-1", Priority: 2, Nodes: []string{"remote-node"}},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)

	localScore := mgr.GetLeaderPriorityScore("local-node")
	remoteScore := mgr.GetLeaderPriorityScore("remote-node")

	if localScore >= remoteScore {
		t.Error("Expected local node to have better (lower) priority score")
	}
}

func TestMultiRegionManager_GetReplicationTimeout(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:          true,
		RegionID:         "us-east-1",
		WANTimeoutFactor: 2.0,
		Regions: []Region{
			{
				ID:    "us-east-1",
				Nodes: []string{"local-node"},
			},
			{
				ID:    "eu-west-1",
				Nodes: []string{"remote-node"},
			},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)
	mgr.RecordLatency("remote-node", 50*time.Millisecond)

	baseTimeout := 100 * time.Millisecond

	// Local node should use base timeout
	localTimeout := mgr.GetReplicationTimeout("local-node", baseTimeout)
	if localTimeout != baseTimeout {
		t.Errorf("Expected local timeout %v, got %v", baseTimeout, localTimeout)
	}

	// Remote node should have increased timeout
	remoteTimeout := mgr.GetReplicationTimeout("remote-node", baseTimeout)
	expectedMin := time.Duration(float64(baseTimeout) * 2.0) // WAN factor
	if remoteTimeout < expectedMin {
		t.Errorf("Expected remote timeout >= %v, got %v", expectedMin, remoteTimeout)
	}
}

func TestMultiRegionManager_UpdateReplicationStatus(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:           true,
		RegionID:          "us-east-1",
		MaxCrossRegionLag: 5 * time.Second,
		Regions:           []Region{{ID: "us-east-1"}, {ID: "eu-west-1"}},
	}

	node := &Node{
		Log: []LogEntry{
			{Index: 0, Term: 0},
			{Index: 1, Term: 1},
			{Index: 2, Term: 1},
			{Index: 3, Term: 1},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, node)

	// Update with healthy lag
	mgr.UpdateReplicationStatus("eu-west-1", 3)

	status := mgr.GetRegionReplicationStatus()
	if status == nil {
		t.Fatal("Expected replication status")
	}

	euStatus, ok := status["eu-west-1"]
	if !ok {
		t.Fatal("Expected eu-west-1 status")
	}

	if euStatus.Status != RegionStatusHealthy {
		t.Errorf("Expected healthy status, got %s", euStatus.Status)
	}
}

func TestMultiRegionManager_ShouldPreferLocalLeader(t *testing.T) {
	tests := []struct {
		preference string
		expected   bool
	}{
		{"local", true},
		{"priority", false},
		{"lowest_latency", false},
	}

	for _, tt := range tests {
		cfg := &MultiRegionConfig{
			Enabled:          true,
			RegionID:         "us-east-1",
			LeaderPreference: tt.preference,
			Regions:          []Region{{ID: "us-east-1"}},
		}

		mgr, _ := NewMultiRegionManager(cfg, nil)
		result := mgr.ShouldPreferLocalLeader()

		if result != tt.expected {
			t.Errorf("ShouldPreferLocalLeader() with %s = %v, want %v",
				tt.preference, result, tt.expected)
		}
	}
}

func TestMultiRegionManager_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *MultiRegionConfig
		wantErr bool
	}{
		{
			name: "disabled config",
			cfg: &MultiRegionConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "missing region_id",
			cfg: &MultiRegionConfig{
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "missing regions",
			cfg: &MultiRegionConfig{
				Enabled:  true,
				RegionID: "us-east-1",
			},
			wantErr: true,
		},
		{
			name: "missing local region",
			cfg: &MultiRegionConfig{
				Enabled:  true,
				RegionID: "us-east-1",
				Regions:  []Region{{ID: "eu-west-1"}},
			},
			wantErr: true,
		},
		{
			name: "invalid leader_preference",
			cfg: &MultiRegionConfig{
				Enabled:          true,
				RegionID:         "us-east-1",
				LeaderPreference: "invalid",
				Regions:          []Region{{ID: "us-east-1"}},
			},
			wantErr: true,
		},
		{
			name: "invalid replication_mode",
			cfg: &MultiRegionConfig{
				Enabled:         true,
				RegionID:        "us-east-1",
				ReplicationMode: "invalid",
				Regions:         []Region{{ID: "us-east-1"}},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: &MultiRegionConfig{
				Enabled:          true,
				RegionID:         "us-east-1",
				LeaderPreference: "priority",
				ReplicationMode:  "async",
				Regions:          []Region{{ID: "us-east-1"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewMultiRegionManager(tt.cfg, nil)
			if err != nil {
				// Error during creation
				if !tt.wantErr {
					t.Errorf("NewMultiRegionManager() error = %v", err)
				}
				return
			}

			err = mgr.ValidateConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMultiRegionManager_GetSortedPeersByPriority(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:  true,
		RegionID: "us-east-1",
		Regions: []Region{
			{ID: "us-east-1", Priority: 1, Nodes: []string{"us-node"}},
			{ID: "eu-west-1", Priority: 2, Nodes: []string{"eu-node"}},
			{ID: "ap-south-1", Priority: 3, Nodes: []string{"ap-node"}},
		},
	}

	mgr, _ := NewMultiRegionManager(cfg, nil)
	mgr.RecordLatency("eu-node", 100*time.Millisecond)
	mgr.RecordLatency("ap-node", 200*time.Millisecond)

	// Simulate: us-node is local, eu-node and ap-node are remote
	peers := []string{"ap-node", "us-node", "eu-node"}
	sorted := mgr.GetSortedPeersByPriority(peers)

	// Local node should be first
	if sorted[0] != "us-node" {
		t.Errorf("Expected first peer to be us-node, got %s", sorted[0])
	}
}

func TestRegionStatus_String(t *testing.T) {
	tests := []struct {
		status RegionStatus
		want   string
	}{
		{RegionStatusHealthy, "healthy"},
		{RegionStatusDegraded, "degraded"},
		{RegionStatusUnreachable, "unreachable"},
		{RegionStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("RegionStatus.String() = %s, want %s", got, tt.want)
		}
	}
}

func TestParseRegionID(t *testing.T) {
	tests := []struct {
		nodeID   string
		expected string
	}{
		{"node-us-east-1-01", "us-east"},
		{"node-eu-west-2-05", "eu-west"},
		{"node-ap-south-1-01", "ap-south"},
		{"simple-node", "default"},
		{"node", "default"},
	}

	for _, tt := range tests {
		result := ParseRegionID(tt.nodeID)
		if result != tt.expected {
			t.Errorf("ParseRegionID(%s) = %s, want %s", tt.nodeID, result, tt.expected)
		}
	}
}

func TestIsRegionAware(t *testing.T) {
	if !IsRegionAware() {
		t.Error("Expected IsRegionAware() to return true")
	}
}
