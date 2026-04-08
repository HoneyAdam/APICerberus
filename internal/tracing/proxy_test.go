package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestProxyTracer_Disabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: false,
	}

	tracer, _ := New(cfg)
	pt := NewProxyTracer(tracer)

	ctx, span := pt.StartUpstreamSpan(context.Background(), "target-1", "localhost:8080")
	if ctx == nil {
		t.Error("Expected context to be returned")
	}

	// End span should not panic
	pt.EndUpstreamSpan(span, nil, nil)
}

func TestProxyTracer_NilTracer(t *testing.T) {
	pt := NewProxyTracer(nil)

	ctx, span := pt.StartUpstreamSpan(context.Background(), "target-1", "localhost:8080")
	if ctx == nil {
		t.Error("Expected context to be returned")
	}

	pt.EndUpstreamSpan(span, nil, nil)
}

func TestProxyTracer_Enabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	ctx, span := pt.StartUpstreamSpan(context.Background(), "target-1", "localhost:8080")
	if ctx == nil {
		t.Error("Expected context to be returned")
	}
	if span == nil {
		t.Error("Expected span to be created")
	}

	// Test with successful response
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 100,
	}
	pt.EndUpstreamSpan(span, resp, nil)
}

func TestProxyTracer_WithError(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	_, span := pt.StartUpstreamSpan(context.Background(), "target-1", "localhost:8080")

	// Test with error
	err := errors.New("connection refused")
	pt.EndUpstreamSpan(span, nil, err)
}

func TestProxyTracer_WithServerError(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	_, span := pt.StartUpstreamSpan(context.Background(), "target-1", "localhost:8080")

	// Test with 5xx response
	resp := &http.Response{
		StatusCode:    http.StatusInternalServerError,
		ContentLength: 0,
	}
	pt.EndUpstreamSpan(span, resp, nil)
}

func TestProxyTracer_TraceRoundTripper_Disabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: false,
	}

	tracer, _ := New(cfg)
	pt := NewProxyTracer(tracer)

	base := http.DefaultTransport
	rt := pt.TraceRoundTripper(base)

	// Should return base transport when disabled
	if rt != base {
		t.Error("Expected base transport when disabled")
	}
}

func TestProxyTracer_TraceRoundTripper_NilTracer(t *testing.T) {
	pt := NewProxyTracer(nil)

	base := http.DefaultTransport
	rt := pt.TraceRoundTripper(base)

	if rt != base {
		t.Error("Expected base transport when tracer is nil")
	}
}

func TestProxyTracer_TraceRoundTripper_Enabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Wrap the transport
	base := &http.Transport{}
	rt := pt.TraceRoundTripper(base)

	// Create a request
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)

	// Execute the request
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Errorf("RoundTrip() error = %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

func TestTracedRoundTripper_WithError(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	// Create a transport that always fails
	base := &failingTransport{err: errors.New("connection refused")}
	rt := pt.TraceRoundTripper(base)

	// Create a request to a non-existent server
	req, _ := http.NewRequest(http.MethodGet, "http://localhost:1", nil)

	// Execute the request
	resp, err := rt.RoundTrip(req)
	if err == nil {
		t.Error("Expected error from failing transport")
	}
	if resp != nil {
		resp.Body.Close()
	}
}

func TestTracedRoundTripper_WithServerError(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	// Create a test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	base := &http.Transport{}
	rt := pt.TraceRoundTripper(base)

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Errorf("RoundTrip() error = %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response")
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}
	resp.Body.Close()
}

// failingTransport is a transport that always returns an error
type failingTransport struct {
	err error
}

func (t *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, t.err
}

func TestEndUpstreamSpan_NilSpan(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "test-service",
		Exporter:     "stdout",
		SamplingRate: 1.0,
	}

	tracer, _ := New(cfg)
	defer func() {
		if tracer != nil {
			tracer.Shutdown(nil)
		}
	}()

	pt := NewProxyTracer(tracer)

	// Should not panic with nil span
	pt.EndUpstreamSpan(nil, nil, nil)
	pt.EndUpstreamSpan(nil, &http.Response{StatusCode: 200}, nil)
	pt.EndUpstreamSpan(nil, nil, errors.New("test error"))
}
