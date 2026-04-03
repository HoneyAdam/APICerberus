package store

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestAuditRepoBatchInsertAndFindByID(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()

	repo := s.Audits()
	entries := []AuditEntry{
		{
			ID:              "audit-1",
			RequestID:       "req-1",
			RouteID:         "route-1",
			Method:          "GET",
			Path:            "/api/users",
			StatusCode:      200,
			LatencyMS:       12,
			BytesIn:         10,
			BytesOut:        24,
			Blocked:         false,
			RequestHeaders:  map[string]any{"X-Req": "1"},
			ResponseHeaders: map[string]any{"X-Res": "ok"},
			CreatedAt:       time.Now().UTC(),
		},
		{
			ID:          "audit-2",
			RequestID:   "req-2",
			RouteID:     "route-2",
			Method:      "POST",
			Path:        "/api/orders",
			StatusCode:  403,
			Blocked:     true,
			BlockReason: "ip_blocked",
			RequestBody: `{"foo":"bar"}`,
			CreatedAt:   time.Now().UTC(),
		},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	found, err := repo.FindByID("audit-2")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if found == nil {
		t.Fatalf("expected audit entry")
	}
	if !found.Blocked || found.BlockReason != "ip_blocked" {
		t.Fatalf("unexpected blocked fields: %+v", found)
	}
	if found.Method != "POST" {
		t.Fatalf("unexpected method: %s", found.Method)
	}
}

func TestAuditRepoSearchWithFilters(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	base := time.Now().UTC().Add(-time.Hour)
	if err := repo.BatchInsert([]AuditEntry{
		{
			ID:          "a1",
			UserID:      "u1",
			RouteID:     "r1",
			RouteName:   "users",
			Method:      "GET",
			StatusCode:  200,
			LatencyMS:   15,
			ClientIP:    "10.0.0.1",
			RequestBody: `{"q":"alpha"}`,
			CreatedAt:   base.Add(1 * time.Minute),
		},
		{
			ID:           "a2",
			UserID:       "u1",
			RouteID:      "r1",
			RouteName:    "users",
			Method:       "POST",
			StatusCode:   502,
			LatencyMS:    90,
			ClientIP:     "10.0.0.2",
			Blocked:      true,
			BlockReason:  "rate_limit",
			RequestBody:  `{"q":"beta"}`,
			ResponseBody: `{"error":"timeout"}`,
			CreatedAt:    base.Add(2 * time.Minute),
		},
		{
			ID:          "a3",
			UserID:      "u2",
			RouteID:     "r2",
			RouteName:   "orders",
			Method:      "GET",
			StatusCode:  404,
			LatencyMS:   40,
			ClientIP:    "10.0.0.3",
			RequestBody: `{"q":"gamma"}`,
			CreatedAt:   base.Add(3 * time.Minute),
		},
	}); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	blocked := true
	from := base.Add(90 * time.Second)
	to := base.Add(4 * time.Minute)
	result, err := repo.Search(AuditSearchFilters{
		UserID:       "u1",
		Method:       "POST",
		StatusMin:    500,
		ClientIP:     "10.0.0.2",
		Blocked:      &blocked,
		BlockReason:  "rate_limit",
		DateFrom:     &from,
		DateTo:       &to,
		MinLatencyMS: 80,
		FullText:     "timeout",
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total=1 got %d", result.Total)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != "a2" {
		t.Fatalf("unexpected entries: %+v", result.Entries)
	}
}

func TestAuditRepoStats(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	if err := repo.BatchInsert([]AuditEntry{
		{ID: "s1", UserID: "u1", ConsumerName: "c1", RouteID: "r1", RouteName: "users", StatusCode: 200, LatencyMS: 10, CreatedAt: now},
		{ID: "s2", UserID: "u1", ConsumerName: "c1", RouteID: "r1", RouteName: "users", StatusCode: 500, LatencyMS: 30, CreatedAt: now.Add(time.Millisecond)},
		{ID: "s3", UserID: "u2", ConsumerName: "c2", RouteID: "r2", RouteName: "orders", StatusCode: 503, LatencyMS: 50, CreatedAt: now.Add(2 * time.Millisecond)},
	}); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	stats, err := repo.Stats(AuditSearchFilters{})
	if err != nil {
		t.Fatalf("Stats error: %v", err)
	}
	if stats.TotalRequests != 3 {
		t.Fatalf("expected total_requests=3 got %d", stats.TotalRequests)
	}
	if stats.ErrorRequests != 2 {
		t.Fatalf("expected error_requests=2 got %d", stats.ErrorRequests)
	}
	if stats.ErrorRate <= 0.66 || stats.ErrorRate >= 0.67 {
		t.Fatalf("unexpected error_rate %f", stats.ErrorRate)
	}
	if len(stats.TopRoutes) == 0 || stats.TopRoutes[0].RouteID != "r1" {
		t.Fatalf("unexpected top routes: %+v", stats.TopRoutes)
	}
	if len(stats.TopUsers) == 0 || stats.TopUsers[0].UserID != "u1" {
		t.Fatalf("unexpected top users: %+v", stats.TopUsers)
	}
}

func TestAuditRepoDeleteOlderThan(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	newTime := time.Now().UTC().Add(-time.Hour)
	if err := repo.BatchInsert([]AuditEntry{
		{ID: "d1", CreatedAt: oldTime},
		{ID: "d2", CreatedAt: oldTime.Add(time.Second)},
		{ID: "d3", CreatedAt: newTime},
	}); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	deleted, err := repo.DeleteOlderThan(time.Now().UTC().Add(-24*time.Hour), 1)
	if err != nil {
		t.Fatalf("DeleteOlderThan error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected deleted=2 got %d", deleted)
	}
	left, err := repo.Search(AuditSearchFilters{Limit: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if left.Total != 1 || left.Entries[0].ID != "d3" {
		t.Fatalf("unexpected remaining rows: %+v", left)
	}
}

func TestAuditRepoDeleteOlderThanRouteAndExclude(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	if err := repo.BatchInsert([]AuditEntry{
		{ID: "rx-default-old", RouteID: "default", CreatedAt: now.Add(-40 * 24 * time.Hour)},
		{ID: "rx-default-new", RouteID: "default", CreatedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "rx-critical-old", RouteID: "critical", CreatedAt: now.Add(-100 * 24 * time.Hour)},
		{ID: "rx-critical-mid", RouteID: "critical", CreatedAt: now.Add(-40 * 24 * time.Hour)},
	}); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	deletedCritical, err := repo.DeleteOlderThanForRoute("critical", now.Add(-90*24*time.Hour), 10)
	if err != nil {
		t.Fatalf("DeleteOlderThanForRoute error: %v", err)
	}
	if deletedCritical != 1 {
		t.Fatalf("expected deletedCritical=1 got %d", deletedCritical)
	}

	deletedDefault, err := repo.DeleteOlderThanExcludingRoutes(now.Add(-30*24*time.Hour), 10, []string{"critical"})
	if err != nil {
		t.Fatalf("DeleteOlderThanExcludingRoutes error: %v", err)
	}
	if deletedDefault != 1 {
		t.Fatalf("expected deletedDefault=1 got %d", deletedDefault)
	}

	remaining, err := repo.Search(AuditSearchFilters{Limit: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if remaining.Total != 2 {
		t.Fatalf("expected 2 remaining entries got %d", remaining.Total)
	}
	ids := map[string]bool{}
	for _, entry := range remaining.Entries {
		ids[entry.ID] = true
	}
	if !ids["rx-default-new"] || !ids["rx-critical-mid"] {
		t.Fatalf("unexpected remaining IDs: %+v", ids)
	}
}

func TestAuditRepoExportFormats(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	if err := repo.BatchInsert([]AuditEntry{
		{ID: "e1", RouteID: "r1", Method: "GET", StatusCode: 200, CreatedAt: time.Now().UTC()},
		{ID: "e2", RouteID: "r2", Method: "POST", StatusCode: 500, CreatedAt: time.Now().UTC().Add(time.Millisecond)},
	}); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	var jsonl bytes.Buffer
	if err := repo.Export(AuditSearchFilters{}, "jsonl", &jsonl); err != nil {
		t.Fatalf("Export jsonl error: %v", err)
	}
	jsonlOut := strings.TrimSpace(jsonl.String())
	if !strings.Contains(jsonlOut, "\"id\":\"e1\"") || !strings.Contains(jsonlOut, "\"id\":\"e2\"") {
		t.Fatalf("unexpected jsonl output: %s", jsonlOut)
	}

	var jsonBuf bytes.Buffer
	if err := repo.Export(AuditSearchFilters{}, "json", &jsonBuf); err != nil {
		t.Fatalf("Export json error: %v", err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal json export: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 json rows got %d", len(parsed))
	}

	var csvBuf bytes.Buffer
	if err := repo.Export(AuditSearchFilters{}, "csv", &csvBuf); err != nil {
		t.Fatalf("Export csv error: %v", err)
	}
	csvOut := csvBuf.String()
	if !strings.Contains(csvOut, "id,created_at") {
		t.Fatalf("csv header missing: %s", csvOut)
	}
	if !strings.Contains(csvOut, "e1") || !strings.Contains(csvOut, "e2") {
		t.Fatalf("csv rows missing: %s", csvOut)
	}
}

func openAuditStore(t *testing.T) *Store {
	t.Helper()
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}
	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	return s
}

// Test List function
func TestAuditRepoList(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	entries := []AuditEntry{
		{ID: "list-1", RouteID: "r1", Method: "GET", Path: "/api/test1", StatusCode: 200, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "list-2", RouteID: "r2", Method: "POST", Path: "/api/test2", StatusCode: 201, CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "list-3", RouteID: "r1", Method: "GET", Path: "/api/test3", StatusCode: 404, CreatedAt: now.Add(-15 * time.Minute)},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Test List with filters
	opts := AuditListOptions{
		RouteID: "r1",
		Limit:   10,
	}
	result, err := repo.List(opts)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries for route r1, got %d", len(result.Entries))
	}

	// Test List with method filter
	opts = AuditListOptions{
		Method: "POST",
		Limit:  10,
	}
	result, err = repo.List(opts)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry for POST method, got %d", len(result.Entries))
	}
}

// Test ListOlderThan function
func TestAuditRepoListOlderThan(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	entries := []AuditEntry{
		{ID: "old-1", RouteID: "r1", Method: "GET", Path: "/api/old1", StatusCode: 200, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "old-2", RouteID: "r1", Method: "POST", Path: "/api/old2", StatusCode: 201, CreatedAt: now.Add(-36 * time.Hour)},
		{ID: "new-1", RouteID: "r1", Method: "GET", Path: "/api/new", StatusCode: 200, CreatedAt: now.Add(-12 * time.Hour)},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// List entries older than 24 hours
	olderThan := now.Add(-24 * time.Hour)
	result, err := repo.ListOlderThan(olderThan, 10)
	if err != nil {
		t.Fatalf("ListOlderThan error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries older than 24h, got %d", len(result))
	}
}

// Test ListOlderThanForRoute function
func TestAuditRepoListOlderThanForRoute(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	entries := []AuditEntry{
		{ID: "route-old-1", RouteID: "r1", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "route-old-2", RouteID: "r1", Method: "POST", Path: "/api/test", StatusCode: 201, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "route-new", RouteID: "r1", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now.Add(-12 * time.Hour)},
		{ID: "other-route-old", RouteID: "r2", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now.Add(-72 * time.Hour)},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// List entries for route r1 older than 24 hours
	olderThan := now.Add(-24 * time.Hour)
	result, err := repo.ListOlderThanForRoute("r1", olderThan, 10)
	if err != nil {
		t.Fatalf("ListOlderThanForRoute error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries for route r1 older than 24h, got %d", len(result))
	}
}

// Test ListOlderThanExcludingRoutes function
func TestAuditRepoListOlderThanExcludingRoutes(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	entries := []AuditEntry{
		{ID: "excl-old-1", RouteID: "r1", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "excl-old-2", RouteID: "r2", Method: "POST", Path: "/api/test", StatusCode: 201, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "excl-old-3", RouteID: "r3", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now.Add(-36 * time.Hour)},
		{ID: "excl-new", RouteID: "r1", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now.Add(-12 * time.Hour)},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// List entries older than 24 hours excluding r1
	olderThan := now.Add(-24 * time.Hour)
	result, err := repo.ListOlderThanExcludingRoutes(olderThan, 10, []string{"r1"})
	if err != nil {
		t.Fatalf("ListOlderThanExcludingRoutes error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries excluding r1, got %d", len(result))
	}
}

// Test DeleteByIDs function
func TestAuditRepoDeleteByIDs(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	entries := []AuditEntry{
		{ID: "del-1", RouteID: "r1", Method: "GET", Path: "/api/test1", StatusCode: 200, CreatedAt: now},
		{ID: "del-2", RouteID: "r1", Method: "POST", Path: "/api/test2", StatusCode: 201, CreatedAt: now},
		{ID: "del-3", RouteID: "r1", Method: "GET", Path: "/api/test3", StatusCode: 404, CreatedAt: now},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Delete specific IDs
	deleted, err := repo.DeleteByIDs([]string{"del-1", "del-3"})
	if err != nil {
		t.Fatalf("DeleteByIDs error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted entries, got %d", deleted)
	}

	// Verify remaining
	result, err := repo.Search(AuditSearchFilters{Limit: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Total != 1 || result.Entries[0].ID != "del-2" {
		t.Errorf("unexpected remaining entries: %+v", result)
	}
}

// Test DeleteByIDs with empty input
func TestAuditRepoDeleteByIDs_Empty(t *testing.T) {
	t.Parallel()

	s := openAuditStore(t)
	defer s.Close()
	repo := s.Audits()

	now := time.Now().UTC()
	entries := []AuditEntry{
		{ID: "keep-1", RouteID: "r1", Method: "GET", Path: "/api/test", StatusCode: 200, CreatedAt: now},
	}

	if err := repo.BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Delete with empty IDs
	deleted, err := repo.DeleteByIDs([]string{})
	if err != nil {
		t.Fatalf("DeleteByIDs error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted entries, got %d", deleted)
	}

	// Verify entry still exists
	result, err := repo.Search(AuditSearchFilters{Limit: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 remaining entry, got %d", result.Total)
	}
}

