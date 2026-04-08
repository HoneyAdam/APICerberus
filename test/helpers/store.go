// Package testhelpers provides utilities for testing APICerebrus.
package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// MockStore wraps a Store with test-specific utilities.
type MockStore struct {
	*store.Store
	dbPath string
	t      testing.TB
}

// NewMockStore creates a new in-memory SQLite store for testing.
// The store is automatically cleaned up when the test completes.
func NewMockStore(t testing.TB) *MockStore {
	t.Helper()

	// Create a temporary directory for the test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        dbPath,
			BusyTimeout: 5 * time.Second,
			JournalMode: "DELETE", // Use DELETE mode for tests to avoid WAL files
			ForeignKeys: true,
		},
	}

	s, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to create mock store: %v", err)
	}

	ms := &MockStore{
		Store:  s,
		dbPath: dbPath,
		t:      t,
	}

	// Register cleanup to close the store after test
	t.Cleanup(func() {
		ms.Cleanup()
	})

	return ms
}

// NewMockStoreWithData creates a mock store pre-populated with test data.
// It creates a default admin user, test users, API keys, and other common test fixtures.
func NewMockStoreWithData(t testing.TB) *MockStore {
	t.Helper()

	ms := NewMockStore(t)

	// Create test users
	_ = context.Background() // For future use

	// Create a regular test user
	testUser := &store.User{
		ID:            "user-test-001",
		Email:         "test@example.com",
		Name:          "Test User",
		Company:       "Test Company",
		PasswordHash:  "$2a$10$testhash", // bcrypt hash placeholder
		Role:          "user",
		Status:        "active",
		CreditBalance: 1000,
		RateLimits:    map[string]any{"default": 100},
		IPWhitelist:   []string{},
		Metadata:      map[string]any{"test": true},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := ms.Users().Create(testUser); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create an admin user
	adminUser := &store.User{
		ID:            "user-admin-001",
		Email:         "admin@example.com",
		Name:          "Admin User",
		Company:       "Admin Company",
		PasswordHash:  "$2a$10$adminhash",
		Role:          "admin",
		Status:        "active",
		CreditBalance: 5000,
		RateLimits:    map[string]any{"default": 1000},
		IPWhitelist:   []string{},
		Metadata:      map[string]any{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := ms.Users().Create(adminUser); err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Create a suspended user for testing access control
	suspendedUser := &store.User{
		ID:            "user-suspended-001",
		Email:         "suspended@example.com",
		Name:          "Suspended User",
		Company:       "Test Company",
		PasswordHash:  "$2a$10$suspendedhash",
		Role:          "user",
		Status:        "suspended",
		CreditBalance: 0,
		RateLimits:    map[string]any{},
		IPWhitelist:   []string{},
		Metadata:      map[string]any{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := ms.Users().Create(suspendedUser); err != nil {
		t.Fatalf("Failed to create suspended user: %v", err)
	}

	// Create API keys for the test user
	_, _, err := ms.APIKeys().Create(testUser.ID, "Test Key 1", "live")
	if err != nil {
		t.Fatalf("Failed to create test API key: %v", err)
	}

	_, _, err = ms.APIKeys().Create(testUser.ID, "Test Key 2", "test")
	if err != nil {
		t.Fatalf("Failed to create test API key: %v", err)
	}

	return ms
}

// Cleanup closes the store and removes the database file.
// This is automatically called via t.Cleanup, but can be called manually if needed.
func (ms *MockStore) Cleanup() {
	if ms.Store != nil {
		if err := ms.Store.Close(); err != nil {
			// Log but don't fail - we're in cleanup
			fmt.Fprintf(os.Stderr, "Warning: failed to close mock store: %v\n", err)
		}
		ms.Store = nil
	}
}

// DB returns the underlying sql.DB for direct database operations.
func (ms *MockStore) DB() *sql.DB {
	if ms.Store == nil {
		ms.t.Fatal("MockStore has been cleaned up")
	}
	return ms.Store.DB()
}

// MustCreateUser creates a user and fails the test if creation fails.
func (ms *MockStore) MustCreateUser(user *store.User) *store.User {
	ms.t.Helper()
	if err := ms.Users().Create(user); err != nil {
		ms.t.Fatalf("Failed to create user %s: %v", user.Email, err)
	}
	return user
}

// MustCreateAPIKey creates an API key and fails the test if creation fails.
// Returns the full API key string and the APIKey struct.
func (ms *MockStore) MustCreateAPIKey(userID, name, mode string) (string, *store.APIKey) {
	ms.t.Helper()
	fullKey, key, err := ms.APIKeys().Create(userID, name, mode)
	if err != nil {
		ms.t.Fatalf("Failed to create API key for user %s: %v", userID, err)
	}
	return fullKey, key
}

// MustFindUserByEmail finds a user by email and fails the test if not found.
func (ms *MockStore) MustFindUserByEmail(email string) *store.User {
	ms.t.Helper()
	user, err := ms.Users().FindByEmail(email)
	if err != nil {
		ms.t.Fatalf("Failed to find user by email %s: %v", email, err)
	}
	if user == nil {
		ms.t.Fatalf("User not found: %s", email)
	}
	return user
}

// MustFindUserByID finds a user by ID and fails the test if not found.
func (ms *MockStore) MustFindUserByID(id string) *store.User {
	ms.t.Helper()
	user, err := ms.Users().FindByID(id)
	if err != nil {
		ms.t.Fatalf("Failed to find user by ID %s: %v", id, err)
	}
	if user == nil {
		ms.t.Fatalf("User not found: %s", id)
	}
	return user
}

// MustResolveAPIKey resolves an API key and fails the test if resolution fails.
func (ms *MockStore) MustResolveAPIKey(rawKey string) (*store.User, *store.APIKey) {
	ms.t.Helper()
	user, key, err := ms.APIKeys().ResolveUserByRawKey(rawKey)
	if err != nil {
		ms.t.Fatalf("Failed to resolve API key: %v", err)
	}
	if user == nil || key == nil {
		ms.t.Fatal("API key resolved to nil user or key")
	}
	return user, key
}

// Exec executes a SQL statement directly on the database.
// Useful for setting up specific test scenarios.
func (ms *MockStore) Exec(query string, args ...any) sql.Result {
	ms.t.Helper()
	result, err := ms.DB().Exec(query, args...)
	if err != nil {
		ms.t.Fatalf("Failed to execute query: %v", err)
	}
	return result
}

// QueryRow executes a query that returns a single row.
func (ms *MockStore) QueryRow(query string, args ...any) *sql.Row {
	return ms.DB().QueryRow(query, args...)
}

// Query executes a query that returns multiple rows.
func (ms *MockStore) Query(query string, args ...any) *sql.Rows {
	ms.t.Helper()
	rows, err := ms.DB().Query(query, args...)
	if err != nil {
		ms.t.Fatalf("Failed to execute query: %v", err)
	}
	return rows
}

// Count returns the count of rows in a table matching the condition.
func (ms *MockStore) Count(table, where string, args ...any) int {
	ms.t.Helper()
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if where != "" {
		query += " WHERE " + where
	}
	var count int
	if err := ms.DB().QueryRow(query, args...).Scan(&count); err != nil {
		ms.t.Fatalf("Failed to count rows: %v", err)
	}
	return count
}

// Truncate removes all data from a table. Use with caution!
func (ms *MockStore) Truncate(tables ...string) {
	ms.t.Helper()
	for _, table := range tables {
		if _, err := ms.DB().Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			ms.t.Fatalf("Failed to truncate table %s: %v", table, err)
		}
	}
}

// WithTransaction executes a function within a database transaction.
// The transaction is committed if the function returns nil, otherwise rolled back.
func (ms *MockStore) WithTransaction(fn func(*sql.Tx) error) error {
	ms.t.Helper()
	tx, err := ms.DB().BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// AssertRowExists asserts that a row exists matching the given condition.
func (ms *MockStore) AssertRowExists(table, where string, args ...any) {
	ms.t.Helper()
	count := ms.Count(table, where, args...)
	if count == 0 {
		ms.t.Errorf("Expected row to exist in %s where %s", table, where)
	}
}

// AssertRowCount asserts the exact count of rows in a table.
func (ms *MockStore) AssertRowCount(table string, expected int) {
	ms.t.Helper()
	actual := ms.Count(table, "")
	if actual != expected {
		ms.t.Errorf("Expected %d rows in %s, got %d", expected, table, actual)
	}
}

// AssertUserExists asserts that a user with the given email exists.
func (ms *MockStore) AssertUserExists(email string) {
	ms.t.Helper()
	user, err := ms.Users().FindByEmail(email)
	if err != nil {
		ms.t.Errorf("Error finding user %s: %v", email, err)
		return
	}
	if user == nil {
		ms.t.Errorf("Expected user %s to exist", email)
	}
}

// AssertUserNotExists asserts that a user with the given email does not exist.
func (ms *MockStore) AssertUserNotExists(email string) {
	ms.t.Helper()
	user, err := ms.Users().FindByEmail(email)
	if err != nil {
		ms.t.Errorf("Error finding user %s: %v", email, err)
		return
	}
	if user != nil {
		ms.t.Errorf("Expected user %s to not exist", email)
	}
}
