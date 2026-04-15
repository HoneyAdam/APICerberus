package graphql

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SSESubscriptionProxy relays GraphQL subscriptions to upstream via WebSocket
// and delivers events to the client using Server-Sent Events (text/event-stream).
//
// Protocol:
//
//	Client sends GET or POST with the subscription query.
//	Server responds with Content-Type: text/event-stream and keeps the connection open.
//	Each upstream subscription event is forwarded as an SSE event:
//
//	  event: next
//	  data: {"data":{"..."}}
//
//	  event: complete
//	  data: {}
//
//	Errors are sent as:
//
//	  event: error
//	  data: {"errors":[{"message":"..."}]}
type SSESubscriptionProxy struct {
	upstreamURL string
}

// NewSSESubscriptionProxy creates an SSE subscription proxy for the given upstream.
func NewSSESubscriptionProxy(upstreamURL string) *SSESubscriptionProxy {
	return &SSESubscriptionProxy{upstreamURL: upstreamURL}
}

// HandleSSE handles a GraphQL subscription request over SSE.
func (p *SSESubscriptionProxy) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if p == nil || w == nil || r == nil {
		return
	}

	query, variables, opName, err := parseSSERequest(r)
	if err != nil {
		writeSSEError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if query == "" {
		writeSSEError(w, "missing subscription query", http.StatusBadRequest)
		return
	}

	// Set SSE headers.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Dial upstream WebSocket.
	conn, rw, err := p.dialUpstream(r.Context())
	if err != nil {
		p.writeEvent(w, "error", json.RawMessage(`{"errors":[{"message":"upstream connection failed"}]}`))
		return
	}
	defer conn.Close()

	// Connection init handshake.
	if err := p.initUpstream(rw); err != nil {
		p.writeEvent(w, "error", json.RawMessage(fmt.Sprintf(
			`{"errors":[{"message":"init failed: %s"}]}`, err.Error())))
		return
	}

	// Send subscribe message.
	subPayload, _ := json.Marshal(map[string]any{
		"query":         query,
		"variables":     variables,
		"operationName": opName,
	})
	subMsg := wsMessage{ID: "sse-1", Type: gqlSubscribe, Payload: subPayload}
	if err := writeWSFrame(rw.Writer, wsOpText, mustMarshal(subMsg)); err != nil {
		p.writeEvent(w, "error", json.RawMessage(`{"errors":[{"message":"subscribe failed"}]}`))
		return
	}
	if err := rw.Writer.Flush(); err != nil {
		return
	}

	// Relay upstream WS frames to SSE.
	ctx := r.Context()
	var closeOnce sync.Once
	closeCh := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
		case <-closeCh:
		}
		closeOnce.Do(func() { conn.Close() })
	}()

	for {
		opcode, payload, err := readUnmaskedFrame(rw.Reader)
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				log.Printf("[DEBUG] sse-subscription: upstream read: %v", err)
				p.writeEvent(w, "error", json.RawMessage(`{"errors":[{"message":"upstream disconnected"}]}`))
			}
			return
		}

		switch opcode {
		case wsOpClose:
			return
		case wsOpPing:
			_ = writeWSFrame(rw.Writer, wsOpPong, payload)
			_ = rw.Writer.Flush()
		case wsOpText:
			var wsMsg wsMessage
			if err := json.Unmarshal(payload, &wsMsg); err != nil {
				continue
			}
			switch wsMsg.Type {
			case gqlNext:
				p.writeEvent(w, "next", wsMsg.Payload)
			case gqlError:
				p.writeEvent(w, "error", wsMsg.Payload)
			case gqlComplete:
				p.writeEvent(w, "complete", wsMsg.Payload)
				close(closeCh)
				return
			case gqlConnectionAck:
				// Already handled.
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

func (p *SSESubscriptionProxy) dialUpstream(ctx context.Context) (net.Conn, *bufio.ReadWriter, error) {
	parsed, err := url.Parse(p.upstreamURL)
	if err != nil {
		return nil, nil, err
	}

	wsURL := *parsed
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		wsURL.Scheme = "wss"
	case "http":
		wsURL.Scheme = "ws"
	}

	host := wsURL.Host
	if !strings.Contains(host, ":") {
		switch wsURL.Scheme {
		case "wss":
			host += ":443"
		default:
			host += ":80"
		}
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	switch strings.ToLower(wsURL.Scheme) {
	case "wss":
		conn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	default:
		conn, err = dialer.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("dial upstream: %w", err)
	}

	reqPath := wsURL.RequestURI()
	if reqPath == "" {
		reqPath = "/"
	}
	upgradeReq := "GET " + reqPath + " HTTP/1.1\r\n" +
		"Host: " + wsURL.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Protocol: graphql-transport-ws\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(upgradeReq)); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("write upgrade request: %w", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	resp, err := http.ReadResponse(rw.Reader, nil)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("read upstream upgrade response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("upstream rejected websocket upgrade: %d", resp.StatusCode)
	}

	return conn, rw, nil
}

func (p *SSESubscriptionProxy) initUpstream(rw *bufio.ReadWriter) error {
	initMsg := wsMessage{Type: gqlConnectionInit}
	if err := writeWSFrame(rw.Writer, wsOpText, mustMarshal(initMsg)); err != nil {
		return fmt.Errorf("write init: %w", err)
	}
	if err := rw.Writer.Flush(); err != nil {
		return fmt.Errorf("flush init: %w", err)
	}

	// Read ack with a 10-second timeout via a timer.
	ackCh := make(chan error, 1)
	go func() {
		for {
			opcode, payload, err := readUnmaskedFrame(rw.Reader)
			if err != nil {
				ackCh <- fmt.Errorf("read ack: %w", err)
				return
			}
			switch opcode {
			case wsOpPing:
				_ = writeWSFrame(rw.Writer, wsOpPong, payload)
				_ = rw.Writer.Flush()
				continue
			case wsOpText:
				var msg wsMessage
				if err := json.Unmarshal(payload, &msg); err != nil {
					ackCh <- fmt.Errorf("parse ack: %w", err)
					return
				}
				if msg.Type != gqlConnectionAck {
					ackCh <- fmt.Errorf("expected connection_ack, got %q", msg.Type)
					return
				}
				ackCh <- nil
				return
			case wsOpClose:
				ackCh <- fmt.Errorf("upstream closed during init")
				return
			}
		}
	}()

	select {
	case err := <-ackCh:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timed out waiting for connection_ack")
	}
}

func (p *SSESubscriptionProxy) writeEvent(w io.Writer, event string, data json.RawMessage) {
	if data == nil {
		data = json.RawMessage(`{}`)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

// parseSSERequest extracts the GraphQL query from the HTTP request.
func parseSSERequest(r *http.Request) (query string, variables map[string]any, opName string, err error) {
	switch r.Method {
	case http.MethodGet:
		query = r.URL.Query().Get("query")
		if v := r.URL.Query().Get("variables"); v != "" {
			if jsonErr := json.Unmarshal([]byte(v), &variables); jsonErr != nil {
				return "", nil, "", fmt.Errorf("invalid variables: %w", jsonErr)
			}
		}
		return query, variables, r.URL.Query().Get("operationName"), nil

	case http.MethodPost:
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if readErr != nil {
			return "", nil, "", fmt.Errorf("read body: %w", readErr)
		}
		var req Request
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			return "", nil, "", fmt.Errorf("parse body: %w", jsonErr)
		}
		return req.Query, req.Variables, req.OperationName, nil

	default:
		return "", nil, "", fmt.Errorf("method %s not allowed", r.Method)
	}
}

func writeSSEError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(code)
	fmt.Fprintf(w, "event: error\ndata: {\"errors\":[{\"message\":%q}]}\n\n", message)
}

// IsSSERequest checks if the client is requesting SSE transport for subscriptions.
func IsSSERequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "text/event-stream") {
		return true
	}
	return r.URL.Query().Get("transport") == "sse"
}

// readUnmaskedFrame reads a single unmasked WebSocket frame (server→client).
func readUnmaskedFrame(r *bufio.Reader) (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	opcode = header[0] & 0x0F
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, err
		}
		length = uint64(ext[0])<<8 | uint64(ext[1])
	case 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, err
		}
		length = 0
		for i := 0; i < 8; i++ {
			length = length<<8 | uint64(ext[i])
		}
	}

	if length > maxWebSocketFrameSize {
		return 0, nil, fmt.Errorf("frame size %d exceeds maximum %d", length, maxWebSocketFrameSize)
	}

	payload = make([]byte, length)
	if length > 0 {
		if _, err = io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	return opcode, payload, nil
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}
