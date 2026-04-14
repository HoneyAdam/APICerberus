package store

import "testing"

func TestSanitizeFTS5Query(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", `""`},
		{"simple word", "hello", `"hello"`},
		{"two words", "hello world", `"hello" OR "world"`},
		{"special chars", `test"query{bad}data(filtered:stuff*more)`, `"testquerybaddatafilteredstuffmore"`},
		{"spaces only", "   ", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFTS5Query(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFTS5Query(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
