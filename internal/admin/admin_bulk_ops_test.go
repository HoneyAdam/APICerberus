package admin

import (
	"net/http"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/logging"
)

// TestBulkOperations covers bulk delete branches and error handling
func TestBulkOperations(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("bulk delete empty resources", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{},
		})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("bulk delete missing resources field", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{})
		if resp["status_code"].(float64) != http.StatusBadRequest {
			t.Errorf("expected 400, got %v", resp["status_code"])
		}
	})

	t.Run("bulk delete invalid resource type", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{
				{"type": "invalid_type", "id": "something"},
			},
		})
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("bulk delete missing id", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{
				{"type": "service", "id": ""},
			},
		})
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("bulk delete nonexistent route", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{
				{"type": "route", "id": "nonexistent-route"},
			},
		})
		// 400 returned when resource not found in all-or-nothing validation
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("bulk delete nonexistent service", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{
				{"type": "service", "id": "nonexistent-svc"},
			},
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("bulk delete nonexistent upstream", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{
				{"type": "upstream", "id": "nonexistent-up"},
			},
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("bulk delete mix valid and invalid", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, map[string]any{
			"resources": []map[string]any{
				{"type": "route", "id": "route-users"},
				{"type": "invalid_type", "id": "x"},
			},
		})
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})
}

// TestWebSocketPool covers GetBuffer/PutBuffer/MetricsSnapshot branches
func TestWebSocketPoolCoverage(t *testing.T) {
	t.Parallel()

	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	pm := NewWebSocketPoolManager()

	t.Run("GetBuffer returns fresh buffer", func(t *testing.T) {
		buf := pm.GetBuffer("test-topic")
		if buf == nil {
			t.Error("expected non-nil buffer")
		}
		if len(buf) != 0 {
			t.Errorf("expected empty buffer, got len=%d", len(buf))
		}
	})

	t.Run("PutBuffer returns buffer to pool", func(t *testing.T) {
		buf := pm.GetBuffer("test-topic-2")
		buf = append(buf, []byte("data")...)
		pm.PutBuffer("test-topic-2", buf)
		// Next get should return a buffer (possibly the recycled one)
		buf2 := pm.GetBuffer("test-topic-2")
		if buf2 == nil {
			t.Error("expected non-nil buffer")
		}
	})

	t.Run("MetricsSnapshot on stopped hub", func(t *testing.T) {
		hub := NewWebSocketHub(logger)
		hub.Stop()
		total, active, _, _, _, _ := hub.MetricsSnapshot()
		if total < 0 {
			t.Error("expected non-negative total connections")
		}
		if active < 0 {
			t.Error("expected non-negative active connections")
		}
	})
}

// TestGraphQLHandler covers the graphql handler basic path
func TestGraphQLHandler(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	t.Run("graphql query", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "{ services { id name } }",
		})
		// Should return 200 with data or errors
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("graphql introspection", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "{ __schema { types { name } } }",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("graphql create route mutation", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation { createRoute(input: {id: "route-graphql", name: "GraphQL Route", service: "svc-users", paths: ["/graphql/*"], methods: ["GET"]}) { id name } }`,
		})
		// Should succeed or fail with validation error
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("graphql create route without id", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation { createRoute(input: {name: "Auto ID Route", service: "svc-users", paths: ["/auto-id"], methods: ["POST"]}) { id name } }`,
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK && sc != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %v", sc)
		}
	})

	t.Run("graphql get routes", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "{ routes { id name service } }",
		})
		if sc := resp["status_code"].(float64); sc != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})
}
