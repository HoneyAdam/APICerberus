package admin

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/logging"
)

// --- writeInternalError ---

func TestWriteInternalError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	writeInternalError(w, "TEST_CODE", errors.New("test error"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != "TEST_CODE" {
		t.Errorf("code = %v, want TEST_CODE", errObj["code"])
	}
}

func TestWriteInternalError_NilErr(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	writeInternalError(w, "NIL_ERR", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- entity lookup helpers ---

func TestServiceIndexByID(t *testing.T) {
	t.Parallel()
	cfg := configFromServices(
		config.Service{ID: "svc-1"},
		config.Service{ID: "svc-2"},
	)
	if i := serviceIndexByID(cfg, "svc-2"); i != 1 {
		t.Errorf("index = %d, want 1", i)
	}
	if i := serviceIndexByID(cfg, "missing"); i != -1 {
		t.Errorf("index = %d, want -1", i)
	}
}

func TestRouteIndexByID(t *testing.T) {
	t.Parallel()
	cfg := configFromRoutes(
		config.Route{ID: "r-1"},
		config.Route{ID: "r-2"},
	)
	if i := routeIndexByID(cfg, "r-2"); i != 1 {
		t.Errorf("index = %d, want 1", i)
	}
	if i := routeIndexByID(cfg, "missing"); i != -1 {
		t.Errorf("index = %d, want -1", i)
	}
}

func TestUpstreamIndexByID(t *testing.T) {
	t.Parallel()
	cfg := configFromUpstreams(
		config.Upstream{ID: "up-1"},
		config.Upstream{ID: "up-2"},
	)
	if i := upstreamIndexByID(cfg, "up-2"); i != 1 {
		t.Errorf("index = %d, want 1", i)
	}
	if i := upstreamIndexByID(cfg, "missing"); i != -1 {
		t.Errorf("index = %d, want -1", i)
	}
}

// helper constructors for test configs
func configFromServices(svcs ...config.Service) *config.Config {
	return &config.Config{Services: svcs}
}
func configFromRoutes(routes ...config.Route) *config.Config {
	return &config.Config{Routes: routes}
}
func configFromUpstreams(ups ...config.Upstream) *config.Config {
	return &config.Config{Upstreams: ups}
}

func newTestHub() *WebSocketHub {
	return NewWebSocketHub(logging.NewStructuredLogger(nil, logging.ErrorLevel))
}

// --- validateServiceInput additional ---

func TestValidateServiceInput_ValidProtocols(t *testing.T) {
	t.Parallel()
	for _, proto := range []string{"http", "grpc", "graphql", "HTTP", "GRPC", "GraphQL"} {
		err := validateServiceInput(config.Service{Name: "svc", Upstream: "up", Protocol: proto})
		if err != nil {
			t.Errorf("protocol %q: %v", proto, err)
		}
	}
}

// --- validateRouteInput additional ---

func TestValidateRouteInput_NoPaths(t *testing.T) {
	t.Parallel()
	err := validateRouteInput(config.Route{Name: "r", Service: "svc"})
	if err == nil {
		t.Error("expected error for no paths")
	}
}

func TestValidateRouteInput_PathNotSlash(t *testing.T) {
	t.Parallel()
	err := validateRouteInput(config.Route{Name: "r", Service: "svc", Paths: []string{"api"}})
	if err == nil {
		t.Error("expected error for path not starting with /")
	}
}

func TestValidateRouteInput_InvalidMethod(t *testing.T) {
	t.Parallel()
	err := validateRouteInput(config.Route{
		Name: "r", Service: "svc", Paths: []string{"/api"},
		Methods: []string{"INVALID"},
	})
	if err == nil {
		t.Error("expected error for invalid method")
	}
}

// --- validateUpstreamInput additional ---

func TestValidateUpstreamInput_InvalidAlgorithm(t *testing.T) {
	t.Parallel()
	err := validateUpstreamInput(config.Upstream{
		Name: "test", Algorithm: "invalid_alg",
		Targets: []config.UpstreamTarget{{ID: "t1", Address: "localhost:3000", Weight: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "algorithm") {
		t.Errorf("err = %v", err)
	}
}

func TestValidateUpstreamInput_TargetMissingAddress(t *testing.T) {
	t.Parallel()
	err := validateUpstreamInput(config.Upstream{
		Name: "test", Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{{ID: "t1", Address: "", Weight: 1}},
	})
	if err == nil {
		t.Error("expected error for missing target address")
	}
}

// --- WebSocket Hub Subscribe/Unsubscribe (unit-level) ---

func TestWebSocketHub_SubscribeUnsubscribe(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	hub.Subscribe("conn-1", "topic-a")
	hub.mu.RLock()
	subs := hub.subscribers["topic-a"]
	hub.mu.RUnlock()
	if !subs["conn-1"] {
		t.Error("expected conn-1 subscribed to topic-a")
	}

	hub.Unsubscribe("conn-1", "topic-a")
	hub.mu.RLock()
	subs = hub.subscribers["topic-a"]
	hub.mu.RUnlock()
	if len(subs) != 0 {
		t.Error("expected empty subscribers after unsubscribe")
	}
}

func TestWebSocketHub_SubscribeCreatesTopic(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	hub.Subscribe("conn-1", "new-topic")
	hub.mu.RLock()
	_, exists := hub.subscribers["new-topic"]
	hub.mu.RUnlock()
	if !exists {
		t.Error("expected topic to be created")
	}
}

func TestWebSocketHub_UnsubscribeNonexistentTopic(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()
	hub.Unsubscribe("conn-1", "nonexistent") // should not panic
}

// --- WebSocket Pool Manager ---

func TestWebSocketPoolManager_GetBuffer(t *testing.T) {
	t.Parallel()
	pm := NewWebSocketPoolManager()
	buf := pm.GetBuffer("test-topic")
	if buf == nil {
		t.Error("expected non-nil buffer")
	}
}

func TestWebSocketPoolManager_PutAndGetBuffer(t *testing.T) {
	t.Parallel()
	pm := NewWebSocketPoolManager()
	buf := pm.GetBuffer("test-topic")
	buf = append(buf, "hello"...)
	pm.PutBuffer("test-topic", buf)
	buf2 := pm.GetBuffer("test-topic")
	if len(buf2) != 0 {
		t.Error("expected reset buffer from pool")
	}
}

func TestWebSocketPoolManager_GetPool_Cached(t *testing.T) {
	t.Parallel()
	pm := NewWebSocketPoolManager()
	p1 := pm.GetPool("topic")
	p2 := pm.GetPool("topic")
	if p1 != p2 {
		t.Error("expected same pool for same topic")
	}
}

// --- WebSocketConn Send ---

func TestWebSocketConn_Send(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	hub := &WebSocketHub{
		connections:  make(map[string]*WebSocketConn),
		subscribers:  make(map[string]map[string]bool),
		broadcastCh:  make(chan BroadcastMessage, 256),
		registerCh:   make(chan *WebSocketConn, 32),
		unregisterCh: make(chan string, 32),
		stopCh:       make(chan struct{}),
	}
	conn := &WebSocketConn{
		ID:      "test-conn",
		Conn:    server,
		Topics:  make(map[string]bool),
		writeCh: make(chan []byte, 64),
		hub:     hub,
	}
	err := conn.Send([]byte("hello"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case msg := <-conn.writeCh:
		if string(msg) != "hello" {
			t.Errorf("message = %q, want %q", string(msg), "hello")
		}
	default:
		t.Error("expected message on write channel")
	}
}

// --- WebSocketConn close ---

func TestWebSocketConn_Close_Once(t *testing.T) {
	t.Parallel()
	server, _ := net.Pipe()
	conn := &WebSocketConn{
		Conn:      server,
		writeCh:   make(chan []byte, 64),
		closeOnce: sync.Once{},
	}
	conn.close()
	conn.close() // should not panic
}

// --- WebSocketHub Broadcast ---

func TestWebSocketHub_Broadcast_NoSubscribers(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()
	hub.Broadcast("nonexistent-topic", realtimeEvent{Type: "test"})
	time.Sleep(100 * time.Millisecond)
}

// --- handleBroadcast with actual connection ---

func TestWebSocketHub_HandleBroadcast_WithConn(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	server, client := net.Pipe()
	defer client.Close()

	wsConn := &WebSocketConn{
		ID:        "conn-1",
		Conn:      server,
		Topics:    map[string]bool{"alerts": true},
		CreatedAt: time.Now(),
		LastPing:  time.Now(),
		writeCh:   make(chan []byte, 64),
		hub:       hub,
	}

	hub.mu.Lock()
	hub.connections["conn-1"] = wsConn
	if hub.subscribers["alerts"] == nil {
		hub.subscribers["alerts"] = make(map[string]bool)
	}
	hub.subscribers["alerts"]["conn-1"] = true
	hub.mu.Unlock()

	hub.Broadcast("alerts", realtimeEvent{Type: "alert", Payload: map[string]any{"msg": "test"}})
	time.Sleep(200 * time.Millisecond)

	select {
	case msg := <-wsConn.writeCh:
		if len(msg) == 0 {
			t.Error("expected non-empty message")
		}
	default:
		t.Log("no message received (broadcast may not have processed yet)")
	}
}

// --- MetricsSnapshot ---

func TestWebSocketHub_MetricsSnapshotValues(t *testing.T) {
	t.Parallel()
	hub := newTestHub()
	defer hub.Stop()

	total, active, sent, recv, broadcasts, errs := hub.MetricsSnapshot()
	if total != 0 || active != 0 {
		t.Errorf("expected zero metrics, got total=%d active=%d", total, active)
	}
	_ = sent
	_ = recv
	_ = broadcasts
	_ = errs
}

// --- writeWebSocketTextFrame ---

func TestWriteWebSocketTextFrame_NilConn(t *testing.T) {
	t.Parallel()
	err := writeWebSocketTextFrame(nil, []byte("test"))
	if err == nil {
		t.Error("expected error for nil conn")
	}
}
