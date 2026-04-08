package tracing

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestMiddleware_Disabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: false,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	middleware := NewMiddleware(tracer)

	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", rec.Body.String())
	}
}

func TestMiddleware_Enabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Exporter:       "stdout",
		SamplingRate:   1.0,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	middleware := NewMiddleware(tracer)

	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "test-api-key-12345")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestMiddleware_ErrorStatus(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	middleware := NewMiddleware(tracer)

	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestMiddleware_ClientError(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	middleware := NewMiddleware(tracer)

	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestMiddleware_WithXForwardedHeaders(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	middleware := NewMiddleware(tracer)

	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("User-Agent", "TestAgent/1.0")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestMiddleware_NilTracer(t *testing.T) {
	middleware := NewMiddleware(nil)

	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestGetScheme(t *testing.T) {
	tests := []struct {
		name     string
		tls      bool
		header   string
		expected string
	}{
		{
			name:     "HTTPS via TLS",
			tls:      true,
			expected: "https",
		},
		{
			name:     "HTTP no TLS",
			tls:      false,
			expected: "http",
		},
		{
			name:     "HTTPS via header",
			tls:      false,
			header:   "https",
			expected: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.tls {
				// Create a mock TLS connection state
				req.TLS = &tls.ConnectionState{}
			}
			if tt.header != "" {
				req.Header.Set("X-Forwarded-Proto", tt.header)
			}

			scheme := getScheme(req)
			if scheme != tt.expected {
				t.Errorf("getScheme() = %v, want %v", scheme, tt.expected)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		expected   string
	}{
		{
			name:     "X-Forwarded-For",
			xff:      "10.0.0.1, 10.0.0.2",
			expected: "10.0.0.1, 10.0.0.2",
		},
		{
			name:     "X-Real-Ip",
			xri:      "10.0.0.3",
			expected: "10.0.0.3",
		},
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "192.168.1.1:1234",
			expected:   "192.168.1.1:1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-Ip", tt.xri)
			}
			if tt.remoteAddr != "" {
				req.RemoteAddr = tt.remoteAddr
			}

			ip := getClientIP(req)
			if ip != tt.expected {
				t.Errorf("getClientIP() = %v, want %v", ip, tt.expected)
			}
		})
	}
}

func TestResponseWriter_WrapsResponse(t *testing.T) {
	rec := httptest.NewRecorder()

	rw := &responseWriter{
		ResponseWriter: rec,
		statusCode:     http.StatusOK,
	}

	// Test WriteHeader
	rw.WriteHeader(http.StatusCreated)
	if rw.statusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, rw.statusCode)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("Expected recorder code %d, got %d", http.StatusCreated, rec.Code)
	}

	// Test Write
	content := []byte("Hello, World!")
	n, err := rw.Write(content)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(content) {
		t.Errorf("Write() = %d, want %d", n, len(content))
	}
	if rw.bytesWritten != int64(len(content)) {
		t.Errorf("bytesWritten = %d, want %d", rw.bytesWritten, len(content))
	}

	// Test Header
	if rw.Header() == nil {
		t.Error("Header() returned nil")
	}
}

func TestResponseWriter_AutoWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()

	rw := &responseWriter{
		ResponseWriter: rec,
		statusCode:     http.StatusOK,
	}

	// Write without calling WriteHeader first
	content := []byte("Test")
	rw.Write(content)

	if !rw.wroteHeader {
		t.Error("Expected wroteHeader to be true after Write")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected recorder code %d, got %d", http.StatusOK, rec.Code)
	}
}
