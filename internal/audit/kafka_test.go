package audit

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestNewKafkaWriter_Disabled(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kw != nil {
		t.Error("expected nil when disabled")
	}
}

func TestNewKafkaWriter_NoBrokers(t *testing.T) {
	t.Parallel()
	_, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:  true,
		Brokers:  []string{},
		Topic:    "test",
		Workers:  1,
		AsyncConnect: true,
	})
	if err == nil {
		t.Error("expected error when no brokers configured")
	}
}

func TestNewKafkaWriter_AsyncConnect(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test-topic",
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
		BatchSize:    10,
		FlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error with async connect: %v", err)
	}
	if kw == nil {
		t.Fatal("expected non-nil writer")
	}
	defer kw.Close()

	if kw.topic != "test-topic" {
		t.Errorf("topic = %q, want test-topic", kw.topic)
	}
}

func TestKafkaWriter_DefaultTopic(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "", // should default
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer kw.Close()

	if kw.topic != "apicerberus.audit" {
		t.Errorf("default topic = %q, want apicerberus.audit", kw.topic)
	}
}

func TestKafkaWriter_Write_Single(t *testing.T) {
	t.Parallel()
	// Start a fake TCP server to capture messages
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var receivedMu sync.Mutex
	var received []map[string]any

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						lines := bytes.Split(buf[:n], []byte{'\n'})
						for _, line := range lines {
							if len(line) == 0 {
								continue
							}
							var m map[string]any
							if err := json.Unmarshal(line, &m); err == nil {
								receivedMu.Lock()
								received = append(received, m)
								receivedMu.Unlock()
							}
						}
					}
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:       true,
		Brokers:       []string{ln.Addr().String()},
		Topic:         "audit-test",
		Workers:       1,
		BufferSize:    100,
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
		WriteTimeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	entry := store.AuditEntry{
		ID:         "test-1",
		RequestID:  "req-1",
		Method:     http.MethodGet,
		Path:       "/api/test",
		StatusCode: 200,
		UserID:     "user-1",
	}

	if err := kw.Write(entry); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	receivedMu.Lock()
	count := len(received)
	receivedMu.Unlock()

	if count == 0 {
		t.Error("expected at least one message")
	}
}

func TestKafkaWriter_WriteBatch(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test",
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	entries := []store.AuditEntry{
		{ID: "1", RequestID: "r1", Method: "GET", Path: "/a"},
		{ID: "2", RequestID: "r2", Method: "POST", Path: "/b"},
		{ID: "3", RequestID: "r3", Method: "PUT", Path: "/c"},
	}

	if err := kw.WriteBatch(entries); err != nil {
		t.Fatalf("write batch: %v", err)
	}

	// Verify batch write completed without error
	// Workers may drain messages concurrently, so exact queue count is nondeterministic.
}

func TestKafkaWriter_WriteBatch_Empty(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test",
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	if err := kw.WriteBatch(nil); err != nil {
		t.Errorf("nil batch should not error: %v", err)
	}
	if err := kw.WriteBatch([]store.AuditEntry{}); err != nil {
		t.Errorf("empty batch should not error: %v", err)
	}
}

func TestKafkaWriter_Stats(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test",
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	stats := kw.Stats()
	if stats.Sent != 0 || stats.Failed != 0 || stats.Dropped != 0 {
		t.Errorf("initial stats should be zero: %+v", stats)
	}
}

func TestKafkaWriter_Enabled(t *testing.T) {
	t.Parallel()
	var kw *KafkaWriter
	if kw.Enabled() {
		t.Error("nil writer should not be enabled")
	}

	kw2, _ := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
	})
	if !kw2.Enabled() {
		t.Error("active writer should be enabled")
	}
	kw2.Close()
}

func TestKafkaWriter_Close_Nil(t *testing.T) {
	t.Parallel()
	var kw *KafkaWriter
	if err := kw.Close(); err != nil {
		t.Errorf("nil Close should not error: %v", err)
	}
}

func TestKafkaWriter_Write_Disabled(t *testing.T) {
	t.Parallel()
	var kw *KafkaWriter
	if err := kw.Write(store.AuditEntry{}); err != nil {
		t.Errorf("nil Write should not error: %v", err)
	}
}

func TestKafkaMessage_Format(t *testing.T) {
	t.Parallel()
	entry := store.AuditEntry{
		ID:         "log-123",
		RequestID:  "req-456",
		Method:     http.MethodPost,
		Path:       "/api/users",
		StatusCode: 201,
		UserID:     "user-789",
	}

	msg := KafkaMessage{
		Version:    "1.0",
		Type:       "audit_log",
		Timestamp:  time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		GatewayID:  "gw-1",
		AuditEntry: &entry,
		Metadata: map[string]any{
			"region": "us-east-1",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["version"] != "1.0" {
		t.Errorf("version = %v, want 1.0", parsed["version"])
	}
	if parsed["type"] != "audit_log" {
		t.Errorf("type = %v, want audit_log", parsed["type"])
	}
	if parsed["gateway_id"] != "gw-1" {
		t.Errorf("gateway_id = %v, want gw-1", parsed["gateway_id"])
	}
	auditEntry, ok := parsed["audit_entry"].(map[string]any)
	if !ok {
		t.Fatal("audit_entry should be a map")
	}
	if auditEntry["method"] != "POST" {
		t.Errorf("method = %v, want POST", auditEntry["method"])
	}
	meta, ok := parsed["metadata"].(map[string]any)
	if !ok {
		t.Fatal("metadata should be a map")
	}
	if meta["region"] != "us-east-1" {
		t.Errorf("region = %v, want us-east-1", meta["region"])
	}
}

func TestKafkaWriter_QueueFull_Drops(t *testing.T) {
	t.Parallel()
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:       true,
		Brokers:       []string{"127.0.0.1:9999"},
		Topic:         "test",
		Workers:       0, // no workers - messages won't be consumed
		AsyncConnect:  true,
		BufferSize:    2,
		BlockOnFull:   false,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	// Fill the queue
	entry := store.AuditEntry{ID: "1", RequestID: "r1"}
	if err := kw.Write(entry); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := kw.Write(entry); err != nil {
		t.Fatalf("second write: %v", err)
	}

	// Third write should be dropped (queue full, BlockOnFull=false)
	err = kw.Write(entry)
	if err == nil {
		t.Error("expected error when queue full")
	}

	stats := kw.Stats()
	if stats.Dropped != 1 {
		t.Errorf("dropped = %d, want 1", stats.Dropped)
	}
}

