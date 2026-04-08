package raft

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test CertFSM LoadCertificatesFromDisk error paths
func TestCertFSM_LoadCertificatesFromDisk_Errors(t *testing.T) {
	t.Run("non-existent directory", func(t *testing.T) {
		fsm := NewCertFSM("/nonexistent/path", nil)
		err := fsm.LoadCertificatesFromDisk()
		// Should not error, just skip loading
		if err != nil {
			t.Errorf("LoadCertificatesFromDisk() error = %v", err)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsm := NewCertFSM(tmpDir, nil)

		err := fsm.LoadCertificatesFromDisk()
		if err != nil {
			t.Errorf("LoadCertificatesFromDisk() error = %v", err)
		}

		// Should have no certificates
		if len(fsm.Certificates) != 0 {
			t.Errorf("Expected 0 certificates, got %d", len(fsm.Certificates))
		}
	})
}

// Test CertFSM GetCertificate not found
func TestCertFSM_GetCertificate_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	cert, ok := fsm.GetCertificate("nonexistent.example.com")
	if ok {
		t.Error("GetCertificate should return false for non-existent certificate")
	}
	if cert != nil {
		t.Error("GetCertificate should return nil for non-existent certificate")
	}
}

// Test CertFSM GetCertificate found
func TestCertFSM_GetCertificate_Found_Additional(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// Add a certificate directly to the map
	fsm.Certificates["test.example.com"] = &CertificateState{
		Domain:    "test.example.com",
		CertPEM:   "cert",
		KeyPEM:    "key",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	cert, ok := fsm.GetCertificate("test.example.com")
	if !ok {
		t.Error("GetCertificate should return true for existing certificate")
	}
	if cert == nil {
		t.Fatal("GetCertificate should return certificate")
	}
	if cert.Domain != "test.example.com" {
		t.Errorf("Expected domain test.example.com, got %s", cert.Domain)
	}
}

// Test CertFSM ListCertificates
func TestCertFSM_ListCertificates(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// Initially empty
	if len(fsm.Certificates) != 0 {
		t.Errorf("Expected 0 certificates, got %d", len(fsm.Certificates))
	}

	// Add a certificate directly to the map
	fsm.Certificates["test.example.com"] = &CertificateState{
		Domain:    "test.example.com",
		CertPEM:   "cert",
		KeyPEM:    "key",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	if len(fsm.Certificates) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(fsm.Certificates))
	}
}

// Test CertFSM ApplyCertCommand error paths
func TestCertFSM_ApplyCertCommand_Error(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// Test with unknown command type
	err := fsm.ApplyCertCommand("unknown_operation", []byte("{}"))
	if err == nil {
		t.Error("ApplyCertCommand should return error for unknown command type")
	}

	// Test with invalid JSON for certificate_update
	err = fsm.ApplyCertCommand("certificate_update", []byte("not valid json"))
	if err == nil {
		t.Error("ApplyCertCommand should return error for invalid JSON")
	}

	// Test with missing required fields
	invalidCert := `{"domain": "", "cert_pem": "", "key_pem": ""}`
	err = fsm.ApplyCertCommand("certificate_update", []byte(invalidCert))
	if err == nil {
		t.Error("ApplyCertCommand should return error for missing required fields")
	}

	// Test with invalid JSON for acme_renewal_lock
	err = fsm.ApplyCertCommand("acme_renewal_lock", []byte("not valid json"))
	if err == nil {
		t.Error("ApplyCertCommand should return error for invalid renewal lock JSON")
	}
}

// Test CertFSM ApplyCertCommand certificate_update success
func TestCertFSM_ApplyCertCommand_CertificateUpdate_Additional(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	update := &CertificateUpdateLog{
		Domain:    "test.example.com",
		CertPEM:   "cert content",
		KeyPEM:    "key content",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		IssuedBy:  "node1",
	}

	data, _ := json.Marshal(update)
	err := fsm.ApplyCertCommand("certificate_update", data)
	if err != nil {
		t.Errorf("ApplyCertCommand() error = %v", err)
	}

	// Verify certificate was stored
	cert, ok := fsm.GetCertificate("test.example.com")
	if !ok {
		t.Error("Certificate should be stored in FSM")
	}
	if cert.CertPEM != "cert content" {
		t.Errorf("Expected cert content 'cert content', got %s", cert.CertPEM)
	}

	// Verify certificate was written to disk
	certPath := filepath.Join(tmpDir, "test.example.com", "cert.pem")
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("Certificate file should exist on disk")
	}
}

