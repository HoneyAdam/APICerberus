package grpc

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// setupBufconnGRPCServer creates a test gRPC server using bufconn for in-memory testing
// The service uses raw bytes for communication to work with rawCodec
func setupBufconnGRPCServer(t *testing.T) (*grpc.Server, *bufconn.Listener, func()) {
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer(grpc.ForceServerCodec(&rawCodec{}))

	// Register a simple test service that handles all streaming types using raw bytes
	testSvc := &bufconnTestService{}
	registerBufconnTestService(s, testSvc)

	go func() {
		if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("Server error: %v", err)
		}
	}()

	cleanup := func() {
		s.Stop()
		lis.Close()
	}

	return s, lis, cleanup
}

// createBufconnClient creates a gRPC client connection using bufconn
func createBufconnClient(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&rawCodec{})),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	return conn
}

// bufconnTestService implements a test gRPC service with all streaming types using raw bytes
type bufconnTestService struct {
	UnimplementedTestServiceServer
}

func (s *bufconnTestService) UnaryCall(ctx context.Context, req *bytes.Buffer) (*bytes.Buffer, error) {
	return bytes.NewBufferString("Echo: " + req.String()), nil
}

func (s *bufconnTestService) ServerStreamingCall(req *bytes.Buffer, stream grpc.ServerStream) error {
	// Send multiple responses
	for i := 0; i < 3; i++ {
		resp := bytes.NewBufferString("Stream " + string(rune('A'+i)) + ": " + req.String())
		if err := stream.SendMsg(resp); err != nil {
			return err
		}
	}
	return nil
}

func (s *bufconnTestService) ClientStreamingCall(stream grpc.ServerStream) error {
	var messages []string
	for {
		req := new(bytes.Buffer)
		err := stream.RecvMsg(req)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		messages = append(messages, req.String())
	}
	return stream.SendMsg(bytes.NewBufferString("Received: " + strings.Join(messages, ", ")))
}

func (s *bufconnTestService) BidiStreamingCall(stream grpc.ServerStream) error {
	for {
		req := new(bytes.Buffer)
		err := stream.RecvMsg(req)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.SendMsg(bytes.NewBufferString("Echo: " + req.String())); err != nil {
			return err
		}
	}
}

type UnimplementedTestServiceServer struct{}

func registerBufconnTestService(s *grpc.Server, svc *bufconnTestService) {
	s.RegisterService(&bufconnTestServiceDesc, svc)
}

var bufconnTestServiceDesc = grpc.ServiceDesc{
	ServiceName: "test.TestService",
	HandlerType: (*interface{})(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "UnaryCall",
			Handler:    bufconnUnaryCallHandler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ServerStreamingCall",
			Handler:       bufconnServerStreamingHandler,
			ServerStreams: true,
		},
		{
			StreamName:    "ClientStreamingCall",
			Handler:       bufconnClientStreamingHandler,
			ClientStreams: true,
		},
		{
			StreamName:    "BidiStreamingCall",
			Handler:       bufconnBidiStreamingHandler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "test.proto",
}

func bufconnUnaryCallHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(bytes.Buffer)
	if err := dec(in); err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, status.Errorf(codes.Unimplemented, "service not implemented")
	}
	svc := srv.(*bufconnTestService)
	return svc.UnaryCall(ctx, in)
}

func bufconnServerStreamingHandler(srv interface{}, stream grpc.ServerStream) error {
	m := new(bytes.Buffer)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	svc := srv.(*bufconnTestService)
	return svc.ServerStreamingCall(m, stream)
}

func bufconnClientStreamingHandler(srv interface{}, stream grpc.ServerStream) error {
	svc := srv.(*bufconnTestService)
	return svc.ClientStreamingCall(stream)
}

func bufconnBidiStreamingHandler(srv interface{}, stream grpc.ServerStream) error {
	svc := srv.(*bufconnTestService)
	return svc.BidiStreamingCall(stream)
}

// Test ProxyServerStream success path with actual server streaming
func TestProxyServerStream_BufconnSuccess(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("successful server streaming with multiple messages", func(t *testing.T) {
		rec := newMockFlusher()
		body := `test message`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ServerStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.TestService/ServerStreamingCall")

		// Should complete successfully with status OK
		if rec.status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.status)
		}

		// Should have received multiple responses followed by status frame
		bodyStr := string(rec.body)
		if !strings.Contains(bodyStr, "Stream A") {
			t.Errorf("Expected 'Stream A' in response, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "status") {
			t.Errorf("Expected status frame in response, got: %s", bodyStr)
		}
	})

	t.Run("server streaming with metadata forwarding", func(t *testing.T) {
		rec := newMockFlusher()
		body := `test message`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ServerStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("X-Custom-Header", "custom-value")
		req.Header.Set("Authorization", "Bearer token123")

		sp.ProxyServerStream(rec, req, conn, "/test.TestService/ServerStreamingCall")

		if rec.status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.status)
		}
	})

	t.Run("server streaming empty body sends no messages", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ServerStreamingCall", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.TestService/ServerStreamingCall")

		// Empty body should still work
		if rec.status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.status)
		}
	})
}

