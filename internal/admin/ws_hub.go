package admin

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/logging"
)

// WebSocketHub manages WebSocket connections with pooling and broadcasting
type WebSocketHub struct {
	mu           sync.RWMutex
	connections  map[string]*WebSocketConn
	pools        *WebSocketPoolManager
	subscribers  map[string]map[string]bool // topic -> connID -> bool
	broadcastCh  chan BroadcastMessage
	registerCh   chan *WebSocketConn
	unregisterCh chan string // connID
	stopCh       chan struct{}
	closed       atomic.Bool
	logger       *logging.StructuredLogger

	// Metrics
	metrics WebSocketMetrics
}

// WebSocketConn wraps a WebSocket connection with metadata
type WebSocketConn struct {
	ID        string
	Conn      net.Conn
	Topics    map[string]bool
	CreatedAt time.Time
	LastPing  time.Time
	mu        sync.RWMutex
	writeCh   chan []byte
	hub       *WebSocketHub
	closeOnce sync.Once
	writeMu   sync.Mutex // serializes all writes to Conn
}

// BroadcastMessage represents a message to broadcast
type BroadcastMessage struct {
	Topic   string
	Event   realtimeEvent
	Exclude string // connID to exclude (for sender exclusion)
}

// WebSocketMetrics tracks connection metrics
type WebSocketMetrics struct {
	TotalConnections    atomic.Int64
	ActiveConnections   atomic.Int64
	MessagesSent        atomic.Int64
	MessagesReceived    atomic.Int64
	BroadcastsDelivered atomic.Int64
	Errors              atomic.Int64
}

// WebSocketPoolManager manages connection pools by topic
type WebSocketPoolManager struct {
	mu    sync.RWMutex
	pools map[string]*sync.Pool
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub(logger *logging.StructuredLogger) *WebSocketHub {
	hub := &WebSocketHub{
		connections:  make(map[string]*WebSocketConn),
		pools:        NewWebSocketPoolManager(),
		subscribers:  make(map[string]map[string]bool),
		broadcastCh:  make(chan BroadcastMessage, 256),
		registerCh:   make(chan *WebSocketConn, 32),
		unregisterCh: make(chan string, 32),
		stopCh:       make(chan struct{}),
		logger:       logger,
	}

	go hub.run()
	go hub.cleanupLoop()

	return hub
}

// NewWebSocketPoolManager creates a new pool manager
func NewWebSocketPoolManager() *WebSocketPoolManager {
	return &WebSocketPoolManager{
		pools: make(map[string]*sync.Pool),
	}
}

// GetPool returns or creates a pool for a topic
func (pm *WebSocketPoolManager) GetPool(topic string) *sync.Pool {
	pm.mu.RLock()
	pool, exists := pm.pools[topic]
	pm.mu.RUnlock()

	if exists {
		return pool
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, exists = pm.pools[topic]; exists {
		return pool
	}

	pool = &sync.Pool{
		New: func() any {
			return make([]byte, 0, 1024)
		},
	}
	pm.pools[topic] = pool
	return pool
}

// GetBuffer gets a buffer from the pool
func (pm *WebSocketPoolManager) GetBuffer(topic string) []byte {
	pool := pm.GetPool(topic)
	buf, ok := pool.Get().([]byte)
	if !ok || buf == nil {
		return make([]byte, 0, 1024)
	}
	return buf
}

// PutBuffer returns a buffer to the pool
func (pm *WebSocketPoolManager) PutBuffer(topic string, buf []byte) {
	pool := pm.GetPool(topic)
	// Reset slice but keep capacity
	pool.Put(buf[:0])
}

// Register registers a new connection
func (h *WebSocketHub) Register(conn net.Conn, topics []string) *WebSocketConn {
	if h.closed.Load() {
		conn.Close()
		return nil
	}

	wsConn := &WebSocketConn{
		ID:        generateConnID(),
		Conn:      conn,
		Topics:    make(map[string]bool),
		CreatedAt: time.Now(),
		LastPing:  time.Now(),
		writeCh:   make(chan []byte, 64),
		hub:       h,
	}

	for _, topic := range topics {
		wsConn.Topics[topic] = true
	}

	h.registerCh <- wsConn
	h.metrics.TotalConnections.Add(1)
	h.metrics.ActiveConnections.Add(1)

	// Start connection handlers
	go wsConn.writePump()
	go wsConn.readPump()

	return wsConn
}

// Unregister removes a connection
func (h *WebSocketHub) Unregister(connID string) {
	select {
	case h.unregisterCh <- connID:
	case <-time.After(time.Second):
		// Timeout - connection may already be gone
	}
}

// Broadcast sends a message to all subscribers of a topic
func (h *WebSocketHub) Broadcast(topic string, event realtimeEvent) {
	select {
	case h.broadcastCh <- BroadcastMessage{Topic: topic, Event: event}:
	case <-time.After(time.Second):
		h.logger.Warn("broadcast channel full, dropping message to topic=%s", topic)
	}
}

// BroadcastExcept sends to all except one connection
func (h *WebSocketHub) BroadcastExcept(topic string, event realtimeEvent, excludeConnID string) {
	select {
	case h.broadcastCh <- BroadcastMessage{Topic: topic, Event: event, Exclude: excludeConnID}:
	case <-time.After(time.Second):
		h.logger.Warn("broadcast channel full, dropping message to topic=%s", topic)
	}
}

// Subscribe adds a connection to a topic
func (h *WebSocketHub) Subscribe(connID string, topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.subscribers[topic]; !exists {
		h.subscribers[topic] = make(map[string]bool)
	}
	h.subscribers[topic][connID] = true

	if conn, exists := h.connections[connID]; exists {
		conn.mu.Lock()
		conn.Topics[topic] = true
		conn.mu.Unlock()
	}
}

// Unsubscribe removes a connection from a topic
func (h *WebSocketHub) Unsubscribe(connID string, topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, exists := h.subscribers[topic]; exists {
		delete(subs, connID)
		if len(subs) == 0 {
			delete(h.subscribers, topic)
		}
	}

	if conn, exists := h.connections[connID]; exists {
		conn.mu.Lock()
		delete(conn.Topics, topic)
		conn.mu.Unlock()
	}
}

// GetMetrics returns a snapshot of current metrics
func (h *WebSocketHub) GetMetrics() WebSocketMetrics {
	return WebSocketMetrics{
		TotalConnections:    atomic.Int64{},
		ActiveConnections:   atomic.Int64{},
		MessagesSent:        atomic.Int64{},
		MessagesReceived:    atomic.Int64{},
		BroadcastsDelivered: atomic.Int64{},
		Errors:              atomic.Int64{},
	}
}

// MetricsSnapshot returns a point-in-time snapshot of all metrics as plain int64 values.
func (h *WebSocketHub) MetricsSnapshot() (totalConn, activeConn, msgSent, msgRecv, broadcasts, errors int64) {
	return h.metrics.TotalConnections.Load(),
		h.metrics.ActiveConnections.Load(),
		h.metrics.MessagesSent.Load(),
		h.metrics.MessagesReceived.Load(),
		h.metrics.BroadcastsDelivered.Load(),
		h.metrics.Errors.Load()
}

// Stop shuts down the hub
func (h *WebSocketHub) Stop() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}

	close(h.stopCh)

	// Close all connections
	h.mu.Lock()
	for _, conn := range h.connections {
		conn.close()
	}
	h.mu.Unlock()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)
	h.logger.Info("websocket hub stopped")
}

