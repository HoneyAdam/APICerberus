package raft

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.ElectionTimeoutMin != 150*time.Millisecond {
		t.Errorf("ElectionTimeoutMin = %v, want 150ms", cfg.ElectionTimeoutMin)
	}
	if cfg.ElectionTimeoutMax != 300*time.Millisecond {
		t.Errorf("ElectionTimeoutMax = %v, want 300ms", cfg.ElectionTimeoutMax)
	}
	if cfg.HeartbeatInterval != 50*time.Millisecond {
		t.Errorf("HeartbeatInterval = %v, want 50ms", cfg.HeartbeatInterval)
	}
	if cfg.MaxEntriesPerAppend != 100 {
		t.Errorf("MaxEntriesPerAppend = %v, want 100", cfg.MaxEntriesPerAppend)
	}
	if cfg.SnapshotThreshold != 10000 {
		t.Errorf("SnapshotThreshold = %v, want 10000", cfg.SnapshotThreshold)
	}
	if cfg.SnapshotInterval != 5*time.Minute {
		t.Errorf("SnapshotInterval = %v, want 5m", cfg.SnapshotInterval)
	}
}

func TestNodeState_String(t *testing.T) {
	tests := []struct {
		state NodeState
		want  string
	}{
		{StateFollower, "Follower"},
		{StateCandidate, "Candidate"},
		{StateLeader, "Leader"},
		{NodeState(99), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("NodeState(%d).String() = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestNewNode_MissingID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = ""
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	_, err := NewNode(cfg, fsm, transport)
	if err == nil {
		t.Error("NewNode should return error when NodeID is missing")
	}
}

func TestNewNode_MissingAddress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = ""

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	_, err := NewNode(cfg, fsm, transport)
	if err == nil {
		t.Error("NewNode should return error when BindAddress is missing")
	}
}

func TestNode_SetStorage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	storage := NewInmemStorage()
	node.SetStorage(storage)

	if node.storage != storage {
		t.Error("SetStorage did not set storage correctly")
	}
}

func TestNode_AddPeer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Test adding peer as follower
	node.AddPeer("node-2", "127.0.0.1:12001")

	if _, ok := node.Peers["node-2"]; !ok {
		t.Error("Peer not added")
	}

	// Test adding peer as leader
	node.State = StateLeader
	node.nextIndex = make(map[string]uint64)
	node.matchIndex = make(map[string]uint64)

	node.AddPeer("node-3", "127.0.0.1:12002")

	if node.nextIndex["node-3"] != 1 {
		t.Errorf("nextIndex = %v, want 1", node.nextIndex["node-3"])
	}
	if node.matchIndex["node-3"] != 0 {
		t.Errorf("matchIndex = %v, want 0", node.matchIndex["node-3"])
	}
}

func TestNode_RemovePeer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Setup
	node.Peers["node-2"] = "127.0.0.1:12001"
	node.nextIndex["node-2"] = 10
	node.matchIndex["node-2"] = 5

	node.RemovePeer("node-2")

	if _, ok := node.Peers["node-2"]; ok {
		t.Error("Peer not removed")
	}
	if _, ok := node.nextIndex["node-2"]; ok {
		t.Error("nextIndex not removed")
	}
	if _, ok := node.matchIndex["node-2"]; ok {
		t.Error("matchIndex not removed")
	}
}

func TestNode_GetState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.State = StateLeader
	if state := node.GetState(); state != StateLeader {
		t.Errorf("GetState() = %v, want StateLeader", state)
	}
}

func TestNode_GetTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 42
	if term := node.GetTerm(); term != 42 {
		t.Errorf("GetTerm() = %v, want 42", term)
	}
}

func TestNode_IsLeader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.State = StateFollower
	if node.IsLeader() {
		t.Error("IsLeader() should be false for follower")
	}

	node.State = StateLeader
	if !node.IsLeader() {
		t.Error("IsLeader() should be true for leader")
	}
}

func TestNode_GetLeaderID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.leaderID = "node-2"
	if id := node.GetLeaderID(); id != "node-2" {
		t.Errorf("GetLeaderID() = %v, want node-2", id)
	}
}

func TestNode_lastLogIndex(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Node starts with dummy entry at index 0
	if idx := node.lastLogIndex(); idx != 0 {
		t.Errorf("lastLogIndex() = %v, want 0", idx)
	}

	// Add an entry
	node.Log = append(node.Log, LogEntry{Index: 1, Term: 1})
	if idx := node.lastLogIndex(); idx != 1 {
		t.Errorf("lastLogIndex() after append = %v, want 1", idx)
	}
}

func TestNode_lastLogTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Node starts with dummy entry at term 0
	if term := node.lastLogTerm(); term != 0 {
		t.Errorf("lastLogTerm() = %v, want 0", term)
	}

	// Add an entry
	node.Log = append(node.Log, LogEntry{Index: 1, Term: 5})
	if term := node.lastLogTerm(); term != 5 {
		t.Errorf("lastLogTerm() after append = %v, want 5", term)
	}
}

