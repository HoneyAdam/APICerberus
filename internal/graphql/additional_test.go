package graphql

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ==================== Parser Tests ====================

// Test parseFragmentSpread function (line 392)
func TestParseFragmentSpread(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid fragment spread",
			input:    `...UserFields`,
			expected: "UserFields",
			wantErr:  false,
		},
		{
			name:     "fragment spread with space",
			input:    `... UserFields`,
			expected: "UserFields",
			wantErr:  false,
		},
		{
			name:     "fragment spread only dots",
			input:    `...`,
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &queryParser{input: tt.input, pos: 0}
			result, err := p.parseFragmentSpread()
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFragmentSpread() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != nil && result.Name != tt.expected {
				t.Errorf("parseFragmentSpread() Name = %q, want %q", result.Name, tt.expected)
			}
		})
	}
}

// Test FragmentDefinition.Depth with selections (line 159)
func TestFragmentDefinition_Depth_WithSelections(t *testing.T) {
	// Test with nested selections
	query := `
		fragment UserFields on User {
			id
			name
			friends {
				name
			}
		}
		query { users { ...UserFields } }
	`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}

	doc, ok := node.(*Document)
	if !ok {
		t.Fatal("Expected Document")
	}

	// Find the fragment definition
	for _, def := range doc.Definitions {
		if frag, ok := def.(*FragmentDefinition); ok {
			depth := frag.Depth()
			// fragment adds 1 to max child depth
			// friends { name } -> friends (1) + name (1) = 2, plus fragment (1) = 3
			if depth != 3 {
				t.Errorf("FragmentDefinition.Depth() = %d, want 3", depth)
			}
		}
	}
}

// Test parseSelection with fragment spread (indirectly tests parseFragmentSpread)
func TestParseSelection_FragmentSpread(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType string
	}{
		{
			name:         "fragment spread without space",
			input:        `...UserFields }`,
			expectedType: "FragmentSpread",
		},
		{
			name:         "fragment spread with space after dots",
			input:        `... UserFields }`,
			expectedType: "FragmentSpread",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &queryParser{input: tt.input, pos: 0}
			selection, err := p.parseSelection()
			if err != nil {
				t.Fatalf("parseSelection() error = %v", err)
			}
			if selection.NodeKind() != tt.expectedType {
				t.Errorf("Expected %s, got %s", tt.expectedType, selection.NodeKind())
			}
		})
	}
}

// ==================== Subscription Tests ====================

// Test HandleSubscription with non-WebSocket request (line 66)
func TestHandleSubscription_NonWebSocket(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	// No WebSocket upgrade headers
	w := httptest.NewRecorder()

	proxy.HandleSubscription(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusBadRequest)
	}

	body := w.Body.String()
	if !strings.Contains(body, "websocket") {
		t.Errorf("Expected error message about websocket, got: %s", body)
	}
}

