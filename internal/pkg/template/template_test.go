package template

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRenderVariables(t *testing.T) {
	t.Parallel()

	ctx := Context{
		Body:            `{"hello":"world"}`,
		Timestamp:       time.Unix(1700000000, 0).UTC(),
		UpstreamLatency: 125 * time.Millisecond,
		ConsumerID:      "consumer-1",
		RouteName:       "users-route",
		RequestID:       "req-abc",
		RemoteAddr:      "203.0.113.10",
		Headers: http.Header{
			"X-Custom": []string{"custom-value"},
		},
	}

	input := "$body|$timestamp_ms|$timestamp_iso|$upstream_latency_ms|$consumer_id|$route_name|$request_id|$remote_addr|$header.X-Custom"
	got := Render(input, ctx)

	expected := `{"hello":"world"}|1700000000000|2023-11-14T22:13:20Z|125|consumer-1|users-route|req-abc|203.0.113.10|custom-value`
	if got != expected {
		t.Fatalf("unexpected render output:\nwant: %s\ngot:  %s", expected, got)
	}
}

func TestApplyJSONPatchAddRemoveRenameNested(t *testing.T) {
	t.Parallel()

	body := []byte(`{"a":1,"nested":{"b":2,"c":3}}`)
	out, err := ApplyJSONPatch(body, JSONPatch{
		Add: map[string]any{
			"nested.new":      "x",
			"metadata.source": "gateway",
		},
		Remove: []string{"nested.c"},
		Rename: map[string]string{
			"a":        "a_renamed",
			"nested.b": "nested.renamed",
		},
	})
	if err != nil {
		t.Fatalf("ApplyJSONPatch error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if _, exists := got["a"]; exists {
		t.Fatalf("expected key a to be renamed")
	}
	if got["a_renamed"].(float64) != 1 {
		t.Fatalf("expected a_renamed=1")
	}

	nested := got["nested"].(map[string]any)
	if _, exists := nested["b"]; exists {
		t.Fatalf("expected nested.b to be renamed")
	}
	if nested["renamed"].(float64) != 2 {
		t.Fatalf("expected nested.renamed=2")
	}
	if _, exists := nested["c"]; exists {
		t.Fatalf("expected nested.c removed")
	}
	if nested["new"].(string) != "x" {
		t.Fatalf("expected nested.new=x")
	}

	meta := got["metadata"].(map[string]any)
	if meta["source"].(string) != "gateway" {
		t.Fatalf("expected metadata.source=gateway")
	}
}

func TestApplyJSONPatchNestedPathCreation(t *testing.T) {
	t.Parallel()

	out, err := ApplyJSONPatch([]byte(`{}`), JSONPatch{
		Add: map[string]any{
			"deep.inner.value": 42,
		},
	})
	if err != nil {
		t.Fatalf("ApplyJSONPatch error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	deep := got["deep"].(map[string]any)
	inner := deep["inner"].(map[string]any)
	if inner["value"].(float64) != 42 {
		t.Fatalf("expected deep.inner.value=42")
	}
}

func TestTransformTemplateModeFullReplacement(t *testing.T) {
	t.Parallel()

	opts := TransformOptions{
		Mode:     ModeTemplate,
		Template: "wrapped=$body consumer=$consumer_id header=$header.X-Test",
	}
	ctx := Context{
		ConsumerID: "consumer-z",
		Headers: http.Header{
			"X-Test": []string{"ok"},
		},
	}
	out, err := Transform([]byte(`{"k":"v"}`), opts, ctx)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if got := string(out); got != `wrapped={"k":"v"} consumer=consumer-z header=ok` {
		t.Fatalf("unexpected template transform output: %q", got)
	}
}

func TestTransformUnsupportedMode(t *testing.T) {
	t.Parallel()

	_, err := Transform([]byte(`{}`), TransformOptions{Mode: Mode("unknown")}, Context{})
	if err == nil || !strings.Contains(err.Error(), "unsupported transform mode") {
		t.Fatalf("expected unsupported mode error, got %v", err)
	}
}

// Additional tests for 100% coverage

func TestRenderZeroTimestamp(t *testing.T) {
	t.Parallel()

	ctx := Context{
		Body:       "test",
		Timestamp:  time.Time{}, // Zero timestamp
		ConsumerID: "consumer-1",
	}

	input := "$body|$timestamp_ms|$consumer_id"
	got := Render(input, ctx)

	// Should use current time when timestamp is zero
	if !strings.Contains(got, "test|") {
		t.Fatalf("expected body to be rendered, got: %s", got)
	}
	if !strings.Contains(got, "|consumer-1") {
		t.Fatalf("expected consumer_id to be rendered, got: %s", got)
	}
}

func TestRenderNilHeaders(t *testing.T) {
	t.Parallel()

	ctx := Context{
		Body:       "test",
		Timestamp:  time.Unix(1700000000, 0).UTC(),
		ConsumerID: "consumer-1",
		Headers:    nil, // Nil headers
	}

	input := "$body|$header.X-Custom"
	got := Render(input, ctx)

	// Should handle nil headers gracefully
	expected := "test|"
	if got != expected {
		t.Fatalf("expected %q, got: %q", expected, got)
	}
}

func TestTransformEmptyBody(t *testing.T) {
	t.Parallel()

	opts := TransformOptions{
		Mode:     ModeTemplate,
		Template: "body=$body",
	}
	ctx := Context{
		Body:       "", // Empty body
		ConsumerID: "consumer-z",
	}
	out, err := Transform([]byte(`{"key":"value"}`), opts, ctx)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	// When ctx.Body is empty, it should use the input body
	if got := string(out); !strings.Contains(got, `{"key":"value"}`) {
		t.Fatalf("expected original body, got: %q", got)
	}
}

func TestTransformEmptyTemplate(t *testing.T) {
	t.Parallel()

	opts := TransformOptions{
		Mode:     ModeTemplate,
		Template: "   ", // Empty/whitespace template
	}
	ctx := Context{
		Body:       `{"key":"value"}`,
		ConsumerID: "consumer-z",
	}
	out, err := Transform([]byte(`{"key":"value"}`), opts, ctx)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	// When template is empty, should use "$body"
	if got := string(out); !strings.Contains(got, `{"key":"value"}`) {
		t.Fatalf("expected body output, got: %q", got)
	}
}

func TestTransformModeEmpty(t *testing.T) {
	t.Parallel()

	// Empty mode should default to ModeJSON
	opts := TransformOptions{
		Mode: "",
		JSON: JSONPatch{
			Add: map[string]any{"key": "value"},
		},
	}
	ctx := Context{}
	out, err := Transform([]byte(`{}`), opts, ctx)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("expected key=value, got: %v", got)
	}
}

func TestApplyJSONPatchParseError(t *testing.T) {
	t.Parallel()

	_, err := ApplyJSONPatch([]byte(`not valid json`), JSONPatch{})
	if err == nil || !strings.Contains(err.Error(), "parse json body") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestApplyJSONPatchNotObject(t *testing.T) {
	t.Parallel()

	_, err := ApplyJSONPatch([]byte(`[1, 2, 3]`), JSONPatch{})
	if err == nil || !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("expected object error, got: %v", err)
	}
}

func TestApplyJSONPatchEmptyBody(t *testing.T) {
	t.Parallel()

	out, err := ApplyJSONPatch([]byte(``), JSONPatch{
		Add: map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("ApplyJSONPatch error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("expected key=value, got: %v", got)
	}
}

func TestApplyJSONPatchWhitespaceBody(t *testing.T) {
	t.Parallel()

	out, err := ApplyJSONPatch([]byte(`
	  `), JSONPatch{
		Add: map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("ApplyJSONPatch error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("expected key=value, got: %v", got)
	}
}

func TestApplyJSONPatchSetPathEmpty(t *testing.T) {
	t.Parallel()

	_, err := ApplyJSONPatch([]byte(`{}`), JSONPatch{
		Add: map[string]any{"": "value"},
	})
	if err == nil || !strings.Contains(err.Error(), "path is empty") {
		t.Fatalf("expected empty path error, got: %v", err)
	}
}

func TestApplyJSONPatchSetPathOverwrite(t *testing.T) {
	t.Parallel()

	// Test overwriting non-map value with a map
	out, err := ApplyJSONPatch([]byte(`{"a": "string value"}`), JSONPatch{
		Add: map[string]any{"a.b": "nested value"},
	})
	if err != nil {
		t.Fatalf("ApplyJSONPatch error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	a := got["a"].(map[string]any)
	if a["b"] != "nested value" {
		t.Fatalf("expected a.b=nested value, got: %v", got)
	}
}

func TestGetPathEmpty(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": 1}
	value, found := getPath(root, "")
	if found {
		t.Fatalf("expected not found for empty path, got: %v", value)
	}
}

func TestGetPathNotFound(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": map[string]any{"b": 1}}
	value, found := getPath(root, "a.x.c")
	if found {
		t.Fatalf("expected not found, got: %v", value)
	}
}

func TestGetPathNotMap(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": "not a map"}
	value, found := getPath(root, "a.b")
	if found {
		t.Fatalf("expected not found when path goes through non-map, got: %v", value)
	}
}

func TestDeletePathEmpty(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": 1}
	deleted := deletePath(root, "")
	if deleted {
		t.Fatalf("expected not deleted for empty path")
	}
}

func TestDeletePathNotFound(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": map[string]any{"b": 1}}
	deleted := deletePath(root, "a.x.c")
	if deleted {
		t.Fatalf("expected not deleted when path not found")
	}
}

func TestDeletePathNotMap(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": "not a map"}
	deleted := deletePath(root, "a.b")
	if deleted {
		t.Fatalf("expected not deleted when path goes through non-map")
	}
}

func TestDeletePathKeyNotFound(t *testing.T) {
	t.Parallel()

	root := map[string]any{"a": map[string]any{"b": 1}}
	deleted := deletePath(root, "a.c")
	if deleted {
		t.Fatalf("expected not deleted when final key not found")
	}
}

func TestSplitPathEmpty(t *testing.T) {
	t.Parallel()

	parts := splitPath("")
	if parts != nil && len(parts) != 0 {
		t.Fatalf("expected nil/empty for empty path, got: %v", parts)
	}
}

func TestSplitPathWhitespace(t *testing.T) {
	t.Parallel()

	parts := splitPath("  a  .  b  .  c  ")
	expected := []string{"a", "b", "c"}
	if len(parts) != len(expected) {
		t.Fatalf("expected %v, got: %v", expected, parts)
	}
	for i, p := range parts {
		if p != expected[i] {
			t.Fatalf("expected %v, got: %v", expected, parts)
		}
	}
}

func TestSplitPathEmptyParts(t *testing.T) {
	t.Parallel()

	parts := splitPath("a..b...c")
	expected := []string{"a", "b", "c"}
	if len(parts) != len(expected) {
		t.Fatalf("expected %v, got: %v", expected, parts)
	}
}

func TestApplyJSONPatchRenameNotFound(t *testing.T) {
	t.Parallel()

	// Renaming a path that doesn't exist should be silently ignored
	out, err := ApplyJSONPatch([]byte(`{"a": 1}`), JSONPatch{
		Rename: map[string]string{"x": "y"},
	})
	if err != nil {
		t.Fatalf("ApplyJSONPatch error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got["a"] != float64(1) {
		t.Fatalf("expected a=1 unchanged, got: %v", got)
	}
}
