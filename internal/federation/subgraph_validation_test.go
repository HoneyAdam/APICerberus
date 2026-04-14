package federation

import (
	"testing"
)

func TestValidateSubgraphURL_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
	}{
		{"public http", "http://api.example.com/graphql"},
		{"public https", "https://api.example.com/graphql"},
		{"public with port", "https://api.example.com:443/graphql"},
		{"public domain", "http://users-service.internal.corp/graphql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateSubgraphURL(tt.url); err != nil {
				t.Errorf("validateSubgraphURL(%q) unexpected error: %v", tt.url, err)
			}
		})
	}
}

func TestValidateSubgraphURL_Invalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
	}{
		{"bad scheme", "ftp://example.com/graphql"},
		{"no host", "http:///graphql"},
		{"loopback ip", "http://127.0.0.1/graphql"},
		{"loopback ipv6", "http://[::1]/graphql"},
		{"private 10.x", "http://10.0.0.1/graphql"},
		{"private 172.16", "http://172.16.0.1/graphql"},
		{"private 192.168", "http://192.168.1.1/graphql"},
		{"link-local metadata", "http://169.254.169.254/graphql"},
		{"multicast", "http://224.0.0.1/graphql"},
		{"unspecified", "http://0.0.0.0/graphql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateSubgraphURL(tt.url); err == nil {
				t.Errorf("validateSubgraphURL(%q) expected error, got nil", tt.url)
			}
		})
	}
}
