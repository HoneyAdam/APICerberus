package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/audit"
	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/federation"
	grpcpkg "github.com/APICerberus/APICerebrus/internal/grpc"
	"github.com/APICerberus/APICerebrus/internal/metrics"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
	"github.com/APICerberus/APICerebrus/internal/plugin"
	"github.com/APICerberus/APICerebrus/internal/store"
	"github.com/APICerberus/APICerebrus/internal/tracing"
)

// Gateway is the HTTP entrypoint that routes, balances and proxies requests.
type Gateway struct {
	mu             sync.RWMutex
	config         *config.Config
	router         *Router
	proxy          *Proxy
	health         *Checker
	store          *store.Store
	billing        *billing.Engine
	auditLogger    *audit.Logger
	auditRetention *audit.RetentionScheduler
	analytics      *analytics.Engine
	upstreams      map[string]*UpstreamPool
	consumers      []config.Consumer
	authAPIKey     *plugin.AuthAPIKey
	authBackoff    *plugin.AuthBackoff
	authRequired   bool
	routePipelines map[string][]plugin.PipelinePlugin
	routeHasAuth   map[string]bool
	httpServer     *http.Server
	httpListener   net.Listener
	httpsServer    *http.Server
	tlsManager     *TLSManager
	grpcServer     *grpcpkg.H2CServer // gRPC h2c server
	startedAt      time.Time

	// GraphQL Federation
	federationEnabled  bool
	subgraphs          *federation.SubgraphManager
	federationComposer *federation.Composer
	federationPlanner  *federation.Planner
	federationExecutor *federation.Executor

	// OpenTelemetry Tracing
	tracer          *tracing.Tracer
	traceMiddleware *tracing.Middleware

	runCtx       context.Context
	healthCancel context.CancelFunc
	auditCancel  context.CancelFunc
	auditDone    chan struct{} // closed when audit goroutine finishes
}

// New initializes all gateway subsystems from config.
func New(cfg *config.Config) (*Gateway, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	router, err := NewRouter(cfg.Routes, cfg.Services)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	upstreamPools := buildUpstreamPools(cfg.Upstreams)
	checker := NewChecker(cfg.Upstreams, upstreamPools)
	consumers := append([]config.Consumer(nil), cfg.Consumers...)
	st, err := openGatewayStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("open gateway store: %w", err)
	}
	apiKeyLookup := buildStoreAPIKeyLookup(st)
	permissionLookup := buildEndpointPermissionLookup(st)
	billingEngine := billing.NewEngine(st, cfg.Billing)
	auditLogger := newAuditLogger(st, cfg)
	auditRetention := newAuditRetention(st, cfg)
	authBackoff := plugin.NewAuthBackoff()
	authAPIKey := newAuthAPIKey(cfg, consumers, apiKeyLookup, authBackoff)
	routePipelines, routeHasAuth, err := plugin.BuildRoutePipelinesWithContext(cfg, plugin.BuilderContext{
		Consumers:        consumers,
		APIKeyLookup:     apiKeyLookup,
		PermissionLookup: permissionLookup,
	})
	if err != nil {
		if st != nil {
			_ = st.Close()
		}
		return nil, fmt.Errorf("build plugin pipelines: %w", err)
	}

	// Initialize OpenTelemetry tracer
	tracer, err := tracing.New(cfg.Tracing)
	if err != nil {
		if st != nil {
			_ = st.Close()
		}
		return nil, fmt.Errorf("initialize tracer: %w", err)
	}

	// Apply deny_private_upstreams SSRF protection before building upstreams.
	SetDenyPrivateUpstreams(cfg.Gateway.DenyPrivateUpstreams)

	g := &Gateway{
		config:         cfg,
		router:         router,
		proxy:          NewProxy(cfg.Gateway.ConnectionPool),
		health:         checker,
		store:          st,
		billing:        billingEngine,
		auditLogger:    auditLogger,
		auditRetention: auditRetention,
		analytics:      analytics.NewEngine(analytics.EngineConfig{}),
		upstreams:      upstreamPools,
		consumers:      consumers,
		authAPIKey:     authAPIKey,
		authBackoff:    authBackoff,
		authRequired:   len(consumers) > 0,
		routePipelines: routePipelines,
		routeHasAuth:   routeHasAuth,
		tracer:         tracer,
		startedAt:      time.Now(),
	}

	// Initialize tracing middleware if enabled
	if tracer.Enabled() {
		g.traceMiddleware = tracing.NewMiddleware(tracer)
	}
	if cfg.Federation.Enabled {
		g.federationEnabled = true
		g.subgraphs = federation.NewSubgraphManager()
		g.federationComposer = federation.NewComposer()
		g.federationExecutor = federation.NewExecutor()
		// Planner is created after schema composition when subgraphs are registered.
	}
	if strings.TrimSpace(cfg.Gateway.HTTPAddr) != "" {
		g.httpServer = g.newHTTPServer(cfg.Gateway.HTTPAddr)
	}
	if strings.TrimSpace(cfg.Gateway.HTTPSAddr) != "" {
		tlsManager, tlsErr := NewTLSManager(cfg.Gateway.TLS)
		if tlsErr != nil {
			if st != nil {
				_ = st.Close()
			}
			return nil, fmt.Errorf("initialize tls manager: %w", tlsErr)
		}
		g.tlsManager = tlsManager
		g.httpsServer = g.newHTTPServer(cfg.Gateway.HTTPSAddr)
	}
	return g, nil
}

