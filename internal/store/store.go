package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

const sqliteDriverName = "apicerberus-sqlite"

type Store struct {
	db  *sql.DB
	cfg config.StoreConfig
}

type Migration struct {
	Version    int
	Name       string
	Statements []string
}

var migrations = []Migration{
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
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
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
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, migration := range migrations {
		applied, err := s.isMigrationApplied(migration.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}

		for _, stmt := range migration.Statements {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply migration %d (%s): %w", migration.Version, migration.Name, err)
			}
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)`,
			migration.Version,
			migration.Name,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

func (s *Store) isMigrationApplied(version int) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&one)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("check migration %d: %w", version, err)
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

	if _, err := s.db.Exec(fmt.Sprintf("PRAGMA journal_mode = %s", journalMode)); err != nil {
		return fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err := s.db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", busyMS)); err != nil {
		return fmt.Errorf("set busy_timeout: %w", err)
	}
	foreignKeys := "OFF"
	if s.cfg.ForeignKeys {
		foreignKeys = "ON"
	}
	if _, err := s.db.Exec(fmt.Sprintf("PRAGMA foreign_keys = %s", foreignKeys)); err != nil {
		return fmt.Errorf("set foreign_keys: %w", err)
	}

	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("ping sqlite db: %w", err)
	}
	return nil
}

func resolveStoreConfig(cfg *config.Config) config.StoreConfig {
	out := config.StoreConfig{
		Path:        "apicerberus.db",
		BusyTimeout: 5 * time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
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
