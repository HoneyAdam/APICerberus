package raft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ClusterManager manages the Raft cluster.
type ClusterManager struct {
	node    *Node
	fsm     *GatewayFSM
	apiAddr string
	apiKey  string
	server  *http.Server
	mu      sync.RWMutex

	// Node health tracking
	nodeHealth map[string]*NodeHealth
}

// NodeHealth tracks the health of a cluster node.
type NodeHealth struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LastSeen  time.Time `json:"last_seen"`
	Healthy   bool      `json:"healthy"`
	FailCount int       `json:"fail_count"`
}

// NewClusterManager creates a new cluster manager.
func NewClusterManager(node *Node, fsm *GatewayFSM, apiAddr, apiKey string) *ClusterManager {
	return &ClusterManager{
		node:       node,
		fsm:        fsm,
		apiAddr:    apiAddr,
		apiKey:     apiKey,
		nodeHealth: make(map[string]*NodeHealth),
	}
}

// Start starts the cluster manager API.
func (cm *ClusterManager) Start() error {
	mux := http.NewServeMux()

	// Cluster status endpoints
	mux.HandleFunc("/admin/api/v1/cluster/status", cm.handleClusterStatus)
	mux.HandleFunc("/admin/api/v1/cluster/nodes", cm.handleNodes)
	mux.HandleFunc("/admin/api/v1/cluster/join", cm.handleJoin)
	mux.HandleFunc("/admin/api/v1/cluster/leave", cm.handleLeave)
	mux.HandleFunc("/admin/api/v1/cluster/snapshot", cm.handleSnapshot)

	// Raft state endpoints
	mux.HandleFunc("/admin/api/v1/raft/state", cm.handleRaftState)
	mux.HandleFunc("/admin/api/v1/raft/stats", cm.handleRaftStats)

	cm.server = &http.Server{
		Addr:         cm.apiAddr,
		Handler:      cm.authMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start health check monitoring
	go cm.monitorClusterHealth()

	go func() {
		if err := cm.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error
		}
	}()

	return nil
}

// Stop stops the cluster manager.
func (cm *ClusterManager) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return cm.server.Shutdown(ctx)
}

// authMiddleware adds authentication to handlers.
func (cm *ClusterManager) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("Authorization")
		if apiKey != "Bearer "+cm.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleClusterStatus returns the cluster status.
func (cm *ClusterManager) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	status := ClusterStatus{
		NodeID:        cm.node.ID,
		State:         cm.node.State.String(),
		Term:          cm.node.CurrentTerm,
		CommitIndex:   cm.node.CommitIndex,
		LastApplied:   cm.node.LastApplied,
		LogSize:       len(cm.node.Log),
		Peers:         cm.node.Peers,
		LeaderID:      cm.node.GetLeaderID(),
		ElectionTimer: "active",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status) // #nosec G104
}