func (g *Gateway) newHTTPServer(addr string) *http.Server {
	gwCfg := g.config.Gateway

	// Apply tracing middleware if enabled
	var handler http.Handler = g
	if g.traceMiddleware != nil {
		handler = g.traceMiddleware.Wrap(g)
	}

	server := &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    gwCfg.ReadTimeout,
		WriteTimeout:   gwCfg.WriteTimeout,
		IdleTimeout:    gwCfg.IdleTimeout,
		MaxHeaderBytes: gwCfg.MaxHeaderBytes,
	}
	return server
}

// ServeHTTP performs route match, target selection and upstream proxying.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	router := g.router
	upstreamPools := g.upstreams
	checker := g.health
	billingEngine := g.billing
	auditLogger := g.auditLogger
	analyticsEngine := g.analytics
	authAPIKey := g.authAPIKey
	authRequired := g.authRequired
	routePipelines := g.routePipelines
	routeHasAuth := g.routeHasAuth
	fedEnabled := g.federationEnabled
	g.mu.RUnlock()

	if analyticsEngine != nil {
		analyticsEngine.IncActiveConns()
		defer analyticsEngine.DecActiveConns()
	}

	// Enforce MaxBodyBytes: check Content-Length first (fast path, no buffering).
	maxBody := g.config.Gateway.MaxBodyBytes
	if maxBody > 0 && r.Body != nil {
		if r.ContentLength > maxBody {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if r.ContentLength < 0 {
			body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusInternalServerError)
				return
			}
			if int64(len(body)) > maxBody {
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
	}

	// Add security headers to all responses
	addSecurityHeaders(w, g.config.Gateway.HTTPSAddr != "")

	// Built-in health and readiness endpoints (bypass routing).
	if g.handleHealth(w, r) {
		return
	}

	// GraphQL Federation batch endpoint (bypass routing).
	if g.federationEnabled && r.URL.Path == "/graphql/batch" {
		g.serveFederationBatch(w, r)
		return
	}

	// Setup request state with response capture for audit/analytics.
	rs := newRequestState()
	w, rs.responseWriter = newResponseCaptureWriter(w, auditLogger, analyticsEngine)
	rs.auditWriter = rs.responseWriter
	rs.requestBodySnapshot = captureRequestBody(r, auditLogger)

	// Defer audit logging and analytics recording.
	defer func() {
		logRequestAudit(auditLogger, r, rs)
		recordAnalytics(analyticsEngine, r, rs)
	}()
	defer rs.runPipelineCleanup()

	// Route matching.
	var err error
	rs.route, rs.service, err = router.Match(r)
	if err != nil {
		rs.markBlocked("route_not_found")
		g.writeError(rs.responseWriter, http.StatusNotFound, "route_not_found", "No matching route")
		return
	}

	// Plugin pipeline (PRE_AUTH → AUTH → PRE_PROXY).
	routeKey := rs.routePipelineKey()
	chain := routePipelines[routeKey]
	rs.pipeline = plugin.NewPipeline(chain)
	rs.pipelineCtx = &plugin.PipelineContext{
		Request:        r,
		ResponseWriter: w,
		Route:          rs.route,
		Service:        rs.service,
		Consumer:       rs.consumer,
		Metadata:       map[string]any{},
	}
	handled, err := rs.pipeline.Execute(rs.pipelineCtx)
	if err != nil {
		rs.markBlocked("plugin_error")
		g.writePluginError(rs.responseWriter, err)
		return
	}
	rs.consumer = rs.pipelineCtx.Consumer
	if rs.pipelineCtx.Request != nil {
		r = rs.pipelineCtx.Request
	}
	if handled || rs.pipelineCtx.Aborted {
		if rs.pipelineCtx.Aborted {
			reason := strings.TrimSpace(rs.pipelineCtx.AbortReason)
			if reason != "" && !strings.Contains(reason, ": handled response") {
				rs.markBlocked(reason)
			}
		}
		rs.writeResponseConsumer(r)
		return
	}

	// Auth chain.
	if g.executeAuthChain(r, rs, authRequired, authAPIKey, routePipelines, routeHasAuth) {
		return
	}

	// Billing pre-proxy.
	if g.executeBillingPreProxy(r, rs, billingEngine, rs.route, rs.consumer) {
		return
	}

	// GraphQL Federation: route through the federation executor.
	if rs.service.Protocol == "graphql" && fedEnabled {
		g.serveFederation(w, r)
		return
	}

	// Proxy chain (upstream selection, forwarding, retry, post-proxy).
	g.executeProxyChain(r, rs, upstreamPools, checker, billingEngine)
}

