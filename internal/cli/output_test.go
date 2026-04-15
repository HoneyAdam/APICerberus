package cli

import (
	"testing"
	"time"
)

func TestTruncateCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"hi", 3, "hi"},
		{"abcdef", 3, "abc"},
		{"abcdef", 0, "abcdef"},
		{"abcdef", -1, "abcdef"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input+"/"+string(rune('0'+tt.max)), func(t *testing.T) {
			t.Parallel()
			got := truncateCell(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncateCell(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	t.Parallel()
	got := padRight("hi", 5)
	if got != "hi   " {
		t.Errorf("padRight(%q, 5) = %q, want %q", "hi", got, "hi   ")
	}
	// Already wide enough
	got2 := padRight("hello", 3)
	if got2 != "hello" {
		t.Errorf("padRight(%q, 3) = %q, want %q", "hello", got2, "hello")
	}
}

func TestAsInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    any
		fallback int
		want     int
	}{
		{"int", 42, 0, 42},
		{"int64", int64(99), 0, 99},
		{"float64", float64(7), 0, 7},
		{"string_valid", "123", 0, 123},
		{"string_invalid", "abc", 10, 10},
		{"nil", nil, -1, -1},
		{"bool", true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := asInt(tt.value, tt.fallback)
			if got != tt.want {
				t.Errorf("asInt(%v, %d) = %d, want %d", tt.value, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestAsString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, ""},
		{"string", "  hello  ", "hello"},
		{"time", time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC), "2026-01-15T10:30:00Z"},
		{"int", 42, "42"},
		{"int64", int64(100), "100"},
		{"float64_int", float64(5.0), "5"},
		{"float64_frac", float64(3.14), "3.14"},
		{"float32_int", float32(7.0), "7"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"slice", []string{"a", "b"}, `["a","b"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := asString(tt.value)
			if got != tt.want {
				t.Errorf("asString(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
