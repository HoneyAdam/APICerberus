package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunService(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/admin/api/v1/services" {
				json.NewEncoder(w).Encode([]map[string]any{
					{"id": "svc-1", "name": "Test Service", "protocol": "http", "upstream": "up-1"},
				})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	err := runService([]string{"list", "--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runService error: %v", err)
	}
}

func TestRunRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/admin/api/v1/routes" {
				json.NewEncoder(w).Encode([]map[string]any{
					{"id": "route-1", "name": "Test Route", "service": "svc-1", "paths": "/api", "methods": "GET", "priority": 100},
				})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	err := runRoute([]string{"list", "--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runRoute error: %v", err)
	}
}

func TestRunUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/admin/api/v1/upstreams" {
				json.NewEncoder(w).Encode([]map[string]any{
					{"id": "up-1", "name": "Test Upstream", "algorithm": "round_robin", "targets": []map[string]any{{"id": "t1"}}},
				})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	err := runUpstream([]string{"list", "--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runUpstream error: %v", err)
	}
}

func TestRunEntityCommand_MissingSubcommand(t *testing.T) {
	err := runEntityCommand("service", "/admin/api/v1/services", []string{})
	if err == nil {
		t.Error("runEntityCommand should return error for missing subcommand")
	}
	if !strings.Contains(err.Error(), "missing service subcommand") {
		t.Errorf("Error should mention missing subcommand, got: %v", err)
	}
}

func TestRunEntityCommand_UnknownSubcommand(t *testing.T) {
	err := runEntityCommand("service", "/admin/api/v1/services", []string{"unknown"})
	if err == nil {
		t.Error("runEntityCommand should return error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown service subcommand") {
		t.Errorf("Error should mention unknown subcommand, got: %v", err)
	}
}

func TestRunEntityList_Service(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/admin/api/v1/services" {
			t.Errorf("Expected services path, got %s", r.URL.Path)
		}

		response := []map[string]any{
			{"id": "svc-1", "name": "Service One", "protocol": "http", "upstream": "up-1"},
			{"id": "svc-2", "name": "Service Two", "protocol": "https", "upstream": "up-2"},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runEntityList("service", "/admin/api/v1/services", []string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runEntityList error: %v", err)
	}
}

func TestRunEntityList_Route(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []map[string]any{
			{"id": "route-1", "name": "Route One", "service": "svc-1", "paths": "/api/v1", "methods": "GET,POST", "priority": 100},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runEntityList("route", "/admin/api/v1/routes", []string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runEntityList error: %v", err)
	}
}

func TestRunEntityList_Upstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []map[string]any{
			{
				"id":        "up-1",
				"name":      "Upstream One",
				"algorithm": "round_robin",
				"targets":   []map[string]any{{"id": "t1"}, {"id": "t2"}},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runEntityList("upstream", "/admin/api/v1/upstreams", []string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runEntityList error: %v", err)
	}
}

func TestRunEntityList_EmptyResults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer upstream.Close()

	err := runEntityList("service", "/admin/api/v1/services", []string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runEntityList error: %v", err)
	}
}

func TestRunEntityList_UnknownEntity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []map[string]any{
			{"id": "unknown-1", "name": "Unknown"},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runEntityList("unknown", "/admin/api/v1/unknown", []string{"--admin-url", upstream.URL, "--admin-key", "test-key"})
	if err != nil {
		t.Errorf("runEntityList error: %v", err)
	}
}

func TestRunEntityAdd_WithBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["name"] != "New Service" {
			t.Errorf("Expected name='New Service', got %v", payload["name"])
		}

		response := map[string]any{
			"id":   "svc-new",
			"name": "New Service",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runEntityAdd("service", "/admin/api/v1/services", []string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--body", `{"name":"New Service","protocol":"http"}`,
	})
	if err != nil {
		t.Errorf("runEntityAdd error: %v", err)
	}
}

func TestRunEntityAdd_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	payloadFile := filepath.Join(tmpDir, "payload.json")
	os.WriteFile(payloadFile, []byte(`{"name":"File Service","protocol":"https"}`), 0644)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["name"] != "File Service" {
			t.Errorf("Expected name='File Service', got %v", payload["name"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "svc-file", "name": "File Service"})
	}))
	defer upstream.Close()

	err := runEntityAdd("service", "/admin/api/v1/services", []string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--file", payloadFile,
	})
	if err != nil {
		t.Errorf("runEntityAdd error: %v", err)
	}
}

