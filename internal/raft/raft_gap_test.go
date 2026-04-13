package raft

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHTTPTransport_SetRPCSecret tests SetRPCSecret and withRPCAuth middleware
func TestHTTPTransport_SetRPCSecret(t *testing.T) {
	t.Run("set and use secret", func(t *testing.T) {
		transport := NewHTTPTransport("127.0.0.1:0", "node-1")
		// Enable TLS before setting RPC secret (required by H6 fix)
		transport.SetTLSConfig(&tls.Config{})
		if err := transport.SetRPCSecret("my-secret"); err != nil {
			t.Fatalf("SetRPCSecret failed: %v", err)
		}

		handler := &mockRPCHandler{
			requestVoteResponse: &RequestVoteResponse{Term: 1, VoteGranted: true},
		}
		transport.handler = handler

		// Build the handler chain the way Start() does it
		mux := http.NewServeMux()
		mux.Handle("/raft/request-vote", transport.withRPCAuth(transport.handleRequestVote))

		req := RequestVoteRequest{Term: 1, CandidateID: "node-1"}
		body, _ := json.Marshal(req)

		// Missing token should get 401
		httpReq := httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httpReq)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("missing token: status = %d, want %d", w.Code, http.StatusUnauthorized)
		}

		// Invalid token should get 401
		httpReq = httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader(body))
		httpReq.Header.Set("X-Raft-Token", "wrong")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httpReq)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("invalid token: status = %d, want %d", w.Code, http.StatusUnauthorized)
		}

		// Correct token should get 200
		httpReq = httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader(body))
		httpReq.Header.Set("X-Raft-Token", "my-secret")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httpReq)
		if w.Code != http.StatusOK {
			t.Errorf("correct token: status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestHTTPTransport_SetTLSConfig(t *testing.T) {
	t.Run("set nil tls config", func(t *testing.T) {
		transport := NewHTTPTransport("127.0.0.1:0", "node-1")
		transport.SetTLSConfig(nil)
		if transport.useTLS {
			t.Error("useTLS should be false when nil config is set")
		}
	})
}

func TestHTTPTransport_withRPCAuth_NoSecret(t *testing.T) {
	t.Run("no secret allows requests through", func(t *testing.T) {
		transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
		handler := &mockRPCHandler{
			requestVoteResponse: &RequestVoteResponse{Term: 1, VoteGranted: true},
		}
		transport.handler = handler

		req := RequestVoteRequest{Term: 1, CandidateID: "node-1"}
		body, _ := json.Marshal(req)

		// No token needed when secret is empty
		httpReq := httptest.NewRequest(http.MethodPost, "/raft/request-vote", bytes.NewReader(body))
		w := httptest.NewRecorder()
		transport.handleRequestVote(w, httpReq)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestHTTPTransport_RemovePeer_NonExistent(t *testing.T) {
	transport := NewHTTPTransport("127.0.0.1:12000", "node-1")
	// Remove a peer that was never set — should not panic
	transport.RemovePeer("non-existent")
}

// TestMultiRegionManager_Start_Stop tests Start/Stop with disabled config
func TestMultiRegionManager_Start_Stop(t *testing.T) {
	t.Run("start with disabled config returns nil", func(t *testing.T) {
		cfg := &MultiRegionConfig{
			Enabled: false,
		}
		mgr := &MultiRegionManager{
			config: cfg,
		}
		err := mgr.Start()
		if err != nil {
			t.Errorf("Start() error = %v, want nil", err)
		}
	})

	t.Run("stop with nil healthCancel", func(t *testing.T) {
		cfg := &MultiRegionConfig{
			Enabled: false,
		}
		mgr := &MultiRegionManager{
			config: cfg,
		}
		// Should not panic
		mgr.Stop()
	})

	t.Run("start and stop with enabled config", func(t *testing.T) {
		cfg := &MultiRegionConfig{
			Enabled:  true,
			RegionID: "us-east-1",
			Regions: []Region{
				{ID: "us-east-1", Name: "US East", Nodes: []string{"node-1"}},
			},
			LeaderPreference:  "priority",
			ReplicationMode:   "async",
			WANTimeoutFactor:  2.0,
			MaxCrossRegionLag: 30 * time.Second,
			RegionWeights:     make(map[string]int),
		}
		node := &Node{
			State: StateFollower,
			Peers: make(map[string]string),
		}
		mgr, err := NewMultiRegionManager(cfg, node)
		if err != nil {
			t.Fatalf("NewMultiRegionManager error = %v", err)
		}
		err = mgr.Start()
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		// Give goroutines a moment to start
		time.Sleep(10 * time.Millisecond)
		mgr.Stop()
	})
}

func TestMultiRegionManager_IsEnabled_NilConfig(t *testing.T) {
	mgr := &MultiRegionManager{config: nil}
	got := mgr.IsEnabled()
	if got {
		t.Error("IsEnabled() should return false for nil config")
	}
}

func TestMultiRegionManager_GetLocalRegion_NilConfig(t *testing.T) {
	mgr := &MultiRegionManager{config: nil}
	got := mgr.GetLocalRegion()
	if got != "" {
		t.Errorf("GetLocalRegion() = %q, want empty string", got)
	}
}

func TestMultiRegionManager_RecordLatency_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: false, RegionID: "us-east-1"}
	mgr := &MultiRegionManager{config: cfg}
	// Should not panic when disabled
	mgr.RecordLatency("node-1", 50*time.Millisecond)
}

func TestMultiRegionManager_GetReplicationTimeout_CrossRegion(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:          true,
		RegionID:         "us-east-1",
		Regions:          []Region{{ID: "us-east-1", Name: "US East", Nodes: []string{"node-1"}}, {ID: "eu-west-1", Name: "EU West", Nodes: []string{"node-2"}}},
		WANTimeoutFactor: 2.0,
	}
	node := &Node{State: StateFollower, Peers: make(map[string]string)}
	mgr, err := NewMultiRegionManager(cfg, node)
	if err != nil {
		t.Fatalf("NewMultiRegionManager error = %v", err)
	}
	// Cross-region node should get extended timeout
	timeout := mgr.GetReplicationTimeout("node-2", 5*time.Second)
	if timeout <= 5*time.Second {
		t.Errorf("cross-region timeout = %v, want > 5s", timeout)
	}
}

func TestMultiRegionManager_GetReplicationTimeout_Local(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:          true,
		RegionID:         "us-east-1",
		Regions:          []Region{{ID: "us-east-1", Name: "US East", Nodes: []string{"node-1"}}},
		WANTimeoutFactor: 2.0,
	}
	node := &Node{State: StateFollower, Peers: make(map[string]string)}
	mgr, err := NewMultiRegionManager(cfg, node)
	if err != nil {
		t.Fatalf("NewMultiRegionManager error = %v", err)
	}
	// Local node should get base timeout
	timeout := mgr.GetReplicationTimeout("node-1", 5*time.Second)
	if timeout != 5*time.Second {
		t.Errorf("local timeout = %v, want 5s", timeout)
	}
}

