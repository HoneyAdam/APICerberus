package observability

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Tracer provides distributed tracing capabilities.
type Tracer struct {
	mu     sync.RWMutex
	spans  map[string]*Span
	config *TraceConfig
}

// TraceConfig holds tracer configuration.
type TraceConfig struct {
	Enabled     bool              `yaml:"enabled"`
	ServiceName string            `yaml:"service_name"`
	Endpoint    string            `yaml:"endpoint"`
	SampleRate  float64           `yaml:"sample_rate"`
	Tags        map[string]string `yaml:"tags"`
}

// DefaultTraceConfig returns default trace configuration.
func DefaultTraceConfig() *TraceConfig {
	return &TraceConfig{
		Enabled:     true,
		ServiceName: "apicerberus-gateway",
		SampleRate:  1.0,
		Tags:        make(map[string]string),
	}
}

// Span represents a trace span.
type Span struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	ParentID   string            `json:"parent_id,omitempty"`
	Name       string            `json:"name"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Tags       map[string]string `json:"tags"`
	Status     SpanStatus        `json:"status"`
	Children   []*Span           `json:"children,omitempty"`
}

// SpanStatus represents the status of a span.
type SpanStatus int

const (
	SpanStatusOK SpanStatus = iota
	SpanStatusError
)

// NewTracer creates a new tracer.
func NewTracer(config *TraceConfig) *Tracer {
	if config == nil {
		config = DefaultTraceConfig()
	}

	return &Tracer{
		spans:  make(map[string]*Span),
		config: config,
	}
}

// StartSpan starts a new span.
func (t *Tracer) StartSpan(name string, opts ...SpanOption) *Span {
	if !t.config.Enabled {
		return nil
	}

	span := &Span{
		TraceID:   generateTraceID(),
		SpanID:    generateSpanID(),
		Name:      name,
		StartTime: time.Now(),
		Tags:      make(map[string]string),
		Status:    SpanStatusOK,
	}

	// Apply service tags
	for k, v := range t.config.Tags {
		span.Tags[k] = v
	}
	span.Tags["service.name"] = t.config.ServiceName

	// Apply options
	for _, opt := range opts {
		opt(span)
	}

	t.mu.Lock()
	t.spans[span.SpanID] = span
	t.mu.Unlock()

	return span
}

// StartSpanFromContext starts a new span from a context.
func (t *Tracer) StartSpanFromContext(ctx context.Context, name string, opts ...SpanOption) (*Span, context.Context) {
	if !t.config.Enabled {
		return nil, ctx
	}

	// Check if there's a parent span in context
	if parent, ok := ctx.Value(spanContextKey).(*Span); ok {
		opts = append(opts, WithParent(parent))
	}

	span := t.StartSpan(name, opts...)
	if span == nil {
		return nil, ctx
	}

	// Store span in context
	ctx = context.WithValue(ctx, spanContextKey, span)

	return span, ctx
}

// Finish finishes a span.
func (s *Span) Finish() {
	if s == nil {
		return
	}
	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
}

// SetTag sets a tag on the span.
func (s *Span) SetTag(key, value string) {
	if s == nil {
		return
	}
	s.Tags[key] = value
}

// SetError marks the span as having an error.
func (s *Span) SetError(err error) {
	if s == nil || err == nil {
		return
	}
	s.Status = SpanStatusError
	s.Tags["error"] = err.Error()
}

// SpanOption configures a span.
type SpanOption func(*Span)

// WithParent sets the parent span.
func WithParent(parent *Span) SpanOption {
	return func(s *Span) {
		if parent != nil {
			s.TraceID = parent.TraceID
			s.ParentID = parent.SpanID
			parent.Children = append(parent.Children, s)
		}
	}
}

// WithTraceID sets the trace ID.
func WithTraceID(traceID string) SpanOption {
	return func(s *Span) {
		s.TraceID = traceID
	}
}

// WithTag sets a tag.
func WithTag(key, value string) SpanOption {
	return func(s *Span) {
		s.Tags[key] = value
	}
}

// context key for span storage
type spanContextKeyType struct{}

var spanContextKey = spanContextKeyType{}

// SpanFromContext extracts a span from context.
func SpanFromContext(ctx context.Context) (*Span, bool) {
	span, ok := ctx.Value(spanContextKey).(*Span)
	return span, ok
}

// TraceIDFromContext extracts trace ID from context.
func TraceIDFromContext(ctx context.Context) string {
	if span, ok := SpanFromContext(ctx); ok {
		return span.TraceID
	}
	return ""
}

// TracingMiddleware wraps an HTTP handler with tracing.
type TracingMiddleware struct {
	tracer *Tracer
	next   http.Handler
}

// NewTracingMiddleware creates a new tracing middleware.
func NewTracingMiddleware(tracer *Tracer, next http.Handler) *TracingMiddleware {
	return &TracingMiddleware{
		tracer: tracer,
		next:   next,
	}
}

// ServeHTTP implements http.Handler.
func (tm *TracingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if tm.tracer == nil || !tm.tracer.config.Enabled {
		tm.next.ServeHTTP(w, r)
		return
	}

	// Extract trace ID from headers if present
	traceID := r.Header.Get("X-Trace-ID")
	parentSpanID := r.Header.Get("X-Span-ID")

	opts := []SpanOption{
		WithTag("http.method", r.Method),
		WithTag("http.url", r.URL.String()),
		WithTag("http.host", r.Host),
	}

	if traceID != "" {
		opts = append(opts, WithTraceID(traceID))
	}

	span, ctx := tm.tracer.StartSpanFromContext(r.Context(), "http.request", opts...)
	if span != nil {
		defer span.Finish()

		// Set parent if provided
		if parentSpanID != "" {
			span.ParentID = parentSpanID
		}

		// Add trace headers to response
		w.Header().Set("X-Trace-ID", span.TraceID)
		w.Header().Set("X-Span-ID", span.SpanID)

		// Wrap response writer to capture status code
		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		tm.next.ServeHTTP(recorder, r.WithContext(ctx))

		// Record status code
		span.SetTag("http.status_code", fmt.Sprintf("%d", recorder.statusCode))
		if recorder.statusCode >= 500 {
			span.SetError(fmt.Errorf("HTTP %d", recorder.statusCode))
		}
	} else {
		tm.next.ServeHTTP(w, r)
	}
}

// statusRecorder records HTTP status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

// generateTraceID generates a unique trace ID.
func generateTraceID() string {
	return fmt.Sprintf("trace_%d", time.Now().UnixNano())
}

// generateSpanID generates a unique span ID.
func generateSpanID() string {
	return fmt.Sprintf("span_%d", time.Now().UnixNano())
}

// GetSpans returns all recorded spans.
func (t *Tracer) GetSpans() []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	spans := make([]*Span, 0, len(t.spans))
	for _, span := range t.spans {
		spans = append(spans, span)
	}
	return spans
}

// ClearSpans clears all recorded spans.
func (t *Tracer) ClearSpans() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.spans = make(map[string]*Span)
}
