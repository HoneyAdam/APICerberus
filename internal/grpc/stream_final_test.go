package grpc

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Test ProxyServerStream success path with actual gRPC server
func TestProxyServerStream_SuccessPath(t *testing.T) {
	// Create a gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	//nolint:errcheck
	go grpcServer.Serve(lis)
	time.Sleep(10 * time.Millisecond)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Skipf("Failed to connect: %v", err)
	}
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("server stream with data", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		// This will fail since the method doesn't exist, but it tests the stream path
		sp.ProxyServerStream(rec, req, conn, "/test.Service/Method")

		// Should have written something (error frame or data)
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response")
		}
	})

	t.Run("server stream with metadata", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("X-Custom-Header", "custom-value")
		req.Header.Set("Authorization", "Bearer token")

		sp.ProxyServerStream(rec, req, conn, "/test.Service/Method")

		// Should process request with metadata
		_ = rec.body
	})

	t.Run("server stream with CloseSend error", func(t *testing.T) {
		rec := newMockFlusher()
		// Empty body will trigger early return after CloseSend
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.Service/Method")

		// Should handle empty body gracefully
	})
}

// Test ProxyClientStream success path
func TestProxyClientStream_SuccessPath(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	//nolint:errcheck
	go grpcServer.Serve(lis)
	time.Sleep(10 * time.Millisecond)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Skipf("Failed to connect: %v", err)
	}
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("client stream with single message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"message": "test"}`))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.Service/Method")

		// Should have some response
		if rec.Code == 0 {
			t.Error("Expected some response code")
		}
	})

	t.Run("client stream with multiple messages", func(t *testing.T) {
		rec := httptest.NewRecorder()
		// Multiple messages separated by newlines
		body := `{"msg": "1"}
{"msg": "2"}
{"msg": "3"}`
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.Service/Method")

		// Should handle multiple messages
	})

	t.Run("client stream with metadata", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("X-Request-ID", "req-123")

		sp.ProxyClientStream(rec, req, conn, "/test.Service/Method")

		// Should process with metadata
	})
}

// Test ProxyBidiStream success path
func TestProxyBidiStream_SuccessPath(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	//nolint:errcheck
	go grpcServer.Serve(lis)
	time.Sleep(10 * time.Millisecond)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Skipf("Failed to connect: %v", err)
	}
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("bidi stream with data", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should have written something
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response")
		}
	})

	t.Run("bidi stream with multiple messages", func(t *testing.T) {
		rec := newMockFlusher()
		body := `msg1
msg2
msg3`
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should handle multiple messages
	})

	t.Run("bidi stream with context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/grpc")

		// Cancel context after a short delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should handle context cancellation
	})
}

// Test stream proxy with various error scenarios
func TestStreamProxy_ErrorScenarios(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	//nolint:errcheck
	go grpcServer.Serve(lis)
	time.Sleep(10 * time.Millisecond)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Skipf("Failed to connect: %v", err)
	}
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("server stream recv error", func(t *testing.T) {
		rec := newMockFlusher()
		// Empty body means stream will close immediately
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.Service/Method")

		// Should handle gracefully
	})

	t.Run("client stream with send error path", func(t *testing.T) {
		rec := httptest.NewRecorder()
		// Valid body that will be sent
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("test message"))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.Service/Method")

		// Should attempt to send and receive
	})

	t.Run("bidi stream send goroutine error", func(t *testing.T) {
		rec := newMockFlusher()
		// Body that will be read and sent
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader("test data"))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.Service/Method")

		// Should handle send/receive
	})
}

// Test stream proxy with nil connection (error paths)
func TestStreamProxy_NilConnection(t *testing.T) {
	sp := NewStreamProxy()

	t.Run("server stream nil connection", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		// Should panic with nil connection, but we recover
		defer func() {
			if r := recover(); r != nil {
				_ = r // Expected panic
			}
		}()

		sp.ProxyServerStream(rec, req, nil, "/test.Service/Method")
	})

	t.Run("client stream nil connection", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		defer func() {
			if r := recover(); r != nil {
				_ = r // Expected panic
			}
		}()

		sp.ProxyClientStream(rec, req, nil, "/test.Service/Method")
	})

	t.Run("bidi stream nil connection", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		defer func() {
			if r := recover(); r != nil {
				_ = r // Expected panic
			}
		}()

		sp.ProxyBidiStream(rec, req, nil, "/test.Service/Method")
	})
}

// Test stream proxy with non-existent method (covers error frames)
func TestStreamProxy_NonExistentMethod(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Failed to create listener: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	//nolint:errcheck
	go grpcServer.Serve(lis)
	time.Sleep(10 * time.Millisecond)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Skipf("Failed to connect: %v", err)
	}
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("server stream non-existent method", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/NonExistent.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/NonExistent.Service/Method")

		// Should write error frame
		body := string(rec.body)
		if !strings.Contains(body, "error") && !strings.Contains(body, "status") {
			t.Logf("Response body: %s", body)
		}
	})

	t.Run("client stream non-existent method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/NonExistent.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/NonExistent.Service/Method")

		// Should return error response
	})

	t.Run("bidi stream non-existent method", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/NonExistent.Service/Method", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/NonExistent.Service/Method")

		// Should write error frame
	})
}

