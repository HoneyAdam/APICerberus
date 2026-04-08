// Package tracing provides OpenTelemetry tracing support for APICerebrus.
package tracing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// Tracer is the global tracer provider wrapper.
type Tracer struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	config   config.TracingConfig
	enabled  bool
}

// New creates a new Tracer with the given configuration.
func New(cfg config.TracingConfig) (*Tracer, error) {
	if !cfg.Enabled {
		return &Tracer{enabled: false}, nil
	}

	// Set defaults
	if cfg.ServiceName == "" {
		cfg.ServiceName = "apicerebrus"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "1.0.0"
	}
	if cfg.Exporter == "" {
		cfg.Exporter = "stdout"
	}
	if cfg.SamplingRate == 0 {
		cfg.SamplingRate = 1.0
	}
	if cfg.BatchTimeout == 0 {
		cfg.BatchTimeout = 5 * time.Second
	}
	if cfg.MaxQueueSize == 0 {
		cfg.MaxQueueSize = 2048
	}
	if cfg.MaxExportBatchSize == 0 {
		cfg.MaxExportBatchSize = 512
	}

	// Create exporter
	exporter, err := createExporter(cfg)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	// Create resource
	res, err := createResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	// Create sampler
	sampler := sdktrace.TraceIDRatioBased(cfg.SamplingRate)

	// Create provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(cfg.BatchTimeout),
			sdktrace.WithMaxQueueSize(cfg.MaxQueueSize),
			sdktrace.WithMaxExportBatchSize(cfg.MaxExportBatchSize),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set as global provider
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer := provider.Tracer(
		cfg.ServiceName,
		trace.WithInstrumentationVersion(cfg.ServiceVersion),
	)

	return &Tracer{
		provider: provider,
		tracer:   tracer,
		config:   cfg,
		enabled:  true,
	}, nil
}

// Enabled returns true if tracing is enabled.
func (t *Tracer) Enabled() bool {
	return t.enabled
}

// Shutdown gracefully shuts down the tracer provider.
func (t *Tracer) Shutdown(ctx context.Context) error {
	if !t.enabled || t.provider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return t.provider.Shutdown(ctx)
}

// StartSpan starts a new span with the given name and options.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !t.enabled {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from context.
func (t *Tracer) SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// Inject propagates the span context into the HTTP headers.
func (t *Tracer) Inject(ctx context.Context, header http.Header) {
	if !t.enabled {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

// Extract extracts the span context from HTTP headers.
func (t *Tracer) Extract(header http.Header) context.Context {
	if !t.enabled {
		return context.Background()
	}
	return otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(header))
}

// createExporter creates the appropriate trace exporter based on configuration.
func createExporter(cfg config.TracingConfig) (sdktrace.SpanExporter, error) {
	switch cfg.Exporter {
	case "otlp", "otlp-http":
		return createOTLPHttpExporter(cfg)
	case "otlp-grpc":
		return createOTLPGrpcExporter(cfg)
	case "stdout", "console":
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
}

// createOTLPHttpExporter creates an OTLP HTTP exporter.
func createOTLPHttpExporter(cfg config.TracingConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{}

	if cfg.OTLPEndpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(cfg.OTLPEndpoint))
	}

	if len(cfg.OTLPHeaders) > 0 {
		headers := make(map[string]string, len(cfg.OTLPHeaders))
		for k, v := range cfg.OTLPHeaders {
			headers[k] = v
		}
		opts = append(opts, otlptracehttp.WithHeaders(headers))
	}

	client := otlptracehttp.NewClient(opts...)
	return otlptrace.New(context.Background(), client)
}

// createOTLPGrpcExporter creates an OTLP gRPC exporter.
func createOTLPGrpcExporter(cfg config.TracingConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{}

	if cfg.OTLPEndpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint))
	}

	if len(cfg.OTLPHeaders) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(cfg.OTLPHeaders))
	}

	client := otlptracegrpc.NewClient(opts...)
	return otlptrace.New(context.Background(), client)
}

// createResource creates a resource with service information.
func createResource(cfg config.TracingConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.ServiceInstanceID(generateInstanceID()),
		semconv.DeploymentEnvironmentName("production"),
	}

	// Add custom attributes
	for k, v := range cfg.Attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
}

// generateInstanceID generates a unique instance ID for this gateway instance.
func generateInstanceID() string {
	return fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), time.Now().UnixNano())
}
