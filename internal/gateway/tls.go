package gateway

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

const certificateRenewalWindow = 30 * 24 * time.Hour

type certificateIssueFunc func(domain string) (*tls.Certificate, error)

// TLSManager provides dynamic certificate lookup for HTTPS listeners.
type TLSManager struct {
	cfg   config.TLSConfig
	certs sync.Map // map[string]*tls.Certificate
	issue certificateIssueFunc
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
	return nil, fmt.Errorf("acme certificate issuance is not implemented yet for %q", serverName)
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
	_ = populateCertificateLeaf(cert)
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
		_ = populateCertificateLeaf(cert)
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
