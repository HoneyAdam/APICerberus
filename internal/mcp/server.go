package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	yamlpkg "github.com/APICerberus/APICerebrus/internal/pkg/yaml"
	"github.com/APICerberus/APICerebrus/internal/version"
)

type toolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type resourceDefinition struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

// Server implements an MCP-compatible JSON-RPC server.
type Server struct {
	mu         sync.RWMutex
	cfg        *config.Config
	gateway    *gateway.Gateway
	admin      *admin.Server
	adminToken string
}

// NewServer builds a new MCP server and in-process admin runtime.
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	runtimeCfg := cloneConfig(cfg)
	gw, adminSrv, err := buildRuntime(runtimeCfg)
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:     runtimeCfg,
		gateway: gw,
		admin:   adminSrv,
	}, nil
}

// Close releases runtime resources.
func (s *Server) Close() error {
	s.mu.Lock()
	gw := s.gateway
	s.gateway = nil
	s.admin = nil
	s.mu.Unlock()

	if gw == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return gw.Shutdown(ctx)
}

func buildRuntime(cfg *config.Config) (*gateway.Gateway, *admin.Server, error) {
	gw, err := gateway.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize gateway runtime: %w", err)
	}
	adminSrv, err := admin.NewServer(cfg, gw)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = gw.Shutdown(ctx)
		cancel()
		return nil, nil, fmt.Errorf("initialize admin runtime: %w", err)
	}
	return gw, adminSrv, nil
}

// HandleRequest processes one JSON-RPC request.
func (s *Server) HandleRequest(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	if strings.TrimSpace(req.JSONRPC) != jsonRPCVersion {
		return errorResponse(req.ID, -32600, "invalid request: jsonrpc must be 2.0", nil)
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		return errorResponse(req.ID, -32600, "invalid request: method is required", nil)
	}

	switch method {
	case "initialize":
		return successResponse(req.ID, map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo": map[string]any{
				"name":    "apicerberus-mcp",
				"version": version.Version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{
					"listChanged": false,
				},
				"resources": map[string]any{
					"listChanged": false,
					"subscribe":   false,
				},
			},
		})
	case "tools/list":
		tools := s.toolDefinitions()
		items := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			items = append(items, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			})
		}
		return successResponse(req.ID, map[string]any{
			"tools": items,
		})
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := decodeParams(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid params", err.Error())
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return errorResponse(req.ID, -32602, "invalid params: tool name is required", nil)
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		result, err := s.callTool(ctx, name, params.Arguments)
		if err != nil {
			return successResponse(req.ID, map[string]any{
				"isError": true,
				"content": []map[string]any{
					{"type": "text", "text": err.Error()},
				},
			})
		}
		return successResponse(req.ID, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "ok"},
			},
			"structuredContent": result,
		})
	case "resources/list":
		resources := s.resourceDefinitions()
		items := make([]map[string]any, 0, len(resources))
		for _, resource := range resources {
			items = append(items, map[string]any{
				"uri":         resource.URI,
				"name":        resource.Name,
				"description": resource.Description,
				"mimeType":    resource.MimeType,
			})
		}
		return successResponse(req.ID, map[string]any{
			"resources": items,
		})
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := decodeParams(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid params", err.Error())
		}
		uri := strings.TrimSpace(params.URI)
		if uri == "" {
			return errorResponse(req.ID, -32602, "invalid params: uri is required", nil)
		}
		resource, err := s.readResource(ctx, uri)
		if err != nil {
			return errorResponse(req.ID, -32000, "resource read failed", err.Error())
		}
		return successResponse(req.ID, map[string]any{
			"contents": []map[string]any{resource},
		})
	default:
		return errorResponse(req.ID, -32601, "method not found", nil)
	}
}

