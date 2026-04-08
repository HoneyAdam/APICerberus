package certmanager

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// mockRaftNode implements RaftNode interface for testing
type mockRaftNode struct {
	isLeader        bool
	nodeID          string
	acquireLockFunc func(domain string, timeout time.Duration) (bool, error)
	proposeFunc     func(domain, certPEM, keyPEM string, expiresAt time.Time) error
}

func (m *mockRaftNode) ProposeCertificateUpdate(domain, certPEM, keyPEM string, expiresAt time.Time) error {
	if m.proposeFunc != nil {
		return m.proposeFunc(domain, certPEM, keyPEM, expiresAt)
	}
	return nil
}

func (m *mockRaftNode) AcquireACMERenewalLock(domain string, timeout time.Duration) (bool, error) {
	if m.acquireLockFunc != nil {
		return m.acquireLockFunc(domain, timeout)
	}
	return true, nil
}

func (m *mockRaftNode) IsLeader() bool {
	return m.isLeader
}

func (m *mockRaftNode) NodeID() string {
	if m.nodeID == "" {
		return "test-node"
	}
	return m.nodeID
}

func TestNewACMEProvider(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewACMEProvider(nil, nil)
		if err == nil {
			t.Error("NewACMEProvider should return error for nil config")
		}
	})

	t.Run("ACME disabled", func(t *testing.T) {
		cfg := &config.Config{
			ACME: config.ACMEConfig{Enabled: false},
		}
		_, err := NewACMEProvider(cfg, nil)
		if err == nil {
			t.Error("NewACMEProvider should return error when ACME disabled")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &config.Config{
			ACME: config.ACMEConfig{
				Enabled:      true,
				Email:        "test@example.com",
				DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
				StoragePath:  tmpDir,
			},
			Cluster: config.ClusterConfig{
				CertificateSync: config.CertificateSyncConfig{
					Enabled:         true,
					RaftReplication: false,
				},
			},
		}

		provider, err := NewACMEProvider(cfg, nil)
		if err != nil {
			t.Fatalf("NewACMEProvider() error = %v", err)
		}
		if provider == nil {
			t.Fatal("NewACMEProvider() returned nil")
		}
		if provider.directoryURL != cfg.ACME.DirectoryURL {
			t.Errorf("directoryURL = %v, want %v", provider.directoryURL, cfg.ACME.DirectoryURL)
		}
		if provider.email != cfg.ACME.Email {
			t.Errorf("email = %v, want %v", provider.email, cfg.ACME.Email)
		}
		if provider.client == nil {
			t.Error("client not initialized")
		}
	})

	t.Run("with raft sync enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &config.Config{
			ACME: config.ACMEConfig{
				Enabled:      true,
				Email:        "test@example.com",
				DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
				StoragePath:  tmpDir,
			},
			Cluster: config.ClusterConfig{
				CertificateSync: config.CertificateSyncConfig{
					Enabled:         true,
					RaftReplication: true,
				},
			},
		}

		raftNode := &mockRaftNode{isLeader: true}
		provider, err := NewACMEProvider(cfg, raftNode)
		if err != nil {
			t.Fatalf("NewACMEProvider() error = %v", err)
		}
		if !provider.useRaftSync {
			t.Error("useRaftSync should be true")
		}
		if provider.raftNode != raftNode {
			t.Error("raftNode not set correctly")
		}
	})
}

