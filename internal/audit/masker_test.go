package audit

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMaskerMaskHeaders(t *testing.T) {
	t.Parallel()

	masker := NewMasker([]string{"Authorization", "X-API-Key"}, nil, "***")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer abc")
	headers.Set("X-API-Key", "key-123")
	headers.Set("X-Trace", "trace-1")

	masked := masker.MaskHeaders(headers)
	if masked["Authorization"] != "***" {
		t.Fatalf("authorization not masked: %#v", masked["Authorization"])
	}
	apiKeyValue := masked["X-API-Key"]
	if apiKeyValue == nil {
		apiKeyValue = masked["X-Api-Key"]
	}
	if apiKeyValue != "***" {
		t.Fatalf("x-api-key not masked: %#v", apiKeyValue)
	}
	if masked["X-Trace"] != "trace-1" {
		t.Fatalf("unexpected trace header: %#v", masked["X-Trace"])
	}
}

func TestMaskerMaskBodyNestedFields(t *testing.T) {
	t.Parallel()

	masker := NewMasker(nil, []string{"password", "user.token", "items.secret"}, "REDACTED")
	raw := []byte(`{"password":"abc","user":{"token":"t-1"},"items":[{"secret":"s-1"},{"secret":"s-2"}]}`)

	maskedRaw := masker.MaskBody(raw)
	var payload map[string]any
	if err := json.Unmarshal(maskedRaw, &payload); err != nil {
		t.Fatalf("unmarshal masked body: %v", err)
	}
	if payload["password"] != "REDACTED" {
		t.Fatalf("password field not masked: %#v", payload["password"])
	}
	user := payload["user"].(map[string]any)
	if user["token"] != "REDACTED" {
		t.Fatalf("nested token not masked: %#v", user["token"])
	}
	items := payload["items"].([]any)
	for _, item := range items {
		entry := item.(map[string]any)
		if entry["secret"] != "REDACTED" {
			t.Fatalf("array secret not masked: %#v", entry["secret"])
		}
	}
}

func TestMaskerMaskHeadersInto(t *testing.T) {
	t.Parallel()

	masker := NewMasker([]string{"Authorization"}, []string{"password"}, "***")

	t.Run("writes into nil dst allocates new map", func(t *testing.T) {
		h := http.Header{}
		h.Set("Authorization", "secret")
		h.Set("X-Request-Id", "abc") // http.Header canonicalizes
		out := masker.MaskHeadersInto(h, nil)
		if out["Authorization"] != "***" {
			t.Fatalf("expected masked auth, got %v", out["Authorization"])
		}
		if out["X-Request-Id"] != "abc" {
			t.Fatalf("expected request id, got %v", out["X-Request-Id"])
		}
	})

	t.Run("reuses existing dst map", func(t *testing.T) {
		dst := make(map[string]any, 32)
		dst["stale_key"] = "stale_value"

		h := http.Header{}
		h.Set("Content-Type", "application/json")
		out := masker.MaskHeadersInto(h, dst)

		if out == nil {
			t.Fatal("expected non-nil map")
		}
		if out["Content-Type"] != "application/json" {
			t.Fatalf("expected content type, got %v", out["Content-Type"])
		}
		if _, exists := out["stale_key"]; exists {
			t.Fatal("stale key should have been cleared")
		}
	})

	t.Run("nil headers clears dst", func(t *testing.T) {
		dst := make(map[string]any)
		dst["old"] = "value"
		out := masker.MaskHeadersInto(nil, dst)
		if len(out) != 0 {
			t.Fatalf("expected empty map, got %v", out)
		}
		// Verify same map returned (compare pointer via reflection-free approach)
		dst["probe"] = true
		if !out["probe"].(bool) {
			t.Fatal("expected same map returned")
		}
	})

	t.Run("nil headers nil dst returns empty map", func(t *testing.T) {
		out := masker.MaskHeadersInto(nil, nil)
		if len(out) != 0 {
			t.Fatalf("expected empty map, got %v", out)
		}
	})
}

func BenchmarkMaskHeadersWithoutPool(b *testing.B) {
	masker := NewMasker([]string{"Authorization", "X-API-Key"}, []string{"password"}, "***")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer token123")
	headers.Set("X-API-Key", "key-abc")
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Request-ID", "req-123")
	headers.Set("User-Agent", "test-agent")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := masker.MaskHeaders(headers)
		_ = result
	}
}

func BenchmarkMaskHeadersWithPool(b *testing.B) {
	masker := NewMasker([]string{"Authorization", "X-API-Key"}, []string{"password"}, "***")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer token123")
	headers.Set("X-API-Key", "key-abc")
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Request-ID", "req-123")
	headers.Set("User-Agent", "test-agent")

	// Prime the pool with a few iterations
	for i := 0; i < 10; i++ {
		m := getHeaderMap()
		masker.MaskHeadersInto(headers, m)
		putHeaderMap(m)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := getHeaderMap()
		masker.MaskHeadersInto(headers, m)
		putHeaderMap(m)
	}
}

func BenchmarkMaskHeadersIntoFreshMap(b *testing.B) {
	masker := NewMasker([]string{"Authorization", "X-API-Key"}, []string{"password"}, "***")
	headers := http.Header{}
	headers.Set("Authorization", "Bearer token123")
	headers.Set("X-API-Key", "key-abc")
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Request-ID", "req-123")
	headers.Set("User-Agent", "test-agent")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := make(map[string]any, 16)
		result := masker.MaskHeadersInto(headers, dst)
		_ = result
	}
}
