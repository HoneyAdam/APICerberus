package gateway

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const certificateRenewalWindow = 30 * 24 * time.Hour

type certificateIssueFunc func(domain string) (*tls.Certificate, error)

// TLSManager provides dynamic certificate lookup for HTTPS listeners.
type TLSManager struct {
	cfg       config.TLSConfig
	certs     sync.Map // map[string]*tls.Certificate
	issue     certificateIssueFunc
	autocertM *autocert.Manager
}

// NewTLSManager prepares certificate sources from configuration.
func NewTLSManager(cfg config.TLSConfig) (*TLSManager, error) {
	cfg.ACMEDir = strings.TrimSpace(cfg.ACMEDir)
	cfg.CertFile = strings.TrimSpace(cfg.CertFile)
	cfg.KeyFile = strings.TrimSpace(cfg.KeyFile)

	manager := &TLSManager{cfg: cfg}
	manager.issue = manager.issueCertificate

	if cfg.Auto && cfg.ACMEDir != "" {
		if err := os.MkdirAll(cfg.ACMEDir, 0o700); err != nil {
			return nil, fmt.Errorf("prepare acme_dir: %w", err)
		}
		manager.autocertM = &autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Cache:  autocert.DirCache(cfg.ACMEDir),
			Email:  cfg.ACMEEmail,
		}
	}

	if cfg.CertFile == "" && cfg.KeyFile == "" {
		return manager, nil
	}
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, errors.New("tls cert_file and key_file must both be provided")
	}

	cert, err := loadCertificatePair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load manual tls certificate: %w", err)
	}
	manager.certs.Store("*", cert)
	return manager, nil
}

func (tm *TLSManager) TLSConfig() *tls.Config {
	nextProtos := []string{"h2", "http/1.1"}
	if tm != nil && tm.cfg.Auto {
		nextProtos = append(nextProtos, acme.ALPNProto)
	}
	var minVersion uint16 = tls.VersionTLS12
	if tm != nil {
		minVersion = parseTLSMinVersion(tm.cfg.MinVersion)
	}
	cfg := &tls.Config{
		MinVersion:     minVersion,
		GetCertificate: tm.GetCertificate,
		NextProtos:     nextProtos,
	}
	if tm != nil && len(tm.cfg.CipherSuites) > 0 {
		cfg.CipherSuites = parseTLSCipherSuites(tm.cfg.CipherSuites)
	} else if minVersion < tls.VersionTLS13 {
		// Safe modern defaults for TLS 1.2
		cfg.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		}
	}
	return cfg
}

