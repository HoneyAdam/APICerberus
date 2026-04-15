package raft

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClusterManager(state NodeState) *ClusterManager {
	node := &Node{
		ID:        "node-1",
		Address:   "127.0.0.1:12000",
		State:     state,
		Peers:     map[string]string{"node-2": "127.0.0.1:12001"},
		Log:       []LogEntry{},
		leaderID:  "node-1",
		stopCh:    make(chan struct{}),
	}
	if state == StateLeader {
		node.nextIndex = map[string]uint64{"node-2": 1}
		node.matchIndex = map[string]uint64{"node-2": 0}
	}
	fsm := NewGatewayFSM()
	return NewClusterManager(node, fsm, "127.0.0.1:0", "test-api-key")
}

func TestAuthMiddleware_Valid(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	handler := cm.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_Invalid(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	handler := cm.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_Empty(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	handler := cm.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleClusterStatus_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	cm.handleClusterStatus(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleClusterStatus_Success(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	cm.node.CurrentTerm = 3
	cm.node.CommitIndex = 10
	cm.node.LastApplied = 9
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleClusterStatus(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var status ClusterStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want %q", status.NodeID, "node-1")
	}
	if status.Term != 3 {
		t.Errorf("Term = %d, want 3", status.Term)
	}
	if status.LeaderID != "node-1" {
		t.Errorf("LeaderID = %q, want %q", status.LeaderID, "node-1")
	}
}

func TestHandleNodes_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	cm.handleNodes(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleNodes_ListNodes(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleNodes(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var nodes []NodeInfo
	if err := json.NewDecoder(w.Body).Decode(&nodes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(nodes) < 1 {
		t.Errorf("expected at least 1 node, got %d", len(nodes))
	}
	// First node should be self
	if nodes[0].ID != "node-1" {
		t.Errorf("first node ID = %q, want %q", nodes[0].ID, "node-1")
	}
}

func TestHandleJoin_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleJoin(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleJoin_InvalidBody(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	cm.handleJoin(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleJoin_NotLeader(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateFollower)
	cm.node.leaderID = "node-2"
	body := `{"node_id":"node-3","address":"127.0.0.1:12002"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	cm.handleJoin(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var resp JoinResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Success {
		t.Error("expected Success = false for non-leader")
	}
}

func TestHandleJoin_AsLeader(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	body := `{"node_id":"node-3","address":"127.0.0.1:12002"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	cm.handleJoin(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp JoinResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success = true for leader")
	}
	if resp.LeaderID != "node-1" {
		t.Errorf("LeaderID = %q, want %q", resp.LeaderID, "node-1")
	}
}

func TestHandleLeave_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleLeave(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLeave_InvalidBody(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	cm.handleLeave(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleLeave_NotLeader(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateFollower)
	body := `{"node_id":"node-2"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	cm.handleLeave(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleLeave_AsLeader(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	body := `{"node_id":"node-2"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	cm.handleLeave(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp LeaveResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success = true")
	}
}

func TestHandleSnapshot_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleSnapshot(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleSnapshot_Success(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	cm.handleSnapshot(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp SnapshotResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success = true")
	}
}

func TestHandleRaftState_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	cm.handleRaftState(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRaftState_Success(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	cm.node.CurrentTerm = 5
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleRaftState(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var state map[string]any
	if err := json.NewDecoder(w.Body).Decode(&state); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if state["node_id"] != "node-1" {
		t.Errorf("node_id = %v, want node-1", state["node_id"])
	}
	if state["is_leader"] != true {
		t.Errorf("is_leader = %v, want true", state["is_leader"])
	}
}

func TestHandleRaftStats_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	cm.handleRaftStats(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRaftStats_Success(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	cm.handleRaftStats(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var stats map[string]any
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := stats["raft"]; !ok {
		t.Error("expected 'raft' key in stats")
	}
	if _, ok := stats["fsm"]; !ok {
		t.Error("expected 'fsm' key in stats")
	}
}

func TestPropose_NotLeader(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateFollower)
	err := cm.Propose(FSMCommand{Type: "add_route"})
	if err == nil {
		t.Fatal("expected error for non-leader propose")
	}
	if !strings.Contains(err.Error(), "not leader") {
		t.Errorf("error = %q, want 'not leader'", err.Error())
	}
}

func TestPropose_AsLeader(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	// AppendEntry will succeed but WaitForCommit will timeout since no replication
	err := cm.Propose(FSMCommand{Type: "add_route"})
	if err == nil {
		t.Fatal("expected error — no peers to replicate to")
	}
	// Should mention "proposal failed" or "not committed"
	if !strings.Contains(err.Error(), "proposal failed") {
		t.Errorf("error = %q, want 'proposal failed'", err.Error())
	}
}

func TestCheckNodeHealth(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	// checkNodeHealth iterates peers and makes HTTP requests that will fail
	cm.checkNodeHealth()
	// Should not panic; peer "node-2" should be unhealthy
	cm.mu.RLock()
	health := cm.nodeHealth["node-2"]
	cm.mu.RUnlock()
	if health == nil {
		t.Fatal("expected health entry for node-2")
	}
	if health.Healthy {
		t.Error("expected unhealthy since no server running")
	}
	if health.FailCount < 1 {
		t.Errorf("FailCount = %d, want >= 1", health.FailCount)
	}
}

func TestCheckNodeHealth_SelfSkipped(t *testing.T) {
	t.Parallel()
	cm := newTestClusterManager(StateLeader)
	// Add self as peer — should be skipped
	cm.node.Peers["node-1"] = "127.0.0.1:12000"
	cm.checkNodeHealth()
	cm.mu.RLock()
	_, exists := cm.nodeHealth["node-1"]
	cm.mu.RUnlock()
	if exists {
		t.Error("self should be skipped in health checks")
	}
}

func TestAcquireACMERenewalLock_NotLeader(t *testing.T) {
	t.Parallel()
	node := &Node{
		ID:     "node-1",
		State:  StateFollower,
		Peers:  map[string]string{},
		Log:    []LogEntry{},
		stopCh: make(chan struct{}),
	}
	_, err := node.AcquireACMERenewalLock("example.com", 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for non-leader")
	}
	if !strings.Contains(err.Error(), "not the leader") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestProposeCertificateUpdate_NotLeader(t *testing.T) {
	t.Parallel()
	node := &Node{
		ID:     "node-1",
		State:  StateFollower,
		Peers:  map[string]string{},
		Log:    []LogEntry{},
		stopCh: make(chan struct{}),
	}
	err := node.ProposeCertificateUpdate("example.com", "cert", "key", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error for non-leader")
	}
}

func TestGetNodeID(t *testing.T) {
	t.Parallel()
	node := &Node{ID: "test-node"}
	if got := node.GetNodeID(); got != "test-node" {
		t.Errorf("GetNodeID() = %q, want %q", got, "test-node")
	}
}

func TestNewClusterManager_NilValues(t *testing.T) {
	t.Parallel()
	cm := NewClusterManager(nil, nil, ":0", "")
	if cm == nil {
		t.Fatal("expected non-nil ClusterManager")
	}
	if cm.nodeHealth == nil {
		t.Error("expected nodeHealth map to be initialized")
	}
}

func TestNodeHealth_JSON(t *testing.T) {
	t.Parallel()
	h := NodeHealth{
		ID:        "node-1",
		Address:   "127.0.0.1:12000",
		LastSeen:  time.Now(),
		Healthy:   true,
		FailCount: 0,
	}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "node-1") {
		t.Error("expected node-1 in JSON output")
	}
}
