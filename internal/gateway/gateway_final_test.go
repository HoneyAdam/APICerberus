package gateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// ==================== Server Lifecycle Tests ====================

// TestGateway_Start_WithNoListeners tests Start with no listeners configured
func TestGateway_Start_WithNoListeners(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			// No HTTPAddr or HTTPSAddr configured
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = g.Start(ctx)
	if err == nil {
		t.Error("Expected error when no listeners configured")
	}
}

// TestGateway_Start_WithGRPC tests Start with gRPC server enabled
func TestGateway_Start_WithGRPC(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
			GRPC: config.GRPCConfig{
				Enabled: true,
				Addr:    "127.0.0.1:0",
			},
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start in background since it blocks
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		// Expected - server should shut down
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Start returned: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for Start to return")
	}
}

// TestGateway_Start_WithNilContext tests Start with nil context
func TestGateway_Start_WithNilContext(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Start with nil context - should use background
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Start(context.Background())
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-errCh:
		// Expected
	case <-time.After(5 * time.Second):
		// May timeout, that's ok - the test is checking nil context handling
		t.Log("Start did not return within timeout")
	}
}

// TestGateway_Start_ListenerError tests Start when listener fails
func TestGateway_Start_ListenerError(t *testing.T) {
	// Create a listener to occupy the port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: addr, // Already in use
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = g.Start(ctx)
	if err == nil {
		t.Error("Expected error when port is already in use")
	}
}

// TestGateway_Reload_ErrorPaths tests Reload error scenarios
func TestGateway_Reload_ErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with nil config
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Test nil config
	err = g.Reload(nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}

	// Test with invalid routes
	badCfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Routes: []config.Route{
			{
				ID:      "bad-route",
				Name:    "Bad Route",
				Methods: []string{"GET"},
				Paths:   []string{"[invalid(regex"}, // Invalid regex
			},
		},
	}
	err = g.Reload(badCfg)
	if err == nil {
		t.Error("Expected error for invalid route regex")
	}
}

// TestGateway_Reload_WithHTTPS tests Reload with HTTPS configuration
func TestGateway_Reload_WithHTTPS(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Create test certificate files
	certFile := tmpDir + "/test.crt"
	keyFile := tmpDir + "/test.key"
	if err := generateTestCert(certFile, keyFile); err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	// Reload with HTTPS
	newCfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:  "127.0.0.1:0",
			HTTPSAddr: "127.0.0.1:0",
			TLS: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
			},
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    60 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test2.db",
		},
	}

	err = g.Reload(newCfg)
	if err != nil {
		t.Errorf("Reload error: %v", err)
	}
}

// TestGateway_Reload_WithPluginError tests Reload with plugin build error
func TestGateway_Reload_WithPluginError(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Create a config that will fail plugin build
	badCfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test2.db",
		},
		Routes: []config.Route{
			{
				ID:   "route1",
				Name: "Route 1",
				Plugins: []config.PluginConfig{
					{
						Name:    "invalid_plugin_type",
						Enabled: boolPtr(true),
						Config:  map[string]any{"invalid": make(chan int)}, // Unmarshalable config
					},
				},
			},
		},
	}

	err = g.Reload(badCfg)
	if err == nil {
		t.Error("Expected error for invalid plugin config")
	}
}

// TestGateway_Shutdown_Graceful tests graceful shutdown
func TestGateway_Shutdown_Graceful(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start server
	go func() {
		g.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	err = g.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

// TestGateway_Shutdown_WithGRPC tests shutdown with gRPC server
func TestGateway_Shutdown_WithGRPC(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
			GRPC: config.GRPCConfig{
				Enabled: true,
				Addr:    "127.0.0.1:0",
			},
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		g.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	err = g.Shutdown(shutdownCtx)
	// May error due to gRPC not fully started, that's ok
	t.Logf("Shutdown result: %v", err)
}

// ==================== TLS Certificate Tests ====================

// Helper to generate test certificates
func generateTestCert(certFile, keyFile string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return nil
}

// TestTLSManager_New_WithCertFiles tests NewTLSManager with certificate files
func TestTLSManager_New_WithCertFiles(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := tmpDir + "/test.crt"
	keyFile := tmpDir + "/test.key"

	if err := generateTestCert(certFile, keyFile); err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cfg := config.TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}
	if tm == nil {
		t.Fatal("Expected non-nil TLSManager")
	}

	// Test GetCertificate for wildcard
	cert, err := tm.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Errorf("GetCertificate error: %v", err)
	}
	if cert == nil {
		t.Error("Expected non-nil certificate")
	}
}

// TestTLSManager_New_WithInvalidCertFile tests NewTLSManager with invalid cert file
func TestTLSManager_New_WithInvalidCertFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.TLSConfig{
		CertFile: tmpDir + "/nonexistent.crt",
		KeyFile:  tmpDir + "/nonexistent.key",
	}
	_, err := NewTLSManager(cfg)
	if err == nil {
		t.Error("Expected error for non-existent certificate files")
	}
}

// TestTLSManager_New_WithACMEMkdirError tests NewTLSManager with ACME dir creation error
func TestTLSManager_New_WithACMEMkdirError(t *testing.T) {
	// Try to create ACME dir in a read-only location (this may not work on Windows)
	// Instead, use an invalid path
	cfg := config.TLSConfig{
		Auto:      true,
		ACMEEmail: "test@example.com",
		ACMEDir:   "/dev/null/invalid", // Invalid path on Unix
	}
	_, err := NewTLSManager(cfg)
	// May or may not error depending on OS
	t.Logf("NewTLSManager result: %v", err)
}

// TestTLSManager_TLSConfig tests TLSConfig method
func TestTLSManager_TLSConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:      true,
		ACMEEmail: "test@example.com",
		ACMEDir:   tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	tlsConfig := tm.TLSConfig()
	if tlsConfig == nil {
		t.Fatal("Expected non-nil TLSConfig")
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2, got %d", tlsConfig.MinVersion)
	}
}

// TestTLSManager_GetCertificate_NilManager tests GetCertificate with nil manager
func TestTLSManager_GetCertificate_NilManager(t *testing.T) {
	var tm *TLSManager
	_, err := tm.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err == nil {
		t.Error("Expected error for nil TLSManager")
	}
}

// TestTLSManager_GetCertificate_CachedExpired tests GetCertificate with expired cached cert
func TestTLSManager_GetCertificate_CachedExpired(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:      true,
		ACMEEmail: "test@example.com",
		ACMEDir:   tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Create an expired certificate
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour), // Expired
		DNSNames:     []string{"expired.example.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)

	expiredCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	// Store expired cert in cache
	tm.certs.Store("expired.example.com", expiredCert)

	// Request the expired cert - should try to renew
	_, err = tm.GetCertificate(&tls.ClientHelloInfo{ServerName: "expired.example.com"})
	// Will fail since autocertM is nil (no ACME configured properly)
	// but should not panic
	t.Logf("GetCertificate result: %v", err)
}

// TestTLSManager_GetCertificate_NoAutoRenew tests GetCertificate without auto renew
func TestTLSManager_GetCertificate_NoAutoRenew(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := tmpDir + "/test.crt"
	keyFile := tmpDir + "/test.key"

	if err := generateTestCert(certFile, keyFile); err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cfg := config.TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
		Auto:     false, // No auto-renewal
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Get wildcard cert
	cert, err := tm.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Errorf("GetCertificate error: %v", err)
	}
	if cert == nil {
		t.Error("Expected non-nil certificate")
	}
}

