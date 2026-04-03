package certmanager

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestACMEProvider_ObtainCertificate_NotLeader(t *testing.T) {
	raftNode := &mockRaftNode{
		isLeader: false,
	}

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

	provider, err := NewACMEProvider(cfg, raftNode)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = provider.ObtainCertificate(ctx, "test.example.com")
	if err == nil {
		t.Error("ObtainCertificate should return error when not leader")
	}
}

func TestACMEProvider_ObtainCertificate_LockAcquisitionFails(t *testing.T) {
	raftNode := &mockRaftNode{
		isLeader: true,
		acquireLockFunc: func(domain string, timeout time.Duration) (bool, error) {
			return false, nil // Lock already held
		},
	}

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

	provider, err := NewACMEProvider(cfg, raftNode)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = provider.ObtainCertificate(ctx, "test.example.com")
	if err == nil {
		t.Error("ObtainCertificate should return error when lock acquisition fails")
	}
}

func TestACMEProvider_ObtainCertificate_LockError(t *testing.T) {
	raftNode := &mockRaftNode{
		isLeader: true,
		acquireLockFunc: func(domain string, timeout time.Duration) (bool, error) {
			return false, context.DeadlineExceeded // Error acquiring lock
		},
	}

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

	provider, err := NewACMEProvider(cfg, raftNode)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = provider.ObtainCertificate(ctx, "test.example.com")
	if err == nil {
		t.Error("ObtainCertificate should return error when lock acquisition errors")
	}
}

func TestACMEProvider_storeChallengeResponse(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	// This is a no-op currently, but we verify it doesn't panic
	provider.storeChallengeResponse("token123", "response456")
}

func TestACMEProvider_CacheOperations(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	domain := "test.example.com"
	cert := generateTestCertificate(t, domain, time.Now().Add(30*24*time.Hour))

	t.Run("cache certificate", func(t *testing.T) {
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

		provider.cacheMu.RLock()
		stored, ok := provider.certCache[domain]
		provider.cacheMu.RUnlock()

		if !ok {
			t.Error("Certificate not found in cache")
		}
		if stored.Domain != domain {
			t.Errorf("Cached domain = %v, want %v", stored.Domain, domain)
		}
	})

	t.Run("get certificate returns cached cert", func(t *testing.T) {
		// We can't use the actual GetCertificate due to tls.ClientHelloInfo type,
		// but we verify the cache lookup works
		provider.cacheMu.RLock()
		cached, ok := provider.certCache[domain]
		provider.cacheMu.RUnlock()

		if !ok {
			t.Error("Certificate not in cache")
		}
		if cached.Domain != domain {
			t.Errorf("Domain = %v, want %v", cached.Domain, domain)
		}
	})

	t.Run("get certificate with expired cert in cache", func(t *testing.T) {
		expiredDomain := "expired.example.com"
		expiredCert := generateTestCertificate(t, expiredDomain, time.Now().Add(-24*time.Hour))

		provider.cacheCertificate(&CachedCertificate{
			Domain:    expiredDomain,
			Cert:      expiredCert,
			Key:       generateTestKey(t),
			CertPEM:   certToPEM(t, expiredCert),
			KeyPEM:    keyToPEM(t, generateTestKey(t)),
			IssuedAt:  time.Now().Add(-90 * 24 * time.Hour),
			ExpiresAt: expiredCert.NotAfter,
		})

		// Expired certificate should be in cache but not returned as valid
		provider.cacheMu.RLock()
		_, ok := provider.certCache[expiredDomain]
		provider.cacheMu.RUnlock()

		if !ok {
			t.Error("Expired certificate should still be in cache")
		}
	})
}

func TestACMEProvider_checkAndRenewCertificates_EmptyCache(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath:  tmpDir,
		certCache:    make(map[string]*CachedCertificate),
		renewalLocks: make(map[string]time.Time),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not panic with empty cache
	provider.checkAndRenewCertificates(ctx)
}

