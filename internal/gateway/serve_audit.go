package gateway

import (
	"net/http"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/audit"
)

// logRequestAudit records the request/response in the audit log.
func logRequestAudit(auditLogger *audit.Logger, r *http.Request, rs *requestState) {
	if auditLogger == nil || rs.auditWriter == nil {
		return
	}
	auditLogger.Log(audit.LogInput{
		Request:        r,
		ResponseWriter: rs.auditWriter,
		Route:          rs.route,
		Service:        rs.service,
		Consumer:       rs.consumer,
		RequestBody:    rs.requestBodySnapshot,
		StartedAt:      rs.requestStartedAt,
		Blocked:        rs.blocked,
		BlockReason:    rs.blockReason,
		ProxyErr:       rs.proxyErrForAudit,
	})
}

// recordAnalytics records the request metrics.
func recordAnalytics(engine *analytics.Engine, r *http.Request, rs *requestState) {
	if engine == nil {
		return
	}

	statusCode := 0
	bytesOut := int64(0)
	if rs.responseWriter != nil {
		statusCode = rs.responseWriter.StatusCode()
		bytesOut = rs.responseWriter.BytesWritten()
	}
	bytesIn := int64(0)
	if r != nil {
		bytesIn = r.ContentLength
	}
	if bytesIn < 0 {
		bytesIn = 0
	}
	if bytesIn == 0 && len(rs.requestBodySnapshot) > 0 {
		bytesIn = int64(len(rs.requestBodySnapshot))
	}

	routeID := ""
	routeName := ""
	serviceName := ""
	userID := ""
	method := ""
	path := ""
	if rs.route != nil {
		routeID = strings.TrimSpace(rs.route.ID)
		routeName = strings.TrimSpace(rs.route.Name)
	}
	if rs.service != nil {
		serviceName = strings.TrimSpace(rs.service.Name)
	}
	if rs.consumer != nil {
		userID = strings.TrimSpace(rs.consumer.ID)
	}
	if r != nil {
		method = strings.TrimSpace(strings.ToUpper(r.Method))
		if r.URL != nil {
			path = strings.TrimSpace(r.URL.Path)
		}
	}
	creditsConsumed := metadataInt64(rs.pipelineCtx, "credits_deducted")

	engine.Record(analytics.RequestMetric{
		Timestamp:       rs.requestStartedAt.UTC(),
		RouteID:         routeID,
		RouteName:       routeName,
		ServiceName:     serviceName,
		UserID:          userID,
		Method:          method,
		Path:            path,
		StatusCode:      statusCode,
		LatencyMS:       time.Since(rs.requestStartedAt).Milliseconds(),
		BytesIn:         bytesIn,
		BytesOut:        bytesOut,
		CreditsConsumed: creditsConsumed,
		Blocked:         rs.blocked,
		Error:           rs.blocked || rs.proxyErrForAudit != nil || statusCode >= http.StatusInternalServerError,
	})
}

// newResponseCaptureWriter creates the audit response capture writer if auditing
// or analytics is enabled. Returns the writer and the wrapped http.ResponseWriter.
func newResponseCaptureWriter(w http.ResponseWriter, auditLogger *audit.Logger, analyticsEngine *analytics.Engine) (http.ResponseWriter, *audit.ResponseCaptureWriter) {
	if auditLogger == nil && analyticsEngine == nil {
		return w, nil
	}
	maxResponseBodyBytes := int64(0)
	if auditLogger != nil {
		maxResponseBodyBytes = auditLogger.MaxResponseBodyBytes()
	}
	responseWriter := audit.NewResponseCaptureWriter(w, maxResponseBodyBytes)
	return responseWriter, responseWriter
}

// captureRequestBody reads and snapshots the request body for audit logging.
func captureRequestBody(r *http.Request, auditLogger *audit.Logger) []byte {
	if auditLogger == nil {
		return nil
	}
	body, _ := audit.CaptureRequestBody(r, auditLogger.MaxRequestBodyBytes())
	return body
}
