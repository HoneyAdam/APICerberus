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

func TestLoggerLogPersistsSynchronouslyWhenNotStarted(t *testing.T) {
	t.Parallel()

	st := openAuditTestStore(t)
	defer st.Close()

	logger := NewLogger(st.Audits(), config.AuditConfig{
		Enabled:              true,
		BufferSize:           10,
		BatchSize:            2,
		FlushInterval:        time.Second,
		MaxRequestBodyBytes:  1024,
		MaxResponseBodyBytes: 1024,
		MaskHeaders:          []string{"Authorization"},
		MaskBodyFields:       []string{"password"},
		MaskReplacement:      "***",
	}, nil)
	if logger == nil {
		t.Fatalf("expected logger")
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer token")

	rr := httptest.NewRecorder()
	capture := NewResponseCaptureWriter(rr, 1024)
	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusCreated)
	_, _ = capture.Write([]byte(`{"password":"upstream-secret"}`))

	logger.Log(LogInput{
		Request:        req,
		ResponseWriter: capture,
		RequestBody:    []byte(`{"password":"client-secret"}`),
		StartedAt:      time.Now().Add(-10 * time.Millisecond),
	})

	listed, err := st.Audits().List(store.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if listed.Total != 1 || len(listed.Entries) != 1 {
		t.Fatalf("expected one audit row, got total=%d len=%d", listed.Total, len(listed.Entries))
	}
	entry := listed.Entries[0]
	if entry.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status code: %d", entry.StatusCode)
	}
	if entry.RequestBody != `{"password":"***"}` {
		t.Fatalf("request body should be masked, got %s", entry.RequestBody)
	}
	if entry.ResponseBody != `{"password":"***"}` {
		t.Fatalf("response body should be masked, got %s", entry.ResponseBody)
	}
	if entry.RequestHeaders["Authorization"] != "***" {
		t.Fatalf("authorization header should be masked: %#v", entry.RequestHeaders["Authorization"])
	}
}

func TestLoggerStartFlushesBufferedEntries(t *testing.T) {
	t.Parallel()

	st := openAuditTestStore(t)
	defer st.Close()

	logger := NewLogger(st.Audits(), config.AuditConfig{
		Enabled:              true,
		BufferSize:           4,
		BatchSize:            1,
		FlushInterval:        10 * time.Millisecond,
		MaxRequestBodyBytes:  256,
		MaxResponseBodyBytes: 256,
	}, nil)
	if logger == nil {
		t.Fatalf("expected logger")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logger.Start(ctx)

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/ping", nil)
	rr := httptest.NewRecorder()
	capture := NewResponseCaptureWriter(rr, 256)
	capture.WriteHeader(http.StatusOK)
	_, _ = capture.Write([]byte(`{"ok":true}`))

	logger.Log(LogInput{
		Request:        req,
		ResponseWriter: capture,
		RequestBody:    []byte{},
		StartedAt:      time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		result, err := st.Audits().List(store.AuditListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("list logs: %v", err)
		}
		if result.Total >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected buffered audit log to flush")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func openAuditTestStore(t *testing.T) *store.Store {
	t.Helper()
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}