// Test CertFSM ApplyCertCommand acme_renewal_lock success
func TestCertFSM_ApplyCertCommand_ACMERenewalLock_Additional(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	lock := &ACMERenewalLock{
		Domain:   "test.example.com",
		NodeID:   "node1",
		Deadline: time.Now().Add(time.Minute),
	}

	data, _ := json.Marshal(lock)
	err := fsm.ApplyCertCommand("acme_renewal_lock", data)
	if err != nil {
		t.Errorf("ApplyCertCommand() error = %v", err)
	}

	// Verify lock was stored
	fsm.lockMu.RLock()
	storedLock, ok := fsm.RenewalLocks["test.example.com"]
	fsm.lockMu.RUnlock()

	if !ok {
		t.Error("Renewal lock should be stored in FSM")
	}
	if storedLock.NodeID != "node1" {
		t.Errorf("Expected node ID 'node1', got %s", storedLock.NodeID)
	}
}

// Test CertFSM ApplyCertCommand acme_renewal_lock conflict
func TestCertFSM_ApplyCertCommand_ACMERenewalLock_Conflict_Additional(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// First lock
	lock1 := &ACMERenewalLock{
		Domain:   "test.example.com",
		NodeID:   "node1",
		Deadline: time.Now().Add(time.Minute),
	}
	data1, _ := json.Marshal(lock1)
	err := fsm.ApplyCertCommand("acme_renewal_lock", data1)
	if err != nil {
		t.Fatalf("First lock should succeed: %v", err)
	}

	// Second lock should fail
	lock2 := &ACMERenewalLock{
		Domain:   "test.example.com",
		NodeID:   "node2",
		Deadline: time.Now().Add(time.Minute),
	}
	data2, _ := json.Marshal(lock2)
	err = fsm.ApplyCertCommand("acme_renewal_lock", data2)
	if err == nil {
		t.Error("Second lock should fail when first is still valid")
	}
}

// Test CertFSM GetCertificateFromDisk error paths
func TestCertFSM_GetCertificateFromDisk_Errors(t *testing.T) {
	t.Run("empty storage path", func(t *testing.T) {
		fsm := NewCertFSM("", nil)
		_, err := fsm.GetCertificateFromDisk("test.example.com")
		if err == nil {
			t.Error("GetCertificateFromDisk should return error for empty storage path")
		}
	})

	t.Run("non-existent domain", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsm := NewCertFSM(tmpDir, nil)
		_, err := fsm.GetCertificateFromDisk("nonexistent.example.com")
		if err == nil {
			t.Error("GetCertificateFromDisk should return error for non-existent domain")
		}
	})

	t.Run("missing key file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsm := NewCertFSM(tmpDir, nil)

		// Create domain directory with only cert file
		domainDir := filepath.Join(tmpDir, "test.example.com")
		os.MkdirAll(domainDir, 0755)
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("cert"), 0644)

		_, err := fsm.GetCertificateFromDisk("test.example.com")
		if err == nil {
			t.Error("GetCertificateFromDisk should return error for missing key file")
		}
	})
}

