package audit

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// LogInput describes one completed request/response exchange.
type LogInput struct {
	Request        *http.Request
	ResponseWriter *ResponseCaptureWriter
	Route          *config.Route
	Service        *config.Service
	Consumer       *config.Consumer
	RequestBody    []byte
	StartedAt      time.Time
	Blocked        bool
	BlockReason    string
	ProxyErr       error
}

// Logger persists audit entries with buffered async writes.
type Logger struct {
	repo        *store.AuditRepo
	cfg         config.AuditConfig
	masker      *Masker
	entries     chan store.AuditEntry
	started     atomic.Bool
	dropped     atomic.Int64
	now         func() time.Time
	kafkaWriter *KafkaWriter
}

func NewLogger(repo *store.AuditRepo, cfg config.AuditConfig, kafka *KafkaWriter) *Logger {
	if repo == nil || !cfg.Enabled {
		return nil
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10_000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		cfg.MaxRequestBodyBytes = 64 << 10
	}
	if cfg.MaxResponseBodyBytes <= 0 {
		cfg.MaxResponseBodyBytes = 64 << 10
	}
	if strings.TrimSpace(cfg.MaskReplacement) == "" {
		cfg.MaskReplacement = "***REDACTED***"
	}

	return &Logger{
		repo:        repo,
		cfg:         cfg,
		masker:      NewMasker(cfg.MaskHeaders, cfg.MaskBodyFields, cfg.MaskReplacement),
		entries:     make(chan store.AuditEntry, cfg.BufferSize),
		now:         time.Now,
		kafkaWriter: kafka,
	}
}

func (l *Logger) Enabled() bool {
	return l != nil && l.repo != nil
}

func (l *Logger) MaxRequestBodyBytes() int64 {
	if l == nil {
		return 0
	}
	return l.cfg.MaxRequestBodyBytes
}

func (l *Logger) MaxResponseBodyBytes() int64 {
	if l == nil {
		return 0
	}
	return l.cfg.MaxResponseBodyBytes
}

func (l *Logger) Dropped() int64 {
	if l == nil {
		return 0
	}
	return l.dropped.Load()
}

// Start consumes buffered entries and flushes them in batches.
func (l *Logger) Start(ctx context.Context) {
	if !l.Enabled() {
		return
	}
	if !l.started.CompareAndSwap(false, true) {
		return
	}

	// Start Kafka writer if enabled
	if l.kafkaWriter != nil && l.kafkaWriter.Enabled() {
		defer l.kafkaWriter.Close()
	}

	ticker := time.NewTicker(l.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]store.AuditEntry, 0, l.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.repo.BatchInsert(batch); err != nil {
			log.Printf("[ERROR] audit: batch insert failed (%d entries): %v", len(batch), err)
		}

		// Also send to Kafka if enabled
		if l.kafkaWriter != nil && l.kafkaWriter.Enabled() {
			if err := l.kafkaWriter.WriteBatch(batch); err != nil {
				log.Printf("[ERROR] audit: kafka write batch failed (%d entries): %v", len(batch), err)
			}
		}

		batch = batch[:0]
	}
	appendEntry := func(entry store.AuditEntry) {
		batch = append(batch, entry)
		if len(batch) >= l.cfg.BatchSize {
			flush()
		}
	}

	for {
		select {
		case <-ctx.Done():
			drain := true
			for drain {
				select {
				case entry := <-l.entries:
					appendEntry(entry)
				default:
					drain = false
				}
			}
			flush()
			return
		case entry := <-l.entries:
			appendEntry(entry)
		case <-ticker.C:
			flush()
		}
	}
}

