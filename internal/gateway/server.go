package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/audit"
	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/plugin"
	"github.com/APICerberus/APICerebrus/internal/store"
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
	upstreams      map[string]*UpstreamPool
	consumers      []config.Consumer
	authAPIKey     *plugin.AuthAPIKey
	authRequired   bool
	routePipelines map[string][]plugin.PipelinePlugin
	routeHasAuth   map[string]bool
	httpServer     *http.Server
	startedAt      time.Time

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
	authAPIKey := newAuthAPIKey(cfg, consumers, nil)
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

	g := &Gateway{
		config:         cfg,
		router:         router,
		proxy:          NewProxy(),
		health:         checker,
		store:          st,
		billing:        billingEngine,
		auditLogger:    auditLogger,
		auditRetention: auditRetention,
		upstreams:      upstreamPools,
		consumers:      consumers,
		authAPIKey:     authAPIKey,
		authRequired:   len(consumers) > 0,
		routePipelines: routePipelines,
		routeHasAuth:   routeHasAuth,
		startedAt:      time.Now(),
	}
	g.httpServer = g.newHTTPServer(cfg.Gateway.HTTPAddr)
	return g, nil
}

func (g *Gateway) newHTTPServer(addr string) *http.Server {
	gwCfg := g.config.Gateway
	server := &http.Server{
		Addr:           addr,
		Handler:        g,
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
	authAPIKey := g.authAPIKey
	authRequired := g.authRequired
	routePipelines := g.routePipelines
	routeHasAuth := g.routeHasAuth
	g.mu.RUnlock()

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
	)
	if auditLogger != nil {
		if body, captureErr := audit.CaptureRequestBody(r, auditLogger.MaxRequestBodyBytes()); captureErr == nil {
			requestBodySnapshot = body
		}
		auditWriter = audit.NewResponseCaptureWriter(w, auditLogger.MaxResponseBodyBytes())
		w = auditWriter
	}
	defer func() {
		if auditLogger == nil || auditWriter == nil {
			return
		}
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
	pipelineCtx := &plugin.PipelineContext{
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
		if capture, ok := pipelineCtx.ResponseWriter.(*plugin.CaptureResponseWriter); ok && capture.HasCaptured() && !capture.IsFlushed() {
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
				time.Sleep(retryPolicy.Backoff(attempt))
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

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = g.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
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
	newAuthAPIKey := newAuthAPIKey(newCfg, newConsumers, nil)
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

	g.httpServer.ReadTimeout = newCfg.Gateway.ReadTimeout
	g.httpServer.WriteTimeout = newCfg.Gateway.WriteTimeout
	g.httpServer.IdleTimeout = newCfg.Gateway.IdleTimeout
	g.httpServer.MaxHeaderBytes = newCfg.Gateway.MaxHeaderBytes

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

// Shutdown gracefully drains active connections and stops background health loops.
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.mu.RLock()
	healthCancel := g.healthCancel
	auditCancel := g.auditCancel
	server := g.httpServer
	st := g.store
	g.mu.RUnlock()

	if healthCancel != nil {
		healthCancel()
	}
	if auditCancel != nil {
		auditCancel()
	}
	shutdownErr := server.Shutdown(ctx)
	if st != nil {
		_ = st.Close()
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

func newAuthAPIKey(cfg *config.Config, consumers []config.Consumer, lookup plugin.APIKeyLookupFunc) *plugin.AuthAPIKey {
	if cfg == nil {
		return plugin.NewAuthAPIKey(consumers, plugin.AuthAPIKeyOptions{
			Lookup: lookup,
		})
	}
	return plugin.NewAuthAPIKey(consumers, plugin.AuthAPIKeyOptions{
		KeyNames:    append([]string(nil), cfg.Auth.APIKey.KeyNames...),
		QueryNames:  append([]string(nil), cfg.Auth.APIKey.QueryNames...),
		CookieNames: append([]string(nil), cfg.Auth.APIKey.CookieNames...),
		Lookup:      lookup,
	})
}

func newAuditLogger(st *store.Store, cfg *config.Config) *audit.Logger {
	if st == nil || cfg == nil {
		return nil
	}
	return audit.NewLogger(st.Audits(), cfg.Audit)
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
	if xff := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			if first := strings.TrimSpace(parts[0]); first != "" {
				return first
			}
		}
	}
	return clientIP(req.RemoteAddr)
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
