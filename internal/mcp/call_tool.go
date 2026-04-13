package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// callTool dispatches a tool call by name to the appropriate handler.
func (s *Server) callTool(ctx context.Context, name string, args map[string]any) (any, error) {
	// --- Gateway tools ---
	switch name {
	case "gateway.services.list":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/services", nil, nil)
	case "gateway.services.create":
		return s.callAdmin(http.MethodPost, "/admin/api/v1/services", payloadFromArgs(args, "service"), nil)
	case "gateway.services.update":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPut, "/admin/api/v1/services/"+url.PathEscape(id), payloadFromArgs(args, "service", "id"), nil)
	case "gateway.services.delete":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		if _, err := s.callAdmin(http.MethodDelete, "/admin/api/v1/services/"+url.PathEscape(id), nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "id": id}, nil

	case "gateway.routes.list":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/routes", nil, nil)
	case "gateway.routes.create":
		return s.callAdmin(http.MethodPost, "/admin/api/v1/routes", payloadFromArgs(args, "route"), nil)
	case "gateway.routes.update":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPut, "/admin/api/v1/routes/"+url.PathEscape(id), payloadFromArgs(args, "route", "id"), nil)
	case "gateway.routes.delete":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		if _, err := s.callAdmin(http.MethodDelete, "/admin/api/v1/routes/"+url.PathEscape(id), nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "id": id}, nil

	case "gateway.upstreams.list":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/upstreams", nil, nil)
	case "gateway.upstreams.create":
		return s.callAdmin(http.MethodPost, "/admin/api/v1/upstreams", payloadFromArgs(args, "upstream"), nil)
	case "gateway.upstreams.update":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPut, "/admin/api/v1/upstreams/"+url.PathEscape(id), payloadFromArgs(args, "upstream", "id"), nil)
	case "gateway.upstreams.delete":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		if _, err := s.callAdmin(http.MethodDelete, "/admin/api/v1/upstreams/"+url.PathEscape(id), nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "id": id}, nil
	}

	// --- User tools ---
	switch name {
	case "users.list":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/users", nil, queryFromArgs(args))
	case "users.create":
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users", payloadFromArgs(args, "user"), nil)
	case "users.update":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPut, "/admin/api/v1/users/"+url.PathEscape(userID), payloadFromArgs(args, "user", "user_id", "id"), nil)
	case "users.suspend":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users/"+url.PathEscape(userID)+"/suspend", nil, nil)
	case "users.activate":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users/"+url.PathEscape(userID)+"/activate", nil, nil)

	case "users.apikeys.list":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodGet, "/admin/api/v1/users/"+url.PathEscape(userID)+"/api-keys", nil, nil)
	case "users.apikeys.create":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users/"+url.PathEscape(userID)+"/api-keys", payloadFromArgs(args, "api_key", "user_id", "id"), nil)
	case "users.apikeys.revoke":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		keyID, err := requireAnyString(args, "key_id", "id")
		if err != nil {
			return nil, err
		}
		if _, err := s.callAdmin(http.MethodDelete, "/admin/api/v1/users/"+url.PathEscape(userID)+"/api-keys/"+url.PathEscape(keyID), nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"revoked": true, "user_id": userID, "key_id": keyID}, nil

	case "users.permissions.list":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodGet, "/admin/api/v1/users/"+url.PathEscape(userID)+"/permissions", nil, nil)
	case "users.permissions.grant":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users/"+url.PathEscape(userID)+"/permissions", payloadFromArgs(args, "permission", "user_id", "id"), nil)
	case "users.permissions.update":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		permissionID, err := requireAnyString(args, "permission_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodPut, "/admin/api/v1/users/"+url.PathEscape(userID)+"/permissions/"+url.PathEscape(permissionID), payloadFromArgs(args, "permission", "user_id", "permission_id", "id"), nil)
	case "users.permissions.revoke":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		permissionID, err := requireAnyString(args, "permission_id", "id")
		if err != nil {
			return nil, err
		}
		if _, err := s.callAdmin(http.MethodDelete, "/admin/api/v1/users/"+url.PathEscape(userID)+"/permissions/"+url.PathEscape(permissionID), nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"revoked": true, "user_id": userID, "permission_id": permissionID}, nil
	}

	// --- Credit tools ---
	switch name {
	case "credits.overview":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/credits/overview", nil, nil)
	case "credits.balance":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodGet, "/admin/api/v1/users/"+url.PathEscape(userID)+"/credits/balance", nil, nil)
	case "credits.topup":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		payload := payloadFromArgs(args, "payload", "user_id", "id")
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users/"+url.PathEscape(userID)+"/credits/topup", payload, nil)
	case "credits.deduct":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		payload := payloadFromArgs(args, "payload", "user_id", "id")
		return s.callAdmin(http.MethodPost, "/admin/api/v1/users/"+url.PathEscape(userID)+"/credits/deduct", payload, nil)
	case "credits.transactions":
		userID, err := requireAnyString(args, "user_id", "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodGet, "/admin/api/v1/users/"+url.PathEscape(userID)+"/credits/transactions", nil, queryFromArgs(args, "user_id", "id"))
	}

	// --- Audit tools ---
	switch name {
	case "audit.search":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/audit-logs", nil, queryFromArgs(args))
	case "audit.detail":
		id, err := requireString(args, "id")
		if err != nil {
			return nil, err
		}
		return s.callAdmin(http.MethodGet, "/admin/api/v1/audit-logs/"+url.PathEscape(id), nil, nil)
	case "audit.stats":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/audit-logs/stats", nil, queryFromArgs(args))
	case "audit.cleanup":
		return s.callAdmin(http.MethodDelete, "/admin/api/v1/audit-logs/cleanup", nil, queryFromArgs(args))
	}

	// --- Analytics tools ---
	switch name {
	case "analytics.overview":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/overview", nil, queryFromArgs(args))
	case "analytics.top_routes":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/top-routes", nil, queryFromArgs(args))
	case "analytics.errors":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/errors", nil, queryFromArgs(args))
	case "analytics.latency":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/latency", nil, queryFromArgs(args))
	}

	// --- Cluster tools ---
	switch name {
	case "cluster.status":
		s.mu.RLock()
		node := s.raftNode
		s.mu.RUnlock()
		if node == nil {
			return map[string]any{
				"mode":       "standalone",
				"status":     "healthy",
				"leader":     "local",
				"node_count": 1,
			}, nil
		}
		peers := node.Peers
		nodeCount := len(peers) + 1
		return map[string]any{
			"mode":         "cluster",
			"status":       node.GetState().String(),
			"leader":       node.GetLeaderID(),
			"node_count":   nodeCount,
			"node_id":      node.ID,
			"term":         node.GetTerm(),
			"commit_index": node.CommitIndex,
			"last_applied": node.LastApplied,
			"peer_count":   len(peers),
		}, nil
	case "cluster.nodes":
		s.mu.RLock()
		node := s.raftNode
		s.mu.RUnlock()
		if node == nil {
			return []map[string]any{
				{
					"id":       "local",
					"name":     "local-node",
					"role":     "leader",
					"healthy":  true,
					"address":  "127.0.0.1",
					"metadata": map[string]any{"mode": "standalone"},
				},
			}, nil
		}
		nodes := []map[string]any{
			{
				"id":        node.ID,
				"address":   node.Address,
				"role":      node.State.String(),
				"is_leader": node.IsLeader(),
				"healthy":   true,
				"metadata": map[string]any{
					"term":         node.GetTerm(),
					"commit_index": node.CommitIndex,
					"last_applied": node.LastApplied,
				},
			},
		}
		for id, addr := range node.Peers {
			nodes = append(nodes, map[string]any{
				"id":        id,
				"address":   addr,
				"role":      "Unknown",
				"is_leader": false,
				"healthy":   true,
				"metadata":  map[string]any{},
			})
		}
		return nodes, nil
	}

	// --- System tools ---
	switch name {
	case "system.status":
		status, err := s.callAdmin(http.MethodGet, "/admin/api/v1/status", nil, nil)
		if err != nil {
			return nil, err
		}
		info, err := s.callAdmin(http.MethodGet, "/admin/api/v1/info", nil, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"status": status,
			"info":   info,
		}, nil
	case "system.reload":
		return s.callAdmin(http.MethodPost, "/admin/api/v1/config/reload", nil, nil)
	case "system.config.export":
		return s.exportConfig()
	case "system.config.import":
		cfg, err := loadConfigFromArgs(args)
		if err != nil {
			return nil, err
		}
		if err := s.swapRuntime(cfg); err != nil {
			return nil, err
		}
		return map[string]any{
			"imported":  true,
			"services":  len(cfg.Services),
			"routes":    len(cfg.Routes),
			"upstreams": len(cfg.Upstreams),
		}, nil
	}

	return nil, fmt.Errorf("unknown tool: %s", name)
}
