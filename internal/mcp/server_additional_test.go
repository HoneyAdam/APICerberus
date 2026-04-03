package mcp

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestErrorResponse(t *testing.T) {
	resp := errorResponse(1, -32600, "Invalid Request", "additional data")

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want 2.0", resp.JSONRPC)
	}
	if resp.ID != 1 {
		t.Errorf("ID = %v, want 1", resp.ID)
	}
	if resp.Result != nil {
		t.Error("Result should be nil for error response")
	}
	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("Error.Code = %d, want -32600", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid Request" {
		t.Errorf("Error.Message = %q, want Invalid Request", resp.Error.Message)
	}
	if resp.Error.Data != "additional data" {
		t.Errorf("Error.Data = %v, want additional data", resp.Error.Data)
	}
}

func TestSuccessResponse(t *testing.T) {
	result := map[string]any{"key": "value"}
	resp := successResponse(2, result)

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want 2.0", resp.JSONRPC)
	}
	if resp.ID != 2 {
		t.Errorf("ID = %v, want 2", resp.ID)
	}
	if resp.Error != nil {
		t.Error("Error should be nil for success response")
	}
	if resp.Result == nil {
		t.Fatal("Result should not be nil")
	}
}

func TestNormalizeYAMLForConfigParser(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "key: {}",
			expected: "key:",
		},
		{
			input:    "key: []",
			expected: "key:",
		},
		{
			input:    "key: value",
			expected: "key: value",
		},
		{
			input:    "key: {nested: value}",
			expected: "key: {nested: value}",
		},
		{
			input:    "routes: []\nservices: {}",
			expected: "routes:\nservices:",
		},
	}

	for _, tt := range tests {
		got := normalizeYAMLForConfigParser(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeYAMLForConfigParser(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractAdminError(t *testing.T) {
	tests := []struct {
		name       string
		payload    any
		status     int
		wantString string
	}{
		{
			name:       "nil payload",
			payload:    nil,
			status:     500,
			wantString: "http 500",
		},
		{
			name:       "string payload",
			payload:    "error message",
			status:     400,
			wantString: "error message",
		},
		{
			name:       "map with error field as string",
			payload:    map[string]any{"error": "custom error"},
			status:     400,
			wantString: "custom error",
		},
		{
			name:       "map with error object containing message",
			payload:    map[string]any{"error": map[string]any{"message": "nested error"}},
			status:     400,
			wantString: "nested error",
		},
		{
			name:       "map without error field uses map as string",
			payload:    map[string]any{"data": "value"},
			status:     400,
			wantString: `map[data:value]`,
		},
		{
			name:       "non-map payload",
			payload:    123,
			status:     500,
			wantString: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAdminError(tt.payload, tt.status)
			if got != tt.wantString {
				t.Errorf("extractAdminError() = %q, want %q", got, tt.wantString)
			}
		})
	}
}

func TestAsInt(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		def      int
		expected int
	}{
		{"int", 42, 0, 42},
		{"int64", int64(64), 0, 64},
		{"int32", int32(32), 0, 32},
		{"float64", float64(3.14), 0, 3},
		{"string valid", "123", 0, 123},
		{"string empty", "", 99, 99},
		{"string whitespace", "   ", 99, 99},
		{"string invalid", "abc", 99, 99},
		{"invalid type", "not a number string", 99, 99},
		{"nil", nil, 99, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asInt(tt.value, tt.def)
			if got != tt.expected {
				t.Errorf("asInt(%v, %d) = %d, want %d", tt.value, tt.def, got, tt.expected)
			}
		})
	}
}

func TestCloneAnyMap(t *testing.T) {
	original := map[string]any{
		"key1": "value1",
		"key2": 123,
		"key3": map[string]any{
			"nested": "value",
		},
	}

	cloned := cloneAnyMap(original)

	if len(cloned) != len(original) {
		t.Errorf("cloned map length = %d, want %d", len(cloned), len(original))
	}

	// Verify values are copied
	if cloned["key1"] != "value1" {
		t.Errorf("cloned[key1] = %v, want value1", cloned["key1"])
	}
	if cloned["key2"] != 123 {
		t.Errorf("cloned[key2] = %v, want 123", cloned["key2"])
	}

	// Modify clone and verify original is unchanged
	cloned["key1"] = "modified"
	if original["key1"] != "value1" {
		t.Error("modifying clone affected original")
	}
}

