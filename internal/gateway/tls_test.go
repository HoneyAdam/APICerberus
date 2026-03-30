package gateway

import (
	"crypto/rand"
	"crypto/rsa"
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

func TestTLSManagerLoadsManualCertificate(t *testing.T) {
	t.Parallel()

	certPath, keyPath := writeTestCertificatePair(t, t.TempDir(), "example.com")

	manager, err := NewTLSManager(config.TLSConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	cert, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Fatalf("GetCertificate error: %v", err)
	}
	if cert == nil || cert.Leaf == nil {
		t.Fatalf("expected loaded leaf certificate")
	}
}

func TestTLSManagerLoadsCertificateFromDiskAndCaches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	domain := "api.example.com"
	writeTestCertificatePairWithPaths(t,
		filepath.Join(dir, domain+".crt"),
		filepath.Join(dir, domain+".key"),
		domain,
	)

	manager, err := NewTLSManager(config.TLSConfig{
		ACMEDir: dir,
	})
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	first, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
	if err != nil {
		t.Fatalf("GetCertificate first call error: %v", err)
	}
	if first == nil {
		t.Fatalf("expected non-nil certificate")
	}

	if err := os.Remove(filepath.Join(dir, domain+".crt")); err != nil {
		t.Fatalf("remove cert file: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, domain+".key")); err != nil {
		t.Fatalf("remove key file: %v", err)
	}

	second, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
	if err != nil {
		t.Fatalf("GetCertificate second call error: %v", err)
	}
	if second == nil {
		t.Fatalf("expected cached certificate")
	}
}

func writeTestCertificatePair(t *testing.T, dir, domain string) (string, string) {
	t.Helper()
	certPath := filepath.Join(dir, domain+".crt")
	keyPath := filepath.Join(dir, domain+".key")
	writeTestCertificatePairWithPaths(t, certPath, keyPath, domain)
	return certPath, keyPath
}

func writeTestCertificatePairWithPaths(t *testing.T, certPath, keyPath, domain string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	notBefore := time.Now().Add(-time.Hour)
	notAfter := time.Now().Add(48 * time.Hour)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: domain,
		},
		DNSNames:              []string{domain},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}
