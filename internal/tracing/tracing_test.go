package tracing

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestNew_Disabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: false,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if tracer.Enabled() {
		t.Error("Expected tracer to be disabled")
	}

	// Shutdown should not error
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tracer.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestNew_StdoutExporter(t *testing.T) {
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

	if !tracer.Enabled() {
		t.Error("Expected tracer to be enabled")
	}

	// Test span creation
	ctx, span := tracer.StartSpan(context.Background(), "test-operation")
	if span == nil {
		t.Error("Expected span to be created")
	}

	span.End()

	// Verify context is returned
	if ctx == nil {
		t.Error("Expected context to be returned")
	}

	// Shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tracer.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestNew_Defaults(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: true,
		// Leave other fields empty to test defaults
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !tracer.Enabled() {
		t.Error("Expected tracer to be enabled")
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tracer.Shutdown(ctx)
}

func TestTracer_InjectExtract(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Exporter:       "stdout",
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Shutdown(context.Background())
		}
	}()

	// Create a span
	ctx, span := tracer.StartSpan(context.Background(), "test-operation")
	defer span.End()

	// Inject into headers
	headers := make(http.Header)
	tracer.Inject(ctx, headers)

	// Verify traceparent header was set
	if headers.Get("Traceparent") == "" {
		t.Error("Expected Traceparent header to be set")
	}

	// Extract from headers
	extractedCtx := tracer.Extract(headers)
	if extractedCtx == nil {
		t.Error("Expected extracted context to not be nil")
	}
}

func TestTracer_DisabledInjectExtract(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: false,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Inject should not panic when disabled
	headers := make(http.Header)
	tracer.Inject(context.Background(), headers)

	// Extract should return background context when disabled
	ctx := tracer.Extract(headers)
	if ctx == nil {
		t.Error("Expected context to be returned even when disabled")
	}
}

func TestTracer_StartSpanWhenDisabled(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled: false,
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// StartSpan should return context and a noop span when disabled
	ctx, span := tracer.StartSpan(context.Background(), "test")

	if ctx == nil {
		t.Error("Expected context to be returned")
	}

	if span == nil {
		t.Error("Expected span to be returned")
	}

	// Should be able to call End without panic
	span.End()
}

func TestTracer_SpanFromContext(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter:    "stdout",
	}

	tracer, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Shutdown(context.Background())
		}
	}()

	// Get span from empty context
	span := tracer.SpanFromContext(context.Background())
	if span == nil {
		t.Error("Expected span to be returned even from empty context")
	}

	// Get span from context with active span
	ctx, activeSpan := tracer.StartSpan(context.Background(), "parent")
	defer activeSpan.End()

	retrievedSpan := tracer.SpanFromContext(ctx)
	if retrievedSpan == nil {
		t.Error("Expected span to be retrieved from context")
	}
}

func TestGenerateInstanceID(t *testing.T) {
	id1 := generateInstanceID()
	time.Sleep(time.Millisecond) // Ensure different timestamp
	id2 := generateInstanceID()

	if id1 == "" {
		t.Error("Expected non-empty instance ID")
	}

	if id1 == id2 {
		t.Error("Expected different instance IDs")
	}
}