func (g *Gateway) serveHTTPS(server *http.Server, tlsManager *TLSManager) error {
	if server == nil {
		return errors.New("https server is nil")
	}
	if tlsManager == nil {
		return errors.New("tls manager is nil")
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	tlsConfig := tlsManager.TLSConfig()
	tlsListener := tls.NewListener(listener, tlsConfig)
	return server.Serve(tlsListener)
}

// Start starts the gateway listener and gracefully shuts it down when context is cancelled.
func (g *Gateway) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	g.mu.Lock()
	g.runCtx = ctx
	healthCtx, healthCancel := context.WithCancel(ctx)
	g.healthCancel = healthCancel
	auditCtx, auditCancel := context.WithCancel(ctx)
	g.auditCancel = auditCancel
	server := g.httpServer
	httpsServer := g.httpsServer
	tlsManager := g.tlsManager
	checker := g.health
	auditLogger := g.auditLogger
	auditRetention := g.auditRetention
	g.mu.Unlock()

	checker.Start(healthCtx)
	if auditLogger != nil {
		g.mu.Lock()
		g.auditDone = make(chan struct{})
		g.mu.Unlock()
		go func() {
			auditLogger.Start(auditCtx)
			g.mu.Lock()
			close(g.auditDone)
			g.mu.Unlock()
		}()
	}
	if auditRetention != nil {
		go auditRetention.Start(auditCtx)
	}

	go func() { // #nosec G118 -- goroutine waits on the request-scoped ctx captured in closure.
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = g.Shutdown(shutdownCtx)
	}()

	serverCount := 0
	if server != nil {
		serverCount++
	}
	if httpsServer != nil {
		serverCount++
	}
	if serverCount == 0 {
		return errors.New("no gateway listeners configured")
	}

	errCh := make(chan error, serverCount)

	// Start gRPC server if enabled
	if g.config.Gateway.GRPC.Enabled && g.config.Gateway.GRPC.Addr != "" {
		grpcConfig := &grpcpkg.H2CConfig{
			Addr:                 g.config.Gateway.GRPC.Addr,
			ReadTimeout:          g.config.Gateway.ReadTimeout,
			WriteTimeout:         g.config.Gateway.WriteTimeout,
			IdleTimeout:          g.config.Gateway.IdleTimeout,
			MaxHeaderBytes:       g.config.Gateway.MaxHeaderBytes,
			MaxConcurrentStreams: 250,
		}
		g.grpcServer = grpcpkg.NewH2CServer(grpcConfig, g)
		if err := g.grpcServer.Start(); err != nil {
			return fmt.Errorf("start gRPC server: %w", err)
		}
	}

	if server != nil {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", server.Addr, err)
		}
		g.httpListener = listener
		go func() {
			err := server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errCh <- err
		}()
	}
	if httpsServer != nil {
		go func() {
			err := g.serveHTTPS(httpsServer, tlsManager)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errCh <- err
		}()
	}

	for i := 0; i < serverCount; i++ {
		err := <-errCh
		if err != nil {
			return err
		}
	}
	return nil
}

