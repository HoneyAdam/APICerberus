package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/audit"
	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/federation"
	grpcpkg "github.com/APICerberus/APICerebrus/internal/grpc"
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
	authAPIKey := newAuthAPIKey(cfg, consumers, nil, authBackoff)
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

	g := &Gateway{
		config:         cfg,
		router:         router,
		proxy:          NewProxy(),
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
	// For chunked bodies (ContentLength == -1), read up to limit+1 and reject if over.
	maxBody := g.config.Gateway.MaxBodyBytes
	if maxBody > 0 && r.Body != nil {
		if r.ContentLength > maxBody {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if r.ContentLength < 0 {
			// Chunked transfer: must read to enforce limit.
			// Buffer the body (up to limit+1) to prevent unbounded memory growth.
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

	requestStartedAt := time.Now()
	var (
		route               *config.Route
		service             *config.Service
		consumer            *config.Consumer
		requestBodySnapshot []byte
		proxyErrForAudit    error
		blocked             bool
		blockReason         string
		auditWriter         *audit.ResponseCaptureWriter
		responseWriter      *audit.ResponseCaptureWriter
		pipelineCtx         *plugin.PipelineContext
	)
	if auditLogger != nil || analyticsEngine != nil {
		maxResponseBodyBytes := int64(0)
		if auditLogger != nil {
			maxResponseBodyBytes = auditLogger.MaxResponseBodyBytes()
		}
		responseWriter = audit.NewResponseCaptureWriter(w, maxResponseBodyBytes)
		w = responseWriter
	}
	if auditLogger != nil {
		if body, captureErr := audit.CaptureRequestBody(r, auditLogger.MaxRequestBodyBytes()); captureErr == nil {
			requestBodySnapshot = body
		}
		auditWriter = responseWriter
	}
	defer func() {
		if auditLogger != nil && auditWriter != nil {
			auditLogger.Log(audit.LogInput{
				Request:        r,
				ResponseWriter: auditWriter,
				Route:          route,
				Service:        service,
				Consumer:       consumer,
				RequestBody:    requestBodySnapshot,
				StartedAt:      requestStartedAt,
				Blocked:        blocked,
				BlockReason:    blockReason,
				ProxyErr:       proxyErrForAudit,
			})
		}

		if analyticsEngine == nil {
			return
		}

		statusCode := 0
		bytesOut := int64(0)
		if responseWriter != nil {
			statusCode = responseWriter.StatusCode()
			bytesOut = responseWriter.BytesWritten()
		}
		bytesIn := r.ContentLength
		if bytesIn < 0 {
			bytesIn = 0
		}
		if bytesIn == 0 && len(requestBodySnapshot) > 0 {
			bytesIn = int64(len(requestBodySnapshot))
		}

		routeID := ""
		routeName := ""
		serviceName := ""
		userID := ""
		method := ""
		path := ""
		if route != nil {
			routeID = strings.TrimSpace(route.ID)
			routeName = strings.TrimSpace(route.Name)
		}
		if service != nil {
			serviceName = strings.TrimSpace(service.Name)
		}
		if consumer != nil {
			userID = strings.TrimSpace(consumer.ID)
		}
		if r != nil {
			method = strings.TrimSpace(strings.ToUpper(r.Method))
			if r.URL != nil {
				path = strings.TrimSpace(r.URL.Path)
			}
		}
		creditsConsumed := metadataInt64(pipelineCtx, "credits_deducted")

		analyticsEngine.Record(analytics.RequestMetric{
			Timestamp:       requestStartedAt.UTC(),
			RouteID:         routeID,
			RouteName:       routeName,
			ServiceName:     serviceName,
			UserID:          userID,
			Method:          method,
			Path:            path,
			StatusCode:      statusCode,
			LatencyMS:       time.Since(requestStartedAt).Milliseconds(),
			BytesIn:         bytesIn,
			BytesOut:        bytesOut,
			CreditsConsumed: creditsConsumed,
			Blocked:         blocked,
			Error:           blocked || proxyErrForAudit != nil || statusCode >= http.StatusInternalServerError,
		})
	}()

	var err error
	route, service, err = router.Match(r)
	if err != nil {
		blocked = true
		blockReason = "route_not_found"
		g.writeError(w, http.StatusNotFound, "route_not_found", "No matching route")
		return
	}

	routeKey := routePipelineKey(route)
	chain := routePipelines[routeKey]
	pipeline := plugin.NewPipeline(chain)
	pipelineCtx = &plugin.PipelineContext{
		Request:        r,
		ResponseWriter: w,
		Route:          route,
		Service:        service,
		Consumer:       consumer,
		Metadata:       map[string]any{},
	}
	defer runPipelineCleanup(pipelineCtx)
	handled, err := pipeline.Execute(pipelineCtx)
	if err != nil {
		blocked = true
		blockReason = "plugin_error"
		g.writePluginError(w, err)
		return
	}
	consumer = pipelineCtx.Consumer
	if pipelineCtx.Request != nil {
		r = pipelineCtx.Request
	}
	if handled || pipelineCtx.Aborted {
		if pipelineCtx.Aborted {
			reason := strings.TrimSpace(pipelineCtx.AbortReason)
			if reason != "" && !strings.Contains(reason, ": handled response") {
				blocked = true
				blockReason = reason
			}
		}
		if consumer != nil {
			setRequestConsumer(r, consumer)
		}
		return
	}

	if authRequired && !routeHasAuth[routeKey] {
		if authAPIKey == nil {
			blocked = true
			blockReason = "auth_unavailable"
			g.writeError(w, http.StatusInternalServerError, "auth_unavailable", "Authentication module is unavailable")
			return
		}
		resolved, err := authAPIKey.Authenticate(r)
		if err != nil {
			blocked = true
			blockReason = "auth_failed"
			g.writeAuthError(w, err)
			return
		}
		consumer = resolved
	}
	if consumer != nil {
		setRequestConsumer(r, consumer)
	}
	billingState, err := applyBillingPreProxy(billingEngine, r, route, consumer, pipelineCtx)
	if err != nil {
		blocked = true
		blockReason = "billing_precheck_failed"
		g.writeBillingError(w, err)
		return
	}
	// GraphQL Federation: route through the federation executor when the
	// matched service uses protocol "graphql" and federation is enabled.
	if service.Protocol == "graphql" && fedEnabled {
		g.serveFederation(w, r)
		return
	}

	retryPolicy := pipelineCtx.Retry

	pool := findUpstreamPool(upstreamPools, service.Upstream)
	if pool == nil {
		blocked = true
		blockReason = "upstream_not_found"
		g.writeError(w, http.StatusBadGateway, "upstream_not_found", "Service upstream is not configured")
		return
	}

	maxAttempts := 1
	if retryPolicy != nil {
		maxAttempts = retryPolicy.MaxAttempts(r.Method)
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	runAfterProxy := func(proxyErr error) {
		pipelineCtx.Consumer = consumer
		pipeline.ExecutePostProxy(pipelineCtx, proxyErr)
		if capture, ok := pipelineCtx.ResponseWriter.(*plugin.TransformCaptureWriter); ok && capture.HasCaptured() && !capture.IsFlushed() {
			_ = capture.Flush()
		}
		if capture, ok := pipelineCtx.ResponseWriter.(*plugin.CaptureResponseWriter); ok && !capture.IsFlushed() {
			_ = capture.Flush()
		}
		consumer = pipelineCtx.Consumer
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		downstreamWriter := pipelineCtx.ResponseWriter
		if downstreamWriter == nil {
			downstreamWriter = w
		}

		target, err := pool.Next(&RequestContext{
			Request:        r,
			ResponseWriter: downstreamWriter,
			Route:          route,
			Consumer:       consumer,
		})
		if err != nil {
			if errors.Is(err, ErrNoHealthyTargets) {
				blocked = true
				blockReason = "no_healthy_target"
				g.writeError(w, http.StatusBadGateway, "no_healthy_target", "No healthy upstream target available")
				return
			}
			blocked = true
			blockReason = "target_selection_failed"
			g.writeError(w, http.StatusBadGateway, "target_selection_failed", "Failed to select upstream target")
			return
		}
		targetID := targetKey(*target)

		if retryPolicy == nil {
			pipelineCtx.Response = nil
			proxyErr := g.proxy.Forward(&RequestContext{
				Request:        r,
				ResponseWriter: downstreamWriter,
				Route:          route,
				Consumer:       consumer,
				UpstreamTimeout: service.ReadTimeout,
			}, target)

			runAfterProxy(proxyErr)

			pool.Done(targetID)
			if proxyErr != nil {
				blocked = true
				blockReason = "proxy_error"
				proxyErrForAudit = proxyErr
				if checker != nil {
					checker.ReportError(pool.Name(), targetID)
				}
				// Proxy already wrote status/body for transport failures.
				return
			}
			applyBillingPostProxy(billingEngine, billingState, pipelineCtx, nil)
			if checker != nil {
				checker.ReportSuccess(pool.Name(), targetID)
			}
			return
		}

		resp, proxyErr := g.proxy.Do(&RequestContext{
			Request:        r,
			ResponseWriter: downstreamWriter,
			Route:          route,
			Consumer:       consumer,
			UpstreamTimeout: service.ReadTimeout,
		}, target)
		pipelineCtx.Response = resp

		shouldRetry := false
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		if retryPolicy.ShouldRetry(r.Method, attempt, statusCode, proxyErr) {
			shouldRetry = true
		}

		if proxyErr != nil {
			if checker != nil {
				checker.ReportError(pool.Name(), targetID)
			}
			if shouldRetry {
				runAfterProxy(proxyErr)
				pool.Done(targetID)
				select {
				case <-r.Context().Done():
					return
				case <-time.After(retryPolicy.Backoff(attempt)):
				}
				continue
			}
			blocked = true
			blockReason = "proxy_error"
			proxyErrForAudit = proxyErr
			writeProxyError(downstreamWriter, proxyErrorStatus(proxyErr))
			runAfterProxy(proxyErr)
			pool.Done(targetID)
			return
		}

		if shouldRetry {
			if checker != nil {
				checker.ReportError(pool.Name(), targetID)
			}
			if resp != nil {
				_ = resp.Body.Close()
			}
			runAfterProxy(nil)
			pool.Done(targetID)
			time.Sleep(retryPolicy.Backoff(attempt))
			continue
		}

		if checker != nil {
			if statusCode >= 500 {
				checker.ReportError(pool.Name(), targetID)
			} else {
				checker.ReportSuccess(pool.Name(), targetID)
			}
		}
		var writeErr error
		if resp != nil {
			writeErr = g.proxy.WriteResponse(downstreamWriter, resp)
			_ = resp.Body.Close()
		}
		runAfterProxy(writeErr)
		applyBillingPostProxy(billingEngine, billingState, pipelineCtx, writeErr)
		pool.Done(targetID)
		if writeErr != nil {
			blocked = true
			blockReason = "response_write_error"
			proxyErrForAudit = writeErr
			return
		}
		return
	}

	blocked = true
	blockReason = "retries_exhausted"
	writeProxyError(w, http.StatusBadGateway)
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
		go auditLogger.Start(auditCtx)
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
	newAuthAPIKey := newAuthAPIKey(newCfg, newConsumers, nil, g.authBackoff)
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
		base := g.runCtx
		if base == nil {
			base = context.Background()
		}
		auditCtx, cancel := context.WithCancel(base)
		g.auditCancel = cancel
		if g.auditLogger != nil {
			go g.auditLogger.Start(auditCtx)
		}
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
	server := g.httpServer
	httpsServer := g.httpsServer
	grpcServer := g.grpcServer
	st := g.store
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
		if st != nil {
			if err := st.Close(); err != nil {
				shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close store: %w", err))
			}
		}

		// Shutdown tracer
		if g.tracer != nil {
			if err := g.tracer.Shutdown(ctx); err != nil {
				shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown tracer: %w", err))
			}
		}

		return shutdownErr
}

type errorResponse struct {
	Error gatewayError `json:"error"`
}

type gatewayError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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

func (g *Gateway) writeAuthError(w http.ResponseWriter, err error) {
	var authErr *plugin.AuthError
	if errors.As(err, &authErr) {
		g.writeError(w, authErr.Status, authErr.Code, authErr.Message)
		return
	}
	var jwtAuthErr *plugin.JWTAuthError
	if errors.As(err, &jwtAuthErr) {
		g.writeError(w, jwtAuthErr.Status, jwtAuthErr.Code, jwtAuthErr.Message)
		return
	}
	g.writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
}

func (g *Gateway) writePluginError(w http.ResponseWriter, err error) {
	switch e := err.(type) {
	case *plugin.AuthError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.JWTAuthError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.IPRestrictError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.CircuitBreakerError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.RequestSizeLimitError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.RequestValidatorError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.BotDetectError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.EndpointPermissionError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	case *plugin.UserIPWhitelistError:
		g.writeError(w, e.Status, e.Code, e.Message)
		return
	}
	g.writeError(w, http.StatusBadRequest, "plugin_error", err.Error())
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

func applyBillingPreProxy(engine *billing.Engine, req *http.Request, route *config.Route, consumer *config.Consumer, ctx *plugin.PipelineContext) (*billingRequestState, error) {
	if engine == nil || !engine.Enabled() || req == nil {
		return nil, nil
	}
	result, err := engine.PreCheck(billing.RequestMeta{
		Consumer:     consumer,
		Route:        route,
		Method:       req.Method,
		RawAPIKey:    extractAPIKey(req),
		CostOverride: extractPermissionCreditCost(ctx),
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	state := &billingRequestState{
		result:    result,
		routeID:   billingRouteID(route),
		requestID: billingRequestID(req, ctx),
	}
	if ctx != nil {
		if ctx.Metadata == nil {
			ctx.Metadata = map[string]any{}
		}
		if result.Cost > 0 {
			ctx.Metadata["credit_cost"] = result.Cost
		}
		if result.ZeroBalance {
			ctx.Metadata["zero_balance"] = true
		}
	}
	return state, nil
}

func applyBillingPostProxy(engine *billing.Engine, state *billingRequestState, ctx *plugin.PipelineContext, proxyErr error) {
	if engine == nil || !engine.Enabled() || state == nil || state.applied {
		return
	}
	if proxyErr != nil || state.result == nil || !state.result.ShouldDeduct {
		return
	}

	requestID := state.requestID
	routeID := state.routeID
	if ctx != nil {
		if requestID == "" {
			requestID = billingRequestID(ctx.Request, ctx)
		}
		if routeID == "" {
			routeID = billingRouteID(ctx.Route)
		}
	}

	newBalance, err := engine.Deduct(state.result, requestID, routeID)
	if err != nil {
		return
	}
	state.applied = true

	if ctx == nil {
		return
	}
	if ctx.Metadata == nil {
		ctx.Metadata = map[string]any{}
	}
	ctx.Metadata["credit_balance_after"] = newBalance
	ctx.Metadata["credits_deducted"] = state.result.Cost
}

func billingRouteID(route *config.Route) string {
	if route == nil {
		return ""
	}
	if value := strings.TrimSpace(route.ID); value != "" {
		return value
	}
	return strings.TrimSpace(route.Name)
}

func billingRequestID(req *http.Request, ctx *plugin.PipelineContext) string {
	if ctx != nil {
		if value := strings.TrimSpace(ctx.CorrelationID); value != "" {
			return value
		}
	}
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.Header.Get("X-Request-ID"))
}

func extractPermissionCreditCost(ctx *plugin.PipelineContext) *int64 {
	if ctx == nil || len(ctx.Metadata) == 0 {
		return nil
	}
	raw, ok := ctx.Metadata["permission_credit_cost"]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case int64:
		v := value
		return &v
	case int:
		v := int64(value)
		return &v
	case float64:
		v := int64(value)
		return &v
	case float32:
		v := int64(value)
		return &v
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

func metadataInt64(ctx *plugin.PipelineContext, key string) int64 {
	if ctx == nil || len(ctx.Metadata) == 0 {
		return 0
	}
	raw, ok := ctx.Metadata[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case float32:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
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
					Code:    "auth_backend_error",
					Message: "API key authentication backend unavailable",
					Status:  http.StatusInternalServerError,
				}
			}
		}
		if user == nil || key == nil {
			return nil, plugin.ErrInvalidAPIKey
		}
		repo.UpdateLastUsed(key.ID, authLookupIP(req))
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
			RateLimits:   cloneAnyMap(permission.RateLimits),
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
	for key, value := range user.Metadata {
		metadata[key] = value
	}
	if len(user.RateLimits) > 0 {
		metadata["rate_limits"] = cloneAnyMap(user.RateLimits)
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
	return &config.Consumer{
		ID:       user.ID,
		Name:     user.Name,
		Metadata: metadata,
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func routePipelineKey(route *config.Route) string {
	if route == nil {
		return ""
	}
	if value := strings.TrimSpace(route.ID); value != "" {
		return value
	}
	return strings.TrimSpace(route.Name)
}

func runPipelineCleanup(ctx *plugin.PipelineContext) {
	if ctx == nil || len(ctx.Cleanup) == 0 {
		return
	}
	for i := len(ctx.Cleanup) - 1; i >= 0; i-- {
		if ctx.Cleanup[i] != nil {
			ctx.Cleanup[i]()
		}
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
		Query     string                 `json:"query"`
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