func decodeParams(raw json.RawMessage, out any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// RunStdio serves JSON-RPC over stdin/stdout.
func (s *Server) RunStdio(ctx context.Context) error {
	decoder := json.NewDecoder(bufio.NewReader(os.Stdin))
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var req JSONRPCRequest
		if err := decoder.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			resp := errorResponse(nil, -32700, "parse error", err.Error())
			if encErr := encoder.Encode(resp); encErr != nil {
				return encErr
			}
			continue
		}

		resp := s.HandleRequest(ctx, req)
		if req.ID == nil {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
}

// RunSSE exposes JSON-RPC over HTTP and a lightweight SSE stream.
func (s *Server) RunSSE(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(errorResponse(nil, -32700, "parse error", err.Error()))
			return
		}
		resp := s.HandleRequest(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		_, _ = fmt.Fprintf(w, "event: ready\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = fmt.Fprintf(w, "event: heartbeat\ndata: {\"ts\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339Nano))
				flusher.Flush()
			}
		}
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() { // #nosec G118 -- goroutine waits on the request-scoped ctx captured in closure.
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = server.Shutdown(shutdownCtx)
		cancel()
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

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
		{Name: "system.config.import", Description: "Import config from path, yaml, or config object.", InputSchema: anyObj},
		{Name: "system.reload", Description: "Trigger runtime config reload.", InputSchema: anyObj},
	}
}

func (s *Server) callTool(ctx context.Context, name string, args map[string]any) (any, error) {
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

	case "analytics.overview":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/overview", nil, queryFromArgs(args))
	case "analytics.top_routes":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/top-routes", nil, queryFromArgs(args))
	case "analytics.errors":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/errors", nil, queryFromArgs(args))
	case "analytics.latency":
		return s.callAdmin(http.MethodGet, "/admin/api/v1/analytics/latency", nil, queryFromArgs(args))

	case "cluster.status":
		return map[string]any{
			"mode":       "standalone",
			"status":     "healthy",
			"leader":     "local",
			"node_count": 1,
		}, nil
	case "cluster.nodes":
		return []map[string]any{
			{
				"id":      "local",
				"name":    "local-node",
				"role":    "leader",
				"healthy": true,
				"address": "127.0.0.1",
				"metadata": map[string]any{
					"mode": "standalone",
				},
			},
		}, nil

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
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *Server) resourceDefinitions() []resourceDefinition {
	return []resourceDefinition{
		{
			URI:         "apicerberus://services",
			Name:        "Services",
			Description: "Current gateway services.",
			MimeType:    "application/json",
		},
		{
			URI:         "apicerberus://routes",
			Name:        "Routes",
			Description: "Current gateway routes.",
			MimeType:    "application/json",
		},
		{
			URI:         "apicerberus://upstreams",
			Name:        "Upstreams",
			Description: "Current gateway upstreams.",
			MimeType:    "application/json",
		},
		{
			URI:         "apicerberus://users",
			Name:        "Users",
			Description: "Platform users.",
			MimeType:    "application/json",
		},
		{
			URI:         "apicerberus://credits/overview",
			Name:        "Credits Overview",
			Description: "Platform credit distribution and usage summary.",
			MimeType:    "application/json",
		},
		{
			URI:         "apicerberus://analytics/overview",
			Name:        "Analytics Overview",
			Description: "High-level analytics metrics.",
			MimeType:    "application/json",
		},
		{
			URI:         "apicerberus://config",
			Name:        "Runtime Config",
			Description: "Current runtime config snapshot.",
			MimeType:    "application/json",
		},
	}
}

func (s *Server) readResource(ctx context.Context, uri string) (map[string]any, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse resource uri: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "apicerberus") {
		return nil, fmt.Errorf("unsupported resource scheme: %s", parsed.Scheme)
	}
	resourceKey := parsed.Host + parsed.Path
	var value any
	switch resourceKey {
	case "services":
		value, err = s.callTool(ctx, "gateway.services.list", map[string]any{})
	case "routes":
		value, err = s.callTool(ctx, "gateway.routes.list", map[string]any{})
	case "upstreams":
		value, err = s.callTool(ctx, "gateway.upstreams.list", map[string]any{})
	case "users":
		value, err = s.callTool(ctx, "users.list", map[string]any{"limit": 100})
	case "credits/overview":
		value, err = s.callTool(ctx, "credits.overview", map[string]any{})
	case "analytics/overview":
		value, err = s.callTool(ctx, "analytics.overview", map[string]any{})
	case "config":
		value, err = s.callTool(ctx, "system.config.export", map[string]any{})
	default:
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
	if err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource: %w", err)
	}
	return map[string]any{
		"uri":      uri,
		"mimeType": "application/json",
		"text":     string(raw),
	}, nil
}

func (s *Server) exportConfig() (map[string]any, error) {
	s.mu.RLock()
	cfg := cloneConfig(s.cfg)
	s.mu.RUnlock()

	yamlBytes, err := yamlpkg.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config yaml: %w", err)
	}
	return map[string]any{
		"config": cfg,
		"yaml":   string(yamlBytes),
	}, nil
}

func loadConfigFromArgs(args map[string]any) (*config.Config, error) {
	if path := strings.TrimSpace(asString(args["path"])); path != "" {
		return config.Load(path)
	}
	if rawYAML := strings.TrimSpace(asString(args["yaml"])); rawYAML != "" {
		return loadConfigFromYAML(rawYAML)
	}
	if rawConfig, ok := args["config"]; ok && rawConfig != nil {
		encoded, err := json.Marshal(rawConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal config payload: %w", err)
		}
		var cfg config.Config
		if err := json.Unmarshal(encoded, &cfg); err != nil {
			return nil, fmt.Errorf("decode config payload: %w", err)
		}
		return &cfg, nil
	}
	return nil, errors.New("config import requires one of: path, yaml, or config")
}