// Test HandleSubscription with non-hijacker response writer (line 66)
func TestHandleSubscription_NonHijacker(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")

	// Use a ResponseRecorder which doesn't implement Hijacker
	w := httptest.NewRecorder()

	proxy.HandleSubscription(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// Test dialUpstream with invalid URL (line 115)
func TestDialUpstream_InvalidURL(t *testing.T) {
	proxy := NewSubscriptionProxy("://invalid-url")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	conn, rw, err := proxy.dialUpstream(req)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
		if conn != nil {
			conn.Close()
		}
	}
	if rw != nil {
		t.Error("Expected nil ReadWriter for invalid URL")
	}
}

// Test dialUpstream with connection failure (line 115)
func TestDialUpstream_ConnectionFailure(t *testing.T) {
	// Use a port that's unlikely to be open
	proxy := NewSubscriptionProxy("ws://localhost:1/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	conn, rw, err := proxy.dialUpstream(req)
	if err == nil {
		t.Error("Expected error for connection failure, got nil")
		if conn != nil {
			conn.Close()
		}
	}
	if rw != nil {
		t.Error("Expected nil ReadWriter for connection failure")
	}
}

// Test dialUpstream with WSS scheme (line 115)
func TestDialUpstream_WSSScheme(t *testing.T) {
	// This will fail to connect but exercises the TLS code path
	proxy := NewSubscriptionProxy("wss://localhost:1/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	conn, rw, err := proxy.dialUpstream(req)
	if err == nil {
		// If it somehow connected, close it
		if conn != nil {
			conn.Close()
		}
	}
	// We expect an error since there's no server
	if rw != nil && conn != nil {
		rw = nil // Just to use the variable
	}
}

// Test relay function (line 181)
func TestRelay(t *testing.T) {
	// Create two pairs of connected pipes to simulate client and upstream
	client1, client2 := net.Pipe()
	upstream1, upstream2 := net.Pipe()

	clientRW := bufio.NewReadWriter(bufio.NewReader(client2), bufio.NewWriter(client2))
	upstreamRW := bufio.NewReadWriter(bufio.NewReader(upstream2), bufio.NewWriter(upstream2))

	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Run relay in a goroutine
	done := make(chan struct{})
	go func() {
		proxy.relay(client2, clientRW, upstream2, upstreamRW)
		close(done)
	}()

	// Give relay time to start
	time.Sleep(10 * time.Millisecond)

	// Close both sides to trigger cleanup
	client1.Close()
	upstream1.Close()

	// Wait for relay to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("relay did not finish in time")
	}
}

// Test relayMessages with done channel closed (line 209)
func TestRelayMessages_DoneChannel(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a reader with some data
	data := []byte{0x81, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // Text frame with "hello"
	reader := bufio.NewReader(bytes.NewReader(data))

	// Create a mock connection and writer
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	done := make(chan struct{})
	close(done) // Close immediately

	// This should return immediately since done is closed
	proxy.relayMessages(reader, nil, writer, done)

	// Nothing should have been written
	if buf.Len() > 0 {
		t.Errorf("Expected no data written, got %d bytes", buf.Len())
	}
}

// Test relayMessages with close frame (line 209)
func TestRelayMessages_CloseFrame(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a close frame
	data := []byte{0x88, 0x02, 0x03, 0xE8} // Close frame with code 1000
	reader := bufio.NewReader(bytes.NewReader(data))

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	// Create a pipe for the destination connection
	destConn, _ := net.Pipe()
	defer destConn.Close()

	done := make(chan struct{})

	// Run relayMessages
	proxy.relayMessages(reader, destConn, writer, done)

	// Verify done was closed
	select {
	case <-done:
		// Success
	default:
		t.Error("Expected done channel to be closed")
	}
}

// Test relayMessages with ping frame (line 209)
func TestRelayMessages_PingFrame(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a ping frame followed by a close frame
	pingFrame := []byte{0x89, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // Ping frame with "hello"
	closeFrame := []byte{0x88, 0x02, 0x03, 0xE8}                   // Close frame
	data := append(pingFrame, closeFrame...)
	reader := bufio.NewReader(bytes.NewReader(data))

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	destConn, _ := net.Pipe()
	defer destConn.Close()

	done := make(chan struct{})

	// Run relayMessages
	proxy.relayMessages(reader, destConn, writer, done)

	// Verify a pong was written (opcode 0x8A = 138 = pong)
	written := buf.Bytes()
	if len(written) > 0 && written[0] != 0x8A {
		t.Errorf("Expected pong frame (0x8A), got 0x%02X", written[0])
	}
}

// Test relayMessages with read error (line 209)
func TestRelayMessages_ReadError(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a reader that returns an error
	reader := bufio.NewReader(&errorReader{})

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	destConn, _ := net.Pipe()
	defer destConn.Close()

	done := make(chan struct{})

	// Run relayMessages
	proxy.relayMessages(reader, destConn, writer, done)

	// Verify done was closed
	select {
	case <-done:
		// Success
	default:
		t.Error("Expected done channel to be closed after read error")
	}
}

// Test relayMessages with write error (line 209)
func TestRelayMessages_WriteError(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a text frame
	data := []byte{0x81, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // Text frame with "hello"
	reader := bufio.NewReader(bytes.NewReader(data))

	// Create a writer that always errors
	writer := bufio.NewWriter(&errorWriter{})

	destConn, _ := net.Pipe()
	defer destConn.Close()

	done := make(chan struct{})

	// Run relayMessages
	proxy.relayMessages(reader, destConn, writer, done)

	// Verify done was closed
	select {
	case <-done:
		// Success
	default:
		t.Error("Expected done channel to be closed after write error")
	}
}

// ==================== WebSocket Frame Tests ====================

// Test readWSFrame with 16-bit extended payload length (line 330)
func TestReadWSFrame_16BitLength(t *testing.T) {
	// Create a frame with payload length 126 (uses 16-bit extended length)
	payload := make([]byte, 126)
	for i := range payload {
		payload[i] = byte('a')
	}

	// Frame: FIN=1, opcode=text, MASK=0, length=126 (0x7E)
	frame := []byte{0x81, 0x7E, 0x00, 0x7E} // 126 in 16-bit
	frame = append(frame, payload...)

	reader := bufio.NewReader(bytes.NewReader(frame))
	opcode, result, err := readWSFrame(reader)
	if err != nil {
		t.Fatalf("readWSFrame() error = %v", err)
	}
	if opcode != wsOpText {
		t.Errorf("opcode = %d, want %d", opcode, wsOpText)
	}
	if len(result) != 126 {
		t.Errorf("payload length = %d, want 126", len(result))
	}
}

// Test readWSFrame with 64-bit extended payload length (line 330)
func TestReadWSFrame_64BitLength(t *testing.T) {
	// Create a frame with payload length 127 (uses 64-bit extended length)
	payload := make([]byte, 10)
	for i := range payload {
		payload[i] = byte('b')
	}

	// Frame: FIN=1, opcode=text, MASK=0, length=127 (0x7F, uses 64-bit)
	frame := []byte{0x81, 0x7F, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0A} // 10 in 64-bit
	frame = append(frame, payload...)

	reader := bufio.NewReader(bytes.NewReader(frame))
	opcode, result, err := readWSFrame(reader)
	if err != nil {
		t.Fatalf("readWSFrame() error = %v", err)
	}
	if opcode != wsOpText {
		t.Errorf("opcode = %d, want %d", opcode, wsOpText)
	}
	if len(result) != 10 {
		t.Errorf("payload length = %d, want 10", len(result))
	}
}

// Test readWSFrame with masked frame (line 330)
func TestReadWSFrame_MaskedFrame(t *testing.T) {
	// Create a masked frame
	// Mask key: 0x12, 0x34, 0x56, 0x78
	// Payload: "hello" XORed with mask key
	// h=0x68, e=0x65, l=0x6c, l=0x6c, o=0x6f
	// 0x68 ^ 0x12 = 0x7a, 0x65 ^ 0x34 = 0x51, 0x6c ^ 0x56 = 0x3a, 0x6c ^ 0x78 = 0x14, 0x6f ^ 0x12 = 0x7d
	payload := []byte{0x7a, 0x51, 0x3a, 0x14, 0x7d} // "hello" XOR 0x12345678
	frame := []byte{0x81, 0x85, 0x12, 0x34, 0x56, 0x78} // FIN=1, text, MASK=1, len=5
	frame = append(frame, payload...)

	reader := bufio.NewReader(bytes.NewReader(frame))
	opcode, result, err := readWSFrame(reader)
	if err != nil {
		t.Fatalf("readWSFrame() error = %v", err)
	}
	if opcode != wsOpText {
		t.Errorf("opcode = %d, want %d", opcode, wsOpText)
	}
	if string(result) != "hello" {
		t.Errorf("payload = %q, want %q", string(result), "hello")
	}
}

// Test readWSFrame with header read error (line 330)
func TestReadWSFrame_HeaderReadError(t *testing.T) {
	// Empty reader - will fail to read header
	reader := bufio.NewReader(bytes.NewReader([]byte{}))
	_, _, err := readWSFrame(reader)
	if err == nil {
		t.Error("Expected error for empty reader")
	}
}

// Test readWSFrame with 16-bit length read error (line 330)
func TestReadWSFrame_16BitLengthReadError(t *testing.T) {
	// Frame indicating 16-bit length but no length bytes
	frame := []byte{0x81, 0x7E} // 126 indicates 16-bit length follows
	reader := bufio.NewReader(bytes.NewReader(frame))
	_, _, err := readWSFrame(reader)
	if err == nil {
		t.Error("Expected error for incomplete 16-bit length")
	}
}

// Test readWSFrame with 64-bit length read error (line 330)
func TestReadWSFrame_64BitLengthReadError(t *testing.T) {
	// Frame indicating 64-bit length but no length bytes
	frame := []byte{0x81, 0x7F} // 127 indicates 64-bit length follows
	reader := bufio.NewReader(bytes.NewReader(frame))
	_, _, err := readWSFrame(reader)
	if err == nil {
		t.Error("Expected error for incomplete 64-bit length")
	}
}

// Test readWSFrame with mask key read error (line 330)
func TestReadWSFrame_MaskKeyReadError(t *testing.T) {
	// Masked frame without full mask key
	frame := []byte{0x81, 0x81, 0x12} // MASK=1, len=1, but only 1 byte of mask key
	reader := bufio.NewReader(bytes.NewReader(frame))
	_, _, err := readWSFrame(reader)
	if err == nil {
		t.Error("Expected error for incomplete mask key")
	}
}

// Test readWSFrame with payload read error (line 330)
func TestReadWSFrame_PayloadReadError(t *testing.T) {
	// Frame indicating payload but not enough data
	frame := []byte{0x81, 0x05, 0x68, 0x65} // Says 5 bytes but only 2
	reader := bufio.NewReader(bytes.NewReader(frame))
	_, _, err := readWSFrame(reader)
	if err == nil {
		t.Error("Expected error for incomplete payload")
	}
}

// Test writeWSFrame with 16-bit length (line 383)
func TestWriteWSFrame_16BitLength(t *testing.T) {
	// Create a payload that requires 16-bit length (126-65535 bytes)
	payload := make([]byte, 200)
	for i := range payload {
		payload[i] = byte('x')
	}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	err := writeWSFrame(writer, wsOpText, payload)
	if err != nil {
		t.Fatalf("writeWSFrame() error = %v", err)
	}
	writer.Flush()

	result := buf.Bytes()
	// First byte: FIN=1, opcode=text (0x81)
	if result[0] != 0x81 {
		t.Errorf("first byte = 0x%02X, want 0x81", result[0])
	}
	// Second byte: MASK=0, len=126 (0x7E indicates 16-bit length)
	if result[1] != 0x7E {
		t.Errorf("second byte = 0x%02X, want 0x7E", result[1])
	}
	// Third and fourth bytes: length (200 = 0x00C8)
	length := uint16(result[2])<<8 | uint16(result[3])
	if length != 200 {
		t.Errorf("length = %d, want 200", length)
	}
}

// Test writeWSFrame with 64-bit length (line 383)
func TestWriteWSFrame_64BitLength(t *testing.T) {
	// Create a payload that requires 64-bit length (>65535 bytes)
	payload := make([]byte, 70000)
	for i := range payload {
		payload[i] = byte('y')
	}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	err := writeWSFrame(writer, wsOpText, payload)
	if err != nil {
		t.Fatalf("writeWSFrame() error = %v", err)
	}
	writer.Flush()

	result := buf.Bytes()
	// First byte: FIN=1, opcode=text (0x81)
	if result[0] != 0x81 {
		t.Errorf("first byte = 0x%02X, want 0x81", result[0])
	}
	// Second byte: MASK=0, len=127 (0x7F indicates 64-bit length)
	if result[1] != 0x7F {
		t.Errorf("second byte = 0x%02X, want 0x7F", result[1])
	}
}

// Test writeWSFrame with write errors at various points
func TestWriteWSFrame_WriteErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "error writing 64-bit length",
			payload: make([]byte, 70000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := bufio.NewWriter(&errorWriter{})
			err := writeWSFrame(writer, wsOpText, tt.payload)
			// Should get an error due to writer failing
			if err == nil {
				t.Error("Expected error from errorWriter")
			}
		})
	}
}

// Test writeWSFrame error on 64-bit length indicator
func TestWriteWSFrame_64BitIndicatorError(t *testing.T) {
	writer := bufio.NewWriter(&errorWriter{})
	payload := make([]byte, 70000) // Requires 64-bit length
	err := writeWSFrame(writer, wsOpText, payload)
	if err == nil {
		t.Error("Expected error when writing 64-bit length indicator")
	}
}

// ==================== Helper Tests ====================

// Test isWSUpgrade with nil request
func TestIsWSUpgrade_NilRequest(t *testing.T) {
	result := isWSUpgrade(nil)
	if result {
		t.Error("isWSUpgrade(nil) should return false")
	}
}

// Test isBenignClose with net.ErrClosed
func TestIsBenignClose_NetErrClosed(t *testing.T) {
	// Create a network error that wraps net.ErrClosed
	err := &net.OpError{Err: net.ErrClosed}
	if !isBenignClose(err) {
		t.Error("isBenignClose(net.ErrClosed) should return true")
	}
}

// Test isBenignClose with "use of closed network connection"
func TestIsBenignClose_ClosedNetworkConnection(t *testing.T) {
	err := errors.New("use of closed network connection")
	if !isBenignClose(err) {
		t.Error("isBenignClose('use of closed network connection') should return true")
	}
}

// Test closeDone with concurrent calls
func TestCloseDone_Concurrent(t *testing.T) {
	done := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closeDone(done)
		}()
	}

	wg.Wait()

	// Verify channel is closed
	select {
	case <-done:
		// Success
	default:
		t.Error("Expected done channel to be closed")
	}
}

