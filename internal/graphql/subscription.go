package graphql

import (
	"bufio"
	"crypto/sha1" // #nosec G505 -- Required by RFC 6455 for WebSocket accept key.
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// wsOpcode constants for WebSocket frame types.
const (
	wsOpText  = 1
	wsOpClose = 8
	wsOpPing  = 9
	wsOpPong  = 10
)

// graphql-ws protocol message types.
const (
	gqlConnectionInit = "connection_init"
	gqlConnectionAck  = "connection_ack"
	gqlPing           = "ping"
	gqlPong           = "pong"
	gqlSubscribe      = "subscribe"
	gqlNext           = "next"
	gqlError          = "error"
	gqlComplete       = "complete"
)

// wsMessage represents a graphql-ws protocol message.
type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// SubscriptionProxy handles GraphQL subscriptions over WebSocket using the
// graphql-ws protocol (https://github.com/enisdenjo/graphql-ws/blob/master/PROTOCOL.md).
type SubscriptionProxy struct {
	upstreamURL string
	client      *http.Client
}

// NewSubscriptionProxy creates a new subscription proxy targeting the given upstream URL.
func NewSubscriptionProxy(upstreamURL string) *SubscriptionProxy {
	return &SubscriptionProxy{
		upstreamURL: upstreamURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HandleSubscription upgrades the incoming HTTP request to a WebSocket connection
// and proxies graphql-ws protocol messages between the client and the upstream.
func (sp *SubscriptionProxy) HandleSubscription(w http.ResponseWriter, r *http.Request) {
	if !isWSUpgrade(r) {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support websocket hijacking", http.StatusInternalServerError)
		return
	}

	// Dial upstream WebSocket.
	upstreamConn, upstreamRW, err := sp.dialUpstream(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to connect to upstream: %v", err), http.StatusBadGateway)
		return
	}

	// Hijack the client connection.
	clientConn, clientRW, err := hijacker.Hijack()
	if err != nil {
		_ = upstreamConn.Close() // #nosec G104
		return
	}

	// Send 101 Switching Protocols to the client.
	resp101 := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + computeAcceptKey(r.Header.Get("Sec-WebSocket-Key")) + "\r\n" +
		"Sec-WebSocket-Protocol: graphql-transport-ws\r\n" +
		"\r\n"
	if _, err := clientRW.WriteString(resp101); err != nil {
		_ = clientConn.Close()   // #nosec G104
		_ = upstreamConn.Close() // #nosec G104
		return
	}
	if err := clientRW.Flush(); err != nil {
		_ = clientConn.Close()   // #nosec G104
		_ = upstreamConn.Close() // #nosec G104
		return
	}

	// Run the bidirectional relay.
	sp.relay(clientConn, clientRW, upstreamConn, upstreamRW)
}

// dialUpstream establishes a WebSocket connection to the upstream server.
func (sp *SubscriptionProxy) dialUpstream(clientReq *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	parsed, err := url.Parse(sp.upstreamURL)
	if err != nil {
		return nil, nil, err
	}

	// Determine dial address.
	host := parsed.Host
	if !strings.Contains(host, ":") {
		switch parsed.Scheme {
		case "wss", "https":
			host += ":443"
		default:
			host += ":80"
		}
	}

	var conn net.Conn
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	switch strings.ToLower(parsed.Scheme) {
	case "wss", "https":
		conn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	default:
		conn, err = dialer.Dial("tcp", host)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("dial upstream: %w", err)
	}

	// Build the HTTP upgrade request for the upstream.
	reqPath := parsed.RequestURI()
	if reqPath == "" {
		reqPath = "/"
	}
	upgradeReq := "GET " + reqPath + " HTTP/1.1\r\n" +
		"Host: " + parsed.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + clientReq.Header.Get("Sec-WebSocket-Key") + "\r\n" +
		"Sec-WebSocket-Protocol: graphql-transport-ws\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(upgradeReq)); err != nil {
		_ = conn.Close() // #nosec G104
		return nil, nil, fmt.Errorf("write upgrade request: %w", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	resp, err := http.ReadResponse(rw.Reader, nil)
	if err != nil {
		_ = conn.Close() // #nosec G104
		return nil, nil, fmt.Errorf("read upstream upgrade response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = resp.Body.Close() // #nosec G104
		_ = conn.Close()      // #nosec G104
		return nil, nil, fmt.Errorf("upstream rejected websocket upgrade: %d", resp.StatusCode)
	}

	return conn, rw, nil
}

// relay runs the bidirectional message relay between client and upstream.
// It handles graphql-ws protocol messages and performs proper cleanup.
func (sp *SubscriptionProxy) relay(clientConn net.Conn, clientRW *bufio.ReadWriter,
	upstreamConn net.Conn, upstreamRW *bufio.ReadWriter) {

	var wg sync.WaitGroup
	done := make(chan struct{})
	var closeOnce sync.Once
	safeClose := func() {
		closeOnce.Do(func() { close(done) })
	}

	// Client -> Upstream relay
	wg.Add(1)
	go func() {
		defer wg.Done()
		sp.relayMessages(clientRW.Reader, upstreamConn, upstreamRW.Writer, done, safeClose)
	}()

	// Upstream -> Client relay
	wg.Add(1)
	go func() {
		defer wg.Done()
		sp.relayMessages(upstreamRW.Reader, clientConn, clientRW.Writer, done, safeClose)
	}()

	// Wait for either direction to finish, then clean up.
	wg.Wait()
	_ = clientConn.Close()   // #nosec G104
	_ = upstreamConn.Close() // #nosec G104
}

// relayMessages reads WebSocket frames from src and writes them to dst.
// It exits when it encounters a close frame, an error, or when done is closed.
func (sp *SubscriptionProxy) relayMessages(src *bufio.Reader, dstConn net.Conn, dstWriter *bufio.Writer, done chan struct{}, safeClose func()) {
	for {
		select {
		case <-done:
			return
		default:
		}

		opcode, payload, err := readWSFrame(src)
		if err != nil {
			// Connection closed or broken; signal done and return.
			safeClose()
			return
		}

		switch opcode {
		case wsOpClose:
			// Forward close frame and exit.
			_ = writeWSFrame(dstWriter, wsOpClose, payload) // #nosec G104
			_ = dstWriter.Flush()                           // #nosec G104
			safeClose()
			return

		case wsOpPing:
			// Respond with pong to the sender.
			_ = writeWSFrame(dstWriter, wsOpPong, payload) // #nosec G104
			_ = dstWriter.Flush()                          // #nosec G104

		case wsOpText:
			// Forward text frames as-is.
			if err := writeWSFrame(dstWriter, wsOpText, payload); err != nil {
				safeClose()
				return
			}
			_ = dstWriter.Flush() // #nosec G104
		}
	}
}

// safeClose is used instead of the global closeDone function.

// IsSubscriptionRequest checks if an HTTP request is a GraphQL subscription
// WebSocket upgrade request.
func IsSubscriptionRequest(r *http.Request) bool {
	return isWSUpgrade(r) && hasGraphQLWSProtocol(r)
}

// IsSubscriptionQuery checks if a parsed GraphQL query is a subscription operation.
func IsSubscriptionQuery(query string) bool {
	ast, err := ParseQuery(query)
	if err != nil {
		return false
	}
	doc, ok := ast.(*Document)
	if !ok {
		return false
	}
	for _, def := range doc.Definitions {
		if op, ok := def.(*Operation); ok && op.Type == "subscription" {
			return true
		}
	}
	return false
}

// --- WebSocket frame helpers (minimal, unmasked server frames) ---

// readWSFrame reads a single WebSocket frame. It handles client-masked frames.
const maxWebSocketFrameSize = 1 << 20 // 1 MB — prevents OOM via oversized frame (CWE-770)

func readWSFrame(r *bufio.Reader) (opcode byte, payload []byte, err error) {
	// First two bytes: FIN+opcode, MASK+payload length.
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	opcode = header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
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

	// Prevent OOM: reject frames exceeding 1 MB
	if length > maxWebSocketFrameSize {
		return 0, nil, fmt.Errorf("websocket frame size %d exceeds maximum %d", length, maxWebSocketFrameSize)
	}

	// RFC 6455: All client-to-server frames MUST be masked.
	// If a client sends an unmasked frame, the server MUST close the connection.
	if !masked {
		return 0, nil, fmt.Errorf("websocket protocol violation: client frame must be masked")
	}

	var maskKey [4]byte
	if masked {
		if _, err = io.ReadFull(r, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload = make([]byte, length)
	if length > 0 {
		if _, err = io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, nil
}

// writeWSFrame writes a single unmasked WebSocket frame (server -> client frames are unmasked).
func writeWSFrame(w *bufio.Writer, opcode byte, payload []byte) error {
	// FIN bit set + opcode
	if err := w.WriteByte(0x80 | opcode); err != nil {
		return err
	}

	length := len(payload)
	switch {
	case length <= 125:
		if err := w.WriteByte(byte(length)); err != nil { // #nosec G115 -- length is guaranteed <= 125 here, safe for byte.
			return err
		}
	case length <= 65535:
		if err := w.WriteByte(126); err != nil {
			return err
		}
		ext := []byte{byte(length >> 8), byte(length)} // #nosec G115 -- length <= 65535 here, low bytes extracted safely.
		if _, err := w.Write(ext); err != nil {
			return err
		}
	default:
		if err := w.WriteByte(127); err != nil {
			return err
		}
		ext := make([]byte, 8)
		for i := 7; i >= 0; i-- {
			ext[i] = byte(length & 0xFF) // #nosec G115 -- extracting low byte, always fits in byte.
			length >>= 8
		}
		if _, err := w.Write(ext); err != nil {
			return err
		}
	}

	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// computeAcceptKey computes the Sec-WebSocket-Accept value per RFC 6455 Section 4.2.2.
func computeAcceptKey(key string) string {
	const websocketGUID = "258EAFA5-E914-47DA-95CA-5AB5DC587FB5"
	h := sha1.New() // #nosec G401 G505 -- Required by RFC 6455 for WebSocket accept key.
	h.Write([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// isWSUpgrade checks if the request has WebSocket upgrade headers.
func isWSUpgrade(r *http.Request) bool {
	if r == nil {
		return false
	}
	hasUpgrade := false
	for _, token := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			hasUpgrade = true
			break
		}
	}
	return hasUpgrade && strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

// hasGraphQLWSProtocol checks if the Sec-WebSocket-Protocol header includes graphql-transport-ws.
func hasGraphQLWSProtocol(r *http.Request) bool {
	for _, proto := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		if strings.TrimSpace(proto) == "graphql-transport-ws" {
			return true
		}
	}
	return false
}
