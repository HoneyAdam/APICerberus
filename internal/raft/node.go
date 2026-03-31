package raft

import (
	"fmt"
	"sync"
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
	Command interface{} `json:"command"`
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
	nextIndex         map[string]uint64
	matchIndex        map[string]uint64

	// State machine
	State NodeState `json:"state"`

	// Cluster membership
	Peers map[string]string `json:"peers"` // node ID -> address

	// Channels for event handling
	electionTimeoutCh chan struct{}
	heartbeatCh       chan struct{}
	applyCh           chan LogEntry
	stopCh            chan struct{}

	// State machine interface
	fsm StateMachine

	// Transport layer
	transport Transport

	// Configuration
	config *Config

	// Synchronization
	mu          sync.RWMutex
	electionTimer *time.Timer
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
	Apply(entry LogEntry) interface{}
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
		electionTimeoutCh: make(chan struct{}),
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

// Start starts the Raft node.
func (n *Node) Start() error {
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

// Stop stops the Raft node.
func (n *Node) Stop() error {
	close(n.stopCh)
	return n.transport.Stop()
}

// run is the main event loop.
func (n *Node) run() {
	for {
		select {
		case <-n.stopCh:
			return
		case <-n.electionTimer.C:
			n.handleElectionTimeout()
		}
	}
}

// handleElectionTimeout handles election timeout.
func (n *Node) handleElectionTimeout() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.State == StateLeader {
		// Leader doesn't need election timer
		return
	}

	// Convert to candidate and start election
	n.becomeCandidate()
}

// becomeCandidate converts node to candidate state and starts election.
func (n *Node) becomeCandidate() {
	n.State = StateCandidate
	n.CurrentTerm++
	n.VotedFor = n.ID

	// Vote for self
	votesReceived := 1
	votesNeeded := (len(n.Peers)+1)/2 + 1

	// Request votes from all peers
	req := &RequestVoteRequest{
		Term:         n.CurrentTerm,
		CandidateID:  n.ID,
		LastLogIndex: n.lastLogIndex(),
		LastLogTerm:  n.lastLogTerm(),
	}

	// Send vote requests concurrently
	for peerID, peerAddr := range n.Peers {
		if peerID == n.ID {
			continue
		}

		go func(id, addr string) {
			resp, err := n.transport.RequestVote(id, req)
			if err != nil {
				return
			}

			n.mu.Lock()
			defer n.mu.Unlock()

			// Check if we're still a candidate in the same term
			if n.State != StateCandidate || n.CurrentTerm != req.Term {
				return
			}

			// If peer has higher term, step down
			if resp.Term > n.CurrentTerm {
				n.CurrentTerm = resp.Term
				n.State = StateFollower
				n.VotedFor = ""
				n.resetElectionTimer()
				return
			}

			// Count vote
			if resp.VoteGranted {
				votesReceived++
				if votesReceived >= votesNeeded {
					n.becomeLeader()
				}
			}
		}(peerID, peerAddr)
	}

	// Reset election timer
	n.resetElectionTimer()
}

// becomeLeader converts node to leader state.
func (n *Node) becomeLeader() {
	if n.State == StateLeader {
		return
	}

	n.State = StateLeader

	// Initialize leader state
	lastIndex := n.lastLogIndex()
	for peerID := range n.Peers {
		n.nextIndex[peerID] = lastIndex + 1
		n.matchIndex[peerID] = 0
	}

	// Start sending heartbeats
	go n.sendHeartbeats()
}

// becomeFollower converts node to follower state.
func (n *Node) becomeFollower(term uint64) {
	n.State = StateFollower
	n.CurrentTerm = term
	n.VotedFor = ""
	n.resetElectionTimer()
}

// sendHeartbeats sends periodic heartbeats to all peers.
func (n *Node) sendHeartbeats() {
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

			req := &AppendEntriesRequest{
				Term:         n.CurrentTerm,
				LeaderID:     n.ID,
				PrevLogIndex: n.lastLogIndex(),
				PrevLogTerm:  n.lastLogTerm(),
				Entries:      []LogEntry{},
				LeaderCommit: n.CommitIndex,
			}
			n.mu.RUnlock()

			// Send to all peers
			for peerID := range n.Peers {
				if peerID == n.ID {
					continue
				}
				go func(id string) {
					n.transport.AppendEntries(id, req)
				}(peerID)
			}
		}
	}
}

