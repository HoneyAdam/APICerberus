package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

func (s *Server) analyticsOverview(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}

	from, to, err := resolveAnalyticsRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}
	metrics := analyticsMetricsInWindow(engine, from, to)

	var totalLatency int64
	var totalErrors int64
	var creditsConsumed int64
	for _, metric := range metrics {
		totalLatency += metric.LatencyMS
		creditsConsumed += metric.CreditsConsumed
		if metric.Error || metric.StatusCode >= http.StatusInternalServerError {
			totalErrors++
		}
	}

	avgLatency := 0.0
	errorRate := 0.0
	if len(metrics) > 0 {
		avgLatency = float64(totalLatency) / float64(len(metrics))
		errorRate = float64(totalErrors) / float64(len(metrics))
	}
	overview := engine.Overview()

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":              from.UTC().Format(time.RFC3339Nano),
		"to":                to.UTC().Format(time.RFC3339Nano),
		"total_requests":    len(metrics),
		"active_conns":      overview.ActiveConns,
		"error_rate":        errorRate,
		"avg_latency_ms":    avgLatency,
		"credits_consumed":  creditsConsumed,
		"lifetime_requests": overview.TotalRequests,
		"lifetime_errors":   overview.TotalErrors,
	})
}

func (s *Server) analyticsTimeSeries(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}

	query := r.URL.Query()
	from, to, err := resolveAnalyticsRange(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}
	granularity, err := resolveAnalyticsGranularity(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_granularity", err.Error())
		return
	}

	metrics := analyticsMetricsInWindow(engine, from, to)
	series := aggregateAnalyticsSeries(metrics, granularity)
	items := make([]map[string]any, 0, len(series))
	for _, bucket := range series {
		items = append(items, map[string]any{
			"timestamp":        bucket.start.UTC().Format(time.RFC3339Nano),
			"requests":         bucket.requests,
			"errors":           bucket.errors,
			"avg_latency_ms":   analyticsAverage(bucket.latencies),
			"p50_latency_ms":   analyticsPercentile(bucket.latencies, 50),
			"p95_latency_ms":   analyticsPercentile(bucket.latencies, 95),
			"p99_latency_ms":   analyticsPercentile(bucket.latencies, 99),
			"status_codes":     bucket.statusCodes,
			"bytes_in":         bucket.bytesIn,
			"bytes_out":        bucket.bytesOut,
			"credits_consumed": bucket.creditsConsumed,
		})
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":        from.UTC().Format(time.RFC3339Nano),
		"to":          to.UTC().Format(time.RFC3339Nano),
		"granularity": granularity.String(),
		"items":       items,
	})
}

func (s *Server) analyticsTopRoutes(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}
	query := r.URL.Query()
	from, to, err := resolveAnalyticsRange(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}
	limit, err := resolveAnalyticsLimit(query, 10, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}

	metrics := analyticsMetricsInWindow(engine, from, to)
	type routeStat struct {
		RouteID   string `json:"route_id"`
		RouteName string `json:"route_name"`
		Count     int64  `json:"count"`
	}
	grouped := map[string]*routeStat{}
	for _, metric := range metrics {
		routeID := strings.TrimSpace(metric.RouteID)
		routeName := strings.TrimSpace(metric.RouteName)
		if routeID == "" && routeName == "" {
			routeID = "unknown"
			routeName = "unknown"
		}
		key := strings.ToLower(routeID + "|" + routeName)
		item := grouped[key]
		if item == nil {
			item = &routeStat{RouteID: routeID, RouteName: routeName}
			grouped[key] = item
		}
		item.Count++
	}
	items := make([]routeStat, 0, len(grouped))
	for _, item := range grouped {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].RouteID < items[j].RouteID
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":   from.UTC().Format(time.RFC3339Nano),
		"to":     to.UTC().Format(time.RFC3339Nano),
		"routes": items,
	})
}

