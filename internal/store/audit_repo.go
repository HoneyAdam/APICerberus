package store

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

const auditSelectColumns = `
	SELECT id, request_id, route_id, route_name, service_name,
	       user_id, consumer_name, method, host, path, query,
	       status_code, latency_ms, bytes_in, bytes_out,
	       client_ip, user_agent, blocked, block_reason,
	       request_headers, request_body, response_headers, response_body,
	       error_message, created_at
	  FROM audit_logs`

type AuditEntry struct {
	ID              string         `json:"id"`
	RequestID       string         `json:"request_id"`
	RouteID         string         `json:"route_id"`
	RouteName       string         `json:"route_name"`
	ServiceName     string         `json:"service_name"`
	UserID          string         `json:"user_id"`
	ConsumerName    string         `json:"consumer_name"`
	Method          string         `json:"method"`
	Host            string         `json:"host"`
	Path            string         `json:"path"`
	Query           string         `json:"query"`
	StatusCode      int            `json:"status_code"`
	LatencyMS       int64          `json:"latency_ms"`
	BytesIn         int64          `json:"bytes_in"`
	BytesOut        int64          `json:"bytes_out"`
	ClientIP        string         `json:"client_ip"`
	UserAgent       string         `json:"user_agent"`
	Blocked         bool           `json:"blocked"`
	BlockReason     string         `json:"block_reason"`
	RequestHeaders  map[string]any `json:"request_headers"`
	RequestBody     string         `json:"request_body"`
	ResponseHeaders map[string]any `json:"response_headers"`
	ResponseBody    string         `json:"response_body"`
	ErrorMessage    string         `json:"error_message"`
	CreatedAt       time.Time      `json:"created_at"`
}

type AuditListOptions struct {
	UserID    string
	RouteID   string
	Method    string
	StatusMin int
	StatusMax int
	Limit     int
	Offset    int
}

type AuditSearchFilters struct {
	UserID       string
	APIKeyPrefix string
	Route        string
	Method       string
	StatusMin    int
	StatusMax    int
	ClientIP     string
	Blocked      *bool
	BlockReason  string
	DateFrom     *time.Time
	DateTo       *time.Time
	MinLatencyMS int64
	FullText     string
	Limit        int
	Offset       int
}

type AuditListResult struct {
	Entries []AuditEntry `json:"entries"`
	Total   int          `json:"total"`
}

type AuditRouteStat struct {
	RouteID   string `json:"route_id"`
	RouteName string `json:"route_name"`
	Count     int64  `json:"count"`
}

type AuditUserStat struct {
	UserID       string `json:"user_id"`
	ConsumerName string `json:"consumer_name"`
	Count        int64  `json:"count"`
}

type AuditStats struct {
	TotalRequests int64            `json:"total_requests"`
	ErrorRequests int64            `json:"error_requests"`
	ErrorRate     float64          `json:"error_rate"`
	AvgLatencyMS  float64          `json:"avg_latency_ms"`
	TopRoutes     []AuditRouteStat `json:"top_routes"`
	TopUsers      []AuditUserStat  `json:"top_users"`
}

type AuditRepo struct {
	db  *sql.DB
	now func() time.Time
}

func (s *Store) Audits() *AuditRepo {
	if s == nil || s.db == nil {
		return nil
	}
	return &AuditRepo{
		db:  s.db,
		now: time.Now,
	}
}

func (r *AuditRepo) BatchInsert(entries []AuditEntry) error {
	if r == nil || r.db == nil {
		return errors.New("audit repo is not initialized")
	}
	if len(entries) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin audit batch insert: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO audit_logs(
			id, request_id, route_id, route_name, service_name,
			user_id, consumer_name, method, host, path, query,
			status_code, latency_ms, bytes_in, bytes_out,
			client_ip, user_agent, blocked, block_reason,
			request_headers, request_body, response_headers, response_body,
			error_message, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare audit insert: %w", err)
	}
	defer stmt.Close()

	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			id, genErr := uuid.NewString()
			if genErr != nil {
				_ = tx.Rollback()
				return genErr
			}
			entry.ID = id
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = r.now().UTC()
		}

		requestHeaders, marshalErr := marshalJSON(entry.RequestHeaders, "{}")
		if marshalErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal audit request headers: %w", marshalErr)
		}
		responseHeaders, marshalErr := marshalJSON(entry.ResponseHeaders, "{}")
		if marshalErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("marshal audit response headers: %w", marshalErr)
		}

		blocked := 0
		if entry.Blocked {
			blocked = 1
		}

		if _, err := stmt.Exec(
			entry.ID,
			strings.TrimSpace(entry.RequestID),
			strings.TrimSpace(entry.RouteID),
			strings.TrimSpace(entry.RouteName),
			strings.TrimSpace(entry.ServiceName),
			strings.TrimSpace(entry.UserID),
			strings.TrimSpace(entry.ConsumerName),
			strings.TrimSpace(strings.ToUpper(entry.Method)),
			strings.TrimSpace(entry.Host),
			strings.TrimSpace(entry.Path),
			strings.TrimSpace(entry.Query),
			entry.StatusCode,
			entry.LatencyMS,
			entry.BytesIn,
			entry.BytesOut,
			strings.TrimSpace(entry.ClientIP),
			strings.TrimSpace(entry.UserAgent),
			blocked,
			strings.TrimSpace(entry.BlockReason),
			requestHeaders,
			entry.RequestBody,
			responseHeaders,
			entry.ResponseBody,
			entry.ErrorMessage,
			entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert audit entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit audit batch insert: %w", err)
	}
	return nil
}