// resetElectionTimer resets the election timer with a random timeout.
func (n *Node) resetElectionTimer() {
	if n.electionTimer != nil {
		n.electionTimer.Stop()
	}

	// Random timeout between min and max
	duration := n.config.ElectionTimeoutMin +
		time.Duration(float64(n.config.ElectionTimeoutMax-n.config.ElectionTimeoutMin)*randFloat())

	n.electionTimer = time.AfterFunc(duration, func() {
		select {
		case n.electionTimeoutCh <- struct{}{}:
		default:
		}
	})
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

func (n *Node) getLogEntry(index uint64) (LogEntry, bool) {
	if index == 0 || index > uint64(len(n.Log)) {
		return LogEntry{}, false
	}
	return n.Log[index], true
}

// randFloat returns a random float between 0 and 1.
func randFloat() float64 {
	// Simple deterministic for now
	return 0.5
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

	if n.State == StateLeader {
		return n.ID
	}
	// In a real implementation, track leader ID from AppendEntries
	return ""
}

// HandleRequestVote handles incoming RequestVote RPCs.
func (n *Node) HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &RequestVoteResponse{
		Term: n.CurrentTerm,
	}

	// If candidate's term is lower, reject
	if req.Term < n.CurrentTerm {
		resp.VoteGranted = false
		return resp
	}

	// If candidate's term is higher, update our term and become follower
	if req.Term > n.CurrentTerm {
		n.CurrentTerm = req.Term
		n.State = StateFollower
		n.VotedFor = ""
	}

	// Check if we can vote for this candidate
	if n.VotedFor == "" || n.VotedFor == req.CandidateID {
		// Check if candidate's log is at least as up-to-date as ours
		lastIndex := n.lastLogIndex()
		lastTerm := n.lastLogTerm()

		if req.LastLogTerm > lastTerm ||
			(req.LastLogTerm == lastTerm && req.LastLogIndex >= lastIndex) {
			n.VotedFor = req.CandidateID
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

	// If leader's term is lower, reject
	if req.Term < n.CurrentTerm {
		resp.Success = false
		return resp
	}

	// If leader's term is higher, update our term and become follower
	if req.Term > n.CurrentTerm {
		n.CurrentTerm = req.Term
		n.State = StateFollower
		n.VotedFor = ""
	}

	// Reset election timer since we heard from leader
	n.resetElectionTimer()

	// Check if we have the previous log entry
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex >= uint64(len(n.Log)) {
			resp.Success = false
			resp.ConflictIndex = uint64(len(n.Log))
			return resp
		}
		if n.Log[req.PrevLogIndex].Term != req.PrevLogTerm {
			resp.Success = false
			// Find conflict term
			resp.ConflictTerm = n.Log[req.PrevLogIndex].Term
			// Find first index with this term
			for i := req.PrevLogIndex; i > 0; i-- {
				if n.Log[i-1].Term != resp.ConflictTerm {
					resp.ConflictIndex = i
					break
				}
			}
			return resp
		}
	}

	// Append new entries
	for i, entry := range req.Entries {
		index := req.PrevLogIndex + 1 + uint64(i)
		if index < uint64(len(n.Log)) {
			// If entry exists with different term, delete it and all following
			if n.Log[index].Term != entry.Term {
				n.Log = n.Log[:index]
				n.Log = append(n.Log, entry)
			}
		} else {
			// Append new entry
			n.Log = append(n.Log, entry)
		}
	}

	// Update commit index
	if req.LeaderCommit > n.CommitIndex {
		lastNewIndex := req.PrevLogIndex + uint64(len(req.Entries))
		if req.LeaderCommit < lastNewIndex {
			n.CommitIndex = req.LeaderCommit
		} else {
			n.CommitIndex = lastNewIndex
		}
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

	// If leader's term is lower, reject
	if req.Term < n.CurrentTerm {
		resp.Success = false
		return resp
	}

	// If leader's term is higher, update our term and become follower
	if req.Term > n.CurrentTerm {
		n.CurrentTerm = req.Term
		n.State = StateFollower
		n.VotedFor = ""
	}

	// Reset election timer
	n.resetElectionTimer()

	// Apply snapshot to FSM
	if n.fsm != nil && len(req.Data) > 0 {
		if err := n.fsm.Restore(req.Data); err != nil {
			resp.Success = false
			return resp
		}
	}

	// Update log to reflect snapshot
	if req.LastIncludedIndex >= uint64(len(n.Log)) {
		n.Log = []LogEntry{{Index: req.LastIncludedIndex, Term: req.LastIncludedTerm}}
	} else {
		n.Log = n.Log[req.LastIncludedIndex+1:]
		n.Log = append([]LogEntry{{Index: req.LastIncludedIndex, Term: req.LastIncludedTerm}}, n.Log...)
	}

	n.LastApplied = req.LastIncludedIndex
	n.CommitIndex = req.LastIncludedIndex

	resp.Success = true
	return resp
}