func (s *Server) analyticsTopConsumers(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}
	query := r.URL.Query()
	from, to, err := resolveAnalyticsRange(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}
	limit, err := resolveAnalyticsLimit(query, 10, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}

	type consumerStat struct {
		UserID string `json:"user_id"`
		Count  int64  `json:"count"`
	}
	grouped := map[string]int64{}
	for _, metric := range analyticsMetricsInWindow(engine, from, to) {
		key := strings.TrimSpace(metric.UserID)
		if key == "" {
			key = "anonymous"
		}
		grouped[key]++
	}
	items := make([]consumerStat, 0, len(grouped))
	for userID, count := range grouped {
		items = append(items, consumerStat{UserID: userID, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].UserID < items[j].UserID
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":      from.UTC().Format(time.RFC3339Nano),
		"to":        to.UTC().Format(time.RFC3339Nano),
		"consumers": items,
	})
}

func (s *Server) analyticsErrors(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}
	from, to, err := resolveAnalyticsRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}

	type errorStat struct {
		StatusCode int    `json:"status_code"`
		RouteID    string `json:"route_id"`
		RouteName  string `json:"route_name"`
		Count      int64  `json:"count"`
	}

	grouped := map[string]*errorStat{}
	var total int64
	for _, metric := range analyticsMetricsInWindow(engine, from, to) {
		if !(metric.Error || metric.StatusCode >= 400) {
			continue
		}
		total++
		routeID := strings.TrimSpace(metric.RouteID)
		routeName := strings.TrimSpace(metric.RouteName)
		if routeID == "" && routeName == "" {
			routeID, routeName = "unknown", "unknown"
		}
		key := fmt.Sprintf("%d|%s|%s", metric.StatusCode, strings.ToLower(routeID), strings.ToLower(routeName))
		item := grouped[key]
		if item == nil {
			item = &errorStat{
				StatusCode: metric.StatusCode,
				RouteID:    routeID,
				RouteName:  routeName,
			}
			grouped[key] = item
		}
		item.Count++
	}

	breakdown := make([]errorStat, 0, len(grouped))
	for _, item := range grouped {
		breakdown = append(breakdown, *item)
	}
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Count == breakdown[j].Count {
			if breakdown[i].StatusCode == breakdown[j].StatusCode {
				return breakdown[i].RouteID < breakdown[j].RouteID
			}
			return breakdown[i].StatusCode < breakdown[j].StatusCode
		}
		return breakdown[i].Count > breakdown[j].Count
	})

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":         from.UTC().Format(time.RFC3339Nano),
		"to":           to.UTC().Format(time.RFC3339Nano),
		"total_errors": total,
		"breakdown":    breakdown,
	})
}

func (s *Server) analyticsLatency(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}
	from, to, err := resolveAnalyticsRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}

	latencies := make([]int64, 0, 128)
	for _, metric := range analyticsMetricsInWindow(engine, from, to) {
		latencies = append(latencies, metric.LatencyMS)
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":           from.UTC().Format(time.RFC3339Nano),
		"to":             to.UTC().Format(time.RFC3339Nano),
		"count":          len(latencies),
		"avg_latency_ms": analyticsAverage(latencies),
		"p50_latency_ms": analyticsPercentile(latencies, 50),
		"p95_latency_ms": analyticsPercentile(latencies, 95),
		"p99_latency_ms": analyticsPercentile(latencies, 99),
	})
}

func (s *Server) analyticsThroughput(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}
	query := r.URL.Query()
	from, to, err := resolveAnalyticsRange(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}
	granularity, err := resolveAnalyticsGranularity(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_granularity", err.Error())
		return
	}

	series := aggregateAnalyticsSeries(analyticsMetricsInWindow(engine, from, to), granularity)
	items := make([]map[string]any, 0, len(series))
	seconds := granularity.Seconds()
	if seconds <= 0 {
		seconds = 1
	}
	for _, bucket := range series {
		items = append(items, map[string]any{
			"timestamp": bucket.start.UTC().Format(time.RFC3339Nano),
			"requests":  bucket.requests,
			"rps":       float64(bucket.requests) / seconds,
		})
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":        from.UTC().Format(time.RFC3339Nano),
		"to":          to.UTC().Format(time.RFC3339Nano),
		"granularity": granularity.String(),
		"items":       items,
	})
}

