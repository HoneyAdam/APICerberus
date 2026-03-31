package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CertificateManager interface for TLS certificate operations
type CertificateManager interface {
	ReloadCertificate(serverName string) error
}

// CertLogEntryType represents the type of certificate log entry
type CertLogEntryType int

const (
	CertLogEntryCertificateUpdate CertLogEntryType = iota // Certificate update
	CertLogEntryACMERenewalLock                           // ACME renewal lock
)

// CertLogEntry is the data stored in each certificate-related Raft log
type CertLogEntry struct {
	Type CertLogEntryType `json:"type"`
	Data []byte           `json:"data"`
}

// CertificateUpdateLog represents a certificate update in Raft log
type CertificateUpdateLog struct {
	Domain    string    `json:"domain"`
	CertPEM   string    `json:"cert_pem"`
	KeyPEM    string    `json:"key_pem"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IssuedBy  string    `json:"issued_by"` // Node ID that issued the cert
}

// ACMERenewalLock represents a lock for ACME renewal
type ACMERenewalLock struct {
	Domain   string    `json:"domain"`
	NodeID   string    `json:"node_id"`
	Deadline time.Time `json:"deadline"` // Lock expires after this time
}

// CertificateState holds certificate data in FSM
type CertificateState struct {
	Domain     string    `json:"domain"`
	CertPEM    string    `json:"cert_pem"`
	KeyPEM     string    `json:"key_pem"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	IssuedBy   string    `json:"issued_by"`
}

// CertFSM extends GatewayFSM with certificate-specific state
type CertFSM struct {
	// Certificates map (domain -> certificate state)
	Certificates map[string]*CertificateState `json:"certificates"`

	// Renewal locks (domain -> lock)
	RenewalLocks map[string]*ACMERenewalLock `json:"-"`

	// Storage path for certificates
	StoragePath string `json:"-"`

	// TLS Manager for hot reload
	tlsManager CertificateManager

	// Logger
	logger *log.Logger

	// Mutex for locks
	lockMu sync.RWMutex
}

// NewCertFSM creates a new certificate FSM
func NewCertFSM(storagePath string, tlsManager CertificateManager) *CertFSM {
	return &CertFSM{
		Certificates: make(map[string]*CertificateState),
		RenewalLocks: make(map[string]*ACMERenewalLock),
		StoragePath:  storagePath,
		tlsManager:   tlsManager,
		logger:       log.New(log.Writer(), "[cert-fsm] ", log.LstdFlags),
	}
}

// SetTLSManager sets the TLS manager for certificate reload
func (c *CertFSM) SetTLSManager(tm CertificateManager) {
	c.tlsManager = tm
}

// ApplyCertCommand applies a certificate-related command
func (c *CertFSM) ApplyCertCommand(cmdType string, data []byte) error {
	switch cmdType {
	case "certificate_update":
		return c.applyCertificateUpdate(data)
	case "acme_renewal_lock":
		return c.applyACMERenewalLock(data)
	default:
		return fmt.Errorf("unknown cert command type: %s", cmdType)
	}
}

// applyCertificateUpdate applies certificate update to FSM
func (c *CertFSM) applyCertificateUpdate(data []byte) error {
	var update CertificateUpdateLog
	if err := json.Unmarshal(data, &update); err != nil {
		return fmt.Errorf("failed to unmarshal certificate update: %w", err)
	}

	// Validate certificate data
	if update.Domain == "" || update.CertPEM == "" || update.KeyPEM == "" {
		return fmt.Errorf("invalid certificate data: missing required fields")
	}

	// Store in FSM state
	c.Certificates[update.Domain] = &CertificateState{
		Domain:    update.Domain,
		CertPEM:   update.CertPEM,
		KeyPEM:    update.KeyPEM,
		IssuedAt:  update.IssuedAt,
		ExpiresAt: update.ExpiresAt,
		IssuedBy:  update.IssuedBy,
	}

	// Write to local disk
	if err := c.writeCertificateToDisk(&update); err != nil {
		c.logger.Printf("[ERROR] failed to write certificate to disk: %v", err)
		// Continue - memory state is updated
	}

	// Notify TLS manager about certificate update
	if c.tlsManager != nil {
		if err := c.tlsManager.ReloadCertificate(update.Domain); err != nil {
			c.logger.Printf("[WARN] failed to reload certificate in TLS manager: %v", err)
		}
	}

	c.logger.Printf("[INFO] certificate updated for %s (issued by %s, expires %s)",
		update.Domain, update.IssuedBy, update.ExpiresAt.Format(time.RFC3339))

	return nil
}

// applyACMERenewalLock applies renewal lock to FSM
func (c *CertFSM) applyACMERenewalLock(data []byte) error {
	var lock ACMERenewalLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("failed to unmarshal renewal lock: %w", err)
	}

	// Store lock in memory (with TTL)
	c.lockMu.Lock()
	defer c.lockMu.Unlock()

	// Check if there's an existing lock
	if existing, ok := c.RenewalLocks[lock.Domain]; ok {
		if time.Now().Before(existing.Deadline) {
			return fmt.Errorf("renewal lock already held by %s until %s",
				existing.NodeID, existing.Deadline.Format(time.RFC3339))
		}
	}

	c.RenewalLocks[lock.Domain] = &lock
	c.logger.Printf("[INFO] ACME renewal lock acquired for %s by %s (deadline: %s)",
		lock.Domain, lock.NodeID, lock.Deadline.Format(time.RFC3339))

	return nil
}

