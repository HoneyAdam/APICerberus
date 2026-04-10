package jsonutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FuzzReadJSON tests JSON request body parsing against malformed and
// adversarial inputs: truncated JSON, malformed Unicode, deeply nested
// structures, oversized payloads, and injection patterns.
func FuzzReadJSON(f *testing.F) {
	seeds := []string{
		`{"name":"test"}`,
		`{}`,
		`{"a":1,"b":"c","d":[1,2,3]}`,
		``,
		`{`,
		`{"trailing":}`,
		`[1,2,3]`,
		`"just a string"`,
		`{"key":"\u0000\u0000"}`,
		`{"key":"` + strings.Repeat("a", 1<<16) + `"}`,
		`{"bad":"\x00"}`,
		`{bad json}`,
		`{"nested":{"a":{"b":{"c":{"d":{"e":{"f":{"g":{"h":{"i":{"j":{"k":{"l":{"m":{"n":{"o":{"p":{"q":{"r":{"s":{"t":{"u":{"v":{"w":{"x":{"y":{"z":{"deep":true}}}}}}}}}}}}}}}}}}}}}}}}}}`,
		`12345678901234567890`,
		`null`,
		`true`,
		`false`,
		`{"key":1e999}`,
		`{"a":1,"a":2}`,
		`{"\uFEFF":"bom key"}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 1<<20 {
			body = body[:1<<20]
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var out map[string]any
		_ = ReadJSON(req, &out, 1024*1024) // 1MB limit

		// Verify WriteJSON can encode the result back
		rr := httptest.NewRecorder()
		_ = WriteJSON(rr, http.StatusOK, out)
	})
}