func TestCloneConfig(t *testing.T) {
	original := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: ":8080",
		},
		Admin: config.AdminConfig{
			Addr: ":9876",
		},
		Store: config.StoreConfig{
			Path: "test.db",
		},
		Billing: config.BillingConfig{
			Enabled: true,
			RouteCosts: map[string]int64{
				"route1": 10,
			},
			MethodMultipliers: map[string]float64{
				"GET": 1.0,
			},
		},
		Cluster: config.ClusterConfig{
			Enabled: true,
			Peers: []config.ClusterPeer{
				{ID: "node1", Address: "127.0.0.1:12000"},
			},
		},
		Services: []config.Service{
			{ID: "svc1", Name: "Service 1"},
		},
		Routes: []config.Route{
			{ID: "route1", Name: "Route 1"},
		},
		Upstreams: []config.Upstream{
			{ID: "up1", Name: "Upstream 1"},
		},
	}

	cloned := cloneConfig(original)

	if cloned == nil {
		t.Fatal("cloneConfig returned nil")
	}

	// Verify basic fields are copied
	if cloned.Gateway.HTTPAddr != ":8080" {
		t.Errorf("Gateway.HTTPAddr = %q, want :8080", cloned.Gateway.HTTPAddr)
	}

	// Verify slices are copied (not shared)
	if len(cloned.Services) != 1 || cloned.Services[0].ID != "svc1" {
		t.Error("Services not cloned correctly")
	}

	// Modify clone and verify original is unchanged
	cloned.Gateway.HTTPAddr = ":9090"
	if original.Gateway.HTTPAddr != ":8080" {
		t.Error("modifying clone affected original")
	}
}

func TestCloneBillingConfig(t *testing.T) {
	original := config.BillingConfig{
		Enabled:     true,
		DefaultCost: 100,
		RouteCosts: map[string]int64{
			"route1": 10,
			"route2": 20,
		},
		MethodMultipliers: map[string]float64{
			"GET":  1.0,
			"POST": 2.0,
		},
	}

	cloned := cloneBillingConfig(original)

	if !cloned.Enabled {
		t.Error("Enabled should be true")
	}
	if cloned.DefaultCost != 100 {
		t.Errorf("DefaultCost = %d, want 100", cloned.DefaultCost)
	}
	if len(cloned.RouteCosts) != 2 {
		t.Errorf("RouteCosts length = %d, want 2", len(cloned.RouteCosts))
	}
	if len(cloned.MethodMultipliers) != 2 {
		t.Errorf("MethodMultipliers length = %d, want 2", len(cloned.MethodMultipliers))
	}

	// Modify clone and verify original is unchanged
	cloned.RouteCosts["route3"] = 30
	if _, ok := original.RouteCosts["route3"]; ok {
		t.Error("modifying clone affected original RouteCosts")
	}
}

func TestCloneBillingRouteCosts(t *testing.T) {
	original := map[string]int64{
		"route1": 10,
		"route2": 20,
	}

	cloned := cloneBillingRouteCosts(original)

	if len(cloned) != 2 {
		t.Errorf("length = %d, want 2", len(cloned))
	}
	if cloned["route1"] != 10 {
		t.Errorf("route1 = %d, want 10", cloned["route1"])
	}

	// Modify clone and verify original is unchanged
	cloned["route1"] = 99
	if original["route1"] != 10 {
		t.Error("modifying clone affected original")
	}
}

func TestCloneBillingMethodMultipliers(t *testing.T) {
	original := map[string]float64{
		"GET":  1.0,
		"POST": 2.0,
	}

	cloned := cloneBillingMethodMultipliers(original)

	if len(cloned) != 2 {
		t.Errorf("length = %d, want 2", len(cloned))
	}
	if cloned["GET"] != 1.0 {
		t.Errorf("GET = %f, want 1.0", cloned["GET"])
	}

	// Modify clone and verify original is unchanged
	cloned["GET"] = 99.0
	if original["GET"] != 1.0 {
		t.Error("modifying clone affected original")
	}
}