// run is the main event loop
func (h *WebSocketHub) run() {
	for {
		select {
		case <-h.stopCh:
			return

		case conn := <-h.registerCh:
			h.handleRegister(conn)

		case connID := <-h.unregisterCh:
			h.handleUnregister(connID)

		case msg := <-h.broadcastCh:
			h.handleBroadcast(msg)
		}
	}
}

// handleRegister processes a new connection
func (h *WebSocketHub) handleRegister(conn *WebSocketConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.connections[conn.ID] = conn

	// Subscribe to requested topics
	for topic := range conn.Topics {
		if _, exists := h.subscribers[topic]; !exists {
			h.subscribers[topic] = make(map[string]bool)
		}
		h.subscribers[topic][conn.ID] = true
	}

	h.logger.Info("websocket connection registered: conn_id=%s topics=%d", conn.ID, len(conn.Topics))
}

// handleUnregister removes a connection
func (h *WebSocketHub) handleUnregister(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn, exists := h.connections[connID]
	if !exists {
		return
	}

	// Remove from all topics
	for topic := range conn.Topics {
		if subs, exists := h.subscribers[topic]; exists {
			delete(subs, connID)
			if len(subs) == 0 {
				delete(h.subscribers, topic)
			}
		}
	}

	conn.close()
	delete(h.connections, connID)
	h.metrics.ActiveConnections.Add(-1)

	h.logger.Info("websocket connection unregistered: conn_id=%s", connID)
}

