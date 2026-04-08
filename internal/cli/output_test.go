package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNormalizeOutputMode(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", "table", false},
		{"table", "table", false},
		{"TABLE", "table", false},
		{"  table  ", "table", false},
		{"json", "json", false},
		{"JSON", "json", false},
		{"  json  ", "json", false},
		{"xml", "", true},
		{"yaml", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeOutputMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeOutputMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("normalizeOutputMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrintJSON(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	data := map[string]string{"key": "value", "test": "data"}
	err := printJSON(data)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("printJSON() error = %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var result map[string]string
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Errorf("printJSON() output is not valid JSON: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("printJSON() output key = %v, want value", result["key"])
	}
}

func TestPrintTable(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	headers := []string{"Name", "Value", "Status"}
	rows := [][]string{
		{"Alice", "100", "Active"},
		{"Bob", "200", "Inactive"},
	}
	printTable(headers, rows)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Name") {
		t.Error("printTable() output missing Name header")
	}
	if !strings.Contains(output, "Alice") {
		t.Error("printTable() output missing Alice row")
	}
}

func TestPrintTable_EmptyHeaders(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printTable([]string{}, [][]string{{"data"}})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("printTable() with empty headers should output nothing, got: %q", output)
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		value string
		width int
		want  string
	}{
		{"hello", 10, "hello     "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"},
		{"", 5, "     "},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := padRight(tt.value, tt.width)
			if got != tt.want {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.value, tt.width, got, tt.want)
			}
		})
	}
}

func TestTruncateCell(t *testing.T) {
	tests := []struct {
		value string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactlyten", 10, "exactlyten"},
		{"this is a very long string", 10, "this is..."},
		{"", 10, ""},
		{"test", 0, "test"},
		{"test", -1, "test"},
		{"test", 2, "te"},
		{"test", 3, "tes"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%d", tt.value, tt.max), func(t *testing.T) {
			got := truncateCell(tt.value, tt.max)
			if got != tt.want {
				t.Errorf("truncateCell(%q, %d) = %q, want %q", tt.value, tt.max, got, tt.want)
			}
		})
	}
}

func TestPrintMapAsKeyValues(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	data := map[string]any{
		"name": "test",
		"id":   123,
	}
	printMapAsKeyValues(data)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "name:") {
		t.Error("printMapAsKeyValues() output missing name key")
	}
	if !strings.Contains(output, "test") {
		t.Error("printMapAsKeyValues() output missing test value")
	}
}

