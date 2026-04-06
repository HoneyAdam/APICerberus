package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestCaptureResponseWriterCaptureAndFlush(t *testing.T) {
	t.Parallel()

	out := httptest.NewRecorder()
	capture := NewCaptureResponseWriter(out)

	capture.Header().Set("X-Test", "yes")
	capture.WriteHeader(http.StatusCreated)
	_, _ = capture.Write([]byte("captured"))

	if err := capture.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if out.Code != http.StatusCreated {
		t.Fatalf("expected status 201 got %d", out.Code)
	}
	if out.Body.String() != "captured" {
		t.Fatalf("unexpected body %q", out.Body.String())
	}
	if out.Header().Get("X-Test") != "yes" {
		t.Fatalf("expected captured header to flush")
	}
}

func TestResponseTransformHeadersAndBody(t *testing.T) {
	t.Parallel()

	out := httptest.NewRecorder()
	transform := NewResponseTransform(ResponseTransformConfig{
		AddHeaders: map[string]string{
			"X-Added": "ok",
		},
		RemoveHeaders: []string{"X-Upstream"},
		ReplaceBody:   `{"status":"rewritten"}`,
	})

	ctx := &PipelineContext{ResponseWriter: out}
	transform.Apply(ctx)

	ctx.ResponseWriter.Header().Set("X-Upstream", "remove-me")
	ctx.ResponseWriter.WriteHeader(http.StatusAccepted)
	_, _ = ctx.ResponseWriter.Write([]byte(`{"status":"original"}`))

	transform.AfterProxy(ctx, nil)
	capture, ok := ctx.ResponseWriter.(*TransformCaptureWriter)
	if !ok {
		t.Fatalf("expected capture response writer")
	}
	if err := capture.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	if out.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 got %d", out.Code)
	}
	if out.Body.String() != `{"status":"rewritten"}` {
		t.Fatalf("unexpected transformed body %q", out.Body.String())
	}
	if out.Header().Get("X-Upstream") != "" {
		t.Fatalf("expected upstream header removed")
	}
	if out.Header().Get("X-Added") != "ok" {
		t.Fatalf("expected added header")
	}
}

func TestBuildRoutePipelinesResponseTransform(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:      "route-response-transform",
				Name:    "route-response-transform",
				Service: "svc",
				Paths:   []string{"/x"},
				Methods: []string{http.MethodGet},
				Plugins: []config.PluginConfig{
					{
						Name: "response-transform",
						Config: map[string]any{
							"replace_body": "ok",
						},
					},
				},
			},
		},
	}

	pipelines, _, err := BuildRoutePipelines(cfg, nil)
	if err != nil {
		t.Fatalf("BuildRoutePipelines error: %v", err)
	}
	chain := pipelines["route-response-transform"]
	if len(chain) != 1 {
		t.Fatalf("expected 1 plugin in chain got %d", len(chain))
	}
	if chain[0].Name() != "response-transform" {
		t.Fatalf("expected response-transform plugin got %q", chain[0].Name())
	}
	if chain[0].Phase() != PhasePostProxy {
		t.Fatalf("expected post-proxy phase got %q", chain[0].Phase())
	}
}
