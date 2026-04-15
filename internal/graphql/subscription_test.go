package graphql

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsSubscriptionQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{"subscription query", "subscription { messageAdded { text } }", true},
		{"query operation", "query { users { name } }", false},
		{"mutation", "mutation { createUser { id } }", false},
		{"bare subscription", "subscription { onEvent }", true},
		{"empty query", "", false},
		{"invalid syntax", "subscription {", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSubscriptionQuery(tt.query); got != tt.expected {
				t.Errorf("IsSubscriptionQuery(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestIsSubscriptionRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupReq    func() *http.Request
		expected    bool
	}{
		{
			"valid subscription request",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/graphql", nil)
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				r.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")
				return r
			},
			true,
		},
		{
			"missing protocol",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/graphql", nil)
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			},
			false,
		},
		{
			"missing upgrade",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/graphql", nil)
				r.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws")
				return r
			},
			false,
		},
		{
			"plain HTTP request",
			func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("{}"))
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSubscriptionRequest(tt.setupReq()); got != tt.expected {
				t.Errorf("IsSubscriptionRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsWSUpgrade(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		setupReq func() *http.Request
		expected bool
	}{
		{
			"valid upgrade",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			},
			true,
		},
		{
			"case insensitive",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Set("Connection", "upgrade")
				r.Header.Set("Upgrade", "WebSocket")
				return r
			},
			true,
		},
		{
			"multiple connection values",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Set("Connection", "keep-alive, Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			},
			true,
		},
		{
			"no upgrade header",
			func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Set("Connection", "keep-alive")
				return r
			},
			false,
		},
		{
			"nil request",
			func() *http.Request { return nil },
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isWSUpgrade(tt.setupReq()); got != tt.expected {
				t.Errorf("isWSUpgrade() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasGraphQLWSProtocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"exact match", "graphql-transport-ws", true},
		{"with other protocols", "graphql-transport-ws, soap", true},
		{"different protocol", "graphql-ws", false},
		{"empty header", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Sec-WebSocket-Protocol", tt.header)
			if got := hasGraphQLWSProtocol(r); got != tt.expected {
				t.Errorf("hasGraphQLWSProtocol(%q) = %v, want %v", tt.header, got, tt.expected)
			}
		})
	}
}

func TestComputeAcceptKey(t *testing.T) {
	t.Parallel()
	// Deterministic output: same key must always produce same accept value
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	got1 := computeAcceptKey(key)
	got2 := computeAcceptKey(key)
	if got1 != got2 {
		t.Errorf("computeAcceptKey not deterministic: %q != %q", got1, got2)
	}
	// Must be valid base64
	if _, err := base64.StdEncoding.DecodeString(got1); err != nil {
		t.Errorf("result is not valid base64: %v", err)
	}
	// Must be 28 chars (20 bytes SHA-1 → base64)
	if len(got1) != 28 {
		t.Errorf("result len = %d, want 28", len(got1))
	}
}

func TestComputeAcceptKey_DifferentInputs(t *testing.T) {
	t.Parallel()
	a := computeAcceptKey("key1")
	b := computeAcceptKey("key2")
	if a == b {
		t.Error("different inputs should produce different outputs")
	}
}

func TestIsIntrospectionQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{"__schema", "{ __schema { types { name } } }", true},
		{"__type", "{ __type(name: \"User\") { fields { name } } }", true},
		{"__typename", "{ search { ... on User { __typename id } } }", true},
		{"__fields", "__fields check", true},
		{"__args", "__args check", true},
		{"normal query", "{ users { name email } }", false},
		{"empty", "", false},
		{"partial match", "my__type_field", true}, // contains "__type"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsIntrospectionQuery(tt.query); got != tt.expected {
				t.Errorf("IsIntrospectionQuery(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestParseQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"simple query", "{ users { name } }", false},
		{"named query", "query GetUsers { users { name } }", false},
		{"mutation", "mutation CreateUser { createUser { id } }", false},
		{"subscription", "subscription { onEvent }", false},
		{"empty string", "", true},
		{"whitespace only", "   \t\n  ", true},
		{"fragment", "fragment UserFields on User { name email } query { users { ...UserFields } }", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuery(%q) error = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
		})
	}
}