func parseTLSMinVersion(v string) uint16 {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

func parseTLSCipherSuites(names []string) []uint16 {
	if len(names) == 0 {
		return nil
	}
	cipherMap := make(map[string]uint16)
	for _, cs := range tls.CipherSuites() {
		cipherMap[strings.ToLower(cs.Name)] = cs.ID
	}
	for name, id := range map[string]uint16{
		"TLS_RSA_WITH_AES_128_CBC_SHA":    tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		"TLS_RSA_WITH_AES_256_CBC_SHA":    tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		"TLS_RSA_WITH_AES_128_GCM_SHA256": tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_RSA_WITH_AES_256_GCM_SHA384": tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	} {
		cipherMap[strings.ToLower(name)] = id
	}
	var out []uint16
	for _, name := range names {
		if id, ok := cipherMap[strings.ToLower(strings.TrimSpace(name))]; ok {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetCertificate is used as tls.Config.GetCertificate callback.
func (tm *TLSManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if tm == nil {
		return nil, errors.New("tls manager is nil")
	}

	serverName := ""
	if hello != nil {
		serverName = strings.ToLower(strings.TrimSpace(hello.ServerName))
	}

	if serverName != "" {
		if cert := tm.cached(serverName); cert != nil {
			return tm.evaluateCertificate(serverName, cert)
		}
	}
	if cert := tm.cached("*"); cert != nil {
		return tm.evaluateCertificate(serverName, cert)
	}

	if serverName != "" {
		cert, err := tm.loadFromDisk(serverName)
		if err == nil {
			tm.certs.Store(serverName, cert)
			return tm.evaluateCertificate(serverName, cert)
		}
	}

	if tm.cfg.Auto {
		return tm.issueAndStore(serverName)
	}
	if serverName == "" {
		return nil, errors.New("no server name provided and no default tls certificate configured")
	}
	return nil, fmt.Errorf("no tls certificate configured for %q", serverName)
}

func (tm *TLSManager) evaluateCertificate(serverName string, cert *tls.Certificate) (*tls.Certificate, error) {
	if !certificateIsValidNow(cert) {
		if tm.cfg.Auto {
			return tm.issueAndStore(serverName)
		}
		return nil, fmt.Errorf("tls certificate for %q is expired", serverName)
	}
	if !certificateNeedsRenewal(cert, certificateRenewalWindow) {
		return cert, nil
	}
	if !tm.cfg.Auto || strings.TrimSpace(serverName) == "" {
		return cert, nil
	}

	renewed, err := tm.issueAndStore(serverName)
	if err != nil {
		// Keep serving current valid cert even if renewal attempt failed.
		return cert, nil
	}
	return renewed, nil
}

func (tm *TLSManager) issueAndStore(serverName string) (*tls.Certificate, error) {
	serverName = strings.ToLower(strings.TrimSpace(serverName))
	if serverName == "" {
		return nil, errors.New("server_name is required for automatic certificate issuance")
	}
	if tm.issue == nil {
		return nil, errors.New("certificate issuer is not configured")
	}
	cert, err := tm.issue(serverName)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, errors.New("certificate issuer returned nil certificate")
	}
	if err := populateCertificateLeaf(cert); err != nil {
		return nil, fmt.Errorf("parse issued certificate: %w", err)
	}
	if err := tm.saveToDisk(serverName, cert); err != nil {
		return nil, fmt.Errorf("persist issued certificate: %w", err)
	}
	tm.certs.Store(serverName, cert)
	return cert, nil
}

func (tm *TLSManager) issueCertificate(serverName string) (*tls.Certificate, error) {
	if tm.autocertM == nil {
		return nil, fmt.Errorf("acme certificate manager is not configured for %q", serverName)
	}
	return tm.autocertM.GetCertificate(&tls.ClientHelloInfo{
		ServerName: serverName,
	})
}

func (tm *TLSManager) cached(name string) *tls.Certificate {
	value, ok := tm.certs.Load(name)
	if !ok {
		return nil
	}
	cert, ok := value.(*tls.Certificate)
	if !ok {
		return nil
	}
	return cert
}

// ReloadCertificate reloads a certificate from disk and updates the cache.
// This is called when a certificate is updated via Raft replication.
func (tm *TLSManager) ReloadCertificate(serverName string) error {
	serverName = strings.ToLower(strings.TrimSpace(serverName))
	if serverName == "" {
		return errors.New("server_name is required")
	}

	cert, err := tm.loadFromDisk(serverName)
	if err != nil {
		return fmt.Errorf("failed to load certificate from disk: %w", err)
	}

	tm.certs.Store(serverName, cert)
	return nil
}

// LoadAllCertificatesFromDisk loads all certificates from disk into cache.
// Useful when joining a cluster and certificates were synced via Raft.
func (tm *TLSManager) LoadAllCertificatesFromDisk() error {
	acmeDir := strings.TrimSpace(tm.cfg.ACMEDir)
	if acmeDir == "" {
		return nil // Nothing to load
	}

	entries, err := os.ReadDir(acmeDir)
	if err != nil {
		return fmt.Errorf("failed to read acme directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		serverName := entry.Name()
		if cert, err := tm.loadFromDisk(serverName); err == nil {
			tm.certs.Store(serverName, cert)
		}
	}

	return nil
}

func (tm *TLSManager) loadFromDisk(serverName string) (*tls.Certificate, error) {
	acmeDir := strings.TrimSpace(tm.cfg.ACMEDir)
	if acmeDir == "" {
		return nil, errors.New("acme_dir is not configured")
	}
	certFile := filepath.Join(acmeDir, serverName+".crt")
	keyFile := filepath.Join(acmeDir, serverName+".key")
	if _, err := os.Stat(certFile); err != nil {
		return nil, err
	}
	if _, err := os.Stat(keyFile); err != nil {
		return nil, err
	}
	return loadCertificatePair(certFile, keyFile)
}

func (tm *TLSManager) saveToDisk(serverName string, cert *tls.Certificate) error {
	acmeDir := strings.TrimSpace(tm.cfg.ACMEDir)
	if acmeDir == "" {
		return errors.New("acme_dir is not configured")
	}
	if cert == nil {
		return errors.New("certificate is nil")
	}
	if len(cert.Certificate) == 0 {
		return errors.New("certificate chain is empty")
	}
	if cert.PrivateKey == nil {
		return errors.New("certificate private key is missing")
	}
	if err := os.MkdirAll(acmeDir, 0o700); err != nil {
		return err
	}

	var certPEM strings.Builder
	for _, der := range cert.Certificate {
		block := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		}
		certPEM.Write(pem.EncodeToMemory(block))
	}
	keyPEM, err := encodePrivateKeyPEM(cert.PrivateKey)
	if err != nil {
		return err
	}

	certPath := filepath.Join(acmeDir, serverName+".crt")
	keyPath := filepath.Join(acmeDir, serverName+".key")
	if err := writeFileAtomic(certPath, []byte(certPEM.String()), 0o600); err != nil {
		return err
	}
	if err := writeFileAtomic(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

func encodePrivateKeyPEM(privateKey any) ([]byte, error) {
	switch key := privateKey.(type) {
	case *rsa.PrivateKey:
		block := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		}
		return pem.EncodeToMemory(block), nil
	case *ecdsa.PrivateKey:
		raw, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
		block := &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: raw,
		}
		return pem.EncodeToMemory(block), nil
	default:
		raw, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		block := &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: raw,
		}
		return pem.EncodeToMemory(block), nil
	}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
		return err
	}
	if err := tempFile.Chmod(perm); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
		return err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}

func loadCertificatePair(certFile, keyFile string) (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cert := &pair
	if err := populateCertificateLeaf(cert); err != nil {
		return nil, fmt.Errorf("parse certificate leaf: %w", err)
	}
	return cert, nil
}

func populateCertificateLeaf(cert *tls.Certificate) error {
	if cert == nil || cert.Leaf != nil || len(cert.Certificate) == 0 {
		return nil
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}
	cert.Leaf = leaf
	return nil
}

func certificateIsValidNow(cert *tls.Certificate) bool {
	if cert == nil {
		return false
	}
	if cert.Leaf == nil {
		if err := populateCertificateLeaf(cert); err != nil {
			log.Printf("[WARN] tls: failed to parse certificate leaf for validity check: %v", err)
			return false
		}
	}
	if cert.Leaf == nil {
		return true
	}
	return time.Now().Before(cert.Leaf.NotAfter)
}

func certificateNeedsRenewal(cert *tls.Certificate, window time.Duration) bool {
	if cert == nil {
		return true
	}
	if cert.Leaf == nil {
		_ = populateCertificateLeaf(cert)
	}
	if cert.Leaf == nil {
		return false
	}
	if window <= 0 {
		window = certificateRenewalWindow
	}
	return time.Now().Add(window).After(cert.Leaf.NotAfter)
}