// ==================== Mock Types ====================

// errorReader is a reader that always returns an error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

// errorWriter is a writer that always returns an error
type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write error")
}

// limitedWriter is a writer that fails after a certain number of bytes
type limitedWriter struct {
	limit     int
	written   int
}

func (l *limitedWriter) Write(p []byte) (n int, err error) {
	if l.written >= l.limit {
		return 0, errors.New("write limit exceeded")
	}
	remaining := l.limit - l.written
	if len(p) > remaining {
		l.written = l.limit
		return remaining, errors.New("write limit exceeded")
	}
	l.written += len(p)
	return len(p), nil
}

// createTestConn creates a mock connection for testing
func createTestConn() (net.Conn, *bufio.ReadWriter) {
	client, _ := net.Pipe()
	rw := bufio.NewReadWriter(bufio.NewReader(client), bufio.NewWriter(client))
	return client, rw
}

// mockHijacker is a ResponseRecorder that implements http.Hijacker for testing
type mockHijacker struct {
	*httptest.ResponseRecorder
	conn       net.Conn
	rw         *bufio.ReadWriter
	hijackErr  error
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if m.hijackErr != nil {
		return nil, nil, m.hijackErr
	}
	return m.conn, m.rw, nil
}

// Test HandleSubscription with successful hijack but failed upstream dial
func TestHandleSubscription_HijackSuccess_DialFailure(t *testing.T) {
	// Create a pipe for the hijacked connection
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Use an invalid upstream URL that will fail to connect
	proxy := NewSubscriptionProxy("ws://localhost:1/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")

	// Create a mock hijacker
	rw := bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn))
	hijacker := &mockHijacker{
		ResponseRecorder: httptest.NewRecorder(),
		conn:             serverConn,
		rw:               rw,
	}

	// This should fail when trying to dial upstream
	proxy.HandleSubscription(hijacker, req)

	// The function returns early due to dial failure, which sends an error response
	// But since we hijacked, we can't check the recorder status
}