func (r *AuditRepo) FindByID(id string) (*AuditEntry, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("audit repo is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("audit id is required")
	}

	row := r.db.QueryRow(auditSelectColumns+`
		 WHERE id = ?
	`, id)
	entry, err := scanAuditRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return entry, nil
}

func (r *AuditRepo) List(opts AuditListOptions) (*AuditListResult, error) {
	return r.Search(AuditSearchFilters{
		UserID:    opts.UserID,
		Route:     opts.RouteID,
		Method:    opts.Method,
		StatusMin: opts.StatusMin,
		StatusMax: opts.StatusMax,
		Limit:     opts.Limit,
		Offset:    opts.Offset,
	})
}

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

func (r *AuditRepo) DeleteOlderThan(cutoff time.Time, batchSize int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("audit repo is not initialized")
	}
	if cutoff.IsZero() {
		return 0, errors.New("cutoff is required")
	}
	if batchSize <= 0 {
		batchSize = 1000
	}

	cutoffRaw := cutoff.UTC().Format(time.RFC3339Nano)
	var deletedTotal int64

	for {
		result, err := r.db.Exec(`
			DELETE FROM audit_logs
			 WHERE id IN (
			 	SELECT id
			 	  FROM audit_logs
			 	 WHERE created_at < ?
			 	 ORDER BY created_at
			 	 LIMIT ?
			 )
		`, cutoffRaw, batchSize)
		if err != nil {
			return deletedTotal, fmt.Errorf("delete audit logs older than cutoff: %w", err)
		}
		affected, _ := result.RowsAffected()
		if affected <= 0 {
			break
		}
		deletedTotal += affected
		if affected < int64(batchSize) {
			break
		}
	}

	return deletedTotal, nil
}

func (r *AuditRepo) Export(filters AuditSearchFilters, format string, w io.Writer) error {
	if r == nil || r.db == nil {
		return errors.New("audit repo is not initialized")
	}
	if w == nil {
		return errors.New("export writer is nil")
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "jsonl"
	}
	switch format {
	case "csv", "json", "jsonl":
	default:
		return errors.New("unsupported export format")
	}

	whereSQL, args := buildAuditWhere(filters)
	query := auditSelectColumns + whereSQL + ` ORDER BY created_at DESC`
	if limit := normalizeAuditExportLimit(filters.Limit); limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
		if offset := normalizeAuditOffset(filters.Offset); offset > 0 {
			query += ` OFFSET ?`
			args = append(args, offset)
		}
	} else if offset := normalizeAuditOffset(filters.Offset); offset > 0 {
		query += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("query audit export rows: %w", err)
	}
	defer rows.Close()

	switch format {
	case "csv":
		return exportAuditCSV(rows, w)
	case "json":
		return exportAuditJSON(rows, w)
	default:
		return exportAuditJSONL(rows, w)
	}
}

func (r *AuditRepo) queryEntries(query string, args ...any) ([]AuditEntry, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	items := make([]AuditEntry, 0, 32)
	for rows.Next() {
		entry, scanErr := scanAuditRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit logs: %w", err)
	}
	return items, nil
}

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
		pattern := "%" + strings.ToLower(value) + "%"
		where = append(where, "(LOWER(path) LIKE ? OR LOWER(request_body) LIKE ? OR LOWER(response_body) LIKE ?)")
		args = append(args, pattern, pattern, pattern)
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

