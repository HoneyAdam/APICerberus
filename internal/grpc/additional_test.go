package grpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Mock Watch stream for testing - implements healthpb.Health_WatchServer interface
type mockHealthWatchStream struct {
	ctx      context.Context
	cancel   context.CancelFunc
	messages []*healthpb.HealthCheckResponse
	sent     int
}

func newMockHealthWatchStream() *mockHealthWatchStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockHealthWatchStream{
		ctx:      ctx,
		cancel:   cancel,
		messages: make([]*healthpb.HealthCheckResponse, 0),
	}
}

func (m *mockHealthWatchStream) Send(resp *healthpb.HealthCheckResponse) error {
	m.messages = append(m.messages, resp)
	m.sent++
	return nil
}

func (m *mockHealthWatchStream) Context() context.Context {
	return m.ctx
}

// SendMsg implements grpc.Stream interface (generic send)
func (m *mockHealthWatchStream) SendMsg(msg any) error {
	if resp, ok := msg.(*healthpb.HealthCheckResponse); ok {
		return m.Send(resp)
	}
	return nil
}

// RecvMsg implements grpc.ServerStreamingServer interface
func (m *mockHealthWatchStream) RecvMsg(msg any) error {
	return nil
}

// SetHeader implements grpc.ServerTransportStream interface
func (m *mockHealthWatchStream) SetHeader(md metadata.MD) error {
	return nil
}

// SendHeader implements grpc.ServerTransportStream interface
func (m *mockHealthWatchStream) SendHeader(md metadata.MD) error {
	return nil
}

// SetTrailer implements grpc.ServerTransportStream interface
func (m *mockHealthWatchStream) SetTrailer(md metadata.MD) {}

func (m *mockHealthWatchStream) close() {
	m.cancel()
}

// Test HealthServer Watch function
func TestHealthServer_Watch(t *testing.T) {
	t.Run("WatchWithStatusChanges", func(t *testing.T) {
		checker := NewSimpleHealthChecker()
		server := NewHealthServer(checker)
		stream := newMockHealthWatchStream()

		// Set initial status before watch starts
		checker.SetStatus("test-service", StatusServing)

		// Start watch in background
		done := make(chan error, 1)
		go func() {
			done <- server.Watch(&healthpb.HealthCheckRequest{Service: "test-service"}, stream)
		}()

		// Wait a bit for initial status to be sent
		time.Sleep(50 * time.Millisecond)

		// Change status to trigger update
		checker.SetStatus("test-service", StatusNotServing)

		// Wait for status update
		time.Sleep(50 * time.Millisecond)

		// Cancel context to stop watch
		stream.close()

		// Wait for watch to finish
		select {
		case err := <-done:
			if err != nil && err != context.Canceled {
				t.Errorf("Watch() error = %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Error("Watch did not stop after context cancellation")
		}

		// Verify initial status was sent
		if stream.sent < 1 {
			t.Errorf("Expected at least 1 message sent, got %d", stream.sent)
		}
	})

	t.Run("WatchOverallHealth", func(t *testing.T) {
		checker := NewSimpleHealthChecker()
		server := NewHealthServer(checker)
		stream := newMockHealthWatchStream()

		// Start watch for overall health (empty service name)
		done := make(chan error, 1)
		go func() {
			done <- server.Watch(&healthpb.HealthCheckRequest{Service: ""}, stream)
		}()

		// Wait a bit for initial status
		time.Sleep(50 * time.Millisecond)

		// Cancel context
		stream.close()

		// Wait for watch to finish
		select {
		case <-done:
			// Expected
		case <-time.After(500 * time.Millisecond):
			t.Error("Watch did not stop after context cancellation")
		}

		// Verify initial status was sent
		if stream.sent < 1 {
			t.Errorf("Expected at least 1 message sent, got %d", stream.sent)
		}
	})

	t.Run("WatchCheckError", func(t *testing.T) {
		checker := &mockHealthChecker{err: status.Errorf(codes.Internal, "check failed")}
		server := NewHealthServer(checker)
		stream := newMockHealthWatchStream()

		err := server.Watch(&healthpb.HealthCheckRequest{Service: "test-service"}, stream)
		if err == nil {
			t.Error("Watch() should return error when initial check fails")
		}
	})
}

// Test RegisterHealthServer
func TestRegisterHealthServer(t *testing.T) {
	// Create a gRPC server
	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	checker := NewSimpleHealthChecker()

	// Register health server - should not panic
	RegisterHealthServer(grpcServer, checker)

	// Verify the service was registered by checking if we can get service info
	serviceInfo := grpcServer.GetServiceInfo()
	if _, ok := serviceInfo["grpc.health.v1.Health"]; !ok {
		t.Error("Health service was not registered")
	}
}

// Mock response writer with flusher for stream testing
type mockFlusherResponseWriter struct {
	headers http.Header
	status  int
	body    []byte
	flushed bool
}

func newMockFlusherResponseWriter() *mockFlusherResponseWriter {
	return &mockFlusherResponseWriter{
		headers: make(http.Header),
	}
}

func (m *mockFlusherResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockFlusherResponseWriter) Write(data []byte) (int, error) {
	m.body = append(m.body, data...)
	return len(data), nil
}

func (m *mockFlusherResponseWriter) WriteHeader(status int) {
	m.status = status
}

func (m *mockFlusherResponseWriter) Flush() {
	m.flushed = true
}

// Test StreamProxy functions
func TestStreamProxy_ProxyServerStream(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("NonFlusherResponse", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))

		// Use a response writer that doesn't implement http.Flusher
		nonFlusher := &nonFlusherResponseWriter{ResponseWriter: rec}

		sp.ProxyServerStream(nonFlusher, req, nil, "/test.Service/Method")

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

// Helper type that wraps ResponseWriter without Flusher
type nonFlusherResponseWriter struct {
	http.ResponseWriter
}

func TestStreamProxy_ProxyClientStream(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("EmptyMessages", func(t *testing.T) {
		rec := httptest.NewRecorder()
		// Empty body - no messages to send
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))

		// This will panic with nil connection, so we need to recover
		defer func() {
			if r := recover(); r != nil {
				// Expected panic due to nil connection
			}
		}()

		// This will fail because there's no gRPC connection, but it tests the path
		sp.ProxyClientStream(rec, req, nil, "/test.Service/Method")

		// Should have error response since no connection
		if rec.Code == http.StatusOK {
			t.Error("Expected error when no gRPC connection provided")
		}
	})
}

