package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestHandleRequestCoreMethods(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()

	initialize := srv.HandleRequest(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	})
	if initialize.Error != nil {
		t.Fatalf("initialize failed: %v", initialize.Error)
	}

	tools := srv.HandleRequest(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	})
	if tools.Error != nil {
		t.Fatalf("tools/list failed: %v", tools.Error)
	}
	toolResult := mustMap(t, tools.Result)
	if sliceLen(toolResult["tools"]) == 0 {
		t.Fatalf("tools/list returned no tools: %#v", tools.Result)
	}

	resources := srv.HandleRequest(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "resources/list",
	})
	if resources.Error != nil {
		t.Fatalf("resources/list failed: %v", resources.Error)
	}
	resourceResult := mustMap(t, resources.Result)
	if sliceLen(resourceResult["resources"]) == 0 {
		t.Fatalf("resources/list returned no resources: %#v", resources.Result)
	}

	readParams := []byte(`{"uri":"apicerberus://config"}`)
	read := srv.HandleRequest(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "resources/read",
		Params:  readParams,
	})
	if read.Error != nil {
		t.Fatalf("resources/read failed: %v", read.Error)
	}
}

func TestGatewayToolsCRUD(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()
	ctx := context.Background()

	createdServiceRaw, err := srv.callTool(ctx, "gateway.services.create", map[string]any{
		"name":     "gateway-tool-service",
		"protocol": "http",
		"upstream": "up-1",
	})
	if err != nil {
		t.Fatalf("create service failed: %v", err)
	}
	createdService := mustMap(t, createdServiceRaw)
	serviceID := mustStringField(t, createdService, "id")

	if _, err := srv.callTool(ctx, "gateway.services.update", map[string]any{
		"id":       serviceID,
		"name":     "gateway-tool-service-v2",
		"protocol": "http",
		"upstream": "up-1",
	}); err != nil {
		t.Fatalf("update service failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "gateway.services.list", map[string]any{}); err != nil {
		t.Fatalf("list services failed: %v", err)
	}

	createdRouteRaw, err := srv.callTool(ctx, "gateway.routes.create", map[string]any{
		"name":    "gateway-tool-route",
		"service": serviceID,
		"paths":   []string{"/tool"},
		"methods": []string{"GET"},
	})
	if err != nil {
		t.Fatalf("create route failed: %v", err)
	}
	createdRoute := mustMap(t, createdRouteRaw)
	routeID := mustStringField(t, createdRoute, "id")

	if _, err := srv.callTool(ctx, "gateway.routes.update", map[string]any{
		"id":      routeID,
		"name":    "gateway-tool-route-v2",
		"service": serviceID,
		"paths":   []string{"/tool-v2"},
		"methods": []string{"GET"},
	}); err != nil {
		t.Fatalf("update route failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "gateway.routes.list", map[string]any{}); err != nil {
		t.Fatalf("list routes failed: %v", err)
	}

	createdUpstreamRaw, err := srv.callTool(ctx, "gateway.upstreams.create", map[string]any{
		"name":      "gateway-tool-upstream",
		"algorithm": "round_robin",
		"targets": []map[string]any{
			{"id": "tool-target-1", "address": "127.0.0.1:6553", "weight": 1},
		},
	})
	if err != nil {
		t.Fatalf("create upstream failed: %v", err)
	}
	createdUpstream := mustMap(t, createdUpstreamRaw)
	upstreamID := mustStringField(t, createdUpstream, "id")

	if _, err := srv.callTool(ctx, "gateway.upstreams.update", map[string]any{
		"id":        upstreamID,
		"name":      "gateway-tool-upstream-v2",
		"algorithm": "round_robin",
		"targets": []map[string]any{
			{"id": "tool-target-1", "address": "127.0.0.1:6553", "weight": 1},
		},
	}); err != nil {
		t.Fatalf("update upstream failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "gateway.upstreams.list", map[string]any{}); err != nil {
		t.Fatalf("list upstreams failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "gateway.routes.delete", map[string]any{"id": routeID}); err != nil {
		t.Fatalf("delete route failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "gateway.services.delete", map[string]any{"id": serviceID}); err != nil {
		t.Fatalf("delete service failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "gateway.upstreams.delete", map[string]any{"id": upstreamID}); err != nil {
		t.Fatalf("delete upstream failed: %v", err)
	}
}

func TestUserCreditAuditAnalyticsClusterSystemTools(t *testing.T) {
	srv := newTestServer(t)
	defer func() { _ = srv.Close() }()
	ctx := context.Background()

	createdUserRaw, err := srv.callTool(ctx, "users.create", map[string]any{
		"email":           "mcp-user@example.com",
		"name":            "MCP User",
		"password":        "secret123",
		"initial_credits": 0,
	})
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	createdUser := mustMap(t, createdUserRaw)
	userID := mustStringField(t, createdUser, "id")

	if _, err := srv.callTool(ctx, "users.list", map[string]any{"limit": 20}); err != nil {
		t.Fatalf("list users failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "users.update", map[string]any{
		"user_id": userID,
		"name":    "MCP User Updated",
	}); err != nil {
		t.Fatalf("update user failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "users.suspend", map[string]any{"user_id": userID}); err != nil {
		t.Fatalf("suspend user failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "users.activate", map[string]any{"user_id": userID}); err != nil {
		t.Fatalf("activate user failed: %v", err)
	}

	createdKeyRaw, err := srv.callTool(ctx, "users.apikeys.create", map[string]any{
		"user_id": userID,
		"name":    "MCP Key",
		"mode":    "test",
	})
	if err != nil {
		t.Fatalf("create user api key failed: %v", err)
	}
	createdKey := mustMap(t, createdKeyRaw)
	key := mustMap(t, createdKey["key"])
	keyID := mustStringField(t, key, "id")
	if _, err := srv.callTool(ctx, "users.apikeys.list", map[string]any{"user_id": userID}); err != nil {
		t.Fatalf("list user api keys failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "users.apikeys.revoke", map[string]any{
		"user_id": userID,
		"key_id":  keyID,
	}); err != nil {
		t.Fatalf("revoke user api key failed: %v", err)
	}

	createdPermissionRaw, err := srv.callTool(ctx, "users.permissions.grant", map[string]any{
		"user_id":  userID,
		"route_id": "route-1",
		"methods":  []string{"GET"},
		"allowed":  true,
	})
	if err != nil {
		t.Fatalf("grant user permission failed: %v", err)
	}
	createdPermission := mustMap(t, createdPermissionRaw)
	permissionID := mustStringField(t, createdPermission, "id")
	if _, err := srv.callTool(ctx, "users.permissions.list", map[string]any{"user_id": userID}); err != nil {
		t.Fatalf("list user permissions failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "users.permissions.update", map[string]any{
		"user_id":       userID,
		"permission_id": permissionID,
		"route_id":      "route-1",
		"methods":       []string{"GET"},
		"allowed":       true,
	}); err != nil {
		t.Fatalf("update user permission failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "users.permissions.revoke", map[string]any{
		"user_id":       userID,
		"permission_id": permissionID,
	}); err != nil {
		t.Fatalf("revoke user permission failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "credits.overview", map[string]any{}); err != nil {
		t.Fatalf("credit overview failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "credits.topup", map[string]any{"user_id": userID, "amount": 50, "reason": "test topup"}); err != nil {
		t.Fatalf("credit topup failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "credits.deduct", map[string]any{"user_id": userID, "amount": 5, "reason": "test deduct"}); err != nil {
		t.Fatalf("credit deduct failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "credits.balance", map[string]any{"user_id": userID}); err != nil {
		t.Fatalf("credit balance failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "credits.transactions", map[string]any{"user_id": userID, "limit": 10}); err != nil {
		t.Fatalf("credit transactions failed: %v", err)
	}

	cfg := cloneConfig(srv.cfg)
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store for audit fixture failed: %v", err)
	}
	defer st.Close()
	now := time.Now().UTC()
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:          "audit-entry-1",
			RequestID:   "req-1",
			RouteID:     "route-1",
			RouteName:   "route-1",
			ServiceName: "service-1",
			UserID:      userID,
			Method:      "GET",
			Path:        "/",
			StatusCode:  200,
			CreatedAt:   now,
		},
	}); err != nil {
		t.Fatalf("insert audit fixture failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "audit.search", map[string]any{"limit": 10}); err != nil {
		t.Fatalf("audit search failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "audit.detail", map[string]any{"id": "audit-entry-1"}); err != nil {
		t.Fatalf("audit detail failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "audit.stats", map[string]any{}); err != nil {
		t.Fatalf("audit stats failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "audit.cleanup", map[string]any{
		"cutoff": now.Add(time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("audit cleanup failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "analytics.overview", map[string]any{}); err != nil {
		t.Fatalf("analytics overview failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "analytics.top_routes", map[string]any{}); err != nil {
		t.Fatalf("analytics top routes failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "analytics.errors", map[string]any{}); err != nil {
		t.Fatalf("analytics errors failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "analytics.latency", map[string]any{}); err != nil {
		t.Fatalf("analytics latency failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "cluster.status", map[string]any{}); err != nil {
		t.Fatalf("cluster status failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "cluster.nodes", map[string]any{}); err != nil {
		t.Fatalf("cluster nodes failed: %v", err)
	}

	if _, err := srv.callTool(ctx, "system.status", map[string]any{}); err != nil {
		t.Fatalf("system status failed: %v", err)
	}
	exportedRaw, err := srv.callTool(ctx, "system.config.export", map[string]any{})
	if err != nil {
		t.Fatalf("system config export failed: %v", err)
	}
	exported := mustMap(t, exportedRaw)
	exportedConfig := exported["config"]
	if exportedConfig == nil {
		t.Fatalf("expected exported config payload, got %#v", exported)
	}

	if _, err := srv.callTool(ctx, "system.reload", map[string]any{}); err != nil {
		t.Fatalf("system reload failed: %v", err)
	}
	if _, err := srv.callTool(ctx, "system.config.import", map[string]any{"config": exportedConfig}); err != nil {
		t.Fatalf("system config import failed: %v", err)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       ":0",
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			IdleTimeout:    5 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		Admin: config.AdminConfig{
			Addr:      ":0",
			APIKey:    "test-admin-key",
			UIEnabled: false,
			UIPath:    "/dashboard",
		},
		Store: config.StoreConfig{
			Path:        filepath.Join(t.TempDir(), "mcp-test.db"),
			BusyTimeout: 2 * time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Billing: config.BillingConfig{
			Enabled:           true,
			DefaultCost:       1,
			RouteCosts:        map[string]int64{},
			MethodMultipliers: map[string]float64{},
			TestModeEnabled:   false,
			ZeroBalanceAction: "reject",
		},
		Audit: config.AuditConfig{
			Enabled:            true,
			BufferSize:         64,
			BatchSize:          10,
			FlushInterval:      time.Second,
			RetentionDays:      7,
			RouteRetentionDays: map[string]int{},
		},
		Services: []config.Service{
			{
				ID:       "svc-1",
				Name:     "service-1",
				Protocol: "http",
				Upstream: "up-1",
			},
		},
		Routes: []config.Route{
			{
				ID:      "route-1",
				Name:    "route-1",
				Service: "svc-1",
				Paths:   []string{"/"},
				Methods: []string{"GET"},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-1",
				Name:      "upstream-1",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-target-1", Address: "127.0.0.1:65535", Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           2 * time.Second,
						Timeout:            time.Second,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
		Auth: config.AuthConfig{
			APIKey: config.APIKeyAuthConfig{
				KeyNames:    []string{"X-API-Key"},
				QueryNames:  []string{"apikey"},
				CookieNames: []string{"apikey"},
			},
		},
	}
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("new mcp server: %v", err)
	}
	return srv
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T (%#v)", value, value)
	}
	return out
}

func mustStringField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	value, ok := m[key]
	if !ok {
		for k, v := range m {
			if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(key)) {
				value = v
				ok = true
				break
			}
		}
	}
	if !ok {
		t.Fatalf("expected key %q (case-insensitive) in %#v", key, m)
	}
	text := asString(value)
	if text == "" {
		t.Fatalf("expected non-empty string in key %q, got %#v", key, value)
	}
	return text
}

func sliceLen(value any) int {
	switch v := value.(type) {
	case []any:
		return len(v)
	case []map[string]any:
		return len(v)
	default:
		return 0
	}
}