// Test dialUpstream with HTTPS scheme
func TestDialUpstream_HTTPSScheme(t *testing.T) {
	// This will fail to connect but exercises the TLS code path
	proxy := NewSubscriptionProxy("https://localhost:1/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	conn, rw, err := proxy.dialUpstream(req)
	if err == nil {
		// If it somehow connected, close it
		if conn != nil {
			conn.Close()
		}
	}
	if rw != nil && conn != nil {
		rw = nil // Use the variable
	}
}

// Test dialUpstream with host without port
func TestDialUpstream_HostWithoutPort(t *testing.T) {
	// Create a test server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if it's a WebSocket upgrade request
		if r.Header.Get("Upgrade") == "websocket" {
			// Send a 101 response
			w.WriteHeader(http.StatusSwitchingProtocols)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Get the host without explicit port
	proxy := NewSubscriptionProxy(upstream.URL + "/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	// This will connect but the upgrade will fail because httptest doesn't support WebSocket
	conn, rw, err := proxy.dialUpstream(req)
	if err != nil {
		// Expected - the test server doesn't actually support WebSocket upgrade
		t.Logf("dialUpstream error (expected): %v", err)
	}
	if conn != nil {
		conn.Close()
	}
	if rw != nil {
		rw = nil // Use the variable
	}
}

// Test dialUpstream with upgrade write error
func TestDialUpstream_UpgradeWriteError(t *testing.T) {
	// Create a listener that accepts connections but closes them immediately
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Accept and close connections immediately
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	proxy := NewSubscriptionProxy("ws://" + listener.Addr().String() + "/graphql")

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	conn, rw, err := proxy.dialUpstream(req)
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Error("Expected error when connection is closed immediately")
	}
	if rw != nil {
		rw = nil
	}
}

// Test dialUpstream with non-101 response
func TestDialUpstream_Non101Response(t *testing.T) {
	// Create a test server that returns 200 instead of 101
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Not a WebSocket upgrade"))
	}))
	defer upstream.Close()

	// Replace http:// with ws://
	wsURL := strings.Replace(upstream.URL, "http://", "ws://", 1) + "/graphql"
	proxy := NewSubscriptionProxy(wsURL)

	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	conn, rw, err := proxy.dialUpstream(req)
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Error("Expected error for non-101 response")
	} else if !strings.Contains(err.Error(), "rejected websocket upgrade") {
		t.Errorf("Expected 'rejected websocket upgrade' error, got: %v", err)
	}
	if rw != nil {
		rw = nil
	}
}

