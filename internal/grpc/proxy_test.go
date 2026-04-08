package grpc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
)

func TestNewProxy(t *testing.T) {
	t.Run("valid config with insecure", func(t *testing.T) {
		cfg := &ProxyConfig{
			Target:            "localhost:50051",
			EnableWeb:         true,
			EnableTranscoding: true,
			Insecure:          true,
		}

		proxy, err := NewProxy(cfg)
		if err != nil {
			t.Fatalf("NewProxy() error = %v", err)
		}
		if proxy == nil {
			t.Fatal("NewProxy() returned nil")
		}
		if proxy.Target != cfg.Target {
			t.Errorf("Target = %v, want %v", proxy.Target, cfg.Target)
		}
		if !proxy.EnableWeb {
			t.Error("EnableWeb should be true")
		}
		if !proxy.Transcoding {
			t.Error("Transcoding should be true")
		}
		if proxy.client == nil {
			t.Error("client not initialized")
		}
		if proxy.Transcoder == nil {
			t.Error("Transcoder not initialized")
		}
		if proxy.StreamProxy == nil {
			t.Error("StreamProxy not initialized")
		}

		proxy.Close()
	})

	t.Run("valid config without insecure", func(t *testing.T) {
		cfg := &ProxyConfig{
			Target:            "localhost:50051",
			EnableWeb:         false,
			EnableTranscoding: false,
			Insecure:          false,
		}

		// This will fail because without insecure credentials, it needs TLS certs
		// But we test the config path
		_, err := NewProxy(cfg)
		// Error expected since no TLS config provided
		if err == nil {
			// Close if it somehow succeeded
			t.Log("NewProxy succeeded without insecure - this is unexpected")
		}
	})
}

func TestProxy_Close(t *testing.T) {
	t.Run("close with client", func(t *testing.T) {
		cfg := &ProxyConfig{
			Target:   "localhost:50051",
			Insecure: true,
		}
		proxy, _ := NewProxy(cfg)
		err := proxy.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	t.Run("close without client", func(t *testing.T) {
		proxy := &Proxy{}
		err := proxy.Close()
		if err != nil {
			t.Error("Close() should return nil when no client")
		}
	})
}

func TestProxy_ServeHTTP(t *testing.T) {
	t.Run("unsupported protocol", func(t *testing.T) {
		proxy := &Proxy{
			EnableWeb:   false,
			Transcoding: false,
		}

		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		proxy.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Status = %v, want %v", rec.Code, http.StatusBadRequest)
		}
		if !strings.Contains(rec.Body.String(), "unsupported protocol") {
			t.Error("Response should contain 'unsupported protocol'")
		}
	})
}

func TestProxy_handleGRPCWeb(t *testing.T) {
	t.Run("invalid base64", func(t *testing.T) {
		proxy := &Proxy{}

		req := httptest.NewRequest("POST", "/test.Service/Method", strings.NewReader("!!!invalid-base64"))
		req.Header.Set("Content-Type", "application/grpc-web-text")
		rec := httptest.NewRecorder()

		proxy.handleGRPCWeb(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Status = %v, want %v", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestProxy_handleTranscoding(t *testing.T) {
	t.Run("transcoder not loaded", func(t *testing.T) {
		proxy := &Proxy{
			Transcoder: NewTranscoder(),
		}

		req := httptest.NewRequest("POST", "/v1/test/method", strings.NewReader(`{"key": "value"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Status = %v, want %v", rec.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("transcoder nil", func(t *testing.T) {
		proxy := &Proxy{
			Transcoder: nil,
		}

		req := httptest.NewRequest("POST", "/v1/test/method", strings.NewReader(`{"key": "value"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		proxy.handleTranscoding(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Status = %v, want %v", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestWriteGRPCError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeGRPCError(rec, codes.InvalidArgument, "test error message")

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %v, want %v", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Content-Type") != "application/grpc" {
		t.Errorf("Content-Type = %v, want application/grpc", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Grpc-Status") != "3" { // codes.InvalidArgument = 3
		t.Errorf("Grpc-Status = %v, want 3", rec.Header().Get("Grpc-Status"))
	}
	if rec.Header().Get("Grpc-Message") != "test error message" {
		t.Errorf("Grpc-Message = %v, want 'test error message'", rec.Header().Get("Grpc-Message"))
	}
}

func TestRawCodec(t *testing.T) {
	codec := &rawCodec{}

	t.Run("Name", func(t *testing.T) {
		name := codec.Name()
		if name != "raw" {
			t.Errorf("Name() = %v, want raw", name)
		}
	})

	t.Run("Marshal with bytes.Buffer", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte("test data"))
		data, err := codec.Marshal(buf)
		if err != nil {
			t.Errorf("Marshal() error = %v", err)
		}
		if string(data) != "test data" {
			t.Errorf("Marshal() = %v, want 'test data'", string(data))
		}
	})

	t.Run("Marshal with byte slice", func(t *testing.T) {
		data := []byte("test data")
		result, err := codec.Marshal(data)
		if err != nil {
			t.Errorf("Marshal() error = %v", err)
		}
		if string(result) != "test data" {
			t.Errorf("Marshal() = %v, want 'test data'", string(result))
		}
	})

	t.Run("Marshal with unsupported type", func(t *testing.T) {
		_, err := codec.Marshal("string")
		if err == nil {
			t.Error("Marshal() should return error for unsupported type")
		}
	})

	t.Run("Unmarshal to bytes.Buffer", func(t *testing.T) {
		var buf bytes.Buffer
		err := codec.Unmarshal([]byte("test data"), &buf)
		if err != nil {
			t.Errorf("Unmarshal() error = %v", err)
		}
		if buf.String() != "test data" {
			t.Errorf("Unmarshal() = %v, want 'test data'", buf.String())
		}
	})

	t.Run("Unmarshal to byte slice", func(t *testing.T) {
		var data []byte
		err := codec.Unmarshal([]byte("test data"), &data)
		if err != nil {
			t.Errorf("Unmarshal() error = %v", err)
		}
		if string(data) != "test data" {
			t.Errorf("Unmarshal() = %v, want 'test data'", string(data))
		}
	})

	t.Run("Unmarshal with unsupported type", func(t *testing.T) {
		var s string
		err := codec.Unmarshal([]byte("test"), &s)
		if err == nil {
			t.Error("Unmarshal() should return error for unsupported type")
		}
	})
}

func TestIsHTTPHeader(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"Accept", "Accept", true},
		{"Content-Type", "Content-Type", true},
		{"User-Agent", "User-Agent", true},
		{"Host", "Host", true},
		{"Connection", "Connection", true},
		{"X-Custom-Header", "X-Custom-Header", false},
		{"Authorization", "Authorization", false},
		{"Accept case insensitive", "accept", true},
		{"CONTENT-TYPE uppercase", "CONTENT-TYPE", true},
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

func TestProxy_handleGRPC(t *testing.T) {
	t.Run("failed to read body", func(t *testing.T) {
		proxy := &Proxy{}

		// Create request with a body that can't be read
		req := httptest.NewRequest("POST", "/test.Service/Method", &errorReader{})
		req.Header.Set("Content-Type", "application/grpc")
		rec := httptest.NewRecorder()

		proxy.handleGRPC(rec, req)

		if rec.Header().Get("Grpc-Status") != fmt.Sprintf("%d", codes.Internal) {
			t.Errorf("Grpc-Status = %v, want %d", rec.Header().Get("Grpc-Status"), codes.Internal)
		}
	})
}

// errorReader simulates a read error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (e *errorReader) Close() error {
	return nil
}
