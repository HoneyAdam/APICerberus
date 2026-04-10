package audit

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// Pools for reusing audit entry allocations in the hot path.
var (
	// headerMapPool reuses map[string]any for request/response headers.
	headerMapPool = sync.Pool{
		New: func() any {
			return make(map[string]any, 16)
		},
	}
)

// getHeaderMap obtains a reusable map from the pool.
func getHeaderMap() map[string]any {
	return headerMapPool.Get().(map[string]any)
}

// putHeaderMap returns a map to the pool after clearing it.
func putHeaderMap(m map[string]any) {
	for k := range m {
		delete(m, k)
	}
	headerMapPool.Put(m)
}

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
		// Retry on SQLITE_BUSY with exponential backoff before giving up
		var err error
		for retries := 0; retries < 3; retries++ {
			err = l.repo.BatchInsert(batch)
			if err == nil {
				break
			}
			if !strings.Contains(err.Error(), "database is locked") && !strings.Contains(err.Error(), "SQLITE_BUSY") {
				break // non-retryable error
			}
			time.Sleep(100 * time.Millisecond * (1 << retries))
		}
		if err != nil {
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
		// Retry on SQLITE_BUSY before giving up
		var err error
		for retries := 0; retries < 3; retries++ {
			err = l.repo.BatchInsert([]store.AuditEntry{entry})
			if err == nil {
				break
			}
			if !strings.Contains(err.Error(), "database is locked") && !strings.Contains(err.Error(), "SQLITE_BUSY") {
				break
			}
			time.Sleep(100 * time.Millisecond * (1 << retries))
		}
		if err != nil {
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
	respHeadersMap := getHeaderMap()
	defer putHeaderMap(respHeadersMap)
	responseBody := []byte(nil)
	if input.ResponseWriter != nil {
		statusCode = input.ResponseWriter.StatusCode()
		bytesOut = input.ResponseWriter.BytesWritten()
		l.masker.MaskHeadersInto(input.ResponseWriter.Header(), respHeadersMap)
		responseBody = input.ResponseWriter.BodyBytes()
	}

	requestID := ""
	host := ""
	path := ""
	query := ""
	method := ""
	clientIP := ""
	userAgent := ""
	reqHeadersMap := getHeaderMap()
	defer putHeaderMap(reqHeadersMap)
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
		l.masker.MaskHeadersInto(input.Request.Header, reqHeadersMap)
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
		buf := l.masker.MaskBody(input.RequestBody)
		maskedRequestBody = make([]byte, len(buf))
		copy(maskedRequestBody, buf)
	}
	if l.cfg.StoreResponseBody {
		buf := l.masker.MaskBody(responseBody)
		maskedResponseBody = make([]byte, len(buf))
		copy(maskedResponseBody, buf)
	}

	// Copy header maps so the pooled maps can be reused immediately.
	var reqHeaders, respHeaders map[string]any
	if len(reqHeadersMap) > 0 {
		reqHeaders = make(map[string]any, len(reqHeadersMap))
		for k, v := range reqHeadersMap {
			reqHeaders[k] = v
		}
	}
	if len(respHeadersMap) > 0 {
		respHeaders = make(map[string]any, len(respHeadersMap))
		for k, v := range respHeadersMap {
			respHeaders[k] = v
		}
	}
	if reqHeaders == nil {
		reqHeaders = map[string]any{}
	}
	if respHeaders == nil {
		respHeaders = map[string]any{}
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
		RequestHeaders:  reqHeaders,
		RequestBody:     string(maskedRequestBody),
		ResponseHeaders: respHeaders,
		ResponseBody:    string(maskedResponseBody),
		ErrorMessage:    errorMessage,
		CreatedAt:       now,
	}
}

func requestClientIP(req *http.Request) string {
	return netutil.ExtractClientIP(req)
}