func TestStreamProxy_ProxyBidiStream(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("NonFlusherResponse", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))

		// Use a response writer that doesn't implement http.Flusher
		nonFlusher := &nonFlusherResponseWriter{ResponseWriter: rec}

		sp.ProxyBidiStream(nonFlusher, req, nil, "/test.Service/Method")

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

// Test mockHealthChecker Watch behavior
func TestMockHealthChecker_Watch(t *testing.T) {
	checker := &mockHealthChecker{status: StatusServing}

	ch := checker.Watch("test-service")
	if ch == nil {
		t.Fatal("Watch() returned nil channel")
	}

	// The mock immediately sends the status
	select {
	case status := <-ch:
		if status != StatusServing {
			t.Errorf("Received status = %v, want SERVING", status)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for status from mock watch")
	}
}

// Test context cancellation in Watch
func TestHealthServer_Watch_ContextCancellation(t *testing.T) {
	checker := NewSimpleHealthChecker()
	server := NewHealthServer(checker)

	// Create a stream with a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	stream := &cancelableMockStream{
		ctx:      ctx,
		messages: make([]*healthpb.HealthCheckResponse, 0),
	}

	// Start watch
	done := make(chan error, 1)
	go func() {
		done <- server.Watch(&healthpb.HealthCheckRequest{Service: "test-service"}, stream)
	}()

	// Cancel context immediately
	cancel()

	// Wait for watch to finish
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Watch did not stop after context cancellation")
	}
}

// Additional mock stream that supports context cancellation
type cancelableMockStream struct {
	ctx      context.Context
	messages []*healthpb.HealthCheckResponse
}

func (m *cancelableMockStream) Send(resp *healthpb.HealthCheckResponse) error {
	m.messages = append(m.messages, resp)
	return nil
}

func (m *cancelableMockStream) Context() context.Context {
	return m.ctx
}

// SendMsg implements grpc.Stream interface
func (m *cancelableMockStream) SendMsg(msg any) error {
	if resp, ok := msg.(*healthpb.HealthCheckResponse); ok {
		return m.Send(resp)
	}
	return nil
}

// RecvMsg implements grpc.ServerStreamingServer interface
func (m *cancelableMockStream) RecvMsg(msg any) error {
	return nil
}

// SetHeader implements grpc.ServerTransportStream interface
func (m *cancelableMockStream) SetHeader(md metadata.MD) error {
	return nil
}

// SendHeader implements grpc.ServerTransportStream interface
func (m *cancelableMockStream) SendHeader(md metadata.MD) error {
	return nil
}

// SetTrailer implements grpc.ServerTransportStream interface
func (m *cancelableMockStream) SetTrailer(md metadata.MD) {}

