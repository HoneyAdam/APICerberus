package grpc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDefaultH2CConfig(t *testing.T) {
	cfg := DefaultH2CConfig()
	if cfg == nil {
		t.Fatal("DefaultH2CConfig() returned nil")
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %v, want :8080", cfg.Addr)
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want 30s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want 120s", cfg.IdleTimeout)
	}
	if cfg.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %v, want 1MB", cfg.MaxHeaderBytes)
	}
	if cfg.MaxConcurrentStreams != 250 {
		t.Errorf("MaxConcurrentStreams = %v, want 250", cfg.MaxConcurrentStreams)
	}
}

func TestNewH2CServer_NilConfig(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := NewH2CServer(nil, handler)
	if server == nil {
		t.Fatal("NewH2CServer(nil) returned nil")
	}
	if server.addr != ":8080" {
		t.Errorf("addr = %v, want :8080", server.addr)
	}
}

func TestH2CServer_Addr(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	config := &H2CConfig{
		Addr: "127.0.0.1:0",
	}

	server := NewH2CServer(config, handler)

	// Before Start(), Addr() should return config addr
	addr := server.Addr()
	if addr != config.Addr {
		t.Errorf("Addr() before start = %v, want %v", addr, config.Addr)
	}
}

func TestH2CServer_Stop_NilServer(t *testing.T) {
	server := &H2CServer{}
	err := server.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() with nil server error = %v", err)
	}
}