// Reload swaps config and routing state while the gateway is running.
func (g *Gateway) Reload(newCfg *config.Config) error {
	if newCfg == nil {
		return errors.New("new config is nil")
	}

	newRouter, err := NewRouter(newCfg.Routes, newCfg.Services)
	if err != nil {
		return fmt.Errorf("rebuild router: %w", err)
	}
	newPools := buildUpstreamPools(newCfg.Upstreams)
	newChecker := NewChecker(newCfg.Upstreams, newPools)
	newConsumers := append([]config.Consumer(nil), newCfg.Consumers...)
	newStore, err := openGatewayStore(newCfg)
	if err != nil {
		return fmt.Errorf("open gateway store: %w", err)
	}
	newAPIKeyLookup := buildStoreAPIKeyLookup(newStore)
	newPermissionLookup := buildEndpointPermissionLookup(newStore)
	newBillingEngine := billing.NewEngine(newStore, newCfg.Billing)
	newAuditLogger := newAuditLogger(newStore, newCfg)
	newAuditRetention := newAuditRetention(newStore, newCfg)
	newAuthAPIKey := newAuthAPIKey(newCfg, newConsumers, newAPIKeyLookup, g.authBackoff)
	newRoutePipelines, newRouteHasAuth, err := plugin.BuildRoutePipelinesWithContext(newCfg, plugin.BuilderContext{
		Consumers:        newConsumers,
		APIKeyLookup:     newAPIKeyLookup,
		PermissionLookup: newPermissionLookup,
	})
	if err != nil {
		if newStore != nil {
			_ = newStore.Close()
		}
		return fmt.Errorf("build plugin pipelines: %w", err)
	}
	var newTLSManager *TLSManager
	if strings.TrimSpace(newCfg.Gateway.HTTPSAddr) != "" {
		newTLSManager, err = NewTLSManager(newCfg.Gateway.TLS)
		if err != nil {
			if newStore != nil {
				_ = newStore.Close()
			}
			return fmt.Errorf("initialize tls manager: %w", err)
		}
	}

	g.mu.Lock()
	oldStore := g.store

	g.config = newCfg
	g.router = newRouter
	g.upstreams = newPools
	g.health = newChecker
	g.store = newStore
	g.billing = newBillingEngine
	g.auditLogger = newAuditLogger
	g.auditRetention = newAuditRetention
	g.consumers = newConsumers
	g.authAPIKey = newAuthAPIKey
	g.authRequired = len(newConsumers) > 0
	g.routePipelines = newRoutePipelines
	g.routeHasAuth = newRouteHasAuth
	g.tlsManager = newTLSManager

	if g.httpServer != nil {
		g.httpServer.ReadTimeout = newCfg.Gateway.ReadTimeout
		g.httpServer.WriteTimeout = newCfg.Gateway.WriteTimeout
		g.httpServer.IdleTimeout = newCfg.Gateway.IdleTimeout
		g.httpServer.MaxHeaderBytes = newCfg.Gateway.MaxHeaderBytes
	}
	if g.httpsServer != nil {
		g.httpsServer.ReadTimeout = newCfg.Gateway.ReadTimeout
		g.httpsServer.WriteTimeout = newCfg.Gateway.WriteTimeout
		g.httpsServer.IdleTimeout = newCfg.Gateway.IdleTimeout
		g.httpsServer.MaxHeaderBytes = newCfg.Gateway.MaxHeaderBytes
	}

	if g.healthCancel != nil {
		g.healthCancel()
		base := g.runCtx
		if base == nil {
			base = context.Background()
		}
		healthCtx, cancel := context.WithCancel(base)
		g.healthCancel = cancel
		g.health.Start(healthCtx)
	}
	if g.auditCancel != nil {
		g.auditCancel()
		oldAuditDone := g.auditDone
		// Release the lock so the old audit goroutine can acquire it to
		// close the channel. The non-audit field updates have already been
		// applied above under the same lock.

		g.mu.Unlock()
		// Wait for the old audit goroutine to finish before reusing the field.
		if oldAuditDone != nil {
			select {
			case <-oldAuditDone:
			case <-time.After(10 * time.Second):
			}
		}

		g.mu.Lock()
		base := g.runCtx
		if base == nil {
			base = context.Background()
		}
		auditCtx, cancel := context.WithCancel(base)
		g.auditCancel = cancel
		g.auditDone = make(chan struct{})
		newAuditLoggerRef := g.auditLogger
		go func() {
			newAuditLoggerRef.Start(auditCtx)
			g.mu.Lock()
			close(g.auditDone)
			g.mu.Unlock()
		}()
		if g.auditRetention != nil {
			go g.auditRetention.Start(auditCtx)
		}
	}
	g.mu.Unlock()

	if oldStore != nil {
		_ = oldStore.Close()
	}
	return nil
}