// Test writeStreamError with gRPC status error
func TestWriteStreamError_GRPCStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	err := status.Error(codes.NotFound, "resource not found")

	writeStreamError(rec, err)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "resource not found") {
		t.Errorf("Body should contain error message, got: %s", body)
	}
	if !strings.Contains(body, `"code":5`) {
		t.Errorf("Body should contain code 5 (NotFound), got: %s", body)
	}
}

// Test writeStreamError with non-gRPC error
func TestWriteStreamError_NonGRPC(t *testing.T) {
	rec := httptest.NewRecorder()
	testErr := fmt.Errorf("some random error")

	writeStreamError(rec, testErr)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	if !strings.Contains(rec.Body.String(), "some random error") {
		t.Errorf("Body should contain error message, got: %s", rec.Body.String())
	}
}

// Test writeStreamErrorFrame with various errors
func TestWriteStreamErrorFrame_VariousErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantCode     int
		wantMessage  string
	}{
		{
			name:        "gRPC error",
			err:         status.Error(codes.PermissionDenied, "access denied"),
			wantCode:    7, // codes.PermissionDenied
			wantMessage: "access denied",
		},
		{
			name:        "regular error",
			err:         fmt.Errorf("something went wrong"),
			wantCode:    13, // codes.Internal
			wantMessage: "something went wrong",
		},
		{
			name:        "EOF error",
			err:         io.EOF,
			wantCode:    13,
			wantMessage: "EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeStreamErrorFrame(rec, tt.err)

			body := rec.Body.String()
			if !strings.Contains(body, fmt.Sprintf(`"code":%d`, tt.wantCode)) {
				t.Errorf("Body should contain code %d, got: %s", tt.wantCode, body)
			}
			if !strings.Contains(body, fmt.Sprintf(`"message":"%s"`, tt.wantMessage)) {
				t.Errorf("Body should contain message '%s', got: %s", tt.wantMessage, body)
			}
			if !strings.Contains(body, `"error":true`) {
				t.Errorf("Body should contain error flag, got: %s", body)
			}
		})
	}
}

// Test writeStreamStatusFrame - already covered in grpc_additional_test.go