func TestParseQuery_ReturnsDocument(t *testing.T) {
	t.Parallel()
	node, err := ParseQuery("{ users { name } }")
	if err != nil {
		t.Fatalf("ParseQuery error: %v", err)
	}
	doc, ok := node.(*Document)
	if !ok {
		t.Fatalf("expected *Document, got %T", node)
	}
	if len(doc.Definitions) == 0 {
		t.Error("expected at least one definition")
	}
}

func TestNodeKind(t *testing.T) {
	t.Parallel()
	doc := &Document{}
	op := &Operation{}
	field := &Field{}
	fs := &FragmentSpread{}
	ifrag := &InlineFragment{}
	fd := &FragmentDefinition{}

	if doc.NodeKind() != "Document" {
		t.Errorf("Document.NodeKind() = %q", doc.NodeKind())
	}
	if op.NodeKind() != "Operation" {
		t.Errorf("Operation.NodeKind() = %q", op.NodeKind())
	}
	if field.NodeKind() != "Field" {
		t.Errorf("Field.NodeKind() = %q", field.NodeKind())
	}
	if fs.NodeKind() != "FragmentSpread" {
		t.Errorf("FragmentSpread.NodeKind() = %q", fs.NodeKind())
	}
	if ifrag.NodeKind() != "InlineFragment" {
		t.Errorf("InlineFragment.NodeKind() = %q", ifrag.NodeKind())
	}
	if fd.NodeKind() != "FragmentDefinition" {
		t.Errorf("FragmentDefinition.NodeKind() = %q", fd.NodeKind())
	}
}

func TestValueKind(t *testing.T) {
	t.Parallel()
	sv := &ScalarValue{}
	lv := &ListValue{}
	ov := &ObjectValue{}

	if sv.ValueKind() != "Scalar" {
		t.Errorf("ScalarValue.ValueKind() = %q", sv.ValueKind())
	}
	if lv.ValueKind() != "List" {
		t.Errorf("ListValue.ValueKind() = %q", lv.ValueKind())
	}
	if ov.ValueKind() != "Object" {
		t.Errorf("ObjectValue.ValueKind() = %q", ov.ValueKind())
	}
}

func TestDocumentDepth(t *testing.T) {
	t.Parallel()
	// Empty document
	doc := &Document{}
	if doc.Depth() != 0 {
		t.Errorf("empty Document.Depth() = %d, want 0", doc.Depth())
	}
	// Single operation with nested fields
	doc = &Document{
		Definitions: []Node{
			&Operation{
				Selections: []Node{
					&Field{
						Name: "user",
						Selections: []Node{
							&Field{Name: "name"},
						},
					},
				},
			},
		},
	}
	d := doc.Depth()
	if d < 2 {
		t.Errorf("Document.Depth() = %d, want >= 2", d)
	}
}

func TestOperationDepth(t *testing.T) {
	t.Parallel()
	op := &Operation{}
	if op.Depth() != 0 {
		t.Errorf("empty Operation.Depth() = %d, want 0", op.Depth())
	}
	op = &Operation{
		Selections: []Node{
			&Field{Name: "a"},
			&Field{Name: "b"},
		},
	}
	if op.Depth() != 1 {
		t.Errorf("Operation.Depth() = %d, want 1", op.Depth())
	}
}

func TestFieldDepth(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "leaf"}
	if f.Depth() != 1 {
		t.Errorf("leaf Field.Depth() = %d, want 1", f.Depth())
	}
	f = &Field{
		Name: "parent",
		Selections: []Node{
			&Field{
				Name: "child",
				Selections: []Node{
					&Field{Name: "grandchild"},
				},
			},
		},
	}
	if f.Depth() != 3 {
		t.Errorf("nested Field.Depth() = %d, want 3", f.Depth())
	}
}