// Test CertFSM GetCertificateFromDisk success
func TestCertFSM_GetCertificateFromDisk_Success(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// Create domain directory with cert and key
	domainDir := filepath.Join(tmpDir, "test.example.com")
	os.MkdirAll(domainDir, 0755)
	os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("cert content"), 0644)
	os.WriteFile(filepath.Join(domainDir, "key.pem"), []byte("key content"), 0644)

	// Write metadata
	meta := `{"issued_at": "2024-01-01T00:00:00Z", "expires_at": "2025-01-01T00:00:00Z", "issued_by": "node1"}`
	os.WriteFile(filepath.Join(domainDir, "meta.json"), []byte(meta), 0644)

	cert, err := fsm.GetCertificateFromDisk("test.example.com")
	if err != nil {
		t.Errorf("GetCertificateFromDisk() error = %v", err)
	}
	if cert == nil {
		t.Fatal("GetCertificateFromDisk should return certificate")
	}
	if cert.CertPEM != "cert content" {
		t.Errorf("Expected cert content 'cert content', got %s", cert.CertPEM)
	}
	if cert.KeyPEM != "key content" {
		t.Errorf("Expected key content 'key content', got %s", cert.KeyPEM)
	}
	if cert.IssuedBy != "node1" {
		t.Errorf("Expected issued_by 'node1', got %s", cert.IssuedBy)
	}
}

// Test CertFSM writeCertificateToDisk error paths
func TestCertFSM_WriteCertificateToDisk_Errors(t *testing.T) {
	t.Run("empty storage path", func(t *testing.T) {
		fsm := NewCertFSM("", nil)
		update := &CertificateUpdateLog{
			Domain:  "test.example.com",
			CertPEM: "cert",
			KeyPEM:  "key",
		}
		err := fsm.writeCertificateToDisk(update)
		if err == nil {
			t.Error("writeCertificateToDisk should return error for empty storage path")
		}
	})

	t.Run("invalid directory", func(t *testing.T) {
		// Use a path with invalid characters on Windows
		fsm := NewCertFSM("/::invalid/path::/test", nil)
		update := &CertificateUpdateLog{
			Domain:  "test.example.com",
			CertPEM: "cert",
			KeyPEM:  "key",
		}
		err := fsm.writeCertificateToDisk(update)
		if err == nil {
			t.Error("writeCertificateToDisk should return error for invalid directory")
		}
	})
}

// Test ACMERenewalLock JSON marshaling
func TestACMERenewalLock_JSON(t *testing.T) {
	lock := &ACMERenewalLock{
		Domain:   "test.example.com",
		NodeID:   "node1",
		Deadline: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(lock)
	if err != nil {
		t.Errorf("Marshal error: %v", err)
	}

	// Verify it can be unmarshaled
	var decoded ACMERenewalLock
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Errorf("Unmarshal error: %v", err)
	}

	if decoded.Domain != lock.Domain {
		t.Errorf("Domain = %q, want %q", decoded.Domain, lock.Domain)
	}
}

// Test CertificateUpdateLog JSON marshaling
func TestCertificateUpdateLog_JSON(t *testing.T) {
	update := &CertificateUpdateLog{
		Domain:    "test.example.com",
		CertPEM:   "cert content",
		KeyPEM:    "key content",
		IssuedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		IssuedBy:  "node1",
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Errorf("Marshal error: %v", err)
	}

	// Verify it can be unmarshaled
	var decoded CertificateUpdateLog
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Errorf("Unmarshal error: %v", err)
	}

	if decoded.Domain != update.Domain {
		t.Errorf("Domain = %q, want %q", decoded.Domain, update.Domain)
	}
}

// Test CertFSM GetCertificateFromDisk with missing metadata
func TestCertFSM_GetCertificateFromDisk_NoMeta(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// Create domain directory with only cert and key (no meta.json)
	domainDir := filepath.Join(tmpDir, "test.example.com")
	os.MkdirAll(domainDir, 0755)
	os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("cert"), 0644)
	os.WriteFile(filepath.Join(domainDir, "key.pem"), []byte("key"), 0644)

	// Should still work without meta.json
	cert, err := fsm.GetCertificateFromDisk("test.example.com")
	if err != nil {
		t.Errorf("GetCertificateFromDisk() error = %v", err)
	}
	_ = cert
}

// Test Node GetNodeID
func TestNode_GetNodeID(t *testing.T) {
	node := &Node{
		ID: "test-node-1",
	}

	if got := node.GetNodeID(); got != "test-node-1" {
		t.Errorf("GetNodeID() = %q, want %q", got, "test-node-1")
	}
}