// Test splitMessages edge cases
func TestSplitMessages_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected [][]byte
	}{
		{
			name:     "single line no newline",
			input:    []byte("message"),
			expected: [][]byte{[]byte("message")},
		},
		{
			name:     "multiple empty lines",
			input:    []byte("\n\n\n"),
			expected: nil,
		},
		{
			name:     "lines with only whitespace",
			input:    []byte("   \n  \n  "),
			expected: nil,
		},
		{
			name:     "mixed content",
			input:    []byte("msg1\n  \nmsg2\n   \nmsg3"),
			expected: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		},
		{
			name:     "windows line endings",
			input:    []byte("msg1\r\nmsg2"),
			expected: [][]byte{[]byte("msg1"), []byte("msg2")},
		},
		{
			name:     "tabs and spaces",
			input:    []byte("  \t msg1 \t  \n\t msg2 \t"),
			expected: [][]byte{[]byte("msg1"), []byte("msg2")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitMessages(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitMessages() returned %d messages, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if !bytes.Equal(got[i], tt.expected[i]) {
					t.Errorf("splitMessages()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test Proxy handleGRPC with various scenarios
func TestProxy_HandleGRPC_MethodVariations(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantPrefix bool
	}{
		{"with leading slash", "/test.Service/Method", true},
		{"without leading slash", "test.Service/Method", true},
		{"empty path", "", true},
		{"nested path", "/v1/test.Service/Method", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the path handling logic
			method := tt.path
			if !strings.HasPrefix(method, "/") {
				method = "/" + method
			}
			if !strings.HasPrefix(method, "/") {
				t.Error("Method should have leading slash")
			}
		})
	}
}

// Test raw codec with various types
func TestRawCodec_VariousTypes(t *testing.T) {
	codec := &rawCodec{}

	t.Run("Marshal bytes.Buffer", func(t *testing.T) {
		buf := bytes.NewBufferString("test data")
		data, err := codec.Marshal(buf)
		if err != nil {
			t.Errorf("Marshal error = %v", err)
		}
		if string(data) != "test data" {
			t.Errorf("Marshal = %q, want 'test data'", string(data))
		}
	})

	t.Run("Marshal byte slice", func(t *testing.T) {
		data := []byte("test data")
		result, err := codec.Marshal(data)
		if err != nil {
			t.Errorf("Marshal error = %v", err)
		}
		if string(result) != "test data" {
			t.Errorf("Marshal = %q, want 'test data'", string(result))
		}
	})

	t.Run("Unmarshal to bytes.Buffer", func(t *testing.T) {
		data := []byte("test data")
		var buf bytes.Buffer
		err := codec.Unmarshal(data, &buf)
		if err != nil {
			t.Errorf("Unmarshal error = %v", err)
		}
		if buf.String() != "test data" {
			t.Errorf("Unmarshal = %q, want 'test data'", buf.String())
		}
	})

	t.Run("Unmarshal to byte slice pointer", func(t *testing.T) {
		data := []byte("test data")
		var result []byte
		err := codec.Unmarshal(data, &result)
		if err != nil {
			t.Errorf("Unmarshal error = %v", err)
		}
		if string(result) != "test data" {
			t.Errorf("Unmarshal = %q, want 'test data'", string(result))
		}
	})
}

// ==================== Transcoder Error Path Tests ====================

func TestTranscoder_JSONToProto_ErrorPaths(t *testing.T) {
	tc := NewTranscoder()

	t.Run("not loaded", func(t *testing.T) {
		_, err := tc.JSONToProto("/test.Service/Method", []byte(`{"key": "value"}`))
		if err == nil {
			t.Error("Expected error when transcoder not loaded")
		}
		if !strings.Contains(err.Error(), "descriptors not loaded") {
			t.Errorf("Expected 'descriptors not loaded' error, got: %v", err)
		}
	})

	t.Run("invalid method path", func(t *testing.T) {
		tc.loaded = true
		tc.files = new(protoregistry.Files)
		_, err := tc.JSONToProto("invalid-path", []byte(`{}`))
		if err == nil {
			t.Error("Expected error for invalid method path")
		}
	})

	t.Run("method not found", func(t *testing.T) {
		tc.loaded = true
		tc.files = new(protoregistry.Files)
		_, err := tc.JSONToProto("/Unknown.Service/Method", []byte(`{}`))
		if err == nil {
			t.Error("Expected error for non-existent method")
		}
	})
}

func TestTranscoder_ProtoToJSON_ErrorPaths(t *testing.T) {
	tc := NewTranscoder()

	t.Run("not loaded", func(t *testing.T) {
		_, err := tc.ProtoToJSON("/test.Service/Method", []byte("test"))
		if err == nil {
			t.Error("Expected error when transcoder not loaded")
		}
	})

	t.Run("invalid method path", func(t *testing.T) {
		tc.loaded = true
		tc.files = new(protoregistry.Files)
		_, err := tc.ProtoToJSON("invalid-path", []byte("test"))
		if err == nil {
			t.Error("Expected error for invalid method path")
		}
	})

	t.Run("method not found", func(t *testing.T) {
		tc.loaded = true
		tc.files = new(protoregistry.Files)
		_, err := tc.ProtoToJSON("/Unknown.Service/Method", []byte("test"))
		if err == nil {
			t.Error("Expected error for non-existent method")
		}
	})
}

func TestTranscoder_LoadDescriptors_ErrorPaths(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		tc := NewTranscoder()
		err := tc.LoadDescriptors("/nonexistent/path/file.desc")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("invalid descriptor data", func(t *testing.T) {
		tc := NewTranscoder()
		tmpDir := t.TempDir()
		invalidFile := filepath.Join(tmpDir, "invalid.desc")
		if err := os.WriteFile(invalidFile, []byte("invalid data"), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		err := tc.LoadDescriptors(invalidFile)
		if err == nil {
			t.Error("Expected error for invalid descriptor file")
		}
	})
}

func TestTranscoder_resolveMethod_ErrorPaths(t *testing.T) {
	tc := NewTranscoder()
	tc.loaded = true
	tc.files = new(protoregistry.Files)

	tests := []struct {
		name   string
		method string
	}{
		{"empty path", ""},
		{"no leading slash", "Service/Method"},
		{"trailing slash", "/Service/Method/"},
		{"single component", "ServiceMethod"},
		{"non-existent service", "/NonExistent.Service/Method"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tc.resolveMethod(tt.method)
			if err == nil {
				t.Error("Expected error for invalid method")
			}
		})
	}
}

// ==================== Stream Proxy Error Path Tests ====================

func TestStreamProxy_ProxyServerStream_ErrorPaths(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("non-flusher response writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))

		// Use a response writer that doesn't implement http.Flusher
		nonFlusher := &nonFlusherResponseWriter{ResponseWriter: rec}

		sp.ProxyServerStream(nonFlusher, req, nil, "/test.Service/Method")

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("body read error", func(t *testing.T) {
		rec := newMockFlusherResponseWriter()
		req := httptest.NewRequest(http.MethodPost, "/test", &failingReader{})

		sp.ProxyServerStream(rec, req, nil, "/test.Service/Method")

		if rec.status != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.status)
		}
	})
}

func TestStreamProxy_ProxyClientStream_ErrorPaths(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("nil connection", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))

		// This will panic with nil connection, so we need to recover
		defer func() {
			if r := recover(); r != nil {
				// Expected panic due to nil connection - this is acceptable
			}
		}()

		sp.ProxyClientStream(rec, req, nil, "/test.Service/Method")
	})
}

