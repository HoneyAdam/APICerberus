package raft

// RequestVoteRequest is sent by candidates to gather votes.
type RequestVoteRequest struct {
	Term         uint64 `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex uint64 `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
}

// RequestVoteResponse is sent by nodes to candidates.
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

// AppendEntriesRequest is sent by leaders to replicate log entries.
type AppendEntriesRequest struct {
	Term         uint64     `json:"term"`
	LeaderID     string     `json:"leader_id"`
	PrevLogIndex uint64     `json:"prev_log_index"`
	PrevLogTerm  uint64     `json:"prev_log_term"`
	Entries      []LogEntry `json:"entries"`
	LeaderCommit uint64     `json:"leader_commit"`
}

// AppendEntriesResponse is sent by followers to leaders.
type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
	// For optimization - hint to leader about conflicting index
	ConflictIndex uint64 `json:"conflict_index,omitempty"`
	ConflictTerm  uint64 `json:"conflict_term,omitempty"`
}

// InstallSnapshotRequest is sent by leaders to transfer snapshots.
type InstallSnapshotRequest struct {
	Term              uint64 `json:"term"`
	LeaderID          string `json:"leader_id"`
	LastIncludedIndex uint64 `json:"last_included_index"`
	LastIncludedTerm  uint64 `json:"last_included_term"`
	Data              []byte `json:"data"`
	Done              bool   `json:"done"`
}

// InstallSnapshotResponse is sent by followers to leaders.
type InstallSnapshotResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

// ClusterStatus represents the status of the Raft cluster.
type ClusterStatus struct {
	NodeID        string            `json:"node_id"`
	State         string            `json:"state"`
	Term          uint64            `json:"term"`
	CommitIndex   uint64            `json:"commit_index"`
	LastApplied   uint64            `json:"last_applied"`
	LogSize       int               `json:"log_size"`
	Peers         map[string]string `json:"peers"`
	LeaderID      string            `json:"leader_id"`
	ElectionTimer string            `json:"election_timer"`
}

// NodeInfo represents information about a cluster node.
type NodeInfo struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	State     string `json:"state"`
	IsLeader  bool   `json:"is_leader"`
	IsHealthy bool   `json:"is_healthy"`
	LastSeen  int64  `json:"last_seen"`
}

// JoinRequest is sent by nodes wanting to join the cluster.
type JoinRequest struct {
	NodeID  string `json:"node_id"`
	Address string `json:"address"`
}

// JoinResponse is sent in response to a join request.
type JoinResponse struct {
	Success   bool              `json:"success"`
	Error     string            `json:"error,omitempty"`
	Peers     map[string]string `json:"peers"`
	LeaderID  string            `json:"leader_id,omitempty"`
	LeaderAddr string           `json:"leader_addr,omitempty"`
}

// LeaveRequest is sent by nodes leaving the cluster.
type LeaveRequest struct {
	NodeID string `json:"node_id"`
}

// LeaveResponse is sent in response to a leave request.
type LeaveResponse struct {
	Success bool `json:"success"`
}

// SnapshotRequest triggers a snapshot on the node.
type SnapshotRequest struct {
	NodeID string `json:"node_id"`
}

// SnapshotResponse is sent after snapshot completes.
type SnapshotResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Index     uint64 `json:"index"`
	Size      int64  `json:"size"`
}
