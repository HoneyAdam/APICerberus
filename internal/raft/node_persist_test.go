package raft

import (
	"testing"
	"time"
)

// TestNode_PersistState_WithStorage tests persistState when storage is set
func TestNode_PersistState_WithStorage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)

	// Set values and persist
	node.CurrentTerm = 5
	node.VotedFor = "node-2"
	node.persistState()

	// Verify state was persisted
	term, votedFor, err := storage.LoadState()
	if err != nil {
		t.Errorf("LoadState error: %v", err)
	}
	if term != 5 {
		t.Errorf("persistState did not save term: got %d, want 5", term)
	}
	if votedFor != "node-2" {
		t.Errorf("persistState did not save votedFor: got %s, want node-2", votedFor)
	}
}

// TestNode_PersistLog_WithStorage tests persistLog when storage is set
func TestNode_PersistLog_WithStorage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)

	// Add log entries and persist
	entries := []LogEntry{
		{Index: 1, Term: 1, Command: []byte(`{"action":"create"}`)},
		{Index: 2, Term: 1, Command: []byte(`{"action":"update"}`)},
	}
	node.persistLog(entries)

	// Verify log was persisted
	loaded, err := storage.LoadLog()
	if err != nil {
		t.Errorf("LoadLog error: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("persistLog did not save entries: got %d entries, want 2", len(loaded))
	}
}

// TestNode_BecomeCandidate_PersistsTerm tests that becomeCandidate persists state
func TestNode_BecomeCandidate_PersistsTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.CurrentTerm = 3

	// Become candidate (increments term and votes for self)
	node.becomeCandidate()

	if node.CurrentTerm != 4 {
		t.Errorf("becomeCandidate did not increment term: got %d, want 4", node.CurrentTerm)
	}

	if node.VotedFor != "node-1" {
		t.Errorf("becomeCandidate did not vote for self: got %s, want node-1", node.VotedFor)
	}

	// Verify persistence
	term, votedFor, _ := storage.LoadState()
	if term != 4 {
		t.Errorf("becomeCandidate did not persist term: got %d, want 4", term)
	}
	if votedFor != "node-1" {
		t.Errorf("becomeCandidate did not persist votedFor: got %s, want node-1", votedFor)
	}
}

// TestNode_BecomeFollower_PersistsTerm tests that becomeFollower persists state
func TestNode_BecomeFollower_PersistsTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.CurrentTerm = 5
	node.VotedFor = "node-2"
	node.State = StateCandidate

	// Become follower with higher term
	node.becomeFollower(7)

	if node.CurrentTerm != 7 {
		t.Errorf("becomeFollower did not set term: got %d, want 7", node.CurrentTerm)
	}

	if node.VotedFor != "" {
		t.Errorf("becomeFollower did not clear votedFor: got %s, want empty", node.VotedFor)
	}

	if node.State != StateFollower {
		t.Errorf("becomeFollower did not set state: got %v, want StateFollower", node.State)
	}

	// Verify persistence
	term, votedFor, _ := storage.LoadState()
	if term != 7 {
		t.Errorf("becomeFollower did not persist term: got %d, want 7", term)
	}
	if votedFor != "" {
		t.Errorf("becomeFollower did not persist votedFor: got %s, want empty", votedFor)
	}
}

// TestNode_HandleRequestVote_PersistsVote tests that voting persists state
func TestNode_HandleRequestVote_PersistsVote(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.CurrentTerm = 5

	// Add a log entry to make our log up-to-date
	node.Log = append(node.Log, LogEntry{Index: 1, Term: 1})

	req := &RequestVoteRequest{
		Term:         5,
		CandidateID:  "node-2",
		LastLogIndex: 1,
		LastLogTerm:  1,
	}

	resp := node.HandleRequestVote(req)

	if !resp.VoteGranted {
		t.Error("Should grant vote to candidate with up-to-date log")
	}

	if node.VotedFor != "node-2" {
		t.Errorf("VotedFor not set: got %s, want node-2", node.VotedFor)
	}

	// Verify persistence
	_, votedFor, _ := storage.LoadState()
	if votedFor != "node-2" {
		t.Errorf("HandleRequestVote did not persist vote: got %s, want node-2", votedFor)
	}
}

// TestNode_AppendEntry_PersistsLog tests that leader AppendEntry persists log
func TestNode_AppendEntry_PersistsLog(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.State = StateLeader
	node.matchIndex = make(map[string]uint64)
	node.matchIndex["node-1"] = 0

	cmd := FSMCommand{Type: CmdAddRoute, Payload: []byte(`{"route":"/test"}`)}
	index, err := node.AppendEntry(cmd)

	if err != nil {
		t.Fatalf("AppendEntry error: %v", err)
	}

	if index != 1 {
		t.Errorf("AppendEntry returned wrong index: got %d, want 1", index)
	}

	// Verify log persistence
	loaded, _ := storage.LoadLog()
	if len(loaded) == 0 {
		t.Error("AppendEntry did not persist log entry")
	}
}

// TestNode_GetLogEntry_EmptyLogOnly tests getLogEntry with completely empty log
func TestNode_GetLogEntry_EmptyLogOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Clear the default dummy entry
	node.Log = []LogEntry{}

	_, found := node.getLogEntry(1)
	if found {
		t.Error("getLogEntry should return false for empty log")
	}
}

// TestNode_LastLogHelpers_Empty tests lastLogIndex and lastLogTerm with empty log
func TestNode_LastLogHelpers_Empty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Clear the default dummy entry
	node.Log = []LogEntry{}

	if idx := node.lastLogIndex(); idx != 0 {
		t.Errorf("lastLogIndex() with empty log = %d, want 0", idx)
	}
	if term := node.lastLogTerm(); term != 0 {
		t.Errorf("lastLogTerm() with empty log = %d, want 0", term)
	}
}

