package certmanager

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"golang.org/x/crypto/acme"
)

// ACMEProvider implements automatic certificate management using ACME/Let's Encrypt
type ACMEProvider struct {
	mu          sync.RWMutex
	client      *acme.Client
	accountKey  crypto.Signer
	account     *acme.Account
	directoryURL string
	email        string
	storagePath  string

	// Raft integration
	raftNode     RaftNode
	useRaftSync  bool

	// Certificate cache
	certCache    map[string]*CachedCertificate
	cacheMu      sync.RWMutex

	// Renewal management
	renewalMu    sync.Mutex
	renewalLocks map[string]time.Time
}

// CachedCertificate holds a certificate with metadata
type CachedCertificate struct {
	Domain    string
	Cert      *x509.Certificate
	Key       crypto.PrivateKey
	CertPEM   []byte
	KeyPEM    []byte
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// RaftNode interface for certificate replication
type RaftNode interface {
	ProposeCertificateUpdate(domain, certPEM, keyPEM string, expiresAt time.Time) error
	AcquireACMERenewalLock(domain string, timeout time.Duration) (bool, error)
	IsLeader() bool
	NodeID() string
}

// Config holds ACME configuration
type Config struct {
	Enabled       bool
	Email         string
	DirectoryURL  string
	StoragePath   string
	UseRaftSync   bool
}

// NewACMEProvider creates a new ACME certificate provider
func NewACMEProvider(cfg *config.Config, raftNode RaftNode) (*ACMEProvider, error) {
	if cfg == nil || !cfg.ACME.Enabled {
		return nil, fmt.Errorf("ACME not enabled")
	}

	provider := &ACMEProvider{
		directoryURL: cfg.ACME.DirectoryURL,
		email:        cfg.ACME.Email,
		storagePath:  cfg.ACME.StoragePath,
		raftNode:     raftNode,
		useRaftSync:  cfg.Cluster.CertificateSync.Enabled && cfg.Cluster.CertificateSync.RaftReplication,
		certCache:    make(map[string]*CachedCertificate),
		renewalLocks: make(map[string]time.Time),
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(provider.storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Load or create account key
	if err := provider.loadOrCreateAccountKey(); err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}

	// Initialize ACME client
	provider.client = &acme.Client{
		Key:          provider.accountKey,
		DirectoryURL: provider.directoryURL,
		UserAgent:    "APICerebrus/1.0",
	}

	// Load existing certificates
	if err := provider.loadExistingCertificates(); err != nil {
		log.Printf("[WARN] acme: failed to load existing certificates: %v", err)
	}

	return provider, nil
}

// GetCertificate returns a certificate for the given domain
// Implements tls.Config.GetCertificate
func (p *ACMEProvider) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := strings.ToLower(hello.ServerName)
	if domain == "" {
		return nil, fmt.Errorf("no SNI provided")
	}

	// Check cache first
	p.cacheMu.RLock()
	cached, ok := p.certCache[domain]
	p.cacheMu.RUnlock()

	if ok && time.Now().Before(cached.ExpiresAt.Add(-24*time.Hour)) {
		// Certificate is valid and not expiring soon
		return &tls.Certificate{
			Certificate: [][]byte{cached.Cert.Raw},
			PrivateKey:  cached.Key,
			Leaf:        cached.Cert,
		}, nil
	}

	// Check disk (in case it was synced from Raft)
	if cert, err := p.loadCertificateFromDisk(domain); err == nil {
		p.cacheCertificate(cert)
		return &tls.Certificate{
			Certificate: [][]byte{cert.Cert.Raw},
			PrivateKey:  cert.Key,
			Leaf:        cert.Cert,
		}, nil
	}

	// Need to obtain certificate
	return nil, fmt.Errorf("certificate not available for %s", domain)
}

// ObtainCertificate obtains a new certificate for the given domain
func (p *ACMEProvider) ObtainCertificate(ctx context.Context, domain string) (*CachedCertificate, error) {
	// Check if we can get lock for renewal
	if p.useRaftSync && p.raftNode != nil {
		if !p.raftNode.IsLeader() {
			return nil, fmt.Errorf("not the leader, cannot obtain certificate")
		}

		// Try to acquire lock
		locked, err := p.raftNode.AcquireACMERenewalLock(domain, 5*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire renewal lock: %w", err)
		}
		if !locked {
			return nil, fmt.Errorf("renewal already in progress")
		}
	}

	// Generate CSR
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate key: %w", err)
	}

	template := &x509.CertificateRequest{
		DNSNames: []string{domain},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, certKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	// Request certificate from ACME
	order, err := p.client.AuthorizeOrder(ctx, []acme.AuthzID{{Type: "dns", Value: domain}})
	if err != nil {
		return nil, fmt.Errorf("failed to authorize order: %w", err)
	}

	// Complete challenges
	for _, authzURL := range order.AuthzURLs {
		if err := p.completeChallenge(ctx, authzURL); err != nil {
			return nil, fmt.Errorf("failed to complete challenge: %w", err)
		}
	}

	// Finalize order
	order, err = p.client.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for order: %w", err)
	}

	// Create certificate
	certChains, _, err := p.client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse certificate
	if len(certChains) == 0 || len(certChains[0]) == 0 {
		return nil, fmt.Errorf("no certificate received")
	}
	cert, err := x509.ParseCertificate(certChains[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyDER, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	result := &CachedCertificate{
		Domain:    domain,
		Cert:      cert,
		Key:       certKey,
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		IssuedAt:  time.Now(),
		ExpiresAt: cert.NotAfter,
	}

	// Sync via Raft if enabled
	if p.useRaftSync && p.raftNode != nil {
		if err := p.raftNode.ProposeCertificateUpdate(domain, string(certPEM), string(keyPEM), cert.NotAfter); err != nil {
			return nil, fmt.Errorf("failed to replicate certificate: %w", err)
		}
		log.Printf("[INFO] acme: certificate for %s replicated via Raft", domain)
	} else {
		// Store locally only
		if err := p.storeCertificateLocally(result); err != nil {
			return nil, fmt.Errorf("failed to store certificate: %w", err)
		}
	}

	// Cache the certificate
	p.cacheCertificate(result)

	return result, nil
}

// completeChallenge completes an ACME challenge
func (p *ACMEProvider) completeChallenge(ctx context.Context, authzURL string) error {
	authz, err := p.client.GetAuthorization(ctx, authzURL)
	if err != nil {
		return err
	}

	if authz.Status == acme.StatusValid {
		return nil // Already authorized
	}

	// Find available challenge
	var challenge *acme.Challenge
	for _, c := range authz.Challenges {
		if c.Type == "http-01" {
			challenge = c
			break
		}
	}
	if challenge == nil {
		return fmt.Errorf("no supported challenge found")
	}

	// Prepare challenge response
	response, err := p.client.HTTP01ChallengeResponse(challenge.Token)
	if err != nil {
		return err
	}

	// Store challenge response (this should be served by the gateway HTTP handler)
	p.storeChallengeResponse(challenge.Token, response)

	// Accept challenge
	if _, err := p.client.Accept(ctx, challenge); err != nil {
		return err
	}

	// Wait for authorization
	_, err = p.client.WaitAuthorization(ctx, authz.URI)
	return err
}

// storeChallengeResponse stores the ACME challenge response
func (p *ACMEProvider) storeChallengeResponse(token, response string) {
	// This will be served by the gateway's HTTP handler at /.well-known/acme-challenge/
	// Implementation depends on your HTTP handler setup
}

// storeCertificateLocally stores certificate to local disk
func (p *ACMEProvider) storeCertificateLocally(cert *CachedCertificate) error {
	domainDir := filepath.Join(p.storagePath, cert.Domain)
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		return err
	}

	certPath := filepath.Join(domainDir, "cert.pem")
	if err := os.WriteFile(certPath, cert.CertPEM, 0600); err != nil {
		return err
	}

	keyPath := filepath.Join(domainDir, "key.pem")
	if err := os.WriteFile(keyPath, cert.KeyPEM, 0600); err != nil {
		return err
	}

	return nil
}

// loadCertificateFromDisk loads certificate from local disk
func (p *ACMEProvider) loadCertificateFromDisk(domain string) (*CachedCertificate, error) {
	domainDir := filepath.Join(p.storagePath, domain)

	certPath := filepath.Join(domainDir, "cert.pem")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	keyPath := filepath.Join(domainDir, "key.pem")
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	// Parse certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Parse key
	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &CachedCertificate{
		Domain:    domain,
		Cert:      cert,
		Key:       key,
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		IssuedAt:  cert.NotBefore,
		ExpiresAt: cert.NotAfter,
	}, nil
}

// cacheCertificate caches a certificate in memory
func (p *ACMEProvider) cacheCertificate(cert *CachedCertificate) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	p.certCache[cert.Domain] = cert
}

