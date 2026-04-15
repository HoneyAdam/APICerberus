package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestMockReturnsCustomBody(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)

	m := NewMock(MockConfig{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        `{"users":[]}`,
	})

	handled := m.Serve(rec, req)
	if !handled {
		t.Fatal("expected handled=true")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body != `{"users":[]}` {
		t.Fatalf("unexpected body: %q", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestMockDefaultBodyWhenEmpty(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)

	m := NewMock(MockConfig{
		StatusCode: http.StatusNotFound,
	})
	m.Serve(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if status, _ := result["status"].(float64); int(status) != 404 {
		t.Fatalf("expected status 404 in body, got %v", result["status"])
	}
	if msg, _ := result["message"].(string); msg != "Not Found" {
		t.Fatalf("expected 'Not Found' message, got %q", msg)
	}
}

func TestMockCustomHeaders(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	m := NewMock(MockConfig{
		StatusCode: http.StatusOK,
		Body:       `ok`,
		Headers: map[string]string{
			"X-Custom":  "value",
			"X-Trace-ID": "abc123",
		},
	})
	m.Serve(rec, req)

	if v := rec.Header().Get("X-Custom"); v != "value" {
		t.Fatalf("expected X-Custom=value, got %q", v)
	}
	if v := rec.Header().Get("X-Trace-ID"); v != "abc123" {
		t.Fatalf("expected X-Trace-ID=abc123, got %q", v)
	}
}

func TestMockStatusCodeClamping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input  int
		expect int
	}{
		{0, 200},
		{-1, 200},
		{99, 200},
		{600, 200},
		{200, 200},
		{201, 201},
		{400, 400},
		{500, 500},
		{503, 503},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			m := NewMock(MockConfig{StatusCode: tt.input})
			if m.statusCode != tt.expect {
				t.Fatalf("input=%d expected=%d got=%d", tt.input, tt.expect, m.statusCode)
			}
		})
	}
}

func TestMockContentTypeDefault(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	m := NewMock(MockConfig{})
	m.Serve(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected default application/json, got %q", ct)
	}
}

func TestMockContentTypeOverride(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	m := NewMock(MockConfig{ContentType: "text/plain", Body: "hello"})
	m.Serve(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("expected text/plain, got %q", ct)
	}
	if body := rec.Body.String(); body != "hello" {
		t.Fatalf("expected 'hello', got %q", body)
	}
}

func TestMockContentLengthSet(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	m := NewMock(MockConfig{Body: "short"})
	m.Serve(rec, req)

	if cl := rec.Header().Get("Content-Length"); cl != "5" {
		t.Fatalf("expected Content-Length=5, got %q", cl)
	}
}

func TestMockNilReceivers(t *testing.T) {
	t.Parallel()

	var m *Mock
	if m.Serve(nil, nil) {
		t.Fatal("nil mock should not handle")
	}
	if m.Serve(httptest.NewRecorder(), nil) {
		t.Fatal("nil mock should not handle")
	}
	if m.Serve(nil, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("nil mock should not handle")
	}
}

func TestMockNilResponseWriter(t *testing.T) {
	t.Parallel()

	m := NewMock(MockConfig{Body: "test"})
	if m.Serve(nil, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("nil ResponseWriter should not handle")
	}
}

func TestMockInPipelineContext(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)

	m := NewMock(MockConfig{
		StatusCode:  http.StatusCreated,
		ContentType: "application/json",
		Body:        `{"order_id":"ord_123"}`,
		Headers:     map[string]string{"X-Request-ID": "req_abc"},
	})

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: rec,
	}

	handled := m.Serve(ctx.ResponseWriter, ctx.Request)
	if !handled {
		t.Fatal("expected handled=true")
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if v := rec.Header().Get("X-Request-ID"); v != "req_abc" {
		t.Fatalf("expected X-Request-ID=req_abc, got %q", v)
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if id, _ := result["order_id"].(string); id != "ord_123" {
		t.Fatalf("expected order_id=ord_123, got %q", id)
	}
}

func TestMockBuildFromRegistry(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, ok := reg.Lookup("mock")
	if !ok {
		t.Fatal("expected mock to be registered")
	}

	plugin, err := factory(config.PluginConfig{
		Name: "mock",
		Config: map[string]any{
			"status_code":  418,
			"content_type": "text/plain",
			"body":         "I'm a teapot",
			"latency_ms":   100,
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("build mock plugin: %v", err)
	}

	if plugin.name != "mock" {
		t.Fatalf("expected name 'mock', got %q", plugin.name)
	}
	if plugin.phase != PhasePreProxy {
		t.Fatalf("expected PhasePreProxy, got %s", plugin.phase)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	handled, err := plugin.run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true from run")
	}

	if rec.Code != 418 {
		t.Fatalf("expected 418, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "I'm a teapot" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestMockBuildFromRegistryDefaultBody(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, _ := reg.Lookup("mock")

	plugin, err := factory(config.PluginConfig{
		Name:   "mock",
		Config: map[string]any{},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	handled, _ := plugin.run(ctx)
	if !handled {
		t.Fatal("expected handled")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 default, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if status, _ := result["status"].(float64); int(status) != 200 {
		t.Fatalf("expected status 200, got %v", result["status"])
	}
}

func TestMockBuildWithHeaders(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, _ := reg.Lookup("mock")

	plugin, err := factory(config.PluginConfig{
		Name: "mock",
		Config: map[string]any{
			"status_code": 200,
			"body":        `{"mock":true}`,
			"headers": map[string]any{
				"X-Mock":    "true",
				"X-Request": "test",
			},
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	plugin.run(ctx)

	if v := rec.Header().Get("X-Mock"); v != "true" {
		t.Fatalf("expected X-Mock=true, got %q", v)
	}
	if v := rec.Header().Get("X-Request"); v != "test" {
		t.Fatalf("expected X-Request=test, got %q", v)
	}
}

func TestMockNamePhasePriority(t *testing.T) {
	t.Parallel()

	m := NewMock(MockConfig{})
	if m.Name() != "mock" {
		t.Fatalf("expected name 'mock', got %q", m.Name())
	}
	if m.Phase() != PhasePreProxy {
		t.Fatalf("expected PhasePreProxy, got %s", m.Phase())
	}
	if m.Priority() != 5 {
		t.Fatalf("expected priority 5, got %d", m.Priority())
	}
}