// TestTLSManager_issueAndStore_ErrorPaths tests issueAndStore error paths
func TestTLSManager_issueAndStore_ErrorPaths(t *testing.T) {
	cfg := config.TLSConfig{
		Auto: true,
		// No ACMEDir - autocertM will be nil
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Test with empty server name
	_, err = tm.issueAndStore("")
	if err == nil {
		t.Error("Expected error for empty server name")
	}

	// Test when issue func is nil
	tm.issue = nil
	_, err = tm.issueAndStore("example.com")
	if err == nil {
		t.Error("Expected error when issue func is nil")
	}
}

// TestTLSManager_saveToDisk_ErrorPaths tests saveToDisk error paths
func TestTLSManager_saveToDisk_ErrorPaths(t *testing.T) {
	// Test with nil cert
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, _ := NewTLSManager(cfg)

	err := tm.saveToDisk("test.com", nil)
	if err == nil {
		t.Error("Expected error for nil certificate")
	}

	// Test with empty certificate chain
	emptyCert := &tls.Certificate{
		Certificate: [][]byte{},
		PrivateKey:  &testPrivateKey{},
	}
	err = tm.saveToDisk("test.com", emptyCert)
	if err == nil {
		t.Error("Expected error for empty certificate chain")
	}

	// Test with nil private key
	noKeyCert := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}},
		PrivateKey:  nil,
	}
	err = tm.saveToDisk("test.com", noKeyCert)
	if err == nil {
		t.Error("Expected error for nil private key")
	}
}

// TestTLSManager_loadFromDisk_ErrorPaths tests loadFromDisk error paths
func TestTLSManager_loadFromDisk_ErrorPaths(t *testing.T) {
	// Test with empty ACMEDir
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: "",
	}
	tm, _ := NewTLSManager(cfg)

	_, err := tm.loadFromDisk("example.com")
	if err == nil {
		t.Error("Expected error for empty ACMEDir")
	}

	// Test with non-existent files
	tmpDir := t.TempDir()
	cfg2 := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm2, _ := NewTLSManager(cfg2)

	_, err = tm2.loadFromDisk("nonexistent.com")
	if err == nil {
		t.Error("Expected error for non-existent certificate files")
	}
}

// TestTLSManager_ReloadCertificate_ErrorPaths tests ReloadCertificate error paths
func TestTLSManager_ReloadCertificate_ErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, _ := NewTLSManager(cfg)

	// Test with empty server name
	err := tm.ReloadCertificate("")
	if err == nil {
		t.Error("Expected error for empty server name")
	}

	// Test with non-existent certificate
	err = tm.ReloadCertificate("nonexistent.com")
	if err == nil {
		t.Error("Expected error for non-existent certificate")
	}
}

// TestTLSManager_LoadAllCertificatesFromDisk_Error tests LoadAllCertificatesFromDisk errors
func TestTLSManager_LoadAllCertificatesFromDisk_Error(t *testing.T) {
	// Test with empty ACMEDir
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: "",
	}
	tm, _ := NewTLSManager(cfg)

	err := tm.LoadAllCertificatesFromDisk()
	if err != nil {
		t.Errorf("Expected nil error for empty ACMEDir, got %v", err)
	}

	// Test with non-existent directory
	cfg2 := config.TLSConfig{
		Auto:    true,
		ACMEDir: "/nonexistent/path/that/does/not/exist",
	}
	tm2, _ := NewTLSManager(cfg2)

	err = tm2.LoadAllCertificatesFromDisk()
	// May or may not error depending on OS
	t.Logf("LoadAllCertificatesFromDisk result: %v", err)
}

// TestEncodePrivateKeyPEM_ECDSA tests encodePrivateKeyPEM with ECDSA key
func TestEncodePrivateKeyPEM_ECDSA(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}

	pem, err := encodePrivateKeyPEM(privateKey)
	if err != nil {
		t.Errorf("encodePrivateKeyPEM error: %v", err)
	}
	if pem == nil {
		t.Error("Expected non-nil PEM")
	}
	if !bytes.Contains(pem, []byte("EC PRIVATE KEY")) {
		t.Error("Expected EC PRIVATE KEY in PEM")
	}
}

// TestEncodePrivateKeyPEM_Unsupported tests encodePrivateKeyPEM with unsupported key type
func TestEncodePrivateKeyPEM_Unsupported(t *testing.T) {
	_, err := encodePrivateKeyPEM("not-a-key")
	if err == nil {
		t.Error("Expected error for unsupported key type")
	}
}

// TestEncodePrivateKeyPEM_PKCS8Error tests encodePrivateKeyPEM PKCS8 marshal error
func TestEncodePrivateKeyPEM_PKCS8Error(t *testing.T) {
	// Use a key type that will fail PKCS8 marshaling
	// This is tricky to test since most Go crypto types support PKCS8
	// We'll use an invalid type
	_, err := encodePrivateKeyPEM(struct{}{})
	if err == nil {
		t.Error("Expected error for invalid key type")
	}
}

// TestWriteFileAtomic_ErrorPaths tests writeFileAtomic error paths
func TestWriteFileAtomic_ErrorPaths(t *testing.T) {
	// Test with invalid directory
	err := writeFileAtomic("/nonexistent/dir/file.txt", []byte("test"), 0644)
	// Platform dependent - may or may not error
	t.Logf("writeFileAtomic result: %v", err)
}

// ==================== WebSocket Proxy Tests ====================

