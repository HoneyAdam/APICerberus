package plugin

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestBrotliAppliedWhenAcceptedAndAboveMinSize(t *testing.T) {
	t.Parallel()

	out := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/resource", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")

	plugin := NewBrotliCompression(BrotliConfig{MinSize: 5})
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: out,
	}
	plugin.Apply(ctx)
	ctx.ResponseWriter.Header().Set("Content-Type", "application/json")
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = ctx.ResponseWriter.Write([]byte(`{"hello":"world"}`))
	plugin.AfterProxy(ctx, nil)

	capture := ctx.ResponseWriter.(*CaptureResponseWriter)
	if err := capture.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	if out.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("expected br content encoding, got %q", out.Header().Get("Content-Encoding"))
	}
	if out.Header().Get("Vary") == "" {
		t.Fatalf("expected Vary header")
	}

	// Decompress to verify
	r := brotli.NewReader(bytes.NewReader(out.Body.Bytes()))
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("decompress brotli: %v", err)
	}
	if string(body) != `{"hello":"world"}` {
		t.Fatalf("unexpected decompressed body %q", string(body))
	}
}

func TestBrotliSkipsWhenBelowThreshold(t *testing.T) {
	t.Parallel()

	out := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/resource", nil)
	req.Header.Set("Accept-Encoding", "br")

	plugin := NewBrotliCompression(BrotliConfig{MinSize: 100})
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: out,
	}
	plugin.Apply(ctx)
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = ctx.ResponseWriter.Write([]byte("tiny"))
	plugin.AfterProxy(ctx, nil)

	capture := ctx.ResponseWriter.(*CaptureResponseWriter)
	if err := capture.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	if out.Header().Get("Content-Encoding") != "" {
		t.Fatalf("did not expect content encoding for tiny body")
	}
	if out.Body.String() != "tiny" {
		t.Fatalf("expected uncompressed body, got %q", out.Body.String())
	}
}

func TestBrotliSkipsWhenClientDoesNotAcceptBr(t *testing.T) {
	t.Parallel()

	out := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/resource", nil)
	// No Accept-Encoding header

	plugin := NewBrotliCompression(BrotliConfig{MinSize: 1})
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: out,
	}
	plugin.Apply(ctx)
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = ctx.ResponseWriter.Write([]byte("plain"))
	plugin.AfterProxy(ctx, nil)

	capture := ctx.ResponseWriter.(*CaptureResponseWriter)
	if err := capture.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	if out.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no compression without Accept-Encoding br")
	}
}

func TestBrotliSkipsWhenAlreadyEncoded(t *testing.T) {
	t.Parallel()

	out := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/resource", nil)
	req.Header.Set("Accept-Encoding", "br")

	plugin := NewBrotliCompression(BrotliConfig{MinSize: 1})
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: out,
	}
	plugin.Apply(ctx)
	ctx.ResponseWriter.Header().Set("Content-Encoding", "gzip")
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = ctx.ResponseWriter.Write([]byte("already encoded"))
	plugin.AfterProxy(ctx, nil)

	capture := ctx.ResponseWriter.(*CaptureResponseWriter)
	if err := capture.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	// Should not have changed the encoding
	if out.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected original encoding preserved, got %q", out.Header().Get("Content-Encoding"))
	}
}

func TestBrotliQualityConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		quality int
		want    int
	}{
		{"default for zero", 0, 6},
		{"default for negative", -1, 6},
		{"clamped above 11", 15, 6},
		{"valid quality 1", 1, 1},
		{"valid quality 11", 11, 11},
		{"valid quality 4", 4, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := NewBrotliCompression(BrotliConfig{Quality: tt.quality})
			if b.quality != tt.want {
				t.Fatalf("expected quality %d, got %d", tt.want, b.quality)
			}
		})
	}
}

func TestBrotliNilReceivers(t *testing.T) {
	t.Parallel()

	var b *BrotliCompression
	b.Apply(nil)
	b.Apply(&PipelineContext{})
	b.AfterProxy(nil, nil)
	b.AfterProxy(&PipelineContext{}, nil)
}

func TestBrotliBuildFromRegistry(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, ok := reg.Lookup("brotli")
	if !ok {
		t.Fatal("expected brotli to be registered")
	}

	plugin, err := factory(config.PluginConfig{
		Name:   "brotli",
		Config: map[string]any{"min_size": 256, "quality": 8},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("build brotli plugin: %v", err)
	}
	if plugin.name != "brotli" {
		t.Fatalf("expected plugin name 'brotli', got %q", plugin.name)
	}
	if plugin.phase != PhasePostProxy {
		t.Fatalf("expected PhasePostProxy, got %s", plugin.phase)
	}
	if plugin.priority != 49 {
		t.Fatalf("expected priority 49 (before gzip), got %d", plugin.priority)
	}
}

func TestBrotliCompressionIsSmaller(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("hello world this is a test string for compression"), 100)

	out := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/resource", nil)
	req.Header.Set("Accept-Encoding", "br")

	plugin := NewBrotliCompression(BrotliConfig{})
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: out,
	}
	plugin.Apply(ctx)
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = ctx.ResponseWriter.Write(body)
	plugin.AfterProxy(ctx, nil)

	capture := ctx.ResponseWriter.(*CaptureResponseWriter)
	if err := capture.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	compressed := out.Body.Bytes()
	if len(compressed) >= len(body) {
		t.Fatalf("expected compressed size (%d) to be smaller than original (%d)", len(compressed), len(body))
	}

	// Verify decompression
	r := brotli.NewReader(bytes.NewReader(compressed))
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(decompressed, body) {
		t.Fatalf("decompressed body does not match original")
	}
}
