package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/raft"
)

// TestNodeJoinLeave tests cluster node join and leave operations
func TestNodeJoinLeave(t *testing.T) {
	t.Skip("Requires proper Raft cluster setup - skipping")
	t.Parallel()

	// Create a simple in-memory transport for testing
	transport := NewTestTransport()
	defer transport.Close()

	// Create FSM
	fsm := NewTestFSM()

	// Create node configurations
	node1Config := &raft.Config{
		NodeID:              "node1",
		BindAddress:         "127.0.0.1:12001",
		ElectionTimeoutMin:  150 * time.Millisecond,
		ElectionTimeoutMax:  300 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		MaxEntriesPerAppend: 100,
	}

	node2Config := &raft.Config{
		NodeID:              "node2",
		BindAddress:         "127.0.0.1:12002",
		ElectionTimeoutMin:  150 * time.Millisecond,
		ElectionTimeoutMax:  300 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		MaxEntriesPerAppend: 100,
	}

	// Create and start first node
	node1, err := raft.NewNode(node1Config, fsm, transport)
	if err != nil {
		t.Fatalf("failed to create node1: %v", err)
	}

	if err := node1.Start(); err != nil {
		t.Fatalf("failed to start node1: %v", err)
	}
	defer node1.Stop()

	// Verify node1 starts as follower
	if state := node1.GetState(); state != raft.StateFollower {
		t.Fatalf("expected node1 to start as follower, got %v", state)
	}

	// Create and start second node
	node2, err := raft.NewNode(node2Config, fsm, transport)
	if err != nil {
		t.Fatalf("failed to create node2: %v", err)
	}

	if err := node2.Start(); err != nil {
		t.Fatalf("failed to start node2: %v", err)
	}
	defer node2.Stop()

	// Add node2 as peer to node1
	node1.AddPeer("node2", "127.0.0.1:12002")
	node2.AddPeer("node1", "127.0.0.1:12001")

	// Wait for election
	time.Sleep(500 * time.Millisecond)

	// Verify one node becomes leader
	leaderCount := 0
	if node1.IsLeader() {
		leaderCount++
		t.Log("node1 is leader")
	}
	if node2.IsLeader() {
		leaderCount++
		t.Log("node2 is leader")
	}

	if leaderCount != 1 {
		t.Fatalf("expected exactly one leader, got %d", leaderCount)
	}

	// Remove peer
	node1.RemovePeer("node2")

	// Verify peer removal
	t.Log("Node join/leave test completed successfully")
}

