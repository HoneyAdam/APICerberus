package plugin

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// MockConfig controls mock response behavior.
type MockConfig struct {
	StatusCode  int
	ContentType string
	Body        string
	Headers     map[string]string
	LatencyMs   int
}

// Mock returns a canned response without proxying to an upstream.
type Mock struct {
	statusCode  int
	contentType string
	body        string
	headers     map[string]string
	latencyMs   int
}

func NewMock(cfg MockConfig) *Mock {
	statusCode := cfg.StatusCode
	if statusCode < 100 || statusCode > 599 {
		statusCode = http.StatusOK
	}
	contentType := cfg.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	return &Mock{
		statusCode:  statusCode,
		contentType: contentType,
		body:        cfg.Body,
		headers:     cfg.Headers,
		latencyMs:   cfg.LatencyMs,
	}
}

func (m *Mock) Name() string  { return "mock" }
func (m *Mock) Phase() Phase  { return PhasePreProxy }
func (m *Mock) Priority() int { return 5 }

// Serve writes the mock response and returns true to stop the pipeline.
func (m *Mock) Serve(w http.ResponseWriter, _ *http.Request) bool {
	if m == nil || w == nil {
		return false
	}
	for k, v := range m.headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", m.contentType)
	body := m.resolveBody()
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(m.statusCode)
	_, _ = w.Write(body)
	return true
}

func (m *Mock) resolveBody() []byte {
	if m.body != "" {
		return []byte(m.body)
	}
	// Default body is a JSON object with status and message.
	obj := map[string]any{
		"status":  m.statusCode,
		"message": http.StatusText(m.statusCode),
	}
	b, _ := json.Marshal(obj)
	return b
}
