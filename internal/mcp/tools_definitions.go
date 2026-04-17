package mcp

func (s *Server) toolDefinitions() []toolDefinition {
	anyObj := map[string]any{"type": "object", "additionalProperties": true}
	idObj := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
		},
		"required":             []string{"id"},
		"additionalProperties": true,
	}
	userIDObj := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{"type": "string"},
		},
		"required":             []string{"user_id"},
		"additionalProperties": true,
	}
	userAndKeyObj := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{"type": "string"},
			"key_id":  map[string]any{"type": "string"},
		},
		"required":             []string{"user_id", "key_id"},
		"additionalProperties": true,
	}
	userAndPermissionObj := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id":       map[string]any{"type": "string"},
			"permission_id": map[string]any{"type": "string"},
		},
		"required":             []string{"user_id", "permission_id"},
		"additionalProperties": true,
	}

	return []toolDefinition{
		{Name: "gateway.services.list", Description: "List gateway services.", InputSchema: anyObj},
		{Name: "gateway.services.create", Description: "Create a gateway service.", InputSchema: anyObj},
		{Name: "gateway.services.update", Description: "Update a gateway service by id.", InputSchema: idObj},
		{Name: "gateway.services.delete", Description: "Delete a gateway service by id.", InputSchema: idObj},
		{Name: "gateway.routes.list", Description: "List gateway routes.", InputSchema: anyObj},
		{Name: "gateway.routes.create", Description: "Create a gateway route.", InputSchema: anyObj},
		{Name: "gateway.routes.update", Description: "Update a gateway route by id.", InputSchema: idObj},
		{Name: "gateway.routes.delete", Description: "Delete a gateway route by id.", InputSchema: idObj},
		{Name: "gateway.upstreams.list", Description: "List gateway upstreams.", InputSchema: anyObj},
		{Name: "gateway.upstreams.create", Description: "Create a gateway upstream.", InputSchema: anyObj},
		{Name: "gateway.upstreams.update", Description: "Update a gateway upstream by id.", InputSchema: idObj},
		{Name: "gateway.upstreams.delete", Description: "Delete a gateway upstream by id.", InputSchema: idObj},

		{Name: "users.list", Description: "List users with optional filters.", InputSchema: anyObj},
		{Name: "users.create", Description: "Create a user.", InputSchema: anyObj},
		{Name: "users.update", Description: "Update a user by id.", InputSchema: userIDObj},
		{Name: "users.suspend", Description: "Suspend a user by id.", InputSchema: userIDObj},
		{Name: "users.activate", Description: "Activate a user by id.", InputSchema: userIDObj},
		{Name: "users.apikeys.list", Description: "List a user's API keys.", InputSchema: userIDObj},
		{Name: "users.apikeys.create", Description: "Create an API key for a user.", InputSchema: userIDObj},
		{Name: "users.apikeys.revoke", Description: "Revoke a user API key.", InputSchema: userAndKeyObj},
		{Name: "users.permissions.list", Description: "List a user's endpoint permissions.", InputSchema: userIDObj},
		{Name: "users.permissions.grant", Description: "Grant a permission to a user.", InputSchema: userIDObj},
		{Name: "users.permissions.update", Description: "Update a user permission.", InputSchema: userAndPermissionObj},
		{Name: "users.permissions.revoke", Description: "Revoke a user permission.", InputSchema: userAndPermissionObj},

		{Name: "credits.overview", Description: "Get platform credit overview.", InputSchema: anyObj},
		{Name: "credits.balance", Description: "Get user credit balance.", InputSchema: userIDObj},
		{Name: "credits.topup", Description: "Top up user credits.", InputSchema: userIDObj},
		{Name: "credits.deduct", Description: "Deduct user credits.", InputSchema: userIDObj},
		{Name: "credits.transactions", Description: "List user credit transactions.", InputSchema: userIDObj},

		{Name: "audit.search", Description: "Search audit logs.", InputSchema: anyObj},
		{Name: "audit.detail", Description: "Get an audit log entry by id.", InputSchema: idObj},
		{Name: "audit.stats", Description: "Get audit log statistics.", InputSchema: anyObj},
		{Name: "audit.cleanup", Description: "Cleanup old audit logs.", InputSchema: anyObj},

		{Name: "analytics.overview", Description: "Get analytics overview.", InputSchema: anyObj},
		{Name: "analytics.top_routes", Description: "Get top routes analytics.", InputSchema: anyObj},
		{Name: "analytics.errors", Description: "Get analytics error breakdown.", InputSchema: anyObj},
		{Name: "analytics.latency", Description: "Get analytics latency stats.", InputSchema: anyObj},

		{Name: "cluster.status", Description: "Get cluster status.", InputSchema: anyObj},
		{Name: "cluster.nodes", Description: "Get cluster node list.", InputSchema: anyObj},

		{Name: "system.status", Description: "Get system status and server info.", InputSchema: anyObj},
		{Name: "system.config.export", Description: "Export current config as YAML and JSON.", InputSchema: anyObj},
		{Name: "system.config.import", Description: "Import config from inline yaml or config object (path: argument was removed in SEC-GQL-010).", InputSchema: anyObj},
		{Name: "system.reload", Description: "Trigger runtime config reload.", InputSchema: anyObj},
	}
}