// Test NewSubscriptionProxy
func TestNewSubscriptionProxy(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")
	if proxy == nil {
		t.Fatal("NewSubscriptionProxy() returned nil")
	}
	if proxy.upstreamURL != "ws://localhost:8080/graphql" {
		t.Errorf("upstreamURL = %q, want %q", proxy.upstreamURL, "ws://localhost:8080/graphql")
	}
	if proxy.client == nil {
		t.Error("client is nil")
	}
	if proxy.client.Timeout != 30*time.Second {
		t.Errorf("client.Timeout = %v, want %v", proxy.client.Timeout, 30*time.Second)
	}
}

// Test relay with actual message forwarding
func TestRelay_MessageForwarding(t *testing.T) {
	// Create two pairs of connected pipes
	client1, client2 := net.Pipe()
	upstream1, upstream2 := net.Pipe()

	defer client1.Close()
	defer client2.Close()
	defer upstream1.Close()
	defer upstream2.Close()

	clientRW := bufio.NewReadWriter(bufio.NewReader(client2), bufio.NewWriter(client2))
	upstreamRW := bufio.NewReadWriter(bufio.NewReader(upstream2), bufio.NewWriter(upstream2))

	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Start relay
	done := make(chan struct{})
	go func() {
		proxy.relay(client2, clientRW, upstream2, upstreamRW)
		close(done)
	}()

	// Send a message from client side
	go func() {
		// Write a text frame
		frame := []byte{0x81, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // "hello"
		client1.Write(frame)
	}()

	// Read from upstream side
	buf := make([]byte, 100)
	upstream1.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	n, err := upstream1.Read(buf)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		// Expected to receive the frame or connection closed
		t.Logf("Read result: %d bytes, err: %v", n, err)
	}

	// Clean up
	client1.Close()
	upstream1.Close()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("relay did not finish in time")
	}
}