// Log converts runtime request state into an audit record and queues it without blocking.
func (l *Logger) Log(input LogInput) {
	if !l.Enabled() {
		return
	}
	entry := l.buildEntry(input)

	if !l.started.Load() {
		if err := l.repo.BatchInsert([]store.AuditEntry{entry}); err != nil {
			log.Printf("[ERROR] audit: direct insert failed: %v", err)
		}
		return
	}

	select {
	case l.entries <- entry:
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) buildEntry(input LogInput) store.AuditEntry {
	now := l.now().UTC()
	started := input.StartedAt
	if started.IsZero() {
		started = now
	}
	latency := now.Sub(started)
	if latency < 0 {
		latency = 0
	}

	statusCode := 0
	bytesOut := int64(0)
	responseHeaders := map[string]any{}
	responseBody := []byte(nil)
	if input.ResponseWriter != nil {
		statusCode = input.ResponseWriter.StatusCode()
		bytesOut = input.ResponseWriter.BytesWritten()
		responseHeaders = l.masker.MaskHeaders(input.ResponseWriter.Header())
		responseBody = input.ResponseWriter.BodyBytes()
	}

	requestID := ""
	host := ""
	path := ""
	query := ""
	method := ""
	clientIP := ""
	userAgent := ""
	requestHeaders := map[string]any{}
	bytesIn := int64(0)
	if input.Request != nil {
		requestID = strings.TrimSpace(input.Request.Header.Get("X-Request-ID"))
		host = strings.TrimSpace(input.Request.Host)
		if input.Request.URL != nil {
			path = strings.TrimSpace(input.Request.URL.Path)
			query = strings.TrimSpace(input.Request.URL.RawQuery)
		}
		method = strings.TrimSpace(strings.ToUpper(input.Request.Method))
		clientIP = requestClientIP(input.Request)
		userAgent = strings.TrimSpace(input.Request.UserAgent())
		requestHeaders = l.masker.MaskHeaders(input.Request.Header)
		if input.Request.ContentLength > 0 {
			bytesIn = input.Request.ContentLength
		}
	}
	if bytesIn <= 0 {
		bytesIn = int64(len(input.RequestBody))
	}

	userID := ""
	consumerName := ""
	if input.Consumer != nil {
		userID = strings.TrimSpace(input.Consumer.ID)
		consumerName = strings.TrimSpace(input.Consumer.Name)
	}

	routeID := ""
	routeName := ""
	if input.Route != nil {
		routeID = strings.TrimSpace(input.Route.ID)
		routeName = strings.TrimSpace(input.Route.Name)
	}

	serviceName := ""
	if input.Service != nil {
		serviceName = strings.TrimSpace(input.Service.Name)
	}

	errorMessage := ""
	if input.ProxyErr != nil && !errors.Is(input.ProxyErr, context.Canceled) {
		errorMessage = input.ProxyErr.Error()
	}

	var maskedRequestBody, maskedResponseBody []byte
	if l.cfg.StoreRequestBody {
		maskedRequestBody = l.masker.MaskBody(input.RequestBody)
	}
	if l.cfg.StoreResponseBody {
		maskedResponseBody = l.masker.MaskBody(responseBody)
	}

	return store.AuditEntry{
		RequestID:       requestID,
		RouteID:         routeID,
		RouteName:       routeName,
		ServiceName:     serviceName,
		UserID:          userID,
		ConsumerName:    consumerName,
		Method:          method,
		Host:            host,
		Path:            path,
		Query:           query,
		StatusCode:      statusCode,
		LatencyMS:       latency.Milliseconds(),
		BytesIn:         bytesIn,
		BytesOut:        bytesOut,
		ClientIP:        clientIP,
		UserAgent:       userAgent,
		Blocked:         input.Blocked,
		BlockReason:     strings.TrimSpace(input.BlockReason),
		RequestHeaders:  requestHeaders,
		RequestBody:     string(maskedRequestBody),
		ResponseHeaders: responseHeaders,
		ResponseBody:    string(maskedResponseBody),
		ErrorMessage:    errorMessage,
		CreatedAt:       now,
	}
}

func requestClientIP(req *http.Request) string {
	return netutil.ExtractClientIP(req)
}
