package raft

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockRPCHandler is a mock implementation of RPCHandler for testing
type mockRPCHandler struct {
	requestVoteCalled       bool
	appendEntriesCalled     bool
	installSnapshotCalled   bool
	requestVoteResponse     *RequestVoteResponse
	appendEntriesResponse   *AppendEntriesResponse
	installSnapshotResponse *InstallSnapshotResponse
}

func (m *mockRPCHandler) HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse {
	m.requestVoteCalled = true
	if m.requestVoteResponse != nil {
		return m.requestVoteResponse
	}
	return &RequestVoteResponse{Term: 1, VoteGranted: true}
}

func (m *mockRPCHandler) HandleAppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse {
	m.appendEntriesCalled = true
	if m.appendEntriesResponse != nil {
		return m.appendEntriesResponse
	}
	return &AppendEntriesResponse{Term: 1, Success: true}
}

func (m *mockRPCHandler) HandleInstallSnapshot(req *InstallSnapshotRequest) *InstallSnapshotResponse {
	m.installSnapshotCalled = true
	if m.installSnapshotResponse != nil {
		return m.installSnapshotResponse
	}
	return &InstallSnapshotResponse{Term: 1, Success: true}
}

func TestNewHTTPTransport(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	if transport == nil {
		t.Fatal("NewHTTPTransport() returned nil")
	}
	if transport.bindAddress != "127.0.0.1:12000" {
		t.Errorf("bindAddress = %v, want 127.0.0.1:12000", transport.bindAddress)
	}
	if transport.nodeID != "node-1" {
		t.Errorf("nodeID = %v, want node-1", transport.nodeID)
	}
	if transport.peers == nil {
		t.Error("peers map not initialized")
	}
	if transport.client == nil {
		t.Error("HTTP client not initialized")
	}
	if transport.client.Timeout != 5*time.Second {
		t.Errorf("client timeout = %v, want 5s", transport.client.Timeout)
	}
}

func TestHTTPTransport_SetPeer(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	transport.SetPeer("node-2", "127.0.0.1:12001")

	transport.mu.RLock()
	addr, ok := transport.peers["node-2"]
	transport.mu.RUnlock()

	if !ok {
		t.Error("Peer not found after SetPeer")
	}
	if addr != "127.0.0.1:12001" {
		t.Errorf("Peer address = %v, want 127.0.0.1:12001", addr)
	}
}

func TestHTTPTransport_RemovePeer(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	transport.SetPeer("node-2", "127.0.0.1:12001")

	transport.RemovePeer("node-2")

	transport.mu.RLock()
	_, ok := transport.peers["node-2"]
	transport.mu.RUnlock()

	if ok {
		t.Error("Peer still exists after RemovePeer")
	}
}

func TestHTTPTransport_LocalAddr(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	if addr := transport.LocalAddr(); addr != "127.0.0.1:12000" {
		t.Errorf("LocalAddr() = %v, want 127.0.0.1:12000", addr)
	}
}