func loadConfigFromYAML(raw string) (*config.Config, error) {
	cfg, err := loadConfigFromYAMLRaw(raw)
	if err == nil {
		return cfg, nil
	}

	normalized := normalizeYAMLForConfigParser(raw)
	if normalized == raw {
		return nil, err
	}
	cfg, normalizedErr := loadConfigFromYAMLRaw(normalized)
	if normalizedErr != nil {
		return nil, err
	}
	return cfg, nil
}

func loadConfigFromYAMLRaw(raw string) (*config.Config, error) {
	temp, err := os.CreateTemp("", "apicerberus-mcp-import-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create temp config file: %w", err)
	}
	path := temp.Name()
	_ = temp.Close()
	defer os.Remove(path)

	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		return nil, fmt.Errorf("write temp config file: %w", err)
	}
	return config.Load(path)
}

func normalizeYAMLForConfigParser(raw string) string {
	out := raw
	// Our custom YAML parser is stricter than flow-style YAML in some cases.
	out = strings.ReplaceAll(out, ": {}", ":")
	out = strings.ReplaceAll(out, ": []", ":")
	return out
}

func (s *Server) swapRuntime(newCfg *config.Config) error {
	if newCfg == nil {
		return errors.New("new config is nil")
	}
	runtimeCfg := cloneConfig(newCfg)
	newGateway, newAdmin, err := buildRuntime(runtimeCfg)
	if err != nil {
		return err
	}

	s.mu.Lock()
	oldGateway := s.gateway
	s.cfg = runtimeCfg
	s.gateway = newGateway
	s.admin = newAdmin
	s.adminToken = ""
	s.mu.Unlock()

	if oldGateway != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = oldGateway.Shutdown(ctx)
		cancel()
	}
	return nil
}

func (s *Server) ensureAdminToken(adminSrv *admin.Server, adminKey string) (string, error) {
	s.mu.RLock()
	token := s.adminToken
	s.mu.RUnlock()
	if token != "" {
		return token, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.adminToken != "" {
		return s.adminToken, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/api/v1/auth/token", nil)
	req.Header.Set("X-Admin-Key", adminKey)
	rec := httptest.NewRecorder()
	adminSrv.ServeHTTP(rec, req)

	responseBytes := bytes.TrimSpace(rec.Body.Bytes())
	var parsed any
	if len(responseBytes) > 0 {
		if err := json.Unmarshal(responseBytes, &parsed); err != nil {
			parsed = string(responseBytes)
		}
	}
	if rec.Code >= 400 {
		return "", fmt.Errorf("admin token exchange failed: %s", extractAdminError(parsed, rec.Code))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(responseBytes, &result); err != nil {
		return "", fmt.Errorf("unmarshal admin token response: %w", err)
	}
	if result.Token == "" {
		return "", errors.New("admin token exchange returned empty token")
	}
	s.adminToken = result.Token
	return result.Token, nil
}

func (s *Server) callAdmin(method, path string, payload any, query url.Values) (any, error) {
	s.mu.RLock()
	adminSrv := s.admin
	adminKey := ""
	if s.cfg != nil {
		adminKey = s.cfg.Admin.APIKey
	}
	s.mu.RUnlock()
	if adminSrv == nil {
		return nil, errors.New("admin runtime is not available")
	}

	token, err := s.ensureAdminToken(adminSrv, adminKey)
	if err != nil {
		return nil, err
	}

	requestPath := path
	if encoded := strings.TrimSpace(query.Encode()); encoded != "" {
		requestPath += "?" + encoded
	}

	var body io.Reader = http.NoBody
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal admin payload: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, requestPath, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	adminSrv.ServeHTTP(rec, req)

	responseBytes := bytes.TrimSpace(rec.Body.Bytes())
	var parsed any
	if len(responseBytes) > 0 {
		if err := json.Unmarshal(responseBytes, &parsed); err != nil {
			parsed = string(responseBytes)
		}
	}

	if rec.Code >= 400 {
		return nil, fmt.Errorf("admin api %s %s failed: %s", method, path, extractAdminError(parsed, rec.Code))
	}
	if rec.Code == http.StatusNoContent || len(responseBytes) == 0 {
		return map[string]any{"ok": true}, nil
	}
	return parsed, nil
}

func extractAdminError(payload any, status int) string {
	if payload == nil {
		return fmt.Sprintf("http %d", status)
	}
	asMap, ok := payload.(map[string]any)
	if !ok {
		return asString(payload)
	}
	rawErr, ok := asMap["error"]
	if !ok {
		return asString(payload)
	}
	errMap, ok := rawErr.(map[string]any)
	if !ok {
		return asString(rawErr)
	}
	message := strings.TrimSpace(asString(errMap["message"]))
	if message == "" {
		message = fmt.Sprintf("http %d", status)
	}
	code := strings.TrimSpace(asString(errMap["code"]))
	if code == "" {
		return message
	}
	return code + ": " + message
}

func queryFromArgs(args map[string]any, ignoreKeys ...string) url.Values {
	values := url.Values{}
	if len(args) == 0 {
		return values
	}
	ignored := make(map[string]struct{}, len(ignoreKeys))
	for _, key := range ignoreKeys {
		ignored[strings.TrimSpace(key)] = struct{}{}
	}
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := ignored[key]; ok {
			continue
		}
		appendQueryValue(values, key, value)
	}
	return values
}

func appendQueryValue(values url.Values, key string, value any) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		if strings.TrimSpace(v) == "" {
			return
		}
		values.Set(key, v)
	case []string:
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			values.Add(key, item)
		}
	case []any:
		for _, item := range v {
			text := strings.TrimSpace(asString(item))
			if text == "" {
				continue
			}
			values.Add(key, text)
		}
	default:
		values.Set(key, asString(value))
	}
}

