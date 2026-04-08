package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestNewKafkaWriter_Disabled(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled: false,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Errorf("NewKafkaWriter() error = %v", err)
	}
	if writer != nil {
		t.Error("Expected nil writer when disabled")
	}
}

func TestNewKafkaWriter_NoBrokers(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled: true,
		Brokers: []string{},
	}

	_, err := NewKafkaWriter(cfg)
	if err == nil {
		t.Error("Expected error when no brokers provided")
	}
}

func TestNewKafkaWriter_DefaultTopic(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true, // Don't block on connection
		Workers:      1,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	if writer.topic != "apicerberus.audit" {
		t.Errorf("Expected default topic 'apicerberus.audit', got %s", writer.topic)
	}
}

func TestNewKafkaWriter_CustomTopic(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		Topic:        "custom.audit.topic",
		AsyncConnect: true,
		Workers:      1,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	if writer.topic != "custom.audit.topic" {
		t.Errorf("Expected topic 'custom.audit.topic', got %s", writer.topic)
	}
}

func TestNewKafkaWriter_DefaultBatchSize(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true,
		Workers:      1,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	if writer.batchSize != 100 {
		t.Errorf("Expected default batch size 100, got %d", writer.batchSize)
	}
}

func TestNewKafkaWriter_CustomBatchSize(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		BatchSize:    500,
		AsyncConnect: true,
		Workers:      1,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	if writer.batchSize != 500 {
		t.Errorf("Expected batch size 500, got %d", writer.batchSize)
	}
}

func TestKafkaWriter_Enabled(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true,
		Workers:      1,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	if !writer.Enabled() {
		t.Error("Expected writer to be enabled")
	}
}

func TestKafkaWriter_Write(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true,
		Workers:      1,
		BufferSize:   100,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	entry := store.AuditEntry{
		RequestID:  "test-req-1",
		UserID:     "user-123",
		Method:     "GET",
		Path:       "/api/test",
		StatusCode: 200,
		CreatedAt:  time.Now().UTC(),
	}

	// Write should not block even if not connected
	err = writer.Write(entry)
	if err != nil {
		t.Logf("Write error (expected if not connected): %v", err)
	}
}

func TestKafkaWriter_WriteBatch(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true,
		Workers:      1,
		BufferSize:   100,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	entries := []store.AuditEntry{
		{RequestID: "req-1", Method: "GET", Path: "/api/1"},
		{RequestID: "req-2", Method: "POST", Path: "/api/2"},
		{RequestID: "req-3", Method: "DELETE", Path: "/api/3"},
	}

	err = writer.WriteBatch(entries)
	if err != nil {
		t.Logf("WriteBatch error (expected if not connected): %v", err)
	}
}

func TestKafkaWriter_Stats(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true,
		Workers:      1,
		BufferSize:   100,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	stats := writer.Stats()

	// Initial stats should be zero
	if stats.Sent != 0 {
		t.Errorf("Expected sent count 0, got %d", stats.Sent)
	}
	if stats.Failed != 0 {
		t.Errorf("Expected failed count 0, got %d", stats.Failed)
	}
}

func TestKafkaWriter_IsHealthy_NotConnected(t *testing.T) {
	cfg := config.KafkaConfig{
		Enabled:      true,
		Brokers:      []string{"localhost:9092"},
		AsyncConnect: true,
		Workers:      1,
	}

	writer, err := NewKafkaWriter(cfg)
	if err != nil {
		t.Fatalf("NewKafkaWriter() error = %v", err)
	}
	defer writer.Close()

	// Should not be healthy if not connected
	if writer.IsHealthy() {
		t.Error("Expected writer to not be healthy when not connected")
	}
}

func TestKafkaMessage_Marshal(t *testing.T) {
	entry := store.AuditEntry{
		RequestID:  "req-123",
		UserID:     "user-456",
		Method:     "GET",
		Path:       "/api/users",
		StatusCode: 200,
		CreatedAt:  time.Now().UTC(),
	}

	msg := KafkaMessage{
		Version:    "1.0",
		Type:       "audit_log",
		Timestamp:  time.Now().UTC(),
		GatewayID:  "gw-001",
		AuditEntry: &entry,
		Metadata: map[string]interface{}{
			"region": "us-east-1",
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected non-empty JSON data")
	}

	// Verify it can be unmarshaled
	var decoded KafkaMessage
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if decoded.Version != msg.Version {
		t.Errorf("Expected version %s, got %s", msg.Version, decoded.Version)
	}
	if decoded.GatewayID != msg.GatewayID {
		t.Errorf("Expected gateway ID %s, got %s", msg.GatewayID, decoded.GatewayID)
	}
}

func (m *KafkaMessage) MarshalJSON() ([]byte, error) {
	type Alias KafkaMessage
	return json.Marshal(&struct {
		Timestamp string `json:"timestamp"`
		*Alias
	}{
		Timestamp: m.Timestamp.Format(time.RFC3339Nano),
		Alias:     (*Alias)(m),
	})
}

func (m *KafkaMessage) UnmarshalJSON(data []byte) error {
	type Alias KafkaMessage
	aux := &struct {
		Timestamp string `json:"timestamp"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	m.Timestamp, err = time.Parse(time.RFC3339Nano, aux.Timestamp)
	return err
}