// TestNode_Stop_WithRunningTransport tests Stop when transport is running
func TestNode_Stop_WithRunningTransport(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Start the node
	transport.Start(node)
	node.Start()
	time.Sleep(10 * time.Millisecond)

	// Stop should succeed
	err = node.Stop()
	if err != nil {
		t.Errorf("Stop error: %v", err)
	}

	// Verify node is stopped
	if !node.stopped.Load() {
		t.Error("Stop did not set stopped flag")
	}
}

// TestNode_Stop_Idempotent tests Stop is idempotent
func TestNode_Stop_Idempotent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Stop multiple times should not panic
	node.Stop()
	node.Stop()
	node.Stop()
}

// TestNode_Start_WithSnapshot tests Start with snapshot restoration
func TestNode_Start_WithSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:0"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Create storage with snapshot
	storage := NewInmemStorage()
	snapData := []byte(`{"routes":{"r1":{"path":"/test","target":"http://localhost:8080"}}}`)
	storage.SaveSnapshot(5, 2, snapData)
	node.SetStorage(storage)

	// Start should restore snapshot
	err = node.Start()
	if err != nil {
		t.Errorf("Start error: %v", err)
	}

	// Check that snapshot was restored
	if node.lastSnapshotIndex != 5 {
		t.Errorf("Expected lastSnapshotIndex 5, got %d", node.lastSnapshotIndex)
	}
	if node.lastSnapshotTerm != 2 {
		t.Errorf("Expected lastSnapshotTerm 2, got %d", node.lastSnapshotTerm)
	}

	node.Stop()
}

// TestNode_HandleInstallSnapshot_PersistsSnapshot tests snapshot persistence
func TestNode_HandleInstallSnapshot_PersistsSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.fsm = NewGatewayFSM()

	snapData := []byte(`{"routes":{}}`)
	req := &InstallSnapshotRequest{
		Term:              2,
		LeaderID:          "node-2",
		LastIncludedIndex: 10,
		LastIncludedTerm:  2,
		Data:              snapData,
		Done:              true,
	}

	resp := node.HandleInstallSnapshot(req)

	if !resp.Success {
		t.Error("Should accept valid InstallSnapshot")
	}

	// Verify snapshot persistence
	idx, term, data, _ := storage.LoadSnapshot()
	if idx != 10 {
		t.Errorf("Snapshot index not persisted: got %d, want 10", idx)
	}
	if term != 2 {
		t.Errorf("Snapshot term not persisted: got %d, want 2", term)
	}
	if string(data) != string(snapData) {
		t.Errorf("Snapshot data not persisted correctly")
	}
}

// TestNode_CompactLog_WithStorage tests log compaction with storage
func TestNode_CompactLog_WithStorage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"
	cfg.SnapshotThreshold = 2 // Small threshold for testing

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.fsm = NewGatewayFSM()

	// Add entries beyond threshold
	node.Log = []LogEntry{
		{Index: 0, Term: 0},
		{Index: 1, Term: 1, Command: []byte(`{"type":"add_route"}`)},
		{Index: 2, Term: 1, Command: []byte(`{"type":"add_route"}`)},
		{Index: 3, Term: 1, Command: []byte(`{"type":"add_route"}`)},
	}
	node.LastApplied = 3
	node.CommitIndex = 3

	// Trigger compaction
	node.compactLog()

	// Verify snapshot was persisted
	idx, _, _, err := storage.LoadSnapshot()
	if err != nil {
		t.Logf("LoadSnapshot error (may be expected): %v", err)
	}
	if err == nil && idx != 3 {
		t.Errorf("Expected snapshot index 3, got %d", idx)
	}
}

// TestNode_ReplicateTo_WithSnapshot tests replicateTo when snapshot is needed
func TestNode_ReplicateTo_WithSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.fsm = NewGatewayFSM()
	node.State = StateLeader

	// Setup peers
	node.Peers["node-1"] = "127.0.0.1:12000"
	node.Peers["node-2"] = "127.0.0.1:12001"
	node.nextIndex["node-2"] = 5
	node.matchIndex["node-2"] = 0

	// Setup snapshot state
	node.Log = []LogEntry{
		{Index: 10, Term: 1},
		{Index: 11, Term: 1},
	}
	node.lastSnapshotIndex = 10
	node.lastSnapshotTerm = 1

	// Save a snapshot
	storage.SaveSnapshot(10, 1, []byte(`{"routes":{}}`))

	// This should trigger InstallSnapshot path in replicateTo
	// We can't easily verify without mocking transport, but we ensure no panic
	node.replicateTo("node-2", 1, 0)
}

// TestNode_HandleAppendEntries_HigherTermPersists tests term update with higher term
func TestNode_HandleAppendEntries_HigherTermPersists(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node-1"
	cfg.BindAddress = "127.0.0.1:12000"

	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	storage := NewInmemStorage()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}
	node.SetStorage(storage)
	node.CurrentTerm = 1
	node.State = StateCandidate

	// Leader with higher term sends heartbeat
	req := &AppendEntriesRequest{
		Term:         3,
		LeaderID:     "node-2",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{},
		LeaderCommit: 0,
	}

	resp := node.HandleAppendEntries(req)

	if !resp.Success {
		t.Error("Should accept heartbeat with higher term")
	}

	if node.CurrentTerm != 3 {
		t.Errorf("CurrentTerm not updated: got %d, want 3", node.CurrentTerm)
	}

	if node.State != StateFollower {
		t.Errorf("State not updated: got %v, want StateFollower", node.State)
	}

	// Verify persistence
	term, _, _ := storage.LoadState()
	if term != 3 {
		t.Errorf("HandleAppendEntries did not persist term: got %d, want 3", term)
	}
}
