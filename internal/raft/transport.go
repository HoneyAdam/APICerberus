package raft

import (
	"bytes"
	"crypto/subtle"
	"crypto/tls"
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

	// TLS configuration for mTLS
	tlsConfig *tls.Config
	useTLS    bool

	// Shared secret for authenticating inter-node RPC calls.
	// When non-empty, all incoming RPC requests must present this
	// secret via the X-Raft-Token header.
	rpcSecret string
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

// SetRPCSecret sets a shared secret that all incoming RPC requests must present
// via the X-Raft-Token header. TLS must be enabled before calling this —
// the secret must never be transmitted over unencrypted connections.
// Returns an error if called when TLS is not active.
func (t *HTTPTransport) SetRPCSecret(secret string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if secret != "" && !t.useTLS {
		return fmt.Errorf("refusing to set RPC secret: TLS is not enabled; X-Raft-Token would be transmitted in cleartext")
	}
	t.rpcSecret = secret
	return nil
}

// SetTLSConfig configures TLS for mTLS communication.
// Call this before Start() to enable mTLS.
func (t *HTTPTransport) SetTLSConfig(config *tls.Config) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tlsConfig = config
	t.useTLS = config != nil
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

const maxRaftRPCBodySize = 10 << 20 // 10 MB — prevents excessive memory allocation on Raft RPC (CWE-770)

// Start starts the HTTP transport server.
func (t *HTTPTransport) Start(handler RPCHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.handler = handler

	mux := http.NewServeMux()
	rpcHandler := t.withRPCAuth
	mux.Handle("/raft/request-vote", http.MaxBytesHandler(rpcHandler(t.handleRequestVote), maxRaftRPCBodySize))
	mux.Handle("/raft/append-entries", http.MaxBytesHandler(rpcHandler(t.handleAppendEntries), maxRaftRPCBodySize))
	mux.Handle("/raft/install-snapshot", http.MaxBytesHandler(rpcHandler(t.handleInstallSnapshot), maxRaftRPCBodySize))

	t.server = &http.Server{
		Addr:         t.bindAddress,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    t.tlsConfig,
	}

	// Create client with TLS if configured
	if t.useTLS {
		t.client = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: t.tlsConfig,
			},
		}
	}

	go func() {
		if t.useTLS {
			if err := t.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Printf("[ERROR] Raft TLS server error: %v", err)
			}
		} else {
			if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[ERROR] Raft server error: %v", err)
			}
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

// withRPCAuth is a middleware that authenticates incoming Raft RPC requests.
// If rpcSecret is set, the request must present a matching X-Raft-Token header.
func (t *HTTPTransport) withRPCAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.mu.RLock()
		secret := t.rpcSecret
		t.mu.RUnlock()

		if secret != "" && !t.authenticateRPC(r, secret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// authenticateRPC checks if the request's X-Raft-Token matches the shared secret.
func (t *HTTPTransport) authenticateRPC(r *http.Request, secret string) bool {
	token := r.Header.Get("X-Raft-Token")
	if token == "" {
		return false
	}
	// Use constant-time comparison to prevent timing attacks
	return cryptoSubtleConstantTimeCompare([]byte(token), []byte(secret)) == 1
}

// cryptoSubtleConstantTimeCompare wraps crypto/subtle.ConstantTimeCompare
// to avoid importing crypto/subtle in this file directly.
func cryptoSubtleConstantTimeCompare(a, b []byte) int {
	return subtle.ConstantTimeCompare(a, b)
}
func (t *HTTPTransport) postRPC(nodeID, path string, req any) ([]byte, error) {
	t.mu.RLock()
	addr, ok := t.peers[nodeID]
	useTLS := t.useTLS
	secret := t.rpcSecret
	t.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown peer: %s", nodeID)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s%s", scheme, addr, path)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Only send Raft token over TLS to prevent token leakage in plaintext.
	if useTLS && secret != "" {
		httpReq.Header.Set("X-Raft-Token", secret)
	}

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
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		http.Error(w, "internal error", http.StatusInternalServerError)
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
		http.Error(w, "internal error", http.StatusInternalServerError)
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