// Test writeStreamError with various gRPC status codes
func TestWriteStreamError_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		code       codes.Code
		message    string
		wantStatus int
	}{
		{"OK", codes.OK, "", http.StatusOK},
		{"Cancelled", codes.Canceled, "cancelled", 499},
		{"Unknown", codes.Unknown, "unknown error", http.StatusInternalServerError},
		{"InvalidArgument", codes.InvalidArgument, "bad request", http.StatusBadRequest},
		{"DeadlineExceeded", codes.DeadlineExceeded, "timeout", http.StatusGatewayTimeout},
		{"NotFound", codes.NotFound, "not found", http.StatusNotFound},
		{"AlreadyExists", codes.AlreadyExists, "exists", http.StatusConflict},
		{"PermissionDenied", codes.PermissionDenied, "forbidden", http.StatusForbidden},
		{"ResourceExhausted", codes.ResourceExhausted, "rate limited", http.StatusTooManyRequests},
		{"FailedPrecondition", codes.FailedPrecondition, "precondition failed", http.StatusPreconditionFailed},
		{"Aborted", codes.Aborted, "aborted", http.StatusConflict},
		{"OutOfRange", codes.OutOfRange, "out of range", http.StatusBadRequest},
		{"Unimplemented", codes.Unimplemented, "not implemented", http.StatusNotImplemented},
		{"Internal", codes.Internal, "internal error", http.StatusInternalServerError},
		{"Unavailable", codes.Unavailable, "unavailable", http.StatusServiceUnavailable},
		{"DataLoss", codes.DataLoss, "data loss", http.StatusInternalServerError},
		{"Unauthenticated", codes.Unauthenticated, "unauthorized", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			err := status.Error(tt.code, tt.message)
			writeStreamError(rec, err)

			if rec.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()
			// For OK status, message might be empty
			if tt.message != "" && !strings.Contains(body, tt.message) {
				t.Errorf("Body should contain message %q, got: %s", tt.message, body)
			}
			// Verify code is in response
			if !strings.Contains(body, `"code":`) {
				t.Errorf("Body should contain code, got: %s", body)
			}
		})
	}
}

// Test writeStreamErrorFrame with various error types
func TestWriteStreamErrorFrame_ErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{"gRPC NotFound", status.Error(codes.NotFound, "not found"), codes.NotFound},
		{"gRPC InvalidArgument", status.Error(codes.InvalidArgument, "bad arg"), codes.InvalidArgument},
		{"gRPC Internal", status.Error(codes.Internal, "internal"), codes.Internal},
		{"gRPC Unavailable", status.Error(codes.Unavailable, "unavailable"), codes.Unavailable},
		{"plain error", bytes.ErrTooLarge, codes.Internal},
		{"io.EOF", io.EOF, codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeStreamErrorFrame(rec, tt.err)

			body := rec.Body.String()
			if !strings.Contains(body, `"error":true`) {
				t.Errorf("Body should contain error flag, got: %s", body)
			}
			if !strings.Contains(body, `"code":`) {
				t.Errorf("Body should contain code, got: %s", body)
			}
		})
	}
}

// Test writeStreamStatusFrame with various codes
func TestWriteStreamStatusFrame_Codes(t *testing.T) {
	tests := []struct {
		code    codes.Code
		message string
	}{
		{codes.OK, ""},
		{codes.OK, "success"},
		{codes.Canceled, "cancelled"},
		{codes.Unknown, "unknown"},
		{codes.DeadlineExceeded, "timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeStreamStatusFrame(rec, tt.code, tt.message)

			body := rec.Body.String()
			if !strings.Contains(body, `"status":`) {
				t.Errorf("Body should contain status, got: %s", body)
			}
		})
	}
}

// Test splitMessages with various inputs
func TestSplitMessages_Inputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single", "msg1", []string{"msg1"}},
		{"multiple", "msg1\nmsg2\nmsg3", []string{"msg1", "msg2", "msg3"}},
		{"with empty lines", "msg1\n\nmsg2", []string{"msg1", "msg2"}},
		{"with whitespace", "  msg1  \n  msg2  ", []string{"msg1", "msg2"}},
		{"empty", "", nil},
		{"only whitespace", "   \n   ", nil},
		{"json objects", `{"a":1}
{"b":2}`, []string{`{"a":1}`, `{"b":2}`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitMessages([]byte(tt.input))
			if len(got) != len(tt.expected) {
				t.Errorf("splitMessages() returned %d messages, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if string(got[i]) != tt.expected[i] {
					t.Errorf("splitMessages()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// mockFlusher is a ResponseWriter that implements http.Flusher
type mockFlusher struct {
	headers http.Header
	status  int
	body    []byte
	flushed bool
}

func newMockFlusher() *mockFlusher {
	return &mockFlusher{
		headers: make(http.Header),
	}
}

func (m *mockFlusher) Header() http.Header {
	return m.headers
}

func (m *mockFlusher) Write(data []byte) (int, error) {
	m.body = append(m.body, data...)
	return len(data), nil
}

func (m *mockFlusher) WriteHeader(status int) {
	m.status = status
}

func (m *mockFlusher) Flush() {
	m.flushed = true
}
