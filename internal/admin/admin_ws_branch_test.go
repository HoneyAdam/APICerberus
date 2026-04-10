package admin

import (
	"testing"

	"github.com/APICerberus/APICerebrus/internal/logging"
)

// TestBroadcastFullChannel covers the timeout branch of Broadcast
func TestBroadcastFullChannel(t *testing.T) {
	t.Parallel()
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	hub.Stop() // Stop so messages aren't consumed from broadcastCh

	// Broadcast after stop should hit timeout (channel not being drained)
	hub.Broadcast("test", realtimeEvent{Type: "test", Payload: map[string]any{"key": "value"}})
	hub.Broadcast("test2", realtimeEvent{Type: "update"})
	hub.Broadcast("test3", realtimeEvent{Type: "error"})
}

// TestBroadcastExceptFullChannel covers the timeout branch of BroadcastExcept
func TestBroadcastExceptFullChannel(t *testing.T) {
	t.Parallel()
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	hub.Stop()

	hub.BroadcastExcept("test", realtimeEvent{Type: "test", Payload: map[string]any{"key": "value"}}, "exclude-1")
	hub.BroadcastExcept("test2", realtimeEvent{Type: "update"}, "exclude-2")
}

// TestGetBufferFreshBuffer covers the fresh buffer creation path
func TestGetBufferFreshBuffer(t *testing.T) {
	t.Parallel()
	pm := NewWebSocketPoolManager()
	buf := pm.GetBuffer("fresh-topic-xyz-123")
	if buf == nil {
		t.Error("expected non-nil buffer")
	}
	if cap(buf) != 1024 {
		t.Errorf("expected capacity 1024, got %d", cap(buf))
	}
}
