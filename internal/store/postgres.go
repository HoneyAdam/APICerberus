package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// PostgresDB wraps a *sql.DB to implement the DB interface for PostgreSQL.
// It uses the pgx stdlib driver for database/sql compatibility.
type PostgresDB struct {
	db *sql.DB
}

// NewPostgresDB creates a new PostgreSQL-backed DB implementation.
func NewPostgresDB(db *sql.DB) *PostgresDB {
	return &PostgresDB{db: db}
}

// QueryContext executes a query that returns rows.
func (p *PostgresDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns at most one row.
func (p *PostgresDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a query that doesn't return rows.
func (p *PostgresDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

// BeginTx starts a new transaction.
func (p *PostgresDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return p.db.BeginTx(ctx, opts)
}

// PingContext checks if the database is reachable.
func (p *PostgresDB) PingContext(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// Close closes the underlying database connection.
func (p *PostgresDB) Close() error {
	return p.db.Close()
}

// Underlying returns the underlying *sql.DB for PostgreSQL-specific operations.
func (p *PostgresDB) Underlying() *sql.DB {
	return p.db
}

// PostgresConfig holds PostgreSQL connection configuration.
type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
	MaxConns int
}

// BuildDSN builds a PostgreSQL connection string from the config.
func (p *PostgresConfig) BuildDSN() string {
	connStr := fmt.Sprintf("host=%s port=%d user=%s dbname=%s",
		p.Host, p.Port, p.User, p.Database)

	if p.Password != "" {
		connStr += fmt.Sprintf(" password=%s", url.QueryEscape(p.Password))
	}

	sslMode := p.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connStr += " sslmode=" + sslMode

	return connStr
}

// OpenPostgres opens a new PostgreSQL connection pool using pgx stdlib driver.
func OpenPostgres(ctx context.Context, cfg PostgresConfig) (*PostgresDB, error) {
	dsn := cfg.BuildDSN()

	// Register pgx as a database/sql driver
	// The pgx driver is registered under "pgx" name
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	maxConns := cfg.MaxConns
	if maxConns == 0 {
		maxConns = 25
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return NewPostgresDB(db), nil
}

// RetryOnTransientError retries a function on transient PostgreSQL errors.
func RetryOnTransientError(maxAttempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isTransientError(err) {
			return err
		}
		time.Sleep(delay)
	}
	return err
}

// isTransientError returns true if the error is a transient PostgreSQL error.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Common transient PostgreSQL errors
	transientCodes := []string{
		"connection refused",
		"connection timed out",
		"deadlock detected",
		"too many clients",
		"remaining connection slots",
	}
	for _, code := range transientCodes {
		if strings.Contains(errStr, code) {
			return true
		}
	}
	return false
}

// PgSearch enables full-text search using PostgreSQL tsvector.
// Usage in query: WHERE to_tsvector('english', path || ' ' || request_body || ' ' || response_body) @@ plainto_tsquery('english', $1)
const PgSearchQueryTpl = "to_tsvector('english', %s) @@ plainto_tsquery('english', $%d)"

// PostgresTx wraps sql.Tx to implement the Tx interface.
type PostgresTx struct {
	tx *sql.Tx
}

// QueryContext implements the Tx interface.
func (p *PostgresTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext implements the Tx interface.
func (p *PostgresTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.tx.QueryRowContext(ctx, query, args...)
}

// ExecContext implements the Tx interface.
func (p *PostgresTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.tx.ExecContext(ctx, query, args...)
}

// Commit commits the transaction.
func (p *PostgresTx) Commit() error {
	return p.tx.Commit()
}

// Rollback rolls back the transaction.
func (p *PostgresTx) Rollback() error {
	return p.tx.Rollback()
}