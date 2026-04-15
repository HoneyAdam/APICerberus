package graphql

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsSSERequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		header string
		query  string
		want   bool
	}{
		{"accept event-stream", http.MethodGet, "text/event-stream", "", true},
		{"accept multiple with event-stream", http.MethodGet, "text/html, text/event-stream", "", true},
		{"transport sse param", http.MethodGet, "", "transport=sse", true},
		{"no sse indicators", http.MethodGet, "application/json", "", false},
		{"nil request handled by func", http.MethodGet, "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tt.method, "/graphql?"+tt.query, nil)
			if tt.header != "" {
				req.Header.Set("Accept", tt.header)
			}
			if got := IsSSERequest(req); got != tt.want {
				t.Fatalf("IsSSERequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSSERequestNil(t *testing.T) {
	t.Parallel()
	if IsSSERequest(nil) {
		t.Fatal("nil request should return false")
	}
}

func TestParseSSERequestGET(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/graphql?query=subscription{messages}&operationName=OnMessage", nil)
	query, vars, opName, err := parseSSERequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query != "subscription{messages}" {
		t.Fatalf("query: got %q", query)
	}
	if opName != "OnMessage" {
		t.Fatalf("opName: got %q", opName)
	}
	if len(vars) != 0 {
		t.Fatalf("vars: expected empty, got %v", vars)
	}
}

func TestParseSSERequestGETWithVariables(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, `/graphql?query=subscription{id}&variables={"room":"general"}`, nil)
	_, vars, _, err := parseSSERequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["room"] != "general" {
		t.Fatalf("expected room=general, got %v", vars)
	}
}

func TestParseSSERequestPOST(t *testing.T) {
	t.Parallel()

	body := `{"query":"subscription{messages}","variables":{"room":"test"},"operationName":"OnMsg"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	query, vars, opName, err := parseSSERequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query != "subscription{messages}" {
		t.Fatalf("query: got %q", query)
	}
	if vars["room"] != "test" {
		t.Fatalf("vars: got %v", vars)
	}
	if opName != "OnMsg" {
		t.Fatalf("opName: got %q", opName)
	}
}

func TestParseSSERequestInvalidMethod(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPut, "/graphql", nil)
	_, _, _, err := parseSSERequest(req)
	if err == nil {
		t.Fatal("expected error for PUT method")
	}
}

func TestParseSSERequestInvalidVariables(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/graphql?query=sub&variables=not-json", nil)
	_, _, _, err := parseSSERequest(req)
	if err == nil {
		t.Fatal("expected error for invalid variables")
	}
}

func TestParseSSERequestInvalidBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	_, _, _, err := parseSSERequest(req)
	if err == nil {
		t.Fatal("expected error for invalid body")
	}
}

func TestWriteSSEError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	writeSSEError(rec, "test error", http.StatusBadRequest)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("expected SSE error event, got %q", body)
	}
	if !strings.Contains(body, "test error") {
		t.Fatalf("expected error message in body, got %q", body)
	}
}

func TestSSENilReceivers(t *testing.T) {
	t.Parallel()

	var p *SSESubscriptionProxy
	p.HandleSSE(nil, nil) // Should not panic
	p.HandleSSE(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestReadUnmaskedFrameText(t *testing.T) {
	t.Parallel()

	// Build a server→client (unmasked) WebSocket text frame.
	data := []byte(`{"type":"next"}`)
	frame := buildUnmaskedFrame(wsOpText, data)

	reader := bufio.NewReader(strings.NewReader(string(frame)))
	opcode, payload, err := readUnmaskedFrame(reader)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if opcode != wsOpText {
		t.Fatalf("opcode: got %d, want %d", opcode, wsOpText)
	}
	if string(payload) != string(data) {
		t.Fatalf("payload: got %q, want %q", string(payload), string(data))
	}
}

func TestReadUnmaskedFrameClose(t *testing.T) {
	t.Parallel()

	frame := buildUnmaskedFrame(wsOpClose, nil)
	reader := bufio.NewReader(strings.NewReader(string(frame)))
	opcode, _, err := readUnmaskedFrame(reader)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if opcode != wsOpClose {
		t.Fatalf("opcode: got %d, want %d", opcode, wsOpClose)
	}
}

func TestReadUnmaskedFramePingPong(t *testing.T) {
	t.Parallel()

	frame := buildUnmaskedFrame(wsOpPing, []byte("hello"))
	reader := bufio.NewReader(strings.NewReader(string(frame)))
	opcode, payload, err := readUnmaskedFrame(reader)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if opcode != wsOpPing {
		t.Fatalf("opcode: got %d, want %d", opcode, wsOpPing)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload: got %q", string(payload))
	}
}

func TestReadUnmaskedFrameLargePayload(t *testing.T) {
	t.Parallel()

	data := make([]byte, 200)
	for i := range data {
		data[i] = byte('A' + i%26)
	}
	frame := buildUnmaskedFrame(wsOpText, data)

	reader := bufio.NewReader(strings.NewReader(string(frame)))
	opcode, payload, err := readUnmaskedFrame(reader)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if opcode != wsOpText {
		t.Fatalf("opcode: got %d", opcode)
	}
	if len(payload) != len(data) {
		t.Fatalf("payload length: got %d, want %d", len(payload), len(data))
	}
}

func TestReadUnmaskedFrameEmpty(t *testing.T) {
	t.Parallel()

	// Empty frame (zero-length payload).
	frame := buildUnmaskedFrame(wsOpText, nil)
	reader := bufio.NewReader(strings.NewReader(string(frame)))
	opcode, payload, err := readUnmaskedFrame(reader)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if opcode != wsOpText {
		t.Fatalf("opcode: got %d", opcode)
	}
	if len(payload) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(payload))
	}
}

func TestSSEProxyMissingQuery(t *testing.T) {
	t.Parallel()

	p := NewSSESubscriptionProxy("http://localhost:0")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)

	p.HandleSSE(rec, req)

	// Should get SSE error for missing query.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMustMarshal(t *testing.T) {
	t.Parallel()

	data := mustMarshal(map[string]string{"key": "value"})
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["key"] != "value" {
		t.Fatalf("expected key=value, got %v", result)
	}
}

// buildUnmaskedFrame constructs an unmasked WebSocket frame for testing.
func buildUnmaskedFrame(opcode byte, payload []byte) []byte {
	var frame []byte
	frame = append(frame, 0x80|opcode) // FIN + opcode

	length := len(payload)
	switch {
	case length <= 125:
		frame = append(frame, byte(length))
	case length <= 65535:
		frame = append(frame, 126)
		frame = append(frame, byte(length>>8), byte(length))
	default:
		frame = append(frame, 127)
		for i := 7; i >= 0; i-- {
			frame = append(frame, byte(length&0xFF))
			length >>= 8
		}
	}

	frame = append(frame, payload...)
	return frame
}
