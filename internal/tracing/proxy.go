package tracing

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ProxyTracer provides tracing for proxy operations.
type ProxyTracer struct {
	tracer *Tracer
}

// NewProxyTracer creates a new proxy tracer.
func NewProxyTracer(tracer *Tracer) *ProxyTracer {
	return &ProxyTracer{tracer: tracer}
}

// StartUpstreamSpan starts a span for an upstream request.
func (pt *ProxyTracer) StartUpstreamSpan(ctx context.Context, targetID, targetAddress string) (context.Context, trace.Span) {
	if pt.tracer == nil || !pt.tracer.Enabled() {
		return ctx, trace.SpanFromContext(ctx)
	}

	ctx, span := pt.tracer.StartSpan(ctx, "proxy.upstream",
		trace.WithSpanKind(trace.SpanKindClient),
	)

	span.SetAttributes(
		attribute.String("upstream.target_id", targetID),
		attribute.String("upstream.target_address", targetAddress),
		attribute.String("proxy.system", "apicerebrus"),
	)

	return ctx, span
}

// EndUpstreamSpan ends the upstream span with the result.
func (pt *ProxyTracer) EndUpstreamSpan(span trace.Span, resp *http.Response, err error) {
	if span == nil {
		return
	}

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(
			attribute.Bool("error", true),
			attribute.String("error.message", err.Error()),
		)
	} else if resp != nil {
		span.SetAttributes(
			attribute.Int("http.status_code", resp.StatusCode),
			attribute.Int64("http.response_content_length", resp.ContentLength),
		)

		if resp.StatusCode >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("Upstream returned %d", resp.StatusCode))
			span.SetAttributes(attribute.Bool("error", true))
		}
	}

	span.End()
}

// TraceRoundTripper wraps an http.RoundTripper with tracing.
func (pt *ProxyTracer) TraceRoundTripper(base http.RoundTripper) http.RoundTripper {
	if pt.tracer == nil || !pt.tracer.Enabled() {
		return base
	}

	return &tracedRoundTripper{
		base:   base,
		tracer: pt.tracer,
	}
}

// tracedRoundTripper wraps http.RoundTripper with tracing.
type tracedRoundTripper struct {
	base   http.RoundTripper
	tracer *Tracer
}

func (t *tracedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Start span for the outgoing request
	ctx, span := t.tracer.StartSpan(ctx, "http.client",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	// Set span attributes
	span.SetAttributes(
		attribute.String("http.method", req.Method),
		attribute.String("http.url", req.URL.String()),
		attribute.String("http.target", req.URL.Path),
		attribute.String("http.host", req.URL.Host),
		attribute.String("http.scheme", req.URL.Scheme),
	)

	// Inject trace context into request headers
	t.tracer.Inject(ctx, req.Header)

	// Update request with new context
	req = req.WithContext(ctx)

	// Execute the request
	resp, err := t.base.RoundTrip(req)

	// Record result
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(
			attribute.Bool("error", true),
			attribute.String("error.type", "transport"),
			attribute.String("error.message", err.Error()),
		)
	} else {
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

		if resp.StatusCode >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
			span.SetAttributes(attribute.Bool("error", true))
		}
	}

	return resp, err
}
