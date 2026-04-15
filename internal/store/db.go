package store

import (
	"context"
	"database/sql"
	"time"
)

// DB is the database interface used by all repositories.
// It abstracts away the underlying DB driver (SQLite or PostgreSQL).
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	PingContext(ctx context.Context) error
	Close() error
}

// Tx is a transaction interface for repository methods that need transactions.
type Tx interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

// DBFunc is a function that takes a DB and returns a repository.
type DBFunc func(DB) any

// Executor is a interface for types that can execute queries.
type Executor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// transaction executes fn within a transaction.
// If fn returns an error, the transaction is rolled back.
// If fn succeeds, the transaction is committed.
func transaction(ctx context.Context, db DB, fn func(Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// retryOnBusy retries a function if the database is busy (SQLite only).
func retryOnBusy(maxAttempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		// Check if it's a busy error
		if !isBusyError(err) {
			return err
		}
		time.Sleep(delay)
	}
	return err
}

// isBusyError returns true if the error is a database busy error.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite busy handling
	if err.Error() == "database is locked" {
		return true
	}
	if err.Error() == "SQLITE_BUSY" {
		return true
	}
	return false
}

// CommonColumn constants for audit logs and other shared fields.
const (
	ColumnID        = "id"
	ColumnCreatedAt = "created_at"
	ColumnUpdatedAt = "updated_at"
)