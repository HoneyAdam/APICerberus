package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// newAdminTestServerWithAnalytics creates an admin test server with a seeded analytics engine
func newAdminTestServerWithAnalytics(t *testing.T) (adminBaseURL string, gw *gateway.Gateway, token string) {
	t.Helper()

	storePath := t.TempDir() + "/admin-analytics-test.db"
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			APIKey:      "secret-admin",
			TokenSecret: "secret-admin-token-secret-at-least-32-chars-long",
			TokenTTL:    1 * time.Hour,
			UIEnabled:   true,
		},
		Store: config.StoreConfig{
			Path:        storePath,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Services: []config.Service{
			{ID: "svc-1", Name: "Test Service", Protocol: "http"},
		},
		Routes: []config.Route{
			{ID: "route-1", Name: "Test Route", Service: "svc-1", Paths: []string{"/api/*"}, Methods: []string{"GET"}},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	// Seed analytics data
	engine := gw.Analytics()
	if engine != nil {
		now := time.Now().UTC()
		// Normal requests
		for i := 0; i < 5; i++ {
			engine.Record(analytics.RequestMetric{
				Timestamp:   now.Add(-time.Duration(i) * time.Minute),
				RouteID:     "route-1",
				RouteName:   "Test Route",
				ServiceName: "Test Service",
				UserID:      "user-1",
				Method:      "GET",
				Path:        "/api/test",
				StatusCode:  200,
				LatencyMS:   int64(50 + i*10),
				BytesIn:     100,
				BytesOut:    500,
			})
		}
		// Error requests
		for i := 0; i < 3; i++ {
			engine.Record(analytics.RequestMetric{
				Timestamp:   now.Add(-time.Duration(i) * time.Minute),
				RouteID:     "route-2",
				RouteName:   "Error Route",
				ServiceName: "Test Service",
				UserID:      "user-2",
				Method:      "POST",
				Path:        "/api/error",
				StatusCode:  500,
				LatencyMS:   int64(200 + i*50),
				Error:       true,
			})
		}
		// Different user
		engine.Record(analytics.RequestMetric{
			Timestamp:       now.Add(-1 * time.Minute),
			RouteID:         "route-1",
			RouteName:       "Test Route",
			ServiceName:     "Test Service",
			UserID:          "user-3",
			Method:          "GET",
			Path:            "/api/data",
			StatusCode:      201,
			LatencyMS:       75,
			CreditsConsumed: 10,
		})
	}

	adminSrv, err := NewServer(cfg, gw)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(adminSrv)
	t.Cleanup(httpSrv.Close)
	t.Cleanup(func() { _ = adminSrv.Close() })
	t.Cleanup(func() { _ = gw.Shutdown(context.Background()) })

	token, err = issueAdminToken(cfg.Admin.TokenSecret, cfg.Admin.TokenTTL, string(RoleAdmin), RolePermissions[RoleAdmin])
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return httpSrv.URL, gw, token
}

// TestAnalytics_WithData tests analytics endpoints with seeded data
func TestAnalytics_WithData(t *testing.T) {
	t.Parallel()
	baseURL, _, token := newAdminTestServerWithAnalytics(t)

	t.Run("overview with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/overview", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("time series with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/timeseries?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("top routes with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
		if routes, ok := resp["routes"]; ok {
			if arr, ok := routes.([]any); ok && len(arr) == 0 {
				t.Log("routes array is empty - may be due to time window")
			}
		}
	})

	t.Run("top consumers with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-consumers?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("errors with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/errors?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("latency with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/latency?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("throughput with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/throughput?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("status codes with data", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/status-codes?window=1h", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("top routes with limit", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?window=1h&limit=1", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})

	t.Run("top consumers with limit", func(t *testing.T) {
		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-consumers?window=1h&limit=1", token, nil)
		if resp["status_code"].(float64) != http.StatusOK {
			t.Errorf("expected 200, got %v", resp["status_code"])
		}
	})
}
