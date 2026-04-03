package graphql

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Test ValueKind methods
func TestScalarValue_ValueKind(t *testing.T) {
	v := &ScalarValue{Value: "test"}
	if v.ValueKind() != "Scalar" {
		t.Errorf("ScalarValue.ValueKind() = %q, want Scalar", v.ValueKind())
	}
}

func TestListValue_ValueKind(t *testing.T) {
	v := &ListValue{Values: []Value{}}
	if v.ValueKind() != "List" {
		t.Errorf("ListValue.ValueKind() = %q, want List", v.ValueKind())
	}
}

func TestObjectValue_ValueKind(t *testing.T) {
	v := &ObjectValue{Fields: map[string]Value{}}
	if v.ValueKind() != "Object" {
		t.Errorf("ObjectValue.ValueKind() = %q, want Object", v.ValueKind())
	}
}

// Test NodeKind methods
func TestDocument_NodeKind(t *testing.T) {
	d := &Document{Definitions: []Node{}}
	if d.NodeKind() != "Document" {
		t.Errorf("Document.NodeKind() = %q, want Document", d.NodeKind())
	}
}

func TestOperation_NodeKind(t *testing.T) {
	o := &Operation{Type: "query"}
	if o.NodeKind() != "Operation" {
		t.Errorf("Operation.NodeKind() = %q, want Operation", o.NodeKind())
	}
}

func TestField_NodeKind(t *testing.T) {
	f := &Field{Name: "test"}
	if f.NodeKind() != "Field" {
		t.Errorf("Field.NodeKind() = %q, want Field", f.NodeKind())
	}
}

func TestFragmentSpread_NodeKind(t *testing.T) {
	fs := &FragmentSpread{Name: "TestFragment"}
	if fs.NodeKind() != "FragmentSpread" {
		t.Errorf("FragmentSpread.NodeKind() = %q, want FragmentSpread", fs.NodeKind())
	}
}

func TestInlineFragment_NodeKind(t *testing.T) {
	frag := &InlineFragment{TypeCondition: "User"}
	if frag.NodeKind() != "InlineFragment" {
		t.Errorf("InlineFragment.NodeKind() = %q, want InlineFragment", frag.NodeKind())
	}
}

func TestFragmentDefinition_NodeKind(t *testing.T) {
	fd := &FragmentDefinition{Name: "TestFragment", Type: "User"}
	if fd.NodeKind() != "FragmentDefinition" {
		t.Errorf("FragmentDefinition.NodeKind() = %q, want FragmentDefinition", fd.NodeKind())
	}
}

// Test Depth methods for various node types
func TestFragmentSpread_Depth(t *testing.T) {
	fs := &FragmentSpread{Name: "TestFragment"}
	if fs.Depth() != 1 {
		t.Errorf("FragmentSpread.Depth() = %d, want 1", fs.Depth())
	}
}

func TestDocument_Depth_Empty(t *testing.T) {
	d := &Document{Definitions: []Node{}}
	if d.Depth() != 0 {
		t.Errorf("Document.Depth() with empty definitions = %d, want 0", d.Depth())
	}
}

func TestOperation_Depth_Empty(t *testing.T) {
	o := &Operation{Type: "query", Selections: []Node{}}
	if o.Depth() != 0 {
		t.Errorf("Operation.Depth() with empty selections = %d, want 0", o.Depth())
	}
}

func TestField_Depth_Leaf(t *testing.T) {
	f := &Field{Name: "id", Selections: []Node{}}
	if f.Depth() != 1 {
		t.Errorf("Field.Depth() with no selections = %d, want 1", f.Depth())
	}
}

func TestInlineFragment_Depth_Empty(t *testing.T) {
	frag := &InlineFragment{TypeCondition: "User", Selections: []Node{}}
	if frag.Depth() != 1 {
		t.Errorf("InlineFragment.Depth() with empty selections = %d, want 1", frag.Depth())
	}
}

func TestFragmentDefinition_Depth_Empty(t *testing.T) {
	fd := &FragmentDefinition{Name: "Test", Type: "User", Selections: []Node{}}
	if fd.Depth() != 1 {
		t.Errorf("FragmentDefinition.Depth() with empty selections = %d, want 1", fd.Depth())
	}
}

// Test parsing inline fragments
func TestParseQuery_WithInlineFragment(t *testing.T) {
	query := `{
		users {
			... on User {
				id
				name
			}
		}
	}`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if node == nil {
		t.Fatal("ParseQuery() returned nil")
	}
	// Should have depth of 3: query -> users -> inline fragment fields
	if node.Depth() != 3 {
		t.Errorf("Depth() = %d, want 3", node.Depth())
	}
}