// handleBroadcast sends message to all subscribers
func (h *WebSocketHub) handleBroadcast(msg BroadcastMessage) {
	h.mu.RLock()
	subscribers, exists := h.subscribers[msg.Topic]
	if !exists || len(subscribers) == 0 {
		h.mu.RUnlock()
		return
	}

	// Get connection IDs to send to
	connIDs := make([]string, 0, len(subscribers))
	for connID := range subscribers {
		if connID != msg.Exclude {
			connIDs = append(connIDs, connID)
		}
	}

	// Collect connections
	conns := make([]*WebSocketConn, 0, len(connIDs))
	for _, connID := range connIDs {
		if conn, exists := h.connections[connID]; exists {
			conns = append(conns, conn)
		}
	}
	h.mu.RUnlock()

	if len(conns) == 0 {
		return
	}

	// Marshal message once
	payload, err := json.Marshal(msg.Event)
	if err != nil {
		h.metrics.Errors.Add(1)
		h.logger.Error("failed to marshal broadcast message: %v", err)
		return
	}

	// Send to all connections concurrently
	var wg sync.WaitGroup
	for _, conn := range conns {
		wg.Add(1)
		go func(c *WebSocketConn) {
			defer wg.Done()
			if err := c.Send(payload); err != nil {
				h.metrics.Errors.Add(1)
			} else {
				h.metrics.BroadcastsDelivered.Add(1)
			}
		}(conn)
	}
	wg.Wait()
}

// cleanupLoop periodically cleans up stale connections
func (h *WebSocketHub) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.cleanupStaleConnections()
		}
	}
}

// cleanupStaleConnections removes dead connections
func (h *WebSocketHub) cleanupStaleConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	timeout := 2 * time.Minute

	for connID, conn := range h.connections {
		conn.mu.RLock()
		lastPing := conn.LastPing
		conn.mu.RUnlock()

		if now.Sub(lastPing) > timeout {
			// Connection is stale
			conn.close()
			delete(h.connections, connID)

			// Remove from all topics (read under conn lock to avoid map race)
			conn.mu.RLock()
			topics := make([]string, 0, len(conn.Topics))
			for topic := range conn.Topics {
				topics = append(topics, topic)
			}
			conn.mu.RUnlock()

			for _, topic := range topics {
				if subs, exists := h.subscribers[topic]; exists {
					delete(subs, connID)
					if len(subs) == 0 {
						delete(h.subscribers, topic)
					}
				}
			}

			h.metrics.ActiveConnections.Add(-1)
			h.logger.Info("websocket connection cleaned up (stale): conn_id=%s", connID)
		}
	}
}

// writePump handles outgoing messages
func (c *WebSocketConn) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.hub.stopCh:
			return

		case msg, ok := <-c.writeCh:
			if !ok {
				return
			}
			c.writeMu.Lock()
			err := writeWebSocketTextFrame(c.Conn, msg)
			c.writeMu.Unlock()
			if err != nil {
				c.hub.metrics.Errors.Add(1)
				return
			}
			c.hub.metrics.MessagesSent.Add(1)

		case <-ticker.C:
			// Send ping
			if err := c.sendPing(); err != nil {
				return
			}
		}
	}
}

// readPump handles incoming messages and pings
func (c *WebSocketConn) readPump() {
	buf := make([]byte, 1024)

	for {
		select {
		case <-c.hub.stopCh:
			return
		default:
		}

		// Set read deadline
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Read frame (simplified - just reads and discards for ping handling)
		n, err := c.Conn.Read(buf)
		if err != nil {
			c.hub.Unregister(c.ID)
			return
		}

		if n > 0 {
			c.mu.Lock()
			c.LastPing = time.Now()
			c.mu.Unlock()
			c.hub.metrics.MessagesReceived.Add(1)

			// Handle WebSocket frames (basic ping/pong)
			if n >= 2 {
				opcode := buf[0] & 0x0F
				if opcode == 0x09 { // Ping frame
					c.sendPong()
				}
			}
		}
	}
}

// Send sends a message to the connection
func (c *WebSocketConn) Send(payload []byte) error {
	select {
	case c.writeCh <- payload:
		return nil
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}

// sendPing sends a WebSocket ping frame
func (c *WebSocketConn) sendPing() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	ping := []byte{0x89, 0x00} // Ping frame with no payload
	_, err := c.Conn.Write(ping)
	return err
}

// sendPong sends a WebSocket pong frame
func (c *WebSocketConn) sendPong() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	pong := []byte{0x8A, 0x00} // Pong frame with no payload
	_, err := c.Conn.Write(pong)
	return err
}

// close closes the connection. Safe to call multiple times.
func (c *WebSocketConn) close() {
	c.closeOnce.Do(func() {
		close(c.writeCh)
		c.Conn.Close()
	})
}

// generateConnID generates a unique connection ID
func generateConnID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString generates a cryptographically random string
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, n)
	randBytes := make([]byte, n)
	if _, err := cryptorand.Read(randBytes); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	for i := range result {
		result[i] = letters[int(randBytes[i])%len(letters)]
	}
	return string(result)
}