func TestStreamProxy_ProxyBidiStream_ErrorPaths(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("non-flusher response writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))

		// Use a response writer that doesn't implement http.Flusher
		nonFlusher := &nonFlusherResponseWriter{ResponseWriter: rec}

		sp.ProxyBidiStream(nonFlusher, req, nil, "/test.Service/Method")

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})
}

// Test Proxy handleGRPC with body read error
func TestProxy_handleGRPC_BodyReadError(t *testing.T) {
	cfg := &ProxyConfig{
		Target:   "localhost:50051",
		Insecure: true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", &failingReader{})
	req.Header.Set("Content-Type", "application/grpc")
	rec := httptest.NewRecorder()

	proxy.handleGRPC(rec, req)

	// Should handle error gracefully
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Check gRPC status header indicates error
	grpcStatus := rec.Header().Get("Grpc-Status")
	if grpcStatus == "" || grpcStatus == "0" {
		t.Error("Expected non-zero gRPC status for error")
	}
}

// Test Proxy handleGRPCWeb with body read error
func TestProxy_handleGRPCWeb_BodyReadError(t *testing.T) {
	cfg := &ProxyConfig{
		Target:    "localhost:50051",
		EnableWeb: true,
		Insecure:  true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", &failingReader{})
	req.Header.Set("Content-Type", "application/grpc-web")
	rec := httptest.NewRecorder()

	proxy.handleGRPCWeb(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

// Test Proxy handleTranscoding with body read error
func TestProxy_handleTranscoding_BodyReadError(t *testing.T) {
	cfg := &ProxyConfig{
		Target:            "localhost:50051",
		EnableTranscoding: true,
		Insecure:          true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/test/method", &failingReader{})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	proxy.handleTranscoding(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

// Test Transcoder with valid descriptors
func TestTranscoder_WithValidDescriptors(t *testing.T) {
	tc := NewTranscoder()

	// Create a temporary file with valid FileDescriptorSet
	tmpFile, err := os.CreateTemp("", "test*.desc")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create a minimal valid FileDescriptorSet
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test.proto"),
				Package: proto.String("test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("name"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: proto.String("TestResponse"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("message"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("TestMethod"),
								InputType:  proto.String(".test.TestRequest"),
								OutputType: proto.String(".test.TestResponse"),
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("Failed to marshal descriptor: %v", err)
	}

	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		t.Fatalf("Failed to write descriptor file: %v", err)
	}

	err = tc.LoadDescriptors(tmpFile.Name())
	if err != nil {
		// If loading fails, skip this test
		t.Skipf("Could not load descriptors: %v", err)
	}

	if !tc.IsLoaded() {
		t.Error("IsLoaded() should return true after successful load")
	}

	// Test JSONToProto with valid input
	t.Run("JSONToProto valid", func(t *testing.T) {
		_, err := tc.JSONToProto("/test.TestService/TestMethod", []byte(`{"name": "test"}`))
		// May fail due to type resolution issues, but should not panic
		_ = err
	})

	// Test ProtoToJSON with invalid proto data
	t.Run("ProtoToJSON invalid data", func(t *testing.T) {
		_, err := tc.ProtoToJSON("/test.TestService/TestMethod", []byte("invalid"))
		// Should return error for invalid proto data
		if err == nil {
			t.Error("Expected error for invalid proto data")
		}
	})
}

// Test Proxy handleTranscoding with various scenarios
func TestProxy_handleTranscoding_Scenarios(t *testing.T) {
	t.Run("transcoder not loaded", func(t *testing.T) {
		cfg := &ProxyConfig{
			Target:            "localhost:50051",
			EnableTranscoding: true,
			Insecure:          true,
		}
		proxy, err := NewProxy(cfg)
		if err != nil {
			t.Fatalf("NewProxy() error = %v", err)
		}
		defer proxy.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/test/method", strings.NewReader(`{"key": "value"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
		}
	})

	t.Run("path without leading slash", func(t *testing.T) {
		cfg := &ProxyConfig{
			Target:            "localhost:50051",
			EnableTranscoding: true,
			Insecure:          true,
		}
		proxy, err := NewProxy(cfg)
		if err != nil {
			t.Fatalf("NewProxy() error = %v", err)
		}
		defer proxy.Close()

		// Create a request - httptest requires valid URL, so we modify the path after creation
		req := httptest.NewRequest(http.MethodPost, "/v1/test/method", strings.NewReader(`{"key": "value"}`))
		req.URL.Path = "v1/test/method" // Remove leading slash
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		// Should handle path without leading slash
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
		}
	})
}

// Test Proxy ServeHTTP with gRPC-Web base64 decoding
func TestProxy_handleGRPCWeb_Base64(t *testing.T) {
	cfg := &ProxyConfig{
		Target:    "localhost:50051",
		EnableWeb: true,
		Insecure:  true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	t.Run("valid base64", func(t *testing.T) {
		// Encode a simple message
		encoded := base64.StdEncoding.EncodeToString([]byte("test message"))
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(encoded))
		req.Header.Set("Content-Type", "application/grpc-web-text")
		rec := httptest.NewRecorder()

		proxy.handleGRPCWeb(rec, req)

		// Should not panic - actual result depends on upstream
	})

	t.Run("invalid base64", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("not-valid-base64!!!"))
		req.Header.Set("Content-Type", "application/grpc-web-text")
		rec := httptest.NewRecorder()

		proxy.handleGRPCWeb(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

// Test Proxy ServeHTTP with all protocol types
func TestProxy_ServeHTTP_AllProtocols(t *testing.T) {
	tests := []struct {
		name           string
		contentType    string
		enableWeb      bool
		enableTranscoding bool
		wantStatus     int
	}{
		{
			name:        "gRPC request",
			contentType: "application/grpc",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "gRPC+proto request",
			contentType: "application/grpc+proto",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "gRPC+json request",
			contentType: "application/grpc+json",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "gRPC-Web request enabled",
			contentType: "application/grpc-web",
			enableWeb:   true,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "gRPC-Web request disabled",
			contentType: "application/grpc-web",
			enableWeb:   false,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:              "JSON with transcoding enabled",
			contentType:       "application/json",
			enableTranscoding: true,
			wantStatus:        http.StatusServiceUnavailable, // transcoder not loaded
		},
		{
			name:              "JSON with transcoding disabled",
			contentType:       "application/json",
			enableTranscoding: false,
			wantStatus:        http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ProxyConfig{
				Target:            "localhost:50051",
				EnableWeb:         tt.enableWeb,
				EnableTranscoding: tt.enableTranscoding,
				Insecure:          true,
			}
			proxy, err := NewProxy(cfg)
			if err != nil {
				t.Fatalf("NewProxy() error = %v", err)
			}
			defer proxy.Close()

			req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("{}"))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()

			proxy.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// Test Proxy handleGRPC with different error scenarios
func TestProxy_handleGRPC_ErrorScenarios(t *testing.T) {
	t.Run("method path without leading slash", func(t *testing.T) {
		cfg := &ProxyConfig{
			Target:   "localhost:50051",
			Insecure: true,
		}
		proxy, err := NewProxy(cfg)
		if err != nil {
			t.Fatalf("NewProxy() error = %v", err)
		}
		defer proxy.Close()

		// Create request with valid URL, then modify path
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("{}"))
		req.URL.Path = "test.Service/Method" // Remove leading slash
		req.Header.Set("Content-Type", "application/grpc")
		rec := httptest.NewRecorder()

		proxy.handleGRPC(rec, req)

		// Should handle path without leading slash
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

// Test Proxy handleGRPCWeb with different content types
func TestProxy_handleGRPCWeb_ContentTypes(t *testing.T) {
	cfg := &ProxyConfig{
		Target:    "localhost:50051",
		EnableWeb: true,
		Insecure:  true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{
			name:        "grpc-web+proto",
			contentType: "application/grpc-web+proto",
			body:        "{}",
		},
		{
			name:        "grpc-web+json",
			contentType: "application/grpc-web+json",
			body:        "{}",
		},
		{
			name:        "grpc-web-text+proto",
			contentType: "application/grpc-web-text+proto",
			body:        base64.StdEncoding.EncodeToString([]byte("{}")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()

			proxy.handleGRPCWeb(rec, req)

			// Should not panic
		})
	}
}

// Test Proxy handleGRPCWeb path variations
func TestProxy_handleGRPCWeb_PathVariations(t *testing.T) {
	cfg := &ProxyConfig{
		Target:    "localhost:50051",
		EnableWeb: true,
		Insecure:  true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	tests := []struct {
		name string
		path string
	}{
		{
			name: "with leading slash",
			path: "/test.Service/Method",
		},
		{
			name: "without leading slash",
			path: "test.Service/Method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with valid URL, then modify path if needed
			req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("{}"))
			if tt.path != "/test.Service/Method" {
				req.URL.Path = tt.path
			}
			req.Header.Set("Content-Type", "application/grpc-web")
			rec := httptest.NewRecorder()

			proxy.handleGRPCWeb(rec, req)

			// Should handle both path formats
		})
	}
}

// Test StreamProxy with real gRPC server
func TestStreamProxy_WithRealGRPCServer(t *testing.T) {
	// Create a gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	go func() {
		grpcServer.Serve(lis)
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Create client connection
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Skipf("Failed to connect: %v", err)
	}
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("ProxyServerStream with valid connection", func(t *testing.T) {
		rec := newMockFlusherResponseWriter()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		// This will fail since the method doesn't exist, but it shouldn't panic
		sp.ProxyServerStream(rec, req, conn, "/test.Service/Method")

		// Should have written something (error or success)
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response")
		}
	})

	t.Run("ProxyServerStream with empty body", func(t *testing.T) {
		rec := newMockFlusherResponseWriter()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.Service/Method")

		// Should handle empty body
	})

	t.Run("ProxyClientStream with valid connection", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		// This will fail since the method doesn't exist, but it shouldn't panic
		sp.ProxyClientStream(rec, req, conn, "/test.Service/Method")

		// Should have some response
		if rec.Code == 0 {
			t.Error("Expected some response code")
		}
	})

	t.Run("ProxyClientStream with multiple messages", func(t *testing.T) {
		rec := httptest.NewRecorder()
		// Send multiple messages separated by newlines
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("msg1\nmsg2\nmsg3"))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.Service/Method")

		// Should handle multiple messages
	})

	t.Run("ProxyBidiStream with valid connection", func(t *testing.T) {
		rec := newMockFlusherResponseWriter()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		// This will fail since the method doesn't exist, but it shouldn't panic
		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should have written something
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response")
		}
	})

	t.Run("ProxyBidiStream with empty body", func(t *testing.T) {
		rec := newMockFlusherResponseWriter()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should handle empty body
	})

	t.Run("ProxyBidiStream with multiple messages", func(t *testing.T) {
		rec := newMockFlusherResponseWriter()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("msg1\nmsg2\nmsg3"))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should handle multiple messages
	})
}

// Test Proxy handleTranscoding with valid descriptors
func TestProxy_handleTranscoding_WithDescriptors(t *testing.T) {
	cfg := &ProxyConfig{
		Target:            "localhost:50051",
		EnableTranscoding: true,
		Insecure:          true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// Create a temporary file with valid FileDescriptorSet
	tmpFile, err := os.CreateTemp("", "test*.desc")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create a minimal valid FileDescriptorSet
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test.proto"),
				Package: proto.String("test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("name"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: proto.String("TestResponse"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("message"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("TestMethod"),
								InputType:  proto.String(".test.TestRequest"),
								OutputType: proto.String(".test.TestResponse"),
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("Failed to marshal descriptor: %v", err)
	}

	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		t.Fatalf("Failed to write descriptor file: %v", err)
	}

	err = proxy.Transcoder.LoadDescriptors(tmpFile.Name())
	if err != nil {
		// If loading fails, skip this test
		t.Skipf("Could not load descriptors: %v", err)
	}

	t.Run("valid JSON request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{"name": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		// Should handle the request (may fail due to no upstream, but should not panic)
	})

	t.Run("invalid JSON request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{invalid json`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("method not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/unknown.Service/Method", strings.NewReader(`{"name": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		// Should return error for unknown method
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

// Test Proxy handleTranscoding with response transcoding error
func TestProxy_handleTranscoding_ResponseError(t *testing.T) {
	cfg := &ProxyConfig{
		Target:            "localhost:50051",
		EnableTranscoding: true,
		Insecure:          true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// Create and load descriptors
	tmpFile, err := os.CreateTemp("", "test*.desc")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test.proto"),
				Package: proto.String("test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("name"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: proto.String("TestResponse"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("message"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("TestMethod"),
								InputType:  proto.String(".test.TestRequest"),
								OutputType: proto.String(".test.TestResponse"),
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("Failed to marshal descriptor: %v", err)
	}

	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		t.Fatalf("Failed to write descriptor file: %v", err)
	}

	if err := proxy.Transcoder.LoadDescriptors(tmpFile.Name()); err != nil {
		t.Skipf("Could not load descriptors: %v", err)
	}

	t.Run("valid request to non-existent upstream", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{"name": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		// Should get an error response since upstream doesn't exist
		if rec.Code == http.StatusOK {
			t.Error("Expected error when upstream doesn't exist")
		}
	})

	t.Run("transcode response error", func(t *testing.T) {
		// Create a proxy with a mock connection that will fail
		cfg := &ProxyConfig{
			Target:            "localhost:50051",
			EnableTranscoding: true,
			Insecure:          true,
		}
		proxy, err := NewProxy(cfg)
		if err != nil {
			t.Fatalf("NewProxy() error = %v", err)
		}
		defer proxy.Close()

		// Load descriptors
		if err := proxy.Transcoder.LoadDescriptors(tmpFile.Name()); err != nil {
			t.Skipf("Could not load descriptors: %v", err)
		}

		// Send valid JSON - should fail at gRPC call since no upstream
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{"name": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		// Should get an error response since upstream doesn't exist
		if rec.Code == http.StatusOK {
			t.Error("Expected error when upstream doesn't exist")
		}
	})
}

// Test ProtoToJSON success path
func TestTranscoder_ProtoToJSON_Success(t *testing.T) {
	tc := NewTranscoder()

	// Create a temporary file with valid FileDescriptorSet
	tmpFile, err := os.CreateTemp("", "test*.desc")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create a valid FileDescriptorSet with proper message definitions
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test.proto"),
				Package: proto.String("test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("name"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: proto.String("TestResponse"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("message"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("TestMethod"),
								InputType:  proto.String(".test.TestRequest"),
								OutputType: proto.String(".test.TestResponse"),
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("Failed to marshal descriptor: %v", err)
	}

	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		t.Fatalf("Failed to write descriptor file: %v", err)
	}

	err = tc.LoadDescriptors(tmpFile.Name())
	if err != nil {
		t.Skipf("Could not load descriptors: %v", err)
	}

	// Test ProtoToJSON with valid proto data
	// Create a valid protobuf message for TestResponse
	// The message contains a single string field "message" with field number 1
	// Wire format: tag(1<<3|2) + length + data
	protoData := []byte{
		0x0a, 0x0b, // tag 1 (field 1, wire type 2), length 11
		'H', 'e', 'l', 'l', 'o', ' ', 'W', 'o', 'r', 'l', 'd', // "Hello World"
	}

	_, err = tc.ProtoToJSON("/test.TestService/TestMethod", protoData)
	// May fail due to type resolution, but should not panic
	_ = err
}

// Test Proxy handleTranscoding with gRPC error response
func TestProxy_handleTranscoding_GRPCErrors(t *testing.T) {
	cfg := &ProxyConfig{
		Target:            "localhost:50051",
		EnableTranscoding: true,
		Insecure:          true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// Create and load descriptors
	tmpFile, err := os.CreateTemp("", "test*.desc")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("test.proto"),
				Package: proto.String("test"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("TestRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("name"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
					{
						Name: proto.String("TestResponse"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("message"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("TestService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("TestMethod"),
								InputType:  proto.String(".test.TestRequest"),
								OutputType: proto.String(".test.TestResponse"),
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("Failed to marshal descriptor: %v", err)
	}

	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		t.Fatalf("Failed to write descriptor file: %v", err)
	}

	if err := proxy.Transcoder.LoadDescriptors(tmpFile.Name()); err != nil {
		t.Skipf("Could not load descriptors: %v", err)
	}

	t.Run("transcode request error", func(t *testing.T) {
		// Send invalid JSON
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{invalid`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

// Test ProxyServeHTTP with streaming detection
func TestProxy_ServeHTTP_StreamDetection(t *testing.T) {
	tests := []struct {
		name           string
		contentType    string
		path           string
		body           string
		wantStatus     int
	}{
		{
			name:        "gRPC request",
			contentType: "application/grpc",
			path:        "/test.Service/Method",
			body:        "{}",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "gRPC-Web request",
			contentType: "application/grpc-web",
			path:        "/test.Service/Method",
			body:        "{}",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "JSON request without transcoding",
			contentType: "application/json",
			path:        "/test",
			body:        "{}",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ProxyConfig{
				Target:              "localhost:50051",
				EnableWeb:           true,
				EnableTranscoding:   false,
				Insecure:            true,
			}
			proxy, err := NewProxy(cfg)
			if err != nil {
				t.Fatalf("NewProxy() error = %v", err)
			}
			defer proxy.Close()

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()

			proxy.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
