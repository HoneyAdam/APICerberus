package portal

import (
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

type playgroundSendRequest struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Query     map[string]string `json:"query"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	APIKey    string            `json:"api_key"`
	TimeoutMS int               `json:"timeout_ms"`
}

func (s *Server) playgroundSend(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}

	var in playgroundSendRequest
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	pathValue := strings.TrimSpace(in.Path)
	if pathValue == "" || !strings.HasPrefix(pathValue, "/") {
		writeError(w, http.StatusBadRequest, "invalid_path", "playground path must start with '/'")
		return
	}
	rawKey := strings.TrimSpace(in.APIKey)
	if rawKey == "" {
		rawKey = strings.TrimSpace(in.Headers["X-API-Key"])
	}
	if rawKey == "" {
		writeError(w, http.StatusBadRequest, "missing_api_key", "playground api_key is required")
		return
	}

	snapshot := s.configSnapshot()
	targetURL := strings.TrimSuffix(resolveGatewayBaseURL(snapshot.GatewayAddr), "/") + pathValue
	if len(in.Query) > 0 {
		query := url.Values{}
		for key, value := range in.Query {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			query.Set(key, value)
		}
		if encoded := query.Encode(); encoded != "" {
			targetURL += "?" + encoded
		}
	}

	req, err := http.NewRequestWithContext(r.Context(), method, targetURL, strings.NewReader(in.Body))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to build upstream request")
		return
	}
	req.Header.Set("X-API-Key", rawKey)
	for key, value := range in.Headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "host") {
			continue
		}
		req.Header.Set(key, value)
	}
	if strings.TrimSpace(req.Header.Get("Content-Type")) == "" && strings.TrimSpace(in.Body) != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	timeout := 30 * time.Second
	if in.TimeoutMS > 0 && in.TimeoutMS < 120000 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}
	client := &http.Client{Timeout: timeout}
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "playground_request_failed", err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "playground_read_failed", err.Error())
		return
	}
	headers := map[string]string{}
	for key, values := range resp.Header {
		if len(values) == 0 {
			continue
		}
		headers[key] = strings.Join(values, ", ")
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"request": map[string]any{
			"method": method,
			"url":    targetURL,
		},
		"response": map[string]any{
			"status_code": resp.StatusCode,
			"headers":     headers,
			"body":        string(body),
			"latency_ms":  time.Since(started).Milliseconds(),
		},
	})
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	items, err := s.store.PlaygroundTemplates().ListByUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_templates_failed", "failed to list templates")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) saveTemplate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	in := store.PlaygroundTemplate{}
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	creating := strings.TrimSpace(in.ID) == ""
	in.UserID = user.ID
	if err := s.store.PlaygroundTemplates().Save(&in); err != nil {
		writeError(w, http.StatusBadRequest, "save_template_failed", err.Error())
		return
	}
	status := http.StatusOK
	if creating {
		status = http.StatusCreated
	}
	_ = jsonutil.WriteJSON(w, status, in)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_template", "template id is required")
		return
	}
	if err := s.store.PlaygroundTemplates().DeleteForUser(id, user.ID); err != nil {
		writeError(w, http.StatusBadRequest, "delete_template_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) usageOverview(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	from, to, err := parsePortalTimeRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	stats, err := s.store.Audits().Stats(store.AuditSearchFilters{
		UserID:   user.ID,
		DateFrom: &from,
		DateTo:   &to,
		Limit:    5000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage_overview_failed", "failed to compute usage overview")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":           from,
		"to":             to,
		"total_requests": stats.TotalRequests,
		"error_requests": stats.ErrorRequests,
		"error_rate":     stats.ErrorRate,
		"avg_latency_ms": stats.AvgLatencyMS,
		"credit_balance": user.CreditBalance,
	})
}

func (s *Server) usageTimeSeries(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	from, to, err := parsePortalTimeRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	granularity, err := parsePortalGranularity(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_granularity", err.Error())
		return
	}
	result, err := s.store.Audits().Search(store.AuditSearchFilters{
		UserID:   user.ID,
		DateFrom: &from,
		DateTo:   &to,
		Limit:    10000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage_timeseries_failed", "failed to compute usage timeseries")
		return
	}
	type bucket struct {
		start      time.Time
		requests   int64
		errors     int64
		latencySum int64
	}
	grouped := map[int64]*bucket{}
	for _, entry := range result.Entries {
		start := entry.CreatedAt.UTC().Truncate(granularity)
		key := start.UnixNano()
		item := grouped[key]
		if item == nil {
			item = &bucket{start: start}
			grouped[key] = item
		}
		item.requests++
		if entry.StatusCode >= 500 {
			item.errors++
		}
		item.latencySum += entry.LatencyMS
	}
	items := make([]map[string]any, 0, len(grouped))
	for _, item := range grouped {
		avg := 0.0
		if item.requests > 0 {
			avg = float64(item.latencySum) / float64(item.requests)
		}
		items = append(items, map[string]any{
			"timestamp":      item.start,
			"requests":       item.requests,
			"errors":         item.errors,
			"avg_latency_ms": avg,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["timestamp"].(time.Time).Before(items[j]["timestamp"].(time.Time))
	})
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"from":        from,
		"to":          to,
		"granularity": granularity.String(),
		"items":       items,
	})
}

func (s *Server) usageTopEndpoints(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	from, to, err := parsePortalTimeRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	stats, err := s.store.Audits().Stats(store.AuditSearchFilters{
		UserID:   user.ID,
		DateFrom: &from,
		DateTo:   &to,
		Limit:    5000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage_top_endpoints_failed", "failed to compute usage top endpoints")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "items": stats.TopRoutes})
}

func (s *Server) usageErrors(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	from, to, err := parsePortalTimeRange(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	result, err := s.store.Audits().Search(store.AuditSearchFilters{
		UserID:   user.ID,
		DateFrom: &from,
		DateTo:   &to,
		Limit:    10000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage_errors_failed", "failed to compute usage errors")
		return
	}
	byCode := map[int]int64{}
	for _, entry := range result.Entries {
		if entry.StatusCode >= 400 {
			byCode[entry.StatusCode]++
		}
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "status_map": byCode})
}