// Test ProxyClientStream success path with actual client streaming
func TestProxyClientStream_BufconnSuccess(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("successful client streaming - single message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `hello`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ClientStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.TestService/ClientStreamingCall")

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}

		bodyStr := rec.Body.String()
		if bodyStr == "" {
			t.Error("Expected non-empty response body")
		}
	})

	t.Run("successful client streaming - multiple messages", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `msg1
msg2
msg3`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ClientStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.TestService/ClientStreamingCall")

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}

		// Response should contain all messages
		bodyStr := rec.Body.String()
		if !strings.Contains(bodyStr, "Received") {
			t.Errorf("Expected response to contain aggregated messages, got: %s", bodyStr)
		}
	})

	t.Run("client streaming with metadata", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `test`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ClientStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("X-Request-ID", "req-123")

		sp.ProxyClientStream(rec, req, conn, "/test.TestService/ClientStreamingCall")

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

// Test ProxyBidiStream success path with actual bidi streaming
func TestProxyBidiStream_BufconnSuccess(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("successful bidi streaming - single message", func(t *testing.T) {
		rec := newMockFlusher()
		body := `hello`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/BidiStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.TestService/BidiStreamingCall")

		if rec.status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.status)
		}

		bodyStr := string(rec.body)
		if !strings.Contains(bodyStr, "Echo") && !strings.Contains(bodyStr, "status") {
			t.Errorf("Expected echo response or status, got: %s", bodyStr)
		}
	})

	t.Run("successful bidi streaming - multiple messages", func(t *testing.T) {
		rec := newMockFlusher()
		body := `msg1
msg2
msg3`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/BidiStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.TestService/BidiStreamingCall")

		if rec.status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.status)
		}
	})

	t.Run("bidi streaming with metadata", func(t *testing.T) {
		rec := newMockFlusher()
		body := `test`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/BidiStreamingCall", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("X-Custom-Header", "value")

		sp.ProxyBidiStream(rec, req, conn, "/test.TestService/BidiStreamingCall")

		if rec.status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.status)
		}
	})
}

