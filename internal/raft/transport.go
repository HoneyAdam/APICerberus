package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// HTTPTransport implements Transport using HTTP.
type HTTPTransport struct {
	bindAddress string
	nodeID      string
	client      *http.Client
	server      *http.Server
	handler     RPCHandler
	peers       map[string]string // nodeID -> address
	mu          sync.RWMutex
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(bindAddress, nodeID string) *HTTPTransport {
	return &HTTPTransport{
		bindAddress: bindAddress,
		nodeID:      nodeID,
		peers:       make(map[string]string),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SetPeer registers a peer's address for RPC routing.
func (t *HTTPTransport) SetPeer(nodeID, address string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[nodeID] = address
}

// RemovePeer removes a peer's address.
func (t *HTTPTransport) RemovePeer(nodeID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, nodeID)
}

// Start starts the HTTP transport server.
func (t *HTTPTransport) Start(handler RPCHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc("/raft/request-vote", t.handleRequestVote)
	mux.HandleFunc("/raft/append-entries", t.handleAppendEntries)
	mux.HandleFunc("/raft/install-snapshot", t.handleInstallSnapshot)

	t.server = &http.Server{
		Addr:         t.bindAddress,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error
		}
	}()

	return nil
}

// Stop stops the HTTP transport server.
func (t *HTTPTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.server != nil {
		return t.server.Close()
	}
	return nil
}

// LocalAddr returns the local address.
func (t *HTTPTransport) LocalAddr() string {
	return t.bindAddress
}

// RequestVote sends a RequestVote RPC to a peer.
func (t *HTTPTransport) RequestVote(nodeID string, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	resp, err := t.postRPC(nodeID, "/raft/request-vote", req)
	if err != nil {
		return nil, err
	}

	var response RequestVoteResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// AppendEntries sends an AppendEntries RPC to a peer.
func (t *HTTPTransport) AppendEntries(nodeID string, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	resp, err := t.postRPC(nodeID, "/raft/append-entries", req)
	if err != nil {
		return nil, err
	}

	var response AppendEntriesResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// InstallSnapshot sends an InstallSnapshot RPC to a peer.
func (t *HTTPTransport) InstallSnapshot(nodeID string, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error) {
	resp, err := t.postRPC(nodeID, "/raft/install-snapshot", req)
	if err != nil {
		return nil, err
	}

	var response InstallSnapshotResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// postRPC sends an RPC request to a peer.
func (t *HTTPTransport) postRPC(nodeID, path string, req any) ([]byte, error) {
	t.mu.RLock()
	addr, ok := t.peers[nodeID]
	t.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown peer: %s", nodeID)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s%s", addr, path)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC failed with status %d", httpResp.StatusCode)
	}

	return io.ReadAll(httpResp.Body)
}

// handleRequestVote handles incoming RequestVote RPCs.
func (t *HTTPTransport) handleRequestVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RequestVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()

	if handler == nil {
		http.Error(w, "Handler not ready", http.StatusServiceUnavailable)
		return
	}

	resp := handler.HandleRequestVote(&req)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[ERROR] raft: failed to encode response: %v", err)
	}
}

// handleAppendEntries handles incoming AppendEntries RPCs.
func (t *HTTPTransport) handleAppendEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AppendEntriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()

	if handler == nil {
		http.Error(w, "Handler not ready", http.StatusServiceUnavailable)
		return
	}

	resp := handler.HandleAppendEntries(&req)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[ERROR] raft: failed to encode response: %v", err)
	}
}

// handleInstallSnapshot handles incoming InstallSnapshot RPCs.
func (t *HTTPTransport) handleInstallSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req InstallSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()

	if handler == nil {
		http.Error(w, "Handler not ready", http.StatusServiceUnavailable)
		return
	}

	resp := handler.HandleInstallSnapshot(&req)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[ERROR] raft: failed to encode response: %v", err)
	}
}

// InmemTransport implements an in-memory transport for testing.
type InmemTransport struct {
	handler RPCHandler
	peers   map[string]*InmemTransport
	mu      sync.RWMutex
}

// NewInmemTransport creates a new in-memory transport.
func NewInmemTransport() *InmemTransport {
	return &InmemTransport{
		peers: make(map[string]*InmemTransport),
	}
}

// Start starts the in-memory transport.
func (t *InmemTransport) Start(handler RPCHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
	return nil
}

// Stop stops the in-memory transport.
func (t *InmemTransport) Stop() error {
	return nil
}

// LocalAddr returns the local address.
func (t *InmemTransport) LocalAddr() string {
	return "inmem"
}

// Connect connects to another in-memory transport.
func (t *InmemTransport) Connect(id string, other *InmemTransport) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[id] = other
}

// RequestVote sends a RequestVote RPC.
func (t *InmemTransport) RequestVote(nodeID string, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	t.mu.RLock()
	peer := t.peers[nodeID]
	t.mu.RUnlock()

	if peer == nil {
		return nil, fmt.Errorf("peer not found: %s", nodeID)
	}

	peer.mu.RLock()
	handler := peer.handler
	peer.mu.RUnlock()

	if handler == nil {
		return nil, fmt.Errorf("peer handler not ready: %s", nodeID)
	}

	return handler.HandleRequestVote(req), nil
}

// AppendEntries sends an AppendEntries RPC.
func (t *InmemTransport) AppendEntries(nodeID string, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	t.mu.RLock()
	peer := t.peers[nodeID]
	t.mu.RUnlock()

	if peer == nil {
		return nil, fmt.Errorf("peer not found: %s", nodeID)
	}

	peer.mu.RLock()
	handler := peer.handler
	peer.mu.RUnlock()

	if handler == nil {
		return nil, fmt.Errorf("peer handler not ready: %s", nodeID)
	}

	return handler.HandleAppendEntries(req), nil
}

// InstallSnapshot sends an InstallSnapshot RPC.
func (t *InmemTransport) InstallSnapshot(nodeID string, req *InstallSnapshotRequest) (*InstallSnapshotResponse, error) {
	t.mu.RLock()
	peer := t.peers[nodeID]
	t.mu.RUnlock()

	if peer == nil {
		return nil, fmt.Errorf("peer not found: %s", nodeID)
	}

	peer.mu.RLock()
	handler := peer.handler
	peer.mu.RUnlock()

	if handler == nil {
		return nil, fmt.Errorf("peer handler not ready: %s", nodeID)
	}

	return handler.HandleInstallSnapshot(req), nil
}