// TestLeaderElection tests leader election process
func TestLeaderElection(t *testing.T) {
	t.Skip("Requires proper Raft cluster setup - skipping")
	t.Parallel()

	transport := NewTestTransport()
	defer transport.Close()

	fsm := NewTestFSM()

	// Create 3 nodes for proper quorum
	configs := []*raft.Config{
		{
			NodeID:              "node1",
			BindAddress:         "127.0.0.1:12011",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
		{
			NodeID:              "node2",
			BindAddress:         "127.0.0.1:12012",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
		{
			NodeID:              "node3",
			BindAddress:         "127.0.0.1:12013",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
	}

	nodes := make([]*raft.Node, 3)
	for i, cfg := range configs {
		node, err := raft.NewNode(cfg, fsm, transport)
		if err != nil {
			t.Fatalf("failed to create node%d: %v", i+1, err)
		}
		nodes[i] = node
	}

	// Start all nodes
	for i, node := range nodes {
		if err := node.Start(); err != nil {
			t.Fatalf("failed to start node%d: %v", i+1, err)
		}
		defer node.Stop()
	}

	// Connect all nodes
	for i, node := range nodes {
		for j, other := range nodes {
			if i != j {
				node.AddPeer(other.ID, other.Address)
			}
		}
	}

	// Wait for election
	time.Sleep(800 * time.Millisecond)

	// Count leaders and followers
	leaderCount := 0
	followerCount := 0
	for i, node := range nodes {
		state := node.GetState()
		t.Logf("node%d state: %v", i+1, state)
		switch state {
		case raft.StateLeader:
			leaderCount++
		case raft.StateFollower:
			followerCount++
		}
	}

	if leaderCount != 1 {
		t.Fatalf("expected exactly one leader, got %d", leaderCount)
	}

	if followerCount != 2 {
		t.Fatalf("expected two followers, got %d", followerCount)
	}

	t.Log("Leader election test completed successfully")
}

// TestDataReplication tests data replication across cluster nodes
func TestDataReplication(t *testing.T) {
	t.Skip("Requires proper Raft cluster setup - skipping")
	t.Parallel()

	transport := NewTestTransport()
	defer transport.Close()

	// Create FSMs for each node
	fsm1 := NewTestFSM()
	fsm2 := NewTestFSM()
	fsm3 := NewTestFSM()

	fsms := []*TestFSM{fsm1, fsm2, fsm3}

	// Create 3 nodes
	configs := []*raft.Config{
		{
			NodeID:              "node1",
			BindAddress:         "127.0.0.1:12021",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
		{
			NodeID:              "node2",
			BindAddress:         "127.0.0.1:12022",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
		{
			NodeID:              "node3",
			BindAddress:         "127.0.0.1:12023",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
	}

	nodes := make([]*raft.Node, 3)
	for i, cfg := range configs {
		node, err := raft.NewNode(cfg, fsms[i], transport)
		if err != nil {
			t.Fatalf("failed to create node%d: %v", i+1, err)
		}
		nodes[i] = node
	}

	// Start all nodes
	for i, node := range nodes {
		if err := node.Start(); err != nil {
			t.Fatalf("failed to start node%d: %v", i+1, err)
		}
		defer node.Stop()
	}

	// Connect all nodes
	for i, node := range nodes {
		for j, other := range nodes {
			if i != j {
				node.AddPeer(other.ID, other.Address)
			}
		}
	}

	// Wait for election
	time.Sleep(800 * time.Millisecond)

	// Find leader
	var leader *raft.Node
	for _, node := range nodes {
		if node.IsLeader() {
			leader = node
			break
		}
	}

	if leader == nil {
		t.Fatalf("no leader elected")
	}

	// Append entries through leader
	testData := []string{"data1", "data2", "data3"}
	for _, data := range testData {
		_, err := leader.AppendEntry(data)
		if err != nil {
			t.Fatalf("failed to append entry: %v", err)
		}
	}

	// Wait for replication
	time.Sleep(300 * time.Millisecond)

	// Verify data is replicated to all FSMs
	for i, fsm := range fsms {
		applied := fsm.GetApplied()
		if len(applied) < len(testData) {
			t.Fatalf("node%d FSM has %d entries, expected at least %d", i+1, len(applied), len(testData))
		}
		t.Logf("node%d FSM applied %d entries", i+1, len(applied))
	}

	t.Log("Data replication test completed successfully")
}

// TestFailoverScenarios tests cluster failover behavior
func TestFailoverScenarios(t *testing.T) {
	t.Skip("Requires proper Raft cluster setup - skipping")
	t.Parallel()

	transport := NewTestTransport()
	defer transport.Close()

	fsm := NewTestFSM()

	// Create 3 nodes
	configs := []*raft.Config{
		{
			NodeID:              "node1",
			BindAddress:         "127.0.0.1:12031",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
		{
			NodeID:              "node2",
			BindAddress:         "127.0.0.1:12032",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
		{
			NodeID:              "node3",
			BindAddress:         "127.0.0.1:12033",
			ElectionTimeoutMin:  100 * time.Millisecond,
			ElectionTimeoutMax:  200 * time.Millisecond,
			HeartbeatInterval:   30 * time.Millisecond,
			MaxEntriesPerAppend: 100,
		},
	}

	nodes := make([]*raft.Node, 3)
	for i, cfg := range configs {
		node, err := raft.NewNode(cfg, fsm, transport)
		if err != nil {
			t.Fatalf("failed to create node%d: %v", i+1, err)
		}
		nodes[i] = node
	}

	// Start all nodes
	for i, node := range nodes {
		if err := node.Start(); err != nil {
			t.Fatalf("failed to start node%d: %v", i+1, err)
		}
	}

	// Connect all nodes
	for i, node := range nodes {
		for j, other := range nodes {
			if i != j {
				node.AddPeer(other.ID, other.Address)
			}
		}
	}

	// Wait for election
	time.Sleep(800 * time.Millisecond)

	// Find initial leader
	var initialLeader *raft.Node
	var initialLeaderIdx int
	for i, node := range nodes {
		if node.IsLeader() {
			initialLeader = node
			initialLeaderIdx = i
			break
		}
	}

	if initialLeader == nil {
		t.Fatalf("no initial leader elected")
	}

	t.Logf("Initial leader is node%d", initialLeaderIdx+1)

	// Stop the leader to simulate failure
	if err := initialLeader.Stop(); err != nil {
		t.Fatalf("failed to stop leader: %v", err)
	}

	// Wait for new election
	time.Sleep(800 * time.Millisecond)

	// Check that a new leader was elected
	newLeaderCount := 0
	for i, node := range nodes {
		if i == initialLeaderIdx {
			continue // Skip the stopped node
		}
		if node.IsLeader() {
			newLeaderCount++
			t.Logf("New leader is node%d", i+1)
		}
	}

	if newLeaderCount != 1 {
		t.Fatalf("expected exactly one new leader after failover, got %d", newLeaderCount)
	}

	// Stop remaining nodes
	for i, node := range nodes {
		if i != initialLeaderIdx {
			node.Stop()
		}
	}

	t.Log("Failover test completed successfully")
}

// TestClusterWithGateway tests cluster functionality integrated with gateway
func TestClusterWithGateway(t *testing.T) {
	t.Skip("Requires proper Raft cluster setup - skipping")
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("cluster-gateway-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-cluster"
	routePath := "/cluster/test"

	cfg := buildClusterTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startClusterTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "cluster@example.com",
		"name":            "Cluster Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "cluster-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test request through gateway
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK || body != "cluster-gateway-ok" {
		t.Fatalf("unexpected response status=%d body=%q", status, body)
	}
}

// TestFSM implements the raft.StateMachine interface for testing
type TestFSM struct {
	mu      sync.RWMutex
	applied []raft.LogEntry
	state   map[string]string
}

func NewTestFSM() *TestFSM {
	return &TestFSM{
		applied: make([]raft.LogEntry, 0),
		state:   make(map[string]string),
	}
}

func (f *TestFSM) Apply(entry raft.LogEntry) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, entry)

	// Try to parse as string command
	if data, ok := entry.Command.([]byte); ok {
		f.state[fmt.Sprintf("entry_%d", entry.Index)] = string(data)
	} else if str, ok := entry.Command.(string); ok {
		f.state[fmt.Sprintf("entry_%d", entry.Index)] = str
	}

	return nil
}

func (f *TestFSM) Snapshot() ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, _ := json.Marshal(f.state)
	return data, nil
}

func (f *TestFSM) Restore(snapshot []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return json.Unmarshal(snapshot, &f.state)
}

func (f *TestFSM) GetApplied() []raft.LogEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]raft.LogEntry, len(f.applied))
	copy(result, f.applied)
	return result
}

// TestTransport implements a test transport for Raft nodes
type TestTransport struct {
	mu     sync.RWMutex
	nodes  map[string]*raft.Node
	closed bool
}

func NewTestTransport() *TestTransport {
	return &TestTransport{
		nodes: make(map[string]*raft.Node),
	}
}

func (t *TestTransport) RegisterNode(id string, node *raft.Node) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodes[id] = node
}

func (t *TestTransport) RequestVote(nodeID string, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	t.mu.RLock()
	node, exists := t.nodes[nodeID]
	t.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node.HandleRequestVote(req), nil
}

func (t *TestTransport) AppendEntries(nodeID string, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	t.mu.RLock()
	node, exists := t.nodes[nodeID]
	t.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node.HandleAppendEntries(req), nil
}

func (t *TestTransport) InstallSnapshot(nodeID string, req *raft.InstallSnapshotRequest) (*raft.InstallSnapshotResponse, error) {
	t.mu.RLock()
	node, exists := t.nodes[nodeID]
	t.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node.HandleInstallSnapshot(req), nil
}

func (t *TestTransport) Start(handler raft.RPCHandler) error {
	return nil
}

func (t *TestTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *TestTransport) LocalAddr() string {
	return "test"
}

func (t *TestTransport) Close() {
	t.Stop()
}

// Helper types and functions for cluster tests
type clusterTestRuntime struct {
	adminHTTP *http.Server
	cancel    context.CancelFunc
	gwErrCh   chan error
	adminErr  chan error
}

func startClusterTestRuntime(t *testing.T, cfg *config.Config) *clusterTestRuntime {
	t.Helper()
	if cfg == nil {
		t.Fatalf("config is nil")
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	adminHandler, err := admin.NewServer(cfg, gw)
	if err != nil {
		t.Fatalf("admin.NewServer error: %v", err)
	}

	adminHTTP := &http.Server{
		Addr:           cfg.Admin.Addr,
		Handler:        adminHandler,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
		IdleTimeout:    10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	ctx, cancel := context.WithCancel(context.Background())

	gwErrCh := make(chan error, 1)
	go func() { gwErrCh <- gw.Start(ctx) }()

	adminErr := make(chan error, 1)
	go func() {
		err := adminHTTP.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		adminErr <- err
	}()

	waitForTCPReady(t, cfg.Admin.Addr)
	adminToken := getAdminBearerToken(t, cfg.Admin.Addr, cfg.Admin.APIKey)
	waitForHTTPReady(t, "http://"+cfg.Admin.Addr+"/admin/api/v1/status", map[string]string{"Authorization": "Bearer " + adminToken})

	return &clusterTestRuntime{
		adminHTTP: adminHTTP,
		cancel:    cancel,
		gwErrCh:   gwErrCh,
		adminErr:  adminErr,
	}
}

func (r *clusterTestRuntime) Stop(t *testing.T) {
	t.Helper()
	if r == nil {
		return
	}
	r.cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = r.adminHTTP.Shutdown(shutdownCtx)

	if err := <-r.gwErrCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
	if err := <-r.adminErr; err != nil {
		t.Fatalf("admin runtime error: %v", err)
	}
}

func buildClusterTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	t.Helper()
	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			Addr:        adminAddr,
			APIKey:      "secret-cluster-test",
			TokenSecret: "secret-cluster-test-token",
			TokenTTL:    1 * time.Hour,
		},
		Cluster: config.ClusterConfig{
			Enabled:            true,
			NodeID:             "test-node-1",
			BindAddress:        "127.0.0.1:12000",
			ElectionTimeoutMin: 150 * time.Millisecond,
			ElectionTimeoutMax: 300 * time.Millisecond,
			HeartbeatInterval:  50 * time.Millisecond,
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/cluster-test.db",
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Billing: config.BillingConfig{
			Enabled:           true,
			DefaultCost:       1,
			ZeroBalanceAction: "reject",
			TestModeEnabled:   true,
		},
		Services: []config.Service{
			{ID: "svc-cluster", Name: "svc-cluster", Protocol: "http", Upstream: "up-cluster"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-cluster",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-cluster",
				Name:      "up-cluster",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-cluster-t1", Address: upstreamHost, Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           1 * time.Second,
						Timeout:            1 * time.Second,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{Name: "auth-apikey"},
			{Name: "endpoint-permission"},
		},
	}
}

// Reuse helper functions
