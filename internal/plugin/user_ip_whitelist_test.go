package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestUserIPWhitelistAllowsExactAndCIDR(t *testing.T) {
	t.Parallel()

	plugin := NewUserIPWhitelist()

	ctxExact := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://gateway.local/x", nil),
		Consumer: &config.Consumer{
			ID: "u1",
			Metadata: map[string]any{
				"ip_whitelist": []string{"203.0.113.7"},
			},
		},
	}
	ctxExact.Request.RemoteAddr = "203.0.113.7:1200"
	if err := plugin.Evaluate(ctxExact); err != nil {
		t.Fatalf("expected exact ip to pass, got %v", err)
	}

	ctxCIDR := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://gateway.local/x", nil),
		Consumer: &config.Consumer{
			ID: "u1",
			Metadata: map[string]any{
				"ip_whitelist": []any{"198.51.100.0/24"},
			},
		},
	}
	ctxCIDR.Request.RemoteAddr = "198.51.100.23:1200"
	if err := plugin.Evaluate(ctxCIDR); err != nil {
		t.Fatalf("expected cidr ip to pass, got %v", err)
	}
}

func TestUserIPWhitelistDeniesMismatch(t *testing.T) {
	t.Parallel()

	plugin := NewUserIPWhitelist()
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://gateway.local/x", nil),
		Consumer: &config.Consumer{
			ID: "u1",
			Metadata: map[string]any{
				"ip_whitelist": []string{"203.0.113.0/24"},
			},
		},
	}
	ctx.Request.RemoteAddr = "198.51.100.1:9999"

	err := plugin.Evaluate(ctx)
	if err == nil {
		t.Fatalf("expected deny on ip mismatch")
	}
	whitelistErr, ok := err.(*UserIPWhitelistError)
	if !ok {
		t.Fatalf("expected UserIPWhitelistError got %T", err)
	}
	if whitelistErr.Code != "ip_not_allowed" || whitelistErr.Status != http.StatusForbidden {
		t.Fatalf("unexpected error payload: %#v", whitelistErr)
	}
}

func TestUserIPWhitelistSkipsWhenNoRules(t *testing.T) {
	t.Parallel()

	plugin := NewUserIPWhitelist()
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://gateway.local/x", nil),
		Consumer: &config.Consumer{
			ID:       "u1",
			Metadata: map[string]any{},
		},
	}
	ctx.Request.RemoteAddr = "198.51.100.1:9999"
	if err := plugin.Evaluate(ctx); err != nil {
		t.Fatalf("expected no whitelist rules to skip check, got %v", err)
	}
}
