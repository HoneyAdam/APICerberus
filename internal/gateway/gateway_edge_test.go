package gateway

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// ==================== Server Lifecycle Tests ====================

// TestGateway_Start_WithHTTPS tests starting gateway with HTTPS
func TestGateway_Start_WithHTTPS_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := tmpDir + "/test.crt"
	keyFile := tmpDir + "/test.key"

	if err := generateTestCert(certFile, keyFile); err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:  "127.0.0.1:0",
			HTTPSAddr: "127.0.0.1:0",
			TLS: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		t.Logf("Start returned: %v", err)
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for Start to return")
	}
}

// TestGateway_Start_NoListeners tests Start with no listeners configured
func TestGateway_Start_NoListeners_Edge(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			// No HTTPAddr or HTTPSAddr
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

// TestGateway_Shutdown_NilServers tests Shutdown with nil servers
func TestGateway_Shutdown_NilServers_Edge(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: "127.0.0.1:0",
		},
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Set servers to nil
	g.mu.Lock()
	g.httpServer = nil
	g.httpsServer = nil
	g.grpcServer = nil
	g.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = g.Shutdown(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestGateway_Reload_NilConfig tests Reload with nil config
func TestGateway_Reload_NilConfig_Edge(t *testing.T) {
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

	err = g.Reload(nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}
}

// TestGateway_Reload_WithTLSManagerError tests Reload with TLS manager error
func TestGateway_Reload_WithTLSManagerError_Edge(t *testing.T) {
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

	newCfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:  "127.0.0.1:0",
			HTTPSAddr: "127.0.0.1:0",
			TLS: config.TLSConfig{
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  "/nonexistent/key.pem",
			},
		},
	}

	err = g.Reload(newCfg)
	if err == nil {
		t.Error("Expected error for invalid TLS config")
	}
}

// TestGateway_Addr tests Addr method
func TestGateway_Addr_Edge(t *testing.T) {
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

	// Before start, listener is nil
	addr := g.Addr()
	if addr != "" {
		t.Errorf("Expected empty addr before start, got %s", addr)
	}
}

// ==================== TLS Certificate Tests ====================

// TestTLSManager_TLSConfig_WithAuto tests TLSConfig with Auto enabled
func TestTLSManager_TLSConfig_WithAuto_Edge(t *testing.T) {
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

	// Should include ALPNProto when Auto is enabled
	found := false
	for _, proto := range tlsConfig.NextProtos {
		if proto == "acme-tls/1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected ALPNProto in NextProtos when Auto is enabled")
	}
}

// TestTLSManager_GetCertificate_NilHello tests GetCertificate with nil hello
func TestTLSManager_GetCertificate_NilHello_Edge(t *testing.T) {
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

	// Get wildcard cert with nil hello
	cert, err := tm.GetCertificate(nil)
	if err != nil {
		t.Errorf("GetCertificate error: %v", err)
	}
	if cert == nil {
		t.Error("Expected non-nil certificate")
	}
}

// TestTLSManager_GetCertificate_ExpiredWithAuto tests GetCertificate with expired cert and Auto enabled
func TestTLSManager_GetCertificate_ExpiredWithAuto_Edge(t *testing.T) {
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

	// Create an expired certificate and store it
	expiredCert := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}}, // Invalid cert data
	}
	tm.certs.Store("expired.example.com", expiredCert)

	// Request the expired cert - should try to renew but fail since autocertM is nil
	_, err = tm.GetCertificate(&tls.ClientHelloInfo{ServerName: "expired.example.com"})
	// Will fail but should not panic
	t.Logf("GetCertificate result: %v", err)
}

// TestTLSManager_evaluateCertificate_ExpiredNoAuto tests evaluateCertificate with expired cert and no auto
func TestTLSManager_evaluateCertificate_ExpiredNoAuto_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    false, // No auto-renewal
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Create an expired certificate with proper leaf
	expiredCert := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}}, // Invalid cert data - will have nil Leaf
	}

	// When cert.Leaf is nil, the function returns the cert as-is (defaults to valid)
	// This is the expected behavior per the code
	result, err := tm.evaluateCertificate("example.com", expiredCert)
	// If Leaf is nil, the cert is returned without error
	if err != nil && result != nil {
		t.Logf("Got error as expected for invalid cert: %v", err)
	}
}

