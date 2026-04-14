package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/migrations"
)

const sqliteDriverName = "apicerberus-sqlite"

type Store struct {
	db  *sql.DB
	cfg config.StoreConfig
}

var migrationsList = []migrations.Migration{
	{
		Version: 1,
		Name:    "init_core_tables",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				email TEXT NOT NULL UNIQUE,
				name TEXT NOT NULL,
				company TEXT NOT NULL DEFAULT '',
				password_hash TEXT NOT NULL DEFAULT '',
				role TEXT NOT NULL DEFAULT 'user',
				status TEXT NOT NULL DEFAULT 'active',
				credit_balance INTEGER NOT NULL DEFAULT 0,
				rate_limits TEXT NOT NULL DEFAULT '{}',
				ip_whitelist TEXT NOT NULL DEFAULT '[]',
				metadata TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS api_keys (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				key_hash TEXT NOT NULL UNIQUE,
				key_prefix TEXT NOT NULL,
				name TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'active',
				expires_at TEXT NOT NULL DEFAULT '',
				last_used_at TEXT NOT NULL DEFAULT '',
				last_used_ip TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				FOREIGN KEY(user_id) REFERENCES users(id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id)`,
			`CREATE TABLE IF NOT EXISTS credit_transactions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				type TEXT NOT NULL,
				amount INTEGER NOT NULL,
				balance_before INTEGER NOT NULL,
				balance_after INTEGER NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				request_id TEXT NOT NULL DEFAULT '',
				route_id TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				FOREIGN KEY(user_id) REFERENCES users(id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_credit_transactions_user_id ON credit_transactions(user_id)`,
		},
	},
	{
		Version: 2,
		Name:    "endpoint_permissions",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS endpoint_permissions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				route_id TEXT NOT NULL,
				methods TEXT NOT NULL DEFAULT '[]',
				allowed INTEGER NOT NULL DEFAULT 1,
				rate_limits TEXT NOT NULL DEFAULT '{}',
				credit_cost TEXT NOT NULL DEFAULT '',
				valid_from TEXT NOT NULL DEFAULT '',
				valid_until TEXT NOT NULL DEFAULT '',
				allowed_days TEXT NOT NULL DEFAULT '[]',
				allowed_hours TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				FOREIGN KEY(user_id) REFERENCES users(id),
				UNIQUE(user_id, route_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_endpoint_permissions_user_id ON endpoint_permissions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_endpoint_permissions_route_id ON endpoint_permissions(route_id)`,
		},
	},
	{
		Version: 3,
		Name:    "audit_logs",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS audit_logs (
				id TEXT PRIMARY KEY,
				request_id TEXT NOT NULL DEFAULT '',
				route_id TEXT NOT NULL DEFAULT '',
				route_name TEXT NOT NULL DEFAULT '',
				service_name TEXT NOT NULL DEFAULT '',
				user_id TEXT NOT NULL DEFAULT '',
				consumer_name TEXT NOT NULL DEFAULT '',
				method TEXT NOT NULL DEFAULT '',
				host TEXT NOT NULL DEFAULT '',
				path TEXT NOT NULL DEFAULT '',
				query TEXT NOT NULL DEFAULT '',
				status_code INTEGER NOT NULL DEFAULT 0,
				latency_ms INTEGER NOT NULL DEFAULT 0,
				bytes_in INTEGER NOT NULL DEFAULT 0,
				bytes_out INTEGER NOT NULL DEFAULT 0,
				client_ip TEXT NOT NULL DEFAULT '',
				user_agent TEXT NOT NULL DEFAULT '',
				blocked INTEGER NOT NULL DEFAULT 0,
				block_reason TEXT NOT NULL DEFAULT '',
				request_headers TEXT NOT NULL DEFAULT '{}',
				request_body TEXT NOT NULL DEFAULT '',
				response_headers TEXT NOT NULL DEFAULT '{}',
				response_body TEXT NOT NULL DEFAULT '',
				error_message TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_route_id ON audit_logs(route_id)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_logs_status_code ON audit_logs(status_code)`,
		},
	},
	{
		Version: 4,
		Name:    "portal_sessions",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				token_hash TEXT NOT NULL UNIQUE,
				user_agent TEXT NOT NULL DEFAULT '',
				client_ip TEXT NOT NULL DEFAULT '',
				expires_at TEXT NOT NULL,
				last_seen_at TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				FOREIGN KEY(user_id) REFERENCES users(id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		},
	},
	{
		Version: 5,
		Name:    "playground_templates",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS playground_templates (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				name TEXT NOT NULL,
				method TEXT NOT NULL DEFAULT 'GET',
				path TEXT NOT NULL DEFAULT '/',
				headers TEXT NOT NULL DEFAULT '{}',
				query_params TEXT NOT NULL DEFAULT '{}',
				body TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				FOREIGN KEY(user_id) REFERENCES users(id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_playground_templates_user_id ON playground_templates(user_id)`,
		},
	},
	{
		Version: 6,
		Name:    "webhooks",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS webhooks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				url TEXT NOT NULL,
				secret TEXT NOT NULL DEFAULT '',
				events TEXT NOT NULL DEFAULT '[]',
				headers TEXT NOT NULL DEFAULT '{}',
				active INTEGER NOT NULL DEFAULT 1,
				retry_count INTEGER NOT NULL DEFAULT 3,
				retry_interval INTEGER NOT NULL DEFAULT 60,
				timeout INTEGER NOT NULL DEFAULT 30,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				last_triggered TEXT NOT NULL DEFAULT ''
			)`,
			`CREATE TABLE IF NOT EXISTS webhook_deliveries (
				id TEXT PRIMARY KEY,
				webhook_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				payload TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				status_code INTEGER NOT NULL DEFAULT 0,
				response TEXT NOT NULL DEFAULT '',
				error TEXT NOT NULL DEFAULT '',
				attempt INTEGER NOT NULL DEFAULT 0,
				max_attempts INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				completed_at TEXT NOT NULL DEFAULT '',
				FOREIGN KEY(webhook_id) REFERENCES webhooks(id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id)`,
			`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status)`,
		},
	},
	{
		Version: 7,
		Name:    "audit_fts5",
		Statements: []string{
			`CREATE VIRTUAL TABLE IF NOT EXISTS audit_logs_fts USING fts5(path, request_body, response_body, content=audit_logs, content_rowid=rowid)`,
			// Triggers to keep FTS5 in sync with audit_logs
			`CREATE TRIGGER IF NOT EXISTS audit_logs_fts_insert AFTER INSERT ON audit_logs BEGIN
				INSERT INTO audit_logs_fts(rowid, path, request_body, response_body) VALUES (new.rowid, new.path, new.request_body, new.response_body);
			END`,
			`CREATE TRIGGER IF NOT EXISTS audit_logs_fts_delete AFTER DELETE ON audit_logs BEGIN
				INSERT INTO audit_logs_fts(audit_logs_fts, rowid, path, request_body, response_body) VALUES('delete', old.rowid, old.path, old.request_body, old.response_body);
			END`,
			`CREATE TRIGGER IF NOT EXISTS audit_logs_fts_update AFTER UPDATE ON audit_logs BEGIN
				INSERT INTO audit_logs_fts(audit_logs_fts, rowid, path, request_body, response_body) VALUES('delete', old.rowid, old.path, old.request_body, old.response_body);
				INSERT INTO audit_logs_fts(rowid, path, request_body, response_body) VALUES (new.rowid, new.path, new.request_body, new.response_body);
			END`,
			// Rebuild FTS5 index from existing audit_logs data
			`INSERT INTO audit_logs_fts(audit_logs_fts) VALUES('rebuild')`,
		},
	},
}

func Open(cfg *config.Config) (*Store, error) {
	effective := resolveStoreConfig(cfg)
	if err := validateStoreConfig(effective); err != nil {
		return nil, err
	}

	registerDriver()

	db, err := sql.Open(sqliteDriverName, effective.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	maxOpenConns := effective.MaxOpenConns
	if maxOpenConns <= 0 {
		maxOpenConns = 25
	}
	// In-memory SQLite databases are per-connection; force a single
	// connection so schema and data are visible across the pool.
	if strings.TrimSpace(effective.Path) == ":memory:" {
		maxOpenConns = 1
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	db.SetConnMaxLifetime(0)

	s := &Store{
		db:  db,
		cfg: effective,
	}

	if err := s.applyPragmas(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.ensureInitialAdminUser(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	return migrations.Migrate(s.db, migrationsList)
}

// MigrationStatus returns applied and pending migrations.
func (s *Store) MigrationStatus() ([]migrations.Migration, []migrations.Migration, error) {
	if s == nil || s.db == nil {
		return nil, nil, errors.New("store is not initialized")
	}
	return migrations.Status(s.db, migrationsList)
}

func (s *Store) applyPragmas() error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}

	journalMode := strings.ToUpper(strings.TrimSpace(s.cfg.JournalMode))
	if journalMode == "" {
		journalMode = "WAL"
	}

	busyMS := int(s.cfg.BusyTimeout / time.Millisecond)
	if busyMS <= 0 {
		busyMS = 5000
	}

	synchronous := strings.ToUpper(strings.TrimSpace(s.cfg.Synchronous))
	if synchronous == "" {
		// NORMAL is safe with WAL mode — same durability as FULL for committed
		// transactions, but avoids fsync on every write, dramatically improving
		// throughput for audit inserts and billing deductions.
		synchronous = "NORMAL"
	}

	cacheSize := s.cfg.CacheSize
	if cacheSize == 0 {
		cacheSize = -64000 // 64 MB page cache
	}

	walAutoCheckpoint := s.cfg.WALAutoCheckpoint
	if walAutoCheckpoint == 0 {
		walAutoCheckpoint = 5000
	}

	foreignKeys := "OFF"
	if s.cfg.ForeignKeys {
		foreignKeys = "ON"
	}

	pragmas := []string{
		fmt.Sprintf("PRAGMA journal_mode = %s", journalMode),
		fmt.Sprintf("PRAGMA busy_timeout = %d", busyMS),
		fmt.Sprintf("PRAGMA synchronous = %s", synchronous),
		fmt.Sprintf("PRAGMA cache_size = %d", cacheSize),
		fmt.Sprintf("PRAGMA wal_autocheckpoint = %d", walAutoCheckpoint),
		"PRAGMA temp_store = MEMORY",
		fmt.Sprintf("PRAGMA foreign_keys = %s", foreignKeys),
	}

	for _, p := range pragmas {
		if _, err := s.db.Exec(p); err != nil {
			return fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}

	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("ping sqlite db: %w", err)
	}
	return nil
}

func resolveStoreConfig(cfg *config.Config) config.StoreConfig {
	out := config.StoreConfig{
		Path:              "apicerberus.db",
		BusyTimeout:       5 * time.Second,
		JournalMode:       "WAL",
		ForeignKeys:       true,
		MaxOpenConns:      25,
		Synchronous:       "NORMAL",
		WALAutoCheckpoint: 5000,
		CacheSize:         -64000,
	}
	if cfg == nil {
		return out
	}
	if v := strings.TrimSpace(cfg.Store.Path); v != "" {
		out.Path = v
	}
	if cfg.Store.BusyTimeout > 0 {
		out.BusyTimeout = cfg.Store.BusyTimeout
	}
	if v := strings.TrimSpace(cfg.Store.JournalMode); v != "" {
		out.JournalMode = strings.ToUpper(v)
	}
	if cfg.Store.ForeignKeys {
		out.ForeignKeys = true
	}
	if cfg.Store.MaxOpenConns > 0 {
		out.MaxOpenConns = cfg.Store.MaxOpenConns
	}
	if v := strings.TrimSpace(cfg.Store.Synchronous); v != "" {
		out.Synchronous = strings.ToUpper(v)
	}
	if cfg.Store.WALAutoCheckpoint > 0 {
		out.WALAutoCheckpoint = cfg.Store.WALAutoCheckpoint
	}
	if cfg.Store.CacheSize != 0 {
		out.CacheSize = cfg.Store.CacheSize
	}
	return out
}

func validateStoreConfig(cfg config.StoreConfig) error {
	if strings.TrimSpace(cfg.Path) == "" {
		return errors.New("store.path is required")
	}
	if cfg.BusyTimeout < 0 {
		return errors.New("store.busy_timeout cannot be negative")
	}

	mode := strings.ToUpper(strings.TrimSpace(cfg.JournalMode))
	switch mode {
	case "WAL", "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "OFF":
	default:
		return fmt.Errorf("store.journal_mode %q is invalid", cfg.JournalMode)
	}
	return nil
}
