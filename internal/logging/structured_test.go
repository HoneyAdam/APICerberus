package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{FatalLevel, "FATAL"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewStructuredLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	if logger == nil {
		t.Fatal("NewStructuredLogger() returned nil")
	}
	if logger.level != InfoLevel {
		t.Errorf("level = %v, want InfoLevel", logger.level)
	}
	if logger.output != &buf {
		t.Error("output not set correctly")
	}
	if logger.service != "apicerberus" {
		t.Errorf("service = %v, want apicerberus", logger.service)
	}
	if logger.version != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", logger.version)
	}
	if logger.hostname == "" {
		t.Error("hostname should not be empty")
	}
	if logger.pid == 0 {
		t.Error("pid should not be 0")
	}
}

func TestNewStructuredLogger_NilOutput(t *testing.T) {
	logger := NewStructuredLogger(nil, InfoLevel)

	if logger.output != os.Stdout {
		t.Error("output should default to os.Stdout when nil")
	}
}

func TestStructuredLogger_WithField(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	newLogger := logger.WithField("key", "value")

	if newLogger.fields["key"] != "value" {
		t.Errorf("fields[key] = %v, want value", newLogger.fields["key"])
	}

	// Original logger should not be modified
	if len(logger.fields) > 0 {
		t.Error("Original logger should not be modified")
	}
}

func TestStructuredLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	newLogger := logger.WithFields(map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})

	if newLogger.fields["key1"] != "value1" {
		t.Errorf("fields[key1] = %v, want value1", newLogger.fields["key1"])
	}
	if newLogger.fields["key2"] != "value2" {
		t.Errorf("fields[key2] = %v, want value2", newLogger.fields["key2"])
	}
}

func TestStructuredLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	ctx := context.WithValue(context.Background(), "trace_id", "abc123")
	ctx = context.WithValue(ctx, "span_id", "span456")

	newLogger := logger.WithContext(ctx)

	if newLogger.fields["trace_id"] != "abc123" {
		t.Errorf("fields[trace_id] = %v, want abc123", newLogger.fields["trace_id"])
	}
	if newLogger.fields["span_id"] != "span456" {
		t.Errorf("fields[span_id] = %v, want span456", newLogger.fields["span_id"])
	}
}

func TestStructuredLogger_LogLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	t.Run("Debug below threshold", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message")
		if buf.Len() > 0 {
			t.Error("Debug should not log when level is Info")
		}
	})

	t.Run("Info at threshold", func(t *testing.T) {
		buf.Reset()
		logger.Info("info message")
		if buf.Len() == 0 {
			t.Error("Info should log when level is Info")
		}
	})

	t.Run("Warn above threshold", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warn message")
		if buf.Len() == 0 {
			t.Error("Warn should log when level is Info")
		}
	})

	t.Run("Error above threshold", func(t *testing.T) {
		buf.Reset()
		logger.Error("error message")
		if buf.Len() == 0 {
			t.Error("Error should log when level is Info")
		}
	})
}

func TestStructuredLogger_LogOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, DebugLevel)

	logger.WithField("user_id", "123").Info("User logged in")

	output := buf.String()
	if !strings.Contains(output, "User logged in") {
		t.Error("Log message not found in output")
	}
	if !strings.Contains(output, "user_id") {
		t.Error("Field not found in output")
	}
	if !strings.Contains(output, "123") {
		t.Error("Field value not found in output")
	}

	// Verify JSON structure
	var entry LogEntry
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Errorf("Log output is not valid JSON: %v", err)
	}
	if entry.Level != "INFO" {
		t.Errorf("Level = %v, want INFO", entry.Level)
	}
	if entry.Message != "User logged in" {
		t.Errorf("Message = %v, want 'User logged in'", entry.Message)
	}
}

func TestStructuredLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	logger.SetLevel(DebugLevel)
	if logger.level != DebugLevel {
		t.Errorf("level = %v, want DebugLevel", logger.level)
	}

	buf.Reset()
	logger.Debug("debug message")
	if buf.Len() == 0 {
		t.Error("Debug should log after level is changed to Debug")
	}
}

func TestStructuredLogger_SetService(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	logger.SetService("my-service")
	if logger.service != "my-service" {
		t.Errorf("service = %v, want my-service", logger.service)
	}
}

func TestStructuredLogger_SetVersion(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	logger.SetVersion("2.0.0")
	if logger.version != "2.0.0" {
		t.Errorf("version = %v, want 2.0.0", logger.version)
	}
}

func TestStructuredLogger_AddHook(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	hookCalled := false
	hook := func(level LogLevel, entry LogEntry) {
		hookCalled = true
	}

	logger.AddHook(hook)
	logger.Info("test message")

	if !hookCalled {
		t.Error("Hook should be called when logging")
	}
}

func TestLogEntry_JSON(t *testing.T) {
	entry := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Level:     "INFO",
		Message:   "test",
		Service:   "test-service",
		Version:   "1.0.0",
		TraceID:   "abc",
		SpanID:    "def",
		Fields:    map[string]interface{}{"key": "value"},
		Caller:    "test.go:10",
		Hostname:  "localhost",
		PID:       123,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Errorf("Failed to marshal LogEntry: %v", err)
	}

	var decoded LogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal LogEntry: %v", err)
	}

	if decoded.Message != entry.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, entry.Message)
	}
	if decoded.Service != entry.Service {
		t.Errorf("Service = %v, want %v", decoded.Service, entry.Service)
	}
}

