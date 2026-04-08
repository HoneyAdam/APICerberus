package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewAdminClient(t *testing.T) {
	t.Run("with explicit URL and key", func(t *testing.T) {
		client, err := newAdminClient("", "http://localhost:9876", "test-key")
		if err != nil {
			t.Fatalf("newAdminClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("newAdminClient() returned nil")
		}
		if client.baseURL != "http://localhost:9876" {
			t.Errorf("baseURL = %v, want http://localhost:9876", client.baseURL)
		}
		if client.adminKey != "test-key" {
			t.Errorf("adminKey = %v, want test-key", client.adminKey)
		}
	})

	t.Run("with port only", func(t *testing.T) {
		client, err := newAdminClient("", ":9999", "key")
		if err != nil {
			t.Fatalf("newAdminClient() error = %v", err)
		}
		if client.baseURL != "http://127.0.0.1:9999" {
			t.Errorf("baseURL = %v, want http://127.0.0.1:9999", client.baseURL)
		}
	})

	t.Run("with config path (invalid)", func(t *testing.T) {
		// This will fail since the config file doesn't exist
		_, err := newAdminClient("/nonexistent/config.yaml", "", "")
		if err == nil {
			t.Error("newAdminClient() should return error for invalid config")
		}
	})
}

func TestResolveAdminConnection(t *testing.T) {
	t.Run("explicit URL and key", func(t *testing.T) {
		base, key, err := resolveAdminConnection("", "http://localhost:9876", "my-key")
		if err != nil {
			t.Fatalf("resolveAdminConnection() error = %v", err)
		}
		if base != "http://localhost:9876" {
			t.Errorf("base = %v, want http://localhost:9876", base)
		}
		if key != "my-key" {
			t.Errorf("key = %v, want my-key", key)
		}
	})

	t.Run("missing URL and key", func(t *testing.T) {
		_, _, err := resolveAdminConnection("/nonexistent/config.yaml", "", "")
		if err == nil {
			t.Error("resolveAdminConnection() should return error when URL and key missing")
		}
	})
}

func TestNormalizeAdminBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "http://127.0.0.1:9876"},
		{"http://localhost:9876", "http://localhost:9876"},
		{"https://admin.example.com", "https://admin.example.com"},
		{":9876", "http://127.0.0.1:9876"},
		{"localhost:9876", "http://localhost:9876"},
		{"http://localhost:9876/", "http://localhost:9876"},
		{"  http://localhost:9876  ", "http://localhost:9876"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAdminBaseURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAdminBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAdminClientCall(t *testing.T) {
	t.Run("successful GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Admin-Key") != "test-key" {
				t.Error("Missing or incorrect X-Admin-Key header")
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}))
		defer server.Close()

		client := &adminClient{
			baseURL:    server.URL,
			adminKey:   "test-key",
			httpClient: &http.Client{},
		}

		resp, err := client.call("GET", "/test", nil, nil)
		if err != nil {
			t.Errorf("call() error = %v", err)
		}
		if resp == nil {
			t.Error("call() returned nil response")
		}
	})

	t.Run("POST with payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST, got %s", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("Missing Content-Type header")
			}

			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"received": payload})
		}))
		defer server.Close()

		client := &adminClient{
			baseURL:    server.URL,
			adminKey:   "test-key",
			httpClient: &http.Client{},
		}

		payload := map[string]string{"name": "test"}
		resp, err := client.call("POST", "/test", nil, payload)
		if err != nil {
			t.Errorf("call() error = %v", err)
		}
		if resp == nil {
			t.Error("call() returned nil response")
		}
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"code":    "INVALID_INPUT",
					"message": "Invalid input data",
				},
			})
		}))
		defer server.Close()

		client := &adminClient{
			baseURL:    server.URL,
			adminKey:   "test-key",
			httpClient: &http.Client{},
		}

		_, err := client.call("GET", "/test", nil, nil)
		if err == nil {
			t.Error("call() should return error for 400 response")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		var client *adminClient
		_, err := client.call("GET", "/test", nil, nil)
		if err == nil {
			t.Error("call() should return error for nil client")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		client := &adminClient{}
		_, err := client.call("GET", "   ", nil, nil)
		if err == nil {
			t.Error("call() should return error for empty path")
		}
	})

	t.Run("with query params", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key") != "value" {
				t.Error("Missing query parameter")
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}))
		defer server.Close()

		client := &adminClient{
			baseURL:    server.URL,
			adminKey:   "test-key",
			httpClient: &http.Client{},
		}

		query := url.Values{}
		query.Set("key", "value")

		resp, err := client.call("GET", "/test", query, nil)
		if err != nil {
			t.Errorf("call() error = %v", err)
		}
		if resp == nil {
			t.Error("call() returned nil response")
		}
	})

	t.Run("no content response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		client := &adminClient{
			baseURL:    server.URL,
			adminKey:   "test-key",
			httpClient: &http.Client{},
		}

		resp, err := client.call("DELETE", "/test", nil, nil)
		if err != nil {
			t.Errorf("call() error = %v", err)
		}
		if resp == nil {
			t.Error("call() should return response for 204")
		}
	})
}

func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    string
	}{
		{
			name: "error object with code and message",
			payload: map[string]any{
				"error": map[string]any{
					"code":    "NOT_FOUND",
					"message": "Resource not found",
				},
			},
			want: "NOT_FOUND: Resource not found",
		},
		{
			name: "error object with message only",
			payload: map[string]any{
				"error": map[string]any{
					"message": "Something went wrong",
				},
			},
			want: "Something went wrong",
		},
		{
			name:    "plain string",
			payload: "Simple error message",
			want:    "Simple error message",
		},
		{
			name:    "empty payload",
			payload: "",
			want:    "unknown error",
		},
		{
			name:    "nil payload",
			payload: nil,
			want:    "unknown error",
		},
		{
			name:    "number",
			payload: 404,
			want:    "404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorMessage(tt.payload)
			if got != tt.want {
				t.Errorf("extractErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