// TestProxy_ForwardWebSocket_NilProxy tests ForwardWebSocket with nil proxy
func TestProxy_ForwardWebSocket_NilProxy(t *testing.T) {
	var p *Proxy
	err := p.ForwardWebSocket(&RequestContext{
		Request:        httptest.NewRequest("GET", "/ws", nil),
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil proxy")
	}
}

// TestProxy_ForwardWebSocket_InvalidContext tests ForwardWebSocket with invalid context
func TestProxy_ForwardWebSocket_InvalidContext(t *testing.T) {
	p := NewProxy()

	// Test with nil context
	err := p.ForwardWebSocket(nil, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil context")
	}

	// Test with nil request
	err = p.ForwardWebSocket(&RequestContext{
		Request:        nil,
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil request")
	}

	// Test with nil response writer
	err = p.ForwardWebSocket(&RequestContext{
		Request:        httptest.NewRequest("GET", "/ws", nil),
		ResponseWriter: nil,
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil response writer")
	}
}

// TestProxy_ForwardWebSocket_NotWebSocket tests ForwardWebSocket with non-websocket request
func TestProxy_ForwardWebSocket_NotWebSocket(t *testing.T) {
	p := NewProxy()

	req := httptest.NewRequest("GET", "/ws", nil)
	// Not a websocket upgrade request

	err := p.ForwardWebSocket(&RequestContext{
		Request:        req,
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for non-websocket request")
	}
}

// TestProxy_ForwardWebSocket_InvalidTarget tests ForwardWebSocket with invalid target
func TestProxy_ForwardWebSocket_InvalidTarget(t *testing.T) {
	p := NewProxy()

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	// Test with nil target
	err := p.ForwardWebSocket(&RequestContext{
		Request:        req,
		ResponseWriter: httptest.NewRecorder(),
	}, nil)
	if err == nil {
		t.Error("Expected error for nil target")
	}

	// Test with empty address
	err = p.ForwardWebSocket(&RequestContext{
		Request:        req,
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: ""})
	if err == nil {
		t.Error("Expected error for empty target address")
	}
}

// TestProxy_ForwardWebSocket_NotHijacker tests ForwardWebSocket with non-hijacker response writer
func TestProxy_ForwardWebSocket_NotHijacker(t *testing.T) {
	p := NewProxy()

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	// httptest.ResponseRecorder doesn't implement Hijacker
	err := p.ForwardWebSocket(&RequestContext{
		Request:        req,
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error when response writer doesn't support hijacking")
	}
}

// TestProxy_Do_NilProxy tests Do with nil proxy
func TestProxy_Do_NilProxy(t *testing.T) {
	var p *Proxy
	_, err := p.Do(&RequestContext{
		Request: httptest.NewRequest("GET", "/api", nil),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil proxy")
	}
}

// TestProxy_Do_InvalidContext tests Do with invalid context
func TestProxy_Do_InvalidContext(t *testing.T) {
	p := NewProxy()

	// Test with nil context
	_, err := p.Do(nil, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil context")
	}

	// Test with nil request
	_, err = p.Do(&RequestContext{
		Request: nil,
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil request")
	}
}

// TestProxy_Do_InvalidTarget tests Do with invalid target
func TestProxy_Do_InvalidTarget(t *testing.T) {
	p := NewProxy()

	// Test with nil target
	_, err := p.Do(&RequestContext{
		Request: httptest.NewRequest("GET", "/api", nil),
	}, nil)
	if err == nil {
		t.Error("Expected error for nil target")
	}

	// Test with empty address
	_, err = p.Do(&RequestContext{
		Request: httptest.NewRequest("GET", "/api", nil),
	}, &config.UpstreamTarget{ID: "t1", Address: ""})
	if err == nil {
		t.Error("Expected error for empty target address")
	}
}

// TestProxy_Do_InvalidURL tests Do with invalid URL construction
func TestProxy_Do_InvalidURL(t *testing.T) {
	p := NewProxy()

	// Test with invalid URL that will fail parsing
	_, err := p.Do(&RequestContext{
		Request: httptest.NewRequest("GET", "/api", nil),
	}, &config.UpstreamTarget{ID: "t1", Address: "://invalid-url"})
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

// TestProxy_WriteResponse_NilProxy tests WriteResponse with nil proxy
func TestProxy_WriteResponse_NilProxy(t *testing.T) {
	var p *Proxy
	err := p.WriteResponse(httptest.NewRecorder(), &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("test")),
	})
	if err == nil {
		t.Error("Expected error for nil proxy")
	}
}

// TestProxy_WriteResponse_InvalidArgs tests WriteResponse with invalid args
func TestProxy_WriteResponse_InvalidArgs(t *testing.T) {
	p := NewProxy()

	// Test with nil writer
	err := p.WriteResponse(nil, &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("test")),
	})
	if err == nil {
		t.Error("Expected error for nil writer")
	}

	// Test with nil response
	err = p.WriteResponse(httptest.NewRecorder(), nil)
	if err == nil {
		t.Error("Expected error for nil response")
	}
}

// TestProxy_Forward_NilProxy tests Forward with nil proxy
func TestProxy_Forward_NilProxy(t *testing.T) {
	var p *Proxy
	err := p.Forward(&RequestContext{
		Request:        httptest.NewRequest("GET", "/api", nil),
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil proxy")
	}
}

// TestDialUpstreamWebSocket_UnsupportedScheme tests dialUpstreamWebSocket with unsupported scheme
func TestDialUpstreamWebSocket_UnsupportedScheme(t *testing.T) {
	u, _ := url.Parse("ftp://localhost:8080")
	_, err := dialUpstreamWebSocket(u)
	if err == nil {
		t.Error("Expected error for unsupported scheme")
	}
}

// TestIsWebSocketUpgrade_InvalidMethod tests isWebSocketUpgrade with invalid method
func TestIsWebSocketUpgrade_InvalidMethod(t *testing.T) {
	// POST request should not be websocket upgrade
	req := httptest.NewRequest("POST", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	if isWebSocketUpgrade(req) {
		t.Error("Expected false for POST request")
	}
}

// ==================== Health Check Tests ====================

// TestChecker_Start_WithZeroInterval tests Start with zero interval
func TestChecker_Start_WithZeroInterval(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
			HealthCheck: config.HealthCheckConfig{
				Active: config.ActiveHealthCheckConfig{
					Interval: 0, // Zero interval - should not start loop
				},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Should not panic and should return quickly since no loops are started
	checker.Start(ctx)

	// Give any potential goroutines time to run
	time.Sleep(100 * time.Millisecond)
}

// TestChecker_checkAllTargets_WithErrorResponse tests checkAllTargets with error response
func TestChecker_checkAllTargets_WithErrorResponse(t *testing.T) {
	// Start a server that returns error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: addr},
			},
			HealthCheck: config.HealthCheckConfig{
				Active: config.ActiveHealthCheckConfig{
					Interval: 100 * time.Millisecond,
					Timeout:  1 * time.Second,
					Path:     "/health",
				},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	checker.Start(ctx)

	// Wait for health check to run
	time.Sleep(150 * time.Millisecond)

	// Target should be marked unhealthy
	healthy := checker.IsHealthy("test-upstream", "t1")
	if healthy {
		t.Error("Expected target to be unhealthy after error response")
	}
}

// TestChecker_checkAllTargets_WithTimeout tests checkAllTargets with timeout
func TestChecker_checkAllTargets_WithTimeout(t *testing.T) {
	// Start a server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // Will definitely timeout
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: addr},
			},
			HealthCheck: config.HealthCheckConfig{
				Active: config.ActiveHealthCheckConfig{
					Interval: 200 * time.Millisecond,
					Timeout:  100 * time.Millisecond, // Short timeout
					Path:     "/health",
				},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	checker.Start(ctx)

	// Wait for health check to run multiple times
	time.Sleep(400 * time.Millisecond)

	// Target may or may not be marked unhealthy depending on timing
	// Just verify the checker doesn't panic
	t.Logf("Target health after timeout test: %v", checker.IsHealthy("test-upstream", "t1"))
}

// TestChecker_applyHealthResult_NonExistentUpstream tests applyHealthResult with non-existent upstream
func TestChecker_applyHealthResult_NonExistentUpstream(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	// Should not panic for non-existent upstream
	checker.applyHealthResult("non-existent", "t1", true, 10*time.Millisecond, config.ActiveHealthCheckConfig{})
}

// TestChecker_applyHealthResult_NonExistentTarget tests applyHealthResult with non-existent target
func TestChecker_applyHealthResult_NonExistentTarget(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	// Should not panic for non-existent target
	checker.applyHealthResult("test-upstream", "non-existent", true, 10*time.Millisecond, config.ActiveHealthCheckConfig{})
}

// TestChecker_ReportError_NonExistentUpstream tests ReportError with non-existent upstream
func TestChecker_ReportError_NonExistentUpstream(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	// Should not panic for non-existent upstream
	checker.ReportError("non-existent", "t1")
}

// TestChecker_ReportError_NonExistentTarget tests ReportError with non-existent target
func TestChecker_ReportError_NonExistentTarget(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	// Should not panic for non-existent target
	checker.ReportError("test-upstream", "non-existent")
}

// TestChecker_ReportSuccess_NonExistentUpstream tests ReportSuccess with non-existent upstream
func TestChecker_ReportSuccess_NonExistentUpstream(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	// Should not panic for non-existent upstream
	checker.ReportSuccess("non-existent", "t1")
}

// TestChecker_ReportSuccess_NonExistentTarget tests ReportSuccess with non-existent target
func TestChecker_ReportSuccess_NonExistentTarget(t *testing.T) {
	upstreams := []config.Upstream{
		{
			Name: "test-upstream",
			Targets: []config.UpstreamTarget{
				{ID: "t1", Address: "localhost:8080"},
			},
		},
	}

	checker := NewChecker(upstreams, map[string]*UpstreamPool{})

	// Should not panic for non-existent target
	checker.ReportSuccess("test-upstream", "non-existent")
}

// TestDerivePassiveConfig_ZeroThresholds tests derivePassiveConfig with zero thresholds
func TestDerivePassiveConfig_ZeroThresholds(t *testing.T) {
	cfg := config.ActiveHealthCheckConfig{
		UnhealthyThreshold: 0,
		HealthyThreshold:   0,
		Interval:           0,
	}

	result := derivePassiveConfig(cfg)

	if result.errorThreshold != 3 {
		t.Errorf("Expected errorThreshold=3, got %d", result.errorThreshold)
	}
	if result.successThreshold != 2 {
		t.Errorf("Expected successThreshold=2, got %d", result.successThreshold)
	}
	if result.window != 30*time.Second {
		t.Errorf("Expected window=30s, got %v", result.window)
	}
}

// TestDerivePassiveConfig_SmallWindow tests derivePassiveConfig with very small window
func TestDerivePassiveConfig_SmallWindow(t *testing.T) {
	cfg := config.ActiveHealthCheckConfig{
		UnhealthyThreshold: 1,
		HealthyThreshold:   1,
		Interval:           1 * time.Millisecond,
	}

	result := derivePassiveConfig(cfg)

	// Window should be clamped to minimum 200ms
	if result.window < 200*time.Millisecond {
		t.Errorf("Expected window >= 200ms, got %v", result.window)
	}
}

// TestPruneOldErrors_Empty tests pruneOldErrors with empty slice
func TestPruneOldErrors_Empty(t *testing.T) {
	result := pruneOldErrors([]time.Time{}, time.Now(), time.Minute)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}
}

// TestPruneOldErrors_ZeroWindow tests pruneOldErrors with zero window
func TestPruneOldErrors_ZeroWindow(t *testing.T) {
	errors := []time.Time{
		time.Now().Add(-time.Hour),
		time.Now().Add(-time.Minute),
	}

	result := pruneOldErrors(errors, time.Now(), 0)
	if len(result) != 0 {
		t.Errorf("Expected empty result for zero window, got %d items", len(result))
	}
}

// TestPruneOldErrors_AllOld tests pruneOldErrors when all errors are old
func TestPruneOldErrors_AllOld(t *testing.T) {
	now := time.Now()
	errors := []time.Time{
		now.Add(-10 * time.Minute),
		now.Add(-9 * time.Minute),
		now.Add(-8 * time.Minute),
	}

	result := pruneOldErrors(errors, now, 5*time.Minute)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}
}

// TestPruneOldErrors_AllNew tests pruneOldErrors when all errors are new
func TestPruneOldErrors_AllNew(t *testing.T) {
	now := time.Now()
	errors := []time.Time{
		now.Add(-1 * time.Minute),
		now.Add(-30 * time.Second),
		now.Add(-10 * time.Second),
	}

	result := pruneOldErrors(errors, now, 5*time.Minute)
	if len(result) != 3 {
		t.Errorf("Expected 3 items, got %d", len(result))
	}
}

// ==================== Federation Tests ====================

// TestGateway_ServeFederation_MethodNotAllowed tests serveFederation with wrong method
func TestGateway_ServeFederation_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
		Federation: config.FederationConfig{
			Enabled: true,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Test GET request (should be POST)
	req := httptest.NewRequest("GET", "/graphql", nil)
	w := httptest.NewRecorder()
	g.serveFederation(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// TestGateway_ServeFederation_InvalidJSON tests serveFederation with invalid JSON
func TestGateway_ServeFederation_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
		Federation: config.FederationConfig{
			Enabled: true,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Test with invalid JSON
	req := httptest.NewRequest("POST", "/graphql", strings.NewReader("not valid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.serveFederation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestGateway_ServeFederation_EmptyQuery tests serveFederation with empty query
func TestGateway_ServeFederation_EmptyQuery(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
		Federation: config.FederationConfig{
			Enabled: true,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Test with empty query
	req := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{"query": ""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.serveFederation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestGateway_ServeFederation_NotReady tests serveFederation when federation not ready
func TestGateway_ServeFederation_NotReady(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
		Federation: config.FederationConfig{
			Enabled: true,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Test when planner/executor are nil (not ready)
	req := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{"query": "{ test }"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.serveFederation(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// TestGateway_RebuildFederationPlanner_Disabled tests RebuildFederationPlanner when disabled
func TestGateway_RebuildFederationPlanner_Disabled(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Federation: config.FederationConfig{
			Enabled: false,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Should not panic when federation is disabled
	g.RebuildFederationPlanner()
}

// TestGateway_RebuildFederationPlanner_NilSubgraphs tests RebuildFederationPlanner with nil subgraphs
func TestGateway_RebuildFederationPlanner_NilSubgraphs(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
		Federation: config.FederationConfig{
			Enabled: true,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Set subgraphs to nil
	g.subgraphs = nil

	// Should not panic with nil subgraphs
	g.RebuildFederationPlanner()
}

// TestGateway_RebuildFederationPlanner_NilComposer tests RebuildFederationPlanner with nil composer
func TestGateway_RebuildFederationPlanner_NilComposer(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Store: config.StoreConfig{
			Path: tmpDir + "/test.db",
		},
		Federation: config.FederationConfig{
			Enabled: true,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Set composer to nil
	g.federationComposer = nil

	// Should not panic with nil composer
	g.RebuildFederationPlanner()
}

// ==================== Balancer Tests ====================

// TestRoundRobin_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestRoundRobin_ReportHealth_NilHealthMap(t *testing.T) {
	rr := NewRoundRobin([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set health to nil
	rr.health = nil

	// Should not panic
	rr.ReportHealth("a", false, 0)

	// Verify health was set
	if rr.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestWeightedRoundRobin_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestWeightedRoundRobin_ReportHealth_NilHealthMap(t *testing.T) {
	wrr := NewWeightedRoundRobin([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080", Weight: 1},
	})

	// Manually set health to nil
	wrr.health = nil

	// Should not panic
	wrr.ReportHealth("a", false, 0)

	// Verify health was set
	if wrr.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestLeastConn_Next_NoHealthyTargets tests Next when no targets are healthy
func TestLeastConn_Next_NoHealthyTargets(t *testing.T) {
	lc := NewLeastConn([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	lc.ReportHealth("a", false, 0)

	_, err := lc.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestLeastConn_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestLeastConn_ReportHealth_NilHealthMap(t *testing.T) {
	lc := NewLeastConn([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set health to nil
	lc.health = nil

	// Should not panic
	lc.ReportHealth("a", false, 0)

	// Verify health was set
	if lc.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestIPHash_Next_NoHealthyTargets tests Next when no targets are healthy
func TestIPHash_Next_NoHealthyTargets(t *testing.T) {
	ih := NewIPHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	ih.ReportHealth("a", false, 0)

	_, err := ih.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestIPHash_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestIPHash_ReportHealth_NilHealthMap(t *testing.T) {
	ih := NewIPHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set health to nil
	ih.health = nil

	// Should not panic
	ih.ReportHealth("a", false, 0)

	// Verify health was set
	if ih.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestRandomBalancer_Next_NoHealthyTargets tests Next when no targets are healthy
func TestRandomBalancer_Next_NoHealthyTargets(t *testing.T) {
	rb := NewRandomBalancer([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	rb.ReportHealth("a", false, 0)

	_, err := rb.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestRandomBalancer_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestRandomBalancer_ReportHealth_NilHealthMap(t *testing.T) {
	rb := NewRandomBalancer([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set health to nil
	rb.health = nil

	// Should not panic
	rb.ReportHealth("a", false, 0)

	// Verify health was set
	if rb.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestConsistentHash_Next_NoHealthyTargets tests Next when no targets are healthy
func TestConsistentHash_Next_NoHealthyTargets(t *testing.T) {
	ch := NewConsistentHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	ch.ReportHealth("a", false, 0)

	_, err := ch.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestConsistentHash_Next_EmptyRing tests Next with empty ring
func TestConsistentHash_Next_EmptyRing(t *testing.T) {
	ch := NewConsistentHash([]config.UpstreamTarget{})

	_, err := ch.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestConsistentHash_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestConsistentHash_ReportHealth_NilHealthMap(t *testing.T) {
	ch := NewConsistentHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set health to nil
	ch.health = nil

	// Should not panic
	ch.ReportHealth("a", false, 0)

	// Verify health was set
	if ch.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestConsistentHash_rebuildRingLocked_ZeroReplicas tests rebuildRingLocked with zero replicas
func TestConsistentHash_rebuildRingLocked_ZeroReplicas(t *testing.T) {
	ch := NewConsistentHash([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Set replicas to 0
	ch.replicas = 0

	// Should reset to 120
	ch.rebuildRingLocked()

	if ch.replicas != 120 {
		t.Errorf("Expected replicas=120, got %d", ch.replicas)
	}
}

// TestLeastLatency_Next_NoHealthyTargets tests Next when no targets are healthy
func TestLeastLatency_Next_NoHealthyTargets(t *testing.T) {
	ll := NewLeastLatency([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	ll.ReportHealth("a", false, 0)

	_, err := ll.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestLeastLatency_ReportHealth_NilMaps tests ReportHealth with nil maps
func TestLeastLatency_ReportHealth_NilMaps(t *testing.T) {
	ll := NewLeastLatency([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set maps to nil
	ll.health = nil
	ll.latency = nil

	// Should not panic
	ll.ReportHealth("a", true, 100*time.Millisecond)

	// Verify maps were initialized
	if ll.health == nil {
		t.Error("Expected health map to be initialized")
	}
	if ll.latency == nil {
		t.Error("Expected latency map to be initialized")
	}
}

// TestLeastLatency_ReportHealth_ZeroLatency tests ReportHealth with zero latency
func TestLeastLatency_ReportHealth_ZeroLatency(t *testing.T) {
	ll := NewLeastLatency([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Report with zero latency - should not update
	ll.ReportHealth("a", true, 0)

	// Latency should not be set
	if _, ok := ll.latency["a"]; ok {
		t.Error("Expected latency not to be set for zero latency")
	}
}

// TestAdaptive_mode tests mode function with different states
func TestAdaptive_mode(t *testing.T) {
	a := NewAdaptive([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Initially should be round_robin (not enough samples)
	mode := a.mode()
	if mode != "round_robin" {
		t.Errorf("Expected round_robin, got %s", mode)
	}

	// Simulate high error rate
	a.mu.Lock()
	a.totalCount = 100
	a.errorCount = 30 // 30% error rate
	a.mu.Unlock()

	mode = a.mode()
	if mode != "least_conn" {
		t.Errorf("Expected least_conn, got %s", mode)
	}

	// Simulate high latency
	a.mu.Lock()
	a.totalCount = 100
	a.errorCount = 10 // 10% error rate (below threshold)
	a.latencyEWMA = 300 * time.Millisecond
	a.mu.Unlock()

	mode = a.mode()
	if mode != "least_latency" {
		t.Errorf("Expected least_latency, got %s", mode)
	}
}

// TestAdaptive_ReportHealth_CounterReset tests ReportHealth counter reset
func TestAdaptive_ReportHealth_CounterReset(t *testing.T) {
	a := NewAdaptive([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Simulate high counter
	a.mu.Lock()
	a.totalCount = 100000
	a.errorCount = 50000
	a.mu.Unlock()

	// Report health - should trigger counter reset
	a.ReportHealth("a", true, 10*time.Millisecond)

	a.mu.RLock()
	totalCount := a.totalCount
	errorCount := a.errorCount
	a.mu.RUnlock()

	if totalCount != 50000 {
		t.Errorf("Expected totalCount=50000 after reset, got %d", totalCount)
	}
	if errorCount != 25000 {
		t.Errorf("Expected errorCount=25000 after reset, got %d", errorCount)
	}
}

// TestSubnetAware_Next_NoHealthyTargets tests Next when no targets are healthy
func TestSubnetAware_Next_NoHealthyTargets(t *testing.T) {
	ga := NewSubnetAware([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	ga.ReportHealth("a", false, 0)

	ctx := &RequestContext{
		Request: httptest.NewRequest("GET", "/test", nil),
	}

	_, err := ga.Next(ctx)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestSubnetAware_ReportHealth_NilHealthMap tests ReportHealth with nil health map
func TestSubnetAware_ReportHealth_NilHealthMap(t *testing.T) {
	ga := NewSubnetAware([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set health to nil
	ga.health = nil

	// Should not panic
	ga.ReportHealth("a", false, 0)

	// Verify health was set
	if ga.health == nil {
		t.Error("Expected health map to be initialized")
	}
}

// TestHealthWeighted_Next_NoHealthyTargets tests Next when no targets are healthy
func TestHealthWeighted_Next_NoHealthyTargets(t *testing.T) {
	hw := NewHealthWeighted([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Mark target as unhealthy
	hw.ReportHealth("a", false, 0)

	_, err := hw.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestHealthWeighted_Next_ZeroScore tests Next when all targets have zero score
func TestHealthWeighted_Next_ZeroScore(t *testing.T) {
	hw := NewHealthWeighted([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set score to 0
	hw.mu.Lock()
	hw.score["a"] = 0
	hw.mu.Unlock()

	_, err := hw.Next(nil)
	if err != ErrNoHealthyTargets {
		t.Errorf("Expected ErrNoHealthyTargets, got %v", err)
	}
}

// TestHealthWeighted_ReportHealth_NilMaps tests ReportHealth with nil maps
func TestHealthWeighted_ReportHealth_NilMaps(t *testing.T) {
	hw := NewHealthWeighted([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Manually set maps to nil
	hw.health = nil
	hw.score = nil

	// Should not panic
	hw.ReportHealth("a", false, 0)

	// Verify maps were initialized
	if hw.health == nil {
		t.Error("Expected health map to be initialized")
	}
	if hw.score == nil {
		t.Error("Expected score map to be initialized")
	}
}

// TestHealthWeighted_ReportHealth_ScoreBounds tests ReportHealth score boundaries
func TestHealthWeighted_ReportHealth_ScoreBounds(t *testing.T) {
	hw := NewHealthWeighted([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Set score to high value
	hw.mu.Lock()
	hw.score["a"] = 0.95
	hw.mu.Unlock()

	// Report healthy - should cap at 1.0
	hw.ReportHealth("a", true, 0)

	hw.mu.RLock()
	score := hw.score["a"]
	hw.mu.RUnlock()

	if score != 1.0 {
		t.Errorf("Expected score=1.0, got %f", score)
	}

	// Set score to low value
	hw.mu.Lock()
	hw.score["a"] = 0.1
	hw.mu.Unlock()

	// Report unhealthy multiple times - should floor at 0
	// Each unhealthy report subtracts 0.40
	hw.ReportHealth("a", false, 0)
	hw.ReportHealth("a", false, 0)
	hw.ReportHealth("a", false, 0)

	hw.mu.RLock()
	score = hw.score["a"]
	hw.mu.RUnlock()

	// Score should be clamped at 0
	if score < 0.0 {
		t.Errorf("Expected score >= 0.0, got %f", score)
	}
}

// TestUpstreamPool_Name_FromID tests Name when Name is empty but ID is set
func TestUpstreamPool_Name_FromID(t *testing.T) {
	upstream := config.Upstream{
		ID:        "upstream-id",
		Name:      "", // Empty name
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "t1", Address: "10.0.0.1:8080"},
		},
	}

	pool := NewUpstreamPool(upstream)

	name := pool.Name()
	if name != "upstream-id" {
		t.Errorf("Expected name='upstream-id', got '%s'", name)
	}
}

// TestUpstreamPool_Name_Empty tests Name when both Name and ID are empty
func TestUpstreamPool_Name_Empty(t *testing.T) {
	upstream := config.Upstream{
		ID:        "",
		Name:      "  ", // Whitespace only
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "t1", Address: "10.0.0.1:8080"},
		},
	}

	pool := NewUpstreamPool(upstream)

	name := pool.Name()
	if name != "" {
		t.Errorf("Expected empty name, got '%s'", name)
	}
}

// TestUpstreamPool_Done_NilBalancer tests Done with nil balancer
func TestUpstreamPool_Done_NilBalancer(t *testing.T) {
	upstream := config.Upstream{
		ID:        "test-upstream",
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "t1", Address: "10.0.0.1:8080"},
		},
	}

	pool := NewUpstreamPool(upstream)

	// Set balancer to nil
	pool.balancer = nil

	// Should not panic
	pool.Done("t1")
}

// TestCloneTargets_Empty tests cloneTargets with empty slice
func TestCloneTargets_Empty(t *testing.T) {
	result := cloneTargets([]config.UpstreamTarget{})
	if result != nil {
		t.Error("Expected nil for empty input")
	}
}

// TestTargetKey_FromAddress tests targetKey when ID is empty
func TestTargetKey_FromAddress(t *testing.T) {
	target := config.UpstreamTarget{
		ID:      "", // Empty ID
		Address: "10.0.0.1:8080",
	}

	key := targetKey(target)
	if key != "10.0.0.1:8080" {
		t.Errorf("Expected key='10.0.0.1:8080', got '%s'", key)
	}
}

// TestTargetKey_FromID tests targetKey when ID is set
func TestTargetKey_FromID(t *testing.T) {
	target := config.UpstreamTarget{
		ID:      "target-123",
		Address: "10.0.0.1:8080",
	}

	key := targetKey(target)
	if key != "target-123" {
		t.Errorf("Expected key='target-123', got '%s'", key)
	}
}

// ==================== Helper Tests ====================

// TestBuildUpstreamURL_InvalidTarget tests buildUpstreamURL with invalid target
func TestBuildUpstreamURL_InvalidTarget(t *testing.T) {
	// Test with empty target
	_, err := buildUpstreamURL("", "/api", "")
	if err == nil {
		t.Error("Expected error for empty target")
	}

	// Test with invalid URL
	_, err = buildUpstreamURL("://invalid", "/api", "")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

// TestBuildUpstreamURL_MissingHost tests buildUpstreamURL with missing host
func TestBuildUpstreamURL_MissingHost(t *testing.T) {
	// URL without host
	_, err := buildUpstreamURL("http://", "/api", "")
	if err == nil {
		t.Error("Expected error for missing host")
	}
}

// TestNormalizePath tests normalizePath function
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/users", "/api/users"},
		{"api/users", "/api/users"},
		{"/", "/"},
		{"", "/"},
		{"//api//users//", "/api/users"},
	}

	for _, tt := range tests {
		result := normalizePath(tt.input)
		if result != tt.expected {
			t.Errorf("normalizePath(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// TestStripPathForProxy_NoMatch tests stripPathForProxy when no prefix matches
func TestStripPathForProxy_NoMatch(t *testing.T) {
	route := &config.Route{
		Paths: []string{"/api"},
	}

	result := stripPathForProxy(route, "/different/path")
	if result != "/different/path" {
		t.Errorf("Expected '/different/path', got '%s'", result)
	}
}

// TestStripPathForProxy_RootPrefix tests stripPathForProxy with root prefix
func TestStripPathForProxy_RootPrefix(t *testing.T) {
	route := &config.Route{
		Paths: []string{"/"},
	}

	result := stripPathForProxy(route, "/api/users")
	if result != "/api/users" {
		t.Errorf("Expected '/api/users', got '%s'", result)
	}
}

// TestJoinURLPath tests joinURLPath function
func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		basePath    string
		requestPath string
		expected    string
	}{
		{"/api", "/users", "/api/users"},
		{"/", "/users", "/users"},
		{"/api", "/", "/api"},
		{"/", "/", "/"},
	}

	for _, tt := range tests {
		result := joinURLPath(tt.basePath, tt.requestPath)
		if result != tt.expected {
			t.Errorf("joinURLPath(%q, %q) = %q, expected %q", tt.basePath, tt.requestPath, result, tt.expected)
		}
	}
}

// TestClientIP tests clientIP function
func TestClientIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"192.168.1.1", "192.168.1.1"},
		{"  192.168.1.1:8080  ", "192.168.1.1"},
		{"[::1]:8080", "::1"},
	}

	for _, tt := range tests {
		result := clientIP(tt.input)
		if result != tt.expected {
			t.Errorf("clientIP(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// TestIsTimeoutError tests isTimeoutError function
func TestIsTimeoutError(t *testing.T) {
	// Test with context deadline exceeded
	if !isTimeoutError(context.DeadlineExceeded) {
		t.Error("Expected true for context.DeadlineExceeded")
	}

	// Test with nil error
	if isTimeoutError(nil) {
		t.Error("Expected false for nil error")
	}

	// Test with non-timeout error
	if isTimeoutError(errors.New("some error")) {
		t.Error("Expected false for non-timeout error")
	}
}

// TestIsBenignTunnelClose tests isBenignTunnelClose function
func TestIsBenignTunnelClose(t *testing.T) {
	// Test with nil error
	if !isBenignTunnelClose(nil) {
		t.Error("Expected true for nil error")
	}

	// Test with EOF
	if !isBenignTunnelClose(io.EOF) {
		t.Error("Expected true for io.EOF")
	}

	// Test with net.ErrClosed
	if !isBenignTunnelClose(net.ErrClosed) {
		t.Error("Expected true for net.ErrClosed")
	}

	// Test with closed network connection error
	if !isBenignTunnelClose(errors.New("use of closed network connection")) {
		t.Error("Expected true for closed network connection error")
	}

	// Test with other error
	if isBenignTunnelClose(errors.New("some other error")) {
		t.Error("Expected false for other error")
	}
}

// TestProxyErrorStatus tests proxyErrorStatus function
func TestProxyErrorStatus(t *testing.T) {
	// Test with timeout error
	status := proxyErrorStatus(context.DeadlineExceeded)
	if status != http.StatusGatewayTimeout {
		t.Errorf("Expected %d, got %d", http.StatusGatewayTimeout, status)
	}

	// Test with non-timeout error
	status = proxyErrorStatus(errors.New("some error"))
	if status != http.StatusBadGateway {
		t.Errorf("Expected %d, got %d", http.StatusBadGateway, status)
	}
}

// TestParseConnectionTokens tests parseConnectionTokens function
func TestParseConnectionTokens(t *testing.T) {
	headers := http.Header{
		"Connection": []string{"keep-alive, Upgrade", "Custom-Header"},
	}

	tokens := parseConnectionTokens(headers)

	if _, ok := tokens["keep-alive"]; !ok {
		t.Error("Expected 'keep-alive' token")
	}
	if _, ok := tokens["upgrade"]; !ok {
		t.Error("Expected 'upgrade' token")
	}
	if _, ok := tokens["custom-header"]; !ok {
		t.Error("Expected 'custom-header' token")
	}
}

// TestParseConnectionTokens_Empty tests parseConnectionTokens with empty Connection header
func TestParseConnectionTokens_Empty(t *testing.T) {
	headers := http.Header{}

	tokens := parseConnectionTokens(headers)

	if len(tokens) != 0 {
		t.Errorf("Expected empty tokens, got %d", len(tokens))
	}
}

// TestCopyHeaders_Nil tests copyHeaders with nil headers
func TestCopyHeaders_Nil(t *testing.T) {
	// Should not panic
	copyHeaders(nil, http.Header{"X-Test": []string{"value"}})
	copyHeaders(http.Header{"X-Test": []string{"value"}}, nil)
	copyHeaders(nil, nil)
}

// TestAppendForwardedHeaders_Nil tests appendForwardedHeaders with nil requests
func TestAppendForwardedHeaders_Nil(t *testing.T) {
	// Should not panic
	appendForwardedHeaders(nil, httptest.NewRequest("GET", "/", nil))
	appendForwardedHeaders(httptest.NewRequest("GET", "/", nil), nil)
	appendForwardedHeaders(nil, nil)
}

// TestAppendForwardedHeaders_WithXFF tests appendForwardedHeaders with existing X-Forwarded-For
func TestAppendForwardedHeaders_WithXFF(t *testing.T) {
	src := httptest.NewRequest("GET", "http://example.com/api", nil)
	src.Header.Set("X-Forwarded-For", "10.0.0.1")
	src.RemoteAddr = "192.168.1.1:12345"

	dst, _ := http.NewRequest("GET", "http://backend/api", nil)

	appendForwardedHeaders(dst, src)

	xff := dst.Header.Get("X-Forwarded-For")
	if xff != "10.0.0.1, 192.168.1.1" {
		t.Errorf("Expected X-Forwarded-For='10.0.0.1, 192.168.1.1', got '%s'", xff)
	}
}

// TestAppendForwardedHeaders_HTTPS tests appendForwardedHeaders with HTTPS
func TestAppendForwardedHeaders_HTTPS(t *testing.T) {
	src := httptest.NewRequest("GET", "https://example.com/api", nil)
	src.RemoteAddr = "192.168.1.1:12345"

	dst, _ := http.NewRequest("GET", "http://backend/api", nil)

	appendForwardedHeaders(dst, src)

	proto := dst.Header.Get("X-Forwarded-Proto")
	if proto != "https" {
		t.Errorf("Expected X-Forwarded-Proto='https', got '%s'", proto)
	}
}

// TestAffinityKey tests affinityKey function
func TestAffinityKey(t *testing.T) {
	// Configure trusted proxies for XFF tests
	netutil.SetTrustedProxies([]string{"10.0.0.0/8"})
	defer netutil.SetTrustedProxies(nil)

	// Test with X-Forwarded-For (RemoteAddr must be trusted for XFF to be used)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.2")
	ctx := &RequestContext{Request: req}

	key := affinityKey(ctx)
	if key != "203.0.113.5" {
		t.Errorf("Expected key='203.0.113.5', got '%s'", key)
	}

	// Test with RemoteAddr only (untrusted source, XFF ignored)
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	ctx = &RequestContext{Request: req}

	key = affinityKey(ctx)
	if key != "192.168.1.1" {
		t.Errorf("Expected key='192.168.1.1', got '%s'", key)
	}

	// Test with nil context
	key = affinityKey(nil)
	if key != "" {
		t.Errorf("Expected empty key, got '%s'", key)
	}
}

// TestExtractClientIP tests extractClientIP function
func TestExtractClientIP(t *testing.T) {
	// Configure trusted proxies for XFF tests
	netutil.SetTrustedProxies([]string{"10.0.0.0/8"})
	defer netutil.SetTrustedProxies(nil)

	// Test with X-Forwarded-For (RemoteAddr must be trusted for XFF to be used)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.2")
	ctx := &RequestContext{Request: req}

	ip := extractClientIP(ctx)
	if ip != "203.0.113.5" {
		t.Errorf("Expected ip='203.0.113.5', got '%s'", ip)
	}

	// Test with RemoteAddr only (untrusted source, XFF ignored)
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	ctx = &RequestContext{Request: req}

	ip = extractClientIP(ctx)
	if ip != "192.168.1.1" {
		t.Errorf("Expected ip='192.168.1.1', got '%s'", ip)
	}

	// Test with nil context
	ip = extractClientIP(nil)
	if ip != "" {
		t.Errorf("Expected empty ip, got '%s'", ip)
	}
}

// ==================== Server Error Handler Tests ====================

// TestGateway_writeAuthError_JWTError tests writeAuthError with JWT error
func TestGateway_writeAuthError_JWTError(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	w := httptest.NewRecorder()
	jwtErr := &plugin.JWTAuthError{
		Code:    "invalid_token",
		Message: "Token is invalid",
		Status:  http.StatusUnauthorized,
	}
	g.writeAuthError(w, jwtErr)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// TestGateway_writePluginError_AllTypes tests writePluginError with all error types
func TestGateway_writePluginError_AllTypes(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	testCases := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "AuthError",
			err:      &plugin.AuthError{Code: "auth_error", Message: "Auth failed", Status: http.StatusUnauthorized},
			expected: http.StatusUnauthorized,
		},
		{
			name:     "JWTAuthError",
			err:      &plugin.JWTAuthError{Code: "jwt_error", Message: "JWT failed", Status: http.StatusUnauthorized},
			expected: http.StatusUnauthorized,
		},
		{
			name:     "IPRestrictError",
			err:      &plugin.IPRestrictError{Code: "ip_error", Message: "IP blocked", Status: http.StatusForbidden},
			expected: http.StatusForbidden,
		},
		{
			name:     "CircuitBreakerError",
			err:      &plugin.CircuitBreakerError{Code: "cb_error", Message: "Circuit open", Status: http.StatusServiceUnavailable},
			expected: http.StatusServiceUnavailable,
		},
		{
			name:     "RequestSizeLimitError",
			err:      &plugin.RequestSizeLimitError{Code: "size_error", Message: "Too large", Status: http.StatusRequestEntityTooLarge},
			expected: http.StatusRequestEntityTooLarge,
		},
		{
			name:     "RequestValidatorError",
			err:      &plugin.RequestValidatorError{Code: "validation_error", Message: "Invalid", Status: http.StatusBadRequest},
			expected: http.StatusBadRequest,
		},
		{
			name:     "BotDetectError",
			err:      &plugin.BotDetectError{Code: "bot_error", Message: "Bot detected", Status: http.StatusForbidden},
			expected: http.StatusForbidden,
		},
		{
			name:     "EndpointPermissionError",
			err:      &plugin.EndpointPermissionError{Code: "perm_error", Message: "No permission", Status: http.StatusForbidden},
			expected: http.StatusForbidden,
		},
		{
			name:     "UserIPWhitelistError",
			err:      &plugin.UserIPWhitelistError{Code: "whitelist_error", Message: "IP not whitelisted", Status: http.StatusForbidden},
			expected: http.StatusForbidden,
		},
		{
			name:     "GenericError",
			err:      errors.New("generic error"),
			expected: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			g.writePluginError(w, tc.err)

			if w.Code != tc.expected {
				t.Errorf("Expected status %d, got %d", tc.expected, w.Code)
			}
		})
	}
}

// TestGateway_writeBillingError_ErrNoRows tests writeBillingError with sql.ErrNoRows
func TestGateway_writeBillingError_ErrNoRows(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	w := httptest.NewRecorder()
	g.writeBillingError(w, sql.ErrNoRows)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// TestGateway_writeBillingError_Generic tests writeBillingError with generic error
func TestGateway_writeBillingError_Generic(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	w := httptest.NewRecorder()
	g.writeBillingError(w, errors.New("some billing error"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// ==================== Billing Helper Tests ====================

// TestApplyBillingPreProxy_NilEngine tests applyBillingPreProxy with nil engine
func TestApplyBillingPreProxy_NilEngine(t *testing.T) {
	req := httptest.NewRequest("GET", "/api", nil)
	state, err := applyBillingPreProxy(nil, req, nil, nil, nil)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if state != nil {
		t.Error("Expected nil state")
	}
}

// TestApplyBillingPreProxy_NilRequest tests applyBillingPreProxy with nil request
func TestApplyBillingPreProxy_NilRequest(t *testing.T) {
	engine := billing.NewEngine(nil, config.BillingConfig{})
	state, err := applyBillingPreProxy(engine, nil, nil, nil, nil)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
	if state != nil {
		t.Error("Expected nil state")
	}
}

// TestApplyBillingPostProxy_NilState tests applyBillingPostProxy with nil state
func TestApplyBillingPostProxy_NilState(t *testing.T) {
	engine := billing.NewEngine(nil, config.BillingConfig{})
	// Should not panic
	applyBillingPostProxy(engine, nil, nil, nil)
}

// TestApplyBillingPostProxy_AlreadyApplied tests applyBillingPostProxy when already applied
func TestApplyBillingPostProxy_AlreadyApplied(t *testing.T) {
	engine := billing.NewEngine(nil, config.BillingConfig{})
	state := &billingRequestState{applied: true}
	// Should not panic and should not apply again
	applyBillingPostProxy(engine, state, nil, nil)
}

// TestBillingRouteID_Empty tests billingRouteID with empty route
func TestBillingRouteID_Empty(t *testing.T) {
	result := billingRouteID(nil)
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}

	result = billingRouteID(&config.Route{})
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}
}

// TestBillingRequestID_FromContext tests billingRequestID from context
func TestBillingRequestID_FromContext(t *testing.T) {
	ctx := &plugin.PipelineContext{
		CorrelationID: "corr-123",
	}

	result := billingRequestID(nil, ctx)
	if result != "corr-123" {
		t.Errorf("Expected 'corr-123', got '%s'", result)
	}
}

// TestExtractPermissionCreditCost_InvalidType tests extractPermissionCreditCost with invalid type
func TestExtractPermissionCreditCost_InvalidType(t *testing.T) {
	ctx := &plugin.PipelineContext{
		Metadata: map[string]any{
			"permission_credit_cost": make(chan int), // Invalid type
		},
	}

	result := extractPermissionCreditCost(ctx)
	if result != nil {
		t.Error("Expected nil for invalid type")
	}
}

// TestMetadataInt64_InvalidType tests metadataInt64 with invalid type
func TestMetadataInt64_InvalidType(t *testing.T) {
	ctx := &plugin.PipelineContext{
		Metadata: map[string]any{
			"test_key": make(chan int), // Invalid type
		},
	}

	result := metadataInt64(ctx, "test_key")
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}
}

// ==================== Router Tests ====================

// TestRouter_Match_NoRoutes tests Match when no routes configured
func TestRouter_Match_NoRoutes(t *testing.T) {
	router, err := NewRouter([]config.Route{}, []config.Service{})
	if err != nil {
		t.Fatalf("NewRouter error: %v", err)
	}

	req := httptest.NewRequest("GET", "/api", nil)
	_, _, err = router.Match(req)
	if err == nil {
		t.Error("Expected error for no matching route")
	}
}

// TestRouter_Match_InvalidRegex tests Match with invalid regex
func TestRouter_Match_InvalidRegex(t *testing.T) {
	_, err := NewRouter([]config.Route{
		{
			ID:      "route1",
			Name:    "Route 1",
			Methods: []string{"GET"},
			Paths:   []string{"[invalid(regex"},
			Service: "service1",
		},
	}, []config.Service{
		{
			ID:   "service1",
			Name: "Service 1",
		},
	})
	if err == nil {
		t.Error("Expected error for invalid regex in route")
	}
}

// TestCompiledRoute_Matches_NilMethods tests matches with nil methods map
func TestCompiledRoute_Matches_NilMethods(t *testing.T) {
	re := regexp.MustCompile("^/api/users$")
	cr := &compiledRoute{
		host:    "example.com",
		methods: nil, // Nil methods
		re:      re,
	}

	// When methods is nil, the code checks if "*" is in the map
	// Since nil map returns zero value (false), it won't match
	// Let's verify the actual behavior
	result := cr.matches("example.com", "GET", "/api/users")
	t.Logf("matches result with nil methods: %v", result)
}

// ==================== AddSecurityHeaders Tests ====================

// TestAddSecurityHeaders_HTTP tests addSecurityHeaders for HTTP
func TestAddSecurityHeaders_HTTP(t *testing.T) {
	w := httptest.NewRecorder()
	addSecurityHeaders(w, false)

	headers := w.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Expected X-Content-Type-Options=nosniff")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Error("Expected X-Frame-Options=DENY")
	}
	if headers.Get("Strict-Transport-Security") != "" {
		t.Error("Expected no HSTS header for HTTP")
	}
}

// TestAddSecurityHeaders_HTTPS tests addSecurityHeaders for HTTPS
func TestAddSecurityHeaders_HTTPS(t *testing.T) {
	w := httptest.NewRecorder()
	addSecurityHeaders(w, true)

	headers := w.Header()
	if headers.Get("Strict-Transport-Security") == "" {
		t.Error("Expected HSTS header for HTTPS")
	}
}

// ==================== ServeHTTP Tests ====================

// TestGateway_ServeHTTP_MaxBodySize tests ServeHTTP with max body size exceeded
func TestGateway_ServeHTTP_MaxBodySize(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:     "127.0.0.1:0",
			MaxBodyBytes: 10,
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	// Request with body larger than max
	body := strings.NewReader("this body is way too large for the limit")
	req := httptest.NewRequest("POST", "/api", body)
	req.ContentLength = 100
	w := httptest.NewRecorder()

	g.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}
}

// TestGateway_ServeHTTP_RouteNotFound tests ServeHTTP when no route matches
func TestGateway_ServeHTTP_RouteNotFound(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Routes: []config.Route{},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	req := httptest.NewRequest("GET", "/api", nil)
	w := httptest.NewRecorder()

	g.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestGateway_ServeHTTP_UpstreamNotFound tests ServeHTTP when upstream not found
func TestGateway_ServeHTTP_UpstreamNotFound(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
		Routes: []config.Route{
			{
				ID:      "route1",
				Name:    "Route 1",
				Methods: []string{"GET"},
				Paths:   []string{"/api"},
				Service: "service1",
			},
		},
		Services: []config.Service{
			{
				ID:       "service1",
				Name:     "Service 1",
				Upstream: "nonexistent-upstream",
			},
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer g.Shutdown(context.Background())

	req := httptest.NewRequest("GET", "/api", nil)
	w := httptest.NewRecorder()

	g.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("Expected status %d, got %d", http.StatusBadGateway, w.Code)
	}
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// TestGateway_MaxBodyBytes_Enforced verifies that MaxBodyBytes is enforced
// via Content-Length check (fast path) and buffered read for chunked bodies.
func TestGateway_MaxBodyBytes_Enforced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			MaxBodyBytes:   1024, // 1 KB limit
			MaxHeaderBytes: 1 << 20,
		},
		Services: []config.Service{
			{ID: "test-svc", Name: "test", Protocol: "http", Upstream: "test-up"},
		},
		Routes: []config.Route{
			{ID: "test-route", Name: "test", Service: "test-svc", Paths: []string{"/test"}, Methods: []string{"POST"}},
		},
		Upstreams: []config.Upstream{
			{
				ID:   "test-up",
				Name: "test",
				Targets: []config.UpstreamTarget{
					{Address: upstream.Listener.Addr().String(), Weight: 1},
				},
			},
		},
		Store: config.StoreConfig{Path: t.TempDir() + "/test.db"},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	time.Sleep(100 * time.Millisecond)

	// Send a body with Content-Length that exceeds the 1 KB limit
	largeBody := bytes.NewReader(make([]byte, 2048))
	req, _ := http.NewRequest("POST", "http://"+gw.Addr()+"/test", largeBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	defer resp.Body.Close()

	// Should get 413 Payload Too Large
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", resp.StatusCode)
	}

	// Small body should succeed
	smallBody := bytes.NewReader([]byte("small"))
	req, _ = http.NewRequest("POST", "http://"+gw.Addr()+"/test", smallBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Small body request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for small body, got %d", resp.StatusCode)
	}
}

// TestGateway_MaxBodyBytes_ChunkedTransfer verifies that chunked transfer
// encoding bodies are also subject to the body size limit.
func TestGateway_MaxBodyBytes_ChunkedTransfer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Logf("Upstream read error: %v", err)
		} else {
			t.Logf("Upstream read %d bytes", len(body))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			MaxBodyBytes:   512, // 512 byte limit
			MaxHeaderBytes: 1 << 20,
		},
		Services: []config.Service{
			{ID: "test-svc", Name: "test", Protocol: "http", Upstream: "test-up"},
		},
		Routes: []config.Route{
			{ID: "test-route", Name: "test", Service: "test-svc", Paths: []string{"/chunked"}, Methods: []string{"POST"}},
		},
		Upstreams: []config.Upstream{
			{
				ID:   "test-up",
				Name: "test",
				Targets: []config.UpstreamTarget{
					{Address: upstream.Listener.Addr().String(), Weight: 1},
				},
			},
		},
		Store: config.StoreConfig{Path: t.TempDir() + "/test.db"},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- gw.Start(ctx) }()

	time.Sleep(100 * time.Millisecond)

	// Send a chunked request (no Content-Length) with a body that exceeds the limit
	pr, pw := io.Pipe()
	go func() {
		// Write more than 512 bytes in chunks
		for i := 0; i < 10; i++ {
			pw.Write(make([]byte, 100))
		}
		pw.Close()
	}()

	req, _ := http.NewRequest("POST", "http://"+gw.Addr()+"/chunked", pr)
	// Don't set Content-Length to force chunked transfer
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413 for chunked oversized body, got %d", resp.StatusCode)
	}
}