func TestACMEProvider_GetCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
		certCache:   make(map[string]*CachedCertificate),
	}

	t.Run("no SNI provided", func(t *testing.T) {
		hello := &tls.ClientHelloInfo{}
		_, err := provider.GetCertificate(hello)
		if err == nil {
			t.Error("GetCertificate should return error when no SNI provided")
		}
	})

	t.Run("certificate from cache", func(t *testing.T) {
		domain := "test.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))

		provider.cacheCertificate(&CachedCertificate{
			Domain:    domain,
			Cert:      cert,
			Key:       generateTestKey(t),
			CertPEM:   certToPEM(t, cert),
			KeyPEM:    keyToPEM(t, generateTestKey(t)),
			IssuedAt:  time.Now(),
			ExpiresAt: cert.NotAfter,
		})

		hello := &tls.ClientHelloInfo{ServerName: domain}
		tlsCert, err := provider.GetCertificate(hello)
		if err != nil {
			t.Errorf("GetCertificate() error = %v", err)
		}
		if tlsCert == nil {
			t.Error("GetCertificate() returned nil")
		}
	})

	t.Run("certificate from disk", func(t *testing.T) {
		domain := "disk.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))
		key := generateTestKey(t)

		// Store certificate to disk
		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), certToPEM(t, cert), 0600)
		os.WriteFile(filepath.Join(domainDir, "key.pem"), keyToPEM(t, key), 0600)

		hello := &tls.ClientHelloInfo{ServerName: domain}
		tlsCert, err := provider.GetCertificate(hello)
		if err != nil {
			t.Errorf("GetCertificate() error = %v", err)
		}
		if tlsCert == nil {
			t.Error("GetCertificate() returned nil")
		}
	})

	t.Run("certificate not available", func(t *testing.T) {
		hello := &tls.ClientHelloInfo{ServerName: "nonexistent.example.com"}
		_, err := provider.GetCertificate(hello)
		if err == nil {
			t.Error("GetCertificate should return error for unknown domain")
		}
	})

	t.Run("expired certificate should not be returned from cache", func(t *testing.T) {
		domain := "expired.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(-1*time.Hour))

		provider.cacheCertificate(&CachedCertificate{
			Domain:    domain,
			Cert:      cert,
			Key:       generateTestKey(t),
			CertPEM:   certToPEM(t, cert),
			KeyPEM:    keyToPEM(t, generateTestKey(t)),
			IssuedAt:  time.Now().Add(-90 * 24 * time.Hour),
			ExpiresAt: cert.NotAfter,
		})

		hello := &tls.ClientHelloInfo{ServerName: domain}
		_, err := provider.GetCertificate(hello)
		if err == nil {
			t.Error("GetCertificate should return error for expired certificate")
		}
	})
}

func TestACMEProvider_storeAndLoadCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
		certCache:   make(map[string]*CachedCertificate),
	}

	t.Run("store certificate locally", func(t *testing.T) {
		domain := "store.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))
		key := generateTestKey(t)

		cached := &CachedCertificate{
			Domain:    domain,
			Cert:      cert,
			Key:       key,
			CertPEM:   certToPEM(t, cert),
			KeyPEM:    keyToPEM(t, key),
			IssuedAt:  time.Now(),
			ExpiresAt: cert.NotAfter,
		}

		err := provider.storeCertificateLocally(cached)
		if err != nil {
			t.Errorf("storeCertificateLocally() error = %v", err)
		}

		// Verify files exist
		domainDir := filepath.Join(tmpDir, domain)
		certPath := filepath.Join(domainDir, "cert.pem")
		keyPath := filepath.Join(domainDir, "key.pem")

		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			t.Error("Certificate file was not created")
		}
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			t.Error("Key file was not created")
		}
	})

	t.Run("load certificate from disk", func(t *testing.T) {
		domain := "load.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))
		key := generateTestKey(t)

		// Store first
		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), certToPEM(t, cert), 0600)
		os.WriteFile(filepath.Join(domainDir, "key.pem"), keyToPEM(t, key), 0600)

		// Load
		loaded, err := provider.loadCertificateFromDisk(domain)
		if err != nil {
			t.Errorf("loadCertificateFromDisk() error = %v", err)
		}
		if loaded == nil {
			t.Fatal("loadCertificateFromDisk() returned nil")
		}
		if loaded.Domain != domain {
			t.Errorf("Domain = %v, want %v", loaded.Domain, domain)
		}
		if loaded.Cert == nil {
			t.Error("Cert is nil")
		}
	})

	t.Run("load non-existent certificate", func(t *testing.T) {
		_, err := provider.loadCertificateFromDisk("nonexistent.example.com")
		if err == nil {
			t.Error("loadCertificateFromDisk should return error for non-existent cert")
		}
	})

	t.Run("load certificate with invalid PEM", func(t *testing.T) {
		domain := "invalid.example.com"
		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("invalid pem"), 0600)
		os.WriteFile(filepath.Join(domainDir, "key.pem"), []byte("invalid pem"), 0600)

		_, err := provider.loadCertificateFromDisk(domain)
		if err == nil {
			t.Error("loadCertificateFromDisk should return error for invalid PEM")
		}
	})
}