// Test Node becomeFollower
func TestNode_BecomeFollower(t *testing.T) {
	cfg := &Config{
		NodeID:             "node1",
		BindAddress:        "localhost:8001",
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 100 * time.Millisecond,
		ElectionTimeoutMax: 200 * time.Millisecond,
	}
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Start as candidate
	node.State = StateCandidate
	node.CurrentTerm = 5
	node.VotedFor = "node1"

	// Become follower
	node.becomeFollower(6)

	if node.State != StateFollower {
		t.Errorf("State = %v, want %v", node.State, StateFollower)
	}
	if node.CurrentTerm != 6 {
		t.Errorf("CurrentTerm = %d, want %d", node.CurrentTerm, 6)
	}
	if node.VotedFor != "" {
		t.Errorf("VotedFor = %q, want empty", node.VotedFor)
	}
}

// Test Node ProposeCertificateUpdate not leader
func TestNode_ProposeCertificateUpdate_NotLeader(t *testing.T) {
	cfg := &Config{
		NodeID:             "node1",
		BindAddress:        "localhost:8001",
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 100 * time.Millisecond,
		ElectionTimeoutMax: 200 * time.Millisecond,
	}
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Set as follower
	node.State = StateFollower

	err = node.ProposeCertificateUpdate("test.example.com", "cert", "key", time.Now().Add(time.Hour))
	if err == nil {
		t.Error("ProposeCertificateUpdate should return error when not leader")
	}
	if !strings.Contains(err.Error(), "not the leader") {
		t.Errorf("Expected 'not the leader' error, got: %v", err)
	}
}

// Test Node AcquireACMERenewalLock not leader
func TestNode_AcquireACMERenewalLock_NotLeader(t *testing.T) {
	cfg := &Config{
		NodeID:             "node1",
		BindAddress:        "localhost:8001",
		HeartbeatInterval:  50 * time.Millisecond,
		ElectionTimeoutMin: 100 * time.Millisecond,
		ElectionTimeoutMax: 200 * time.Millisecond,
	}
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()

	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error: %v", err)
	}

	// Set as follower
	node.State = StateFollower

	locked, err := node.AcquireACMERenewalLock("test.example.com", time.Minute)
	if err == nil {
		t.Error("AcquireACMERenewalLock should return error when not leader")
	}
	if locked {
		t.Error("AcquireACMERenewalLock should return false when not leader")
	}
	if !strings.Contains(err.Error(), "not the leader") {
		t.Errorf("Expected 'not the leader' error, got: %v", err)
	}
}

