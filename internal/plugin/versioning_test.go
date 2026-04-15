package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestExtractVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input          string
		wantVersion    string
		wantRemaining  string
	}{
		{"/v1/users", "1", "/users"},
		{"/v2/api/orders", "2", "/api/orders"},
		{"/v10/health", "10", "/health"},
		{"/api/v1/users", "", "/api/v1/users"},
		{"/users", "", "/users"},
		{"/v/users", "", "/v/users"},
		{"/v1", "1", "/"},
		{"/v0/test", "0", "/test"},
		{"/vx/users", "", "/vx/users"},
		{"", "", ""},
		{"/", "", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			v, r := extractVersion(tt.input)
			if v != tt.wantVersion {
				t.Fatalf("version: want %q got %q", tt.wantVersion, v)
			}
			if r != tt.wantRemaining {
				t.Fatalf("remaining: want %q got %q", tt.wantRemaining, r)
			}
		})
	}
}

func TestVersioningAddsHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{})
	handled, err := v.Apply(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("should not be handled")
	}
	if h := req.Header.Get("X-API-Version"); h != "1" {
		t.Fatalf("expected X-API-Version=1, got %q", h)
	}
}

func TestVersioningCustomHeaderName(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v2/orders", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{HeaderName: "X-Api-Version"})
	v.Apply(ctx)

	if h := req.Header.Get("X-Api-Version"); h != "2" {
		t.Fatalf("expected X-Api-Version=2, got %q", h)
	}
}

func TestVersioningStripPrefix(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v1/users/123", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{StripPrefix: true})
	v.Apply(ctx)

	if req.URL.Path != "/users/123" {
		t.Fatalf("expected /users/123, got %q", req.URL.Path)
	}
}

func TestVersioningNoStripByDefault(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{})
	v.Apply(ctx)

	if req.URL.Path != "/v1/users" {
		t.Fatalf("path should not change by default, got %q", req.URL.Path)
	}
}

func TestVersioningDefaultVersion(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{DefaultVersion: "2"})
	v.Apply(ctx)

	if h := req.Header.Get("X-API-Version"); h != "2" {
		t.Fatalf("expected default version 2, got %q", h)
	}
}

func TestVersioningRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v3/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{Versions: []string{"1", "2"}})
	handled, err := v.Apply(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("unsupported version should be handled (rejected)")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestVersioningAllowsSupportedVersion(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{Versions: []string{"1", "2"}})
	handled, _ := v.Apply(ctx)
	if handled {
		t.Fatal("supported version should not be handled")
	}
	if h := req.Header.Get("X-API-Version"); h != "1" {
		t.Fatalf("expected version 1, got %q", h)
	}
}

func TestVersioningDeprecationHeaders(t *testing.T) {
	t.Parallel()

	sunset := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{
		Deprecation: map[string]DeprecationInfo{
			"1": {
				Sunset:  sunset,
				Message: "Use v2 instead",
				Link:    "https://docs.example.com/migration",
			},
		},
	})
	handled, _ := v.Apply(ctx)
	if handled {
		t.Fatal("deprecated (non-disabled) should still pass through")
	}

	if d := rec.Header().Get("Deprecation"); d != "true" {
		t.Fatalf("expected Deprecation=true, got %q", d)
	}
	if s := rec.Header().Get("Sunset"); s == "" {
		t.Fatal("expected Sunset header")
	}
	if n := rec.Header().Get("X-Deprecation-Notice"); n != "Use v2 instead" {
		t.Fatalf("expected notice, got %q", n)
	}
	if l := rec.Header().Get("Link"); !strings.Contains(l, "migration") {
		t.Fatalf("expected Link header with migration URL, got %q", l)
	}
}

func TestVersioningDisabledVersionReturnsGone(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v0/test", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{
		Deprecation: map[string]DeprecationInfo{
			"0": {
				Disabled: true,
				Message:  "v0 removed in Jan 2025",
			},
		},
	})
	handled, _ := v.Apply(ctx)
	if !handled {
		t.Fatal("disabled version should be handled")
	}
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone, got %d", rec.Code)
	}
}

func TestVersioningMetadataPopulated(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/v3/data", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{})
	v.Apply(ctx)

	if ctx.Metadata == nil {
		t.Fatal("expected metadata to be populated")
	}
	if ver, _ := ctx.Metadata["api_version"].(string); ver != "3" {
		t.Fatalf("expected api_version=3, got %v", ctx.Metadata["api_version"])
	}
}