func TestNode_getLogEntry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Empty log
	_, ok := node.getLogEntry(1)
	if ok {
		t.Error("getLogEntry should return false for empty log")
	}

	// Add entries
	node.Log = []LogEntry{
		{Index: 0, Term: 0},
		{Index: 1, Term: 1, Command: []byte("cmd1")},
		{Index: 2, Term: 1, Command: []byte("cmd2")},
	}

	// Get valid entry
	entry, ok := node.getLogEntry(1)
	if !ok {
		t.Error("getLogEntry should return true for existing entry")
	}
	if entry.Index != 1 {
		t.Errorf("entry.Index = %v, want 1", entry.Index)
	}

	// Get out of bounds entry (before base)
	_, ok = node.getLogEntry(0)
	if ok {
		t.Error("getLogEntry should return false for index at base")
	}

	// Get out of bounds entry (after end)
	_, ok = node.getLogEntry(10)
	if ok {
		t.Error("getLogEntry should return false for index beyond log")
	}
}

func TestNode_AppendEntry_NotLeader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.State = StateFollower

	cmd := FSMCommand{Type: CmdAddRoute, Payload: []byte("{}")}
	_, err = node.AppendEntry(cmd)
	if err == nil {
		t.Error("AppendEntry should return error when not leader")
	}
}

func TestNode_AppendEntry_InvalidCommand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.State = StateLeader

	// Try to append something that can't be marshaled (channel)
	_, err = node.AppendEntry(make(chan int))
	if err == nil {
		t.Error("AppendEntry should return error for unmarshalable command")
	}
}

func TestNode_HandleRequestVote_LowerTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 5

	req := &RequestVoteRequest{
		Term:        3,
		CandidateID: "node-2",
	}
	resp := node.HandleRequestVote(req)

	if resp.VoteGranted {
		t.Error("Should not grant vote for lower term")
	}
	if resp.Term != 5 {
		t.Errorf("resp.Term = %v, want 5", resp.Term)
	}
}

func TestNode_HandleRequestVote_HigherTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 2
	node.State = StateLeader
	node.VotedFor = "node-1"

	req := &RequestVoteRequest{
		Term:         5,
		CandidateID:  "node-2",
		LastLogIndex: 1,
		LastLogTerm:  1,
	}
	// Add a log entry to make our log up-to-date
	node.Log = append(node.Log, LogEntry{Index: 1, Term: 1})

	resp := node.HandleRequestVote(req)

	// Response contains the term at the start (before update)
	if resp.Term != 2 {
		t.Errorf("resp.Term = %v, want 2 (term before update)", resp.Term)
	}
	// But node should have stepped down
	if node.State != StateFollower {
		t.Errorf("node.State = %v, want StateFollower", node.State)
	}
	// And updated its term
	if node.CurrentTerm != 5 {
		t.Errorf("node.CurrentTerm = %v, want 5", node.CurrentTerm)
	}
}

func TestNode_HandleRequestVote_AlreadyVoted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 1
	node.VotedFor = "node-2"

	req := &RequestVoteRequest{
		Term:         1,
		CandidateID:  "node-3",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}
	resp := node.HandleRequestVote(req)

	if resp.VoteGranted {
		t.Error("Should not grant vote if already voted for someone else")
	}
}

func TestNode_HandleRequestVote_OutdatedLog(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 5
	node.Log = []LogEntry{
		{Index: 0, Term: 0},
		{Index: 1, Term: 5},
		{Index: 2, Term: 5},
	}

	// Candidate has older log
	req := &RequestVoteRequest{
		Term:         5,
		CandidateID:  "node-2",
		LastLogIndex: 1,
		LastLogTerm:  5,
	}
	resp := node.HandleRequestVote(req)

	if resp.VoteGranted {
		t.Error("Should not grant vote if candidate has outdated log")
	}
}

func TestNode_HandleAppendEntries_LowerTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 5

	req := &AppendEntriesRequest{
		Term:     3,
		LeaderID: "node-2",
	}
	resp := node.HandleAppendEntries(req)

	if resp.Success {
		t.Error("Should reject AppendEntries with lower term")
	}
	if resp.Term != 5 {
		t.Errorf("resp.Term = %v, want 5", resp.Term)
	}
}

