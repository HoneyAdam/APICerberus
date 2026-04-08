package plugin

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// CompressionConfig controls gzip response compression behavior.
type CompressionConfig struct {
	MinSize int
}

// Compression compresses captured response bodies for gzip-capable clients.
type Compression struct {
	minSize int
}

func NewCompression(cfg CompressionConfig) *Compression {
	minSize := cfg.MinSize
	if minSize < 0 {
		minSize = 0
	}
	return &Compression{minSize: minSize}
}

func (c *Compression) Name() string  { return "compression" }
func (c *Compression) Phase() Phase  { return PhasePostProxy }
func (c *Compression) Priority() int { return 50 }

func (c *Compression) Apply(in *PipelineContext) {
	if c == nil || in == nil || in.ResponseWriter == nil {
		return
	}
	if _, ok := in.ResponseWriter.(*CaptureResponseWriter); ok {
		return
	}
	in.ResponseWriter = NewCaptureResponseWriter(in.ResponseWriter)
}

func (c *Compression) AfterProxy(in *PipelineContext, _ error) {
	if c == nil || in == nil {
		return
	}
	capture, ok := in.ResponseWriter.(*CaptureResponseWriter)
	if !ok || !capture.HasCaptured() {
		return
	}

	if in.Request == nil || !acceptsGzip(in.Request) {
		return
	}
	if encoding := strings.TrimSpace(capture.Header().Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return
	}

	body := capture.BodyBytes()
	if len(body) < c.minSize {
		return
	}

	compressed, err := gzipBytes(body)
	if err != nil {
		return
	}

	capture.SetBody(compressed)
	capture.Header().Set("Content-Encoding", "gzip")
	ensureVaryAcceptEncoding(capture.Header())
	capture.Header().Set("Content-Length", strconv.Itoa(len(compressed)))
}

func gzipBytes(data []byte) ([]byte, error) {
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func acceptsGzip(req *http.Request) bool {
	if req == nil {
		return false
	}
	value := strings.ToLower(req.Header.Get("Accept-Encoding"))
	return strings.Contains(value, "gzip")
}

func ensureVaryAcceptEncoding(header http.Header) {
	if header == nil {
		return
	}
	current := header.Get("Vary")
	if current == "" {
		header.Set("Vary", "Accept-Encoding")
		return
	}
	for _, part := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "Accept-Encoding") {
			return
		}
	}
	header.Set("Vary", current+", Accept-Encoding")
}

func gunzipBytes(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, 10<<20))
}