// Addr returns the HTTP server address
func (g *Gateway) Addr() string {
	g.mu.RLock()
	listener := g.httpListener
	g.mu.RUnlock()

	if listener != nil {
		return listener.Addr().String()
	}
	return ""
}

// Shutdown gracefully drains active connections and stops background health loops.
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.mu.RLock()
	healthCancel := g.healthCancel
	auditCancel := g.auditCancel
	auditDone := g.auditDone
	server := g.httpServer
	httpsServer := g.httpsServer
	grpcServer := g.grpcServer
	st := g.store
	analyticsEng := g.analytics
	g.mu.RUnlock()

	if healthCancel != nil {
		healthCancel()
	}
	if auditCancel != nil {
		auditCancel()
	}
	var shutdownErr error
	if server != nil {
		if err := server.Shutdown(ctx); err != nil {
			shutdownErr = err
		}
	}
	if httpsServer != nil {
		if err := httpsServer.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if grpcServer != nil {
		if err := grpcServer.Stop(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}

	// Wait for audit goroutine to finish draining and flushing.
	if auditCancel != nil && auditDone != nil {
		select {
		case <-auditDone:
		case <-ctx.Done():
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("audit drain timeout: %w", ctx.Err()))
		}
	}

	if st != nil {
		if err := st.Close(); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close store: %w", err))
		}
	}

	// Shutdown tracer (flushes pending spans)
	if g.tracer != nil {
		if err := g.tracer.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown tracer: %w", err))
		}
	}

	// Flush analytics (drain ring buffer into final time-series snapshot)
	if analyticsEng != nil {
		analyticsEng.Shutdown(ctx)
	}

	return shutdownErr
}

type errorResponse struct {
	Error gatewayError `json:"error"`
}

type gatewayError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type billingRequestState struct {
	result    *billing.PreCheckResult
	routeID   string
	requestID string
	applied   bool
}

func (g *Gateway) writeError(w http.ResponseWriter, status int, code, message string) {
	resp := errorResponse{
		Error: gatewayError{
			Code:    code,
			Message: message,
		},
	}
	_ = jsonutil.WriteJSON(w, status, resp)
}

// writeErrorRoute writes an error using the route's configured error format
// (HTML if route or global html_errors is enabled, otherwise JSON).
func (g *Gateway) writeErrorRoute(w http.ResponseWriter, status int, code, message string, route *config.Route) {
	htmlEnabled := g.config.Gateway.HTMLErrors || (route != nil && route.HTMLErrors)
	if htmlEnabled {
		htmlErrorPage(w, status, code, message)
		return
	}
	g.writeError(w, status, code, message)
}

func (g *Gateway) writeAuthError(w http.ResponseWriter, err error) {
	var pe *plugin.PluginError
	if errors.As(err, &pe) {
		g.writeError(w, pe.Status, pe.Code, pe.Message)
		return
	}
	g.writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
}

func (g *Gateway) writePluginError(w http.ResponseWriter, err error) {
	var pe *plugin.PluginError
	if errors.As(err, &pe) {
		g.writeError(w, pe.Status, pe.Code, pe.Message)
		return
	}
	g.writeError(w, http.StatusBadRequest, "plugin_error", "plugin processing error")
}

func (g *Gateway) writeBillingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInsufficientCredits):
		g.writeError(w, http.StatusPaymentRequired, "insufficient_credits", "Your credit balance is exhausted")
		return
	case errors.Is(err, sql.ErrNoRows):
		g.writeError(w, http.StatusUnauthorized, "invalid_consumer", "Authenticated consumer is not a valid user")
		return
	}
	g.writeError(w, http.StatusInternalServerError, "billing_error", "Billing check failed")
}

func newAuthAPIKey(cfg *config.Config, consumers []config.Consumer, lookup plugin.APIKeyLookupFunc, backoff *plugin.AuthBackoff) *plugin.AuthAPIKey {
	if cfg == nil {
		return plugin.NewAuthAPIKey(consumers, plugin.AuthAPIKeyOptions{
			Lookup:  lookup,
			Backoff: backoff,
		})
	}
	return plugin.NewAuthAPIKey(consumers, plugin.AuthAPIKeyOptions{
		KeyNames:    append([]string(nil), cfg.Auth.APIKey.KeyNames...),
		QueryNames:  append([]string(nil), cfg.Auth.APIKey.QueryNames...),
		CookieNames: append([]string(nil), cfg.Auth.APIKey.CookieNames...),
		Lookup:      lookup,
		Backoff:     backoff,
	})
}

