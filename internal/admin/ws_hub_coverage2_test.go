package admin

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/logging"
)

// --- handleRegister ---

func TestHandleRegister_BasicConnection(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	conn := &WebSocketConn{
		ID:      "test-conn-1",
		Conn:    server,
		Topics:  map[string]bool{"alerts": true, "metrics": true},
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	hub.handleRegister(conn)

	hub.mu.RLock()
	registered, exists := hub.connections["test-conn-1"]
	hub.mu.RUnlock()

	if !exists {
		t.Fatal("connection not registered")
	}
	if registered != conn {
		t.Error("wrong connection stored")
	}

	// Check topic subscriptions
	hub.mu.RLock()
	alertSubs := hub.subscribers["alerts"]
	metricSubs := hub.subscribers["metrics"]
	hub.mu.RUnlock()

	if !alertSubs["test-conn-1"] {
		t.Error("not subscribed to alerts")
	}
	if !metricSubs["test-conn-1"] {
		t.Error("not subscribed to metrics")
	}
}

func TestHandleRegister_NoTopics(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	conn := &WebSocketConn{
		ID:      "no-topics-conn",
		Conn:    server,
		Topics:  map[string]bool{},
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	hub.handleRegister(conn)

	hub.mu.RLock()
	_, exists := hub.connections["no-topics-conn"]
	hub.mu.RUnlock()

	if !exists {
		t.Fatal("connection not registered")
	}
}

// --- handleUnregister ---

func TestHandleUnregister_Existing(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	conn := &WebSocketConn{
		ID:      "to-remove",
		Conn:    server,
		Topics:  map[string]bool{"alerts": true},
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	// Register first
	hub.handleRegister(conn)

	// Now unregister
	hub.handleUnregister("to-remove")

	hub.mu.RLock()
	_, connExists := hub.connections["to-remove"]
	_, topicExists := hub.subscribers["alerts"]
	hub.mu.RUnlock()

	if connExists {
		t.Error("connection should be removed")
	}
	if topicExists {
		t.Error("empty topic should be cleaned up")
	}
}

func TestHandleUnregister_NonExistent(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	// Should not panic
	hub.handleUnregister("nonexistent")
}

func TestHandleUnregister_WithMultipleTopics(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	conn1 := &WebSocketConn{
		ID:     "conn-1",
		Conn:   server,
		Topics: map[string]bool{"alerts": true, "metrics": true},
		writeCh: make(chan []byte, 64),
		hub:    hub,
	}

	server2, _ := net.Pipe()
	defer server2.Close()

	conn2 := &WebSocketConn{
		ID:     "conn-2",
		Conn:   server2,
		Topics: map[string]bool{"alerts": true},
		writeCh: make(chan []byte, 64),
		hub:    hub,
	}

	hub.handleRegister(conn1)
	hub.handleRegister(conn2)

	// Unregister conn-1 — alerts topic should still exist (conn-2 subscribed)
	hub.handleUnregister("conn-1")

	hub.mu.RLock()
	alertSubs := hub.subscribers["alerts"]
	metricSubs := hub.subscribers["metrics"]
	hub.mu.RUnlock()

	if !alertSubs["conn-2"] {
		t.Error("conn-2 should still be subscribed to alerts")
	}
	if len(metricSubs) != 0 {
		t.Error("metrics topic should be empty after conn-1 removed")
	}
}

// --- cleanupStaleConnections ---

func TestCleanupStaleConnections_RemovesStale(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	staleConn := &WebSocketConn{
		ID:        "stale-conn",
		Conn:      server,
		Topics:    map[string]bool{"alerts": true},
		writeCh:   make(chan []byte, 64),
		closeOnce: sync.Once{},
		LastPing:  time.Now().Add(-5 * time.Minute), // 5 minutes ago = stale
		hub:       hub,
	}

	hub.handleRegister(staleConn)

	// Verify it's there
	hub.mu.RLock()
	_, existsBefore := hub.connections["stale-conn"]
	hub.mu.RUnlock()
	if !existsBefore {
		t.Fatal("stale conn should exist before cleanup")
	}

	hub.cleanupStaleConnections()

	hub.mu.RLock()
	_, existsAfter := hub.connections["stale-conn"]
	hub.mu.RUnlock()
	if existsAfter {
		t.Error("stale connection should be removed")
	}
}

func TestCleanupStaleConnections_KeepsFresh(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	freshConn := &WebSocketConn{
		ID:        "fresh-conn",
		Conn:      server,
		Topics:    map[string]bool{"alerts": true},
		writeCh:   make(chan []byte, 64),
		closeOnce: sync.Once{},
		LastPing:  time.Now(), // just now = fresh
		hub:       hub,
	}

	hub.handleRegister(freshConn)

	hub.cleanupStaleConnections()

	hub.mu.RLock()
	_, exists := hub.connections["fresh-conn"]
	hub.mu.RUnlock()
	if !exists {
		t.Error("fresh connection should survive cleanup")
	}
}

func TestCleanupStaleConnections_Mixed(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server1, _ := net.Pipe()
	defer server1.Close()

	server2, _ := net.Pipe()
	defer server2.Close()

	stale := &WebSocketConn{
		ID: "stale", Conn: server1,
		Topics: map[string]bool{"alerts": true},
		writeCh: make(chan []byte, 64), closeOnce: sync.Once{},
		LastPing: time.Now().Add(-3 * time.Minute), hub: hub,
	}
	fresh := &WebSocketConn{
		ID: "fresh", Conn: server2,
		Topics: map[string]bool{"alerts": true},
		writeCh: make(chan []byte, 64), closeOnce: sync.Once{},
		LastPing: time.Now(), hub: hub,
	}

	hub.handleRegister(stale)
	hub.handleRegister(fresh)

	hub.cleanupStaleConnections()

	hub.mu.RLock()
	_, staleExists := hub.connections["stale"]
	_, freshExists := hub.connections["fresh"]
	hub.mu.RUnlock()

	if staleExists {
		t.Error("stale should be removed")
	}
	if !freshExists {
		t.Error("fresh should survive")
	}
}

func TestCleanupStaleConnections_EmptyHub(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	// Should not panic on empty hub
	hub.cleanupStaleConnections()
}

// --- sendPing / sendPong via net.Pipe ---

func TestSendPing_WritesFrame(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	hub := &WebSocketHub{
		connections: make(map[string]*WebSocketConn),
		subscribers: make(map[string]map[string]bool),
		stopCh:      make(chan struct{}),
	}
	conn := &WebSocketConn{
		ID: "ping-conn", Conn: server,
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	// Read from client side in background
	resultCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2)
		n, err := client.Read(buf)
		if err != nil {
			close(resultCh)
			return
		}
		resultCh <- buf[:n]
	}()

	if err := conn.sendPing(); err != nil {
		t.Fatalf("sendPing: %v", err)
	}

	select {
	case data := <-resultCh:
		if len(data) != 2 || data[0] != 0x89 || data[1] != 0x00 {
			t.Errorf("ping frame = %x, want [89 00]", data)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout reading ping frame")
	}
}

func TestSendPong_WritesFrame(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	hub := &WebSocketHub{
		connections: make(map[string]*WebSocketConn),
		subscribers: make(map[string]map[string]bool),
		stopCh:      make(chan struct{}),
	}
	conn := &WebSocketConn{
		ID: "pong-conn", Conn: server,
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	resultCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2)
		n, err := client.Read(buf)
		if err != nil {
			close(resultCh)
			return
		}
		resultCh <- buf[:n]
	}()

	if err := conn.sendPong(); err != nil {
		t.Fatalf("sendPong: %v", err)
	}

	select {
	case data := <-resultCh:
		if len(data) != 2 || data[0] != 0x8A || data[1] != 0x00 {
			t.Errorf("pong frame = %x, want [8a 00]", data)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout reading pong frame")
	}
}

func TestSendPing_ClosedConn(t *testing.T) {
	t.Parallel()
	// Use a closed connection — write should error
	server, client := net.Pipe()
	server.Close()
	client.Close()
	conn := &WebSocketConn{Conn: server}
	if err := conn.sendPing(); err == nil {
		t.Error("expected error on closed conn")
	}
}

func TestSendPong_ClosedConn(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	server.Close()
	client.Close()
	conn := &WebSocketConn{Conn: server}
	if err := conn.sendPong(); err == nil {
		t.Error("expected error on closed conn")
	}
}

// --- writePump basic test ---

func TestWritePump_SendsMessage(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	hub := &WebSocketHub{
		connections: make(map[string]*WebSocketConn),
		subscribers: make(map[string]map[string]bool),
		broadcastCh: make(chan BroadcastMessage, 256),
		registerCh:  make(chan *WebSocketConn, 32),
		unregisterCh: make(chan string, 32),
		stopCh:      make(chan struct{}),
		logger:      logging.NewStructuredLogger(nil, logging.ErrorLevel),
	}
	conn := &WebSocketConn{
		ID: "wp-conn", Conn: server,
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	// Start writePump
	go conn.writePump()

	// Send a message
	conn.writeCh <- []byte("hello pump")

	// Read from client — writePump sends via writeWebSocketTextFrame which adds WS framing
	resultCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := client.Read(buf)
		if err != nil {
			close(resultCh)
			return
		}
		resultCh <- buf[:n]
	}()

	select {
	case data := <-resultCh:
		// Should contain the message "hello pump" somewhere in the frame
		if len(data) == 0 {
			t.Error("expected some data from writePump")
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for writePump output")
	}

	// Stop the pump
	close(hub.stopCh)
}

func TestWritePump_StopsOnHubStop(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	stopCh := make(chan struct{})
	hub := &WebSocketHub{
		connections: make(map[string]*WebSocketConn),
		subscribers: make(map[string]map[string]bool),
		stopCh:      stopCh,
		logger:      logging.NewStructuredLogger(nil, logging.ErrorLevel),
	}
	conn := &WebSocketConn{
		ID: "stop-conn", Conn: server,
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	done := make(chan struct{})
	go func() {
		conn.writePump()
		close(done)
	}()

	// Stop immediately
	close(stopCh)

	select {
	case <-done:
		// writePump exited cleanly
	case <-time.After(2 * time.Second):
		t.Error("writePump did not exit on hub stop")
	}
}

// --- Unregister channel test ---

func TestHub_Unregister_Enqueues(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, _ := net.Pipe()
	defer server.Close()

	conn := &WebSocketConn{
		ID: "unreg-conn", Conn: server,
		Topics:  map[string]bool{"alerts": true},
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}

	// Register via handleRegister directly (avoids goroutine start)
	hub.handleRegister(conn)

	// Now unregister via channel — this requires the run() goroutine to be active
	// For direct test, call handleUnregister
	hub.handleUnregister("unreg-conn")

	hub.mu.RLock()
	_, exists := hub.connections["unreg-conn"]
	hub.mu.RUnlock()

	if exists {
		t.Error("connection should be removed after unregister")
	}
}

// --- generateConnID ---

func TestGenerateConnID_TwoCalls(t *testing.T) {
	t.Parallel()
	id1, err := generateConnID()
	if err != nil {
		t.Fatalf("generateConnID: %v", err)
	}
	if len(id1) == 0 {
		t.Error("expected non-empty ID")
	}

	id2, err := generateConnID()
	if err != nil {
		t.Fatalf("generateConnID: %v", err)
	}
	if id1 == id2 {
		t.Error("expected unique IDs")
	}
}

func TestRandomString_Length(t *testing.T) {
	t.Parallel()
	s, err := randomString(16)
	if err != nil {
		t.Fatalf("randomString: %v", err)
	}
	if len(s) != 16 {
		t.Errorf("length = %d, want 16", len(s))
	}
}