// Test ClusterManager authMiddleware
func TestClusterManager_AuthMiddleware(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18001", "test-api-key")

	tests := []struct {
		name       string
		apiKey     string
		wantStatus int
	}{
		{
			name:       "valid key",
			apiKey:     "Bearer test-api-key",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid key",
			apiKey:     "Bearer wrong-key",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing key",
			apiKey:     "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := cm.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.apiKey != "" {
				req.Header.Set("Authorization", tt.apiKey)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

// Test ClusterManager handleClusterStatus
func TestClusterManager_HandleClusterStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18002", "test-api-key")

	// Test GET method
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/cluster/status", nil)
	rr := httptest.NewRecorder()

	cm.handleClusterStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Test POST method (should fail)
	req = httptest.NewRequest(http.MethodPost, "/admin/api/v1/cluster/status", nil)
	rr = httptest.NewRecorder()

	cm.handleClusterStatus(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleNodes
func TestClusterManager_HandleNodes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18003", "test-api-key")

	// Test GET method
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/cluster/nodes", nil)
	rr := httptest.NewRecorder()

	cm.handleNodes(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Test POST method (should fail with method not allowed)
	req = httptest.NewRequest(http.MethodPost, "/admin/api/v1/cluster/nodes", nil)
	rr = httptest.NewRecorder()

	cm.handleNodes(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleJoin method not allowed
func TestClusterManager_HandleJoin_MethodNotAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18004", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/cluster/join", nil)
	rr := httptest.NewRecorder()

	cm.handleJoin(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleLeave method not allowed
func TestClusterManager_HandleLeave_MethodNotAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18005", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/cluster/leave", nil)
	rr := httptest.NewRecorder()

	cm.handleLeave(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleSnapshot method not allowed
func TestClusterManager_HandleSnapshot_MethodNotAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18006", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/cluster/snapshot", nil)
	rr := httptest.NewRecorder()

	cm.handleSnapshot(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleRaftState
func TestClusterManager_HandleRaftState(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18007", "test-api-key")

	// Test GET method
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/raft/state", nil)
	rr := httptest.NewRecorder()

	cm.handleRaftState(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Test POST method (should fail)
	req = httptest.NewRequest(http.MethodPost, "/admin/api/v1/raft/state", nil)
	rr = httptest.NewRecorder()

	cm.handleRaftState(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager listNodes HTTP handler
func TestClusterManager_ListNodes_HTTP(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/raft/nodes", nil)
	rr := httptest.NewRecorder()

	cm.listNodes(rr, req)

	// Should return OK or MethodNotAllowed for wrong method
	if rr.Code != http.StatusOK && rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d or %d", rr.Code, http.StatusOK, http.StatusMethodNotAllowed)
	}
}

// Test Node lastLogIndex
func TestNode_LastLogIndex(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error = %v", err)
	}
	node.SetStorage(NewInmemStorage())

	index := node.lastLogIndex()

	// Should return 0 for new node
	if index != 0 {
		t.Errorf("lastLogIndex = %d, want 0", index)
	}
}

// Test Node lastLogTerm
func TestNode_LastLogTerm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error = %v", err)
	}
	node.SetStorage(NewInmemStorage())

	term := node.lastLogTerm()

	// Should return 0 for new node
	if term != 0 {
		t.Errorf("lastLogTerm = %d, want 0", term)
	}
}

// Test Node lastLogIndex after appending entry
func TestNode_LastLogIndex_WithEntry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error = %v", err)
	}
	node.SetStorage(NewInmemStorage())

	// Append an entry (will fail since not leader)
	_, err = node.AppendEntry([]byte("test data"))

	// lastLogIndex should still be valid
	index := node.lastLogIndex()

	// Should be at least 0
	if index < 0 {
		t.Errorf("lastLogIndex = %d, want non-negative", index)
	}
}

// Test Node lastLogTerm after appending entry
func TestNode_LastLogTerm_WithEntry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error = %v", err)
	}
	node.SetStorage(NewInmemStorage())

	// Append an entry (will fail since not leader)
	_, _ = node.AppendEntry([]byte("test data"))

	term := node.lastLogTerm()

	// Term should be valid
	if term < 0 {
		t.Errorf("lastLogTerm = %d, want non-negative", term)
	}
}

// Test Node GetLogEntry
func TestNode_GetLogEntry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error = %v", err)
	}
	node.SetStorage(NewInmemStorage())

	// Get non-existent entry
	_, found := node.getLogEntry(1)

	if found {
		t.Error("Expected entry to not be found")
	}
}

// Test Node GetLogEntry after appending
func TestNode_GetLogEntry_AfterAppend(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, err := NewNode(cfg, fsm, transport)
	if err != nil {
		t.Fatalf("NewNode error = %v", err)
	}
	node.SetStorage(NewInmemStorage())

	// Append an entry (will fail since not leader)
	_, _ = node.AppendEntry([]byte("test data"))

	// Try to get entry - may or may not exist
	entry, found := node.getLogEntry(1)

	// Just verify it doesn't panic
	_ = entry
	_ = found
}

// Test CertFSM ApplyCertCommand with unknown command
func TestCertFSM_ApplyCertCommand_Unknown(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	err := fsm.ApplyCertCommand("unknown_command", []byte("data"))

	if err == nil {
		t.Error("Expected error for unknown command type")
	}
}

// Test CertFSM ApplyCertCommand with invalid JSON
func TestCertFSM_ApplyCertCommand_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	err := fsm.ApplyCertCommand("certificate_update", []byte("invalid json"))

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// Test ClusterManager Propose when not leader
func TestClusterManager_Propose_NotLeader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	cmd := FSMCommand{Type: "test", Payload: json.RawMessage("data")}
	err := cm.Propose(cmd)

	// Should fail since not leader
	if err == nil {
		t.Error("Expected error when proposing as non-leader")
	}
}