func newAuditLogger(st *store.Store, cfg *config.Config) *audit.Logger {
	if st == nil || cfg == nil {
		return nil
	}
	return audit.NewLogger(st.Audits(), cfg.Audit, nil)
}

func newAuditRetention(st *store.Store, cfg *config.Config) *audit.RetentionScheduler {
	if st == nil || cfg == nil {
		return nil
	}
	return audit.NewRetentionScheduler(st.Audits(), cfg.Audit)
}

func openGatewayStore(cfg *config.Config) (*store.Store, error) {
	if cfg == nil || strings.TrimSpace(cfg.Store.Path) == "" {
		return nil, nil
	}
	return store.Open(cfg)
}

func buildStoreAPIKeyLookup(st *store.Store) plugin.APIKeyLookupFunc {
	if st == nil {
		return nil
	}
	repo := st.APIKeys()
	if repo == nil {
		return nil
	}

	return func(rawKey string, req *http.Request) (*config.Consumer, error) {
		user, key, err := repo.ResolveUserByRawKey(rawKey)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrAPIKeyNotFound):
				return nil, plugin.ErrInvalidAPIKey
			case errors.Is(err, store.ErrAPIKeyExpired):
				return nil, plugin.ErrExpiredAPIKey
			case errors.Is(err, store.ErrAPIKeyRevoked), errors.Is(err, store.ErrAPIKeyUserDown):
				return nil, plugin.ErrInvalidAPIKey
			default:
				return nil, &plugin.AuthError{
					PluginError: plugin.PluginError{
						Code:    "auth_backend_error",
						Message: "API key authentication backend unavailable",
						Status:  http.StatusInternalServerError,
					},
				}
			}
		}
		if user == nil || key == nil {
			return nil, plugin.ErrInvalidAPIKey
		}
		repo.UpdateLastUsed(req.Context(), key.ID, authLookupIP(req))
		return userToConsumer(user), nil
	}
}

func buildEndpointPermissionLookup(st *store.Store) plugin.EndpointPermissionLookupFunc {
	if st == nil {
		return nil
	}
	repo := st.Permissions()
	if repo == nil {
		return nil
	}
	return func(userID, routeID string) (*plugin.EndpointPermissionRecord, error) {
		permission, err := repo.FindByUserAndRoute(userID, routeID)
		if err != nil {
			return nil, err
		}
		if permission == nil {
			return nil, nil
		}
		return &plugin.EndpointPermissionRecord{
			ID:           permission.ID,
			UserID:       permission.UserID,
			RouteID:      permission.RouteID,
			Methods:      append([]string(nil), permission.Methods...),
			Allowed:      permission.Allowed,
			RateLimits:   config.CloneAnyMap(permission.RateLimits),
			CreditCost:   permission.CreditCost,
			ValidFrom:    permission.ValidFrom,
			ValidUntil:   permission.ValidUntil,
			AllowedDays:  append([]int(nil), permission.AllowedDays...),
			AllowedHours: append([]string(nil), permission.AllowedHours...),
		}, nil
	}
}

func authLookupIP(req *http.Request) string {
	if req == nil {
		return ""
	}
	return netutil.ExtractClientIP(req)
}

func userToConsumer(user *store.User) *config.Consumer {
	if user == nil {
		return nil
	}
	metadata := map[string]any{
		"email":   user.Email,
		"company": user.Company,
		"role":    user.Role,
		"status":  user.Status,
	}
	maps.Copy(metadata, user.Metadata)

	// Map user.RateLimits to Consumer.RateLimit struct.
	// User.RateLimits is a JSON map that may contain "requests_per_second"
	// and "burst" keys (matching the plugin config schema).
	var rateLimit config.ConsumerRateLimit
	if len(user.RateLimits) > 0 {
		metadata["rate_limits"] = config.CloneAnyMap(user.RateLimits)
		if v, ok := user.RateLimits["requests_per_second"]; ok {
			switch n := v.(type) {
			case float64:
				rateLimit.RequestsPerSecond = int(n)
			case int:
				rateLimit.RequestsPerSecond = n
			}
		}
		if v, ok := user.RateLimits["burst"]; ok {
			switch n := v.(type) {
			case float64:
				rateLimit.Burst = int(n)
			case int:
				rateLimit.Burst = n
			}
		}
	}

	// Extract ACL groups from user metadata if present.
	var aclGroups []string
	if aclRaw, ok := user.Metadata["acl_groups"]; ok {
		switch v := aclRaw.(type) {
		case []string:
			aclGroups = v
		case []any:
			aclGroups = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					aclGroups = append(aclGroups, s)
				}
			}
		}
	}

	if len(user.IPWhitelist) > 0 {
		whitelist := make([]string, 0, len(user.IPWhitelist))
		for _, value := range user.IPWhitelist {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			whitelist = append(whitelist, value)
		}
		if len(whitelist) > 0 {
			metadata["ip_whitelist"] = whitelist
		}
	}
	if user.CreditBalance > 0 {
		metadata["credit_balance"] = user.CreditBalance
	}
	return &config.Consumer{
		ID:        user.ID,
		Name:      user.Name,
		RateLimit: rateLimit,
		ACLGroups: aclGroups,
		Metadata:  metadata,
	}
}

