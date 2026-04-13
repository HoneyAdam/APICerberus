package gateway

import (
	"errors"
	"net/http"
	"time"

	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// executeProxyChain handles upstream target selection, proxying, retry logic,
// and post-proxy hooks. It always writes the response.
func (g *Gateway) executeProxyChain(r *http.Request, rs *requestState, pools map[string]*UpstreamPool, checker *Checker, billingEngine *billing.Engine) {
	pool := findUpstreamPool(pools, rs.service.Upstream)
	if pool == nil {
		rs.markBlocked("upstream_not_found")
		g.writeErrorRoute(gwResponseWriter(rs), http.StatusBadGateway, "upstream_not_found", "Service upstream is not configured", rs.route)
		return
	}

	retryPolicy := rs.pipelineCtx.Retry
	maxAttempts := 1
	if retryPolicy != nil {
		maxAttempts = retryPolicy.MaxAttempts(r.Method)
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		downstreamWriter := rs.getDownstreamWriter(rs.responseWriter)

		target, err := pool.Next(&RequestContext{
			Request:        r,
			ResponseWriter: downstreamWriter,
			Route:          rs.route,
			Consumer:       rs.consumer,
		})
		if err != nil {
			if errors.Is(err, ErrNoHealthyTargets) {
				rs.markBlocked("no_healthy_target")
				g.writeErrorRoute(gwResponseWriter(rs), http.StatusBadGateway, "no_healthy_target", "No healthy upstream target available", rs.route)
				return
			}
			rs.markBlocked("target_selection_failed")
			g.writeErrorRoute(gwResponseWriter(rs), http.StatusBadGateway, "target_selection_failed", "Failed to select upstream target", rs.route)
			return
		}
		targetID := targetKey(*target)

		if retryPolicy == nil {
			g.proxyForward(r, rs, pool, targetID, target, checker, billingEngine)
			return
		}

		if g.proxyRetry(r, rs, pool, targetID, target, checker, billingEngine, retryPolicy, attempt) {
			return
		}
		// Retry: backoff and continue loop.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(retryPolicy.Backoff(attempt)):
		}
	}

	rs.markBlocked("retries_exhausted")
	writeProxyError(rs.responseWriter, http.StatusBadGateway)
}

// proxyForward handles single-attempt (no retry) proxying.
func (g *Gateway) proxyForward(r *http.Request, rs *requestState, pool *UpstreamPool, targetID string, target *config.UpstreamTarget, checker *Checker, billingEngine *billing.Engine) {
	rs.setPipelineResponse(nil)
	proxyErr := g.proxy.Forward(&RequestContext{
		Request:         r,
		ResponseWriter:  rs.getDownstreamWriter(rs.responseWriter),
		Route:           rs.route,
		Consumer:        rs.consumer,
		UpstreamTimeout: rs.service.ReadTimeout,
	}, target)

	rs.runAfterProxy(proxyErr)
	pool.Done(targetID)

	if proxyErr != nil {
		rs.markBlocked("proxy_error")
		rs.proxyErrForAudit = proxyErr
		if checker != nil {
			checker.ReportError(pool.Name(), targetID)
		}
		return
	}

	g.executeBillingPostProxy(rs, billingEngine, nil)
	if checker != nil {
		checker.ReportSuccess(pool.Name(), targetID)
	}
}

// proxyRetry handles retry-aware proxying. Returns true when done (success or final failure).
func (g *Gateway) proxyRetry(r *http.Request, rs *requestState, pool *UpstreamPool, targetID string, target *config.UpstreamTarget, checker *Checker, billingEngine *billing.Engine, retryPolicy *plugin.Retry, attempt int) bool {
	resp, proxyErr := g.proxy.Do(&RequestContext{
		Request:         r,
		ResponseWriter:  rs.getDownstreamWriter(rs.responseWriter),
		Route:           rs.route,
		Consumer:        rs.consumer,
		UpstreamTimeout: rs.service.ReadTimeout,
	}, target)
	rs.setPipelineResponse(resp)

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
			rs.runAfterProxy(proxyErr)
			pool.Done(targetID)
			return false // signal: retry
		}
		rs.markBlocked("proxy_error")
		rs.proxyErrForAudit = proxyErr
		downstreamWriter := rs.getDownstreamWriter(rs.responseWriter)
		writeProxyError(downstreamWriter, proxyErrorStatus(proxyErr))
		rs.runAfterProxy(proxyErr)
		pool.Done(targetID)
		return true
	}

	if shouldRetry {
		if checker != nil {
			checker.ReportError(pool.Name(), targetID)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		rs.runAfterProxy(nil)
		pool.Done(targetID)
		return false // signal: retry
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
		writeErr = g.proxy.WriteResponse(rs.getDownstreamWriter(rs.responseWriter), resp)
		_ = resp.Body.Close()
	}
	rs.runAfterProxy(writeErr)
	g.executeBillingPostProxy(rs, billingEngine, writeErr)
	pool.Done(targetID)
	if writeErr != nil {
		rs.markBlocked("response_write_error")
		rs.proxyErrForAudit = writeErr
	}
	return true
}
