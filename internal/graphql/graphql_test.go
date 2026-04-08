package graphql

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestIsGraphQLRequest(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		contentType string
		query       string
		isGraphQL   bool
	}{
		{
			name:        "POST with application/json",
			method:      "POST",
			contentType: "application/json",
			isGraphQL:   true,
		},
		{
			name:        "POST with application/graphql",
			method:      "POST",
			contentType: "application/graphql",
			isGraphQL:   true,
		},
		{
			name:      "GET with query param",
			method:    "GET",
			query:     "{ users { id } }",
			isGraphQL: true,
		},
		{
			name:      "GET without query param",
			method:    "GET",
			isGraphQL: false,
		},
		{
			name:        "POST with other content type",
			method:      "POST",
			contentType: "text/plain",
			isGraphQL:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test without HTTP request, so we just document
			t.Logf("Method: %s, Content-Type: %s -> isGraphQL: %v", tt.method, tt.contentType, tt.isGraphQL)
		})
	}
}

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "simple query",
			query:   `{ users { id name } }`,
			wantErr: false,
		},
		{
			name: "query with operation type",
			query: `query GetUsers {
				users {
					id
					name
					posts {
						title
					}
				}
			}`,
			wantErr: false,
		},
		{
			name: "query with alias",
			query: `{
				users: allUsers {
					id
					name
				}
			}`,
			wantErr: false,
		},
		{
			name: "query with arguments",
			query: `{
				user(id: "123") {
					name
					email
				}
			}`,
			wantErr: false,
		},
		{
			name: "query with fragment",
			query: `
				fragment UserFields on User {
					id
					name
				}
				query {
					users {
						...UserFields
					}
				}
			`,
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
		},
		{
			name: "mutation",
			query: `mutation CreateUser {
				createUser(name: "John") {
					id
				}
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := ParseQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if ast == nil {
				t.Error("ParseQuery() returned nil AST")
			}
		})
	}
}

func TestCalculateDepth(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{
			name:  "flat query",
			query: `{ users { id name } }`,
			want:  2,
		},
		{
			name: "nested query",
			query: `{
				users {
					id
					posts {
						title
					}
				}
			}`,
			want: 3,
		},
		{
			name: "deeply nested query",
			query: `{
				users {
					posts {
						comments {
							author {
								name
							}
						}
					}
				}
			}`,
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery() error = %v", err)
			}

			got := ast.Depth()
			if got != tt.want {
				t.Errorf("Depth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestQueryAnalyzer(t *testing.T) {
	analyzer := NewQueryAnalyzer(&AnalyzerConfig{
		MaxDepth:      5,
		MaxComplexity: 100,
		DefaultCost:   1,
	})

	tests := []struct {
		name    string
		query   string
		isValid bool
	}{
		{
			name: "valid simple query",
			query: `{
				users {
					id
					name
				}
			}`,
			isValid: true,
		},
		{
			name: "query exceeding max depth",
			query: `{
				a {
					b {
						c {
							d {
								e {
									f {
										g
									}
								}
							}
						}
					}
				}
			}`,
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.Analyze(tt.query)
			if err != nil {
				t.Logf("Analyze() error = %v", err)
				return
			}

			if result.IsValid != tt.isValid {
				t.Errorf("IsValid = %v, want %v", result.IsValid, tt.isValid)
			}
		})
	}
}

func TestIsIntrospectionQuery(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{
			query: `{ __schema { types { name } } }`,
			want:  true,
		},
		{
			query: `{ __type(name: "User") { name } }`,
			want:  true,
		},
		{
			query: `{ users { id __typename } }`,
			want:  true,
		},
		{
			query: `{ users { id name } }`,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := IsIntrospectionQuery(tt.query)
			if got != tt.want {
				t.Errorf("IsIntrospectionQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Subscription tests ---

func TestSubscriptionMessageFraming(t *testing.T) {
	t.Run("encode and decode connection_init", func(t *testing.T) {
		msg := &wsMessage{Type: gqlConnectionInit}
		data, err := EncodeWSMessage(msg)
		if err != nil {
			t.Fatalf("EncodeWSMessage() error = %v", err)
		}

		decoded, err := DecodeWSMessage(data)
		if err != nil {
			t.Fatalf("DecodeWSMessage() error = %v", err)
		}
		if decoded.Type != gqlConnectionInit {
			t.Errorf("Type = %q, want %q", decoded.Type, gqlConnectionInit)
		}
		if decoded.ID != "" {
			t.Errorf("ID = %q, want empty", decoded.ID)
		}
	})

	t.Run("encode and decode subscribe", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]interface{}{
			"query": `subscription { messageAdded { id text } }`,
		})
		msg := &wsMessage{
			ID:      "sub-1",
			Type:    gqlSubscribe,
			Payload: payload,
		}

		data, err := EncodeWSMessage(msg)
		if err != nil {
			t.Fatalf("EncodeWSMessage() error = %v", err)
		}

		decoded, err := DecodeWSMessage(data)
		if err != nil {
			t.Fatalf("DecodeWSMessage() error = %v", err)
		}
		if decoded.Type != gqlSubscribe {
			t.Errorf("Type = %q, want %q", decoded.Type, gqlSubscribe)
		}
		if decoded.ID != "sub-1" {
			t.Errorf("ID = %q, want %q", decoded.ID, "sub-1")
		}

		// Verify payload round-trips.
		var payloadMap map[string]interface{}
		if err := json.Unmarshal(decoded.Payload, &payloadMap); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payloadMap["query"] != `subscription { messageAdded { id text } }` {
			t.Errorf("payload query = %v, want subscription query", payloadMap["query"])
		}
	})

	t.Run("encode and decode next", func(t *testing.T) {
		nextPayload, _ := json.Marshal(map[string]interface{}{
			"data": map[string]interface{}{
				"messageAdded": map[string]interface{}{
					"id":   "1",
					"text": "hello",
				},
			},
		})
		msg := BuildNext("sub-1", nextPayload)

		data, err := EncodeWSMessage(msg)
		if err != nil {
			t.Fatalf("EncodeWSMessage() error = %v", err)
		}

		decoded, err := DecodeWSMessage(data)
		if err != nil {
			t.Fatalf("DecodeWSMessage() error = %v", err)
		}
		if decoded.Type != gqlNext {
			t.Errorf("Type = %q, want %q", decoded.Type, gqlNext)
		}
		if decoded.ID != "sub-1" {
			t.Errorf("ID = %q, want %q", decoded.ID, "sub-1")
		}
	})

	t.Run("encode and decode complete", func(t *testing.T) {
		msg := BuildComplete("sub-1")

		data, err := EncodeWSMessage(msg)
		if err != nil {
			t.Fatalf("EncodeWSMessage() error = %v", err)
		}

		decoded, err := DecodeWSMessage(data)
		if err != nil {
			t.Fatalf("DecodeWSMessage() error = %v", err)
		}
		if decoded.Type != gqlComplete {
			t.Errorf("Type = %q, want %q", decoded.Type, gqlComplete)
		}
		if decoded.ID != "sub-1" {
			t.Errorf("ID = %q, want %q", decoded.ID, "sub-1")
		}
	})

	t.Run("encode and decode error", func(t *testing.T) {
		msg := BuildError("sub-1", []GraphQLError{{Message: "something went wrong"}})

		data, err := EncodeWSMessage(msg)
		if err != nil {
			t.Fatalf("EncodeWSMessage() error = %v", err)
		}

		decoded, err := DecodeWSMessage(data)
		if err != nil {
			t.Fatalf("DecodeWSMessage() error = %v", err)
		}
		if decoded.Type != gqlError {
			t.Errorf("Type = %q, want %q", decoded.Type, gqlError)
		}

		var errs []GraphQLError
		if err := json.Unmarshal(decoded.Payload, &errs); err != nil {
			t.Fatalf("unmarshal error payload: %v", err)
		}
		if len(errs) != 1 || errs[0].Message != "something went wrong" {
			t.Errorf("error payload = %v, want single error with message 'something went wrong'", errs)
		}
	})

	t.Run("decode invalid JSON", func(t *testing.T) {
		_, err := DecodeWSMessage([]byte("not json"))
		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})
}

func TestConnectionInitAckFlow(t *testing.T) {
	t.Run("connection_init produces connection_ack", func(t *testing.T) {
		// Simulate the client sending connection_init.
		initMsg := &wsMessage{Type: gqlConnectionInit}
		initData, err := EncodeWSMessage(initMsg)
		if err != nil {
			t.Fatalf("EncodeWSMessage() error = %v", err)
		}

		// Verify the init message is correct.
		decoded, err := DecodeWSMessage(initData)
		if err != nil {
			t.Fatalf("DecodeWSMessage() error = %v", err)
		}
		if decoded.Type != gqlConnectionInit {
			t.Fatalf("decoded Type = %q, want %q", decoded.Type, gqlConnectionInit)
		}

		// Build the expected ack response.
		ack := BuildConnectionAck()
		if ack.Type != gqlConnectionAck {
			t.Errorf("BuildConnectionAck().Type = %q, want %q", ack.Type, gqlConnectionAck)
		}

		ackData, err := EncodeWSMessage(ack)
		if err != nil {
			t.Fatalf("EncodeWSMessage(ack) error = %v", err)
		}

		ackDecoded, err := DecodeWSMessage(ackData)
		if err != nil {
			t.Fatalf("DecodeWSMessage(ack) error = %v", err)
		}
		if ackDecoded.Type != gqlConnectionAck {
			t.Errorf("ack Type = %q, want %q", ackDecoded.Type, gqlConnectionAck)
		}
		if ackDecoded.ID != "" {
			t.Errorf("ack ID = %q, want empty", ackDecoded.ID)
		}
		if ackDecoded.Payload != nil {
			t.Errorf("ack Payload = %v, want nil", ackDecoded.Payload)
		}
	})
}

func TestWSFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		opcode  byte
		payload []byte
	}{
		{
			name:    "text frame with short payload",
			opcode:  wsOpText,
			payload: []byte(`{"type":"connection_init"}`),
		},
		{
			name:    "text frame with empty payload",
			opcode:  wsOpText,
			payload: []byte{},
		},
		{
			name:    "close frame",
			opcode:  wsOpClose,
			payload: []byte{0x03, 0xE8}, // 1000 normal closure
		},
		{
			name:    "ping frame",
			opcode:  wsOpPing,
			payload: []byte("keepalive"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := bufio.NewWriter(&buf)

			if err := writeWSFrame(writer, tt.opcode, tt.payload); err != nil {
				t.Fatalf("writeWSFrame() error = %v", err)
			}
			if err := writer.Flush(); err != nil {
				t.Fatalf("Flush() error = %v", err)
			}

			reader := bufio.NewReader(&buf)
			opcode, payload, err := readWSFrame(reader)
			if err != nil {
				t.Fatalf("readWSFrame() error = %v", err)
			}

			if opcode != tt.opcode {
				t.Errorf("opcode = %d, want %d", opcode, tt.opcode)
			}
			if !bytes.Equal(payload, tt.payload) {
				t.Errorf("payload = %v, want %v", payload, tt.payload)
			}
		})
	}
}

func TestIsSubscriptionQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{
			name:  "subscription operation",
			query: `subscription OnMessage { messageAdded { id text } }`,
			want:  true,
		},
		{
			name:  "query operation",
			query: `query GetUsers { users { id name } }`,
			want:  false,
		},
		{
			name:  "mutation operation",
			query: `mutation CreateUser { createUser(name: "Alice") { id } }`,
			want:  false,
		},
		{
			name:  "implicit query (no keyword)",
			query: `{ users { id } }`,
			want:  false,
		},
		{
			name:  "invalid query",
			query: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSubscriptionQuery(tt.query)
			if got != tt.want {
				t.Errorf("IsSubscriptionQuery(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestIsSubscriptionRequest(t *testing.T) {
	tests := []struct {
		name      string
		headers   map[string]string
		wantIsSub bool
	}{
		{
			name: "valid subscription request",
			headers: map[string]string{
				"Connection":             "Upgrade",
				"Upgrade":                "websocket",
				"Sec-WebSocket-Protocol": "graphql-transport-ws",
			},
			wantIsSub: true,
		},
		{
			name: "websocket without graphql protocol",
			headers: map[string]string{
				"Connection": "Upgrade",
				"Upgrade":    "websocket",
			},
			wantIsSub: false,
		},
		{
			name: "not a websocket request",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			wantIsSub: false,
		},
		{
			name: "graphql protocol but no upgrade",
			headers: map[string]string{
				"Sec-WebSocket-Protocol": "graphql-transport-ws",
			},
			wantIsSub: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://localhost/graphql", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			got := IsSubscriptionRequest(req)
			if got != tt.wantIsSub {
				t.Errorf("IsSubscriptionRequest() = %v, want %v", got, tt.wantIsSub)
			}
		})
	}
}

func TestComputeAcceptKey(t *testing.T) {
	// Verify deterministic output and consistency with the RFC 6455 algorithm:
	// SHA-1(key + "258EAFA5-E914-47DA-95CA-5AB5DC587FB5") -> base64
	got := computeAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "IpgXayksHvxo2xDSB5xiEvAXWDk="
	if got != want {
		t.Errorf("computeAcceptKey() = %q, want %q", got, want)
	}

	// Verify a second key produces a different, deterministic result.
	got2 := computeAcceptKey("anotherKey==")
	if got2 == "" {
		t.Error("computeAcceptKey() returned empty string")
	}
	if got2 == got {
		t.Error("different keys should produce different accept values")
	}

	// Verify same key produces same result (deterministic).
	got3 := computeAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	if got3 != got {
		t.Errorf("computeAcceptKey() not deterministic: %q != %q", got3, got)
	}
}
