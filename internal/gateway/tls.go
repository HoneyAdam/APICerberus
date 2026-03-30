package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TLSManager provides dynamic certificate lookup for HTTPS listeners.
type TLSManager struct {
	cfg   config.TLSConfig
	certs sync.Map // map[string]*tls.Certificate
}

// NewTLSManager prepares certificate sources from configuration.
func NewTLSManager(cfg config.TLSConfig) (*TLSManager, error) {
	manager := &TLSManager{cfg: cfg}

	certFile := strings.TrimSpace(cfg.CertFile)
	keyFile := strings.TrimSpace(cfg.KeyFile)
	if certFile == "" && keyFile == "" {
		return manager, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, errors.New("tls cert_file and key_file must both be provided")
	}

	cert, err := loadCertificatePair(certFile, keyFile)
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
		if cert := tm.cached(serverName); cert != nil && certificateIsValidNow(cert) {
			return cert, nil
		}
	}
	if cert := tm.cached("*"); cert != nil && certificateIsValidNow(cert) {
		return cert, nil
	}

	if serverName != "" {
		cert, err := tm.loadFromDisk(serverName)
		if err == nil {
			tm.certs.Store(serverName, cert)
			return cert, nil
		}
	}

	if tm.cfg.Auto {
		return nil, fmt.Errorf("acme certificate issuance is not implemented yet for %q", serverName)
	}
	if serverName == "" {
		return nil, errors.New("no server name provided and no default tls certificate configured")
	}
	return nil, fmt.Errorf("no tls certificate configured for %q", serverName)
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
