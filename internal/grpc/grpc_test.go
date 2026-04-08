package grpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestIsGRPCRequest(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "gRPC request",
			contentType: "application/grpc",
			want:        true,
		},
		{
			name:        "gRPC+proto request",
			contentType: "application/grpc+proto",
			want:        true,
		},
		{
			name:        "gRPC+json request",
			contentType: "application/grpc+json",
			want:        true,
		},
		{
			name:        "JSON request",
			contentType: "application/json",
			want:        false,
		},
		{
			name:        "no content type",
			contentType: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got := IsGRPCRequest(req)
			if got != tt.want {
				t.Errorf("IsGRPCRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGRPCWebRequest(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "gRPC-Web request",
			contentType: "application/grpc-web",
			want:        true,
		},
		{
			name:        "gRPC-Web text",
			contentType: "application/grpc-web-text",
			want:        true,
		},
		{
			name:        "gRPC request",
			contentType: "application/grpc",
			want:        false,
		},
		{
			name:        "JSON request",
			contentType: "application/json",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got := IsGRPCWebRequest(req)
			if got != tt.want {
				t.Errorf("IsGRPCWebRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestH2CServer(t *testing.T) {
	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from h2c"))
	})

	// Create h2c server with random port
	config := &H2CConfig{
		Addr:                 "127.0.0.1:0",
		ReadTimeout:          5,
		WriteTimeout:         5,
		IdleTimeout:          10,
		MaxHeaderBytes:       1024,
		MaxConcurrentStreams: 100,
	}

	server := NewH2CServer(config, handler)

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start h2c server: %v", err)
	}

	// Get actual address
	addr := server.Addr()

	// Wait for server to be ready
	var resp *http.Response
	var err error
	for i := 0; i < 20; i++ {
		resp, err = http.Get("http://" + addr + "/test")
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hello from h2c") {
		t.Errorf("Expected body to contain 'Hello from h2c', got: %s", string(body))
	}

	// Stop server
	ctx := context.Background()
	server.Stop(ctx)
}

func TestMetadataFromHeaders(t *testing.T) {
	headers := http.Header{
		"Content-Type":    []string{"application/grpc"},
		"X-Custom-Header": []string{"value1", "value2"},
		"Authorization":   []string{"Bearer token"},
		"User-Agent":      []string{"test"},
	}

	md := metadataFromHeaders(headers)

	// HTTP-specific headers should be skipped
	if _, ok := md["user-agent"]; ok {
		t.Error("user-agent should be skipped")
	}

	// Custom headers should be included (lowercased)
	if v, ok := md["x-custom-header"]; !ok || len(v) != 2 {
		t.Error("x-custom-header should be included with 2 values")
	}
}

func TestGRPCStatusToHTTP(t *testing.T) {
	tests := []struct {
		grpcCode codes.Code
		want     int
	}{
		{codes.OK, http.StatusOK},
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.NotFound, http.StatusNotFound},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.PermissionDenied, http.StatusForbidden},
		{codes.Internal, http.StatusInternalServerError},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.grpcCode.String(), func(t *testing.T) {
			got := GRPCStatusToHTTP(tt.grpcCode)
			if got != tt.want {
				t.Errorf("GRPCStatusToHTTP(%v) = %d, want %d", tt.grpcCode, got, tt.want)
			}
		})
	}
}

func TestHTTPStatusToGRPC(t *testing.T) {
	tests := []struct {
		httpStatus int
		want       codes.Code
	}{
		{http.StatusOK, codes.OK},
		{http.StatusBadRequest, codes.InvalidArgument},
		{http.StatusUnauthorized, codes.Unauthenticated},
		{http.StatusForbidden, codes.PermissionDenied},
		{http.StatusNotFound, codes.NotFound},
		{http.StatusInternalServerError, codes.Internal},
		{http.StatusServiceUnavailable, codes.Unavailable},
		{http.StatusGatewayTimeout, codes.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.httpStatus), func(t *testing.T) {
			got := HTTPStatusToGRPC(tt.httpStatus)
			if got != tt.want {
				t.Errorf("HTTPStatusToGRPC(%d) = %v, want %v", tt.httpStatus, got, tt.want)
			}
		})
	}
}

func TestSimpleHealthChecker(t *testing.T) {
	checker := NewSimpleHealthChecker()

	// Test unknown service
	status, err := checker.Check("unknown-service")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if status != StatusUnknown {
		t.Errorf("Expected StatusUnknown, got %v", status)
	}

	// Set status
	checker.SetStatus("test-service", StatusServing)

	status, err = checker.Check("test-service")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if status != StatusServing {
		t.Errorf("Expected StatusServing, got %v", status)
	}

	// Test overall health
	status, err = checker.Check("")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if status != StatusServing {
		t.Errorf("Expected StatusServing for overall health, got %v", status)
	}

	// Set one service to not serving
	checker.SetStatus("another-service", StatusNotServing)
	status, err = checker.Check("")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if status != StatusNotServing {
		t.Errorf("Expected StatusNotServing for overall health, got %v", status)
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUnknown, "UNKNOWN"},
		{StatusServing, "SERVING"},
		{StatusNotServing, "NOT_SERVING"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.want {
				t.Errorf("Status.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusToGRPC(t *testing.T) {
	tests := []struct {
		status Status
		want   healthpb.HealthCheckResponse_ServingStatus
	}{
		{StatusUnknown, healthpb.HealthCheckResponse_UNKNOWN},
		{StatusServing, healthpb.HealthCheckResponse_SERVING},
		{StatusNotServing, healthpb.HealthCheckResponse_NOT_SERVING},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			got := tt.status.ToGRPC()
			if got != tt.want {
				t.Errorf("Status.ToGRPC() = %v, want %v", got, tt.want)
			}
		})
	}
}