func TestRunEntityAdd_MissingPayload(t *testing.T) {
	err := runEntityAdd("service", "/admin/api/v1/services", []string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runEntityAdd should return error for missing payload")
	}
}

func TestRunEntityGet(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/svc-1") {
			t.Errorf("Expected path to end with /svc-1, got %s", r.URL.Path)
		}

		response := map[string]any{
			"id":       "svc-1",
			"name":     "Service One",
			"protocol": "http",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	err := runEntityGet("service", "/admin/api/v1/services", []string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "svc-1",
	})
	if err != nil {
		t.Errorf("runEntityGet error: %v", err)
	}
}

func TestRunEntityGet_PositionalArg(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/svc-2") {
			t.Errorf("Expected path to end with /svc-2, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "svc-2"})
	}))
	defer upstream.Close()

	err := runEntityGet("service", "/admin/api/v1/services", []string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"svc-2",
	})
	if err != nil {
		t.Errorf("runEntityGet error: %v", err)
	}
}

func TestRunEntityGet_MissingID(t *testing.T) {
	err := runEntityGet("service", "/admin/api/v1/services", []string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runEntityGet should return error for missing ID")
	}
}

func TestRunEntityUpdate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/svc-1") {
			t.Errorf("Expected path to end with /svc-1, got %s", r.URL.Path)
		}

		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["name"] != "Updated Service" {
			t.Errorf("Expected name='Updated Service', got %v", payload["name"])
		}

		json.NewEncoder(w).Encode(map[string]any{"id": "svc-1", "name": "Updated Service"})
	}))
	defer upstream.Close()

	err := runEntityUpdate("service", "/admin/api/v1/services", []string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "svc-1",
		"--body", `{"name":"Updated Service"}`,
	})
	if err != nil {
		t.Errorf("runEntityUpdate error: %v", err)
	}
}

func TestRunEntityUpdate_MissingID(t *testing.T) {
	err := runEntityUpdate("service", "/admin/api/v1/services", []string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
		"--body", `{"name":"Updated Service"}`,
	})
	if err == nil {
		t.Error("runEntityUpdate should return error for missing ID")
	}
}

func TestRunEntityDelete(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/svc-1") {
			t.Errorf("Expected path to end with /svc-1, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	err := runEntityDelete("service", "/admin/api/v1/services", []string{
		"--admin-url", upstream.URL,
		"--admin-key", "test-key",
		"--id", "svc-1",
	})
	if err != nil {
		t.Errorf("runEntityDelete error: %v", err)
	}
}

func TestRunEntityDelete_MissingID(t *testing.T) {
	err := runEntityDelete("service", "/admin/api/v1/services", []string{
		"--admin-url", "http://localhost:9876",
		"--admin-key", "test-key",
	})
	if err == nil {
		t.Error("runEntityDelete should return error for missing ID")
	}
}

func TestLoadJSONPayload_FromBody(t *testing.T) {
	payload, err := loadJSONPayload("", `{"name":"Test","value":123}`)
	if err != nil {
		t.Errorf("loadJSONPayload error: %v", err)
	}
	if payload["name"] != "Test" {
		t.Errorf("Expected name='Test', got %v", payload["name"])
	}
}

func TestLoadJSONPayload_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	payloadFile := filepath.Join(tmpDir, "payload.json")
	os.WriteFile(payloadFile, []byte(`{"name":"File Test","value":456}`), 0644)

	payload, err := loadJSONPayload(payloadFile, "")
	if err != nil {
		t.Errorf("loadJSONPayload error: %v", err)
	}
	if payload["name"] != "File Test" {
		t.Errorf("Expected name='File Test', got %v", payload["name"])
	}
}

func TestLoadJSONPayload_MissingBoth(t *testing.T) {
	_, err := loadJSONPayload("", "")
	if err == nil {
		t.Error("loadJSONPayload should return error when both path and body are empty")
	}
}

func TestLoadJSONPayload_InvalidJSON(t *testing.T) {
	_, err := loadJSONPayload("", `invalid json`)
	if err == nil {
		t.Error("loadJSONPayload should return error for invalid JSON")
	}
}

func TestLoadJSONPayload_FileNotFound(t *testing.T) {
	_, err := loadJSONPayload("/nonexistent/path/payload.json", "")
	if err == nil {
		t.Error("loadJSONPayload should return error for non-existent file")
	}
}