func TestNode_HandleAppendEntries_StalePrevLogIndex(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Simulate compacted log
	node.Log = []LogEntry{
		{Index: 10, Term: 1},
		{Index: 11, Term: 1},
	}
	node.lastSnapshotIndex = 10

	req := &AppendEntriesRequest{
		Term:         1,
		LeaderID:     "node-2",
		PrevLogIndex: 5, // Before our base index
		PrevLogTerm:  1,
	}
	resp := node.HandleAppendEntries(req)

	if resp.Success {
		t.Error("Should reject AppendEntries with stale prevLogIndex")
	}
	if resp.ConflictIndex != 11 {
		t.Errorf("ConflictIndex = %v, want 11", resp.ConflictIndex)
	}
}

func TestNode_HandleAppendEntries_MissingPrevLogEntry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.Log = []LogEntry{
		{Index: 0, Term: 0},
		{Index: 1, Term: 1},
	}

	req := &AppendEntriesRequest{
		Term:         1,
		LeaderID:     "node-2",
		PrevLogIndex: 5, // Beyond our log
		PrevLogTerm:  1,
	}
	resp := node.HandleAppendEntries(req)

	if resp.Success {
		t.Error("Should reject AppendEntries with missing prevLogEntry")
	}
	if resp.ConflictIndex != 2 {
		t.Errorf("ConflictIndex = %v, want 2", resp.ConflictIndex)
	}
}

func TestNode_HandleAppendEntries_TermMismatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.Log = []LogEntry{
		{Index: 0, Term: 0},
		{Index: 1, Term: 1},
		{Index: 2, Term: 1},
		{Index: 3, Term: 2},
	}

	// Leader expects entry at index 3 with term 1, but we have term 2
	req := &AppendEntriesRequest{
		Term:         3,
		LeaderID:     "node-2",
		PrevLogIndex: 3,
		PrevLogTerm:  1,
	}
	resp := node.HandleAppendEntries(req)

	if resp.Success {
		t.Error("Should reject AppendEntries with term mismatch")
	}
	if resp.ConflictTerm != 2 {
		t.Errorf("ConflictTerm = %v, want 2", resp.ConflictTerm)
	}
}

func TestNode_HandleInstallSnapshot_LowerTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 5

	req := &InstallSnapshotRequest{
		Term:     3,
		LeaderID: "node-2",
		Data:     []byte("snapshot"),
	}
	resp := node.HandleInstallSnapshot(req)

	if resp.Success {
		t.Error("Should reject InstallSnapshot with lower term")
	}
	if resp.Term != 5 {
		t.Errorf("resp.Term = %v, want 5", resp.Term)
	}
}

func TestNode_HandleInstallSnapshot_InvalidData(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	node.CurrentTerm = 1
	// Set up FSM
	node.fsm = NewGatewayFSM()

	req := &InstallSnapshotRequest{
		Term:              1,
		LeaderID:          "node-2",
		LastIncludedIndex: 10,
		LastIncludedTerm:  1,
		Data:              []byte("invalid json"),
	}
	resp := node.HandleInstallSnapshot(req)

	if resp.Success {
		t.Error("Should reject InstallSnapshot with invalid data")
	}
}

func TestNode_WaitForCommit_Timeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Wait for commit on an index that will never be committed
	err = node.WaitForCommit(100, 50*time.Millisecond)
	if err == nil {
		t.Error("WaitForCommit should timeout")
	}
}

func TestNode_WaitForCommit_NodeStopped(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Start and then stop the node
	transport.Start(node)
	node.Start()
	node.Stop()

	// Wait should fail because node is stopped
	err = node.WaitForCommit(1, time.Second)
	if err == nil {
		t.Error("WaitForCommit should fail when node is stopped")
	}
}

// TestNode_Start_WithStorage tests Start with storage restoration
func TestNode_Start_WithStorage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:0"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Create storage with some state
	storage := NewInmemStorage()
	storage.SaveState(5, "node-voted")
	node.SetStorage(storage)

	// Start should restore state from storage
	err = node.Start()
	if err != nil {
		t.Errorf("Start error: %v", err)
	}

	// Check that state was restored
	if node.CurrentTerm != 5 {
		t.Errorf("Expected term 5, got %d", node.CurrentTerm)
	}
	if node.VotedFor != "node-voted" {
		t.Errorf("Expected votedFor 'node-voted', got %s", node.VotedFor)
	}

	node.Stop()
}

// TestNode_Start_WithLogEntries tests Start with log entries restoration
func TestNode_Start_WithLogEntries(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:0"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Create storage with log entries
	storage := NewInmemStorage()
	entries := []LogEntry{
		{Index: 1, Term: 1, Command: []byte(`{"type":"test"}`)},
		{Index: 2, Term: 1, Command: []byte(`{"type":"test2"}`)},
	}
	storage.SaveLog(entries)
	node.SetStorage(storage)

	// Start should restore log entries
	err = node.Start()
	if err != nil {
		t.Errorf("Start error: %v", err)
	}

	// Check that log was restored
	if len(node.Log) != 2 {
		t.Errorf("Expected 2 log entries, got %d", len(node.Log))
	}

	node.Stop()
}