func (s *Server) analyticsStatusCodes(w http.ResponseWriter, r *http.Request) {
	engine := s.gateway.Analytics()
	if engine == nil {
		writeError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics engine is not initialized")
		return
	}
	from, to, err := resolveAnalyticsRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_analytics_range", err.Error())
		return
	}

	counts := map[int]int64{}
	var total int64
	for _, metric := range analyticsMetricsInWindow(engine, from, to) {
		if metric.StatusCode <= 0 {
			continue
		}
		counts[metric.StatusCode]++
		total++
	}

	type statusItem struct {
		StatusCode int   `json:"status_code"`
		Count      int64 `json:"count"`
	}
	items := make([]statusItem, 0, len(counts))
	for code, count := range counts {
		items = append(items, statusItem{StatusCode: code, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StatusCode < items[j].StatusCode
	})

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":         from.UTC().Format(time.RFC3339Nano),
		"to":           to.UTC().Format(time.RFC3339Nano),
		"total":        total,
		"status_codes": items,
	})
}

func (s *Server) listAlerts(w http.ResponseWriter, _ *http.Request) {
	s.evaluateAlerts()
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"rules":   s.alertEngine.ListRules(),
		"history": s.alertEngine.History(200),
	})
}

func (s *Server) createAlert(w http.ResponseWriter, r *http.Request) {
	var in analytics.AlertRule
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		in.ID = id
	}
	if _, exists := s.alertEngine.GetRule(in.ID); exists {
		writeError(w, http.StatusConflict, "alert_rule_exists", "alert rule id already exists")
		return
	}

	rule, err := s.alertEngine.UpsertRule(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_alert_rule", err.Error())
		return
	}
	s.evaluateAlerts()
	_ = jsonutil.WriteJSON(w, http.StatusCreated, rule)
}

func (s *Server) updateAlert(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_alert_rule", "alert id is required")
		return
	}

	var in analytics.AlertRule
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		in.ID = id
	}
	if in.ID != id {
		writeError(w, http.StatusBadRequest, "invalid_alert_rule", "path id and payload id must match")
		return
	}
	if _, exists := s.alertEngine.GetRule(id); !exists {
		writeError(w, http.StatusNotFound, "alert_rule_not_found", "alert rule not found")
		return
	}

	rule, err := s.alertEngine.UpsertRule(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_alert_rule", err.Error())
		return
	}
	s.evaluateAlerts()
	_ = jsonutil.WriteJSON(w, http.StatusOK, rule)
}

func (s *Server) deleteAlert(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_alert_rule", "alert id is required")
		return
	}
	if !s.alertEngine.DeleteRule(id) {
		writeError(w, http.StatusNotFound, "alert_rule_not_found", "alert rule not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) evaluateAlerts() []analytics.AlertHistoryEntry {
	if s == nil || s.alertEngine == nil || s.gateway == nil {
		return nil
	}
	analyticsEngine := s.gateway.Analytics()
	if analyticsEngine == nil {
		return nil
	}

	metrics := analyticsEngine.Latest(5000)
	upstreamHealthPercent := s.currentUpstreamHealthPercent()
	return s.alertEngine.Evaluate(metrics, upstreamHealthPercent, time.Now().UTC())
}

func (s *Server) currentUpstreamHealthPercent() float64 {
	if s == nil || s.gateway == nil {
		return 100
	}

	upstreams := s.snapshotUpstreams()
	total := 0
	healthy := 0

	for _, upstream := range upstreams {
		lookup := strings.TrimSpace(upstream.ID)
		if lookup == "" {
			lookup = strings.TrimSpace(upstream.Name)
		}
		if lookup == "" {
			continue
		}

		state := s.gateway.UpstreamHealth(lookup)
		for _, target := range upstream.Targets {
			targetID := strings.TrimSpace(target.ID)
			if targetID == "" {
				continue
			}
			total++
			if state[targetID] {
				healthy++
			}
		}
	}

	if total == 0 {
		return 100
	}
	return (float64(healthy) / float64(total)) * 100
}

func resolveAnalyticsRange(query url.Values) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	to := now
	var err error
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		to, err = parseAuditTime(raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("to must be RFC3339")
		}
	}

	window := time.Hour
	if raw := strings.TrimSpace(query.Get("window")); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			return time.Time{}, time.Time{}, errors.New("window must be a valid duration")
		}
		if parsed > 0 {
			window = parsed
		}
	}

	from := to.Add(-window)
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		from, err = parseAuditTime(raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("from must be RFC3339")
		}
	}
	if from.After(to) {
		from, to = to, from
	}
	return from.UTC(), to.UTC(), nil
}

