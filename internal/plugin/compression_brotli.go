package plugin

import (
	"bytes"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

// BrotliConfig controls Brotli response compression behavior.
type BrotliConfig struct {
	MinSize int
	Quality int // 0-11, default 6
}

// BrotliCompression compresses captured response bodies for Brotli-capable clients.
type BrotliCompression struct {
	minSize int
	quality int
}

func NewBrotliCompression(cfg BrotliConfig) *BrotliCompression {
	minSize := cfg.MinSize
	if minSize < 0 {
		minSize = 0
	}
	quality := cfg.Quality
	if quality <= 0 || quality > 11 {
		quality = 6
	}
	return &BrotliCompression{minSize: minSize, quality: quality}
}

func (b *BrotliCompression) Name() string  { return "brotli" }
func (b *BrotliCompression) Phase() Phase  { return PhasePostProxy }
func (b *BrotliCompression) Priority() int { return 49 } // Run before gzip (50) so brotli is preferred

func (b *BrotliCompression) Apply(in *PipelineContext) {
	if b == nil || in == nil || in.ResponseWriter == nil {
		return
	}
	if _, ok := in.ResponseWriter.(*CaptureResponseWriter); ok {
		return
	}
	in.ResponseWriter = NewCaptureResponseWriter(in.ResponseWriter)
}

func (b *BrotliCompression) AfterProxy(in *PipelineContext, _ error) {
	if b == nil || in == nil {
		return
	}
	capture, ok := in.ResponseWriter.(*CaptureResponseWriter)
	if !ok || !capture.HasCaptured() {
		return
	}

	if in.Request == nil || !acceptsEncoding(in.Request, "br") {
		return
	}
	if encoding := strings.TrimSpace(capture.Header().Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return
	}

	body := capture.BodyBytes()
	if len(body) < b.minSize {
		return
	}

	compressed, err := brotliBytes(body, b.quality)
	if err != nil {
		log.Printf("[WARN] brotli: compression failed: %v", err)
		return
	}

	capture.SetBody(compressed)
	capture.Header().Set("Content-Encoding", "br")
	ensureVaryAcceptEncoding(capture.Header())
	capture.Header().Set("Content-Length", strconv.Itoa(len(compressed)))
}

func brotliBytes(data []byte, quality int) ([]byte, error) {
	var out bytes.Buffer
	w := brotli.NewWriterOptions(&out, brotli.WriterOptions{Quality: quality})
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func acceptsEncoding(req *http.Request, encoding string) bool {
	if req == nil {
		return false
	}
	value := strings.ToLower(req.Header.Get("Accept-Encoding"))
	return strings.Contains(value, encoding)
}
