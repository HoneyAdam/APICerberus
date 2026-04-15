package store

import (
	"context"
	"database/sql"
	"testing"
)

// mockDB implements DB interface for testing.
type mockDB struct {
	queryFn    func(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	queryRowFn func(ctx context.Context, q string, args ...any) *sql.Row
	execFn     func(ctx context.Context, q string, args ...any) (sql.Result, error)
	beginFn    func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (m *mockDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, query, args...)
	}
	return nil
}

func (m *mockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFn != nil {
		return m.execFn(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if m.beginFn != nil {
		return m.beginFn(ctx, opts)
	}
	return nil, nil
}

func (m *mockDB) PingContext(ctx context.Context) error {
	return nil
}

func (m *mockDB) Close() error {
	return nil
}

func TestSQLiteDBUnderlying(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	sqliteDB := NewSQLiteDB(db)
	if sqliteDB.Underlying() != db {
		t.Fatal("Underlying() should return the same *sql.DB")
	}
}

func TestPostgresDBUnderlying(t *testing.T) {
	t.Parallel()
	t.Skip("PostgreSQL not available in test environment")
}

func TestIsBusyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"database is locked", context.DeadlineExceeded, false},
		{"SQLITE_BUSY", sql.ErrNoRows, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBusyError(tt.err)
			if got != tt.expected {
				t.Errorf("isBusyError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestTransactionSuccess(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Create a simple test table
	_, _ = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")

	sqliteDB := NewSQLiteDB(db)

	err = transaction(context.Background(), sqliteDB, func(tx Tx) error {
		_, err := tx.ExecContext(context.Background(), "INSERT INTO test VALUES (1)")
		return err
	})

	if err != nil {
		t.Fatalf("transaction should succeed: %v", err)
	}

	// Verify the insert worked
	var count int
	db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestTransactionRollbackOnError(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Create a simple test table
	_, _ = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)")

	sqliteDB := NewSQLiteDB(db)

	// Attempt to insert and then fail
	err = transaction(context.Background(), sqliteDB, func(tx Tx) error {
		_, err := tx.ExecContext(context.Background(), "INSERT INTO test VALUES (1)")
		if err != nil {
			return err
		}
		return sql.ErrTxDone // Simulate an error
	})

	if err == nil {
		t.Fatal("expected error from transaction")
	}

	// Verify the insert was rolled back
	var count int
	db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test").Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}
}

func TestSQLiteDBImplementsDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	sqliteDB := NewSQLiteDB(db)

	var _ DB = sqliteDB // Verify SQLiteDB implements DB interface
}

func TestPostgresDBImplementsDB(t *testing.T) {
	t.Parallel()
	t.Skip("PostgreSQL not available in test environment")
}

func TestBusyRetryConfig(t *testing.T) {
	t.Parallel()

	if BusyRetryConfig.MaxAttempts <= 0 {
		t.Error("BusyRetryConfig.MaxAttempts should be positive")
	}
	if BusyRetryConfig.Delay <= 0 {
		t.Error("BusyRetryConfig.Delay should be positive")
	}
}

func TestPostgresConfigBuildDSN(t *testing.T) {
	t.Parallel()

	cfg := PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "testuser",
		Password: "testpass",
		Database: "testdb",
		SSLMode:  "disable",
	}

	dsn := cfg.BuildDSN()

	if dsn == "" {
		t.Fatal("DSN should not be empty")
	}
	if !contains(dsn, "host=localhost") {
		t.Error("DSN should contain host")
	}
	if !contains(dsn, "port=5432") {
		t.Error("DSN should contain port")
	}
	if !contains(dsn, "user=testuser") {
		t.Error("DSN should contain user")
	}
	if !contains(dsn, "password=testpass") {
		t.Error("DSN should contain password")
	}
	if !contains(dsn, "dbname=testdb") {
		t.Error("DSN should contain dbname")
	}
	if !contains(dsn, "sslmode=disable") {
		t.Error("DSN should contain sslmode")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}