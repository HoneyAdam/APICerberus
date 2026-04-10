package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// newAdminTestServerWithHandle returns the admin server instance for direct method access
func newAdminTestServerWithHandle(t *testing.T) (adminBaseURL string, upstreamURL string, storePath string, token string, adminSrv *Server) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	storePath = t.TempDir() + "/admin-bulk-test.db"
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
			{ID: "svc-users", Name: "svc-users", Protocol: "http", Upstream: "up-users"},
		},
		Routes: []config.Route{
			{ID: "route-users", Name: "route-users", Service: "svc-users", Paths: []string{"/users"}, Methods: []string{http.MethodGet}},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-users",
				Name:      "up-users",
				Algorithm: "round_robin",
				Targets:   []config.UpstreamTarget{{ID: "up-users-t1", Address: mustHost(t, upstream.URL), Weight: 1}},
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}
	adminSrv, err = NewServer(cfg, gw)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(adminSrv)
	t.Cleanup(httpSrv.Close)
	t.Cleanup(func() { _ = adminSrv.Close() })
	t.Cleanup(func() { _ = gw.Shutdown(context.Background()) })

	token, err = issueAdminToken(cfg.Admin.TokenSecret, cfg.Admin.TokenTTL)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return httpSrv.URL, upstream.URL, storePath, token, adminSrv
}

// TestBulkTransactionDirect covers NewBulkTransaction, Complete, and DB operations
func TestBulkTransactionDirect(t *testing.T) {
	t.Parallel()
	_, _, _, _, srv := newAdminTestServerWithHandle(t)

	t.Run("new bulk transaction", func(t *testing.T) {
		tx := srv.NewBulkTransaction()
		if tx == nil {
			t.Fatal("expected non-nil transaction")
		}
		if tx.completed {
			t.Error("expected transaction to not be completed")
		}
		if tx.srv == nil {
			t.Error("expected transaction to have server reference")
		}
	})

	t.Run("complete marks transaction done", func(t *testing.T) {
		tx := srv.NewBulkTransaction()
		tx.Complete()
		if !tx.completed {
			t.Error("expected transaction to be completed")
		}
	})

	t.Run("new bulk database operation", func(t *testing.T) {
		op, err := srv.NewBulkDatabaseOperation()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if op == nil {
			t.Fatal("expected non-nil operation")
		}
		// Commit should work
		err = op.Commit()
		if err != nil {
			t.Errorf("expected no error on commit, got %v", err)
		}
	})

	t.Run("rollback before commit", func(t *testing.T) {
		op, err := srv.NewBulkDatabaseOperation()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		err = op.Rollback()
		if err != nil {
			t.Errorf("expected no error on rollback, got %v", err)
		}
	})

	t.Run("exec within transaction", func(t *testing.T) {
		op, err := srv.NewBulkDatabaseOperation()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer op.Rollback()
		result, err := op.Exec("SELECT 1")
		if err != nil {
			t.Errorf("expected no error on exec, got %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("query row within transaction", func(t *testing.T) {
		op, err := srv.NewBulkDatabaseOperation()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer op.Rollback()
		row := op.QueryRow("SELECT 1")
		if row == nil {
			t.Error("expected non-nil row")
		}
		var val int
		err = row.Scan(&val)
		if err != nil {
			t.Errorf("expected no error scanning, got %v", err)
		}
		if val != 1 {
			t.Errorf("expected 1, got %d", val)
		}
	})

	t.Run("query within transaction", func(t *testing.T) {
		op, err := srv.NewBulkDatabaseOperation()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer op.Rollback()
		rows, err := op.Query("SELECT 1")
		if err != nil {
			t.Errorf("expected no error on query, got %v", err)
		}
		if rows == nil {
			t.Error("expected non-nil rows")
		} else {
			defer rows.Close()
		}
	})
}