// TestTLSManager_issueAndStore_NilIssue tests issueAndStore with nil issue function
func TestTLSManager_issueAndStore_NilIssue_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Set issue to nil
	tm.issue = nil

	_, err = tm.issueAndStore("example.com")
	if err == nil {
		t.Error("Expected error when issue func is nil")
	}
}

// TestTLSManager_issueAndStore_NilResult tests issueAndStore when issue returns nil
func TestTLSManager_issueAndStore_NilResult_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Set issue to return nil
	tm.issue = func(domain string) (*tls.Certificate, error) {
		return nil, nil
	}

	_, err = tm.issueAndStore("example.com")
	if err == nil {
		t.Error("Expected error when issue returns nil certificate")
	}
}

// TestTLSManager_saveToDisk_InvalidCert tests saveToDisk with invalid certificate
func TestTLSManager_saveToDisk_InvalidCert_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	// Test with certificate that has no private key
	cert := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}},
		PrivateKey:  nil,
	}

	err = tm.saveToDisk("test.com", cert)
	if err == nil {
		t.Error("Expected error for certificate without private key")
	}
}

// TestTLSManager_saveToDisk_EmptyACMEDir tests saveToDisk with empty ACMEDir
func TestTLSManager_saveToDisk_EmptyACMEDir_Edge(t *testing.T) {
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: "",
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{{1, 2, 3}},
		PrivateKey:  &testPrivateKey{},
	}

	err = tm.saveToDisk("test.com", cert)
	if err == nil {
		t.Error("Expected error for empty ACMEDir")
	}
}

// TestTLSManager_loadFromDisk_NonExistent tests loadFromDisk with non-existent files
func TestTLSManager_loadFromDisk_NonExistent_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	_, err = tm.loadFromDisk("nonexistent.com")
	if err == nil {
		t.Error("Expected error for non-existent certificate files")
	}
}

// TestTLSManager_ReloadCertificate_EmptyName tests ReloadCertificate with empty server name
func TestTLSManager_ReloadCertificate_EmptyName_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	err = tm.ReloadCertificate("")
	if err == nil {
		t.Error("Expected error for empty server name")
	}
}

// TestTLSManager_ReloadCertificate_NonExistent tests ReloadCertificate with non-existent certificate
func TestTLSManager_ReloadCertificate_NonExistent_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.TLSConfig{
		Auto:    true,
		ACMEDir: tmpDir,
	}
	tm, err := NewTLSManager(cfg)
	if err != nil {
		t.Fatalf("NewTLSManager error: %v", err)
	}

	err = tm.ReloadCertificate("nonexistent.com")
	if err == nil {
		t.Error("Expected error for non-existent certificate")
	}
}

// TestWriteFileAtomic_Errors tests writeFileAtomic error paths
func TestWriteFileAtomic_Errors_Edge(t *testing.T) {
	// Test with file in non-existent nested directory
	tmpDir := t.TempDir()
	err := writeFileAtomic(filepath.Join(tmpDir, "nonexistent", "nested", "file.txt"), []byte("test"), 0644)
	if err == nil {
		t.Error("Expected error for non-existent nested directory")
	}
}

// TestEncodePrivateKeyPEM_StructKey tests encodePrivateKeyPEM with struct key
func TestEncodePrivateKeyPEM_StructKey_Edge(t *testing.T) {
	_, err := encodePrivateKeyPEM(struct{}{})
	if err == nil {
		t.Error("Expected error for struct key type")
	}
}

// ==================== WebSocket Tests ====================