func buildUpstreamPools(upstreams []config.Upstream) map[string]*UpstreamPool {
	out := make(map[string]*UpstreamPool, len(upstreams)*2)
	for _, up := range upstreams {
		pool := NewUpstreamPool(up)
		if strings.TrimSpace(up.Name) != "" {
			out[up.Name] = pool
		}
		if strings.TrimSpace(up.ID) != "" {
			out[up.ID] = pool
		}
	}
	return out
}

func findUpstreamPool(pools map[string]*UpstreamPool, nameOrID string) *UpstreamPool {
	if pools == nil {
		return nil
	}
	return pools[nameOrID]
}

// Uptime returns gateway running duration since construction.
func (g *Gateway) Uptime() time.Duration {
	g.mu.RLock()
	started := g.startedAt
	g.mu.RUnlock()
	return time.Since(started)
}

// Analytics returns the runtime analytics engine.
func (g *Gateway) Analytics() *analytics.Engine {
	g.mu.RLock()
	engine := g.analytics
	g.mu.RUnlock()
	return engine
}

// Store returns the store instance.
func (g *Gateway) Store() *store.Store {
	g.mu.RLock()
	st := g.store
	g.mu.RUnlock()
	return st
}

// UpstreamHealth returns current target health by target ID for the given upstream name.
func (g *Gateway) UpstreamHealth(upstreamName string) map[string]bool {
	g.mu.RLock()
	checker := g.health
	g.mu.RUnlock()

	snapshot := checker.Snapshot(upstreamName)
	out := make(map[string]bool, len(snapshot))
	for targetID, state := range snapshot {
		out[targetID] = state.Healthy
	}
	return out
}

// Subgraphs returns the federation SubgraphManager (nil when federation is disabled).
func (g *Gateway) Subgraphs() *federation.SubgraphManager {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.subgraphs
}

// handleHealth serves built-in /health and /ready endpoints.
// M-004 NOTE: These endpoints bypass the plugin pipeline and cannot be rate-limited
// by the standard rate limiting plugins. They also skip authentication.
// Network-level protection (firewall, load balancer rate limiting) should be used
// in front of APICerebrus to protect these endpoints from DoS attacks.
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/health":
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"uptime": g.Uptime().String(),
		})
		return true
	case "/ready":
		// Gateway is ready when it has been started and all subsystems initialized.
		// We check that the store is accessible (ping SQLite) and the health checker is running.
		var ready = true
		var reasons []string
		g.mu.RLock()
		st := g.store
		checker := g.health
		g.mu.RUnlock()

		if st != nil {
			if err := st.DB().Ping(); err != nil {
				ready = false
				reasons = append(reasons, "database: "+err.Error())
			}
		}
		if checker == nil {
			ready = false
			reasons = append(reasons, "health checker: not initialized")
		}

		status := "ok"
		code := http.StatusOK
		if !ready {
			status = "not ready"
			code = http.StatusServiceUnavailable
		}
		resp := map[string]any{"status": status}
		if len(reasons) > 0 {
			resp["reasons"] = reasons
		}
		_ = jsonutil.WriteJSON(w, code, resp)
		return true
	case "/health/audit-drops":
		g.mu.RLock()
		logger := g.auditLogger
		g.mu.RUnlock()

		var dropped int64
		if logger != nil {
			dropped = logger.Dropped()
		}
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"dropped_entries": dropped,
			"audit_enabled":   logger != nil,
		})
		return true
	case "/metrics":
		metrics.DefaultRegistry.PrometheusHandler().ServeHTTP(w, r)
		return true
	default:
		return false
	}
}

// FederationComposer returns the federation Composer (nil when federation is disabled).
func (g *Gateway) FederationComposer() *federation.Composer {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.federationComposer
}

// FederationEnabled reports whether GraphQL Federation is active.
func (g *Gateway) FederationEnabled() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.federationEnabled
}

