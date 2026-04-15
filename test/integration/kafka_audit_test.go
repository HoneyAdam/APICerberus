package integration

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/audit"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// startFakeBroker starts a TCP listener that collects newline-delimited JSON lines.
// Returns the listener address, a channel of received raw lines, and a stop function.
func startFakeBroker(t *testing.T) (addr string, lines <-chan string, stop func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ch := make(chan string, 256)
	var wg sync.WaitGroup

	done := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				scanner := bufio.NewScanner(c)
				for scanner.Scan() {
					select {
					case ch <- scanner.Text():
					case <-done:
						return
					}
				}
			}(conn)
		}
	}()

	return ln.Addr().String(), ch, func() {
		ln.Close()
		close(done)
		wg.Wait()
	}
}

func TestKafkaWriterWritesToTCPBroker(t *testing.T) {
	addr, lines, stopBroker := startFakeBroker(t)
	defer stopBroker()

	kw, err := audit.NewKafkaWriter(config.KafkaConfig{
		Enabled:       true,
		Brokers:       []string{addr},
		Topic:         "test-audit",
		BatchSize:     1,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    16,
		DialTimeout:   2 * time.Second,
		WriteTimeout:  5 * time.Second,
		Workers:       1,
		GatewayID:     "test-gw",
	})
	if err != nil {
		t.Fatalf("NewKafkaWriter: %v", err)
	}
	defer kw.Close()

	// Give the worker goroutine time to start
	time.Sleep(200 * time.Millisecond)

	entry := store.AuditEntry{
		ID:        "audit-001",
		RequestID: "req-001",
		Method:    "GET",
		Path:      "/api/v1/test",
		Host:      "example.com",
		UserID:    "user-1",
	}

	if err := kw.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Wait for the message to arrive
	select {
	case raw := <-lines:
		var msg map[string]any
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("unmarshal message: %v", err)
		}
		if msg["topic"] != "test-audit" {
			t.Errorf("expected topic test-audit, got %v", msg["topic"])
		}
		// The "value" field should contain the KafkaMessage JSON
		valueStr, ok := msg["value"].(string)
		if !ok {
			t.Fatal("value field is not a string")
		}
		var kafkaMsg map[string]any
		if err := json.Unmarshal([]byte(valueStr), &kafkaMsg); err != nil {
			t.Fatalf("unmarshal value: %v", err)
		}
		if kafkaMsg["type"] != "audit_log" {
			t.Errorf("expected type audit_log, got %v", kafkaMsg["type"])
		}
		if kafkaMsg["gateway_id"] != "test-gw" {
			t.Errorf("expected gateway_id test-gw, got %v", kafkaMsg["gateway_id"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Kafka message")
	}
}

func TestKafkaWriterBatchWrite(t *testing.T) {
	addr, lines, stopBroker := startFakeBroker(t)
	defer stopBroker()

	kw, err := audit.NewKafkaWriter(config.KafkaConfig{
		Enabled:       true,
		Brokers:       []string{addr},
		Topic:         "test-batch",
		BatchSize:     10,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    64,
		DialTimeout:   2 * time.Second,
		WriteTimeout:  5 * time.Second,
		Workers:       1,
	})
	if err != nil {
		t.Fatalf("NewKafkaWriter: %v", err)
	}
	defer kw.Close()

	// Give the worker goroutine time to start
	time.Sleep(200 * time.Millisecond)

	entries := []store.AuditEntry{
		{ID: "batch-1", Method: "GET", Path: "/a"},
		{ID: "batch-2", Method: "POST", Path: "/b"},
		{ID: "batch-3", Method: "DELETE", Path: "/c"},
	}

	if err := kw.WriteBatch(entries); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	// Collect 3 messages
	received := 0
	timeout := time.After(5 * time.Second)
	for received < 3 {
		select {
		case <-lines:
			received++
		case <-timeout:
			t.Fatalf("timed out: received %d/3 messages", received)
		}
	}

	if received != 3 {
		t.Errorf("expected 3 messages, got %d", received)
	}
}

func TestKafkaWriterDisabled(t *testing.T) {
	kw, err := audit.NewKafkaWriter(config.KafkaConfig{Enabled: false})
	if err != nil {
		t.Fatalf("NewKafkaWriter: %v", err)
	}
	if kw != nil {
		t.Error("expected nil writer when disabled")
	}
}

func TestKafkaWriterNoBrokersError(t *testing.T) {
	_, err := audit.NewKafkaWriter(config.KafkaConfig{
		Enabled: true,
		Brokers: []string{},
	})
	if err == nil {
		t.Error("expected error when no brokers configured")
	}
}

func TestKafkaWriterConnectionFailure(t *testing.T) {
	_, err := audit.NewKafkaWriter(config.KafkaConfig{
		Enabled:     true,
		Brokers:     []string{"127.0.0.1:54321"}, // closed port - should fail
		DialTimeout: 500 * time.Millisecond,
	})
	if err == nil {
		t.Error("expected error when broker is unreachable")
	}
}
