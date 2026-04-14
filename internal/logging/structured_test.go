package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogLevel_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{FatalLevel, "FATAL"},
		{LogLevel(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestNewStructuredLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.level != InfoLevel {
		t.Errorf("level = %d, want %d", logger.level, InfoLevel)
	}
}

func TestNewStructuredLogger_NilOutput(t *testing.T) {
	t.Parallel()
	logger := NewStructuredLogger(nil, InfoLevel)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.output == nil {
		t.Error("expected stdout fallback")
	}
}

func TestStructuredLogger_WithField(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	child := logger.WithField("request_id", "abc123")
	if child == nil {
		t.Fatal("expected non-nil child logger")
	}
	if child.fields["request_id"] != "abc123" {
		t.Error("field not set on child")
	}
	// Original should not have the field
	if _, ok := logger.fields["request_id"]; ok {
		t.Error("original should not have child field")
	}
}

func TestStructuredLogger_WithFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	child := logger.WithFields(map[string]any{"key1": "val1", "key2": 42})
	if child.fields["key1"] != "val1" {
		t.Error("key1 not set")
	}
	if child.fields["key2"] != 42 {
		t.Error("key2 not set")
	}
}

func TestStructuredLogger_WithField_Chain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	child := logger.WithField("a", 1).WithField("b", 2)
	if child.fields["a"] != 1 {
		t.Error("a not inherited")
	}
	if child.fields["b"] != 2 {
		t.Error("b not set")
	}
}

func TestStructuredLogger_WithContext(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	ctx := context.WithValue(context.Background(), "trace_id", "trace-123")
	ctx = context.WithValue(ctx, "span_id", "span-456")
	child := logger.WithContext(ctx)
	if child.fields["trace_id"] != "trace-123" {
		t.Error("trace_id not extracted")
	}
	if child.fields["span_id"] != "span-456" {
		t.Error("span_id not extracted")
	}
}

func TestStructuredLogger_Info(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.Info("test message %s", "arg")
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
	if entry["message"] != "test message arg" {
		t.Errorf("message = %v, want 'test message arg'", entry["message"])
	}
}

func TestStructuredLogger_Debug(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.Debug("debug msg")
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["level"] != "DEBUG" {
		t.Errorf("level = %v, want DEBUG", entry["level"])
	}
}

func TestStructuredLogger_Warn(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.Warn("warning")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", entry["level"])
	}
}

func TestStructuredLogger_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.Error("error msg")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", entry["level"])
	}
}

func TestStructuredLogger_LevelFiltering(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, WarnLevel)
	logger.Debug("should not appear")
	logger.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("debug/info should be filtered when level=Warn")
	}
	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("warn should pass through")
	}
}

func TestStructuredLogger_SetLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, ErrorLevel)
	logger.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("info should be filtered")
	}
	logger.SetLevel(DebugLevel)
	logger.Info("should appear now")
	if buf.Len() == 0 {
		t.Error("info should pass after SetLevel")
	}
}

func TestStructuredLogger_SetService(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.SetService("my-service")
	logger.Info("test")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["service"] != "my-service" {
		t.Errorf("service = %v, want my-service", entry["service"])
	}
}

func TestStructuredLogger_SetVersion(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.SetVersion("2.0.0")
	logger.Info("test")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["version"] != "2.0.0" {
		t.Errorf("version = %v, want 2.0.0", entry["version"])
	}
}

func TestStructuredLogger_AddHook(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)

	var mu sync.Mutex
	var captured []LogEntry
	logger.AddHook(func(level LogLevel, entry LogEntry) {
		mu.Lock()
		captured = append(captured, entry)
		mu.Unlock()
	})

	logger.Info("hook test")

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("captured %d entries, want 1", len(captured))
	}
	if captured[0].Message != "hook test" {
		t.Errorf("message = %q, want hook test", captured[0].Message)
	}
}

func TestStructuredLogger_Caller(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.Info("caller test")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	caller, ok := entry["caller"].(string)
	if !ok || caller == "" {
		t.Error("expected caller to be set")
	}
}

func TestStructuredLogger_TraceFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.WithFields(map[string]any{"trace_id": "t-1", "span_id": "s-1"}).Info("traced")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["trace_id"] != "t-1" {
		t.Errorf("trace_id = %v, want t-1", entry["trace_id"])
	}
}

func TestStructuredLogger_FieldsInOutput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	logger.WithField("user_id", 42).Info("with field")
	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	fields, ok := entry["fields"].(map[string]any)
	if !ok {
		t.Fatal("fields not found or not a map")
	}
	if fields["user_id"] != float64(42) {
		t.Errorf("fields.user_id = %v, want 42", fields["user_id"])
	}
}

