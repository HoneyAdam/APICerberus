package tracing

import (
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Middleware is an HTTP middleware that traces incoming requests.
type Middleware struct {
	tracer *Tracer
}

// NewMiddleware creates a new tracing middleware.
func NewMiddleware(tracer *Tracer) *Middleware {
	return &Middleware{tracer: tracer}
}

// Wrap wraps an HTTP handler with tracing.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if m.tracer == nil || !m.tracer.Enabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from incoming request
		ctx := m.tracer.Extract(r.Header)

		// Start span
		ctx, span := m.tracer.StartSpan(ctx, "http.request",
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		// Add span attributes
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.target", r.URL.Path),
			attribute.String("http.host", r.Host),
			attribute.String("http.scheme", getScheme(r)),
			attribute.String("http.flavor", r.Proto),
			attribute.String("http.user_agent", r.UserAgent()),
		)

		// Add client IP
		if clientIP := getClientIP(r); clientIP != "" {
			span.SetAttributes(attribute.String("http.client_ip", clientIP))
		}

		// Add API key info if present
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			span.SetAttributes(attribute.String("api.key_prefix", apiKey[:min(len(apiKey), 8)]+"..."))
		}

		// Create response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			span:           span,
		}

		// Update request with new context
		r = r.WithContext(ctx)

		// Record start time
		start := time.Now()

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Record duration
		duration := time.Since(start)

		// Set response attributes
		span.SetAttributes(
			attribute.Int("http.status_code", wrapped.statusCode),
			attribute.Int64("http.response_content_length", wrapped.bytesWritten),
			attribute.Int64("http.request_content_length", r.ContentLength),
			attribute.Int64("http.duration_ms", duration.Milliseconds()),
		)

		// Set status based on response code
		if wrapped.statusCode >= 500 {
			span.SetStatus(codes.Error, "Server Error")
			span.SetAttributes(attribute.Bool("error", true))
		} else if wrapped.statusCode >= 400 {
			span.SetStatus(codes.Error, "Client Error")
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
	span         trace.Span
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true

		// Record error details for 5xx responses
		if code >= 500 {
			rw.span.SetAttributes(
				attribute.Int("http.status_code", code),
				attribute.String("error.type", "http_"+strconv.Itoa(code)),
			)
		}
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func (rw *responseWriter) Header() http.Header {
	return rw.ResponseWriter.Header()
}

// getScheme returns the scheme (http or https) from the request.
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-Ip header
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
