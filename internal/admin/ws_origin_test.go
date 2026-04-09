package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestIsValidWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name           string
		adminAddr      string
		allowedOrigins []string
		originHeader   string
		want           bool
	}{
		// --- No allowed origins configured (strict same-origin) ---
		{
			name:         "empty origin rejected",
			adminAddr:    ":9876",
			originHeader: "",
			want:         false,
		},
		{
			name:         "null origin rejected",
			adminAddr:    ":9876",
			originHeader: "null",
			want:         false,
		},
		{
			name:         "localhost same port accepted",
			adminAddr:    "localhost:9876",
			originHeader: "http://localhost:9876",
			want:         true,
		},
		{
			name:         "localhost wrong port rejected",
			adminAddr:    "localhost:9876",
			originHeader: "http://localhost:3000",
			want:         false,
		},
		{
			name:         "0.0.0.0 maps to localhost",
			adminAddr:    "0.0.0.0:9876",
			originHeader: "http://localhost:9876",
			want:         true,
		},
		{
			name:         "external host rejected when no allowed origins",
			adminAddr:    "localhost:9876",
			originHeader: "http://evil.com:9876",
			want:         false,
		},
		{
			name:         "non-http scheme rejected",
			adminAddr:    "localhost:9876",
			originHeader: "file://localhost",
			want:         false,
		},
		// --- Explicit allowed origins ---
		{
			name:           "exact host match",
			adminAddr:      ":9876",
			allowedOrigins: []string{"https://app.example.com"},
			originHeader:   "https://app.example.com",
			want:           true,
		},
		{
			name:           "exact host mismatch",
			adminAddr:      ":9876",
			allowedOrigins: []string{"https://app.example.com"},
			originHeader:   "https://evil.example.com",
			want:           false,
		},
		{
			name:           "wildcard subdomain match",
			adminAddr:      ":9876",
			allowedOrigins: []string{"*.example.com"},
			originHeader:   "https://dashboard.example.com",
			want:           true,
		},
		{
			name:           "wildcard multi-level subdomain",
			adminAddr:      ":9876",
			allowedOrigins: []string{"*.example.com"},
			originHeader:   "https://a.b.example.com",
			want:           true,
		},
		{
			name:           "wildcard no match for base domain",
			adminAddr:      ":9876",
			allowedOrigins: []string{"*.example.com"},
			originHeader:   "https://example.com",
			want:           false,
		},
		{
			name:           "host with port match",
			adminAddr:      ":9876",
			allowedOrigins: []string{"app.example.com:3000"},
			originHeader:   "http://app.example.com:3000",
			want:           true,
		},
		{
			name:           "host with port mismatch",
			adminAddr:      ":9876",
			allowedOrigins: []string{"app.example.com:3000"},
			originHeader:   "http://app.example.com:8080",
			want:           false,
		},
		{
			name:           "full URL with scheme match",
			adminAddr:      ":9876",
			allowedOrigins: []string{"https://app.example.com"},
			originHeader:   "https://app.example.com",
			want:           true,
		},
		{
			name:           "full URL with scheme mismatch",
			adminAddr:      ":9876",
			allowedOrigins: []string{"https://app.example.com"},
			originHeader:   "http://app.example.com",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Admin.Addr = tt.adminAddr
			cfg.Admin.AllowedOrigins = tt.allowedOrigins

			s := &Server{cfg: cfg}

			req := httptest.NewRequest("GET", "/admin/api/v1/ws", nil)
			if tt.originHeader != "" {
				req.Header.Set("Origin", tt.originHeader)
			}

			got := s.isValidWebSocketOrigin(req)
			if got != tt.want {
				t.Errorf("isValidWebSocketOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchHost(t *testing.T) {
	tests := []struct {
		originHost string
		allowed    string
		want       bool
	}{
		{"app.example.com", "app.example.com", true},
		{"app.example.com", "evil.example.com", false},
		{"sub.example.com", "*.example.com", true},
		{"a.b.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"evil-example.com", "*.example.com", false},
		{"localhost", "localhost", true},
		{"127.0.0.1", "localhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.originHost+"_"+tt.allowed, func(t *testing.T) {
			got := matchHost(tt.originHost, tt.allowed)
			if got != tt.want {
				t.Errorf("matchHost(%q, %q) = %v, want %v", tt.originHost, tt.allowed, got, tt.want)
			}
		})
	}
}