// writeCertificateToDisk writes certificate to local storage
func (c *CertFSM) writeCertificateToDisk(update *CertificateUpdateLog) error {
	if c.StoragePath == "" {
		return fmt.Errorf("storage path not configured")
	}

	// Create domain directory
	domainDir := filepath.Join(c.StoragePath, update.Domain)
	if err := os.MkdirAll(domainDir, 0755); err != nil {
		return fmt.Errorf("failed to create domain directory: %w", err)
	}

	// Atomic write certificate
	certPath := filepath.Join(domainDir, "cert.pem")
	if err := atomicWriteFile(certPath, []byte(update.CertPEM)); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Atomic write private key
	keyPath := filepath.Join(domainDir, "key.pem")
	if err := atomicWriteFile(keyPath, []byte(update.KeyPEM)); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Write metadata
	metaPath := filepath.Join(domainDir, "meta.json")
	meta := map[string]interface{}{
		"domain":     update.Domain,
		"issued_at":  update.IssuedAt,
		"expires_at": update.ExpiresAt,
		"issued_by":  update.IssuedBy,
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := atomicWriteFile(metaPath, metaData); err != nil {
		c.logger.Printf("[WARN] failed to write certificate metadata: %v", err)
		// Non-fatal, continue
	}

	return nil
}

// GetCertificate returns certificate from FSM state
func (c *CertFSM) GetCertificate(domain string) (*CertificateState, bool) {
	cert, ok := c.Certificates[domain]
	return cert, ok
}

// GetCertificateFromDisk reads certificate from local disk
func (c *CertFSM) GetCertificateFromDisk(domain string) (*CertificateUpdateLog, error) {
	if c.StoragePath == "" {
		return nil, fmt.Errorf("storage path not configured")
	}

	domainDir := filepath.Join(c.StoragePath, domain)

	certPath := filepath.Join(domainDir, "cert.pem")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	keyPath := filepath.Join(domainDir, "key.pem")
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	// Read metadata if available
	metaPath := filepath.Join(domainDir, "meta.json")
	meta := make(map[string]interface{})
	if metaData, err := os.ReadFile(metaPath); err == nil {
		json.Unmarshal(metaData, &meta)
	}

	result := &CertificateUpdateLog{
		Domain:  domain,
		CertPEM: string(certPEM),
		KeyPEM:  string(keyPEM),
	}

	if issuedAt, ok := meta["issued_at"].(string); ok {
		result.IssuedAt, _ = time.Parse(time.RFC3339, issuedAt)
	}
	if expiresAt, ok := meta["expires_at"].(string); ok {
		result.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	}
	if issuedBy, ok := meta["issued_by"].(string); ok {
		result.IssuedBy = issuedBy
	}

	return result, nil
}

// LoadCertificatesFromDisk loads all certificates from disk into FSM
func (c *CertFSM) LoadCertificatesFromDisk() error {
	if c.StoragePath == "" {
		return nil
	}

	entries, err := os.ReadDir(c.StoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cert, err := c.GetCertificateFromDisk(entry.Name())
		if err != nil {
			c.logger.Printf("[WARN] failed to load certificate for %s: %v", entry.Name(), err)
			continue
		}
		c.Certificates[cert.Domain] = &CertificateState{
			Domain:    cert.Domain,
			CertPEM:   cert.CertPEM,
			KeyPEM:    cert.KeyPEM,
			IssuedAt:  cert.IssuedAt,
			ExpiresAt: cert.ExpiresAt,
			IssuedBy:  cert.IssuedBy,
		}
		c.logger.Printf("[INFO] loaded certificate for %s from disk", cert.Domain)
	}

	return nil
}

// atomicWriteFile writes data to a temporary file and renames it atomically
func atomicWriteFile(path string, data []byte) error {
	// Create temp file in same directory
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-cert-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	tmpFile.Close()

	// Set permissions (readable only by owner)
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

// ProposeCertificateUpdate proposes a certificate update to the Raft cluster
func (n *Node) ProposeCertificateUpdate(domain, certPEM, keyPEM string, expiresAt time.Time) error {
	// Must be leader to propose
	if !n.IsLeader() {
		return fmt.Errorf("not the leader, cannot propose certificate update")
	}

	update := CertificateUpdateLog{
		Domain:    domain,
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: expiresAt,
		IssuedBy:  n.ID,
	}

	data, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal certificate update: %w", err)
	}

	// Create FSM command
	cmd := FSMCommand{
		Type:    "certificate_update",
		Payload: data,
	}

	// Append to Raft log
	_, err = n.AppendEntry(cmd)
	if err != nil {
		return fmt.Errorf("failed to append certificate update to log: %w", err)
	}

	return nil
}

// AcquireACMERenewalLock tries to acquire a lock for ACME renewal
func (n *Node) AcquireACMERenewalLock(domain string, timeout time.Duration) (bool, error) {
	// Must be leader to propose lock
	if !n.IsLeader() {
		return false, fmt.Errorf("not the leader, cannot acquire renewal lock")
	}

	lock := ACMERenewalLock{
		Domain:   domain,
		NodeID:   n.ID,
		Deadline: time.Now().Add(timeout),
	}

	data, err := json.Marshal(lock)
	if err != nil {
		return false, err
	}

	// Create FSM command
	cmd := FSMCommand{
		Type:    "acme_renewal_lock",
		Payload: data,
	}

	// Append to Raft log
	_, err = n.AppendEntry(cmd)
	if err != nil {
		return false, err // Lock already held or other error
	}

	return true, nil
}

// RaftNode interface for certificate manager
type RaftNode interface {
	ProposeCertificateUpdate(domain, certPEM, keyPEM string, expiresAt time.Time) error
	AcquireACMERenewalLock(domain string, timeout time.Duration) (bool, error)
	IsLeader() bool
	GetNodeID() string
}

// GetNodeID returns the node ID
func (n *Node) GetNodeID() string {
	return n.ID
}

// Ensure Node implements RaftNode interface
var _ RaftNode = (*Node)(nil)
