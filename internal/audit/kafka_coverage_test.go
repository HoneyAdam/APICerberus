package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestNewKafkaWriter_SyncConnectFail(t *testing.T) {
	_, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:1"},
		Topic:        "test",
		Workers:      1,
		AsyncConnect: false,
		BufferSize:   100,
		DialTimeout:  100 * time.Millisecond,
	})
	if err == nil {
		t.Error("expected error when sync connect fails")
	}
}

func TestNewKafkaWriter_DefaultBatchSize(t *testing.T) {
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test",
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   100,
		BatchSize:    0,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	if kw.batchSize != 100 {
		t.Errorf("batchSize = %d, want 100", kw.batchSize)
	}
}

func TestNewKafkaWriter_DefaultFlushInterval(t *testing.T) {
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:       true,
		Brokers:       []string{"127.0.0.1:9999"},
		Topic:         "test",
		Workers:       1,
		AsyncConnect:  true,
		BufferSize:    100,
		FlushInterval: 0,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	if kw.flushInterval != time.Second {
		t.Errorf("flushInterval = %v, want 1s", kw.flushInterval)
	}
}

func TestKafkaWriter_BlockOnFull(t *testing.T) {
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test",
		Workers:      0,
		AsyncConnect: true,
		BufferSize:   2,
		BlockOnFull:  true,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	entry := store.AuditEntry{ID: "1", RequestID: "r1"}
	kw.Write(entry)
	kw.Write(entry)

	done := make(chan error, 1)
	go func() {
		done <- kw.Write(entry)
	}()

	// Drain a message to unblock the writer
	select {
	case <-kw.messages:
	case err := <-done:
		if err != nil {
			t.Errorf("blocked write failed: %v", err)
		}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("blocked write: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("BlockOnFull write timed out")
	}
}

func TestKafkaWriter_WriteBatch_Disabled(t *testing.T) {
	var kw *KafkaWriter
	if err := kw.WriteBatch([]store.AuditEntry{{ID: "1"}}); err != nil {
		t.Errorf("nil WriteBatch should not error: %v", err)
	}
}

func TestKafkaWriter_Write_NilRequestID(t *testing.T) {
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

	entry := store.AuditEntry{ID: "1", RequestID: "", UserID: "user-key"}
	if err := kw.Write(entry); err != nil {
		t.Fatalf("write: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	_ = kw.Stats()
}

func TestKafkaStats_Fields(t *testing.T) {
	t.Parallel()
	s := KafkaStats{Sent: 10, Failed: 2, Dropped: 1, Queued: 5}
	if s.Sent != 10 || s.Failed != 2 || s.Dropped != 1 || s.Queued != 5 {
		t.Errorf("unexpected stats: %+v", s)
	}
}

func TestKafkaMessage_NilAuditEntry(t *testing.T) {
	t.Parallel()
	msg := KafkaMessage{
		Version:   "1.0",
		Type:      "audit_log",
		Timestamp: time.Now().UTC(),
		GatewayID: "gw-1",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["gateway_id"] != "gw-1" {
		t.Errorf("gateway_id = %v, want gw-1", parsed["gateway_id"])
	}
}

func TestKafkaWriter_DefaultBufferSize(t *testing.T) {
	kw, err := NewKafkaWriter(config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"127.0.0.1:9999"},
		Topic:        "test",
		Workers:      1,
		AsyncConnect: true,
		BufferSize:   0,
	})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	defer kw.Close()

	if kw.messages == nil {
		t.Error("messages channel should not be nil")
	}
}