func payloadFromArgs(args map[string]any, nestedKey string, ignoreKeys ...string) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	if nestedKey != "" {
		if raw, ok := args[nestedKey]; ok {
			if payload, ok := raw.(map[string]any); ok {
				return cloneAnyMap(payload)
			}
		}
	}

	ignored := make(map[string]struct{}, len(ignoreKeys))
	for _, key := range ignoreKeys {
		ignored[strings.TrimSpace(key)] = struct{}{}
	}
	if nestedKey != "" {
		ignored[strings.TrimSpace(nestedKey)] = struct{}{}
	}

	out := make(map[string]any, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := ignored[key]; ok {
			continue
		}
		out[key] = value
	}
	return out
}

func requireString(args map[string]any, key string) (string, error) {
	value := strings.TrimSpace(asString(args[key]))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requireAnyString(args map[string]any, keys ...string) (string, error) {
	for _, key := range keys {
		value := strings.TrimSpace(asString(args[key]))
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s is required", strings.Join(keys, " or "))
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneConfig(src *config.Config) *config.Config {
	if src == nil {
		return &config.Config{}
	}
	out := *src
	if len(src.Audit.RouteRetentionDays) > 0 {
		out.Audit.RouteRetentionDays = make(map[string]int, len(src.Audit.RouteRetentionDays))
		for route, days := range src.Audit.RouteRetentionDays {
			out.Audit.RouteRetentionDays[route] = days
		}
	}
	out.Billing = cloneBillingConfig(src.Billing)
	out.Services = append([]config.Service(nil), src.Services...)
	out.Routes = append([]config.Route(nil), src.Routes...)
	out.GlobalPlugins = clonePluginConfigs(src.GlobalPlugins)
	for i := range out.Routes {
		out.Routes[i].Plugins = clonePluginConfigs(src.Routes[i].Plugins)
	}

	out.Upstreams = append([]config.Upstream(nil), src.Upstreams...)
	for i := range out.Upstreams {
		out.Upstreams[i].Targets = append([]config.UpstreamTarget(nil), src.Upstreams[i].Targets...)
	}

	out.Consumers = append([]config.Consumer(nil), src.Consumers...)
	for i := range out.Consumers {
		out.Consumers[i].APIKeys = append([]config.ConsumerAPIKey(nil), src.Consumers[i].APIKeys...)
		out.Consumers[i].ACLGroups = append([]string(nil), src.Consumers[i].ACLGroups...)
		if src.Consumers[i].Metadata != nil {
			out.Consumers[i].Metadata = make(map[string]any, len(src.Consumers[i].Metadata))
			for k, v := range src.Consumers[i].Metadata {
				out.Consumers[i].Metadata[k] = v
			}
		}
	}
	out.Auth.APIKey.KeyNames = append([]string(nil), src.Auth.APIKey.KeyNames...)
	out.Auth.APIKey.QueryNames = append([]string(nil), src.Auth.APIKey.QueryNames...)
	out.Auth.APIKey.CookieNames = append([]string(nil), src.Auth.APIKey.CookieNames...)
	return &out
}

func clonePluginConfigs(in []config.PluginConfig) []config.PluginConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.PluginConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].Enabled != nil {
			v := *in[i].Enabled
			out[i].Enabled = &v
		}
		if in[i].Config != nil {
			out[i].Config = make(map[string]any, len(in[i].Config))
			for k, v := range in[i].Config {
				out[i].Config[k] = v
			}
		}
	}
	return out
}

func cloneBillingConfig(in config.BillingConfig) config.BillingConfig {
	out := in
	out.RouteCosts = cloneBillingRouteCosts(in.RouteCosts)
	out.MethodMultipliers = cloneBillingMethodMultipliers(in.MethodMultipliers)
	return out
}

func cloneBillingRouteCosts(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return map[string]int64{}
	}
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBillingMethodMultipliers(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func asInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}
