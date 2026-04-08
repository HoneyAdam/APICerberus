package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents log levels
type LogLevel int

const (
	DebugLevel LogLevel = 0
	InfoLevel  LogLevel = 1
	WarnLevel  LogLevel = 2
	ErrorLevel LogLevel = 3
	FatalLevel LogLevel = 4
)

// String returns the string representation of log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// StructuredLogger provides structured JSON logging
type StructuredLogger struct {
	mu       sync.RWMutex
	level    LogLevel
	output   io.Writer
	encoder  *json.Encoder
	fields   map[string]any
	hooks    []LogHook
	caller   bool
	service  string
	version  string
	hostname string
	pid      int
}

// LogHook is a function that can process log entries
type LogHook func(level LogLevel, entry LogEntry)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Service   string                 `json:"service,omitempty"`
	Version   string                 `json:"version,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	SpanID    string                 `json:"span_id,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	Hostname  string                 `json:"hostname,omitempty"`
	PID       int                    `json:"pid,omitempty"`
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(output io.Writer, level LogLevel) *StructuredLogger {
	if output == nil {
		output = os.Stdout
	}

	hostname, _ := os.Hostname()

	return &StructuredLogger{
		level:    level,
		output:   output,
		encoder:  json.NewEncoder(output),
		fields:   make(map[string]any),
		hooks:    make([]LogHook, 0),
		caller:   true,
		service:  "apicerberus",
		version:  "1.0.0",
		hostname: hostname,
		pid:      os.Getpid(),
	}
}

// WithField adds a field to the logger
func (l *StructuredLogger) WithField(key string, value any) *StructuredLogger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &StructuredLogger{
		level:   l.level,
		output:  l.output,
		encoder: l.encoder,
		fields:  make(map[string]any),
		hooks:   l.hooks,
		caller:  l.caller,
		service: l.service,
		version: l.version,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new field
	newLogger.fields[key] = value

	return newLogger
}

// WithFields adds multiple fields
func (l *StructuredLogger) WithFields(fields map[string]any) *StructuredLogger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &StructuredLogger{
		level:   l.level,
		output:  l.output,
		encoder: l.encoder,
		fields:  make(map[string]any),
		hooks:   l.hooks,
		caller:  l.caller,
		service: l.service,
		version: l.version,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// WithContext extracts trace info from context
func (l *StructuredLogger) WithContext(ctx context.Context) *StructuredLogger {
	// Extract trace ID and span ID from context
	traceID, _ := ctx.Value("trace_id").(string)
	spanID, _ := ctx.Value("span_id").(string)

	return l.WithFields(map[string]any{
		"trace_id": traceID,
		"span_id":  spanID,
	})
}

// log writes a log entry
func (l *StructuredLogger) log(level LogLevel, msg string, args ...any) {
	if level < l.level {
		return
	}

	message := fmt.Sprintf(msg, args...)

	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		Service:   l.service,
		Version:   l.version,
		Fields:    l.fields,
		Hostname:  l.hostname,
		PID:       l.pid,
	}

	// Add caller info
	if l.caller {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			entry.Caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}

	// Extract trace info from fields
	if traceID, ok := l.fields["trace_id"].(string); ok {
		entry.TraceID = traceID
	}
	if spanID, ok := l.fields["span_id"].(string); ok {
		entry.SpanID = spanID
	}

	l.mu.Lock()
	l.encoder.Encode(entry)
	l.mu.Unlock()

	// Run hooks
	for _, hook := range l.hooks {
		hook(level, entry)
	}

	// Fatal handling
	if level == FatalLevel {
		os.Exit(1)
	}
}

// Debug logs a debug message
func (l *StructuredLogger) Debug(msg string, args ...any) {
	l.log(DebugLevel, msg, args...)
}

// Info logs an info message
func (l *StructuredLogger) Info(msg string, args ...any) {
	l.log(InfoLevel, msg, args...)
}

// Warn logs a warning message
func (l *StructuredLogger) Warn(msg string, args ...any) {
	l.log(WarnLevel, msg, args...)
}

// Error logs an error message
func (l *StructuredLogger) Error(msg string, args ...any) {
	l.log(ErrorLevel, msg, args...)
}

// Fatal logs a fatal message
func (l *StructuredLogger) Fatal(msg string, args ...any) {
	l.log(FatalLevel, msg, args...)
}

// AddHook adds a log hook
func (l *StructuredLogger) AddHook(hook LogHook) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks = append(l.hooks, hook)
}

// SetLevel sets the log level
func (l *StructuredLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetService sets the service name
func (l *StructuredLogger) SetService(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.service = name
}

// SetVersion sets the version
func (l *StructuredLogger) SetVersion(version string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.version = version
}

// FileLogHook writes logs to a file
type FileLogHook struct {
	writer io.Writer
	level  LogLevel
}

// NewFileLogHook creates a file log hook
func NewFileLogHook(path string, level LogLevel) (*FileLogHook, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &FileLogHook{
		writer: file,
		level:  level,
	}, nil
}

// Hook returns the hook function
func (h *FileLogHook) Hook() LogHook {
	encoder := json.NewEncoder(h.writer)
	return func(level LogLevel, entry LogEntry) {
		if level >= h.level {
			encoder.Encode(entry)
		}
	}
}

// Close closes the file hook
func (h *FileLogHook) Close() error {
	if closer, ok := h.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// FilterLogHook filters logs by level
func FilterLogHook(minLevel LogLevel) LogHook {
	return func(level LogLevel, entry LogEntry) {
		if level >= minLevel {
			// Process
		}
	}
}

// RequestLogger middleware logs HTTP requests
type RequestLogger struct {
	logger *StructuredLogger
}

// NewRequestLogger creates a request logger
func NewRequestLogger(logger *StructuredLogger) *RequestLogger {
	return &RequestLogger{logger: logger}
}

// Middleware returns the middleware function
func (rl *RequestLogger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		logger := rl.logger.WithFields(map[string]any{
			"method":         r.Method,
			"path":           r.URL.Path,
			"status":         wrapped.statusCode,
			"duration":       duration.Milliseconds(),
			"duration_human": duration.String(),
			"client_ip":      getClientIP(r),
			"user_agent":     r.UserAgent(),
		})

		if wrapped.statusCode >= 500 {
			logger.Error("HTTP request error")
		} else if wrapped.statusCode >= 400 {
			logger.Warn("HTTP request warning")
		} else {
			logger.Info("HTTP request")
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take first IP
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-Ip
	realIP := r.Header.Get("X-Real-Ip")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// Global logger
var globalLogger = NewStructuredLogger(os.Stdout, InfoLevel)

// GetGlobalLogger returns the global logger
func GetGlobalLogger() *StructuredLogger {
	return globalLogger
}

// SetGlobalLogger sets the global logger
func SetGlobalLogger(logger *StructuredLogger) {
	globalLogger = logger
}

// Convenience functions
func Debug(msg string, args ...any) {
	globalLogger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	globalLogger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	globalLogger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	globalLogger.Error(msg, args...)
}

func Fatal(msg string, args ...any) {
	globalLogger.Fatal(msg, args...)
}

// Package-level WithField
func WithField(key string, value any) *StructuredLogger {
	return globalLogger.WithField(key, value)
}

// Package-level WithFields
func WithFields(fields map[string]any) *StructuredLogger {
	return globalLogger.WithFields(fields)
}