func TestHTTPTransport_handleRequestVote(t *testing.T) {
	handler := &mockRPCHandler{}
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	transport.handler = handler

	req := RequestVoteRequest{
		Term:         1,
		CandidateID:  "node-2",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader(body))
	w := httptest.NewRecorder()

	transport.handleRequestVote(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
	if !handler.requestVoteCalled {
		t.Error("HandleRequestVote was not called")
	}

	var resp RequestVoteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if !resp.VoteGranted {
		t.Error("Expected VoteGranted to be true")
	}
}

func TestHTTPTransport_handleRequestVote_InvalidMethod(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodGet, "/raft/request-vote", nil)
	w := httptest.NewRecorder()

	transport.handleRequestVote(w, httpReq)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHTTPTransport_handleRequestVote_InvalidBody(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader([]byte("invalid")))
	w := httptest.NewRecorder()

	transport.handleRequestVote(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHTTPTransport_handleRequestVote_NoHandler(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	// handler is nil

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()

	transport.handleRequestVote(w, httpReq)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHTTPTransport_handleAppendEntries(t *testing.T) {
	handler := &mockRPCHandler{}
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	transport.handler = handler

	req := AppendEntriesRequest{
		Term:         1,
		LeaderID:     "node-1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{},
		LeaderCommit: 0,
	}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/append-entries", bytes.NewReader(body))
	w := httptest.NewRecorder()

	transport.handleAppendEntries(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
	if !handler.appendEntriesCalled {
		t.Error("HandleAppendEntries was not called")
	}

	var resp AppendEntriesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Error("Expected Success to be true")
	}
}

func TestHTTPTransport_handleAppendEntries_InvalidMethod(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodGet, "/raft/append-entries", nil)
	w := httptest.NewRecorder()

	transport.handleAppendEntries(w, httpReq)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHTTPTransport_handleInstallSnapshot(t *testing.T) {
	handler := &mockRPCHandler{}
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	transport.handler = handler

	req := InstallSnapshotRequest{
		Term:              1,
		LeaderID:          "node-1",
		LastIncludedIndex: 100,
		LastIncludedTerm:  1,
		Data:              []byte("snapshot data"),
		Done:              true,
	}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/install-snapshot", bytes.NewReader(body))
	w := httptest.NewRecorder()

	transport.handleInstallSnapshot(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
	if !handler.installSnapshotCalled {
		t.Error("HandleInstallSnapshot was not called")
	}
}

func TestHTTPTransport_postRPC_UnknownPeer(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	// Don't register node-2 as a peer

	req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
	_, err := transport.postRPC("node-2", "/raft/request-vote", req)

	if err == nil {
		t.Error("Expected error for unknown peer")
	}
}

func TestNewInmemTransport(t *testing.T) {
	transport := NewInmemTransport()
	if transport == nil {
		t.Fatal("NewInmemTransport() returned nil")
	}
	if transport.peers == nil {
		t.Error("peers map not initialized")
	}
}

func TestInmemTransport_StartStop(t *testing.T) {
	transport := NewInmemTransport()
	handler := &mockRPCHandler{}

	err := transport.Start(handler)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	if transport.handler != handler {
		t.Error("Handler not set correctly")
	}

	err = transport.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestInmemTransport_LocalAddr(t *testing.T) {
	transport := NewInmemTransport()
	if addr := transport.LocalAddr(); addr != "inmem" {
		t.Errorf("LocalAddr() = %v, want inmem", addr)
	}
}

func TestInmemTransport_Connect(t *testing.T) {
	t1 := NewInmemTransport()
	t2 := NewInmemTransport()

	t1.Connect("node-2", t2)

	t1.mu.RLock()
	peer, ok := t1.peers["node-2"]
	t1.mu.RUnlock()

	if !ok {
		t.Error("Peer not found after Connect")
	}
	if peer != t2 {
		t.Error("Connected peer is not t2")
	}
}

func TestInmemTransport_RequestVote(t *testing.T) {
	t1 := NewInmemTransport()
	t2 := NewInmemTransport()
	handler := &mockRPCHandler{
		requestVoteResponse: &RequestVoteResponse{Term: 2, VoteGranted: true},
	}

	t2.Start(handler)
	t1.Connect("node-2", t2)

	req := &RequestVoteRequest{Term: 2, CandidateID: "node-1"}
	resp, err := t1.RequestVote("node-2", req)

	if err != nil {
		t.Errorf("RequestVote() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RequestVote() returned nil response")
	}
	if !resp.VoteGranted {
		t.Error("Expected VoteGranted to be true")
	}
	if resp.Term != 2 {
		t.Errorf("Term = %d, want 2", resp.Term)
	}
}

func TestInmemTransport_RequestVote_PeerNotFound(t *testing.T) {
	t1 := NewInmemTransport()
	// Don't connect to any peer

	req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
	_, err := t1.RequestVote("node-2", req)

	if err == nil {
		t.Error("Expected error for non-existent peer")
	}
}

func TestInmemTransport_RequestVote_NoHandler(t *testing.T) {
	t1 := NewInmemTransport()
	t2 := NewInmemTransport()
	// Don't start handler on t2

	t1.Connect("node-2", t2)

	req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
	_, err := t1.RequestVote("node-2", req)

	if err == nil {
		t.Error("Expected error when peer handler not ready")
	}
}

func TestInmemTransport_AppendEntries(t *testing.T) {
	t1 := NewInmemTransport()
	t2 := NewInmemTransport()
	handler := &mockRPCHandler{
		appendEntriesResponse: &AppendEntriesResponse{Term: 1, Success: true},
	}

	t2.Start(handler)
	t1.Connect("node-2", t2)

	req := &AppendEntriesRequest{Term: 1, LeaderID: "node-1"}
	resp, err := t1.AppendEntries("node-2", req)

	if err != nil {
		t.Errorf("AppendEntries() error = %v", err)
	}
	if resp == nil {
		t.Fatal("AppendEntries() returned nil response")
	}
	if !resp.Success {
		t.Error("Expected Success to be true")
	}
}

func TestInmemTransport_AppendEntries_PeerNotFound(t *testing.T) {
	t1 := NewInmemTransport()

	req := &AppendEntriesRequest{Term: 1, LeaderID: "node-1"}
	_, err := t1.AppendEntries("node-2", req)

	if err == nil {
		t.Error("Expected error for non-existent peer")
	}
}

func TestInmemTransport_InstallSnapshot(t *testing.T) {
	t1 := NewInmemTransport()
	t2 := NewInmemTransport()
	handler := &mockRPCHandler{
		installSnapshotResponse: &InstallSnapshotResponse{Term: 1, Success: true},
	}

	t2.Start(handler)
	t1.Connect("node-2", t2)

	req := &InstallSnapshotRequest{Term: 1, LeaderID: "node-1", Data: []byte("snapshot")}
	resp, err := t1.InstallSnapshot("node-2", req)

	if err != nil {
		t.Errorf("InstallSnapshot() error = %v", err)
	}
	if resp == nil {
		t.Fatal("InstallSnapshot() returned nil response")
	}
	if !resp.Success {
		t.Error("Expected Success to be true")
	}
}

func TestInmemTransport_InstallSnapshot_PeerNotFound(t *testing.T) {
	t1 := NewInmemTransport()

	req := &InstallSnapshotRequest{Term: 1, LeaderID: "node-1", Data: []byte("snapshot")}
	_, err := t1.InstallSnapshot("node-2", req)

	if err == nil {
		t.Error("Expected error for non-existent peer")
	}
}

// TestHTTPTransport_StartStop tests starting and stopping the HTTP server
func TestHTTPTransport_StartStop(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:0", "node-1") // :0 for random port
	handler := &mockRPCHandler{}

	err := transport.Start(handler)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give the server a moment to start
	time.Sleep(10 * time.Millisecond)

	// Verify server is running
	if transport.server == nil {
		t.Error("Server not initialized after Start")
	}

	err = transport.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

// TestHTTPTransport_Integration tests the full HTTP transport flow with an actual server
func TestHTTPTransport_Integration(t *testing.T) {
	// Create a test HTTP server
	handler := &mockRPCHandler{
		requestVoteResponse: &RequestVoteResponse{Term: 5, VoteGranted: true},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/raft/request-vote", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req RequestVoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := handler.HandleRequestVote(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Create transport and point it to the test server
	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", server.Listener.Addr().String())

	// Send request
	req := &RequestVoteRequest{Term: 5, CandidateID: "node-1"}
	resp, err := transport.RequestVote("node-2", req)

	if err != nil {
		t.Errorf("RequestVote() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RequestVote() returned nil response")
	}
	if !resp.VoteGranted {
		t.Error("Expected VoteGranted to be true")
	}
	if resp.Term != 5 {
		t.Errorf("Term = %d, want 5", resp.Term)
	}
}

// TestHTTPTransport_RPCErrorStatus tests handling of non-200 status codes
func TestHTTPTransport_RPCErrorStatus(t *testing.T) {
	// Create a test server that returns errors
	mux := http.NewServeMux()
	mux.HandleFunc("/raft/request-vote", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", server.Listener.Addr().String())

	req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
	_, err := transport.RequestVote("node-2", req)

	if err == nil {
		t.Error("Expected error for non-200 status")
	}
}

// TestHTTPTransport_InvalidResponse tests handling of invalid JSON responses
func TestHTTPTransport_InvalidResponse(t *testing.T) {
	// Create a test server that returns invalid JSON
	mux := http.NewServeMux()
	mux.HandleFunc("/raft/request-vote", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", server.Listener.Addr().String())

	req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
	_, err := transport.RequestVote("node-2", req)

	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}
}

// TestHTTPTransport_NetworkError tests handling of network errors
func TestHTTPTransport_NetworkError(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", "127.0.0.1:1") // Port 1 is unlikely to be open

	// Use a very short timeout to fail faster
	transport.client.Timeout = 100 * time.Millisecond

	req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
	_, err := transport.RequestVote("node-2", req)

	if err == nil {
		t.Error("Expected error for network failure")
	}
}

// TestHTTPTransport_AppendEntries_Integration tests AppendEntries via HTTP
func TestHTTPTransport_AppendEntries_Integration(t *testing.T) {
	handler := &mockRPCHandler{
		appendEntriesResponse: &AppendEntriesResponse{Term: 3, Success: true},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/raft/append-entries", func(w http.ResponseWriter, r *http.Request) {
		var req AppendEntriesRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp := handler.HandleAppendEntries(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", server.Listener.Addr().String())

	req := &AppendEntriesRequest{Term: 3, LeaderID: "node-1", Entries: []LogEntry{{Index: 1, Term: 1}}}
	resp, err := transport.AppendEntries("node-2", req)

	if err != nil {
		t.Errorf("AppendEntries() error = %v", err)
	}
	if resp == nil {
		t.Fatal("AppendEntries() returned nil response")
	}
	if resp.Term != 3 {
		t.Errorf("Term = %d, want 3", resp.Term)
	}
}

// TestHTTPTransport_InstallSnapshot_Integration tests InstallSnapshot via HTTP
func TestHTTPTransport_InstallSnapshot_Integration(t *testing.T) {
	handler := &mockRPCHandler{
		installSnapshotResponse: &InstallSnapshotResponse{Term: 2, Success: true},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/raft/install-snapshot", func(w http.ResponseWriter, r *http.Request) {
		var req InstallSnapshotRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp := handler.HandleInstallSnapshot(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", server.Listener.Addr().String())

	req := &InstallSnapshotRequest{Term: 2, LeaderID: "node-1", Data: []byte("test snapshot data"), LastIncludedIndex: 100}
	resp, err := transport.InstallSnapshot("node-2", req)

	if err != nil {
		t.Errorf("InstallSnapshot() error = %v", err)
	}
	if resp == nil {
		t.Fatal("InstallSnapshot() returned nil response")
	}
	if resp.Term != 2 {
		t.Errorf("Term = %d, want 2", resp.Term)
	}
}

// TestHTTPTransport_handleAppendEntries_InvalidBody tests error handling
func TestHTTPTransport_handleAppendEntries_InvalidBody(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/append-entries", bytes.NewReader([]byte("invalid")))
	w := httptest.NewRecorder()

	transport.handleAppendEntries(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestHTTPTransport_handleAppendEntries_NoHandler tests error handling
func TestHTTPTransport_handleAppendEntries_NoHandler(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/append-entries", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()

	transport.handleAppendEntries(w, httpReq)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// TestHTTPTransport_handleInstallSnapshot_InvalidMethod tests method validation
func TestHTTPTransport_handleInstallSnapshot_InvalidMethod(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodGet, "/raft/install-snapshot", nil)
	w := httptest.NewRecorder()

	transport.handleInstallSnapshot(w, httpReq)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestHTTPTransport_handleInstallSnapshot_InvalidBody tests error handling
func TestHTTPTransport_handleInstallSnapshot_InvalidBody(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/install-snapshot", bytes.NewReader([]byte("invalid")))
	w := httptest.NewRecorder()

	transport.handleInstallSnapshot(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestHTTPTransport_handleInstallSnapshot_NoHandler tests error handling
func TestHTTPTransport_handleInstallSnapshot_NoHandler(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/raft/install-snapshot", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()

	transport.handleInstallSnapshot(w, httpReq)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// TestInmemTransport_ConcurrentAccess tests thread safety
func TestInmemTransport_ConcurrentAccess(t *testing.T) {
	t1 := NewInmemTransport()
	t2 := NewInmemTransport()
	handler := &mockRPCHandler{
		requestVoteResponse: &RequestVoteResponse{Term: 1, VoteGranted: true},
	}

	t2.Start(handler)
	t1.Connect("node-2", t2)

	// Concurrent requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := &RequestVoteRequest{Term: 1, CandidateID: "node-1"}
			_, err := t1.RequestVote("node-2", req)
			if err != nil {
				t.Errorf("Concurrent RequestVote error: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestHTTPTransport_Start_WithHandler verifies the handler is properly set
func TestHTTPTransport_Start_WithHandler(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	handler := &mockRPCHandler{}

	err := transport.Start(handler)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer transport.Stop()

	if transport.handler != handler {
		t.Error("Handler not set correctly in transport")
	}
}

// TestHTTPTransport_Stop_NoServer tests stopping when server is nil
func TestHTTPTransport_Stop_NoServer(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	// Don't start the server

	err := transport.Stop()
	if err != nil {
		t.Errorf("Stop() should return nil when server not started, got %v", err)
	}
}

// TestHTTPTransport_postRPC_MarshalError tests error when marshaling request
func TestHTTPTransport_postRPC_MarshalError(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", "127.0.0.1:12001")

	// Try to marshal a channel (which can't be marshaled)
	badReq := make(chan int)
	_, err := transport.postRPC("node-2", "/raft/request-vote", badReq)

	if err == nil {
		t.Error("Expected error when marshaling invalid type")
	}
}

// TestHTTPTransport_postRPC_NewRequestError tests error creating HTTP request
// This is hard to trigger without mocking, but we can test the success path
func TestHTTPTransport_postRPC_SuccessPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/raft/test", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write(body) // Echo back
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	transport := NewHTTPTransport("127.0.0.1:0", "node-1")
	transport.SetPeer("node-2", server.Listener.Addr().String())

	req := map[string]string{"test": "data"}
	resp, err := transport.postRPC("node-2", "/raft/test", req)

	if err != nil {
		t.Errorf("postRPC() error = %v", err)
	}
	if resp == nil {
		t.Fatal("postRPC() returned nil response")
	}

	var result map[string]string
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if result["test"] != "data" {
		t.Errorf("Response data = %v, want data", result["test"])
	}
}
