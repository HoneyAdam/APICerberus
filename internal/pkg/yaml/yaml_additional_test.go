package yaml

import (
	"reflect"
	"strings"
	"testing"
	"time"
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

// Test coerceString function
func TestCoerceString(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{"string", "hello", "hello", false},
		{"bool true", true, "true", false},
		{"bool false", false, "false", false},
		{"int", int(42), "42", false},
		{"int8", int8(8), "8", false},
		{"int16", int16(16), "16", false},
		{"int32", int32(32), "32", false},
		{"int64", int64(64), "64", false},
		{"uint", uint(100), "100", false},
		{"uint8", uint8(200), "200", false},
		{"uint16", uint16(1000), "1000", false},
		{"uint32", uint32(10000), "10000", false},
		{"uint64", uint64(100000), "100000", false},
		{"float32", float32(3.14), "3.14", false},
		{"float64", float64(2.718), "2.718", false},
		{"unsupported type", []int{1, 2, 3}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("coerceString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("coerceString() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test coerceBool function
func TestCoerceBool(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    bool
		wantErr bool
	}{
		{"bool true", true, true, false},
		{"bool false", false, false, false},
		{"string true", "true", true, false},
		{"string TRUE", "TRUE", true, false},
		{"string yes", "yes", true, false},
		{"string on", "on", true, false},
		{"string 1", "1", true, false},
		{"string false", "false", false, false},
		{"string no", "no", false, false},
		{"string off", "off", false, false},
		{"string 0", "0", false, false},
		{"string invalid", "invalid", false, true},
		{"int", 42, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceBool(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("coerceBool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("coerceBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test coerceInt function
func TestCoerceInt(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		bits    int
		want    int64
		wantErr bool
	}{
		{"int", int(42), 64, 42, false},
		{"int8", int8(8), 64, 8, false},
		{"int16", int16(16), 64, 16, false},
		{"int32", int32(32), 64, 32, false},
		{"int64", int64(64), 64, 64, false},
		{"uint", uint(100), 64, 100, false},
		{"uint8", uint8(200), 64, 200, false},
		{"float32", float32(3.14), 64, 3, false},
		{"float64", float64(2.718), 64, 2, false},
		{"string valid", "42", 64, 42, false},
		{"string invalid", "not a number", 64, 0, true},
		{"unsupported type", []int{1, 2, 3}, 64, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceInt(tt.input, tt.bits)
			if (err != nil) != tt.wantErr {
				t.Errorf("coerceInt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("coerceInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test coerceFloat function
func TestCoerceFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		bits    int
		want    float64
		wantErr bool
	}{
		{"float32", float32(3.14), 64, 3.14, false},
		{"float64", float64(2.718), 64, 2.718, false},
		{"int", int(42), 64, 42.0, false},
		{"int8", int8(8), 64, 8.0, false},
		{"int16", int16(16), 64, 16.0, false},
		{"int32", int32(32), 64, 32.0, false},
		{"int64", int64(64), 64, 64.0, false},
		{"uint", uint(100), 64, 100.0, false},
		{"uint8", uint8(200), 64, 200.0, false},
		{"string valid", "3.14159", 64, 3.14159, false},
		{"string invalid", "not a float", 64, 0, true},
		{"unsupported type", []float64{1.1, 2.2}, 64, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceFloat(tt.input, tt.bits)
			if (err != nil) != tt.wantErr {
				t.Errorf("coerceFloat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			// Compare with tolerance for floating point
			if diff := got - tt.want; diff < -0.0001 || diff > 0.0001 {
				t.Errorf("coerceFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test coerceDuration function
func TestCoerceDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    time.Duration
		wantErr bool
	}{
		{"duration", time.Hour + 30*time.Minute, time.Hour + 30*time.Minute, false},
		{"string valid", "1h30m", time.Hour + 30*time.Minute, false},
		{"string invalid", "invalid duration", 0, true},
		{"int", int(1000), 1000, false},
		{"int64", int64(2000), 2000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coerceDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("coerceDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("coerceDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test writeSequence with various types
func TestWriteSequence(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "nil slice",
			input:    ([]string)(nil),
			expected: "[]",
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: "[]",
		},
		{
			name:     "string slice",
			input:    []string{"a", "b", "c"},
			expected: "- a",
		},
		{
			name:     "int slice",
			input:    []int{1, 2, 3},
			expected: "- 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			val := reflect.ValueOf(tt.input)
			err := writeSequence(&b, val, 0)
			if err != nil {
				t.Errorf("writeSequence() error = %v", err)
				return
			}
			got := b.String()
			if !strings.Contains(got, tt.expected) {
				t.Errorf("writeSequence() = %q, should contain %q", got, tt.expected)
			}
		})
	}
}

// Test writeYAMLValue with nil and pointer values
func TestWriteYAMLValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "nil pointer",
			input:    (*string)(nil),
			expected: "null",
		},
		{
			name:     "nil interface",
			input:    (any)(nil),
			expected: "null",
		},
		{
			name:     "simple string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "boolean true",
			input:    true,
			expected: "true",
		},
		{
			name:     "boolean false",
			input:    false,
			expected: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			val := reflect.ValueOf(tt.input)
			err := writeYAMLValue(&b, val, 0)
			if err != nil {
				t.Errorf("writeYAMLValue() error = %v", err)
				return
			}
			got := b.String()
			if !strings.Contains(got, tt.expected) {
				t.Errorf("writeYAMLValue() = %q, should contain %q", got, tt.expected)
			}
		})
	}
}

// Test Marshal with empty input
func TestMarshal_Empty(t *testing.T) {
	result, err := Marshal("")
	if err != nil {
		t.Errorf("Marshal(\"\") error = %v", err)
	}
	// Empty string is marshaled as quoted empty string
	if string(result) != `""`+"\n" {
		t.Errorf("Marshal(\"\") = %q, want quoted empty string", string(result))
	}
}

// Test Marshal with struct
func TestMarshal_Struct(t *testing.T) {
	type TestStruct struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	input := TestStruct{Name: "test", Value: 42}
	result, err := Marshal(input)
	if err != nil {
		t.Errorf("Marshal() error = %v", err)
		return
	}
	if !strings.Contains(string(result), "name: test") {
		t.Errorf("Marshal() = %q, should contain 'name: test'", string(result))
	}
	if !strings.Contains(string(result), "value: 42") {
		t.Errorf("Marshal() = %q, should contain 'value: 42'", string(result))
	}
}