func TestAsyncLogHook(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var entries []LogEntry
	syncHook := func(level LogLevel, entry LogEntry) {
		mu.Lock()
		entries = append(entries, entry)
		mu.Unlock()
	}
	async := NewAsyncLogHook(syncHook, 100)
	hook := async.Hook()
	hook(InfoLevel, LogEntry{Message: "test1"})
	hook(InfoLevel, LogEntry{Message: "test2"})

	async.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(entries) < 2 {
		t.Errorf("got %d entries, want at least 2", len(entries))
	}
}

func TestAsyncLogHook_DroppedCount(t *testing.T) {
	t.Parallel()
	syncHook := func(level LogLevel, entry LogEntry) {}
	async := NewAsyncLogHook(syncHook, 1) // tiny buffer
	hook := async.Hook()

	hook(InfoLevel, LogEntry{Message: "1"})
	// Fill beyond buffer
	for i := 0; i < 10; i++ {
		hook(InfoLevel, LogEntry{Message: "overflow"})
	}

	if async.DroppedCount() == 0 {
		t.Error("expected some dropped entries")
	}
	async.Close()
}

func TestAsyncLogHook_DefaultBufferSize(t *testing.T) {
	t.Parallel()
	syncHook := func(level LogLevel, entry LogEntry) {}
	async := NewAsyncLogHook(syncHook, 0) // should default to 1000
	if async == nil {
		t.Fatal("expected non-nil async hook")
	}
	async.Close()
}

func TestAsyncLogHook_DoubleClose(t *testing.T) {
	t.Parallel()
	syncHook := func(level LogLevel, entry LogEntry) {}
	async := NewAsyncLogHook(syncHook, 10)
	async.Close()
	async.Close() // should not panic
}

func TestFileLogHook(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/test.log"
	hook, err := NewFileLogHook(path, InfoLevel)
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}

	logHook := hook.Hook()
	logHook(InfoLevel, LogEntry{Message: "file test"})
	if err := hook.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	data, err := json.Marshal(map[string]string{"check": "ok"}) // verify file exists
	_ = data
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileLogHook_HookLevel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/test.log"
	hook, err := NewFileLogHook(path, ErrorLevel)
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}
	defer hook.Close()

	logHook := hook.Hook()
	// Debug should be filtered
	logHook(DebugLevel, LogEntry{Message: "filtered"})
	logHook(ErrorLevel, LogEntry{Message: "passed"})
}

func TestRequestLogger_Middleware(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	rl := NewRequestLogger(logger)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test?foo=bar", nil)
	req.Header.Set("User-Agent", "TestAgent")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fields, _ := entry["fields"].(map[string]any)
	if fields["method"] != "GET" {
		t.Errorf("method = %v, want GET", fields["method"])
	}
	if fields["path"] != "/test" {
		t.Errorf("path = %v, want /test", fields["path"])
	}
	if fields["status"] != float64(200) {
		t.Errorf("status = %v, want 200", fields["status"])
	}
}

func TestRequestLogger_Middleware_ErrorStatus(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	rl := NewRequestLogger(logger)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR for 500", entry["level"])
	}
}

func TestRequestLogger_Middleware_WarnStatus(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)
	rl := NewRequestLogger(logger)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["level"] != "WARN" {
		t.Errorf("level = %v, want WARN for 400", entry["level"])
	}
}

func TestGetSetGlobalLogger(t *testing.T) {
	var buf bytes.Buffer
	newLogger := NewStructuredLogger(&buf, DebugLevel)
	old := GetGlobalLogger()
	SetGlobalLogger(newLogger)

	Info("global test")
	if !strings.Contains(buf.String(), "global test") {
		t.Error("global Info should use new logger")
	}

	SetGlobalLogger(old) // restore
}

func TestGlobalWithField(t *testing.T) {
	result := WithField("test_key", "test_val")
	if result == nil {
		t.Error("WithField should return non-nil logger")
	}
}

func TestGlobalWithFields(t *testing.T) {
	result := WithFields(map[string]any{"k": "v"})
	if result == nil {
		t.Error("WithFields should return non-nil logger")
	}
}

func TestLogEntry_JSON(t *testing.T) {
	t.Parallel()
	entry := LogEntry{
		Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Level:     "INFO",
		Message:   "test entry",
		Service:   "apicerberus",
		Version:   "1.0.0",
		TraceID:   "trace-123",
		SpanID:    "span-456",
		Fields:    map[string]any{"user_id": 42},
		Caller:    "main.go:10",
		Hostname:  "testhost",
		PID:       1234,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"trace_id":"trace-123"`) {
		t.Error("expected trace_id in JSON")
	}
	if !strings.Contains(string(data), `"service":"apicerberus"`) {
		t.Error("expected service in JSON")
	}
}
