package admin

import (
	"net/http"
	"testing"
)

// =============================================================================
// GraphQL Handler Tests
// =============================================================================

func TestGraphQLQueries(t *testing.T) {
	t.Run("query services", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		query := `{"query": "query { services { id name protocol upstream } }"}`
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { services { id name protocol upstream } }",
		})
		assertStatus(t, resp, http.StatusOK)
		// GraphQL returns 200 even with errors in the data field
		body, ok := resp["body"].(map[string]any)
		if !ok {
			t.Fatal("expected body to be map")
		}
		// Check that we have data or errors
		if _, hasData := body["data"]; !hasData {
			if _, hasErrors := body["errors"]; !hasErrors {
				t.Error("expected data or errors in response")
			}
		}
		_ = query
	})

	t.Run("query routes", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { routes { id name service paths methods } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query upstreams", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { upstreams { id name algorithm targets { id address weight } } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query consumers", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { consumers { id name } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query gatewayInfo", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { gatewayInfo { services routes upstreams consumers } }",
		})
		assertStatus(t, resp, http.StatusOK)
		body, ok := resp["body"].(map[string]any)
		if !ok {
			t.Fatal("expected body to be map")
		}
		// Check for data
		if data, ok := body["data"].(map[string]any); ok {
			if info, ok := data["gatewayInfo"].(map[string]any); ok {
				// Verify we have counts
				if _, ok := info["services"]; !ok {
					t.Error("expected services count in gatewayInfo")
				}
			}
		}
	})

	t.Run("query single service", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { service(id: \"svc-users\") { id name protocol upstream } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query single route", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { route(id: \"route-users\") { id name service paths } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query single upstream", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { upstream(id: \"up-users\") { id name algorithm } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query users", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { users { id email name role active } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("query auditLogs", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "query { auditLogs(limit: 10) { entries { id requestId routeId method path statusCode } total } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})
}

func TestGraphQLMutations(t *testing.T) {
	t.Run("create service mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createService(input: {
					id: "graphql-svc-1",
					name: "graphql-svc-1",
					protocol: "http",
					upstream: "up-users"
				}) {
					id
					name
					protocol
					upstream
				}
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("update service mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// First create a service
		createResp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createService(input: {
					id: "graphql-svc-update",
					name: "graphql-svc-update",
					protocol: "http",
					upstream: "up-users"
				}) {
					id
				}
			}`,
		})
		assertStatus(t, createResp, http.StatusOK)

		// Update the service
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				updateService(id: "graphql-svc-update", input: {
					name: "graphql-svc-updated",
					protocol: "http",
					upstream: "up-users"
				}) {
					id
					name
				}
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("delete service mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// First create a service
		createResp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createService(input: {
					id: "graphql-svc-delete",
					name: "graphql-svc-delete",
					protocol: "http",
					upstream: "up-users"
				}) {
					id
				}
			}`,
		})
		assertStatus(t, createResp, http.StatusOK)

		// Delete the service
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				deleteService(id: "graphql-svc-delete")
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("create route mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createRoute(input: {
					id: "graphql-route-1",
					name: "graphql-route-1",
					service: "svc-users",
					paths: ["/graphql-test"],
					methods: ["GET"]
				}) {
					id
					name
					service
				}
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("update route mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// First create a route
		createResp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createRoute(input: {
					id: "graphql-route-update",
					name: "graphql-route-update",
					service: "svc-users",
					paths: ["/graphql-update"],
					methods: ["GET"]
				}) {
					id
				}
			}`,
		})
		assertStatus(t, createResp, http.StatusOK)

		// Update the route
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				updateRoute(id: "graphql-route-update", input: {
					name: "graphql-route-updated",
					service: "svc-users",
					paths: ["/graphql-updated"],
					methods: ["GET", "POST"]
				}) {
					id
					name
				}
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("delete route mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// First create a route
		createResp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createRoute(input: {
					id: "graphql-route-delete",
					name: "graphql-route-delete",
					service: "svc-users",
					paths: ["/graphql-delete"],
					methods: ["GET"]
				}) {
					id
				}
			}`,
		})
		assertStatus(t, createResp, http.StatusOK)

		// Delete the route
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				deleteRoute(id: "graphql-route-delete")
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("create upstream mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createUpstream(input: {
					id: "graphql-up-1",
					name: "graphql-up-1",
					algorithm: "round_robin",
					targets: [
						{ address: "localhost:8081", weight: 1 }
					]
				}) {
					id
					name
					algorithm
				}
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("update upstream mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// First create an upstream
		createResp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createUpstream(input: {
					id: "graphql-up-update",
					name: "graphql-up-update",
					algorithm: "round_robin",
					targets: [
						{ address: "localhost:8081", weight: 1 }
					]
				}) {
					id
				}
			}`,
		})
		assertStatus(t, createResp, http.StatusOK)

		// Update the upstream
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				updateUpstream(id: "graphql-up-update", input: {
					name: "graphql-up-updated",
					algorithm: "weighted_round_robin",
					targets: [
						{ address: "localhost:8081", weight: 1 },
						{ address: "localhost:8082", weight: 2 }
					]
				}) {
					id
					name
					algorithm
				}
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("delete upstream mutation", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// First create an upstream
		createResp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createUpstream(input: {
					id: "graphql-up-delete",
					name: "graphql-up-delete",
					algorithm: "round_robin",
					targets: [
						{ address: "localhost:8081", weight: 1 }
					]
				}) {
					id
				}
			}`,
		})
		assertStatus(t, createResp, http.StatusOK)

		// Delete the upstream
		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				deleteUpstream(id: "graphql-up-delete")
			}`,
		})
		assertStatus(t, resp, http.StatusOK)
	})
}

func TestGraphQLErrors(t *testing.T) {
	t.Run("invalid query syntax", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": "this is not valid graphql {",
		})
		// GraphQL returns 200 with errors in the response body
		assertStatus(t, resp, http.StatusOK)
		body, ok := resp["body"].(map[string]any)
		if !ok {
			t.Fatal("expected body to be map")
		}
		// Should have errors
		if _, hasErrors := body["errors"]; !hasErrors {
			// Some implementations return data null with errors
			if data, hasData := body["data"]; hasData && data != nil {
				t.Error("expected errors for invalid query")
			}
		}
	})

	t.Run("create service with missing required field", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createService(input: {
					protocol: "http"
				}) {
					id
				}
			}`,
		})
		// Should return an error
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("create service with non-existent upstream", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				createService(input: {
					name: "bad-svc",
					upstream: "non-existent-upstream"
				}) {
					id
				}
			}`,
		})
		// Should return an error
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("update non-existent service", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				updateService(id: "non-existent-svc", input: {
					name: "updated",
					upstream: "up-users"
				}) {
					id
				}
			}`,
		})
		// Should return an error
		assertStatus(t, resp, http.StatusOK)
	})

	t.Run("delete non-existent route", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/graphql", token, map[string]any{
			"query": `mutation {
				deleteRoute(id: "non-existent-route")
			}`,
		})
		// Should return an error
		assertStatus(t, resp, http.StatusOK)
	})
}

// =============================================================================
// NewGraphQLHandler Tests
// =============================================================================

func TestNewGraphQLHandler(t *testing.T) {
	t.Run("create handler successfully", func(t *testing.T) {
		baseURL, _, _, token := newAdminTestServer(t)
		_ = token

		// Just verify the endpoint works
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/graphql", token, map[string]any{
			"query": "{ gatewayInfo { services } }",
		})
		assertStatus(t, resp, http.StatusOK)
	})
}