// Test parsing with list values
func TestParseQuery_WithListValue(t *testing.T) {
	query := `{
		users(ids: ["1", "2", "3"]) {
			id
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

// Test parsing with object values
func TestParseQuery_WithObjectValue(t *testing.T) {
	query := `{
		user(filter: { name: "John", active: true }) {
			id
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

// Test parsing with nested list in object
func TestParseQuery_WithNestedListObject(t *testing.T) {
	query := `{
		users(filter: { ids: ["1", "2"], status: "active" }) {
			id
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

// Test parsing empty list
func TestParseQuery_WithEmptyList(t *testing.T) {
	query := `{
		users(ids: []) {
			id
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

// Test parsing empty object
func TestParseQuery_WithEmptyObject(t *testing.T) {
	query := `{
		users(filter: {}) {
			id
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

// Test parsing with directives
func TestParseQuery_WithDirectives(t *testing.T) {
	query := `{
		users @include(if: true) {
			id
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

// Test parsing fragment definition
func TestParseQuery_WithFragmentDefinition(t *testing.T) {
	query := `
		fragment UserFields on User {
			id
			name
		}
		query {
			users {
				...UserFields
			}
		}
	`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if node == nil {
		t.Fatal("ParseQuery() returned nil")
	}
	// Check it's a document
	doc, ok := node.(*Document)
	if !ok {
		t.Fatal("ParseQuery() did not return a Document")
	}
	if len(doc.Definitions) != 2 {
		t.Errorf("Document has %d definitions, want 2", len(doc.Definitions))
	}
}

// Test parsing subscription operation
func TestParseQuery_Subscription(t *testing.T) {
	query := `subscription OnMessage {
		messageAdded {
			id
			text
		}
	}`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	op, ok := node.(*Document)
	if !ok {
		t.Fatalf("Expected Document, got %T", node)
	}
	if len(op.Definitions) != 1 {
		t.Fatalf("Expected 1 definition, got %d", len(op.Definitions))
	}
	operation, ok := op.Definitions[0].(*Operation)
	if !ok {
		t.Fatalf("Expected Operation, got %T", op.Definitions[0])
	}
	if operation.Type != "subscription" {
		t.Errorf("Operation.Type = %q, want subscription", operation.Type)
	}
}

// Test parsing with variables
func TestParseQuery_WithVariables(t *testing.T) {
	query := `query GetUser($id: ID!) {
		user(id: $id) {
			id
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

// Test parsing complex nested query
func TestParseQuery_ComplexNested(t *testing.T) {
	query := `query GetUsers {
		users(limit: 10) {
			id
			name
			posts {
				title
				comments {
					author {
						name
					}
				}
			}
		}
	}`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if node.Depth() != 5 {
		t.Errorf("Depth() = %d, want 5", node.Depth())
	}
}

// Test parsing with multiple root fields
func TestParseQuery_MultipleRootFields(t *testing.T) {
	query := `{
		users {
			id
		}
		posts {
			title
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

// Test parsing with boolean and null values
func TestParseQuery_BooleanAndNullValues(t *testing.T) {
	query := `{
		users(active: true, status: null, verified: false) {
			id
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

// Test parsing with number values
func TestParseQuery_NumberValues(t *testing.T) {
	query := `{
		users(limit: 10, offset: 0, price: 19.99) {
			id
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

// Test parsing with string containing escaped characters
func TestParseQuery_EscapedString(t *testing.T) {
	query := `{
		user(name: "John \"Johnny\" Doe") {
			id
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

// Test parsing with enum values
func TestParseQuery_EnumValues(t *testing.T) {
	query := `{
		users(status: ACTIVE, role: ADMIN) {
			id
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

// Test parsing deeply nested inline fragment
func TestParseQuery_DeepInlineFragment(t *testing.T) {
	query := `{
		users {
			... on User {
				friends {
					... on User {
						name
					}
				}
			}
		}
	}`
	node, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if node == nil {
		t.Fatal("ParseQuery() returned nil")
	}
	// Depth: query -> users -> friends -> name = 4, but inline fragments add 1 each
	// So: query(0) -> users(1) -> inline(2) -> friends(3) -> inline(4) -> name(5)
	if node.Depth() != 5 {
		t.Errorf("Depth() = %d, want 5", node.Depth())
	}
}

// Test Proxy ServeHTTP with regular request
func TestProxy_ServeHTTP_RegularRequest(t *testing.T) {
	// Create a simple upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"users":[]}}`))
	}))
	defer upstream.Close()

	proxy, err := NewProxy(&ProxyConfig{
		TargetURL: upstream.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}

	req := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{"query":"{ users { id } }"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	proxy.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"data"`) {
		t.Errorf("Response body does not contain data: %s", string(body))
	}
}

// Test Proxy ServeHTTP with subscription request (WebSocket upgrade)
func TestProxy_ServeHTTP_SubscriptionRequest(t *testing.T) {
	// Create upstream server that accepts WebSocket
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just check the headers, don't actually upgrade
		if r.Header.Get("Upgrade") == "websocket" {
			w.WriteHeader(http.StatusSwitchingProtocols)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy, err := NewProxy(&ProxyConfig{
		TargetURL: upstream.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}

	// Request with WebSocket upgrade headers
	req := httptest.NewRequest("GET", "/graphql", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()

	// This will attempt WebSocket upgrade path
	proxy.ServeHTTP(w, req)
}

// Test isBenignClose function
func TestIsBenignClose(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: true, // nil is considered benign
		},
		{
			name:     "io.EOF error",
			err:      io.EOF,
			expected: true, // EOF is considered benign
		},
		{
			name:     "other error",
			err:      http.ErrHandlerTimeout,
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

// Test closeDone helper
func TestCloseDone(t *testing.T) {
	done := make(chan struct{})
	closeDone(done)
	// Should not panic and channel should be closed
	select {
	case <-done:
		// Expected - channel is closed
	default:
		t.Error("Expected done channel to be closed")
	}

	// Closing already closed channel should not panic
	closeDone(done)
}

// Test isWSUpgrade function
func TestIsWSUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string][]string
		expected bool
	}{
		{
			name:     "no upgrade header",
			headers:  map[string][]string{},
			expected: false,
		},
		{
			name: "upgrade websocket",
			headers: map[string][]string{
				"Connection": []string{"upgrade"},
				"Upgrade":    []string{"websocket"},
			},
			expected: true,
		},
		{
			name: "upgrade WebSocket (case insensitive)",
			headers: map[string][]string{
				"Connection": []string{"Upgrade"},
				"Upgrade":    []string{"WebSocket"},
			},
			expected: true,
		},
		{
			name: "upgrade other",
			headers: map[string][]string{
				"Upgrade": []string{"HTTP/2"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header = tt.headers
			result := isWSUpgrade(req)
			if result != tt.expected {
				t.Errorf("isWSUpgrade() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test queryParser peekN function
func TestQueryParser_PeekN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		n        int
		expected byte
	}{
		{
			name:     "peek 0",
			input:    "query",
			n:        0,
			expected: 'q',
		},
		{
			name:     "peek 1",
			input:    "query",
			n:        1,
			expected: 'u',
		},
		{
			name:     "peek 4",
			input:    "query",
			n:        4,
			expected: 'y',
		},
		{
			name:     "peek beyond end",
			input:    "ab",
			n:        10,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &queryParser{input: tt.input, pos: 0}
			result := p.peekN(tt.n)
			if result != tt.expected {
				t.Errorf("peekN(%d) = %q, want %q", tt.n, result, tt.expected)
			}
		})
	}
}

// Test queryParser advance function
func TestQueryParser_Advance(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		advanceCount int
		expectedPos  int
		expectedChar byte
	}{
		{
			name:         "advance 1",
			input:        "query",
			advanceCount: 1,
			expectedPos:  1,
			expectedChar: 'u',
		},
		{
			name:         "advance 3",
			input:        "query",
			advanceCount: 3,
			expectedPos:  3,
			expectedChar: 'r',
		},
		{
			name:         "advance beyond end",
			input:        "ab",
			advanceCount: 10,
			expectedPos:  2,
			expectedChar: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &queryParser{input: tt.input, pos: 0}
			p.advance(tt.advanceCount)
			if p.pos != tt.expectedPos {
				t.Errorf("pos = %d, want %d", p.pos, tt.expectedPos)
			}
			if p.peek() != tt.expectedChar {
				t.Errorf("peek() = %q, want %q", p.peek(), tt.expectedChar)
			}
		})
	}
}

// Test parseSelection with fragment spread
func TestParseSelection_FragmentSpread(t *testing.T) {
	input := "...TestFragment"
	p := &queryParser{input: input, pos: 0}
	selection, err := p.parseSelection()
	if err != nil {
		t.Fatalf("parseSelection() error = %v", err)
	}
	if selection == nil {
		t.Fatal("parseSelection() returned nil")
	}
	// Should be a FragmentSpread
	if _, ok := selection.(*FragmentSpread); !ok {
		t.Errorf("Expected FragmentSpread, got %T", selection)
	}
}


// Test parseInlineFragment function
func TestParseInlineFragment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid inline fragment",
			input:   `... on User { id name }`,
			wantErr: false,
		},
		{
			name:    "inline fragment without type condition",
			input:   `... { id name }`,
			wantErr: false,
		},
		{
			name:    "empty inline fragment",
			input:   `... on User {}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &queryParser{input: tt.input, pos: 0}
			_, err := p.parseInlineFragment()
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInlineFragment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