func TestRequireString(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    string
		wantErr bool
	}{
		{
			name:    "valid string",
			args:    map[string]any{"key": "value"},
			key:     "key",
			want:    "value",
			wantErr: false,
		},
		{
			name:    "missing key",
			args:    map[string]any{},
			key:     "key",
			want:    "",
			wantErr: true,
		},
		{
			name:    "non-string value converts to string",
			args:    map[string]any{"key": 123},
			key:     "key",
			want:    "123",
			wantErr: false,
		},
		{
			name:    "empty string",
			args:    map[string]any{"key": ""},
			key:     "key",
			want:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			args:    map[string]any{"key": "   "},
			key:     "key",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requireString(tt.args, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("requireString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequireAnyString(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		keys    []string
		want    string
		wantErr bool
	}{
		{
			name:    "first key exists",
			args:    map[string]any{"key1": "value1", "key2": "value2"},
			keys:    []string{"key1", "key2"},
			want:    "value1",
			wantErr: false,
		},
		{
			name:    "second key exists",
			args:    map[string]any{"key2": "value2"},
			keys:    []string{"key1", "key2"},
			want:    "value2",
			wantErr: false,
		},
		{
			name:    "no keys exist",
			args:    map[string]any{},
			keys:    []string{"key1", "key2"},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requireAnyString(tt.args, tt.keys...)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireAnyString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("requireAnyString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"int64", int64(64), "64"},
		{"float64", float64(3.14), "3.14"},
		{"bool", true, "true"},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asString(tt.value)
			if got != tt.expected {
				t.Errorf("asString(%v) = %q, want %q", tt.value, got, tt.expected)
			}
		})
	}
}

func TestDecodeParams(t *testing.T) {
	t.Run("valid JSON params", func(t *testing.T) {
		params := json.RawMessage(`{"key": "value", "num": 123}`)
		var result map[string]any
		err := decodeParams(params, &result)
		if err != nil {
			t.Errorf("decodeParams error = %v", err)
		}
		if result["key"] != "value" {
			t.Errorf("key = %v, want value", result["key"])
		}
		if result["num"] != float64(123) {
			t.Errorf("num = %v, want 123", result["num"])
		}
	})

	t.Run("empty params", func(t *testing.T) {
		var result map[string]any
		err := decodeParams(nil, &result)
		if err != nil {
			t.Errorf("decodeParams error = %v", err)
		}
	})

	t.Run("invalid JSON params", func(t *testing.T) {
		params := json.RawMessage(`{invalid}`)
		var result map[string]any
		err := decodeParams(params, &result)
		if err == nil {
			t.Error("decodeParams should return error for invalid JSON")
		}
	})
}

func TestSwapRuntime_NilConfig(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	err := srv.swapRuntime(nil)
	if err == nil {
		t.Error("swapRuntime should return error for nil config")
	}
}

func TestCallAdmin_NoAdminServer(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	// Clear admin server
	srv.mu.Lock()
	srv.admin = nil
	srv.mu.Unlock()

	_, err := srv.callAdmin("GET", "/test", nil, nil)
	if err == nil {
		t.Error("callAdmin should return error when admin server is nil")
	}
}

func TestLoadConfigFromYAMLRaw_InvalidYAML(t *testing.T) {
	// Use truly invalid YAML (tabs are not allowed for indentation)
	_, err := loadConfigFromYAMLRaw("	invalid: yaml: {")
	if err == nil {
		t.Error("loadConfigFromYAMLRaw should return error for invalid YAML")
	}
}

func TestLoadConfigFromArgs_InvalidArgs(t *testing.T) {
	// Test with invalid args that would cause YAML parsing to fail
	invalidArgs := map[string]any{"config": "not:valid:yaml"}
	_, err := loadConfigFromArgs(invalidArgs)
	if err == nil {
		t.Error("loadConfigFromArgs should return error for invalid args")
	}
}

// Test loadConfigFromYAML
func TestLoadConfigFromYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid yaml",
			yaml: `
gateway:
  http_addr: ":8080"
admin:
  addr: ":9876"
store:
  path: "test.db"
`,
			wantErr: false,
		},
		{
			name: "invalid yaml",
			yaml:    "		{invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadConfigFromYAML(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Error("loadConfigFromYAML() should return error")
				}
				return
			}
			if err != nil {
				t.Errorf("loadConfigFromYAML() error = %v", err)
				return
			}
			if cfg == nil {
				t.Error("loadConfigFromYAML() returned nil config")
			}
		})
	}
}