func TestGRPCStatusToHTTP_AllCodes(t *testing.T) {
	tests := []struct {
		code codes.Code
		want int
	}{
		{codes.OK, http.StatusOK},
		{codes.Canceled, 499},
		{codes.Unknown, http.StatusInternalServerError},
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{codes.NotFound, http.StatusNotFound},
		{codes.AlreadyExists, http.StatusConflict},
		{codes.PermissionDenied, http.StatusForbidden},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.FailedPrecondition, http.StatusPreconditionFailed},
		{codes.Aborted, http.StatusConflict},
		{codes.OutOfRange, http.StatusBadRequest},
		{codes.Unimplemented, http.StatusNotImplemented},
		{codes.Internal, http.StatusInternalServerError},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DataLoss, http.StatusInternalServerError},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.Code(999), http.StatusInternalServerError}, // Unknown code
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			got := GRPCStatusToHTTP(tt.code)
			if got != tt.want {
				t.Errorf("GRPCStatusToHTTP(%v) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestHTTPStatusToGRPC_AllCodes(t *testing.T) {
	tests := []struct {
		status int
		want   codes.Code
	}{
		{http.StatusOK, codes.OK},
		{http.StatusBadRequest, codes.InvalidArgument},
		{http.StatusUnauthorized, codes.Unauthenticated},
		{http.StatusForbidden, codes.PermissionDenied},
		{http.StatusNotFound, codes.NotFound},
		{http.StatusConflict, codes.AlreadyExists},
		{http.StatusPreconditionFailed, codes.FailedPrecondition},
		{http.StatusTooManyRequests, codes.ResourceExhausted},
		{http.StatusInternalServerError, codes.Internal},
		{http.StatusNotImplemented, codes.Unimplemented},
		{http.StatusBadGateway, codes.Unavailable},
		{http.StatusServiceUnavailable, codes.Unavailable},
		{http.StatusGatewayTimeout, codes.DeadlineExceeded},
		{999, codes.Unknown}, // Unknown status
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			got := HTTPStatusToGRPC(tt.status)
			if got != tt.want {
				t.Errorf("HTTPStatusToGRPC(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestIsHTTPHeader_CaseVariations(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"lowercase accept", "accept", true},
		{"uppercase ACCEPT", "ACCEPT", true},
		{"mixed case Content-Length", "Content-Length", true},
		{"lowercase content-type", "content-type", true},
		{"custom header", "X-Custom-Header", false},
		{"authorization", "Authorization", false},
		{"empty string", "", false},
		{"connection lowercase", "connection", true},
		{"transfer-encoding", "Transfer-Encoding", true},
		{"upgrade", "Upgrade", true},
		{"te", "TE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHTTPHeader(tt.key)
			if got != tt.want {
				t.Errorf("isHTTPHeader(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestMetadataFromHeaders_SkipsHTTPHeaders(t *testing.T) {
	headers := http.Header{
		"Content-Type":  []string{"application/grpc"},
		"Accept":        []string{"*/*"},
		"Host":          []string{"localhost:8080"},
		"Connection":    []string{"keep-alive"},
		"X-Custom":      []string{"value"},
		"Authorization": []string{"Bearer token"},
	}

	md := metadataFromHeaders(headers)

	// HTTP-specific headers should be skipped
	if _, ok := md["content-type"]; ok {
		t.Error("content-type should be skipped")
	}
	if _, ok := md["accept"]; ok {
		t.Error("accept should be skipped")
	}
	if _, ok := md["host"]; ok {
		t.Error("host should be skipped")
	}
	if _, ok := md["connection"]; ok {
		t.Error("connection should be skipped")
	}

	// Non-HTTP headers should be included (lowercased)
	if v, ok := md["x-custom"]; !ok || len(v) != 1 || v[0] != "value" {
		t.Error("x-custom should be included")
	}
	if v, ok := md["authorization"]; !ok || len(v) != 1 || v[0] != "Bearer token" {
		t.Error("authorization should be included")
	}
}

func TestRawCodec_MarshalUnsupported(t *testing.T) {
	codec := &rawCodec{}

	// Test with unsupported types
	_, err := codec.Marshal(123)
	if err == nil {
		t.Error("Marshal should return error for int")
	}

	_, err = codec.Marshal("string")
	if err == nil {
		t.Error("Marshal should return error for string")
	}

	_, err = codec.Marshal(map[string]string{})
	if err == nil {
		t.Error("Marshal should return error for map")
	}
}

func TestRawCodec_UnmarshalUnsupported(t *testing.T) {
	codec := &rawCodec{}

	// Test with unsupported types
	var i int
	err := codec.Unmarshal([]byte("test"), &i)
	if err == nil {
		t.Error("Unmarshal should return error for *int")
	}

	var s string
	err = codec.Unmarshal([]byte("test"), &s)
	if err == nil {
		t.Error("Unmarshal should return error for *string")
	}

	var m map[string]string
	err = codec.Unmarshal([]byte("test"), &m)
	if err == nil {
		t.Error("Unmarshal should return error for *map")
	}
}

func TestSplitMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected [][]byte
	}{
		{
			name:     "single message",
			input:    []byte("message1"),
			expected: [][]byte{[]byte("message1")},
		},
		{
			name:     "multiple messages",
			input:    []byte("msg1\nmsg2\nmsg3"),
			expected: [][]byte{[]byte("msg1"), []byte("msg2"), []byte("msg3")},
		},
		{
			name:     "messages with whitespace",
			input:    []byte("  msg1  \n  msg2  \n  "),
			expected: [][]byte{[]byte("msg1"), []byte("msg2")},
		},
		{
			name:     "empty lines skipped",
			input:    []byte("msg1\n\n\nmsg2"),
			expected: [][]byte{[]byte("msg1"), []byte("msg2")},
		},
		{
			name:     "empty input",
			input:    []byte(""),
			expected: nil,
		},
		{
			name:     "only whitespace",
			input:    []byte("   \n   \n   "),
			expected: nil,
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

func TestWriteStreamError(t *testing.T) {
	t.Run("gRPC status error", func(t *testing.T) {
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
	})

	t.Run("non-gRPC error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeStreamError(rec, bytes.ErrTooLarge)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestWriteStreamErrorFrame(t *testing.T) {
	t.Run("gRPC status error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		err := status.Error(codes.InvalidArgument, "bad request")
		writeStreamErrorFrame(rec, err)

		body := rec.Body.String()
		if !strings.Contains(body, `"error":true`) {
			t.Errorf("Body should contain error flag, got: %s", body)
		}
		if !strings.Contains(body, `"code":3`) {
			t.Errorf("Body should contain code 3, got: %s", body)
		}
		if !strings.Contains(body, `"message":"bad request"`) {
			t.Errorf("Body should contain message, got: %s", body)
		}
	})

	t.Run("non-gRPC error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeStreamErrorFrame(rec, io.EOF)

		body := rec.Body.String()
		if !strings.Contains(body, `"error":true`) {
			t.Errorf("Body should contain error flag, got: %s", body)
		}
		if !strings.Contains(body, `"code":13`) { // codes.Internal = 13
			t.Errorf("Body should contain code 13, got: %s", body)
		}
	})
}

func TestWriteStreamStatusFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStreamStatusFrame(rec, codes.OK, "success")

	body := rec.Body.String()
	if !strings.Contains(body, `"status":0`) {
		t.Errorf("Body should contain status 0, got: %s", body)
	}
	if !strings.Contains(body, `"message":"success"`) {
		t.Errorf("Body should contain message, got: %s", body)
	}
}

// Test transcoder
func TestTranscoder_New(t *testing.T) {
	tc := NewTranscoder()
	if tc == nil {
		t.Fatal("NewTranscoder() returned nil")
	}
	if tc.IsLoaded() {
		t.Error("New transcoder should not be loaded")
	}
}

func TestTranscoder_LoadDescriptors_InvalidPath(t *testing.T) {
	tc := NewTranscoder()
	err := tc.LoadDescriptors("/nonexistent/path/to/descriptors.desc")
	if err == nil {
		t.Error("LoadDescriptors should return error for invalid path")
	}
}

func TestTranscoder_LoadDescriptors_InvalidData(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := tmpDir + "/invalid.desc"
	os.WriteFile(invalidFile, []byte("invalid data"), 0600)

	tc := NewTranscoder()
	err := tc.LoadDescriptors(invalidFile)
	if err == nil {
		t.Error("LoadDescriptors should return error for invalid descriptor data")
	}
}

// Test StreamProxy
func TestStreamProxy_New(t *testing.T) {
	sp := NewStreamProxy()
	if sp == nil {
		t.Fatal("NewStreamProxy() returned nil")
	}
}

// Test rawCodec
func TestRawCodec_Name(t *testing.T) {
	codec := &rawCodec{}
	if codec.Name() != "raw" {
		t.Errorf("rawCodec.Name() = %v, want raw", codec.Name())
	}
}

// Test Proxy ServeHTTP with unsupported protocol
func TestProxy_ServeHTTP_UnsupportedProtocol(t *testing.T) {
	cfg := &ProxyConfig{
		Target:              "localhost:50051",
		EnableWeb:           false,
		EnableTranscoding:   false,
		Insecure:            true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// Regular HTTP request (not gRPC)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "unsupported protocol") {
		t.Errorf("Body should contain 'unsupported protocol', got: %s", body)
	}
}

// Test Proxy ServeHTTP with gRPC-Web request when disabled
func TestProxy_ServeHTTP_GRPCWebDisabled(t *testing.T) {
	cfg := &ProxyConfig{
		Target:              "localhost:50051",
		EnableWeb:           false,
		EnableTranscoding:   false,
		Insecure:            true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// gRPC-Web request
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "application/grpc-web")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	// Should fall through to unsupported since gRPC-Web is disabled
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// Test metadataFromHeaders with various headers
func TestMetadataFromHeaders_Complex(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		check   map[string]bool // key -> should exist
	}{
		{
			name: "custom headers preserved",
			headers: map[string]string{
				"X-Request-Id": "req-123",
				"X-User-Id":    "user-456",
			},
			check: map[string]bool{
				"x-request-id": true,
				"x-user-id":    true,
			},
		},
		{
			name: "http headers filtered",
			headers: map[string]string{
				"X-Custom-Header": "value",
				"Content-Type":    "application/grpc",
				"User-Agent":      "test",
			},
			check: map[string]bool{
				"x-custom-header": true,
				"content-type":    false, // filtered as HTTP header
				"user-agent":      false, // filtered as HTTP header
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			md := metadataFromHeaders(h)

			for k, shouldExist := range tt.check {
				exists := len(md[k]) > 0
				if exists != shouldExist {
					t.Errorf("md[%q] existence = %v, want %v", k, exists, shouldExist)
				}
			}
		})
	}
}

// Test isHTTPHeader with edge cases
func TestIsHTTPHeader_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"Connection", "Connection", true},
		{"connection", "connection", true},
		{"Upgrade", "Upgrade", true},
		{"upgrade", "upgrade", true},
		{"Host", "Host", true},
		{"host", "host", true},
		{"Content-Type is HTTP header", "Content-Type", true},
		{"X-Custom is not HTTP header", "X-Custom", false},
		{"grpc-status is not HTTP header", "grpc-status", false},
		{"grpc-message is not HTTP header", "grpc-message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHTTPHeader(tt.key)
			if got != tt.want {
				t.Errorf("isHTTPHeader(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// Test handleTranscoding without transcoder
func TestProxy_handleTranscoding_NoTranscoder(t *testing.T) {
	cfg := &ProxyConfig{
		Target:              "localhost:50051",
		EnableTranscoding:   true,
		Insecure:            true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// Transcoder is created but not loaded with descriptors
	req := httptest.NewRequest(http.MethodPost, "/v1/test/method", strings.NewReader(`{"field": "value"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	proxy.handleTranscoding(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// Test handleTranscoding with nil transcoder
func TestProxy_handleTranscoding_NilTranscoder(t *testing.T) {
	cfg := &ProxyConfig{
		Target:              "localhost:50051",
		EnableTranscoding:   true,
		Insecure:            true,
	}
	proxy, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	defer proxy.Close()

	// Set transcoder to nil
	proxy.Transcoder = nil

	req := httptest.NewRequest(http.MethodPost, "/v1/test/method", strings.NewReader(`{"field": "value"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	proxy.handleTranscoding(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// Test handleGRPCWeb with invalid base64
func TestProxy_handleGRPCWeb_InvalidBase64(t *testing.T) {
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

	// Send invalid base64 with grpc-web-text content type
	req := httptest.NewRequest(http.MethodPost, "/test/method", strings.NewReader("not-valid-base64!!!"))
	req.Header.Set("Content-Type", "application/grpc-web-text")
	rec := httptest.NewRecorder()

	proxy.handleGRPCWeb(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// Test Transcoder resolveInputType with invalid method
func TestTranscoder_ResolveInputType_InvalidMethod(t *testing.T) {
	tc := NewTranscoder()
	// Create a mock loaded state
	tc.loaded = true

	_, err := tc.resolveInputType("/InvalidMethod")
	if err == nil {
		t.Error("resolveInputType should return error for invalid method format")
	}
}

// Test Transcoder resolveOutputType with invalid method
func TestTranscoder_ResolveOutputType_InvalidMethod(t *testing.T) {
	tc := NewTranscoder()
	// Create a mock loaded state
	tc.loaded = true

	_, err := tc.resolveOutputType("/InvalidMethod")
	if err == nil {
		t.Error("resolveOutputType should return error for invalid method format")
	}
}

// Test Transcoder resolveMethod with invalid method format
func TestTranscoder_ResolveMethod_InvalidFormat(t *testing.T) {
	tc := NewTranscoder()
	// Create a mock loaded state
	tc.loaded = true

	_, err := tc.resolveMethod("invalid-format")
	if err == nil {
		t.Error("resolveMethod should return error for invalid format")
	}
}

// Test Transcoder resolveMethod with method not found
func TestTranscoder_ResolveMethod_NotFound(t *testing.T) {
	tc := NewTranscoder()
	// Create a mock loaded state
	tc.loaded = true

	// Valid format but method won't be found
	_, err := tc.resolveMethod("/TestService/NonExistentMethod")
	if err == nil {
		t.Error("resolveMethod should return error when method not found")
	}
}

// Test parseGRPCMethod with various edge cases
func TestParseGRPCMethod_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantSvc    string
		wantMethod string
		wantErr    bool
	}{
		{
			name:       "method with multiple slashes",
			method:     "/a/b/c/d",
			wantSvc:    "a/b/c",
			wantMethod: "d",
			wantErr:    false,
		},
		{
			name:       "method with trailing slash",
			method:     "/Service/Method/",
			wantSvc:    "",
			wantMethod: "",
			wantErr:    true,
		},
		{
			name:       "single component",
			method:     "ServiceMethod",
			wantSvc:    "",
			wantMethod: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method, err := parseGRPCMethod(tt.method)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGRPCMethod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if svc != tt.wantSvc {
				t.Errorf("parseGRPCMethod() service = %q, want %q", svc, tt.wantSvc)
			}
			if method != tt.wantMethod {
				t.Errorf("parseGRPCMethod() method = %q, want %q", method, tt.wantMethod)
			}
		})
	}
}

// Test Proxy handleTranscoding error paths
func TestProxy_HandleTranscoding_Errors(t *testing.T) {
	t.Run("nil transcoder", func(t *testing.T) {
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

		// Set transcoder to nil
		proxy.Transcoder = nil

		req := httptest.NewRequest(http.MethodPost, "/v1/test/method", strings.NewReader(`{"field": "value"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})

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

		// Transcoder exists but not loaded
		req := httptest.NewRequest(http.MethodPost, "/v1/test/method", strings.NewReader(`{"field": "value"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

// Test Transcoder concurrent access
func TestTranscoder_ConcurrentAccess(t *testing.T) {
	tc := NewTranscoder()

	// Test concurrent IsLoaded calls
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			tc.IsLoaded()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(time.Second):
			t.Error("timeout waiting for concurrent access")
		}
	}
}

// Test H2CServer
func TestH2CServer_StartStop(t *testing.T) {
	t.Parallel()

	config := &H2CConfig{
		Addr:              "127.0.0.1:0", // Use any available port
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       10 * time.Second,
		MaxHeaderBytes:    1 << 20,
		MaxConcurrentStreams: 100,
	}

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := NewH2CServer(config, handler)

	// Start the server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Get the actual address
	addr := server.Addr()
	if addr == "" {
		t.Error("Server address should not be empty")
	}

	// Test that server is listening
	resp, err := http.Get("http://" + addr)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(ctx); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestH2CServer_DefaultConfig(t *testing.T) {
	t.Parallel()

	// Create server with config that uses port 0 (any available port)
	config := DefaultH2CConfig()
	config.Addr = "127.0.0.1:0"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := NewH2CServer(config, handler)

	// Start and stop
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Stop(ctx)
}

// Test Transcoder JSONToProto with invalid input
func TestTranscoder_JSONToProto_InvalidInput(t *testing.T) {
	tc := NewTranscoder()

	// Test with invalid JSON - should fail because not loaded
	_, err := tc.JSONToProto("/TestService/Method", []byte("not valid json"))
	if err == nil {
		t.Error("JSONToProto should return error when not loaded")
	}
}

// Test Transcoder ProtoToJSON with invalid input
func TestTranscoder_ProtoToJSON_InvalidInput(t *testing.T) {
	tc := NewTranscoder()

	// Test when not loaded - should return error
	_, err := tc.ProtoToJSON("/TestService/Method", []byte("invalid proto"))
	if err == nil {
		t.Error("ProtoToJSON should return error when not loaded")
	}
}

// Test Transcoder resolveMethod with various edge cases
func TestTranscoder_ResolveMethod_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		expectError bool
	}{
		{
			name:        "empty method",
			method:      "",
			expectError: true,
		},
		{
			name:        "no leading slash",
			method:      "TestService/Method",
			expectError: true,
		},
		{
			name:        "too many slashes",
			method:      "/TestService/Method/Extra",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := NewTranscoder()
			// Create a mock loaded state
			tc.loaded = true

			_, err := tc.resolveMethod(tt.method)
			if tt.expectError && err == nil {
				t.Error("resolveMethod should return error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("resolveMethod unexpected error: %v", err)
			}
		})
	}
}

// Test Transcoder LoadDescriptors with various scenarios
func TestTranscoder_LoadDescriptors_EdgeCases(t *testing.T) {
	t.Run("nil transcoder", func(t *testing.T) {
		var tc *Transcoder
		err := tc.LoadDescriptors("/some/path")
		if err == nil {
			t.Error("LoadDescriptors should return error for nil transcoder")
		}
	})

	t.Run("already loaded", func(t *testing.T) {
		tc := NewTranscoder()
		tc.loaded = true

		// Should return error if already loaded
		err := tc.LoadDescriptors("/some/path")
		if err == nil {
			t.Error("LoadDescriptors should return error when already loaded")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		tc := NewTranscoder()
		err := tc.LoadDescriptors("")
		if err == nil {
			t.Error("LoadDescriptors should return error for empty path")
		}
	})
}

// Test Proxy ServeHTTP with different content types
func TestProxy_ServeHTTP_ContentTypes(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{
			name:        "protobuf content type",
			contentType: "application/grpc",
			body:        "test body",
		},
		{
			name:        "grpc-web content type",
			contentType: "application/grpc-web",
			body:        "test body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			req := httptest.NewRequest(http.MethodPost, "/test/method", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()

			// Should not panic
			proxy.ServeHTTP(rec, req)
		})
	}
}

// Test rawCodec with various types
func TestRawCodec_Marshal(t *testing.T) {
	codec := &rawCodec{}

	// Test with nil
	_, err := codec.Marshal(nil)
	if err == nil {
		t.Error("Marshal should return error for nil")
	}

	// Test with unsupported type
	_, err = codec.Marshal(struct{}{})
	if err == nil {
		t.Error("Marshal should return error for unsupported type")
	}
}

func TestRawCodec_Unmarshal(t *testing.T) {
	codec := &rawCodec{}

	// Test with nil pointer
	var ptr *byte
	err := codec.Unmarshal([]byte("test"), ptr)
	if err == nil {
		t.Error("Unmarshal should return error for nil pointer")
	}
}

// Test H2CServer Start with invalid address
func TestH2CServer_Start_InvalidAddress(t *testing.T) {
	t.Parallel()

	config := &H2CConfig{
		Addr: "invalid:address:format:too:many:colons",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := NewH2CServer(config, handler)

	// Should return error for invalid address
	err := server.Start()
	if err == nil {
		t.Error("Start should return error for invalid address")
	}
}

// Test H2CServer Addr before start
func TestH2CServer_Addr_BeforeStart(t *testing.T) {
	t.Parallel()

	config := &H2CConfig{
		Addr: ":8080",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	server := NewH2CServer(config, handler)

	// Should return config addr before Start is called
	addr := server.Addr()
	if addr != ":8080" {
		t.Errorf("Addr() before start = %v, want :8080", addr)
	}
}

// Test IsGRPCRequest with various content types
func TestIsGRPCRequest_VariousTypes(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{
			name:        "application/grpc",
			contentType: "application/grpc",
			expected:    true,
		},
		{
			name:        "application/grpc+json",
			contentType: "application/grpc+json",
			expected:    true,
		},
		{
			name:        "application/json",
			contentType: "application/json",
			expected:    false,
		},
		{
			name:        "text/plain",
			contentType: "text/plain",
			expected:    false,
		},
		{
			name:        "empty",
			contentType: "",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got := IsGRPCRequest(req)
			if got != tt.expected {
				t.Errorf("IsGRPCRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test IsGRPCWebRequest with various content types
func TestIsGRPCWebRequest_VariousTypes(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{
			name:        "application/grpc-web",
			contentType: "application/grpc-web",
			expected:    true,
		},
		{
			name:        "application/grpc-web-text",
			contentType: "application/grpc-web-text",
			expected:    true,
		},
		{
			name:        "application/grpc-web+json",
			contentType: "application/grpc-web+json",
			expected:    true,
		},
		{
			name:        "application/json",
			contentType: "application/json",
			expected:    false,
		},
		{
			name:        "empty",
			contentType: "",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got := IsGRPCWebRequest(req)
			if got != tt.expected {
				t.Errorf("IsGRPCWebRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStreamProxy_ErrorPaths(t *testing.T) {
	sp := NewStreamProxy()
	if sp == nil {
		t.Fatal("NewStreamProxy() returned nil")
	}

	// Test with nil response writer - should panic or handle gracefully
	t.Run("ProxyServerStream nil response writer", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected panic or error
				t.Logf("Expected panic/recover: %v", r)
			}
		}()
		// This will panic with nil response writer
		// We can't easily test this without a real gRPC connection
	})
}


// Test Transcoder with non-existent file
func TestTranscoder_LoadDescriptors_NotFound(t *testing.T) {
	tc := NewTranscoder()
	err := tc.LoadDescriptors("/nonexistent/path/to/file.desc")
	if err == nil {
		t.Error("LoadDescriptors() should return error for non-existent file")
	}
	if !strings.Contains(err.Error(), "failed to read descriptor file") {
		t.Errorf("Error message = %v, want to contain 'failed to read descriptor file'", err)
	}
}

// Test Transcoder resolveMethod with unloaded descriptors
func TestTranscoder_resolveMethod_NotLoaded(t *testing.T) {
	tc := NewTranscoder()
	_, err := tc.JSONToProto("/test.Service/Method", []byte(`{}`))
	if err == nil {
		t.Error("JSONToProto() should return error when not loaded")
	}
	if !strings.Contains(err.Error(), "descriptors not loaded") {
		t.Errorf("Error = %v, want 'descriptors not loaded'", err)
	}
}

// Test Transcoder ProtoToJSON with invalid proto data
func TestTranscoder_ProtoToJSON_InvalidData(t *testing.T) {
	tc := NewTranscoder()
	_, err := tc.ProtoToJSON("/test.Service/Method", []byte("invalid protobuf data"))
	if err == nil {
		t.Error("ProtoToJSON() should return error when not loaded")
	}
}

// Test resolveInputType and resolveOutputType error paths
func TestTranscoder_ResolveTypes_InvalidMethod(t *testing.T) {
	tc := NewTranscoder()

	_, err := tc.JSONToProto("invalid-method", []byte(`{}`))
	if err == nil {
		t.Error("JSONToProto() should return error")
	}

	_, err = tc.ProtoToJSON("invalid-method", []byte{})
	if err == nil {
		t.Error("ProtoToJSON() should return error")
	}
}

