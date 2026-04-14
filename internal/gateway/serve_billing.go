package gateway

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// executeBillingPreProxy checks credits before proxying. Returns true if the
// request was handled (response already written due to billing error).
func (g *Gateway) executeBillingPreProxy(r *http.Request, rs *requestState, engine *billing.Engine, route *config.Route, consumer *config.Consumer) bool {
	state, err := applyBillingPreProxy(engine, r, route, consumer, rs.pipelineCtx)
	if err != nil {
		rs.markBlocked("billing_precheck_failed")
		g.writeBillingError(gwResponseWriter(rs), err)
		return true
	}
	rs.billingState = state
	return false
}

// executeBillingPostProxy deducts credits after a successful proxy response.
func (g *Gateway) executeBillingPostProxy(rs *requestState, engine *billing.Engine, proxyErr error) {
	applyBillingPostProxy(engine, rs.billingState, rs.pipelineCtx, proxyErr)
}

// --- Billing helpers (moved from server.go) ---

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

	var newBalance int64
	var err error
	for retries := 0; retries < 3; retries++ {
		newBalance, err = engine.Deduct(ctx.Request.Context(), state.result, requestID, routeID)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "database is locked") && !strings.Contains(err.Error(), "SQLITE_BUSY") {
			return
		}
		time.Sleep(100 * time.Millisecond * (1 << retries))
	}
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