func TestPrintMapAsKeyValues_Empty(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printMapAsKeyValues(map[string]any{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "(empty)") {
		t.Error("printMapAsKeyValues() with empty map should output '(empty)'")
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{"  spaced  ", "spaced"},
		{123, "123"},
		{int64(456), "456"},
		{float64(3.14), "3.14"},
		{float64(42.0), "42"},
		{float32(2.5), "2.5"},
		{true, "true"},
		{false, "false"},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "2024-01-01 00:00:00 +0000 UTC"},
		{map[string]int{"a": 1}, `{"a":1}`},
		{[]int{1, 2, 3}, "[1,2,3]"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T_%v", tt.input, tt.input), func(t *testing.T) {
			got := asString(tt.input)
			if got != tt.want {
				t.Errorf("asString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAsBool(t *testing.T) {
	tests := []struct {
		input    any
		fallback bool
		want     bool
	}{
		{true, false, true},
		{false, true, false},
		{"true", false, true},
		{"TRUE", false, true},
		{"yes", false, true},
		{"on", false, true},
		{"1", false, true},
		{"false", true, false},
		{"FALSE", true, false},
		{"no", true, false},
		{"off", true, false},
		{"0", true, false},
		{"maybe", true, true},
		{123, true, true},
		{nil, false, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v_%v", tt.input, tt.fallback), func(t *testing.T) {
			got := asBool(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("asBool(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestAsInt(t *testing.T) {
	tests := []struct {
		input    any
		fallback int
		want     int
	}{
		{42, 0, 42},
		{int64(100), 0, 100},
		{float64(3.14), 0, 3},
		{"123", 0, 123},
		{"  456  ", 0, 456},
		{"not a number", 999, 999},
		{true, 0, 0},
		{nil, 0, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v_%v", tt.input, tt.fallback), func(t *testing.T) {
			got := asInt(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("asInt(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestAsSlice(t *testing.T) {
	tests := []struct {
		input any
		want  []any
	}{
		{[]any{1, 2, 3}, []any{1, 2, 3}},
		{[]map[string]any{{"a": 1}, {"b": 2}}, []any{map[string]any{"a": 1}, map[string]any{"b": 2}}},
		{"not a slice", nil},
		{123, nil},
		{nil, nil},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T", tt.input), func(t *testing.T) {
			got := asSlice(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("asSlice() length = %v, want %v", len(got), len(tt.want))
			}
		})
	}
}

func TestAsMap(t *testing.T) {
	tests := []struct {
		input any
		want  map[string]any
	}{
		{map[string]any{"a": 1}, map[string]any{"a": 1}},
		{"not a map", nil},
		{123, nil},
		{nil, nil},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T", tt.input), func(t *testing.T) {
			got := asMap(tt.input)
			if tt.want == nil && got != nil {
				t.Errorf("asMap() = %v, want nil", got)
			}
			if tt.want != nil && got == nil {
				t.Errorf("asMap() = nil, want %v", tt.want)
			}
		})
	}
}

func TestFindFirst(t *testing.T) {
	m := map[string]any{
		"name":  "test",
		"value": 123,
	}

	tests := []struct {
		m      map[string]any
		keys   []string
		want   any
		wantOk bool
	}{
		{m, []string{"name"}, "test", true},
		{m, []string{"value"}, 123, true},
		{m, []string{"missing"}, nil, false},
		{m, []string{"missing", "name"}, "test", true},
		{nil, []string{"name"}, nil, false},
		{m, []string{}, nil, false},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, "_"), func(t *testing.T) {
			got, ok := findFirst(tt.m, tt.keys...)
			if ok != tt.wantOk {
				t.Errorf("findFirst() ok = %v, want %v", ok, tt.wantOk)
			}
			if tt.wantOk && got != tt.want {
				t.Errorf("findFirst() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindString(t *testing.T) {
	m := map[string]any{
		"name": "test",
		"num":  123,
	}

	tests := []struct {
		keys   []string
		want   string
		wantOk bool
	}{
		{[]string{"name"}, "test", true},
		{[]string{"num"}, "123", true},
		{[]string{"missing"}, "", false},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, "_"), func(t *testing.T) {
			got, ok := findString(m, tt.keys...)
			if ok != tt.wantOk {
				t.Errorf("findString() ok = %v, want %v", ok, tt.wantOk)
			}
			if got != tt.want {
				t.Errorf("findString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequireArg(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		want    string
		wantErr bool
	}{
		{"value", "arg", "value", false},
		{"  value  ", "arg", "value", false},
		{"", "arg", "", true},
		{"   ", "arg", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := requireArg(tt.input, tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireArg(%q, %q) error = %v, wantErr %v", tt.input, tt.name, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("requireArg(%q, %q) = %q, want %q", tt.input, tt.name, got, tt.want)
			}
		})
	}
}

func TestRequireInt(t *testing.T) {
	tests := []struct {
		value   int
		name    string
		want    int
		wantErr bool
	}{
		{1, "count", 1, false},
		{100, "limit", 100, false},
		{0, "count", 0, true},
		{-1, "count", 0, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.value), func(t *testing.T) {
			got, err := requireInt(tt.value, tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireInt(%d, %q) error = %v, wantErr %v", tt.value, tt.name, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("requireInt(%d, %q) = %d, want %d", tt.value, tt.name, got, tt.want)
			}
		})
	}
}