func exportAuditCSV(rows *sql.Rows, w io.Writer) error {
	cw := csv.NewWriter(w)
	header := []string{
		"id", "created_at", "request_id", "route_id", "route_name", "service_name",
		"user_id", "consumer_name", "method", "host", "path", "query",
		"status_code", "latency_ms", "bytes_in", "bytes_out", "client_ip", "user_agent",
		"blocked", "block_reason", "request_headers", "request_body", "response_headers", "response_body", "error_message",
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for rows.Next() {
		entry, err := scanAuditRows(rows)
		if err != nil {
			return err
		}
		reqHeaders, _ := json.Marshal(entry.RequestHeaders)
		resHeaders, _ := json.Marshal(entry.ResponseHeaders)
		record := []string{
			entry.ID,
			entry.CreatedAt.UTC().Format(time.RFC3339Nano),
			entry.RequestID,
			entry.RouteID,
			entry.RouteName,
			entry.ServiceName,
			entry.UserID,
			entry.ConsumerName,
			entry.Method,
			entry.Host,
			entry.Path,
			entry.Query,
			fmt.Sprint(entry.StatusCode),
			fmt.Sprint(entry.LatencyMS),
			fmt.Sprint(entry.BytesIn),
			fmt.Sprint(entry.BytesOut),
			entry.ClientIP,
			entry.UserAgent,
			fmt.Sprint(entry.Blocked),
			entry.BlockReason,
			string(reqHeaders),
			entry.RequestBody,
			string(resHeaders),
			entry.ResponseBody,
			entry.ErrorMessage,
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate export rows: %w", err)
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("flush csv writer: %w", err)
	}
	return nil
}

func exportAuditJSONL(rows *sql.Rows, w io.Writer) error {
	for rows.Next() {
		entry, err := scanAuditRows(rows)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal jsonl entry: %w", err)
		}
		if _, err := w.Write(encoded); err != nil {
			return fmt.Errorf("write jsonl entry: %w", err)
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return fmt.Errorf("write jsonl newline: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate export rows: %w", err)
	}
	return nil
}

func exportAuditJSON(rows *sql.Rows, w io.Writer) error {
	if _, err := io.WriteString(w, "["); err != nil {
		return fmt.Errorf("write json export prefix: %w", err)
	}
	first := true
	for rows.Next() {
		entry, err := scanAuditRows(rows)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal json export entry: %w", err)
		}
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return fmt.Errorf("write json export separator: %w", err)
			}
		}
		first = false
		if _, err := w.Write(encoded); err != nil {
			return fmt.Errorf("write json export entry: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate export rows: %w", err)
	}
	if _, err := io.WriteString(w, "]"); err != nil {
		return fmt.Errorf("write json export suffix: %w", err)
	}
	return nil
}

func scanAuditRow(row *sql.Row) (*AuditEntry, error) {
	var (
		entry                                 AuditEntry
		blockedInt                            int
		requestHeadersRaw, responseHeadersRaw string
		createdAtRaw                          string
	)
	if err := row.Scan(
		&entry.ID,
		&entry.RequestID,
		&entry.RouteID,
		&entry.RouteName,
		&entry.ServiceName,
		&entry.UserID,
		&entry.ConsumerName,
		&entry.Method,
		&entry.Host,
		&entry.Path,
		&entry.Query,
		&entry.StatusCode,
		&entry.LatencyMS,
		&entry.BytesIn,
		&entry.BytesOut,
		&entry.ClientIP,
		&entry.UserAgent,
		&blockedInt,
		&entry.BlockReason,
		&requestHeadersRaw,
		&entry.RequestBody,
		&responseHeadersRaw,
		&entry.ResponseBody,
		&entry.ErrorMessage,
		&createdAtRaw,
	); err != nil {
		return nil, err
	}
	if err := decodeAuditFields(&entry, blockedInt, requestHeadersRaw, responseHeadersRaw, createdAtRaw); err != nil {
		return nil, err
	}
	return &entry, nil
}

func scanAuditRows(rows *sql.Rows) (*AuditEntry, error) {
	var (
		entry                                 AuditEntry
		blockedInt                            int
		requestHeadersRaw, responseHeadersRaw string
		createdAtRaw                          string
	)
	if err := rows.Scan(
		&entry.ID,
		&entry.RequestID,
		&entry.RouteID,
		&entry.RouteName,
		&entry.ServiceName,
		&entry.UserID,
		&entry.ConsumerName,
		&entry.Method,
		&entry.Host,
		&entry.Path,
		&entry.Query,
		&entry.StatusCode,
		&entry.LatencyMS,
		&entry.BytesIn,
		&entry.BytesOut,
		&entry.ClientIP,
		&entry.UserAgent,
		&blockedInt,
		&entry.BlockReason,
		&requestHeadersRaw,
		&entry.RequestBody,
		&responseHeadersRaw,
		&entry.ResponseBody,
		&entry.ErrorMessage,
		&createdAtRaw,
	); err != nil {
		return nil, err
	}
	if err := decodeAuditFields(&entry, blockedInt, requestHeadersRaw, responseHeadersRaw, createdAtRaw); err != nil {
		return nil, err
	}
	return &entry, nil
}

func decodeAuditFields(entry *AuditEntry, blockedInt int, requestHeadersRaw, responseHeadersRaw, createdAtRaw string) error {
	if entry == nil {
		return errors.New("audit entry is nil")
	}
	entry.Blocked = blockedInt == 1

	entry.RequestHeaders = map[string]any{}
	if strings.TrimSpace(requestHeadersRaw) == "" {
		requestHeadersRaw = "{}"
	}
	if err := json.Unmarshal([]byte(requestHeadersRaw), &entry.RequestHeaders); err != nil {
		return fmt.Errorf("decode audit request_headers: %w", err)
	}

	entry.ResponseHeaders = map[string]any{}
	if strings.TrimSpace(responseHeadersRaw) == "" {
		responseHeadersRaw = "{}"
	}
	if err := json.Unmarshal([]byte(responseHeadersRaw), &entry.ResponseHeaders); err != nil {
		return fmt.Errorf("decode audit response_headers: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return fmt.Errorf("decode audit created_at: %w", err)
	}
	entry.CreatedAt = createdAt
	return nil
}