// loadExistingCertificates loads all existing certificates from disk
func (p *ACMEProvider) loadExistingCertificates() error {
	entries, err := os.ReadDir(p.storagePath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if cert, err := p.loadCertificateFromDisk(entry.Name()); err == nil {
			p.cacheCertificate(cert)
			log.Printf("[INFO] acme: loaded certificate for %s (expires: %s)",
				cert.Domain, cert.ExpiresAt.Format(time.RFC3339))
		}
	}

	return nil
}

// loadOrCreateAccountKey loads or creates the ACME account key
func (p *ACMEProvider) loadOrCreateAccountKey() error {
	keyPath := filepath.Join(p.storagePath, "account.key")

	// Try to load existing key
	if keyData, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(keyData)
		if block != nil {
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err == nil {
				p.accountKey = key
				return nil
			}
		}
	}

	// Create new key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// Store key
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return err
	}

	p.accountKey = key
	return nil
}

// StartRenewalScheduler starts the certificate renewal scheduler
func (p *ACMEProvider) StartRenewalScheduler(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run immediately on start
	p.checkAndRenewCertificates(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkAndRenewCertificates(ctx)
		}
	}
}

// checkAndRenewCertificates checks all certificates and renews if needed
func (p *ACMEProvider) checkAndRenewCertificates(ctx context.Context) {
	if p.useRaftSync && p.raftNode != nil && !p.raftNode.IsLeader() {
		// Only leader handles renewals in Raft mode
		return
	}

	p.cacheMu.RLock()
	certs := make([]*CachedCertificate, 0, len(p.certCache))
	for _, cert := range p.certCache {
		certs = append(certs, cert)
	}
	p.cacheMu.RUnlock()

	for _, cert := range certs {
		// Renew if expires within 30 days
		if time.Until(cert.ExpiresAt) < 30*24*time.Hour {
			log.Printf("[INFO] acme: renewing certificate for %s (expires: %s)",
				cert.Domain, cert.ExpiresAt.Format(time.RFC3339))

			if _, err := p.ObtainCertificate(ctx, cert.Domain); err != nil {
				log.Printf("[ERROR] acme: failed to renew certificate for %s: %v", cert.Domain, err)
			}
		}
	}
}
