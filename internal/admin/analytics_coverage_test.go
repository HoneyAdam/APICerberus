package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestAnalyticsErrorsVariousRanges(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath := newAdminTestServer(t)

	// Seed data
	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	seedStore.Audits().BatchInsert([]store.AuditEntry{
		{ID: "a1", RequestID: "r1", RouteID: "route-users", ServiceName: "svc-users", Method: "GET", Path: "/users", StatusCode: 500, LatencyMS: 100, ClientIP: "127.0.0.1", CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "a2", RequestID: "r2", RouteID: "route-users", ServiceName: "svc-users", Method: "POST", Path: "/users", StatusCode: 502, LatencyMS: 200, ClientIP: "127.0.0.1", CreatedAt: now.Add(-15 * time.Minute)},
	})
	seedStore.Close()

	ranges := []string{"?window=1h", "?window=24h", "?window=168h", "?window=720h", "?from=2024-01-01T00:00:00Z&to=2024-12-31T00:00:00Z"}
	for _, r := range ranges {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/errors"+r, "secret-admin", nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200 for range %s, got %v", r, resp["status_code"])
		}
	}
}

func TestAnalyticsTimeSeriesAggregation(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath := newAdminTestServer(t)

	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	entries := make([]store.AuditEntry, 50)
	for i := 0; i < 50; i++ {
		entries[i] = store.AuditEntry{
			ID: "ts" + string(rune('a'+i%26)), RequestID: "r" + string(rune('a'+i%26)),
			RouteID: "route-users", ServiceName: "svc-users", Method: "GET",
			Path: "/users", StatusCode: 200, LatencyMS: int64(50 + i*10),
			ClientIP: "127.0.0.1", CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
	}
	seedStore.Audits().BatchInsert(entries)
	seedStore.Close()

	granularities := []string{"1m", "5m", "1h", "24h"}
	for _, g := range granularities {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/timeseries?window=24h&granularity="+g, "secret-admin", nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200 for granularity %s, got %v", g, resp["status_code"])
		}
	}
}

func TestAnalyticsLatencyPercentiles(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath := newAdminTestServer(t)

	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	entries := make([]store.AuditEntry, 100)
	for i := 0; i < 100; i++ {
		entries[i] = store.AuditEntry{
			ID: "p" + string(rune('a'+i%26)) + string(rune('0'+i/26)), RequestID: "rp" + string(rune('a'+i%26)),
			RouteID: "route-users", ServiceName: "svc-users", Method: "GET",
			Path: "/users", StatusCode: 200, LatencyMS: int64(i * 10),
			ClientIP: "127.0.0.1", CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
	}
	seedStore.Audits().BatchInsert(entries)
	seedStore.Close()

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/latency?window=24h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "p50_latency_ms")
	assertHasJSONField(t, resp, "p95_latency_ms")
	assertHasJSONField(t, resp, "p99_latency_ms")
}

func TestAnalyticsTopRoutesAndConsumers(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath := newAdminTestServer(t)

	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedStore.Audits().BatchInsert([]store.AuditEntry{
			{ID: "tr" + string(rune(i)), RequestID: "rq" + string(rune(i)), RouteID: "route-users", ServiceName: "svc-users",
				Method: "GET", Path: "/users", StatusCode: 200, LatencyMS: 50,
				ClientIP: "127.0.0.1", CreatedAt: now.Add(-time.Duration(i) * time.Minute)},
		})
	}
	seedStore.Close()

	limits := []string{"5", "10", "50"}
	for _, l := range limits {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?window=24h&limit="+l, "secret-admin", nil)
		assertStatus(t, resp, http.StatusOK)
		assertHasJSONField(t, resp, "routes")
	}
}

func TestAnalyticsThroughputAndStatusCodes(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath := newAdminTestServer(t)

	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	codes := []int{200, 201, 400, 401, 403, 404, 500, 502, 503}
	for i, code := range codes {
		seedStore.Audits().BatchInsert([]store.AuditEntry{
			{ID: "sc" + string(rune(i)), RequestID: "rsc" + string(rune(i)), RouteID: "route-users", ServiceName: "svc-users",
				Method: "GET", Path: "/users", StatusCode: code, LatencyMS: 50,
				ClientIP: "127.0.0.1", CreatedAt: now.Add(-time.Duration(i) * time.Minute)},
		})
	}
	seedStore.Close()

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/throughput?window=24h&granularity=1h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "items")

	resp = mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/status-codes?window=24h", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "status_codes")
}

func TestAnalyticsOverviewVariousWindows(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath := newAdminTestServer(t)

	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	seedStore.Audits().BatchInsert([]store.AuditEntry{
		{ID: "ov1", RequestID: "rov1", RouteID: "route-users", ServiceName: "svc-users",
			Method: "GET", Path: "/users", StatusCode: 200, LatencyMS: 50,
			ClientIP: "127.0.0.1", CreatedAt: now.Add(-5 * time.Minute)},
	})
	seedStore.Close()

	windows := []string{"1h", "24h", "168h"}
	for _, w := range windows {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/overview?window="+w, "secret-admin", nil)
		assertStatus(t, resp, http.StatusOK)
		assertHasJSONField(t, resp, "total_requests")
	}
}
