package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// NodeState represents the current state of a Raft node.
type NodeState int

const (
	StateFollower NodeState = iota
	StateCandidate
	StateLeader
)

func (s NodeState) String() string {
	switch s {
	case StateFollower:
		return "Follower"
	case StateCandidate:
		return "Candidate"
	case StateLeader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// LogEntry represents a single entry in the Raft log.
type LogEntry struct {
	Index   uint64      `json:"index"`
	Term    uint64      `json:"term"`
	Command any `json:"command"`
}

// Node represents a single Raft node in the cluster.
type Node struct {
	// Identity
	ID      string `json:"id"`
	Address string `json:"address"`

	// Persistent state (must be persisted to stable storage)
	CurrentTerm uint64     `json:"current_term"`
	VotedFor    string     `json:"voted_for"`
	Log         []LogEntry `json:"log"`

	// Volatile state
	CommitIndex uint64 `json:"commit_index"`
	LastApplied uint64 `json:"last_applied"`

	// Volatile state for leaders
	nextIndex  map[string]uint64
	matchIndex map[string]uint64

	// Snapshot state
	lastSnapshotIndex uint64
	lastSnapshotTerm  uint64

	// State machine
	State NodeState `json:"state"`

	// Leader tracking
	leaderID string

	// Cluster membership
	Peers map[string]string `json:"peers"` // node ID -> address

	// Channels for event handling
	electionTimeoutCh chan struct{}
	heartbeatCh       chan struct{}
	applyCh           chan LogEntry
	stopCh            chan struct{}
	stopped           atomic.Bool
	heartbeatRunning  atomic.Bool

	// State machine interface
	fsm StateMachine

	// Transport layer
	transport Transport

	// Persistent storage (optional)
	storage Storage

	// Configuration
	config *Config

	// Synchronization
	mu            sync.RWMutex
	electionTimer *time.Timer
}

// Storage interface for persisting Raft state.
type Storage interface {
	SaveState(term uint64, votedFor string) error
	LoadState() (term uint64, votedFor string, err error)
	SaveLog(entries []LogEntry) error
	LoadLog() ([]LogEntry, error)
	SaveSnapshot(index, term uint64, data []byte) error
	LoadSnapshot() (index, term uint64, data []byte, err error)
}

// Config holds Raft node configuration.
type Config struct {
	// Node ID and address
	NodeID      string `yaml:"node_id"`
	BindAddress string `yaml:"bind_address"`

	// Election timeout range (randomized between min and max)
	ElectionTimeoutMin time.Duration `yaml:"election_timeout_min"`
	ElectionTimeoutMax time.Duration `yaml:"election_timeout_max"`

	// Heartbeat interval (leader sends heartbeats this often)
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`

	// Maximum log entries per AppendEntries RPC
	MaxEntriesPerAppend int `yaml:"max_entries_per_append"`

	// Snapshot configuration
	SnapshotThreshold uint64        `yaml:"snapshot_threshold"`
	SnapshotInterval  time.Duration `yaml:"snapshot_interval"`
}

// DefaultConfig returns a default Raft configuration.
func DefaultConfig() *Config {
	return &Config{
		ElectionTimeoutMin:  150 * time.Millisecond,
		ElectionTimeoutMax:  300 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		MaxEntriesPerAppend: 100,
		SnapshotThreshold:   10000,
		SnapshotInterval:    5 * time.Minute,
	}
}

// StateMachine interface for applying committed log entries.
type StateMachine interface {
	Apply(entry LogEntry) any
	Snapshot() ([]byte, error)
	Restore(snapshot []byte) error
}

// Transport interface for network communication between nodes.
type Transport interface {
	// RPC calls
	RequestVote(nodeID string, req *RequestVoteRequest) (*RequestVoteResponse, error)
	AppendEntries(nodeID string, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
	InstallSnapshot(nodeID string, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error)

	// Server lifecycle
	Start(handler RPCHandler) error
	Stop() error

	// Address
	LocalAddr() string
}

// RPCHandler handles incoming RPC requests.
type RPCHandler interface {
	HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse
	HandleAppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse
	HandleInstallSnapshot(req *InstallSnapshotRequest) *InstallSnapshotResponse
}

// NewNode creates a new Raft node.
func NewNode(config *Config, fsm StateMachine, transport Transport) (*Node, error) {
	if config.NodeID == "" {
		return nil, fmt.Errorf("node ID is required")
	}
	if config.BindAddress == "" {
		return nil, fmt.Errorf("bind address is required")
	}

	n := &Node{
		ID:                config.NodeID,
		Address:           config.BindAddress,
		Log:               make([]LogEntry, 0),
		State:             StateFollower,
		Peers:             make(map[string]string),
		nextIndex:         make(map[string]uint64),
		matchIndex:        make(map[string]uint64),
		electionTimeoutCh: make(chan struct{}, 1),
		heartbeatCh:       make(chan struct{}),
		applyCh:           make(chan LogEntry, 100),
		stopCh:            make(chan struct{}),
		fsm:               fsm,
		transport:         transport,
		config:            config,
	}

	// Add dummy entry at index 0
	n.Log = append(n.Log, LogEntry{Index: 0, Term: 0})

	return n, nil
}

// SetStorage sets the persistent storage backend.
func (n *Node) SetStorage(s Storage) {
	n.storage = s
}

// Start starts the Raft node.
func (n *Node) Start() error {
	// Restore persisted state if available
	if n.storage != nil {
		term, votedFor, err := n.storage.LoadState()
		if err == nil {
			n.CurrentTerm = term
			n.VotedFor = votedFor
		}
		logEntries, err := n.storage.LoadLog()
		if err == nil && len(logEntries) > 0 {
			n.Log = logEntries
		}

		// Restore snapshot if available
		snapIndex, snapTerm, snapData, err := n.storage.LoadSnapshot()
		if err == nil && snapIndex > 0 && len(snapData) > 0 {
			if n.fsm != nil {
				if err := n.fsm.Restore(snapData); err == nil {
					n.lastSnapshotIndex = snapIndex
					n.lastSnapshotTerm = snapTerm
					if snapIndex > n.LastApplied {
						n.LastApplied = snapIndex
					}
					if snapIndex > n.CommitIndex {
						n.CommitIndex = snapIndex
					}
				}
			}
		}
	}

	// Start transport
	if err := n.transport.Start(n); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Start election timer
	n.resetElectionTimer()

	// Start main processing goroutine
	go n.run()

	return nil
}

// Stop stops the Raft node. Safe to call multiple times.
func (n *Node) Stop() error {
	if !n.stopped.CompareAndSwap(false, true) {
		return nil
	}
	close(n.stopCh)
	if n.electionTimer != nil {
		n.electionTimer.Stop()
	}
	return n.transport.Stop()
}

// run is the main event loop.
func (n *Node) run() {
	for {
		select {
		case <-n.stopCh:
			return
		case <-n.electionTimeoutCh:
			n.handleElectionTimeout()
		}
	}
}

// handleElectionTimeout handles election timeout.
func (n *Node) handleElectionTimeout() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.State == StateLeader {
		return
	}

	n.becomeCandidate()
}

// becomeCandidate converts node to candidate state and starts election.
func (n *Node) becomeCandidate() {
	n.State = StateCandidate
	n.CurrentTerm++
	n.VotedFor = n.ID
	n.leaderID = ""

	n.persistState()

	// Use atomic counter for thread-safe vote counting
	var votesReceived atomic.Int32
	votesReceived.Store(1) // vote for self
	votesNeeded := int32((len(n.Peers)+1)/2 + 1) // #nosec G115 -- Raft peer counts are small and fit safely in int32.

	currentTerm := n.CurrentTerm

	req := &RequestVoteRequest{
		Term:         n.CurrentTerm,
		CandidateID:  n.ID,
		LastLogIndex: n.lastLogIndex(),
		LastLogTerm:  n.lastLogTerm(),
	}

	for peerID := range n.Peers {
		if peerID == n.ID {
			continue
		}

		go func(id string) {
			resp, err := n.transport.RequestVote(id, req)
			if err != nil {
				return
			}

			n.mu.Lock()
			defer n.mu.Unlock()

			if n.State != StateCandidate || n.CurrentTerm != currentTerm {
				return
			}

			if resp.Term > n.CurrentTerm {
				n.becomeFollower(resp.Term)
				return
			}

			if resp.VoteGranted {
				if votesReceived.Add(1) >= votesNeeded {
					n.becomeLeader()
				}
			}
		}(peerID)
	}

	n.resetElectionTimer()
}

// becomeLeader converts node to leader state.
func (n *Node) becomeLeader() {
	if n.State == StateLeader {
		return
	}

	n.State = StateLeader
	n.leaderID = n.ID

	// Initialize leader volatile state
	lastIndex := n.lastLogIndex()
	for peerID := range n.Peers {
		n.nextIndex[peerID] = lastIndex + 1
		n.matchIndex[peerID] = 0
	}

	// Start sending heartbeats and replicating log
	if n.heartbeatRunning.CompareAndSwap(false, true) {
		go n.sendHeartbeats()
	}
}

// becomeFollower converts node to follower state.
func (n *Node) becomeFollower(term uint64) {
	n.State = StateFollower
	n.CurrentTerm = term
	n.VotedFor = ""
	n.persistState()
	n.resetElectionTimer()
}

// sendHeartbeats sends periodic heartbeats with log entries to all peers.
func (n *Node) sendHeartbeats() {
	defer n.heartbeatRunning.Store(false)
	ticker := time.NewTicker(n.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			if n.State != StateLeader {
				n.mu.RUnlock()
				return
			}

			currentTerm := n.CurrentTerm
			commitIndex := n.CommitIndex
			peers := make(map[string]string, len(n.Peers))
			for id, addr := range n.Peers {
				peers[id] = addr
			}
			n.mu.RUnlock()

			for peerID := range peers {
				if peerID == n.ID {
					continue
				}
				go n.replicateTo(peerID, currentTerm, commitIndex)
			}
		}
	}
}

// replicateTo sends log entries to a specific peer.
func (n *Node) replicateTo(peerID string, term, commitIndex uint64) {
	n.mu.RLock()
	nextIdx, ok := n.nextIndex[peerID]
	if !ok {
		n.mu.RUnlock()
		return
	}

	// If the follower is too far behind (needs entries we already compacted),
	// send an InstallSnapshot instead of AppendEntries.
	if n.lastSnapshotIndex > 0 && nextIdx <= n.lastSnapshotIndex {
		snapIndex := n.lastSnapshotIndex
		snapTerm := n.lastSnapshotTerm

		// Load snapshot data
		var snapData []byte
		if n.storage != nil {
			_, _, data, err := n.storage.LoadSnapshot()
			if err == nil && len(data) > 0 {
				snapData = data
			}
		}
		// If no storage, take a live snapshot from the FSM
		if snapData == nil && n.fsm != nil {
			data, err := n.fsm.Snapshot()
			if err == nil {
				snapData = data
			}
		}
		n.mu.RUnlock()

		if snapData == nil {
			return
		}

		snapReq := &InstallSnapshotRequest{
			Term:              term,
			LeaderID:          n.ID,
			LastIncludedIndex: snapIndex,
			LastIncludedTerm:  snapTerm,
			Data:              snapData,
			Done:              true,
		}

		snapResp, err := n.transport.InstallSnapshot(peerID, snapReq)
		if err != nil {
			return
		}

		n.mu.Lock()
		defer n.mu.Unlock()

		if snapResp.Term > n.CurrentTerm {
			n.becomeFollower(snapResp.Term)
			return
		}

		if snapResp.Success {
			n.nextIndex[peerID] = snapIndex + 1
			n.matchIndex[peerID] = snapIndex
		}
		return
	}

	baseIndex := n.Log[0].Index

	// Build PrevLogIndex/PrevLogTerm
	prevLogIndex := nextIdx - 1
	prevLogTerm := uint64(0)
	if prevLogIndex >= baseIndex {
		offset := prevLogIndex - baseIndex
		if offset < uint64(len(n.Log)) {
			prevLogTerm = n.Log[offset].Term
		}
	}

	// Collect entries to send
	var entries []LogEntry
	lastLogIdx := n.lastLogIndex()
	if nextIdx <= lastLogIdx {
		startOffset := nextIdx - baseIndex
		end := nextIdx + uint64(n.config.MaxEntriesPerAppend) // #nosec G115 -- MaxEntriesPerAppend is a small positive config value.
		if end > lastLogIdx+1 {
			end = lastLogIdx + 1
		}
		endOffset := end - baseIndex
		if startOffset < uint64(len(n.Log)) && endOffset <= uint64(len(n.Log)) {
			entries = make([]LogEntry, endOffset-startOffset)
			copy(entries, n.Log[startOffset:endOffset])
		}
	}
	n.mu.RUnlock()

	req := &AppendEntriesRequest{
		Term:         term,
		LeaderID:     n.ID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: commitIndex,
	}

	resp, err := n.transport.AppendEntries(peerID, req)
	if err != nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Step down if peer has higher term
	if resp.Term > n.CurrentTerm {
		n.becomeFollower(resp.Term)
		return
	}

	// Ignore stale responses
	if n.State != StateLeader || n.CurrentTerm != term {
		return
	}

	if resp.Success {
		// Advance nextIndex and matchIndex
		newMatchIndex := prevLogIndex + uint64(len(entries))
		if newMatchIndex > n.matchIndex[peerID] {
			n.matchIndex[peerID] = newMatchIndex
		}
		n.nextIndex[peerID] = newMatchIndex + 1
		n.advanceCommitIndex()
	} else {
		// Backtrack nextIndex using conflict info
		if resp.ConflictTerm > 0 && resp.ConflictIndex > 0 {
			n.nextIndex[peerID] = resp.ConflictIndex
		} else if n.nextIndex[peerID] > 1 {
			n.nextIndex[peerID]--
		}
	}
}

// advanceCommitIndex checks if we can advance the commit index.
func (n *Node) advanceCommitIndex() {
	baseIndex := n.Log[0].Index
	for idx := n.lastLogIndex(); idx > n.CommitIndex; idx-- {
		if idx < baseIndex {
			break
		}
		offset := idx - baseIndex
		if offset >= uint64(len(n.Log)) {
			continue
		}
		// Only commit entries from current term (Raft safety property)
		if n.Log[offset].Term != n.CurrentTerm {
			continue
		}

		// Count replicas (including self)
		replicaCount := 1
		for peerID := range n.Peers {
			if peerID == n.ID {
				continue
			}
			if n.matchIndex[peerID] >= idx {
				replicaCount++
			}
		}

		// Check for majority
		totalNodes := len(n.Peers) + 1 // peers + self
		if replicaCount > totalNodes/2 {
			n.CommitIndex = idx
			n.applyCommitted()
			break
		}
	}
}

// applyCommitted applies committed but unapplied log entries to the FSM.
func (n *Node) applyCommitted() {
	for n.LastApplied < n.CommitIndex {
		n.LastApplied++
		// Guard against uint64 underflow: if the log base index is ahead of
		// LastApplied, the entry was already covered by a prior snapshot.
		if len(n.Log) == 0 || n.Log[0].Index > n.LastApplied {
			continue
		}
		// Compute position in the slice relative to the snapshot base index.
		offset := n.LastApplied - n.Log[0].Index
		if offset < uint64(len(n.Log)) {
			entry := n.Log[offset]
			if n.fsm != nil {
				n.fsm.Apply(entry)
			}
		}
	}
	n.maybeSnapshot()
}

// maybeSnapshot checks whether a snapshot should be taken and triggers compaction.
// Must be called with n.mu held.
func (n *Node) maybeSnapshot() {
	if n.config.SnapshotThreshold == 0 {
		return
	}
	if n.LastApplied-n.lastSnapshotIndex >= n.config.SnapshotThreshold {
		n.compactLog()
	}
}

// compactLog takes a snapshot of the FSM and trims the log.
// Must be called with n.mu held.
func (n *Node) compactLog() {
	if n.fsm == nil {
		return
	}

	// Take a snapshot of the FSM
	snapData, err := n.fsm.Snapshot()
	if err != nil {
		return
	}

	lastIncludedIndex := n.LastApplied
	lastIncludedTerm := uint64(0)
	baseIndex := n.Log[0].Index

	// Find the term of the last applied entry using offset-based access
	appliedOffset := lastIncludedIndex - baseIndex
	if appliedOffset < uint64(len(n.Log)) {
		lastIncludedTerm = n.Log[appliedOffset].Term
	} else if len(n.Log) > 0 {
		// The entry might have been compacted already; use last log entry's term
		lastIncludedTerm = n.Log[len(n.Log)-1].Term
	}

	// Persist the snapshot
	if n.storage != nil {
		if err := n.storage.SaveSnapshot(lastIncludedIndex, lastIncludedTerm, snapData); err != nil {
			return
		}
	}

	// Trim the log: keep only entries after lastIncludedIndex
	// Replace compacted entries with a single dummy entry that records the snapshot boundary
	if appliedOffset < uint64(len(n.Log)) {
		tail := make([]LogEntry, len(n.Log)-int(appliedOffset)) // #nosec G115 -- appliedOffset is bounded by len(n.Log) above.
		tail[0] = LogEntry{Index: lastIncludedIndex, Term: lastIncludedTerm}
		copy(tail[1:], n.Log[appliedOffset+1:])
		n.Log = tail
	} else {
		n.Log = []LogEntry{{Index: lastIncludedIndex, Term: lastIncludedTerm}}
	}

	n.lastSnapshotIndex = lastIncludedIndex
	n.lastSnapshotTerm = lastIncludedTerm
}

// resetElectionTimer resets the election timer with a random timeout.
func (n *Node) resetElectionTimer() {
	if n.electionTimer != nil {
		n.electionTimer.Stop()
	}

	// Random timeout between min and max
	duration := n.config.ElectionTimeoutMin +
		time.Duration(float64(n.config.ElectionTimeoutMax-n.config.ElectionTimeoutMin)*rand.Float64()) // #nosec G404 -- math/rand/v2 is acceptable for Raft election timeout jitter.

	n.electionTimer = time.AfterFunc(duration, func() {
		select {
		case n.electionTimeoutCh <- struct{}{}:
		default:
		}
	})
}

// persistState persists current term and votedFor to stable storage.
func (n *Node) persistState() {
	if n.storage != nil {
		if err := n.storage.SaveState(n.CurrentTerm, n.VotedFor); err != nil {
			log.Printf("[WARN] raft: failed to persist state: %v", err)
		}
	}
}

// persistLog persists new log entries to stable storage.
func (n *Node) persistLog(entries []LogEntry) {
	if n.storage != nil {
		if err := n.storage.SaveLog(entries); err != nil {
			log.Printf("[WARN] raft: failed to persist log: %v", err)
		}
	}
}

// Helper functions for log access.
func (n *Node) lastLogIndex() uint64 {
	if len(n.Log) == 0 {
		return 0
	}
	return n.Log[len(n.Log)-1].Index
}

func (n *Node) lastLogTerm() uint64 {
	if len(n.Log) == 0 {
		return 0
	}
	return n.Log[len(n.Log)-1].Term
}

// AddPeer adds a peer to the cluster.
func (n *Node) AddPeer(id, address string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Peers[id] = address
	if n.State == StateLeader {
		n.nextIndex[id] = n.lastLogIndex() + 1
		n.matchIndex[id] = 0
	}
}

// RemovePeer removes a peer from the cluster.
func (n *Node) RemovePeer(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	delete(n.Peers, id)
	delete(n.nextIndex, id)
	delete(n.matchIndex, id)
}

// GetState returns the current node state.
func (n *Node) GetState() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.State
}

// GetTerm returns the current term.
func (n *Node) GetTerm() uint64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.CurrentTerm
}

// IsLeader returns true if this node is the leader.
func (n *Node) IsLeader() bool {
	return n.GetState() == StateLeader
}

// GetLeaderID returns the leader's ID (empty if unknown).
func (n *Node) GetLeaderID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.leaderID
}

// AppendEntry appends a new entry to the leader's log.
// Returns the index of the new entry.
func (n *Node) AppendEntry(command any) (uint64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.State != StateLeader {
		return 0, fmt.Errorf("not leader")
	}

	data, err := json.Marshal(command)
	if err != nil {
		return 0, err
	}

	entry := LogEntry{
		Index:   n.lastLogIndex() + 1,
		Term:    n.CurrentTerm,
		Command: data,
	}

	n.Log = append(n.Log, entry)
	n.persistLog([]LogEntry{entry})

	// Update own matchIndex
	n.matchIndex[n.ID] = entry.Index

	return entry.Index, nil
}

// WaitForCommit waits until the given index is committed or timeout.
func (n *Node) WaitForCommit(index uint64, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("commit timeout for index %d", index)
		case <-n.stopCh:
			return fmt.Errorf("node stopped")
		case <-ticker.C:
			n.mu.RLock()
			committed := n.CommitIndex >= index
			n.mu.RUnlock()
			if committed {
				return nil
			}
		}
	}
}

// HandleRequestVote handles incoming RequestVote RPCs.
func (n *Node) HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &RequestVoteResponse{
		Term: n.CurrentTerm,
	}

	if req.Term < n.CurrentTerm {
		resp.VoteGranted = false
		return resp
	}

	if req.Term > n.CurrentTerm {
		n.CurrentTerm = req.Term
		n.State = StateFollower
		n.VotedFor = ""
		n.leaderID = ""
		n.persistState()
	}

	if n.VotedFor == "" || n.VotedFor == req.CandidateID {
		lastIndex := n.lastLogIndex()
		lastTerm := n.lastLogTerm()

		if req.LastLogTerm > lastTerm ||
			(req.LastLogTerm == lastTerm && req.LastLogIndex >= lastIndex) {
			n.VotedFor = req.CandidateID
			n.persistState()
			n.resetElectionTimer()
			resp.VoteGranted = true
			return resp
		}
	}

	resp.VoteGranted = false
	return resp
}

// HandleAppendEntries handles incoming AppendEntries RPCs.
func (n *Node) HandleAppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &AppendEntriesResponse{
		Term: n.CurrentTerm,
	}

	if req.Term < n.CurrentTerm {
		resp.Success = false
		return resp
	}

	// Valid leader — track it and reset to follower
	n.leaderID = req.LeaderID
	if req.Term > n.CurrentTerm || n.State != StateFollower {
		n.CurrentTerm = req.Term
		n.State = StateFollower
		n.VotedFor = ""
		n.persistState()
	}

	n.resetElectionTimer()

	// Check previous log entry consistency
	baseIndex := n.Log[0].Index
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex < baseIndex {
			// We already have a snapshot past this point; this is stale
			resp.Success = false
			resp.ConflictIndex = baseIndex + 1
			return resp
		}
		prevOffset := req.PrevLogIndex - baseIndex
		if prevOffset >= uint64(len(n.Log)) {
			resp.Success = false
			resp.ConflictIndex = baseIndex + uint64(len(n.Log))
			return resp
		}
		if n.Log[prevOffset].Term != req.PrevLogTerm {
			resp.Success = false
			resp.ConflictTerm = n.Log[prevOffset].Term
			for i := prevOffset; i > 0; i-- {
				if n.Log[i-1].Term != resp.ConflictTerm {
					resp.ConflictIndex = baseIndex + i
					break
				}
			}
			return resp
		}
	}

	// Append new entries
	var newEntries []LogEntry
	for i, entry := range req.Entries {
		index := req.PrevLogIndex + 1 + uint64(i)
		offset := index - baseIndex
		if offset < uint64(len(n.Log)) {
			if n.Log[offset].Term != entry.Term {
				n.Log = n.Log[:offset]
				n.Log = append(n.Log, entry)
				newEntries = append(newEntries, entry)
			}
		} else {
			n.Log = append(n.Log, entry)
			newEntries = append(newEntries, entry)
		}
	}

	if len(newEntries) > 0 {
		n.persistLog(newEntries)
	}

	// Update commit index
	if req.LeaderCommit > n.CommitIndex {
		lastNewIndex := req.PrevLogIndex + uint64(len(req.Entries))
		if req.LeaderCommit < lastNewIndex {
			n.CommitIndex = req.LeaderCommit
		} else {
			n.CommitIndex = lastNewIndex
		}
		n.applyCommitted()
	}

	resp.Success = true
	return resp
}

// HandleInstallSnapshot handles incoming InstallSnapshot RPCs.
func (n *Node) HandleInstallSnapshot(req *InstallSnapshotRequest) *InstallSnapshotResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &InstallSnapshotResponse{
		Term: n.CurrentTerm,
	}

	if req.Term < n.CurrentTerm {
		resp.Success = false
		return resp
	}

	n.leaderID = req.LeaderID
	if req.Term > n.CurrentTerm {
		n.CurrentTerm = req.Term
		n.State = StateFollower
		n.VotedFor = ""
		n.persistState()
	}

	n.resetElectionTimer()

	// Apply snapshot to FSM
	if n.fsm != nil && len(req.Data) > 0 {
		if err := n.fsm.Restore(req.Data); err != nil {
			resp.Success = false
			return resp
		}
	}

	// Update log to reflect snapshot
	baseIndex := n.Log[0].Index
	if req.LastIncludedIndex >= baseIndex+uint64(len(n.Log)) {
		// Snapshot is ahead of our entire log; discard all entries
		n.Log = []LogEntry{{Index: req.LastIncludedIndex, Term: req.LastIncludedTerm}}
	} else if req.LastIncludedIndex >= baseIndex {
		// Keep entries after the snapshot point
		offset := req.LastIncludedIndex - baseIndex
		remaining := n.Log[offset+1:]
		n.Log = make([]LogEntry, 1+len(remaining))
		n.Log[0] = LogEntry{Index: req.LastIncludedIndex, Term: req.LastIncludedTerm}
		copy(n.Log[1:], remaining)
	}
	// If req.LastIncludedIndex < baseIndex, we already have a newer snapshot; ignore

	if req.LastIncludedIndex > n.LastApplied {
		n.LastApplied = req.LastIncludedIndex
	}
	if req.LastIncludedIndex > n.CommitIndex {
		n.CommitIndex = req.LastIncludedIndex
	}

	n.lastSnapshotIndex = req.LastIncludedIndex
	n.lastSnapshotTerm = req.LastIncludedTerm

	// Persist the snapshot
	if n.storage != nil {
		if err := n.storage.SaveSnapshot(req.LastIncludedIndex, req.LastIncludedTerm, req.Data); err != nil {
			log.Printf("[WARN] raft: failed to save snapshot: %v", err)
		}
	}

	resp.Success = true
	return resp
}
