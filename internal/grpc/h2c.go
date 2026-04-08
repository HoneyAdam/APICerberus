package grpc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// H2CServer wraps an HTTP server with HTTP/2 h2c support.
// h2c allows HTTP/2 without TLS (prior knowledge or upgrade).
type H2CServer struct {
	addr     string
	handler  http.Handler
	server   *http.Server
	listener net.Listener
}

// H2CConfig contains configuration for the h2c server.
type H2CConfig struct {
	Addr                 string
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	MaxHeaderBytes       int
	MaxConcurrentStreams uint32
}

// DefaultH2CConfig returns a default configuration.
func DefaultH2CConfig() *H2CConfig {
	return &H2CConfig{
		Addr:                 ":8080",
		ReadTimeout:          30 * time.Second,
		WriteTimeout:         30 * time.Second,
		IdleTimeout:          120 * time.Second,
		MaxHeaderBytes:       1 << 20, // 1MB
		MaxConcurrentStreams: 250,
	}
}

// NewH2CServer creates a new h2c-enabled HTTP server.
func NewH2CServer(config *H2CConfig, handler http.Handler) *H2CServer {
	if config == nil {
		config = DefaultH2CConfig()
	}

	h2s := &http2.Server{
		MaxConcurrentStreams: config.MaxConcurrentStreams,
	}

	// Wrap handler with h2c support
	// h2c.NewHandler handles both:
	// - HTTP/2 prior knowledge (direct h2c)
	// - HTTP/1.1 upgrade to HTTP/2 (h2c upgrade)
	h2cHandler := h2c.NewHandler(handler, h2s)

	server := &http.Server{
		Addr:           config.Addr,
		Handler:        h2cHandler,
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    config.IdleTimeout,
		MaxHeaderBytes: config.MaxHeaderBytes,
	}

	return &H2CServer{
		addr:    config.Addr,
		handler: handler,
		server:  server,
	}
}

// Start begins listening and serving requests.
func (s *H2CServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	s.listener = listener

	// Serve in a goroutine
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Log error but don't block
			fmt.Printf("h2c server error: %v\n", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *H2CServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Addr returns the actual listening address.
func (s *H2CServer) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// IsGRPCRequest checks if a request is a gRPC request.
func IsGRPCRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return contentType == "application/grpc" ||
		contentType == "application/grpc+proto" ||
		contentType == "application/grpc+json"
}

// IsGRPCWebRequest checks if a request is a gRPC-Web request.
func IsGRPCWebRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return contentType == "application/grpc-web" ||
		contentType == "application/grpc-web+proto" ||
		contentType == "application/grpc-web+json" ||
		contentType == "application/grpc-web-text" ||
		contentType == "application/grpc-web-text+proto"
}