func TestACMEProvider_cacheCertificate(t *testing.T) {
	provider := &ACMEProvider{
		certCache: make(map[string]*CachedCertificate),
	}

	domain := "cache.example.com"
	cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))

	cached := &CachedCertificate{
		Domain:    domain,
		Cert:      cert,
		Key:       generateTestKey(t),
		CertPEM:   certToPEM(t, cert),
		KeyPEM:    keyToPEM(t, generateTestKey(t)),
		IssuedAt:  time.Now(),
		ExpiresAt: cert.NotAfter,
	}

	provider.cacheCertificate(cached)

	// Verify cache
	provider.cacheMu.RLock()
	stored, ok := provider.certCache[domain]
	provider.cacheMu.RUnlock()

	if !ok {
		t.Error("Certificate not found in cache")
	}
	if stored.Domain != domain {
		t.Errorf("Cached domain = %v, want %v", stored.Domain, domain)
	}
}

func TestACMEProvider_loadExistingCertificates(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
		certCache:   make(map[string]*CachedCertificate),
	}

	// Create multiple certificate directories
	domains := []string{"a.example.com", "b.example.com"}
	for _, domain := range domains {
		cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))
		key := generateTestKey(t)

		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), certToPEM(t, cert), 0600)
		os.WriteFile(filepath.Join(domainDir, "key.pem"), keyToPEM(t, key), 0600)
	}

	// Create a file (not a directory) to test filtering
	os.WriteFile(filepath.Join(tmpDir, "not-a-domain.txt"), []byte("test"), 0644)

	err := provider.loadExistingCertificates()
	if err != nil {
		t.Errorf("loadExistingCertificates() error = %v", err)
	}

	// Verify both certificates loaded
	for _, domain := range domains {
		provider.cacheMu.RLock()
		_, ok := provider.certCache[domain]
		provider.cacheMu.RUnlock()
		if !ok {
			t.Errorf("Certificate for %s not loaded", domain)
		}
	}
}

func TestACMEProvider_loadOrCreateAccountKey(t *testing.T) {
	t.Run("create new key", func(t *testing.T) {
		tmpDir := t.TempDir()
		provider := &ACMEProvider{
			storagePath: tmpDir,
			certCache:   make(map[string]*CachedCertificate),
		}

		err := provider.loadOrCreateAccountKey()
		if err != nil {
			t.Errorf("loadOrCreateAccountKey() error = %v", err)
		}
		if provider.accountKey == nil {
			t.Error("accountKey is nil after creation")
		}

		// Verify key file was created
		keyPath := filepath.Join(tmpDir, "account.key")
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			t.Error("Account key file was not created")
		}
	})

	t.Run("load existing key", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a key first
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		keyDER, _ := x509.MarshalECPrivateKey(key)
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		os.WriteFile(filepath.Join(tmpDir, "account.key"), keyPEM, 0600)

		provider := &ACMEProvider{
			storagePath: tmpDir,
			certCache:   make(map[string]*CachedCertificate),
		}

		err := provider.loadOrCreateAccountKey()
		if err != nil {
			t.Errorf("loadOrCreateAccountKey() error = %v", err)
		}
		if provider.accountKey == nil {
			t.Error("accountKey is nil after loading")
		}
	})
}