func TestNewFileLogHook(t *testing.T) {
	tmpDir := t.TempDir()
	hookPath := tmpDir + "/test.log"

	hook, err := NewFileLogHook(hookPath, InfoLevel)
	if err != nil {
		t.Errorf("NewFileLogHook() error = %v", err)
	}
	if hook == nil {
		t.Fatal("NewFileLogHook() returned nil")
	}
	defer hook.Close()

	// Verify file was created
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Error("Log file was not created")
	}
}

func TestFileLogHook_Hook(t *testing.T) {
	tmpDir := t.TempDir()
	hookPath := tmpDir + "/test.log"

	hook, err := NewFileLogHook(hookPath, InfoLevel)
	if err != nil {
		t.Fatalf("NewFileLogHook() error = %v", err)
	}
	defer hook.Close()

	logHook := hook.Hook()
	logHook(InfoLevel, LogEntry{
		Level:   "INFO",
		Message: "test message",
	})

	// Give time for write
	time.Sleep(10 * time.Millisecond)

	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Errorf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Error("Log message not written to file")
	}
}

func TestFilterLogHook(t *testing.T) {
	filter := FilterLogHook(WarnLevel)

	// Should filter out below Warn
	filter(InfoLevel, LogEntry{Level: "INFO", Message: "info"})

	// Should not filter Warn and above
	filter(WarnLevel, LogEntry{Level: "WARN", Message: "warn"})
	filter(ErrorLevel, LogEntry{Level: "ERROR", Message: "error"})
}

func TestNewRequestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)

	rl := NewRequestLogger(logger)
	if rl == nil {
		t.Fatal("NewRequestLogger() returned nil")
	}
	if rl.logger != logger {
		t.Error("logger not set correctly")
	}
}

func TestRequestLogger_Middleware(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)
	rl := NewRequestLogger(logger)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %v, want %v", rec.Code, http.StatusOK)
	}

	output := buf.String()
	if !strings.Contains(output, "HTTP request") {
		t.Error("Request log not found")
	}
	if !strings.Contains(output, "GET") {
		t.Error("Method not logged")
	}
	if !strings.Contains(output, "/test") {
		t.Error("Path not logged")
	}
}

func TestRequestLogger_Middleware_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, InfoLevel)
	rl := NewRequestLogger(logger)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "error") {
		t.Error("Error level log not found")
	}
}

func TestGetClientIP(t *testing.T) {
	// Configure trusted proxies for XFF tests
	netutil.SetTrustedProxies([]string{"127.0.0.0/8"})
	defer netutil.SetTrustedProxies(nil)

	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{
			name:    "X-Forwarded-For",
			headers: map[string]string{"X-Forwarded-For": "10.0.0.1, 127.0.0.1"},
			remote:  "127.0.0.1:1234",
			want:    "10.0.0.1",
		},
		{
			name:    "X-Real-Ip",
			headers: map[string]string{"X-Real-Ip": "10.0.0.3"},
			remote:  "127.0.0.1:1234",
			want:    "10.0.0.3",
		},
		{
			name:    "RemoteAddr",
			headers: map[string]string{},
			remote:  "192.168.1.1:1234",
			want:    "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = tt.remote

			got := getClientIP(req)
			if got != tt.want {
				t.Errorf("getClientIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlobalLogger(t *testing.T) {
	// Save original
	original := globalLogger
	defer func() { globalLogger = original }()

	// Test GetGlobalLogger
	logger := GetGlobalLogger()
	if logger == nil {
		t.Error("GetGlobalLogger() returned nil")
	}

	// Test SetGlobalLogger
	var buf bytes.Buffer
	newLogger := NewStructuredLogger(&buf, InfoLevel)
	SetGlobalLogger(newLogger)

	if globalLogger != newLogger {
		t.Error("SetGlobalLogger did not set global logger")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	// Save original
	original := globalLogger
	defer func() { globalLogger = original }()

	var buf bytes.Buffer
	SetGlobalLogger(NewStructuredLogger(&buf, DebugLevel))

	Debug("debug: %s", "test")
	if !strings.Contains(buf.String(), "debug: test") {
		t.Error("Debug convenience function failed")
	}

	buf.Reset()
	Info("info: %s", "test")
	if !strings.Contains(buf.String(), "info: test") {
		t.Error("Info convenience function failed")
	}

	buf.Reset()
	Warn("warn: %s", "test")
	if !strings.Contains(buf.String(), "warn: test") {
		t.Error("Warn convenience function failed")
	}

	buf.Reset()
	Error("error: %s", "test")
	if !strings.Contains(buf.String(), "error: test") {
		t.Error("Error convenience function failed")
	}
}

func TestWithField(t *testing.T) {
	// Save original
	original := globalLogger
	defer func() { globalLogger = original }()

	var buf bytes.Buffer
	SetGlobalLogger(NewStructuredLogger(&buf, InfoLevel))

	logger := WithField("key", "value")
	if logger.fields["key"] != "value" {
		t.Error("WithField did not add field")
	}
}

func TestWithFields(t *testing.T) {
	// Save original
	original := globalLogger
	defer func() { globalLogger = original }()

	var buf bytes.Buffer
	SetGlobalLogger(NewStructuredLogger(&buf, InfoLevel))

	logger := WithFields(map[string]interface{}{"key": "value"})
	if logger.fields["key"] != "value" {
		t.Error("WithFields did not add fields")
	}
}
