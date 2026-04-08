package admin

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type realtimeEvent struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   any `json:"payload"`
}

type realtimeStream struct {
	gateway             *gateway.Gateway
	lastMetricSignature string
	healthSnapshot      map[string]bool
}

func newRealtimeStream(gw *gateway.Gateway) *realtimeStream {
	return &realtimeStream{
		gateway:        gw,
		healthSnapshot: map[string]bool{},
	}
}

func (s *Server) handleRealtimeWebSocket(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketUpgradeRequest(r) {
		writeError(w, http.StatusBadRequest, "invalid_websocket_upgrade", "Request is not a valid WebSocket upgrade")
		return
	}
	// Validate Origin header to prevent CSWSH attacks
	if !s.isValidWebSocketOrigin(r) {
		writeError(w, http.StatusForbidden, "invalid_origin", "Invalid WebSocket origin")
		return
	}
	if !s.isWebSocketAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "admin_unauthorized", "Invalid admin key")
		return
	}

	conn, hijacked, err := upgradeToWebSocket(w, r)
	if err != nil {
		if !hijacked {
			writeError(w, http.StatusInternalServerError, "websocket_upgrade_failed", err.Error())
		}
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		close(done)
	}()

	stream := newRealtimeStream(s.gateway)
	if err := writeRealtimeEvent(conn, realtimeEvent{
		Type:      "connected",
		Timestamp: time.Now().UTC(),
		Payload: map[string]any{
			"message": "realtime stream connected",
		},
	}); err != nil {
		return
	}

	initial := stream.collectEvents(s.snapshotUpstreams())
	for _, event := range initial {
		if err := writeRealtimeEvent(conn, event); err != nil {
			return
		}
	}

	metricTicker := time.NewTicker(time.Second)
	healthTicker := time.NewTicker(250 * time.Millisecond)
	defer metricTicker.Stop()
	defer healthTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-metricTicker.C:
			events := stream.collectRequestMetricEvents()
			for _, event := range events {
				if err := writeRealtimeEvent(conn, event); err != nil {
					return
				}
			}
		case <-healthTicker.C:
			events := stream.collectHealthEvents(s.snapshotUpstreams())
			for _, event := range events {
				if err := writeRealtimeEvent(conn, event); err != nil {
					return
				}
			}
		}
	}
}

func (s *Server) isWebSocketAuthorized(r *http.Request) bool {
	s.mu.RLock()
	cfg := s.cfg.Admin
	s.mu.RUnlock()

	// Allow Bearer token via query parameter (common for WebSocket clients)
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		if err := verifyAdminToken(token, cfg.TokenSecret); err == nil {
			return true
		}
	}
	if token := strings.TrimSpace(r.Header.Get("Authorization")); token != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(token, prefix) {
			if err := verifyAdminToken(token[len(prefix):], cfg.TokenSecret); err == nil {
				return true
			}
		}
	}

	// Fall back to static key
	expected := strings.TrimSpace(cfg.APIKey)
	if expected == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if provided == "" {
		provided = strings.TrimSpace(r.URL.Query().Get("api_key"))
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// isValidWebSocketOrigin validates the Origin header to prevent CSWSH attacks
func (s *Server) isValidWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	// If no Origin header, check Referer as fallback (some clients may not send Origin)
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	// If neither Origin nor Referer, reject for security
	if origin == "" {
		return false
	}

	// Parse the origin URL
	origin = strings.ToLower(strings.TrimSpace(origin))

	// Check if origin matches allowed patterns
	// Allow same-origin requests (empty Origin typically means same-origin)
	if origin == "" || origin == "null" {
		return false
	}

	// Get the admin address host
	s.mu.RLock()
	adminAddr := s.cfg.Admin.Addr
	s.mu.RUnlock()

	// Extract host from admin address
	host := adminAddr
	if idx := strings.LastIndex(adminAddr, ":"); idx != -1 {
		host = adminAddr[:idx]
	}

	// Allow if origin contains the admin host
	if strings.Contains(origin, host) || host == "" || host == "0.0.0.0" || host == "127.0.0.1" {
		// For localhost/127.0.0.1, be more strict - only allow localhost origins
		if host == "127.0.0.1" || host == "localhost" || host == "" || host == "0.0.0.0" {
			return strings.Contains(origin, "localhost") ||
				strings.Contains(origin, "127.0.0.1") ||
				strings.HasPrefix(origin, "https://") // Allow HTTPS origins
		}
		return true
	}

	return false
}

func (s *Server) snapshotUpstreams() []config.Upstream {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.cfg.Upstreams) == 0 {
		return nil
	}
	out := make([]config.Upstream, len(s.cfg.Upstreams))
	for i := range s.cfg.Upstreams {
		out[i] = s.cfg.Upstreams[i]
		out[i].Targets = append([]config.UpstreamTarget(nil), s.cfg.Upstreams[i].Targets...)
	}
	return out
}

func (stream *realtimeStream) collectEvents(upstreams []config.Upstream) []realtimeEvent {
	events := make([]realtimeEvent, 0, 16)
	events = append(events, stream.collectRequestMetricEvents()...)
	events = append(events, stream.collectHealthEvents(upstreams)...)
	return events
}

