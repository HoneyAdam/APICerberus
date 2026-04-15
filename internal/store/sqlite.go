package store

import (
	"context"
	"database/sql"
	"time"
)

// SQLiteDB wraps a *sql.DB to implement the DB interface for SQLite.
type SQLiteDB struct {
	db *sql.DB
}

// NewSQLiteDB creates a new SQLite-backed DB implementation.
func NewSQLiteDB(db *sql.DB) *SQLiteDB {
	return &SQLiteDB{db: db}
}

// QueryContext executes a query that returns rows.
func (s *SQLiteDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns at most one row.
func (s *SQLiteDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a query that doesn't return rows.
func (s *SQLiteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

// BeginTx starts a new transaction.
func (s *SQLiteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, opts)
}

// PingContext checks if the database is reachable.
func (s *SQLiteDB) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the underlying database connection.
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// Underlying returns the underlying *sql.DB for SQLite-specific operations.
func (s *SQLiteDB) Underlying() *sql.DB {
	return s.db
}

// SQLiteTx wraps sql.Tx to implement the Tx interface.
type SQLiteTx struct {
	tx *sql.Tx
}

// QueryContext implements the Tx interface.
func (s *SQLiteTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext implements the Tx interface.
func (s *SQLiteTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.tx.QueryRowContext(ctx, query, args...)
}

// ExecContext implements the Tx interface.
func (s *SQLiteTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.tx.ExecContext(ctx, query, args...)
}

// Commit commits the transaction.
func (s *SQLiteTx) Commit() error {
	return s.tx.Commit()
}

// Rollback rolls back the transaction.
func (s *SQLiteTx) Rollback() error {
	return s.tx.Rollback()
}

// BusyRetryConfig holds retry configuration for SQLite busy handling.
var BusyRetryConfig = struct {
	MaxAttempts int
	Delay       time.Duration
}{
	MaxAttempts: 5,
	Delay:        50 * time.Millisecond,
}