func TestACMEProvider_checkAndRenewCertificates(t *testing.T) {
	t.Run("non-leader skips renewal with Raft sync", func(t *testing.T) {
		tmpDir := t.TempDir()
		provider := &ACMEProvider{
			storagePath:  tmpDir,
			certCache:    make(map[string]*CachedCertificate),
			useRaftSync:  true,
			renewalLocks: make(map[string]time.Time),
			raftNode:     &mockRaftNode{isLeader: false},
		}

		// Add an expiring certificate
		domain := "expire.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(7*24*time.Hour))

		provider.cacheCertificate(&CachedCertificate{
			Domain:    domain,
			Cert:      cert,
			Key:       generateTestKey(t),
			CertPEM:   certToPEM(t, cert),
			KeyPEM:    keyToPEM(t, generateTestKey(t)),
			IssuedAt:  time.Now().Add(-60 * 24 * time.Hour),
			ExpiresAt: cert.NotAfter,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// This should return early because we're not the leader
		provider.checkAndRenewCertificates(ctx)
		// No panic = success for non-leader
	})

	t.Run("non-leader skips renewal", func(t *testing.T) {
		tmpDir := t.TempDir()
		provider := &ACMEProvider{
			storagePath:  tmpDir,
			certCache:    make(map[string]*CachedCertificate),
			useRaftSync:  true,
			renewalLocks: make(map[string]time.Time),
			raftNode:     &mockRaftNode{isLeader: false},
		}

		// Add an expiring certificate
		domain := "expire.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(7*24*time.Hour))

		provider.cacheCertificate(&CachedCertificate{
			Domain:    domain,
			Cert:      cert,
			Key:       generateTestKey(t),
			CertPEM:   certToPEM(t, cert),
			KeyPEM:    keyToPEM(t, generateTestKey(t)),
			IssuedAt:  time.Now().Add(-60 * 24 * time.Hour),
			ExpiresAt: cert.NotAfter,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// This should return early because we're not the leader
		provider.checkAndRenewCertificates(ctx)
		// No panic = success for non-leader
	})

	t.Run("certificate not expiring soon", func(t *testing.T) {
		tmpDir := t.TempDir()
		provider := &ACMEProvider{
			storagePath:  tmpDir,
			certCache:    make(map[string]*CachedCertificate),
			renewalLocks: make(map[string]time.Time),
		}

		// Add a certificate that doesn't need renewal
		domain := "fresh.example.com"
		cert := generateTestCertificate(t, domain, time.Now().Add(60*24*time.Hour)) // Expires in 60 days

		provider.cacheCertificate(&CachedCertificate{
			Domain:    domain,
			Cert:      cert,
			Key:       generateTestKey(t),
			CertPEM:   certToPEM(t, cert),
			KeyPEM:    keyToPEM(t, generateTestKey(t)),
			IssuedAt:  time.Now(),
			ExpiresAt: cert.NotAfter,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		provider.checkAndRenewCertificates(ctx)
		// Certificate not expiring, should not trigger renewal
	})
}

func TestCachedCertificate(t *testing.T) {
	domain := "test.example.com"
	cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))
	key := generateTestKey(t)

	cached := &CachedCertificate{
		Domain:    domain,
		Cert:      cert,
		Key:       key,
		CertPEM:   certToPEM(t, cert),
		KeyPEM:    keyToPEM(t, key),
		IssuedAt:  time.Now(),
		ExpiresAt: cert.NotAfter,
	}

	if cached.Domain != domain {
		t.Errorf("Domain = %v, want %v", cached.Domain, domain)
	}
	if cached.Cert == nil {
		t.Error("Cert is nil")
	}
	if cached.Key == nil {
		t.Error("Key is nil")
	}
	if len(cached.CertPEM) == 0 {
		t.Error("CertPEM is empty")
	}
	if len(cached.KeyPEM) == 0 {
		t.Error("KeyPEM is empty")
	}
}

func TestACMEConfig(t *testing.T) {
	cfg := config.ACMEConfig{
		Enabled:      true,
		Email:        "admin@example.com",
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Email != "admin@example.com" {
		t.Errorf("Email = %v, want admin@example.com", cfg.Email)
	}
}

// Helper functions for generating test certificates
func generateTestCertificate(t *testing.T, domain string, notAfter time.Time) *x509.Certificate {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              notAfter,
		DNSNames:              []string{domain},
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	key := generateTestKey(t)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}

func generateTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	return key
}

func certToPEM(t *testing.T, cert *x509.Certificate) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func keyToPEM(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("Failed to marshal key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}