func TestMultiRegionManager_UpdateReplicationStatus_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: false}
	mgr := &MultiRegionManager{config: cfg}
	// Should not panic
	mgr.UpdateReplicationStatus("eu-west-1", 100)
}

func TestMultiRegionManager_GetRegionReplicationStatus_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: false}
	mgr := &MultiRegionManager{config: cfg}
	got := mgr.GetRegionReplicationStatus()
	if got != nil {
		t.Error("GetRegionReplicationStatus() should return nil when disabled")
	}
}

func TestMultiRegionManager_ShouldReplicateToRegion_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: false}
	mgr := &MultiRegionManager{config: cfg}
	got := mgr.ShouldReplicateToRegion("eu-west-1")
	if !got {
		t.Error("ShouldReplicateToRegion() should return true when disabled")
	}
}

func TestMultiRegionManager_ShouldReplicateToRegion_Unreachable(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: true, RegionID: "us-east-1"}
	mgr := &MultiRegionManager{
		config: cfg,
		regionReplicationStatus: map[string]*RegionReplicationStatus{
			"eu-west-1": {RegionID: "eu-west-1", Status: RegionStatusUnreachable},
		},
	}
	got := mgr.ShouldReplicateToRegion("eu-west-1")
	if got {
		t.Error("ShouldReplicateToRegion() should return false for unreachable region")
	}
}

func TestMultiRegionManager_GetSortedPeersByPriority_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: false}
	mgr := &MultiRegionManager{config: cfg}
	peers := []string{"node-1", "node-2"}
	got := mgr.GetSortedPeersByPriority(peers)
	if len(got) != 2 {
		t.Errorf("got %d peers, want 2", len(got))
	}
}

func TestMultiRegionManager_GetRegionAwareTimeout_Disabled(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: false}
	mgr := &MultiRegionManager{config: cfg}
	got := mgr.GetRegionAwareTimeout("node-1", 5*time.Second)
	if got != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", got)
	}
}

func TestMultiRegionManager_GetRegionAwareTimeout_CrossRegion(t *testing.T) {
	cfg := &MultiRegionConfig{
		Enabled:          true,
		RegionID:         "us-east-1",
		Regions:          []Region{{ID: "us-east-1", Name: "US East", Nodes: []string{"node-1"}}, {ID: "eu-west-1", Name: "EU West", Nodes: []string{"node-2"}}},
		WANTimeoutFactor: 2.0,
	}
	node := &Node{State: StateFollower, Peers: make(map[string]string)}
	mgr, err := NewMultiRegionManager(cfg, node)
	if err != nil {
		t.Fatalf("NewMultiRegionManager error = %v", err)
	}
	got := mgr.GetRegionAwareTimeout("node-2", 5*time.Second)
	if got <= 5*time.Second {
		t.Errorf("cross-region timeout = %v, want > 5s", got)
	}
}

func TestMultiRegionManager_CheckRegionHealth_NilNode(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: true, RegionID: "us-east-1"}
	mgr := &MultiRegionManager{config: cfg}
	// checkRegionHealth returns early when node is nil
	mgr.checkRegionHealth()
}

func TestMultiRegionManager_MeasureLatencies_NilNode(t *testing.T) {
	cfg := &MultiRegionConfig{Enabled: true, RegionID: "us-east-1"}
	mgr := &MultiRegionManager{config: cfg}
	// measureLatencies returns early when node is nil
	mgr.measureLatencies()
}