func TestVersioningNilReceivers(t *testing.T) {
	t.Parallel()

	var v *Versioning
	handled, err := v.Apply(nil)
	if handled || err != nil {
		t.Fatal("nil should be no-op")
	}
	handled, err = v.Apply(&PipelineContext{})
	if handled || err != nil {
		t.Fatal("nil receiver should be no-op")
	}
}

func TestVersioningNoVersionNoDefault(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	v := NewVersioning(VersioningConfig{})
	handled, _ := v.Apply(ctx)
	if handled {
		t.Fatal("should not be handled when no version and no default")
	}
	if h := req.Header.Get("X-API-Version"); h != "" {
		t.Fatalf("expected no version header, got %q", h)
	}
}

func TestVersioningNamePhasePriority(t *testing.T) {
	t.Parallel()

	v := NewVersioning(VersioningConfig{})
	if v.Name() != "versioning" {
		t.Fatalf("expected 'versioning', got %q", v.Name())
	}
	if v.Phase() != PhasePreProxy {
		t.Fatalf("expected PhasePreProxy, got %s", v.Phase())
	}
	if v.Priority() != 8 {
		t.Fatalf("expected priority 8, got %d", v.Priority())
	}
}

func TestVersioningBuildFromRegistry(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, ok := reg.Lookup("versioning")
	if !ok {
		t.Fatal("expected versioning to be registered")
	}

	plugin, err := factory(config.PluginConfig{
		Name: "versioning",
		Config: map[string]any{
			"default_version": "1",
			"versions":        []any{"1", "2"},
			"strip_prefix":    true,
			"header_name":     "X-Ver",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if plugin.name != "versioning" {
		t.Fatalf("expected name 'versioning', got %q", plugin.name)
	}
	if plugin.phase != PhasePreProxy {
		t.Fatalf("expected PhasePreProxy, got %s", plugin.phase)
	}

	req := httptest.NewRequest(http.MethodGet, "/v2/orders", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	handled, err := plugin.run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if handled {
		t.Fatal("v2 is supported, should not be handled")
	}
	if h := req.Header.Get("X-Ver"); h != "2" {
		t.Fatalf("expected X-Ver=2, got %q", h)
	}
	if req.URL.Path != "/orders" {
		t.Fatalf("expected stripped path /orders, got %q", req.URL.Path)
	}
}

func TestVersioningBuildFromRegistryRejectsBadVersion(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, _ := reg.Lookup("versioning")

	plugin, _ := factory(config.PluginConfig{
		Name: "versioning",
		Config: map[string]any{
			"versions": []any{"1", "2"},
		},
	}, BuilderContext{})

	req := httptest.NewRequest(http.MethodGet, "/v5/test", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	handled, err := plugin.run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !handled {
		t.Fatal("v5 is unsupported, should be handled (rejected)")
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if body["error"] == nil {
		t.Fatal("expected error in response body")
	}
}

func TestVersioningDeprecationFromConfig(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, _ := reg.Lookup("versioning")

	plugin, _ := factory(config.PluginConfig{
		Name: "versioning",
		Config: map[string]any{
			"versions": []any{"1", "2"},
			"deprecation": map[string]any{
				"1": map[string]any{
					"sunset":  "2026-12-31T00:00:00Z",
					"message": "Migrate to v2",
					"link":    "https://docs.example.com/v2",
				},
			},
		},
	}, BuilderContext{})

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	handled, _ := plugin.run(ctx)
	if handled {
		t.Fatal("deprecated v1 should still pass through")
	}
	if d := rec.Header().Get("Deprecation"); d != "true" {
		t.Fatalf("expected Deprecation header, got %q", d)
	}
	if n := rec.Header().Get("X-Deprecation-Notice"); n != "Migrate to v2" {
		t.Fatalf("expected notice, got %q", n)
	}
}

func TestVersioningDisabledFromConfig(t *testing.T) {
	t.Parallel()

	reg := NewDefaultRegistry()
	factory, _ := reg.Lookup("versioning")

	plugin, _ := factory(config.PluginConfig{
		Name: "versioning",
		Config: map[string]any{
			"deprecation": map[string]any{
				"0": map[string]any{
					"disabled": true,
					"message":  "v0 removed",
				},
			},
		},
	}, BuilderContext{})

	req := httptest.NewRequest(http.MethodGet, "/v0/test", nil)
	rec := httptest.NewRecorder()
	ctx := &PipelineContext{Request: req, ResponseWriter: rec}

	handled, _ := plugin.run(ctx)
	if !handled {
		t.Fatal("disabled version should be handled")
	}
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d", rec.Code)
	}
}
