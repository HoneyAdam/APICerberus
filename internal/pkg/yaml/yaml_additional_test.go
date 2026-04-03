package yaml

import (
	"reflect"
	"strings"
	"testing"
)

// Test decodeArray
func TestDecodeArray(t *testing.T) {
	tests := []struct {
		name    string
		dst     any
		src     any
		wantErr bool
	}{
		{
			name:    "valid array",
			dst:     func() any { s := make([]string, 2); return &s }(),
			src:     []any{"a", "b"},
			wantErr: false,
		},
		{
			name:    "not a sequence",
			dst:     func() any { s := make([]string, 1); return &s }(),
			src:     "not a sequence",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dstVal := reflect.ValueOf(tt.dst).Elem()
			err := decodeArray(dstVal, tt.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeArray() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test toSnakeCase
func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"Port", "port"},
		{"HTTPPort", "h_t_t_p_port"}, // function adds underscore before each uppercase letter
		{"MaxBodySize", "max_body_size"},
		{"simple", "simple"},
		{"A", "a"},
		{"Already_snake", "already_snake"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSnakeCase(tt.input)
			if got != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test coerceUint
func TestCoerceUint(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		bits    int
		want    uint64
		wantErr bool
	}{
		{"int", int(42), 64, 42, false},
		{"int8", int8(8), 64, 8, false},
		{"int16", int16(16), 64, 16, false},
		{"int32", int32(32), 64, 32, false},
		{"int64", int64(64), 64, 64, false},
		{"uint", uint(100), 64, 100, false},
		{"uint8", uint8(200), 64, 200, false},
		{"uint16", uint16(1000), 64, 1000, false},
		{"uint32", uint32(10000), 64, 10000, false},
		{"uint64", uint64(100000), 64, 100000, false},
		{"string valid", "42", 64, 42, false},
		{"string invalid", "not a number", 64, 0, true},
		{"unsupported type", 3.14, 64, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceUint(tt.input, tt.bits)
			if (err != nil) != tt.wantErr {
				t.Errorf("coerceUint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("coerceUint() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test writeMap
func TestWriteMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: "{}",
		},
		{
			name:     "nil map",
			input:    nil,
			expected: "{}",
		},
		{
			name:     "simple map",
			input:    map[string]string{"a": "1", "b": "2"},
			expected: "a:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			val := reflect.ValueOf(tt.input)
			err := writeMap(&b, val, 0)
			if err != nil {
				t.Errorf("writeMap() error = %v", err)
				return
			}
			got := b.String()
			if !strings.Contains(got, tt.expected) {
				t.Errorf("writeMap() = %q, should contain %q", got, tt.expected)
			}
		})
	}
}

// Test Node.Kind methods
func TestNodeKind(t *testing.T) {
	t.Run("NodeMap.Kind", func(t *testing.T) {
		nm := newNodeMap()
		if nm.Kind() != KindMap {
			t.Errorf("NodeMap.Kind() = %v, want %v", nm.Kind(), KindMap)
		}
	})

	t.Run("NodeSequence.Kind", func(t *testing.T) {
		ns := &NodeSequence{Items: []Node{}}
		if ns.Kind() != KindSequence {
			t.Errorf("NodeSequence.Kind() = %v, want %v", ns.Kind(), KindSequence)
		}
	})

	t.Run("NodeScalar.Kind", func(t *testing.T) {
		ns := &NodeScalar{Value: "test"}
		if ns.Kind() != KindScalar {
			t.Errorf("NodeScalar.Kind() = %v, want %v", ns.Kind(), KindScalar)
		}
	})
}

// Test token.isBlankLine
func TestTokenIsBlankLine(t *testing.T) {
	tests := []struct {
		name     string
		token    token
		expected bool
	}{
		{
			name:     "blank line",
			token:    token{original: "   ", cleaned: ""},
			expected: true,
		},
		{
			name:     "non-blank line",
			token:    token{original: "key: value", cleaned: "key: value"},
			expected: false,
		},
		{
			name:     "empty line",
			token:    token{original: "", cleaned: ""},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.token.isBlankLine()
			if got != tt.expected {
				t.Errorf("token.isBlankLine() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test yamlFieldName with snake case conversion
func TestYamlFieldName_SnakeCase(t *testing.T) {
	type TestStruct struct {
		HTTPPort    int    `yaml:"http_port"`
		MaxBodySize int    // no tag, should convert to snake_case
		SkipField   string `yaml:"-"`
	}

	typ := reflect.TypeOf(TestStruct{})

	tests := []struct {
		fieldName string
		wantName  string
		wantSkip  bool
	}{
		{"HTTPPort", "http_port", false},
		{"MaxBodySize", "max_body_size", false},
		{"SkipField", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			field, _ := typ.FieldByName(tt.fieldName)
			name, skip := yamlFieldName(field)
			if skip != tt.wantSkip {
				t.Errorf("yamlFieldName() skip = %v, want %v", skip, tt.wantSkip)
			}
			if name != tt.wantName {
				t.Errorf("yamlFieldName() name = %v, want %v", name, tt.wantName)
			}
		})
	}
}
