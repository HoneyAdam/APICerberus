package admin

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func (s *Server) searchAuditLogs(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	result, err := st.Audits().Search(filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "search_audit_logs_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) searchUserAuditLogs(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}
	filters.UserID = strings.TrimSpace(r.PathValue("id"))

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	result, err := st.Audits().Search(filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "search_user_audit_logs_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) getAuditLog(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_audit_id", "audit log id is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	entry, err := st.Audits().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "get_audit_log_failed", err.Error())
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "audit_log_not_found", "Audit log not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, entry)
}

func (s *Server) auditLogStats(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}
	filters.Limit = 0
	filters.Offset = 0

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	stats, err := st.Audits().Stats(filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit_stats_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, stats)
}

func (s *Server) exportAuditLogs(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "jsonl"
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	var body bytes.Buffer
	if err := st.Audits().Export(filters, format, &body); err != nil {
		writeError(w, http.StatusBadRequest, "export_audit_logs_failed", err.Error())
		return
	}

	fileExt := auditExportFileExtension(format)
	fileName := fmt.Sprintf("audit-logs-%s.%s", time.Now().UTC().Format("20060102-150405"), fileExt)
	w.Header().Set("Content-Type", auditExportContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func (s *Server) cleanupAuditLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cutoff, err := resolveAuditCleanupCutoff(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cleanup_cutoff", err.Error())
		return
	}

	batchSize := 1000
	if raw := strings.TrimSpace(query.Get("batch_size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_batch_size", "batch_size must be numeric")
			return
		}
		if parsed > 0 {
			batchSize = parsed
		}
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	deleted, err := st.Audits().DeleteOlderThan(cutoff, batchSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit_cleanup_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":    deleted,
		"cutoff":     cutoff.UTC().Format(time.RFC3339Nano),
		"batch_size": batchSize,
	})
}

func parseAuditSearchFilters(query url.Values) (store.AuditSearchFilters, error) {
	filters := store.AuditSearchFilters{
		UserID:       strings.TrimSpace(query.Get("user_id")),
		APIKeyPrefix: strings.TrimSpace(query.Get("api_key_prefix")),
		Route:        strings.TrimSpace(query.Get("route")),
		Method:       strings.TrimSpace(query.Get("method")),
		ClientIP:     strings.TrimSpace(query.Get("client_ip")),
		BlockReason:  strings.TrimSpace(query.Get("block_reason")),
		FullText:     strings.TrimSpace(firstNonEmpty(query.Get("q"), query.Get("search"))),
	}

	if raw := strings.TrimSpace(firstNonEmpty(query.Get("status_min"), query.Get("status_code_min"))); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("status_min must be numeric")
		}
		filters.StatusMin = value
	}
	if raw := strings.TrimSpace(firstNonEmpty(query.Get("status_max"), query.Get("status_code_max"))); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("status_max must be numeric")
		}
		filters.StatusMax = value
	}
	if raw := strings.TrimSpace(query.Get("min_latency_ms")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return filters, errors.New("min_latency_ms must be numeric")
		}
		filters.MinLatencyMS = value
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("limit must be numeric")
		}
		filters.Limit = value
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("offset must be numeric")
		}
		filters.Offset = value
	}
	if raw := strings.TrimSpace(query.Get("blocked")); raw != "" {
		value, err := parseBoolString(raw)
		if err != nil {
			return filters, errors.New("blocked must be true or false")
		}
		filters.Blocked = &value
	}
	if raw := strings.TrimSpace(query.Get("date_from")); raw != "" {
		value, err := parseAuditTime(raw)
		if err != nil {
			return filters, errors.New("date_from must be RFC3339")
		}
		filters.DateFrom = &value
	}
	if raw := strings.TrimSpace(query.Get("date_to")); raw != "" {
		value, err := parseAuditTime(raw)
		if err != nil {
			return filters, errors.New("date_to must be RFC3339")
		}
		filters.DateTo = &value
	}
	return filters, nil
}

func resolveAuditCleanupCutoff(query url.Values) (time.Time, error) {
	if raw := strings.TrimSpace(query.Get("cutoff")); raw != "" {
		value, err := parseAuditTime(raw)
		if err != nil {
			return time.Time{}, errors.New("cutoff must be RFC3339")
		}
		return value, nil
	}

	days := 30
	if raw := strings.TrimSpace(query.Get("older_than_days")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return time.Time{}, errors.New("older_than_days must be numeric")
		}
		if value > 0 {
			days = value
		}
	}
	return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour), nil
}

func parseAuditTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("empty time value")
	}
	if value, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return value, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func parseBoolString(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("invalid boolean")
	}
}

func auditExportContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "text/csv; charset=utf-8"
	case "json":
		return "application/json; charset=utf-8"
	default:
		return "application/x-ndjson; charset=utf-8"
	}
}

func auditExportFileExtension(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "csv"
	case "json":
		return "json"
	default:
		return "jsonl"
	}
}