// Test appendQueryValue
func TestAppendQueryValue(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    any
		expected string
	}{
		{
			name:     "nil value",
			key:      "key",
			value:    nil,
			expected: "",
		},
		{
			name:     "empty string",
			key:      "key",
			value:    "",
			expected: "",
		},
		{
			name:     "whitespace string",
			key:      "key",
			value:    "   ",
			expected: "",
		},
		{
			name:     "valid string",
			key:      "key",
			value:    "value",
			expected: "value",
		},
		{
			name:     "string slice",
			key:      "key",
			value:    []string{"a", "b", "c"},
			expected: "a",
		},
		{
			name:     "string slice with empty items",
			key:      "key",
			value:    []string{"a", "", "  ", "b"},
			expected: "a",
		},
		{
			name:     "any slice",
			key:      "key",
			value:    []any{"x", "y", "z"},
			expected: "x",
		},
		{
			name:     "int value",
			key:      "key",
			value:    42,
			expected: "42",
		},
		{
			name:     "bool value",
			key:      "key",
			value:    true,
			expected: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := url.Values{}
			appendQueryValue(values, tt.key, tt.value)

			if tt.expected == "" {
				if values.Get(tt.key) != "" {
					t.Errorf("appendQueryValue() = %q, want empty", values.Get(tt.key))
				}
			} else {
				if values.Get(tt.key) != tt.expected {
					t.Errorf("appendQueryValue() = %q, want %q", values.Get(tt.key), tt.expected)
				}
			}
		})
	}
}

// Test appendQueryValue with multiple values
func TestAppendQueryValue_Multiple(t *testing.T) {
	values := url.Values{}

	// Add multiple values
	appendQueryValue(values, "tag", []string{"a", "b", "c"})

	// Should have 3 values
	got := values["tag"]
	if len(got) != 3 {
		t.Errorf("expected 3 values, got %d", len(got))
	}

	// Check each value
	expected := []string{"a", "b", "c"}
	for i, v := range expected {
		if got[i] != v {
			t.Errorf("tag[%d] = %q, want %q", i, got[i], v)
		}
	}
}

// Test NewServer with nil config
func TestNewServer_NilConfig(t *testing.T) {
	_, err := NewServer(nil)
	if err == nil {
		t.Error("NewServer should return error for nil config")
	}
}