func resolveAnalyticsGranularity(query url.Values) (time.Duration, error) {
	raw := strings.TrimSpace(firstNonEmpty(query.Get("granularity"), query.Get("bucket")))
	if raw == "" {
		return time.Minute, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New("granularity must be a valid duration")
	}
	if parsed <= 0 {
		return 0, errors.New("granularity must be greater than zero")
	}
	if parsed < time.Second {
		parsed = time.Second
	}
	return parsed, nil
}

func resolveAnalyticsLimit(query url.Values, fallback, max int) (int, error) {
	limit := fallback
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, errors.New("limit must be numeric")
		}
		if value > 0 {
			limit = value
		}
	}
	if limit <= 0 {
		limit = fallback
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit, nil
}

func analyticsMetricsInWindow(engine *analytics.Engine, from, to time.Time) []analytics.RequestMetric {
	if engine == nil {
		return nil
	}
	all := engine.Latest(0)
	if len(all) == 0 {
		return nil
	}
	from = from.UTC()
	to = to.UTC()
	if from.After(to) {
		from, to = to, from
	}
	items := make([]analytics.RequestMetric, 0, len(all))
	for _, metric := range all {
		ts := metric.Timestamp.UTC()
		if ts.Before(from) || ts.After(to) {
			continue
		}
		items = append(items, metric)
	}
	return items
}

type analyticsSeriesItem struct {
	start           time.Time
	requests        int64
	errors          int64
	latencies       []int64
	statusCodes     map[int]int64
	bytesIn         int64
	bytesOut        int64
	creditsConsumed int64
}

func aggregateAnalyticsSeries(metrics []analytics.RequestMetric, granularity time.Duration) []analyticsSeriesItem {
	if granularity <= 0 {
		granularity = time.Minute
	}
	grouped := map[int64]*analyticsSeriesItem{}
	for _, metric := range metrics {
		start := metric.Timestamp.UTC().Truncate(granularity)
		key := start.UnixNano()
		item := grouped[key]
		if item == nil {
			item = &analyticsSeriesItem{
				start:       start,
				latencies:   make([]int64, 0, 64),
				statusCodes: map[int]int64{},
			}
			grouped[key] = item
		}
		item.requests++
		if metric.Error || metric.StatusCode >= http.StatusInternalServerError {
			item.errors++
		}
		item.latencies = append(item.latencies, metric.LatencyMS)
		if metric.StatusCode > 0 {
			item.statusCodes[metric.StatusCode]++
		}
		item.bytesIn += metric.BytesIn
		item.bytesOut += metric.BytesOut
		item.creditsConsumed += metric.CreditsConsumed
	}

	out := make([]analyticsSeriesItem, 0, len(grouped))
	for _, item := range grouped {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].start.Before(out[j].start)
	})
	return out
}

func analyticsAverage(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum int64
	for _, value := range values {
		sum += value
	}
	return float64(sum) / float64(len(values))
}

func analyticsPercentile(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}
	if percentile <= 0 {
		percentile = 1
	}
	if percentile > 100 {
		percentile = 100
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	rank := (percentile*len(sorted) + 99) / 100
	if rank <= 0 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
