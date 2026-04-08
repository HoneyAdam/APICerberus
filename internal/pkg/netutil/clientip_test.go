package netutil

import (
	"net/http"
	"testing"
)

func TestRemoteAddrIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"10.0.0.1:12345", "10.0.0.1"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"127.0.0.1", "127.0.0.1"},
		{"", ""},
	}

	for _, tt := range tests {
		result := RemoteAddrIP(tt.input)
		if result != tt.expected {
			t.Errorf("RemoteAddrIP(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractClientIP_NoHeaders(t *testing.T) {
	req := &http.Request{RemoteAddr: "192.168.1.1:8080"}
	if got := ExtractClientIP(req); got != "192.168.1.1" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "192.168.1.1")
	}
}

func TestExtractClientIP_WithXForwardedFor(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "10.0.0.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.5, 10.0.0.2, 10.0.0.3"}},
	}
	if got := ExtractClientIP(req); got != "203.0.113.5" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "203.0.113.5")
	}
}

func TestExtractClientIP_WithXRealIP(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "10.0.0.1:8080",
		Header:     http.Header{"X-Real-Ip": []string{"198.51.100.10"}},
	}
	if got := ExtractClientIP(req); got != "198.51.100.10" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "198.51.100.10")
	}
}

func TestExtractClientIP_XForwardedForPreferredOverXRealIP(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "10.0.0.1:8080",
		Header: http.Header{
			"X-Forwarded-For": []string{"203.0.113.5"},
			"X-Real-Ip":       []string{"198.51.100.10"},
		},
	}
	if got := ExtractClientIP(req); got != "203.0.113.5" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "203.0.113.5")
	}
}

func TestExtractClientIP_EmptyXForwardedForFallsBack(t *testing.T) {
	req := &http.Request{
		RemoteAddr: "192.168.1.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{""}},
	}
	if got := ExtractClientIP(req); got != "192.168.1.1" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "192.168.1.1")
	}
}

func TestExtractClientIP_NilRequest(t *testing.T) {
	if got := ExtractClientIP(nil); got != "" {
		t.Errorf("ExtractClientIP(nil) = %q, want %q", got, "")
	}
}

func TestExtractClientIP_TrustedProxies_UntrustedSource(t *testing.T) {
	SetTrustedProxies([]string{"10.0.0.1"})
	defer SetTrustedProxies(nil)

	req := &http.Request{
		RemoteAddr: "192.168.1.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.5"}},
	}
	// Source is untrusted, should ignore X-Forwarded-For
	if got := ExtractClientIP(req); got != "192.168.1.1" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "192.168.1.1")
	}
}

func TestExtractClientIP_TrustedProxies_TrustedSource(t *testing.T) {
	SetTrustedProxies([]string{"10.0.0.1"})
	defer SetTrustedProxies(nil)

	req := &http.Request{
		RemoteAddr: "10.0.0.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.5"}},
	}
	// Source is trusted, should use X-Forwarded-For
	if got := ExtractClientIP(req); got != "203.0.113.5" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "203.0.113.5")
	}
}

func TestExtractClientIP_TrustedProxies_EmptyList(t *testing.T) {
	SetTrustedProxies([]string{})
	defer SetTrustedProxies(nil)

	req := &http.Request{
		RemoteAddr: "192.168.1.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.5"}},
	}
	// Empty list means trust all for backward compatibility
	if got := ExtractClientIP(req); got != "203.0.113.5" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "203.0.113.5")
	}
}

func TestSetTrustedProxies_WithWhitespace(t *testing.T) {
	SetTrustedProxies([]string{" 10.0.0.1 ", "", " 192.168.1.1 "})
	defer SetTrustedProxies(nil)

	// Trusted
	req1 := &http.Request{
		RemoteAddr: "10.0.0.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.5"}},
	}
	if got := ExtractClientIP(req1); got != "203.0.113.5" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "203.0.113.5")
	}

	// Trusted
	req2 := &http.Request{
		RemoteAddr: "192.168.1.1:8080",
		Header:     http.Header{"X-Forwarded-For": []string{"203.0.113.6"}},
	}
	if got := ExtractClientIP(req2); got != "203.0.113.6" {
		t.Errorf("ExtractClientIP() = %q, want %q", got, "203.0.113.6")
	}
}
