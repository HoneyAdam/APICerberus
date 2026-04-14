package raft

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewCertFSM(t *testing.T) {
	t.Parallel()
	fsm := NewCertFSM("/tmp/certs", nil)
	if fsm == nil {
		t.Fatal("expected non-nil FSM")
	}
	if fsm.Certificates == nil {
		t.Error("expected initialized certificates map")
	}
	if fsm.StoragePath != "/tmp/certs" {
		t.Errorf("StoragePath = %q, want /tmp/certs", fsm.StoragePath)
	}
}

func TestCertFSM_GetCertificate_NotFound(t *testing.T) {
	t.Parallel()
	fsm := NewCertFSM("", nil)
	_, ok := fsm.GetCertificate("example.com")
	if ok {
		t.Error("expected not found for missing domain")
	}
}

func TestCertFSM_GetCertificate_Found(t *testing.T) {
	t.Parallel()
	fsm := NewCertFSM("", nil)
	fsm.Certificates["example.com"] = &CertificateState{
		Domain:    "example.com",
		CertPEM:   "cert-data",
		KeyPEM:    "key-data",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		IssuedBy:  "node-1",
	}
	cert, ok := fsm.GetCertificate("example.com")
	if !ok {
		t.Fatal("expected to find certificate")
	}
	if cert.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", cert.Domain)
	}
	if cert.CertPEM != "cert-data" {
		t.Errorf("CertPEM = %q, want cert-data", cert.CertPEM)
	}
}

func TestCertFSM_SetTLSManager(t *testing.T) {
	t.Parallel()
	fsm := NewCertFSM("", nil)
	if fsm.tlsManager != nil {
		t.Error("expected nil tlsManager initially")
	}
	mock := &mockCertManager{}
	fsm.SetTLSManager(mock)
	if fsm.tlsManager == nil {
		t.Error("expected tlsManager to be set")
	}
}

func TestCertificateState_Fields(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cs := &CertificateState{
		Domain:    "test.example.com",
		CertPEM:   "-----BEGIN CERT-----",
		KeyPEM:    "-----BEGIN KEY-----",
		IssuedAt:  now,
		ExpiresAt: now.Add(90 * 24 * time.Hour),
		IssuedBy:  "node-leader",
	}
	if cs.Domain != "test.example.com" {
		t.Errorf("Domain = %q", cs.Domain)
	}
	if cs.IssuedBy != "node-leader" {
		t.Errorf("IssuedBy = %q", cs.IssuedBy)
	}
}

func TestCertificateUpdateLog_JSON(t *testing.T) {
	t.Parallel()
	log := CertificateUpdateLog{
		Domain:    "example.com",
		CertPEM:   "cert",
		KeyPEM:    "key",
		IssuedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		IssuedBy:  "node-1",
	}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed CertificateUpdateLog
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", parsed.Domain)
	}
	if parsed.IssuedBy != "node-1" {
		t.Errorf("IssuedBy = %q, want node-1", parsed.IssuedBy)
	}
}

func TestACMERenewalLock_Fields(t *testing.T) {
	t.Parallel()
	lock := ACMERenewalLock{
		Domain:   "example.com",
		NodeID:   "node-1",
		Deadline: time.Now().Add(10 * time.Minute),
	}
	if lock.Domain != "example.com" {
		t.Errorf("Domain = %q", lock.Domain)
	}
	if lock.NodeID != "node-1" {
		t.Errorf("NodeID = %q", lock.NodeID)
	}
}

// mockCertManager implements CertificateManager for testing
type mockCertManager struct{}

func (m *mockCertManager) ReloadCertificate(serverName string) error {
	return nil
}
