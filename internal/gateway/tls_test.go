package gateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestTLSManagerLoadsManualCertificate(t *testing.T) {
	t.Parallel()

	certPath, keyPath := writeTestCertificatePair(t, t.TempDir(), "example.com", 48*time.Hour)

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
	writeTestCertificatePairWithPaths(
		t,
		filepath.Join(dir, domain+".crt"),
		filepath.Join(dir, domain+".key"),
		domain,
		48*time.Hour,
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

func TestTLSManagerRenewsSoonExpiringCertificateWhenAutoEnabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	domain := "renew.example.com"

	current := mustSelfSignedCertificate(t, domain, 12*time.Hour)
	renewed := mustSelfSignedCertificate(t, domain, 72*time.Hour)

	manager, err := NewTLSManager(config.TLSConfig{
		Auto:      true,
		ACMEEmail: "admin@example.com",
		ACMEDir:   dir,
	})
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}
	manager.certs.Store(domain, current)
	manager.issue = func(name string) (*tls.Certificate, error) {
		if name != domain {
			return nil, errors.New("unexpected domain")
		}
		return renewed, nil
	}

	got, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
	if err != nil {
		t.Fatalf("GetCertificate error: %v", err)
	}
	if got == nil || got.Leaf == nil {
		t.Fatalf("expected renewed certificate")
	}
	if !got.Leaf.NotAfter.After(current.Leaf.NotAfter) {
		t.Fatalf("expected renewed cert with later expiry, current=%v renewed=%v", current.Leaf.NotAfter, got.Leaf.NotAfter)
	}

	if _, err := os.Stat(filepath.Join(dir, domain+".crt")); err != nil {
		t.Fatalf("expected renewed certificate file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, domain+".key")); err != nil {
		t.Fatalf("expected renewed key file: %v", err)
	}
}

func TestTLSManagerKeepsValidCertWhenRenewalFails(t *testing.T) {
	t.Parallel()

	domain := "fallback.example.com"
	current := mustSelfSignedCertificate(t, domain, 12*time.Hour)

	manager, err := NewTLSManager(config.TLSConfig{
		Auto:      true,
		ACMEEmail: "admin@example.com",
		ACMEDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}
	manager.certs.Store(domain, current)
	manager.issue = func(string) (*tls.Certificate, error) {
		return nil, errors.New("simulated issuer failure")
	}

	got, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
	if err != nil {
		t.Fatalf("GetCertificate should fall back to current valid cert, got error: %v", err)
	}
	if got != current {
		t.Fatalf("expected fallback to current certificate")
	}
}

func writeTestCertificatePair(t *testing.T, dir, domain string, validFor time.Duration) (string, string) {
	t.Helper()
	certPath := filepath.Join(dir, domain+".crt")
	keyPath := filepath.Join(dir, domain+".key")
	writeTestCertificatePairWithPaths(t, certPath, keyPath, domain, validFor)
	return certPath, keyPath
}

func writeTestCertificatePairWithPaths(t *testing.T, certPath, keyPath, domain string, validFor time.Duration) {
	t.Helper()
	cert, certPEM, keyPEM := mustSelfSignedCertificateAndPEM(t, domain, validFor)
	if cert == nil {
		t.Fatalf("expected generated certificate")
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func mustSelfSignedCertificate(t *testing.T, domain string, validFor time.Duration) *tls.Certificate {
	t.Helper()
	cert, _, _ := mustSelfSignedCertificateAndPEM(t, domain, validFor)
	return cert
}

func mustSelfSignedCertificateAndPEM(t *testing.T, domain string, validFor time.Duration) (*tls.Certificate, []byte, []byte) {
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
	if validFor <= 0 {
		validFor = 24 * time.Hour
	}
	notAfter := time.Now().Add(validFor)

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

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build tls pair: %v", err)
	}
	cert := &pair
	if err := populateCertificateLeaf(cert); err != nil {
		t.Fatalf("parse cert leaf: %v", err)
	}
	return cert, certPEM, keyPEM
}

// --- parseTLSCipherSuites ---

func TestParseTLSCipherSuites_Valid(t *testing.T) {
	t.Parallel()
	suites := parseTLSCipherSuites([]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"})
	if len(suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites))
	}
	if suites[0] == 0 {
		t.Error("expected non-zero cipher suite ID")
	}
}

func TestParseTLSCipherSuites_Multiple(t *testing.T) {
	t.Parallel()
	suites := parseTLSCipherSuites([]string{
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	})
	if len(suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(suites))
	}
}

func TestParseTLSCipherSuites_Empty(t *testing.T) {
	t.Parallel()
	suites := parseTLSCipherSuites(nil)
	if suites != nil {
		t.Errorf("expected nil for empty input, got %v", suites)
	}
	suites = parseTLSCipherSuites([]string{})
	if suites != nil {
		t.Errorf("expected nil for empty slice, got %v", suites)
	}
}

func TestParseTLSCipherSuites_Unknown(t *testing.T) {
	t.Parallel()
	suites := parseTLSCipherSuites([]string{"TLS_FAKE_CIPHER_SUITE"})
	if suites != nil {
		t.Errorf("expected nil for unknown cipher, got %v", suites)
	}
}

func TestParseTLSCipherSuites_Mixed(t *testing.T) {
	t.Parallel()
	suites := parseTLSCipherSuites([]string{
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"TLS_FAKE_CIPHER",
	})
	if len(suites) != 1 {
		t.Errorf("expected 1 valid suite, got %d", len(suites))
	}
}

// --- parseTLSMinVersion ---

func TestParseTLSMinVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  uint16
	}{
		{"1.2", tls.VersionTLS12},
		{"1.3", tls.VersionTLS13},
		{"1.0", tls.VersionTLS12}, // deprecated → enforced 1.2
		{"1.1", tls.VersionTLS12}, // deprecated → enforced 1.2
		{"", tls.VersionTLS12},    // default
		{"unknown", tls.VersionTLS12},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := parseTLSMinVersion(tt.input)
			if got != tt.want {
				t.Errorf("parseTLSMinVersion(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