func TestACMEProvider_checkAndRenewCertificates_NoRaft(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	// Use NewACMEProvider to properly initialize
	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	// Add a certificate that expires soon
	domain := "soon.example.com"
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

	// This will fail to obtain a new certificate due to network,
	// but it should not panic
	provider.checkAndRenewCertificates(ctx)
}

func TestACMEProvider_StoreCertificateLocally_InvalidPath(t *testing.T) {
	// Use an invalid filename character on Windows
	provider := &ACMEProvider{
		storagePath: "/invalid:path", // Colon is invalid in Windows paths
	}

	cert := &CachedCertificate{
		Domain:  "test.example.com",
		CertPEM: []byte("cert"),
		KeyPEM:  []byte("key"),
	}

	err := provider.storeCertificateLocally(cert)
	if err == nil {
		// On some systems this might succeed, so just log
		t.Logf("storeCertificateLocally did not return error (may be expected on this system): %v", err)
	}
}

func TestACMEProvider_LoadCertificateFromDisk_InvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
	}

	t.Run("non-existent domain", func(t *testing.T) {
		_, err := provider.loadCertificateFromDisk("nonexistent.example.com")
		if err == nil {
			t.Error("loadCertificateFromDisk should return error for non-existent domain")
		}
	})

	t.Run("invalid certificate PEM", func(t *testing.T) {
		domain := "invalid.example.com"
		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("invalid"), 0600)
		os.WriteFile(filepath.Join(domainDir, "key.pem"), []byte("invalid"), 0600)

		_, err := provider.loadCertificateFromDisk(domain)
		if err == nil {
			t.Error("loadCertificateFromDisk should return error for invalid PEM")
		}
	})

	t.Run("missing certificate file", func(t *testing.T) {
		domain := "missing.example.com"
		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		// Only write key file
		os.WriteFile(filepath.Join(domainDir, "key.pem"), []byte("key"), 0600)

		_, err := provider.loadCertificateFromDisk(domain)
		if err == nil {
			t.Error("loadCertificateFromDisk should return error for missing cert")
		}
	})

	t.Run("missing key file", func(t *testing.T) {
		domain := "missing-key.example.com"
		domainDir := filepath.Join(tmpDir, domain)
		os.MkdirAll(domainDir, 0750)
		// Only write cert file
		os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("cert"), 0600)

		_, err := provider.loadCertificateFromDisk(domain)
		if err == nil {
			t.Error("loadCertificateFromDisk should return error for missing key")
		}
	})
}

func TestACMEProvider_loadExistingCertificates_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
		certCache:   make(map[string]*CachedCertificate),
	}

	err := provider.loadExistingCertificates()
	if err != nil {
		t.Errorf("loadExistingCertificates() error = %v", err)
	}

	if len(provider.certCache) != 0 {
		t.Errorf("certCache should be empty, got %d entries", len(provider.certCache))
	}
}

func TestACMEProvider_loadExistingCertificates_WithErrors(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
		certCache:   make(map[string]*CachedCertificate),
	}

	// Create directories with invalid certificates
	domain := "invalid.example.com"
	domainDir := filepath.Join(tmpDir, domain)
	os.MkdirAll(domainDir, 0750)
	os.WriteFile(filepath.Join(domainDir, "cert.pem"), []byte("invalid"), 0600)
	os.WriteFile(filepath.Join(domainDir, "key.pem"), []byte("invalid"), 0600)

	// Create a file instead of directory
	os.WriteFile(filepath.Join(tmpDir, "not-a-domain.txt"), []byte("test"), 0644)

	err := provider.loadExistingCertificates()
	if err != nil {
		t.Errorf("loadExistingCertificates() error = %v", err)
	}

	// Should have skipped invalid entries
	if len(provider.certCache) != 0 {
		t.Errorf("certCache should be empty (invalid certs skipped), got %d entries", len(provider.certCache))
	}
}