// handleNodes handles node management.
func (cm *ClusterManager) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cm.listNodes(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listNodes returns all nodes in the cluster.
func (cm *ClusterManager) listNodes(w http.ResponseWriter, r *http.Request) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	nodes := make([]NodeInfo, 0)

	// Add self
	nodes = append(nodes, NodeInfo{
		ID:        cm.node.ID,
		Address:   cm.node.Address,
		State:     cm.node.State.String(),
		IsLeader:  cm.node.State == StateLeader,
		IsHealthy: true,
		LastSeen:  time.Now().Unix(),
	})

	// Add peers
	for id, addr := range cm.node.Peers {
		health, ok := cm.nodeHealth[id]
		if !ok {
			health = &NodeHealth{
				ID:       id,
				Address:  addr,
				Healthy:  false,
				LastSeen: time.Time{},
			}
		}

		nodes = append(nodes, NodeInfo{
			ID:        id,
			Address:   addr,
			State:     "Unknown", // Would be updated via gossip
			IsLeader:  false,
			IsHealthy: health.Healthy,
			LastSeen:  health.LastSeen.Unix(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(nodes) // #nosec G104
}

// handleJoin handles node join requests.
func (cm *ClusterManager) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Only leader can add peers
	if !cm.node.IsLeader() {
		leaderID := cm.node.GetLeaderID()
		leaderAddr := ""
		if addr, ok := cm.node.Peers[leaderID]; ok {
			leaderAddr = addr
		}

		resp := JoinResponse{
			Success:    false,
			Error:      "not leader",
			LeaderID:   leaderID,
			LeaderAddr: leaderAddr,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(resp) // #nosec G104
		return
	}

	// Add peer to cluster
	cm.node.AddPeer(req.NodeID, req.Address)

	// Return current peers
	cm.mu.RLock()
	peers := make(map[string]string)
	for id, addr := range cm.node.Peers {
		peers[id] = addr
	}
	cm.mu.RUnlock()

	resp := JoinResponse{
		Success:  true,
		Peers:    peers,
		LeaderID: cm.node.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp) // #nosec G104
}

// handleLeave handles node leave requests.
func (cm *ClusterManager) handleLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Only leader can remove peers
	if !cm.node.IsLeader() {
		resp := LeaveResponse{Success: false}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(resp) // #nosec G104
		return
	}

	cm.node.RemovePeer(req.NodeID)

	resp := LeaveResponse{Success: true}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp) // #nosec G104
}

// handleSnapshot handles snapshot requests.
func (cm *ClusterManager) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot, err := cm.fsm.Snapshot()
	if err != nil {
		resp := SnapshotResponse{
			Success: false,
			Error:   err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(resp) // #nosec G104
		return
	}

	resp := SnapshotResponse{
		Success: true,
		Index:   cm.node.LastApplied,
		Size:    int64(len(snapshot)),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp) // #nosec G104
}

// handleRaftState returns the current Raft state.
func (cm *ClusterManager) handleRaftState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := map[string]any{
		"node_id":        cm.node.ID,
		"state":          cm.node.State.String(),
		"term":           cm.node.CurrentTerm,
		"commit_index":   cm.node.CommitIndex,
		"last_applied":   cm.node.LastApplied,
		"last_log_index": cm.node.lastLogIndex(),
		"last_log_term":  cm.node.lastLogTerm(),
		"is_leader":      cm.node.IsLeader(),
		"leader_id":      cm.node.GetLeaderID(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state) // #nosec G104
}

// handleRaftStats returns Raft statistics.
func (cm *ClusterManager) handleRaftStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get FSM stats
	fsmStats := cm.fsm.GetClusterStatus()

	stats := map[string]any{
		"raft": map[string]any{
			"log_size":     len(cm.node.Log),
			"commit_index": cm.node.CommitIndex,
			"last_applied": cm.node.LastApplied,
			"current_term": cm.node.CurrentTerm,
			"state":        cm.node.State.String(),
			"peer_count":   len(cm.node.Peers),
		},
		"fsm": fsmStats,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats) // #nosec G104
}

// monitorClusterHealth monitors the health of cluster nodes.
func (cm *ClusterManager) monitorClusterHealth() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.checkNodeHealth()
		}
	}
}

// checkNodeHealth checks the health of all nodes.
func (cm *ClusterManager) checkNodeHealth() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for id, addr := range cm.node.Peers {
		if id == cm.node.ID {
			continue
		}

		health, ok := cm.nodeHealth[id]
		if !ok {
			health = &NodeHealth{
				ID:      id,
				Address: addr,
			}
			cm.nodeHealth[id] = health
		}

		// Health check via cluster status endpoint
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s/admin/api/v1/cluster/status", addr))

		if err != nil || resp.StatusCode != http.StatusOK {
			health.FailCount++
			if health.FailCount >= 3 {
				health.Healthy = false
			}
		} else {
			health.FailCount = 0
			health.Healthy = true
			health.LastSeen = time.Now()
			if resp != nil {
				_ = resp.Body.Close() // #nosec G104
			}
		}
	}
}

// Propose proposes a command to be applied to the FSM via Raft consensus.
func (cm *ClusterManager) Propose(cmd FSMCommand) error {
	if !cm.node.IsLeader() {
		return fmt.Errorf("not leader")
	}

	// Append entry to leader's log
	index, err := cm.node.AppendEntry(cmd)
	if err != nil {
		return err
	}

	// Wait for the entry to be committed (replicated to majority)
	if err := cm.node.WaitForCommit(index, 5*time.Second); err != nil {
		return fmt.Errorf("proposal failed: %w", err)
	}

	return nil
}