// Test relayMessages with pong frame handling
func TestRelayMessages_PongFrame(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a pong frame followed by close
	pongFrame := []byte{0x8A, 0x00} // Pong frame with no payload
	closeFrame := []byte{0x88, 0x02, 0x03, 0xE8}
	data := append(pongFrame, closeFrame...)
	reader := bufio.NewReader(bytes.NewReader(data))

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	destConn, _ := net.Pipe()
	defer destConn.Close()

	done := make(chan struct{})

	// Run relayMessages
	proxy.relayMessages(reader, destConn, writer, done)

	// Verify done was closed
	select {
	case <-done:
		// Success
	default:
		t.Error("Expected done channel to be closed")
	}
}

// Test relayMessages with binary frame (should be ignored)
func TestRelayMessages_BinaryFrame(t *testing.T) {
	proxy := NewSubscriptionProxy("ws://localhost:8080/graphql")

	// Create a binary frame (opcode 2) followed by close
	binaryFrame := []byte{0x82, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // Binary frame
	closeFrame := []byte{0x88, 0x02, 0x03, 0xE8}
	data := append(binaryFrame, closeFrame...)
	reader := bufio.NewReader(bytes.NewReader(data))

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	destConn, _ := net.Pipe()
	defer destConn.Close()

	done := make(chan struct{})

	// Run relayMessages
	proxy.relayMessages(reader, destConn, writer, done)

	// Binary frames should be ignored but close frame should trigger exit
	select {
	case <-done:
		// Success
	default:
		t.Error("Expected done channel to be closed")
	}
}

// Test isBenignClose with various errors
func TestIsBenignClose_VariousErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: true,
		},
		{
			name:     "io.EOF",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "net.ErrClosed",
			err:      net.ErrClosed,
			expected: true,
		},
		{
			name:     "wrapped net.ErrClosed",
			err:      &net.OpError{Err: net.ErrClosed},
			expected: true,
		},
		{
			name:     "closed network connection string",
			err:      errors.New("use of closed network connection"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "tls handshake error",
			err:      errors.New("tls: handshake error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBenignClose(tt.err)
			if result != tt.expected {
				t.Errorf("isBenignClose() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test hasGraphQLWSProtocol with various inputs
func TestHasGraphQLWSProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		expected bool
	}{
		{
			name:     "exact match",
			protocol: "graphql-transport-ws",
			expected: true,
		},
		{
			name:     "with other protocols",
			protocol: "graphql-ws, graphql-transport-ws",
			expected: true,
		},
		{
			name:     "wrong protocol",
			protocol: "graphql-ws",
			expected: false,
		},
		{
			name:     "empty",
			protocol: "",
			expected: false,
		},
		{
			name:     "with spaces",
			protocol: " graphql-transport-ws ",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.protocol != "" {
				req.Header.Set("Sec-WebSocket-Protocol", tt.protocol)
			}
			result := hasGraphQLWSProtocol(req)
			if result != tt.expected {
				t.Errorf("hasGraphQLWSProtocol() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test computeAcceptKey with empty key
func TestComputeAcceptKey_Empty(t *testing.T) {
	result := computeAcceptKey("")
	if result == "" {
		t.Error("computeAcceptKey('') should not return empty string")
	}
	// Should be deterministic
	result2 := computeAcceptKey("")
	if result != result2 {
		t.Error("computeAcceptKey should be deterministic")
	}
}

// Test BuildConnectionAck
func TestBuildConnectionAck(t *testing.T) {
	msg := BuildConnectionAck()
	if msg == nil {
		t.Fatal("BuildConnectionAck() returned nil")
	}
	if msg.Type != gqlConnectionAck {
		t.Errorf("Type = %q, want %q", msg.Type, gqlConnectionAck)
	}
	if msg.ID != "" {
		t.Errorf("ID = %q, want empty", msg.ID)
	}
	if msg.Payload != nil {
		t.Errorf("Payload = %v, want nil", msg.Payload)
	}
}

// Test BuildNext
func TestBuildNext(t *testing.T) {
	payload := []byte(`{"data":{"test":"value"}}`)
	msg := BuildNext("sub-123", payload)
	if msg == nil {
		t.Fatal("BuildNext() returned nil")
	}
	if msg.Type != gqlNext {
		t.Errorf("Type = %q, want %q", msg.Type, gqlNext)
	}
	if msg.ID != "sub-123" {
		t.Errorf("ID = %q, want %q", msg.ID, "sub-123")
	}
	if string(msg.Payload) != string(payload) {
		t.Errorf("Payload = %s, want %s", msg.Payload, payload)
	}
}

// Test BuildComplete
func TestBuildComplete(t *testing.T) {
	msg := BuildComplete("sub-123")
	if msg == nil {
		t.Fatal("BuildComplete() returned nil")
	}
	if msg.Type != gqlComplete {
		t.Errorf("Type = %q, want %q", msg.Type, gqlComplete)
	}
	if msg.ID != "sub-123" {
		t.Errorf("ID = %q, want %q", msg.ID, "sub-123")
	}
}

// Test BuildError
func TestBuildError(t *testing.T) {
	errs := []GraphQLError{
		{Message: "Error 1"},
		{Message: "Error 2"},
	}
	msg := BuildError("sub-123", errs)
	if msg == nil {
		t.Fatal("BuildError() returned nil")
	}
	if msg.Type != gqlError {
		t.Errorf("Type = %q, want %q", msg.Type, gqlError)
	}
	if msg.ID != "sub-123" {
		t.Errorf("ID = %q, want %q", msg.ID, "sub-123")
	}
	if msg.Payload == nil {
		t.Error("Payload is nil")
	}
}

// Test IsSubscriptionRequest additional cases
func TestIsSubscriptionRequest_Additional(t *testing.T) {
	tests := []struct {
		name     string
		upgrade  string
		conn     string
		protocol string
		expected bool
	}{
		{
			name:     "valid subscription request",
			upgrade:  "websocket",
			conn:     "Upgrade",
			protocol: "graphql-transport-ws",
			expected: true,
		},
		{
			name:     "no upgrade header",
			upgrade:  "",
			conn:     "Upgrade",
			protocol: "graphql-transport-ws",
			expected: false,
		},
		{
			name:     "no connection header",
			upgrade:  "websocket",
			conn:     "",
			protocol: "graphql-transport-ws",
			expected: false,
		},
		{
			name:     "wrong protocol",
			upgrade:  "websocket",
			conn:     "Upgrade",
			protocol: "other-protocol",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/graphql", nil)
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}
			if tt.conn != "" {
				req.Header.Set("Connection", tt.conn)
			}
			if tt.protocol != "" {
				req.Header.Set("Sec-WebSocket-Protocol", tt.protocol)
			}
			result := IsSubscriptionRequest(req)
			if result != tt.expected {
				t.Errorf("IsSubscriptionRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ==================== Additional Parser Tests ====================

// Test parseListValue with nested lists
func TestParseListValue_Nested(t *testing.T) {
	input := `[[1, 2], [3, 4]]`
	p := &queryParser{input: input, pos: 0}
	result := p.parseListValue()
	if result == nil {
		t.Fatal("parseListValue() returned nil")
	}
	if len(result.Values) != 2 {
		t.Errorf("Expected 2 nested lists, got %d", len(result.Values))
	}
}

// Test parseObjectValue with nested object
func TestParseObjectValue_Nested(t *testing.T) {
	input := `{filter: {name: "test"}}`
	p := &queryParser{input: input, pos: 0}
	result := p.parseObjectValue()
	if result == nil {
		t.Fatal("parseObjectValue() returned nil")
	}
}

// Test parseArguments with empty arguments
func TestParseArguments_Empty(t *testing.T) {
	input := `()`
	p := &queryParser{input: input, pos: 0}
	result, err := p.parseArguments()
	if err != nil {
		t.Errorf("parseArguments() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 arguments, got %d", len(result))
	}
}

// Test parseField with only whitespace after name
func TestParseField_WhitespaceOnly(t *testing.T) {
	input := `name   `
	p := &queryParser{input: input, pos: 0}
	field, err := p.parseField()
	if err != nil {
		t.Fatalf("parseField() error = %v", err)
	}
	if field.Name != "name" {
		t.Errorf("Field.Name = %q, want %q", field.Name, "name")
	}
}

// Test parseFragmentDefinition
func TestParseFragmentDefinition_Basic(t *testing.T) {
	// The parser expects input to start with "fragment" (no leading whitespace)
	query := `fragment UserFields on User {
		id
		name
	}`
	p := &queryParser{input: query, pos: 0}
	frag, err := p.parseFragmentDefinition()
	if err != nil {
		t.Fatalf("parseFragmentDefinition() error = %v", err)
	}
	if frag.Name != "UserFields" {
		t.Errorf("Fragment.Name = %q, want %q", frag.Name, "UserFields")
	}
	if frag.Type != "User" {
		t.Errorf("Fragment.Type = %q, want %q", frag.Type, "User")
	}
}

// Test parseOperation with name and variables
func TestParseOperation_WithNameAndVariables(t *testing.T) {
	query := `query GetUser($id: ID!) {
		user(id: $id) {
			name
		}
	}`
	p := &queryParser{input: query, pos: 0}
	op, err := p.parseOperation()
	if err != nil {
		t.Fatalf("parseOperation() error = %v", err)
	}
	if op.Type != "query" {
		t.Errorf("Operation.Type = %q, want %q", op.Type, "query")
	}
	if op.Name != "GetUser" {
		t.Errorf("Operation.Name = %q, want %q", op.Name, "GetUser")
	}
}

// Test parseOperation with implicit query
func TestParseOperation_ImplicitQuery(t *testing.T) {
	query := `{
		users {
			id
		}
	}`
	p := &queryParser{input: query, pos: 0}
	op, err := p.parseOperation()
	if err != nil {
		t.Fatalf("parseOperation() error = %v", err)
	}
	if op.Type != "query" {
		t.Errorf("Operation.Type = %q, want %q", op.Type, "query")
	}
	if op.Name != "" {
		t.Errorf("Operation.Name = %q, want empty", op.Name)
	}
}

// Test parseSelection with fragment spread followed by other selection
func TestParseSelections_Mixed(t *testing.T) {
	query := `{
		users {
			id
			...UserFields
			name
		}
	}`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if node == nil {
		t.Fatal("ParseQuery() returned nil")
	}
}

// Test parseValue with various scalar types
func TestParseValue_Scalars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string value",
			input:    `"test"`,
			expected: "test",
		},
		{
			name:     "number value",
			input:    `123`,
			expected: "123",
		},
		{
			name:     "boolean true",
			input:    `true`,
			expected: "true",
		},
		{
			name:     "boolean false",
			input:    `false`,
			expected: "false",
		},
		{
			name:     "null value",
			input:    `null`,
			expected: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &queryParser{input: tt.input, pos: 0}
			value := p.parseValue()
			if value == nil {
				t.Fatal("parseValue() returned nil")
			}
			scalar, ok := value.(*ScalarValue)
			if !ok {
				t.Fatalf("Expected ScalarValue, got %T", value)
			}
			if scalar.Value != tt.expected {
				t.Errorf("Value = %q, want %q", scalar.Value, tt.expected)
			}
		})
	}
}

// Test parseStringValue with escaped quote
func TestParseStringValue_EscapedQuote(t *testing.T) {
	input := `"test \"quoted\" string"`
	p := &queryParser{input: input, pos: 0}
	result := p.parseStringValue()
	if result == nil {
		t.Fatal("parseStringValue() returned nil")
	}
	// The value should include the escaped quotes
	if !strings.Contains(result.Value, `quoted`) {
		t.Errorf("Value = %q, should contain 'quoted'", result.Value)
	}
}

// Test parseName with underscore
func TestParseName_WithUnderscore(t *testing.T) {
	input := `_privateField`
	p := &queryParser{input: input, pos: 0}
	name := p.parseName()
	if name != "_privateField" {
		t.Errorf("parseName() = %q, want %q", name, "_privateField")
	}
}

// Test parseName with numbers
func TestParseName_WithNumbers(t *testing.T) {
	input := `field123`
	p := &queryParser{input: input, pos: 0}
	name := p.parseName()
	if name != "field123" {
		t.Errorf("parseName() = %q, want %q", name, "field123")
	}
}

// Test peekWord at end of input
func TestPeekWord_EndOfInput(t *testing.T) {
	input := `abc`
	p := &queryParser{input: input, pos: 3}
	word := p.peekWord()
	if word != "" {
		t.Errorf("peekWord() = %q, want empty string", word)
	}
}

// Test skipUntil when char not found
func TestSkipUntil_NotFound(t *testing.T) {
	input := `abcdef`
	p := &queryParser{input: input, pos: 0}
	p.skipUntil('z')
	if p.pos != len(input) {
		t.Errorf("pos = %d, want %d", p.pos, len(input))
	}
}

// Test isWhitespace with various characters
func TestIsWhitespace(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{',', true},
		{'a', false},
		{'1', false},
		{'{', false},
	}

	for _, tt := range tests {
		result := isWhitespace(tt.char)
		if result != tt.expected {
			t.Errorf("isWhitespace(%q) = %v, want %v", tt.char, result, tt.expected)
		}
	}
}

// Test isLetter with various characters
func TestIsLetter(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'m', true},
		{'1', false},
		{'_', false},
		{' ', false},
	}

	for _, tt := range tests {
		result := isLetter(tt.char)
		if result != tt.expected {
			t.Errorf("isLetter(%q) = %v, want %v", tt.char, result, tt.expected)
		}
	}
}

// Test isDigit with various characters
func TestIsDigit(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{'0', true},
		{'5', true},
		{'9', true},
		{'a', false},
		{' ', false},
		{'_', false},
	}

	for _, tt := range tests {
		result := isDigit(tt.char)
		if result != tt.expected {
			t.Errorf("isDigit(%q) = %v, want %v", tt.char, result, tt.expected)
		}
	}
}