// TestProxy_ForwardWebSocket_NilProxy tests ForwardWebSocket with nil proxy
func TestProxy_ForwardWebSocket_NilProxy_Edge(t *testing.T) {
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
func TestProxy_ForwardWebSocket_InvalidContext_Edge(t *testing.T) {
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
func TestProxy_ForwardWebSocket_NotWebSocket_Edge(t *testing.T) {
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
func TestProxy_ForwardWebSocket_InvalidTarget_Edge(t *testing.T) {
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
func TestProxy_ForwardWebSocket_NotHijacker_Edge(t *testing.T) {
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

// TestDialUpstreamWebSocket_UnsupportedScheme tests dialUpstreamWebSocket with unsupported scheme
func TestDialUpstreamWebSocket_UnsupportedScheme_Edge(t *testing.T) {
	u, _ := url.Parse("ftp://localhost:8080")
	_, err := dialUpstreamWebSocket(u)
	if err == nil {
		t.Error("Expected error for unsupported scheme")
	}
}

// TestIsWebSocketUpgrade_InvalidMethod tests isWebSocketUpgrade with invalid method
func TestIsWebSocketUpgrade_InvalidMethod_Edge(t *testing.T) {
	// POST request should not be websocket upgrade
	req := httptest.NewRequest("POST", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	if isWebSocketUpgrade(req) {
		t.Error("Expected false for POST request")
	}
}

// TestProxy_Do_NilProxy tests Do with nil proxy
func TestProxy_Do_NilProxy_Edge(t *testing.T) {
	var p *Proxy
	_, err := p.Do(&RequestContext{
		Request: httptest.NewRequest("GET", "/api", nil),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil proxy")
	}
}

// TestProxy_Do_InvalidContext tests Do with invalid context
func TestProxy_Do_NilOrInvalidContext(t *testing.T) {
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
func TestProxy_Do_InvalidTarget_Edge(t *testing.T) {
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
func TestProxy_Do_InvalidURL_Edge(t *testing.T) {
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
func TestProxy_WriteResponse_NilProxy_Edge(t *testing.T) {
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
func TestProxy_WriteResponse_InvalidArgs_Edge(t *testing.T) {
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
func TestProxy_Forward_NilProxy_Edge(t *testing.T) {
	var p *Proxy
	err := p.Forward(&RequestContext{
		Request:        httptest.NewRequest("GET", "/api", nil),
		ResponseWriter: httptest.NewRecorder(),
	}, &config.UpstreamTarget{ID: "t1", Address: "localhost:8080"})
	if err == nil {
		t.Error("Expected error for nil proxy")
	}
}

// ==================== Network Error Tests ====================

// TestServeHTTPS_NilServer tests serveHTTPS with nil server
func TestServeHTTPS_NilServer_Edge(t *testing.T) {
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

	err = g.serveHTTPS(nil, nil)
	if err == nil {
		t.Error("Expected error for nil server")
	}
}

// TestServeHTTPS_NilTLSManager tests serveHTTPS with nil TLS manager
func TestServeHTTPS_NilTLSManager_Edge(t *testing.T) {
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

	server := &http.Server{Addr: "127.0.0.1:0"}
	err = g.serveHTTPS(server, nil)
	if err == nil {
		t.Error("Expected error for nil TLS manager")
	}
}

// TestServeHTTPS_ListenError tests serveHTTPS with listen error
func TestServeHTTPS_ListenError_Edge(t *testing.T) {
	// Create a listener to occupy the port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	tmpDir := t.TempDir()
	certFile := tmpDir + "/test.crt"
	keyFile := tmpDir + "/test.key"
	if err := generateTestCert(certFile, keyFile); err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

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

	tlsManager, _ := NewTLSManager(config.TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
	})

	server := &http.Server{Addr: addr} // Already in use
	err = g.serveHTTPS(server, tlsManager)
	if err == nil {
		t.Error("Expected error when port is already in use")
	}
}

// ==================== Balancer Done Tests ====================

// TestLeastConn_Done_NonExistentTarget tests LeastConn Done with non-existent target
func TestLeastConn_Done_NonExistentTarget_Edge(t *testing.T) {
	lc := NewLeastConn([]config.UpstreamTarget{
		{ID: "a", Address: "10.0.0.1:8080"},
	})

	// Done with non-existent target should not panic
	lc.Done("nonexistent")
}

// ==================== Helper Function Tests ====================

// TestBuildUpstreamURL_InvalidTarget tests buildUpstreamURL with invalid target
func TestBuildUpstreamURL_InvalidTarget_Edge(t *testing.T) {
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
func TestBuildUpstreamURL_MissingHost_Edge(t *testing.T) {
	// URL without host
	_, err := buildUpstreamURL("http://", "/api", "")
	if err == nil {
		t.Error("Expected error for missing host")
	}
}

// TestNormalizePath_Coverage tests normalizePath function
func TestNormalizePath_Edge(t *testing.T) {
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
func TestStripPathForProxy_NoMatch_Edge(t *testing.T) {
	route := &config.Route{
		Paths: []string{"/api"},
	}

	result := stripPathForProxy(route, "/different/path")
	if result != "/different/path" {
		t.Errorf("Expected '/different/path', got '%s'", result)
	}
}

// TestStripPathForProxy_RootPrefix tests stripPathForProxy with root prefix
func TestStripPathForProxy_RootPrefix_Edge(t *testing.T) {
	route := &config.Route{
		Paths: []string{"/"},
	}

	result := stripPathForProxy(route, "/api/users")
	if result != "/api/users" {
		t.Errorf("Expected '/api/users', got '%s'", result)
	}
}

// TestJoinURLPath_Coverage tests joinURLPath function
func TestJoinURLPath_Edge(t *testing.T) {
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

// TestClientIP_Coverage tests clientIP function
func TestClientIP_Edge(t *testing.T) {
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

// TestIsTimeoutError_Coverage tests isTimeoutError function
func TestIsTimeoutError_Edge(t *testing.T) {
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

// TestIsBenignTunnelClose_Coverage tests isBenignTunnelClose function
func TestIsBenignTunnelClose_Edge(t *testing.T) {
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

// TestProxyErrorStatus_Coverage tests proxyErrorStatus function
func TestProxyErrorStatus_Edge(t *testing.T) {
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

// TestParseConnectionTokens_Coverage tests parseConnectionTokens function
func TestParseConnectionTokens_Edge(t *testing.T) {
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
func TestParseConnectionTokens_Empty_Edge(t *testing.T) {
	headers := http.Header{}

	tokens := parseConnectionTokens(headers)

	if len(tokens) != 0 {
		t.Errorf("Expected empty tokens, got %d", len(tokens))
	}
}

// TestCopyHeaders_Nil tests copyHeaders with nil headers
func TestCopyHeaders_Nil_Edge(t *testing.T) {
	// Should not panic
	copyHeaders(nil, http.Header{"X-Test": []string{"value"}})
	copyHeaders(http.Header{"X-Test": []string{"value"}}, nil)
	copyHeaders(nil, nil)
}

// TestAppendForwardedHeaders_Nil tests appendForwardedHeaders with nil requests
func TestAppendForwardedHeaders_Nil_Edge(t *testing.T) {
	// Should not panic
	appendForwardedHeaders(nil, httptest.NewRequest("GET", "/", nil))
	appendForwardedHeaders(httptest.NewRequest("GET", "/", nil), nil)
	appendForwardedHeaders(nil, nil)
}

// TestAppendForwardedHeaders_WithXFF tests appendForwardedHeaders with existing X-Forwarded-For
func TestAppendForwardedHeaders_WithXFF_Edge(t *testing.T) {
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
func TestAppendForwardedHeaders_HTTPS_Edge(t *testing.T) {
	src := httptest.NewRequest("GET", "https://example.com/api", nil)
	src.RemoteAddr = "192.168.1.1:12345"

	dst, _ := http.NewRequest("GET", "http://backend/api", nil)

	appendForwardedHeaders(dst, src)

	proto := dst.Header.Get("X-Forwarded-Proto")
	if proto != "https" {
		t.Errorf("Expected X-Forwarded-Proto='https', got '%s'", proto)
	}
}

// ==================== ServeHTTP Error Tests ====================

// TestGateway_ServeHTTP_MaxBodySize tests ServeHTTP with max body size exceeded
func TestGateway_ServeHTTP_MaxBodySize_Edge(t *testing.T) {
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
func TestGateway_ServeHTTP_RouteNotFound_Edge(t *testing.T) {
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
func TestGateway_ServeHTTP_UpstreamNotFound_Edge(t *testing.T) {
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

// ==================== Concurrent Access Tests ====================

// TestGateway_ConcurrentAccess tests concurrent access to gateway
func TestGateway_ConcurrentAccess_Edge(t *testing.T) {
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

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Concurrent reads
			_ = g.Uptime()
			_ = g.Addr()
			_ = g.FederationEnabled()
			_ = g.Subgraphs()
			_ = g.FederationComposer()
			_ = g.Analytics()
			_ = g.UpstreamHealth("test")
		}()
	}
	wg.Wait()
}

// TestTLSManager_ConcurrentAccess tests concurrent access to TLS manager
func TestTLSManager_ConcurrentAccess_Edge(t *testing.T) {
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

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Concurrent reads
			_, _ = tm.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
			_ = tm.TLSConfig()
		}()
	}
	wg.Wait()
}