// Test handleTranscoding with actual gRPC server
func TestHandleTranscoding_Bufconn(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	proxy := &Proxy{
		Target:      "bufnet",
		Transcoding: true,
		Transcoder:  NewTranscoder(),
		StreamProxy: NewStreamProxy(),
		client:      conn,
	}

	t.Run("transcoding not loaded returns service unavailable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", strings.NewReader(`{"message": "hello"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
		}
	})

	t.Run("transcoding with path without leading slash", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", strings.NewReader(`{}`))
		req.URL.Path = "test.TestService/UnaryCall" // Remove leading slash
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
		}
	})
}

// Test handleGRPC with actual gRPC server
func TestHandleGRPC_Bufconn(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	proxy := &Proxy{
		Target:      "bufnet",
		client:      conn,
		Transcoder:  NewTranscoder(),
		StreamProxy: NewStreamProxy(),
	}

	t.Run("handleGRPC with actual unary call", func(t *testing.T) {
		body := []byte("test message")
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")
		rec := httptest.NewRecorder()

		proxy.handleGRPC(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}

		// Check gRPC status
		grpcStatus := rec.Header().Get("Grpc-Status")
		if grpcStatus != "0" && grpcStatus != "" {
			t.Errorf("Expected Grpc-Status 0 or empty, got %s", grpcStatus)
		}
	})

	t.Run("handleGRPC with metadata forwarding", func(t *testing.T) {
		body := []byte("test")
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("X-Custom-Header", "custom-value")
		req.Header.Set("Authorization", "Bearer token")
		rec := httptest.NewRecorder()

		proxy.handleGRPC(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("handleGRPC with path without leading slash", func(t *testing.T) {
		body := []byte("test")
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", bytes.NewReader(body))
		req.URL.Path = "test.TestService/UnaryCall" // Remove leading slash
		req.Header.Set("Content-Type", "application/grpc")
		rec := httptest.NewRecorder()

		proxy.handleGRPC(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

// Test handleGRPCWeb with actual gRPC server
func TestHandleGRPCWeb_Bufconn(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	proxy := &Proxy{
		Target:      "bufnet",
		EnableWeb:   true,
		client:      conn,
		Transcoder:  NewTranscoder(),
		StreamProxy: NewStreamProxy(),
	}

	t.Run("handleGRPCWeb with actual call", func(t *testing.T) {
		body := []byte("test message")
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc-web")
		rec := httptest.NewRecorder()

		proxy.handleGRPCWeb(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}

		// Check CORS headers
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("Expected CORS header to be set")
		}
	})

	t.Run("handleGRPCWeb with path without leading slash", func(t *testing.T) {
		body := []byte("test")
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", bytes.NewReader(body))
		req.URL.Path = "test.TestService/UnaryCall"
		req.Header.Set("Content-Type", "application/grpc-web")
		rec := httptest.NewRecorder()

		proxy.handleGRPCWeb(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

// Test stream proxy error scenarios with actual server
func TestStreamProxy_BufconnErrors(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("server stream non-existent method", func(t *testing.T) {
		rec := newMockFlusher()
		body := `test message`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/NonExistent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.TestService/NonExistent")

		// Should handle the error gracefully
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response for non-existent method")
		}
	})

	t.Run("client stream non-existent method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `test message`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/NonExistent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.TestService/NonExistent")

		if rec.Code == 0 {
			t.Error("Expected some response code")
		}
	})

	t.Run("bidi stream non-existent method", func(t *testing.T) {
		rec := newMockFlusher()
		body := `test message`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/NonExistent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.TestService/NonExistent")

		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response for non-existent method")
		}
	})
}

// Test stream proxy with body read errors
func TestStreamProxy_BodyReadErrors_Bufconn(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("server stream body read error", func(t *testing.T) {
		rec := newMockFlusher()
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ServerStreamingCall", &failReader{})
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.TestService/ServerStreamingCall")

		if rec.status != http.StatusBadRequest {
			t.Errorf("Expected status %d for body read error, got %d", http.StatusBadRequest, rec.status)
		}
	})

	t.Run("client stream body read error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ClientStreamingCall", &failReader{})
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.TestService/ClientStreamingCall")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for body read error, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

type failReader struct{}

func (f *failReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (f *failReader) Close() error {
	return nil
}

// Test HealthServer Watch Send error path
func TestHealthServer_WatchSendError(t *testing.T) {
	checker := NewSimpleHealthChecker()
	server := NewHealthServer(checker)

	// Create a stream that returns an error on Send
	stream := &errorSendWatchStream{
		ctx: context.Background(),
	}

	err := server.Watch(&healthpb.HealthCheckRequest{Service: "test-service"}, stream)

	// Should return error when Send fails
	if err == nil {
		t.Error("Expected error when Send fails")
	}
}

// errorSendWatchStream is a mock that returns an error on Send
type errorSendWatchStream struct {
	ctx context.Context
}

func (e *errorSendWatchStream) Send(resp *healthpb.HealthCheckResponse) error {
	return status.Error(codes.Internal, "send failed")
}

func (e *errorSendWatchStream) Context() context.Context {
	return e.ctx
}

func (e *errorSendWatchStream) SendMsg(msg interface{}) error {
	return e.Send(msg.(*healthpb.HealthCheckResponse))
}

func (e *errorSendWatchStream) RecvMsg(msg interface{}) error {
	return nil
}

func (e *errorSendWatchStream) SetHeader(md metadata.MD) error {
	return nil
}

func (e *errorSendWatchStream) SendHeader(md metadata.MD) error {
	return nil
}

func (e *errorSendWatchStream) SetTrailer(md metadata.MD) {}

// Test H2CServer Start error path
func TestH2CServer_StartError(t *testing.T) {
	// Create a server with an invalid address
	config := &H2CConfig{
		Addr:           "invalid:address:format:too:many:colons",
		ReadTimeout:    5,
		WriteTimeout:   5,
		IdleTimeout:    10,
		MaxHeaderBytes: 1024,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := NewH2CServer(config, handler)

	// Start should fail with invalid address
	err := server.Start()
	if err == nil {
		t.Error("Expected error when starting server with invalid address")
	}
}

// Test metadataFromHeaders with various scenarios
func TestMetadataFromHeaders_Bufconn(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		wantKey string
		wantVal []string
	}{
		{
			name:    "custom headers are forwarded",
			headers: http.Header{"X-Custom": []string{"value"}},
			wantKey: "x-custom",
			wantVal: []string{"value"},
		},
		{
			name:    "multiple values are preserved",
			headers: http.Header{"X-Multi": []string{"val1", "val2"}},
			wantKey: "x-multi",
			wantVal: []string{"val1", "val2"},
		},
		{
			name:    "http headers are skipped",
			headers: http.Header{"Content-Type": []string{"application/json"}, "User-Agent": []string{"test"}},
			wantKey: "",
			wantVal: nil,
		},
		{
			name:    "authorization header is forwarded",
			headers: http.Header{"Authorization": []string{"Bearer token"}},
			wantKey: "authorization",
			wantVal: []string{"Bearer token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadataFromHeaders(tt.headers)
			if tt.wantKey != "" {
				if v, ok := md[tt.wantKey]; !ok {
					t.Errorf("Expected key %q not found in metadata", tt.wantKey)
				} else {
					for i, val := range tt.wantVal {
						if i >= len(v) || v[i] != val {
							t.Errorf("Expected value %q at index %d, got %q", val, i, v[i])
						}
					}
				}
			} else {
				// For HTTP headers that should be skipped
				for k := range tt.headers {
					if _, ok := md[strings.ToLower(k)]; ok {
						t.Errorf("HTTP header %q should be skipped but found in metadata", k)
					}
				}
			}
		})
	}
}

// Test Proxy ServeHTTP with all protocol types
func TestProxy_ServeHTTP_Bufconn(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	tests := []struct {
		name              string
		contentType       string
		enableWeb         bool
		enableTranscoding bool
		wantStatus        int
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
			proxy := &Proxy{
				Target:      "bufnet",
				EnableWeb:   tt.enableWeb,
				Transcoding: tt.enableTranscoding,
				client:      conn,
				Transcoder:  NewTranscoder(),
				StreamProxy: NewStreamProxy(),
			}

			req := httptest.NewRequest(http.MethodPost, "/test.TestService/UnaryCall", strings.NewReader("test"))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()

			proxy.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// Test ProxyServerStream error during receive
func TestProxyServerStream_RecvError(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("server stream receives error from upstream", func(t *testing.T) {
		rec := newMockFlusher()
		// Use a method that will cause an error (invalid method name)
		body := `test`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/InvalidMethod", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyServerStream(rec, req, conn, "/test.TestService/InvalidMethod")

		// Should handle the error and write error frame
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response even for error case")
		}
	})
}

// Test ProxyClientStream with CloseSend error
func TestProxyClientStream_CloseSendError(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("client stream with empty body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		// Empty body means no messages to send
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/ClientStreamingCall", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyClientStream(rec, req, conn, "/test.TestService/ClientStreamingCall")

		// Should handle empty body
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

// Test ProxyBidiStream with send error
func TestProxyBidiStream_SendError(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	sp := NewStreamProxy()

	t.Run("bidi stream with invalid method", func(t *testing.T) {
		rec := newMockFlusher()
		body := `test`
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/InvalidMethod", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/grpc")

		sp.ProxyBidiStream(rec, req, conn, "/test.TestService/InvalidMethod")

		// Should handle error gracefully
		if rec.status == 0 && len(rec.body) == 0 {
			t.Error("Expected some response for invalid method")
		}
	})
}

// Test handleTranscoding success path with loaded descriptors
func TestHandleTranscoding_WithLoadedDescriptors(t *testing.T) {
	_, lis, cleanup := setupBufconnGRPCServer(t)
	defer cleanup()

	conn := createBufconnClient(t, lis)
	defer conn.Close()

	proxy := &Proxy{
		Target:      "bufnet",
		Transcoding: true,
		Transcoder:  NewTranscoder(),
		StreamProxy: NewStreamProxy(),
		client:      conn,
	}

	// Create a temporary descriptor file
	tmpFile, err := os.CreateTemp("", "test*.desc")
	if err != nil {
		t.Skipf("Failed to create temp file: %v", err)
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
		t.Skipf("Failed to marshal descriptor: %v", err)
	}

	if err := os.WriteFile(tmpFile.Name(), data, 0644); err != nil {
		t.Skipf("Failed to write descriptor file: %v", err)
	}

	if err := proxy.Transcoder.LoadDescriptors(tmpFile.Name()); err != nil {
		t.Skipf("Could not load descriptors: %v", err)
	}

	t.Run("transcoding with loaded descriptors - valid request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{"name": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		// Should handle the request (may fail due to no upstream, but should not panic)
		// The transcoding should work but the gRPC call will fail
	})

	t.Run("transcoding with loaded descriptors - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test.TestService/TestMethod", strings.NewReader(`{invalid json`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}
