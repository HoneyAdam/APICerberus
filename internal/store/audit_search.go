package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Search returns audit entries matching the provided filters with pagination.
func (r *AuditRepo) Search(filters AuditSearchFilters) (*AuditListResult, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("audit repo is not initialized")
	}

	whereSQL, args := buildAuditWhere(filters)

	countSQL := "SELECT COUNT(*) FROM audit_logs" + whereSQL
	var total int
	if err := r.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count audit logs: %w", err)
	}

	limit := normalizeAuditLimit(filters.Limit)
	offset := normalizeAuditOffset(filters.Offset)
	query := auditSelectColumns + whereSQL + `
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`
	queryArgs := append(append([]any(nil), args...), limit, offset)

	entries, err := r.queryEntries(query, queryArgs...)
	if err != nil {
		return nil, err
	}

	return &AuditListResult{
		Entries: entries,
		Total:   total,
	}, nil
}

// Stats returns aggregated audit statistics for the given filters.
func (r *AuditRepo) Stats(filters AuditSearchFilters) (*AuditStats, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("audit repo is not initialized")
	}

	whereSQL, args := buildAuditWhere(filters)
	stats := &AuditStats{
		TopRoutes: []AuditRouteStat{},
		TopUsers:  []AuditUserStat{},
	}

	totalsSQL := `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(latency_ms), 0)
		  FROM audit_logs` + whereSQL
	if err := r.db.QueryRow(totalsSQL, args...).Scan(&stats.TotalRequests, &stats.ErrorRequests, &stats.AvgLatencyMS); err != nil {
		return nil, fmt.Errorf("query audit stats totals: %w", err)
	}
	if stats.TotalRequests > 0 {
		stats.ErrorRate = float64(stats.ErrorRequests) / float64(stats.TotalRequests)
	}

	routesSQL := `
		SELECT route_id, route_name, COUNT(*)
		  FROM audit_logs` + whereSQL + `
		 GROUP BY route_id, route_name
		 ORDER BY COUNT(*) DESC
		 LIMIT 10`
	routeRows, err := r.db.Query(routesSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit top routes: %w", err)
	}
	defer routeRows.Close()
	for routeRows.Next() {
		var item AuditRouteStat
		if err := routeRows.Scan(&item.RouteID, &item.RouteName, &item.Count); err != nil {
			return nil, fmt.Errorf("scan audit top route: %w", err)
		}
		stats.TopRoutes = append(stats.TopRoutes, item)
	}
	if err := routeRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit top routes: %w", err)
	}

	usersSQL := `
		SELECT user_id, consumer_name, COUNT(*)
		  FROM audit_logs` + whereSQL + `
		 GROUP BY user_id, consumer_name
		 ORDER BY COUNT(*) DESC
		 LIMIT 10`
	userRows, err := r.db.Query(usersSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit top users: %w", err)
	}
	defer userRows.Close()
	for userRows.Next() {
		var item AuditUserStat
		if err := userRows.Scan(&item.UserID, &item.ConsumerName, &item.Count); err != nil {
			return nil, fmt.Errorf("scan audit top user: %w", err)
		}
		stats.TopUsers = append(stats.TopUsers, item)
	}
	if err := userRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit top users: %w", err)
	}

	return stats, nil
}

// --- SQL helpers ---

func buildAuditWhere(filters AuditSearchFilters) (string, []any) {
	where := make([]string, 0, 12)
	args := make([]any, 0, 18)

	if value := strings.TrimSpace(filters.UserID); value != "" {
		where = append(where, "user_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filters.APIKeyPrefix); value != "" {
		where = append(where, "LOWER(request_headers) LIKE ?")
		args = append(args, "%"+strings.ToLower(value)+"%")
	}
	if value := strings.TrimSpace(filters.Route); value != "" {
		where = append(where, "(route_id = ? OR LOWER(route_name) = ?)")
		args = append(args, value, strings.ToLower(value))
	}
	if value := strings.TrimSpace(filters.Method); value != "" {
		where = append(where, "method = ?")
		args = append(args, strings.ToUpper(value))
	}
	if filters.StatusMin > 0 {
		where = append(where, "status_code >= ?")
		args = append(args, filters.StatusMin)
	}
	if filters.StatusMax > 0 {
		where = append(where, "status_code <= ?")
		args = append(args, filters.StatusMax)
	}
	if value := strings.TrimSpace(filters.ClientIP); value != "" {
		where = append(where, "client_ip = ?")
		args = append(args, value)
	}
	if filters.Blocked != nil {
		blocked := 0
		if *filters.Blocked {
			blocked = 1
		}
		where = append(where, "blocked = ?")
		args = append(args, blocked)
	}
	if value := strings.TrimSpace(filters.BlockReason); value != "" {
		where = append(where, "LOWER(block_reason) = ?")
		args = append(args, strings.ToLower(value))
	}
	if filters.DateFrom != nil {
		where = append(where, "created_at >= ?")
		args = append(args, filters.DateFrom.UTC().Format(time.RFC3339Nano))
	}
	if filters.DateTo != nil {
		where = append(where, "created_at <= ?")
		args = append(args, filters.DateTo.UTC().Format(time.RFC3339Nano))
	}
	if filters.MinLatencyMS > 0 {
		where = append(where, "latency_ms >= ?")
		args = append(args, filters.MinLatencyMS)
	}
	if value := strings.TrimSpace(filters.FullText); value != "" {
		// Use FTS5 for full-text search when available (migration v7+).
		// Fall back to LIKE if FTS5 table doesn't exist.
		ftsQuery := sanitizeFTS5Query(value)
		where = append(where, "audit_logs.rowid IN (SELECT rowid FROM audit_logs_fts WHERE audit_logs_fts MATCH ?)")
		args = append(args, ftsQuery)
	}

	if len(where) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

func normalizeAuditLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func normalizeAuditExportLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit > 100000 {
		return 100000
	}
	return limit
}

func normalizeAuditOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// sanitizeFTS5Query escapes special FTS5 characters so user input can be
// safely used in a MATCH expression. FTS5 treats " { } ( ) : * as special.
// We wrap each word in quotes for phrase matching.
func sanitizeFTS5Query(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "\"\""
	}
	// Split into tokens, escape each, wrap in quotes for phrase matching
	tokens := strings.Fields(input)
	escaped := make([]string, 0, len(tokens))
	for _, token := range tokens {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range token {
			switch r {
			case '"', '{', '}', '(', ')', ':', '*':
				// Strip FTS5 special characters
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		escaped = append(escaped, b.String())
	}
	return strings.Join(escaped, " OR ")
}