func (stream *realtimeStream) collectRequestMetricEvents() []realtimeEvent {
	if stream == nil || stream.gateway == nil {
		return nil
	}
	engine := stream.gateway.Analytics()
	if engine == nil {
		return nil
	}

	latest := engine.Latest(32)
	if len(latest) == 0 {
		return nil
	}

	pendingNewestFirst := make([]analytics.RequestMetric, 0, len(latest))
	for _, metric := range latest {
		signature := metricSignature(metric)
		if stream.lastMetricSignature != "" && signature == stream.lastMetricSignature {
			break
		}
		pendingNewestFirst = append(pendingNewestFirst, metric)
	}
	if len(pendingNewestFirst) == 0 {
		return nil
	}

	events := make([]realtimeEvent, 0, len(pendingNewestFirst))
	for i := len(pendingNewestFirst) - 1; i >= 0; i-- {
		metric := pendingNewestFirst[i]
		timestamp := metric.Timestamp.UTC()
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		events = append(events, realtimeEvent{
			Type:      "request_metric",
			Timestamp: timestamp,
			Payload:   metric,
		})
	}

	stream.lastMetricSignature = metricSignature(pendingNewestFirst[0])
	return events
}

func (stream *realtimeStream) collectHealthEvents(upstreams []config.Upstream) []realtimeEvent {
	if stream == nil || stream.gateway == nil {
		return nil
	}

	now := time.Now().UTC()
	events := make([]realtimeEvent, 0, 8)
	nextSnapshot := make(map[string]bool, len(stream.healthSnapshot))

	for _, upstream := range upstreams {
		upstreamID := strings.TrimSpace(upstream.ID)
		upstreamName := strings.TrimSpace(upstream.Name)
		upstreamLookup := upstreamID
		if upstreamLookup == "" {
			upstreamLookup = upstreamName
		}
		if upstreamLookup == "" {
			continue
		}

		state := stream.gateway.UpstreamHealth(upstreamLookup)
		for _, target := range upstream.Targets {
			targetID := strings.TrimSpace(target.ID)
			if targetID == "" {
				continue
			}
			healthy := state[targetID]
			key := upstreamLookup + "::" + targetID

			previous, seen := stream.healthSnapshot[key]
			nextSnapshot[key] = healthy
			if seen && previous == healthy {
				continue
			}

			events = append(events, realtimeEvent{
				Type:      "health_change",
				Timestamp: now,
				Payload: map[string]any{
					"upstream_id":   upstreamID,
					"upstream_name": upstreamName,
					"target_id":     targetID,
					"healthy":       healthy,
				},
			})
		}
	}

	stream.healthSnapshot = nextSnapshot
	return events
}

func metricSignature(metric analytics.RequestMetric) string {
	return fmt.Sprintf("%d|%s|%s|%s|%d|%d|%d",
		metric.Timestamp.UTC().UnixNano(),
		strings.TrimSpace(metric.RouteID),
		strings.TrimSpace(metric.Path),
		strings.TrimSpace(metric.Method),
		metric.StatusCode,
		metric.LatencyMS,
		metric.BytesOut,
	)
}

func writeRealtimeEvent(conn net.Conn, event realtimeEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return writeWebSocketTextFrame(conn, payload)
}

func writeWebSocketTextFrame(conn net.Conn, payload []byte) error {
	if conn == nil {
		return fmt.Errorf("nil websocket connection")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	defer conn.SetWriteDeadline(time.Time{})

	header := []byte{0x81}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, byte(length))
	case length <= 0xFFFF:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127,
			byte(uint64(length)>>56),
			byte(uint64(length)>>48),
			byte(uint64(length)>>40),
			byte(uint64(length)>>32),
			byte(uint64(length)>>24),
			byte(uint64(length)>>16),
			byte(uint64(length)>>8),
			byte(uint64(length)),
		)
	}

	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func isWebSocketUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return headerHasToken(r.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func headerHasToken(raw, token string) bool {
	for _, item := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(item), token) {
			return true
		}
	}
	return false
}

func upgradeToWebSocket(w http.ResponseWriter, r *http.Request) (net.Conn, bool, error) {
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, false, fmt.Errorf("missing websocket key")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, false, fmt.Errorf("response writer does not support hijacking")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, false, err
	}

	accept := websocketAccept(key)
	if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		_ = conn.Close()
		return nil, true, err
	}
	if _, err := rw.WriteString("Upgrade: websocket\r\n"); err != nil {
		_ = conn.Close()
		return nil, true, err
	}
	if _, err := rw.WriteString("Connection: Upgrade\r\n"); err != nil {
		_ = conn.Close()
		return nil, true, err
	}
	if _, err := rw.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n"); err != nil {
		_ = conn.Close()
		return nil, true, err
	}
	if _, err := rw.WriteString("\r\n"); err != nil {
		_ = conn.Close()
		return nil, true, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, true, err
	}

	return conn, true, nil
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID)) // #nosec G505: Required by RFC 6455 for WebSocket accept key
	return base64.StdEncoding.EncodeToString(sum[:])
}