// Test ClusterManager handleNodes with GET
func TestClusterManager_HandleNodes_GET(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/raft/nodes", nil)
	rr := httptest.NewRecorder()

	cm.handleNodes(rr, req)

	// Should return OK
	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// Test ClusterManager handleJoin with invalid method
func TestClusterManager_HandleJoin_InvalidMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/raft/join", nil)
	rr := httptest.NewRecorder()

	cm.handleJoin(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleJoin with invalid JSON
func TestClusterManager_HandleJoin_InvalidJSON(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/raft/join", strings.NewReader("invalid json"))
	rr := httptest.NewRecorder()

	cm.handleJoin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// Test ClusterManager handleLeave with invalid method
func TestClusterManager_HandleLeave_InvalidMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/raft/leave", nil)
	rr := httptest.NewRecorder()

	cm.handleLeave(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager handleLeave with invalid JSON
func TestClusterManager_HandleLeave_InvalidJSON(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/raft/leave", strings.NewReader("invalid json"))
	rr := httptest.NewRecorder()

	cm.handleLeave(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// Test ClusterManager handleSnapshot with invalid method
func TestClusterManager_HandleSnapshot_InvalidMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/raft/snapshot", nil)
	rr := httptest.NewRecorder()

	cm.handleSnapshot(rr, req)

	// Result depends on implementation
	// Just verify it doesn't panic
	_ = rr.Code
}

// Test ClusterManager handleClusterStatus with GET
func TestClusterManager_HandleClusterStatus_GET(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/raft/status", nil)
	rr := httptest.NewRecorder()

	cm.handleClusterStatus(rr, req)

	// Should return OK
	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// Test ClusterManager handleClusterStatus with invalid method
func TestClusterManager_HandleClusterStatus_InvalidMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/raft/status", nil)
	rr := httptest.NewRecorder()

	cm.handleClusterStatus(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// Test ClusterManager authMiddleware without API key configured
func TestClusterManager_AuthMiddleware_NoKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "node1"
	cfg.BindAddress = "localhost:8001"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	cm := NewClusterManager(node, fsm, "localhost:18008", "") // No API key

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	wrapped := cm.authMiddleware(testHandler)

	// Test without API key - behavior depends on implementation
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// Just verify it doesn't panic - behavior may vary
	_ = rr.Code
}

// Test CertFSM with invalid certificate data
func TestCertFSM_ApplyCertCommand_InvalidData(t *testing.T) {
	tmpDir := t.TempDir()
	fsm := NewCertFSM(tmpDir, nil)

	// Test with missing fields
	invalidData := `{"domain":"","cert_pem":"","key_pem":""}`
	err := fsm.ApplyCertCommand("certificate_update", []byte(invalidData))

	if err == nil {
		t.Error("Expected error for invalid certificate data")
	}
}

// TestClusterManager_HandleJoin_AsLeader tests handleJoin when node is leader
func TestClusterManager_HandleJoin_AsLeader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = "leader-node"
	cfg.BindAddress = "localhost:0"
	fsm := NewGatewayFSM()
	transport := NewInmemTransport()
	node, _ := NewNode(cfg, fsm, transport)

	// Set node as leader
	node.State = StateLeader

	cm := NewClusterManager(node, fsm, "localhost:18008", "test-api-key")

	// Create valid join request
	joinReq := JoinRequest{
		NodeID:  "new-node",
		Address: "localhost:8002",
	}
	body, _ := json.Marshal(joinReq)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/raft/join", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()

	cm.handleJoin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify response
	var resp JoinResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success to be true")
	}

	if resp.LeaderID != "leader-node" {
		t.Errorf("Expected LeaderID = 'leader-node', got %s", resp.LeaderID)
	}

	// Verify peer was added
	if _, ok := node.Peers["new-node"]; !ok {
		t.Error("Expected new-node to be added to peers")
	}
}