// serveFederation handles a GraphQL request through the federation executor.
func (g *Gateway) serveFederation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Federation endpoint requires POST")
		return
	}

	var gqlReq struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}

	if err := jsonutil.ReadJSON(r, &gqlReq, 1<<20); err != nil {
		g.writeError(w, http.StatusBadRequest, "invalid_graphql_request", err.Error())
		return
	}
	if strings.TrimSpace(gqlReq.Query) == "" {
		g.writeError(w, http.StatusBadRequest, "invalid_graphql_request", "query is required")
		return
	}

	g.mu.RLock()
	planner := g.federationPlanner
	executor := g.federationExecutor
	g.mu.RUnlock()

	if planner == nil || executor == nil {
		g.writeError(w, http.StatusServiceUnavailable, "federation_not_ready", "Schema has not been composed yet")
		return
	}

	plan, err := planner.Plan(gqlReq.Query, gqlReq.Variables)
	if err != nil {
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"errors": []map[string]string{{"message": fmt.Sprintf("query planning failed: %v", err)}},
		})
		return
	}

	result, err := executor.Execute(r.Context(), plan)
	if err != nil {
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"errors": []map[string]string{{"message": fmt.Sprintf("execution failed: %v", err)}},
		})
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

// batchGraphQLRequest represents a single request within a batch.
type batchGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// serveFederationBatch handles batched GraphQL requests through the federation executor.
// Accepts an array of GraphQL requests and returns an array of results, executing
// each query in parallel.
func (g *Gateway) serveFederationBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Batch endpoint requires POST")
		return
	}

	var batch []batchGraphQLRequest
	if err := jsonutil.ReadJSON(r, &batch, 1<<22); err != nil {
		g.writeError(w, http.StatusBadRequest, "invalid_batch_request", err.Error())
		return
	}
	// M-012: Limit batch size to prevent resource exhaustion via large batch submissions.
	const maxBatchSize = 100
	if len(batch) > maxBatchSize {
		g.writeError(w, http.StatusBadRequest, "batch_too_large",
			fmt.Sprintf("batch size %d exceeds maximum of %d", len(batch), maxBatchSize))
		return
	}
	if len(batch) == 0 {
		g.writeError(w, http.StatusBadRequest, "empty_batch", "Batch must contain at least one request")
		return
	}

	g.mu.RLock()
	planner := g.federationPlanner
	executor := g.federationExecutor
	g.mu.RUnlock()

	if planner == nil || executor == nil {
		g.writeError(w, http.StatusServiceUnavailable, "federation_not_ready", "Schema has not been composed yet")
		return
	}

	// Execute all queries in parallel.
	results := make([]any, len(batch))
	var wg sync.WaitGroup
	for i, req := range batch {
		wg.Add(1)
		go func(idx int, gqlReq batchGraphQLRequest) {
			defer wg.Done()

			if strings.TrimSpace(gqlReq.Query) == "" {
				results[idx] = map[string]any{
					"errors": []map[string]string{{"message": "query is required"}},
				}
				return
			}

			plan, err := planner.Plan(gqlReq.Query, gqlReq.Variables)
			if err != nil {
				results[idx] = map[string]any{
					"errors": []map[string]string{{"message": fmt.Sprintf("query planning failed: %v", err)}},
				}
				return
			}

			result, err := executor.Execute(r.Context(), plan)
			if err != nil {
				results[idx] = map[string]any{
					"errors": []map[string]string{{"message": fmt.Sprintf("execution failed: %v", err)}},
				}
				return
			}

			results[idx] = result
		}(i, req)
	}
	wg.Wait()

	_ = jsonutil.WriteJSON(w, http.StatusOK, results)
}

// RebuildFederationPlanner rebuilds the federation planner from the current
// subgraphs and composed schema. Called after schema composition.
func (g *Gateway) RebuildFederationPlanner() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.subgraphs == nil || g.federationComposer == nil {
		return
	}
	subgraphs := g.subgraphs.ListSubgraphs()
	entities := g.federationComposer.GetEntities()
	g.federationPlanner = federation.NewPlanner(subgraphs, entities)
}

// addSecurityHeaders adds essential security headers to all responses
func addSecurityHeaders(w http.ResponseWriter, isHTTPS bool) {
	// Prevent MIME type sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Prevent clickjacking
	w.Header().Set("X-Frame-Options", "DENY")
	// Enable XSS protection in browsers
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	// Control referrer information
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// Content Security Policy
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	// Permissions Policy
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

	// HSTS for HTTPS connections
	if isHTTPS {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	}
}
