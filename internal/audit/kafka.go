package audit

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// KafkaWriter handles streaming audit logs to Kafka.
type KafkaWriter struct {
	config        config.KafkaConfig
	brokers       []string
	topic         string
	batchSize     int
	flushInterval time.Duration

	// Connection state
	conn      net.Conn
	mu        sync.RWMutex
	connected bool

	// Message queue
	messages chan *kafkaMessage
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// Stats
	sentCount    int64
	failedCount  int64
	droppedCount int64
}

// kafkaMessage represents a message to be sent to Kafka.
type kafkaMessage struct {
	Key       []byte
	Value     []byte
	Timestamp time.Time
	Topic     string
}

// KafkaMessage represents the JSON format of an audit log message sent to Kafka.
type KafkaMessage struct {
	Version    string            `json:"version"`
	Type       string            `json:"type"`
	Timestamp  time.Time         `json:"timestamp"`
	GatewayID  string            `json:"gateway_id"`
	AuditEntry *store.AuditEntry `json:"audit_entry,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// NewKafkaWriter creates a new Kafka writer for audit logs.
func NewKafkaWriter(cfg config.KafkaConfig) (*KafkaWriter, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers required")
	}

	topic := cfg.Topic
	if topic == "" {
		topic = "apicerberus.audit"
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	kw := &KafkaWriter{
		config:        cfg,
		brokers:       cfg.Brokers,
		topic:         topic,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		messages:      make(chan *kafkaMessage, cfg.BufferSize),
		stopCh:        make(chan struct{}),
	}

	// Initial connection
	if err := kw.connect(); err != nil {
		if !cfg.AsyncConnect {
			return nil, err
		}
		// Async connect - will retry in background
		go kw.retryConnect()
	}

	// Warn operators that this is using a text protocol fallback, not real Kafka protocol
	log.Printf("[WARN] audit: using text protocol fallback for Kafka - not compatible with real Kafka brokers. For production, use a proper Kafka client library.")

	// Start background workers
	for i := 0; i < cfg.Workers; i++ {
		kw.wg.Add(1)
		go kw.worker()
	}

	return kw, nil
}

// Enabled returns true if Kafka streaming is enabled.
func (kw *KafkaWriter) Enabled() bool {
	return kw != nil
}

// Write sends an audit entry to Kafka.
func (kw *KafkaWriter) Write(entry store.AuditEntry) error {
	if !kw.Enabled() {
		return nil
	}

	msg := &KafkaMessage{
		Version:    "1.0",
		Type:       "audit_log",
		Timestamp:  time.Now().UTC(),
		GatewayID:  kw.config.GatewayID,
		AuditEntry: &entry,
		Metadata: map[string]any{
			"region":      kw.config.Region,
			"data_center": kw.config.Datacenter,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	key := []byte(entry.RequestID)
	if key == nil {
		key = []byte(entry.UserID)
	}

	kafkaMsg := &kafkaMessage{
		Key:       key,
		Value:     data,
		Timestamp: msg.Timestamp,
		Topic:     kw.topic,
	}

	select {
	case kw.messages <- kafkaMsg:
		return nil
	default:
		kw.droppedCount++
		if kw.config.BlockOnFull {
			kw.messages <- kafkaMsg
			return nil
		}
		return fmt.Errorf("kafka message queue full, message dropped")
	}
}

// WriteBatch sends multiple audit entries to Kafka.
func (kw *KafkaWriter) WriteBatch(entries []store.AuditEntry) error {
	if !kw.Enabled() || len(entries) == 0 {
		return nil
	}

	for i := range entries {
		if err := kw.Write(entries[i]); err != nil {
			return err
		}
	}
	return nil
}

// Close shuts down the Kafka writer.
func (kw *KafkaWriter) Close() error {
	if !kw.Enabled() {
		return nil
	}

	close(kw.stopCh)
	kw.wg.Wait()

	kw.mu.Lock()
	defer kw.mu.Unlock()

	if kw.conn != nil {
		_ = kw.conn.Close() // #nosec G104 // Best-effort cleanup in Close().
		kw.conn = nil
	}
	kw.connected = false

	return nil
}

// Stats returns current statistics.
func (kw *KafkaWriter) Stats() KafkaStats {
	return KafkaStats{
		Sent:    kw.sentCount,
		Failed:  kw.failedCount,
		Dropped: kw.droppedCount,
		Queued:  int64(len(kw.messages)),
	}
}

// KafkaStats holds streaming statistics.
type KafkaStats struct {
	Sent    int64 `json:"sent"`
	Failed  int64 `json:"failed"`
	Dropped int64 `json:"dropped"`
	Queued  int64 `json:"queued"`
}

// worker processes messages in the background.
func (kw *KafkaWriter) worker() {
	defer kw.wg.Done()

	ticker := time.NewTicker(kw.flushInterval)
	defer ticker.Stop()

	batch := make([]*kafkaMessage, 0, kw.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := kw.sendBatch(batch); err != nil {
			kw.failedCount += int64(len(batch))
		} else {
			kw.sentCount += int64(len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-kw.stopCh:
			// Drain remaining messages
			drain := true
			for drain {
				select {
				case msg := <-kw.messages:
					batch = append(batch, msg)
					if len(batch) >= kw.batchSize {
						flush()
					}
				default:
					drain = false
				}
			}
			flush()
			return

		case msg := <-kw.messages:
			batch = append(batch, msg)
			if len(batch) >= kw.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// sendBatch sends a batch of messages to Kafka.
func (kw *KafkaWriter) sendBatch(batch []*kafkaMessage) error {
	kw.mu.RLock()
	connected := kw.connected
	kw.mu.RUnlock()

	if !connected {
		// Try to reconnect
		if err := kw.connect(); err != nil {
			return err
		}
	}

	// Simple protocol implementation
	// In production, use a proper Kafka client library
	for _, msg := range batch {
		if err := kw.sendMessage(msg); err != nil {
			return err
		}
	}

	return nil
}

// sendMessage sends a single message using simple text protocol.
func (kw *KafkaWriter) sendMessage(msg *kafkaMessage) error {
	kw.mu.Lock()
	defer kw.mu.Unlock()

	if kw.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Format as JSON line protocol
	data, err := json.Marshal(map[string]any{
		"topic":     msg.Topic,
		"key":       string(msg.Key),
		"value":     string(msg.Value),
		"timestamp": msg.Timestamp.Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}

	data = append(data, '\n')

	// Set timeout
	_ = kw.conn.SetWriteDeadline(time.Now().Add(kw.config.WriteTimeout)) // #nosec G104 // Best-effort deadline set.
	defer func() { _ = kw.conn.SetWriteDeadline(time.Time{}) }()         // #nosec G104 // Best-effort reset.

	_, err = kw.conn.Write(data)
	return err
}

// connect establishes connection to Kafka.
func (kw *KafkaWriter) connect() error {
	kw.mu.Lock()
	defer kw.mu.Unlock()

	if kw.connected && kw.conn != nil {
		return nil
	}

	// Try each broker
	var lastErr error
	for _, broker := range kw.brokers {
		conn, err := kw.dial(broker)
		if err == nil {
			kw.conn = conn
			kw.connected = true
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("failed to connect to any broker: %w", lastErr)
}

// dial connects to a single broker.
func (kw *KafkaWriter) dial(broker string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   kw.config.DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dialer.Dial("tcp", broker)
	if err != nil {
		return nil, err
	}

	// Configure TLS if enabled
	if kw.config.TLS.Enabled {
		if kw.config.TLS.SkipVerify {
			log.Printf("[WARN] kafka: TLS certificate verification is disabled. This is insecure and should not be used in production.")
		}
		tlsConfig := &tls.Config{
			InsecureSkipVerify: kw.config.TLS.SkipVerify, // #nosec G402 -- InsecureSkipVerify is admin-configurable via Kafka TLS config.
			ServerName:         kw.config.TLS.ServerName,
		}
		conn = tls.Client(conn, tlsConfig)
	}

	return conn, nil
}

// retryConnect attempts to reconnect in the background.
func (kw *KafkaWriter) retryConnect() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-kw.stopCh:
			return
		case <-ticker.C:
			if err := kw.connect(); err == nil {
				return // Connected successfully
			}
		}
	}
}
