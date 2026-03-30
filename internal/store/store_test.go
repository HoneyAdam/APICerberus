package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestOpenInMemoryAppliesPragmasAndMigrations(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 3 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer s.Close()

	var foreignKeys int
	if err := s.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign_keys=1 got %d", foreignKeys)
	}

	var busyTimeout int
	if err := s.DB().QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout pragma: %v", err)
	}
	if busyTimeout < 3000 {
		t.Fatalf("expected busy_timeout >= 3000ms got %d", busyTimeout)
	}

	expectedTables := []string{"schema_migrations", "users", "api_keys", "credit_transactions", "endpoint_permissions", "audit_logs", "sessions", "playground_templates"}
	for _, table := range expectedTables {
		var name string
		err := s.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s to exist: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "apicerberus.db")
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        dbPath,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}

	s1, err := Open(cfg)
	if err != nil {
		t.Fatalf("first Open error: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("first close error: %v", err)
	}

	s2, err := Open(cfg)
	if err != nil {
		t.Fatalf("second Open error: %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations rows: %v", err)
	}
	if count != len(migrations) {
		t.Fatalf("expected %d applied migrations got %d", len(migrations), count)
	}
}

func TestValidateStoreConfig(t *testing.T) {
	t.Parallel()

	err := validateStoreConfig(config.StoreConfig{
		Path:        "",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	})
	if err == nil {
		t.Fatalf("expected validation error for empty path")
	}
}