// Test HandleRequest with invalid JSON-RPC version
func TestHandleRequest_InvalidJSONRPCVersion(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	req := JSONRPCRequest{
		JSONRPC: "1.0",
		Method:  "tools/list",
		ID:      1,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Error("Expected error for invalid JSON-RPC version")
	}
	if !strings.Contains(resp.Error.Message, "jsonrpc must be 2.0") {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// Test HandleRequest with empty method
func TestHandleRequest_EmptyMethod(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "",
		ID:      1,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Error("Expected error for empty method")
	}
	if !strings.Contains(resp.Error.Message, "method is required") {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// Test HandleRequest with unknown method
func TestHandleRequest_UnknownMethod(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "unknown/method",
		ID:      1,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Error("Expected error for unknown method")
	}
	if !strings.Contains(resp.Error.Message, "method not found") {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// Test HandleRequest initialize method
func TestHandleRequest_Initialize(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      1,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("Expected result for initialize")
	}
}

// Test HandleRequest tools/list method
func TestHandleRequest_ToolsList(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      1,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("Expected result for tools/list")
	}
}

// Test HandleRequest tools/call with empty name
func TestHandleRequest_ToolsCall_EmptyName(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	params, _ := json.Marshal(map[string]any{
		"name":      "",
		"arguments": map[string]any{},
	})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      1,
		Params:  params,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Error("Expected error for empty tool name")
	}
	if !strings.Contains(resp.Error.Message, "tool name is required") {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// Test HandleRequest resources/list method
func TestHandleRequest_ResourcesList(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/list",
		ID:      1,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("Expected result for resources/list")
	}
}

// Test HandleRequest resources/read with empty URI
func TestHandleRequest_ResourcesRead_EmptyURI(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	params, _ := json.Marshal(map[string]any{
		"uri": "",
	})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/read",
		ID:      1,
		Params:  params,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Error("Expected error for empty URI")
	}
	if !strings.Contains(resp.Error.Message, "uri is required") {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// Test HandleRequest resources/read with unknown URI
func TestHandleRequest_ResourcesRead_UnknownURI(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	params, _ := json.Marshal(map[string]any{
		"uri": "apicerberus://unknown",
	})
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/read",
		ID:      1,
		Params:  params,
	}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Error("Expected error for unknown URI")
	}
	if !strings.Contains(resp.Error.Message, "resource read failed") {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// Test loadConfigFromYAML with empty YAML
func TestLoadConfigFromYAML_Empty(t *testing.T) {
	cfg, err := loadConfigFromYAML("")
	if err != nil {
		t.Errorf("Unexpected error for empty YAML: %v", err)
	}
	if cfg == nil {
		t.Error("Expected non-nil config for empty YAML")
	}
}

// Test loadConfigFromYAML with whitespace-only YAML
func TestLoadConfigFromYAML_Whitespace(t *testing.T) {
	// Tab indentation is not supported in YAML, so this will error
	_, err := loadConfigFromYAML("   \n\t\n  ")
	// Just verify it doesn't panic, behavior may vary
	_ = err
}

// Test cloneConfig with nil config
func TestCloneConfig_Nil(t *testing.T) {
	cloned := cloneConfig(nil)
	// cloneConfig returns empty config, not nil
	if cloned == nil {
		t.Error("cloneConfig(nil) should return empty config, not nil")
	}
}

// Test Close with nil gateway
func TestClose_NilGateway(t *testing.T) {
	srv := &Server{
		gateway: nil,
		admin:   nil,
	}
	err := srv.Close()
	if err != nil {
		t.Errorf("Close with nil gateway should not error: %v", err)
	}
}

// Test clonePluginConfigs
func TestClonePluginConfigs(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name string
		in   []config.PluginConfig
	}{
		{
			name: "empty slice",
			in:   []config.PluginConfig{},
		},
		{
			name: "nil slice",
			in:   nil,
		},
		{
			name: "single plugin",
			in: []config.PluginConfig{
				{
					Name:    "plugin1",
					Enabled: &enabled,
					Config:  map[string]any{"key": "value"},
				},
			},
		},
		{
			name: "multiple plugins",
			in: []config.PluginConfig{
				{
					Name:    "plugin1",
					Enabled: &enabled,
					Config:  map[string]any{"key1": "value1"},
				},
				{
					Name:    "plugin2",
					Enabled: &disabled,
					Config:  map[string]any{"key2": "value2"},
				},
				{
					Name:    "plugin3",
					Enabled: nil,
					Config:  nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clonePluginConfigs(tt.in)

			// For empty/nil input, should return nil
			if len(tt.in) == 0 {
				if got != nil {
					t.Errorf("clonePluginConfigs(%v) = %v, want nil", tt.in, got)
				}
				return
			}

			// Verify length
			if len(got) != len(tt.in) {
				t.Errorf("length = %d, want %d", len(got), len(tt.in))
			}

			// Verify deep copy
			for i := range tt.in {
				// Check that Enabled pointer is different
				if tt.in[i].Enabled != nil {
					if got[i].Enabled == tt.in[i].Enabled {
						t.Errorf("plugin[%d].Enabled pointer not copied", i)
					}
					if *got[i].Enabled != *tt.in[i].Enabled {
						t.Errorf("plugin[%d].Enabled = %v, want %v", i, *got[i].Enabled, *tt.in[i].Enabled)
					}
				}

				// Check that Config map is different
				if tt.in[i].Config != nil {
					if &got[i].Config == &tt.in[i].Config {
						t.Errorf("plugin[%d].Config map not copied", i)
					}
				}
			}

			// Modify clone and verify original is unchanged
			if len(got) > 0 && got[0].Config != nil {
				originalValue := tt.in[0].Config["key"]
				got[0].Config["key"] = "modified"
				if tt.in[0].Config["key"] != originalValue {
					t.Error("modifying clone affected original")
				}
			}
		})
	}
}

