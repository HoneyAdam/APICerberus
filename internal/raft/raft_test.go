package raft

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNodeStateString(t *testing.T) {
	tests := []struct {
		state NodeState
		want  string
	}{
		{StateFollower, "Follower"},
		{StateCandidate, "Candidate"},
		{StateLeader, "Leader"},
		{NodeState(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewNode(t *testing.T) {
	config := DefaultConfig()
	config.NodeID = "test-node-1"
	config.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(config, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode() error = %v", err)
	}

	if node.ID != config.NodeID {
		t.Errorf("ID = %v, want %v", node.ID, config.NodeID)
	}

	if node.State != StateFollower {
		t.Errorf("State = %v, want %v", node.State, StateFollower)
	}

	if node.CurrentTerm != 0 {
		t.Errorf("CurrentTerm = %v, want %v", node.CurrentTerm, 0)
	}
}

func TestNewNodeMissingID(t *testing.T) {
	config := DefaultConfig()
	config.BindAddress = "127.0.0.1:12001"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	_, err := NewNode(config, fsm, transport)
	if err == nil {
		t.Error("NewNode() expected error for missing ID")
	}
}

func TestNewNodeMissingAddress(t *testing.T) {
	config := DefaultConfig()
	config.NodeID = "test-node-2"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	_, err := NewNode(config, fsm, transport)
	if err == nil {
		t.Error("NewNode() expected error for missing address")
	}
}

func TestNodeAddRemovePeer(t *testing.T) {
	config := DefaultConfig()
	config.NodeID = "test-node-3"
	config.BindAddress = "127.0.0.1:12002"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, _ := NewNode(config, fsm, transport)

	// Add peer
	node.AddPeer("peer-1", "127.0.0.1:12003")
	if _, ok := node.Peers["peer-1"]; !ok {
		t.Error("Expected peer-1 to be added")
	}

	// Remove peer
	node.RemovePeer("peer-1")
	if _, ok := node.Peers["peer-1"]; ok {
		t.Error("Expected peer-1 to be removed")
	}
}

func TestHandleRequestVote(t *testing.T) {
	config := DefaultConfig()
	config.NodeID = "test-node-4"
	config.BindAddress = "127.0.0.1:12004"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, _ := NewNode(config, fsm, transport)

	// Test: Candidate with lower term is rejected
	node.CurrentTerm = 2 // Set our term higher

	req := &RequestVoteRequest{
		Term:         1,
		CandidateID:  "peer-1",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}

	resp := node.HandleRequestVote(req)
	if resp.VoteGranted {
		t.Error("Expected vote to be denied for lower term candidate")
	}

	// Reset node
	node.CurrentTerm = 1
	node.Log = append(node.Log, LogEntry{Index: 1, Term: 1})

	// Test: Candidate with higher or equal term should be accepted if log is up-to-date
	req = &RequestVoteRequest{
		Term:         2,
		CandidateID:  "peer-1",
		LastLogIndex: 1,
		LastLogTerm:  1,
	}

	resp = node.HandleRequestVote(req)
	if !resp.VoteGranted {
		t.Error("Expected vote to be granted for higher term candidate")
	}
}

func TestHandleAppendEntries(t *testing.T) {
	config := DefaultConfig()
	config.NodeID = "test-node-5"
	config.BindAddress = "127.0.0.1:12005"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, _ := NewNode(config, fsm, transport)

	// Set our term higher than leader
	node.CurrentTerm = 2

	// Test: Leader with lower term is rejected
	req := &AppendEntriesRequest{
		Term:         1,
		LeaderID:     "leader-1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{},
		LeaderCommit: 0,
	}

	resp := node.HandleAppendEntries(req)
	if resp.Success {
		t.Error("Expected AppendEntries to be rejected for lower term")
	}

	// Test: Leader with higher term converts node to follower
	node.CurrentTerm = 1

	req = &AppendEntriesRequest{
		Term:         2,
		LeaderID:     "leader-1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{},
		LeaderCommit: 0,
	}

	resp = node.HandleAppendEntries(req)
	if !resp.Success {
		t.Error("Expected AppendEntries to be accepted for higher term")
	}

	if node.CurrentTerm != 2 {
		t.Errorf("Expected term to be updated to 2, got %d", node.CurrentTerm)
	}

	if node.State != StateFollower {
		t.Errorf("Expected state to be Follower, got %v", node.State)
	}
}

func TestGatewayFSMApply(t *testing.T) {
	fsm := NewGatewayFSM()

	// Test AddRoute
	route := &RouteConfig{
		ID:        "route-1",
		Name:      "Test Route",
		ServiceID: "service-1",
		Paths:     []string{"/test"},
		Methods:   []string{"GET"},
	}

	cmd := FSMCommand{
		Type:    CmdAddRoute,
		Payload: mustMarshal(t, route),
	}

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: mustMarshal(t, cmd),
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() error = %v", result)
	}

	r, ok := fsm.GetRoute("route-1")
	if !ok {
		t.Error("Expected route to be added")
	}
	if r.Name != "Test Route" {
		t.Errorf("Expected route name 'Test Route', got '%s'", r.Name)
	}

	// Test DeleteRoute
	cmd = FSMCommand{
		Type:    CmdDeleteRoute,
		Payload: mustMarshal(t, "route-1"),
	}

	entry = LogEntry{
		Index:   2,
		Term:    1,
		Command: mustMarshal(t, cmd),
	}

	fsm.Apply(entry)

	_, ok = fsm.GetRoute("route-1")
	if ok {
		t.Error("Expected route to be deleted")
	}
}

func TestGatewayFSMSnapshotRestore(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add some data
	fsm.Routes["route-1"] = &RouteConfig{ID: "route-1", Name: "Route 1"}
	fsm.Services["service-1"] = &ServiceConfig{ID: "service-1", Name: "Service 1"}
	fsm.RateLimitCounters["key-1"] = 100
	fsm.CreditBalances["user-1"] = 500

	// Create snapshot
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	// Create new FSM and restore
	newFSM := NewGatewayFSM()
	if err := newFSM.Restore(snapshot); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify data
	if _, ok := newFSM.GetRoute("route-1"); !ok {
		t.Error("Expected route to be restored")
	}

	if _, ok := newFSM.GetService("service-1"); !ok {
		t.Error("Expected service to be restored")
	}

	if newFSM.GetRateLimitCounter("key-1") != 100 {
		t.Error("Expected rate limit counter to be restored")
	}

	if newFSM.GetCreditBalance("user-1") != 500 {
		t.Error("Expected credit balance to be restored")
	}
}

func TestGatewayFSMCredits(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add credits
	update := struct {
		UserID string `json:"user_id"`
		Amount int64  `json:"amount"`
		Set    bool   `json:"set"`
	}{
		UserID: "user-1",
		Amount: 100,
		Set:    true,
	}

	cmd := FSMCommand{
		Type:    CmdUpdateCredits,
		Payload: mustMarshal(t, update),
	}

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: mustMarshal(t, cmd),
	}

	fsm.Apply(entry)

	if fsm.GetCreditBalance("user-1") != 100 {
		t.Errorf("Expected credit balance 100, got %d", fsm.GetCreditBalance("user-1"))
	}

	// Increment credits
	update.Set = false
	update.Amount = 50

	cmd.Payload = mustMarshal(t, update)
	entry.Index = 2
	entry.Command = mustMarshal(t, cmd)

	fsm.Apply(entry)

	if fsm.GetCreditBalance("user-1") != 150 {
		t.Errorf("Expected credit balance 150, got %d", fsm.GetCreditBalance("user-1"))
	}
}

func TestGatewayFSMHealthCheck(t *testing.T) {
	fsm := NewGatewayFSM()

	status := HealthStatus{
		ID:        "target-1",
		Healthy:   true,
		LastCheck: time.Now().Unix(),
		Successes: 5,
	}

	cmd := FSMCommand{
		Type:    CmdUpdateHealthCheck,
		Payload: mustMarshal(t, status),
	}

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: mustMarshal(t, cmd),
	}

	fsm.Apply(entry)

	s, ok := fsm.GetHealthCheck("target-1")
	if !ok {
		t.Fatal("Expected health check to be added")
	}

	if !s.Healthy {
		t.Error("Expected health status to be healthy")
	}
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	return data
}
