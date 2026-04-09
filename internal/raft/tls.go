package raft

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// TLSCertificateManager manages certificates for Raft mTLS.
type TLSCertificateManager struct {
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	nodeCert  *tls.Certificate
	nodeID    string
	clusterID string
}

// NewTLSCertificateManager creates a new certificate manager.
func NewTLSCertificateManager(nodeID, clusterID string) (*TLSCertificateManager, error) {
	return &TLSCertificateManager{
		nodeID:    nodeID,
		clusterID: clusterID,
	}, nil
}

// GenerateCA generates a new CA certificate and key.
func (m *TLSCertificateManager) GenerateCA() error {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate CA key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"APICerebrus Raft Cluster"},
			CommonName:   fmt.Sprintf("%s CA", m.clusterID),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	m.caCert = cert
	m.caKey = key
	return nil
}

// GenerateNodeCertificate generates a certificate for this node.
func (m *TLSCertificateManager) GenerateNodeCertificate() error {
	if m.caCert == nil || m.caKey == nil {
		return fmt.Errorf("CA not initialized, call GenerateCA first")
	}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate node key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"APICerebrus Raft Cluster"},
			CommonName:   m.nodeID,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{m.nodeID, "localhost"},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return fmt.Errorf("failed to create node certificate: %w", err)
	}

	m.nodeCert = &tls.Certificate{
		Certificate: [][]byte{certBytes, m.caCert.Raw},
		PrivateKey:  key,
	}

	return nil
}

// GetTLSConfig returns a TLS configuration for mTLS.
func (m *TLSCertificateManager) GetTLSConfig() (*tls.Config, error) {
	if m.nodeCert == nil {
		return nil, fmt.Errorf("node certificate not generated")
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(m.caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{*m.nodeCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ExportCACert exports the CA certificate in PEM format.
func (m *TLSCertificateManager) ExportCACert() ([]byte, error) {
	if m.caCert == nil {
		return nil, fmt.Errorf("CA not initialized")
	}

	return pemEncodeCert(m.caCert.Raw), nil
}

// ImportCACert imports a CA certificate from PEM format.
func (m *TLSCertificateManager) ImportCACert(pemData []byte) error {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	m.caCert = cert
	return nil
}

// pemEncodeCert encodes a certificate to PEM format.
func pemEncodeCert(cert []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
}
