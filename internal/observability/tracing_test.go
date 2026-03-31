package observability

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTracer(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	if tracer == nil {
		t.Fatal("NewTracer() returned nil")
	}

	if tracer.config.ServiceName != config.ServiceName {
		t.Errorf("ServiceName = %v, want %v", tracer.config.ServiceName, config.ServiceName)
	}
}

func TestTracerStartSpan(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	span := tracer.StartSpan("test-operation")
	if span == nil {
		t.Fatal("StartSpan() returned nil")
	}

	if span.Name != "test-operation" {
		t.Errorf("Name = %v, want test-operation", span.Name)
	}

	if span.TraceID == "" {
		t.Error("TraceID should not be empty")
	}

	if span.SpanID == "" {
		t.Error("SpanID should not be empty")
	}

	if span.Tags["service.name"] != config.ServiceName {
		t.Errorf("service.name tag = %v, want %v", span.Tags["service.name"], config.ServiceName)
	}
}

func TestTracerStartSpanDisabled(t *testing.T) {
	config := &TraceConfig{Enabled: false}
	tracer := NewTracer(config)

	span := tracer.StartSpan("test-operation")
	if span != nil {
		t.Error("StartSpan() should return nil when disabled")
	}
}

func TestSpanFinish(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	span := tracer.StartSpan("test-operation")
	time.Sleep(1 * time.Millisecond) // Ensure some time passes
	span.Finish()

	if span.EndTime.IsZero() {
		t.Error("EndTime should not be zero after Finish()")
	}

	if span.Duration == 0 {
		t.Error("Duration should not be zero after Finish()")
	}
}

func TestSpanSetTag(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	span := tracer.StartSpan("test-operation")
	span.SetTag("key", "value")

	if span.Tags["key"] != "value" {
		t.Errorf("Tag[key] = %v, want value", span.Tags["key"])
	}
}

func TestSpanSetError(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	span := tracer.StartSpan("test-operation")
	span.SetError(fmt.Errorf("something went wrong"))

	if span.Status != SpanStatusError {
		t.Errorf("Status = %v, want SpanStatusError", span.Status)
	}

	if span.Tags["error"] != "something went wrong" {
		t.Errorf("error tag = %v, want 'something went wrong'", span.Tags["error"])
	}
}

func TestSpanFromContext(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	// Start span and add to context
	span, ctx := tracer.StartSpanFromContext(context.Background(), "test-operation")
	if span == nil {
		t.Fatal("StartSpanFromContext() returned nil")
	}

	// Extract from context
	extracted, ok := SpanFromContext(ctx)
	if !ok {
		t.Error("Expected span to be in context")
	}

	if extracted.SpanID != span.SpanID {
		t.Error("Extracted span ID should match original")
	}
}

func TestTraceIDFromContext(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	span, ctx := tracer.StartSpanFromContext(context.Background(), "test-operation")
	if span == nil {
		t.Fatal("StartSpanFromContext() returned nil")
	}

	traceID := TraceIDFromContext(ctx)
	if traceID != span.TraceID {
		t.Errorf("TraceID = %v, want %v", traceID, span.TraceID)
	}
}

func TestTraceIDFromContextEmpty(t *testing.T) {
	traceID := TraceIDFromContext(context.Background())
	if traceID != "" {
		t.Errorf("Expected empty trace ID, got %v", traceID)
	}
}

func TestTracingMiddleware(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := NewTracingMiddleware(tracer, handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	// Check that trace headers are set
	if rec.Header().Get("X-Trace-ID") == "" {
		t.Error("Expected X-Trace-ID header")
	}

	if rec.Header().Get("X-Span-ID") == "" {
		t.Error("Expected X-Span-ID header")
	}
}

func TestTracingMiddlewareWithExistingTrace(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewTracingMiddleware(tracer, handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Trace-ID", "existing-trace-id")
	req.Header.Set("X-Span-ID", "existing-span-id")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	// Should preserve existing trace ID
	if rec.Header().Get("X-Trace-ID") != "existing-trace-id" {
		t.Error("Should preserve existing trace ID")
	}
}

func TestTracingMiddlewareDisabled(t *testing.T) {
	config := &TraceConfig{Enabled: false}
	tracer := NewTracer(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewTracingMiddleware(tracer, handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	// Should not set trace headers when disabled
	if rec.Header().Get("X-Trace-ID") != "" {
		t.Error("Should not set X-Trace-ID when disabled")
	}
}

func TestWithParent(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	parent := tracer.StartSpan("parent-operation")
	child := tracer.StartSpan("child-operation", WithParent(parent))

	if child.TraceID != parent.TraceID {
		t.Error("Child should have same trace ID as parent")
	}

	if child.ParentID != parent.SpanID {
		t.Error("Child should have parent span ID")
	}
}

func TestWithTag(t *testing.T) {
	span := &Span{
		Tags: make(map[string]string),
	}

	opt := WithTag("key", "value")
	opt(span)

	if span.Tags["key"] != "value" {
		t.Errorf("Tag = %v, want value", span.Tags["key"])
	}
}

func TestTracerGetSpans(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	tracer.StartSpan("span-1")
	time.Sleep(time.Microsecond) // Ensure unique span IDs
	tracer.StartSpan("span-2")

	spans := tracer.GetSpans()
	if len(spans) != 2 {
		t.Errorf("Expected 2 spans, got %d", len(spans))
	}
}

func TestTracerClearSpans(t *testing.T) {
	config := DefaultTraceConfig()
	tracer := NewTracer(config)

	tracer.StartSpan("span-1")
	tracer.ClearSpans()

	spans := tracer.GetSpans()
	if len(spans) != 0 {
		t.Errorf("Expected 0 spans after clear, got %d", len(spans))
	}
}

