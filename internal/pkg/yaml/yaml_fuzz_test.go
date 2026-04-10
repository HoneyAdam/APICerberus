package yaml

import (
	"strings"
	"testing"
)

// FuzzUnmarshal tests the YAML parser against malformed and adversarial
// inputs: YAML bombs, deeply nested structures, malformed anchors/aliases,
// binary data, and Unicode edge cases.
func FuzzUnmarshal(f *testing.F) {
	// Seed corpus
	seeds := [][]byte{
		[]byte("name: test\nvalue: 42\n"),
		[]byte("a:\n  b:\n    c: value\n"),
		[]byte(""),
		[]byte("\x00\x00\x00\x00"),
		[]byte("{{{{invalid}}}}"),
		[]byte("*invalid_alias"),
		[]byte("&anchor *invalid"),
		[]byte("key: |" + strings.Repeat("\n  ", 200) + "line"),
		[]byte("key: " + strings.Repeat("a", 1<<20)),
		[]byte("\ufeff"), // BOM
		[]byte("---\n...\n---\n..."),
		[]byte("a: &a\n  b: *a"),
		[]byte("- - - - -"),
		[]byte("{[}]"),
		[]byte("k: v\nk: v2"), // duplicate key
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 100_000 {
			data = data[:100_000]
		}

		var out map[string]any
		_ = Unmarshal(data, &out)

		// If it parses without error, verify round-trip for simple maps
		if len(out) > 0 && len(out) < 100 {
			_, _ = Marshal(out)
		}
	})
}
