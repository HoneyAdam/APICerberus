package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	"github.com/APICerberus/APICerebrus/internal/raft"
	"github.com/APICerberus/APICerebrus/internal/version"
)

// Server implements an MCP-compatible JSON-RPC server.
type Server struct {
	mu         sync.RWMutex
	cfg        *config.Config
	gateway    *gateway.Gateway
	admin      *admin.Server
	adminToken string
	raftNode   *raft.Node
}

// NewServer builds a new MCP server and in-process admin runtime.
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	runtimeCfg := config.CloneConfig(cfg)
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

// SetRaftNode wires an optional Raft node for live cluster queries.
func (s *Server) SetRaftNode(node *raft.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raftNode = node
}

// Close releases runtime resources.
func (s *Server) Close() error {
	s.mu.Lock()
	gw := s.gateway
	node := s.raftNode
	s.gateway = nil
	s.admin = nil
	s.raftNode = nil
	s.mu.Unlock()

	var shutdownErr error
	if node != nil {
		if err := node.Stop(); err != nil {
			shutdownErr = err
		}
	}
	if gw == nil {
		return shutdownErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := gw.Shutdown(ctx); err != nil {
		if shutdownErr != nil {
			return fmt.Errorf("raft stop: %w; gateway shutdown: %v", shutdownErr, err)
		}
		return err
	}
	return shutdownErr
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
			return errorResponse(req.ID, -32603, err.Error(), nil)
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
		// Require X-Admin-Key header matching the configured admin key.
		// Stdio transport is inherently local; SSE transport is network-accessible and needs auth.
		s.mu.RLock()
		adminKey := ""
		if s.cfg != nil {
			adminKey = s.cfg.Admin.APIKey
		}
		s.mu.RUnlock()
		if adminKey == "" || !secureCompare(r.Header.Get("X-Admin-Key"), adminKey) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
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

func (s *Server) ensureAdminToken(adminSrv *admin.Server, adminKey string) (string, error) {
	// First check without lock (racy but fast path)
	s.mu.RLock()
	token := s.adminToken
	s.mu.RUnlock()
	if token != "" {
		return token, nil
	}

	// Slow path: acquire exclusive lock and double-check
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

	// Token is delivered via admin_session cookie, not response body (CWE-319)
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "apicerberus_admin_session" {
			s.adminToken = c.Value
			return c.Value, nil
		}
	}
	return "", errors.New("admin token exchange returned no session cookie")
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
		return coerce.AsString(payload)
	}
	rawErr, ok := asMap["error"]
	if !ok {
		return coerce.AsString(payload)
	}
	errMap, ok := rawErr.(map[string]any)
	if !ok {
		return coerce.AsString(rawErr)
	}
	message := strings.TrimSpace(coerce.AsString(errMap["message"]))
	if message == "" {
		message = fmt.Sprintf("http %d", status)
	}
	code := strings.TrimSpace(coerce.AsString(errMap["code"]))
	if code == "" {
		return message
	}
	return code + ": " + message
}

// secureCompare compares two strings in constant time to prevent timing attacks.
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
