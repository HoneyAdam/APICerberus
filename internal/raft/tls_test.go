package raft

import (
	"crypto/tls"
	"testing"
)

func TestTLSCertificateManager(t *testing.T) {
	manager, err := NewTLSCertificateManager("node1", "test-cluster")
	if err != nil {
		t.Fatalf("NewTLSCertificateManager error: %v", err)
	}

	// Generate CA
	if err := manager.GenerateCA(); err != nil {
		t.Fatalf("GenerateCA error: %v", err)
	}

	// Export CA
	caCert, err := manager.ExportCACert()
	if err != nil {
		t.Fatalf("ExportCACert error: %v", err)
	}
	if len(caCert) == 0 {
		t.Fatal("CA certificate is empty")
	}

	// Generate node certificate
	if err := manager.GenerateNodeCertificate(); err != nil {
		t.Fatalf("GenerateNodeCertificate error: %v", err)
	}

	// Get TLS config
	config, err := manager.GetTLSConfig()
	if err != nil {
		t.Fatalf("GetTLSConfig error: %v", err)
	}
	if config == nil {
		t.Fatal("TLS config is nil")
	}

	// Verify TLS settings
	if config.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected TLS 1.3, got %d", config.MinVersion)
	}
	if config.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("expected client auth required")
	}
}

func TestTLSCertificateManager_NoCA(t *testing.T) {
	manager, err := NewTLSCertificateManager("node1", "test-cluster")
	if err != nil {
		t.Fatalf("NewTLSCertificateManager error: %v", err)
	}

	// Try to generate node cert without CA
	err = manager.GenerateNodeCertificate()
	if err == nil {
		t.Fatal("expected error when generating node cert without CA")
	}

	// Try to get TLS config without cert
	_, err = manager.GetTLSConfig()
	if err == nil {
		t.Fatal("expected error when getting TLS config without cert")
	}
}

func TestTLSCertificateManager_ImportCACert(t *testing.T) {
	// Create first manager and generate CA
	manager1, _ := NewTLSCertificateManager("node1", "test-cluster")
	manager1.GenerateCA()
	caCert, _ := manager1.ExportCACert()

	// Create second manager and import CA
	manager2, _ := NewTLSCertificateManager("node2", "test-cluster")
	err := manager2.ImportCACert(caCert)
	if err != nil {
		t.Fatalf("ImportCACert error: %v", err)
	}

	if manager2.caCert == nil {
		t.Fatal("CA cert not imported")
	}
}