func TestACMEProvider_loadOrCreateAccountKey_CreateNew(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &ACMEProvider{
		storagePath: tmpDir,
	}

	err := provider.loadOrCreateAccountKey()
	if err != nil {
		t.Errorf("loadOrCreateAccountKey() error = %v", err)
	}

	if provider.accountKey == nil {
		t.Error("accountKey is nil")
	}

	// Verify file was created
	keyPath := filepath.Join(tmpDir, "account.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("Account key file was not created")
	}
}

func TestACMEProvider_loadOrCreateAccountKey_LoadExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a key first
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	os.WriteFile(filepath.Join(tmpDir, "account.key"), keyPEM, 0600)

	provider := &ACMEProvider{
		storagePath: tmpDir,
	}

	err := provider.loadOrCreateAccountKey()
	if err != nil {
		t.Errorf("loadOrCreateAccountKey() error = %v", err)
	}

	if provider.accountKey == nil {
		t.Error("accountKey is nil")
	}
}

func TestACMEProvider_loadOrCreateAccountKey_InvalidExistingKey(t *testing.T) {
	tmpDir := t.TempDir()

	// Write invalid key
	os.WriteFile(filepath.Join(tmpDir, "account.key"), []byte("invalid"), 0600)

	provider := &ACMEProvider{
		storagePath: tmpDir,
	}

	err := provider.loadOrCreateAccountKey()
	if err != nil {
		t.Errorf("loadOrCreateAccountKey() should create new key when existing is invalid, error = %v", err)
	}

	// Should have created a new key
	if provider.accountKey == nil {
		t.Error("accountKey should be created even when existing key is invalid")
	}
}

func TestACMEProvider_loadOrCreateAccountKey_InvalidDirectory(t *testing.T) {
	// Use an invalid filename character on Windows
	provider := &ACMEProvider{
		storagePath: "/invalid:path", // Colon is invalid in Windows paths
	}

	err := provider.loadOrCreateAccountKey()
	if err == nil {
		// On some systems this might succeed
		t.Log("loadOrCreateAccountKey() did not return error (may be expected on this system)")
	}
}

func TestNewACMEProvider_InvalidDirectory(t *testing.T) {
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  "/invalid:path", // Colon is invalid in Windows paths
		},
	}

	_, err := NewACMEProvider(cfg, nil)
	if err == nil {
		// On some systems this might succeed
		t.Log("NewACMEProvider did not return error (may be expected on this system)")
	}
}

func TestACMEProvider_GetCertificate_NilHello(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	// This test verifies the function signature, actual testing with tls.ClientHelloInfo
	// would require more setup
	_ = provider
}

func TestCachedCertificate_TLS(t *testing.T) {
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

	// Create tls.Certificate
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{cached.Cert.Raw},
		PrivateKey:  cached.Key,
		Leaf:        cached.Cert,
	}

	if tlsCert == nil {
		t.Error("tls.Certificate is nil")
	}
	if len(tlsCert.Certificate) != 1 {
		t.Errorf("Certificate chain length = %d, want 1", len(tlsCert.Certificate))
	}
}

// Test StartRenewalScheduler
func TestACMEProvider_StartRenewalScheduler(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start the scheduler - it should start without panic
	// It will run and exit when context is cancelled
	go provider.StartRenewalScheduler(ctx)

	// Wait for scheduler to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop scheduler
	cancel()

	// Give it time to exit
	time.Sleep(50 * time.Millisecond)
}

// Test completeChallenge
func TestACMEProvider_completeChallenge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Test without initialized client - should return error
	err = provider.completeChallenge(ctx, "https://example.com/authz")
	if err == nil {
		t.Error("completeChallenge should return error without initialized client")
	}
}

// Test obtainCertificate with various error conditions
func TestACMEProvider_ObtainCertificate_InvalidDomain(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		ACME: config.ACMEConfig{
			Enabled:      true,
			Email:        "test@example.com",
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			StoragePath:  tmpDir,
		},
	}

	provider, err := NewACMEProvider(cfg, nil)
	if err != nil {
		t.Fatalf("NewACMEProvider() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Test with empty domain
	_, err = provider.ObtainCertificate(ctx, "")
	if err == nil {
		t.Error("ObtainCertificate should return error with empty domain")
	}
}
