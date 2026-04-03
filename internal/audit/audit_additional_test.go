package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// Test ResponseCaptureWriter Flush
func TestResponseCaptureWriter_Flush(t *testing.T) {
	t.Parallel()

	// Create a ResponseCaptureWriter with an httptest.ResponseRecorder
	inner := httptest.NewRecorder()
	capture := NewResponseCaptureWriter(inner, 1024)

	// Flush should not panic
	capture.Flush()
}

// Test ResponseCaptureWriter Hijack
func TestResponseCaptureWriter_Hijack(t *testing.T) {
	t.Parallel()

	// Create a ResponseCaptureWriter with an httptest.ResponseRecorder
	inner := httptest.NewRecorder()
	capture := NewResponseCaptureWriter(inner, 1024)

	// Hijack should return ErrNotSupported since httptest.ResponseRecorder doesn't support Hijack
	_, _, err := capture.Hijack()
	if err != http.ErrNotSupported {
		t.Errorf("Hijack() error = %v, want %v", err, http.ErrNotSupported)
	}
}

// Test ResponseCaptureWriter Push
func TestResponseCaptureWriter_Push(t *testing.T) {
	t.Parallel()

	// Create a ResponseCaptureWriter with an httptest.ResponseRecorder
	inner := httptest.NewRecorder()
	capture := NewResponseCaptureWriter(inner, 1024)

	// Push should return ErrNotSupported since httptest.ResponseRecorder doesn't support Push
	err := capture.Push("/test", nil)
	if err != http.ErrNotSupported {
		t.Errorf("Push() error = %v, want %v", err, http.ErrNotSupported)
	}
}

// Test Logger MaxRequestBodyBytes
func TestLogger_MaxRequestBodyBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		logger   *Logger
		expected int64
	}{
		{
			name:     "nil logger",
			logger:   nil,
			expected: 0,
		},
		{
			name: "with config",
			logger: &Logger{
				cfg: config.AuditConfig{
					MaxRequestBodyBytes: 1024,
				},
			},
			expected: 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.logger.MaxRequestBodyBytes()
			if result != tt.expected {
				t.Errorf("MaxRequestBodyBytes() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test Logger MaxResponseBodyBytes
func TestLogger_MaxResponseBodyBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		logger   *Logger
		expected int64
	}{
		{
			name:     "nil logger",
			logger:   nil,
			expected: 0,
		},
		{
			name: "with config",
			logger: &Logger{
				cfg: config.AuditConfig{
					MaxResponseBodyBytes: 2048,
				},
			},
			expected: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.logger.MaxResponseBodyBytes()
			if result != tt.expected {
				t.Errorf("MaxResponseBodyBytes() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test Logger Dropped
func TestLogger_Dropped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		logger   *Logger
		expected int64
	}{
		{
			name:     "nil logger",
			logger:   nil,
			expected: 0,
		},
		{
			name:     "empty logger",
			logger:   &Logger{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.logger.Dropped()
			if result != tt.expected {
				t.Errorf("Dropped() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test Logger Start with nil repo
func TestLogger_Start_Disabled(t *testing.T) {
	t.Parallel()

	// Logger with nil repo should not panic on Start
	logger := &Logger{
		cfg: config.AuditConfig{
			Enabled:       true,
			FlushInterval: time.Second,
			BatchSize:     10,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not panic
	logger.Start(ctx)
}

// Test Logger Start when already started
func TestLogger_Start_AlreadyStarted(t *testing.T) {
	t.Parallel()

	st := openAuditTestStoreForAdditional(t)
	defer st.Close()

	logger := NewLogger(st.Audits(), config.AuditConfig{
		Enabled:       true,
		FlushInterval: time.Second,
		BatchSize:     10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start once
	go logger.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Try to start again - should not panic and should return immediately
	logger.Start(ctx)
}

func openAuditTestStoreForAdditional(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(&config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	})
	if err != nil {
		t.Fatalf("open store error: %v", err)
	}
	return st
}
