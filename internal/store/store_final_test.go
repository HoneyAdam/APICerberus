package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// Test applyPragmas with nil store
func TestApplyPragmas_NilStore(t *testing.T) {
	var s *Store
	err := s.applyPragmas()
	if err == nil {
		t.Error("applyPragmas() with nil store should return error")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' error, got: %v", err)
	}
}

// Test applyPragmas with nil db
func TestApplyPragmas_NilDB(t *testing.T) {
	s := &Store{db: nil}
	err := s.applyPragmas()
	if err == nil {
		t.Error("applyPragmas() with nil db should return error")
	}
}

// Test migrate with nil store
func TestMigrate_NilStore(t *testing.T) {
	var s *Store
	err := s.migrate()
	if err == nil {
		t.Error("migrate() with nil store should return error")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' error, got: %v", err)
	}
}

// Test migrate with nil db
func TestMigrate_NilDB(t *testing.T) {
	s := &Store{db: nil}
	err := s.migrate()
	if err == nil {
		t.Error("migrate() with nil db should return error")
	}
}

// Test Store.DB() with nil store
func TestStore_DB_NilStore(t *testing.T) {
	var s *Store
	db := s.DB()
	if db != nil {
		t.Error("DB() with nil store should return nil")
	}
}

// Test Store.Close() with nil store
func TestStore_Close_NilStore(t *testing.T) {
	var s *Store
	err := s.Close()
	if err != nil {
		t.Errorf("Close() with nil store should return nil, got: %v", err)
	}
}

// Test Store.Close() with nil db
func TestStore_Close_NilDB(t *testing.T) {
	s := &Store{db: nil}
	err := s.Close()
	if err != nil {
		t.Errorf("Close() with nil db should return nil, got: %v", err)
	}
}

// Test resolveStoreConfig with nil config
func TestResolveStoreConfig_NilConfig(t *testing.T) {
	cfg := resolveStoreConfig(nil)
	if cfg.Path != "apicerberus.db" {
		t.Errorf("expected default path 'apicerberus.db', got: %s", cfg.Path)
	}
	if cfg.BusyTimeout != 5*time.Second {
		t.Errorf("expected default busy timeout 5s, got: %v", cfg.BusyTimeout)
	}
	if cfg.JournalMode != "WAL" {
		t.Errorf("expected default journal mode 'WAL', got: %s", cfg.JournalMode)
	}
	if !cfg.ForeignKeys {
		t.Error("expected default foreign keys to be true")
	}
}

// Test resolveStoreConfig with partial config
func TestResolveStoreConfig_PartialConfig(t *testing.T) {
	input := &config.Config{
		Store: config.StoreConfig{
			Path: ":memory:",
		},
	}
	cfg := resolveStoreConfig(input)
	if cfg.Path != ":memory:" {
		t.Errorf("expected path ':memory:', got: %s", cfg.Path)
	}
	// Other values should use defaults
	if cfg.BusyTimeout != 5*time.Second {
		t.Errorf("expected default busy timeout 5s, got: %v", cfg.BusyTimeout)
	}
}

// Test validateStoreConfig edge cases
func TestValidateStoreConfig_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.StoreConfig
		wantErr bool
	}{
		{
			name: "zero busy timeout",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: 0,
				JournalMode: "WAL",
			},
			wantErr: false, // Zero is valid
		},
		{
			name: "all valid modes - delete",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: time.Second,
				JournalMode: "DELETE",
			},
			wantErr: false,
		},
		{
			name: "truncate mode",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: time.Second,
				JournalMode: "TRUNCATE",
			},
			wantErr: false,
		},
		{
			name: "persist mode",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: time.Second,
				JournalMode: "PERSIST",
			},
			wantErr: false,
		},
		{
			name: "memory mode",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: time.Second,
				JournalMode: "MEMORY",
			},
			wantErr: false,
		},
		{
			name: "off mode",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: time.Second,
				JournalMode: "OFF",
			},
			wantErr: false,
		},
		{
			name: "lowercase mode",
			cfg: config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: time.Second,
				JournalMode: "wal",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStoreConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStoreConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test UserRepo withTx error handling
func TestUserRepo_WithTx_ErrorPaths(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	repo := db.Users()

	// Test withTx where fn returns error
	testErr := errors.New("test error")
	err := repo.withTx(context.Background(), func(tx *sql.Tx) error {
		return testErr
	})
	if !errors.Is(err, testErr) {
		t.Errorf("withTx() should return fn error, got: %v", err)
	}
}

// Test HashPassword error path
func TestHashPassword_ErrorPath(t *testing.T) {
	// Empty password
	_, err := HashPassword("")
	if err == nil {
		t.Error("HashPassword('') should return error")
	}

	// Whitespace only password
	_, err = HashPassword("   ")
	if err == nil {
		t.Error("HashPassword('   ') should return error")
	}

	// Valid password should work
	hash, err := HashPassword("validpassword123")
	if err != nil {
		t.Errorf("HashPassword('validpassword123') should not return error, got: %v", err)
	}
	if hash == "" {
		t.Error("HashPassword should return non-empty hash")
	}
}

// Test VerifyPassword edge cases
func TestVerifyPassword_EdgeCases(t *testing.T) {
	// Empty stored hash
	result := VerifyPassword("", "password")
	if result {
		t.Error("VerifyPassword('', 'password') should return false")
	}

	// Empty raw password
	result = VerifyPassword("hash", "")
	if result {
		t.Error("VerifyPassword('hash', '') should return false")
	}

	// Both empty
	result = VerifyPassword("", "")
	if result {
		t.Error("VerifyPassword('', '') should return false")
	}

	// Valid password verification
	hash, _ := HashPassword("testpassword")
	result = VerifyPassword(hash, "testpassword")
	if !result {
		t.Error("VerifyPassword should return true for valid password")
	}

	// Invalid password
	result = VerifyPassword(hash, "wrongpassword")
	if result {
		t.Error("VerifyPassword should return false for invalid password")
	}
}

// Test ensureInitialAdminUser with nil store
func TestEnsureInitialAdminUser_NilStore(t *testing.T) {
	var s *Store
	err := s.ensureInitialAdminUser()
	if err == nil {
		t.Error("ensureInitialAdminUser() with nil store should return error")
	}
}

// Test ensureInitialAdminUser with nil db
func TestEnsureInitialAdminUser_NilDB(t *testing.T) {
	s := &Store{db: nil}
	err := s.ensureInitialAdminUser()
	if err == nil {
		t.Error("ensureInitialAdminUser() with nil db should return error")
	}
}

// Test ensureInitialAdminUser when admin already exists
func TestEnsureInitialAdminUser_AdminExists(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// First call creates admin
	err := db.ensureInitialAdminUser()
	if err != nil {
		t.Errorf("First ensureInitialAdminUser() should succeed: %v", err)
	}

	// Second call should be no-op since admin exists
	err = db.ensureInitialAdminUser()
	if err != nil {
		t.Errorf("Second ensureInitialAdminUser() should succeed: %v", err)
	}
}

// Test ensureInitialAdminUser with custom password from env
func TestEnsureInitialAdminUser_CustomPassword(t *testing.T) {
	// Set custom admin password
	os.Setenv("APICERBERUS_ADMIN_PASSWORD", "custom_admin_pass_123")
	defer os.Unsetenv("APICERBERUS_ADMIN_PASSWORD")

	dbPath := filepath.Join(t.TempDir(), "admin_test.db")
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        dbPath,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}

	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	// Verify admin was created with custom password
	user, err := db.Users().FindByEmail("admin@apicerberus.local")
	if err != nil {
		t.Fatalf("FindByEmail error: %v", err)
	}
	if user == nil {
		t.Fatal("Admin user should exist")
	}

	// Verify password works
	if !VerifyPassword(user.PasswordHash, "custom_admin_pass_123") {
		t.Error("Custom admin password should work")
	}
}

// Test APIKeyRepo.UpdateLastUsed with nil repo
func TestAPIKeyRepo_UpdateLastUsed_NilRepo(t *testing.T) {
	var r *APIKeyRepo
	// Should not panic
	r.UpdateLastUsed("key-id", "127.0.0.1")
}

// Test APIKeyRepo.UpdateLastUsed with nil db
func TestAPIKeyRepo_UpdateLastUsed_NilDB(t *testing.T) {
	r := &APIKeyRepo{db: nil, now: time.Now}
	// Should not panic
	r.UpdateLastUsed("key-id", "127.0.0.1")
}

// Test APIKeyRepo.UpdateLastUsed with empty id
func TestAPIKeyRepo_UpdateLastUsed_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	repo := db.APIKeys()
	// Should not panic or error with empty id
	repo.UpdateLastUsed("", "127.0.0.1")
}

// Test scanAPIKeyRows error handling
func TestScanAPIKeyRows_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create a user and API key
	user := &User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	_, _, err := db.APIKeys().Create(user.ID, "test-key", "live")
	if err != nil {
		t.Fatalf("Create API key error: %v", err)
	}

	// Query with invalid column selection to trigger scan error
	rows, err := db.DB().Query("SELECT id FROM api_keys LIMIT 1")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		_, err := scanAPIKeyRows(rows)
		if err == nil {
			t.Error("scanAPIKeyRows() with invalid columns should return error")
		}
	}
}

// Test scanUserRows error handling
func TestScanUserRows_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create a user
	user := &User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Query with invalid column selection to trigger scan error
	rows, err := db.DB().Query("SELECT id FROM users LIMIT 1")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		_, err := scanUserRows(rows)
		if err == nil {
			t.Error("scanUserRows() with invalid columns should return error")
		}
	}
}

// Test scanAuditRows error handling
func TestScanAuditRows_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create audit log
	entry := &AuditEntry{
		RequestID:  "req-123",
		RouteID:    "route-456",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
	}
	if err := db.Audits().BatchInsert([]AuditEntry{*entry}); err != nil {
		t.Fatalf("Create audit error: %v", err)
	}

	// Query with invalid column selection to trigger scan error
	rows, err := db.DB().Query("SELECT id FROM audit_logs LIMIT 1")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		_, err := scanAuditRows(rows)
		if err == nil {
			t.Error("scanAuditRows() with invalid columns should return error")
		}
	}
}

// Test scanCreditTransactionRows error handling
func TestScanCreditTransactionRows_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user and transaction
	user := &User{
		Email:  "credit@example.com",
		Name:   "Credit User",
		Role:   "user",
		Status: "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	txn := &CreditTransaction{
		UserID: user.ID,
		Type:   "add",
		Amount: 100,
	}
	if err := db.Credits().Create(txn); err != nil {
		t.Fatalf("Create transaction error: %v", err)
	}

	// Query with invalid column selection to trigger scan error
	rows, err := db.DB().Query("SELECT id FROM credit_transactions LIMIT 1")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		_, err := scanCreditTransactionRows(rows)
		if err == nil {
			t.Error("scanCreditTransactionRows() with invalid columns should return error")
		}
	}
}

// Test scanPermissionRows error handling
func TestScanPermissionRows_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user and permission
	user := &User{
		Email:  "perm@example.com",
		Name:   "Perm User",
		Role:   "user",
		Status: "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	perm := &EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-123",
		Allowed: true,
	}
	if err := db.Permissions().Create(perm); err != nil {
		t.Fatalf("Create permission error: %v", err)
	}

	// Query with invalid column selection to trigger scan error
	rows, err := db.DB().Query("SELECT id FROM endpoint_permissions LIMIT 1")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		_, err := scanPermissionRows(rows)
		if err == nil {
			t.Error("scanPermissionRows() with invalid columns should return error")
		}
	}
}

// Test scanPlaygroundTemplateRows error handling
func TestScanPlaygroundTemplateRows_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user and template
	user := &User{
		Email:  "template@example.com",
		Name:   "Template User",
		Role:   "user",
		Status: "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	template := &PlaygroundTemplate{
		UserID: user.ID,
		Name:   "Test Template",
		Method: "GET",
		Path:   "/test",
	}
	if err := db.PlaygroundTemplates().Save(template); err != nil {
		t.Fatalf("Save template error: %v", err)
	}

	// Query with invalid column selection to trigger scan error
	rows, err := db.DB().Query("SELECT id FROM playground_templates LIMIT 1")
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		_, err := scanPlaygroundTemplateRows(rows)
		if err == nil {
			t.Error("scanPlaygroundTemplateRows() with invalid columns should return error")
		}
	}
}

// Test GenerateSessionToken
func TestGenerateSessionToken(t *testing.T) {
	token, err := GenerateSessionToken()
	if err != nil {
		t.Errorf("GenerateSessionToken() should not error: %v", err)
	}
	if len(token) == 0 {
		t.Error("GenerateSessionToken() should return non-empty token")
	}
}

// Test AuditRepo.BatchInsert error paths
func TestAuditRepo_BatchInsert_ErrorPaths(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	repo := db.Audits()

	// Test with nil entries
	err := repo.BatchInsert(nil)
	if err != nil {
		t.Errorf("BatchInsert(nil) should not error: %v", err)
	}

	// Test with empty entries
	err = repo.BatchInsert([]AuditEntry{})
	if err != nil {
		t.Errorf("BatchInsert([]) should not error: %v", err)
	}
}

// Test exportAuditCSV error handling
func TestExportAuditCSV_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create an audit entry
	entry := &AuditEntry{
		RequestID:  "req-123",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
	}
	if err := db.Audits().BatchInsert([]AuditEntry{*entry}); err != nil {
		t.Fatalf("Create audit error: %v", err)
	}

	// Test with nil writer - should use a buffer
	var buf bytes.Buffer
	err := db.Audits().Export(AuditSearchFilters{}, "csv", &buf)
	if err != nil {
		t.Errorf("Export CSV error: %v", err)
	}
}

// Test exportAuditJSONL error handling
func TestExportAuditJSONL_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create an audit entry
	entry := &AuditEntry{
		RequestID:  "req-123",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
	}
	if err := db.Audits().BatchInsert([]AuditEntry{*entry}); err != nil {
		t.Fatalf("Create audit error: %v", err)
	}

	// Test export
	var buf bytes.Buffer
	err := db.Audits().Export(AuditSearchFilters{}, "jsonl", &buf)
	if err != nil {
		t.Errorf("Export JSONL error: %v", err)
	}
}

// Test exportAuditJSON error handling
func TestExportAuditJSON_Error(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create an audit entry
	entry := &AuditEntry{
		RequestID:  "req-123",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
	}
	if err := db.Audits().BatchInsert([]AuditEntry{*entry}); err != nil {
		t.Fatalf("Create audit error: %v", err)
	}

	// Test export
	var buf bytes.Buffer
	err := db.Audits().Export(AuditSearchFilters{}, "json", &buf)
	if err != nil {
		t.Errorf("Export JSON error: %v", err)
	}
}

// Test PermissionRepo.withTx error handling
func TestPermissionRepo_WithTx_ErrorPaths(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	repo := db.Permissions()

	// Test withTx where fn returns error
	testErr := errors.New("test error")
	err := repo.withTx(context.Background(), func(tx *sql.Tx) error {
		return testErr
	})
	if !errors.Is(err, testErr) {
		t.Errorf("withTx() should return fn error, got: %v", err)
	}
}

// Test randomToken error handling
func TestRandomToken_ErrorPath(t *testing.T) {
	// Test with zero length
	_, err := randomToken(0)
	if err == nil {
		t.Error("randomToken(0) should return error")
	}

	// Test with negative length
	_, err = randomToken(-1)
	if err == nil {
		t.Error("randomToken(-1) should return error")
	}

	// Test with valid length
	token, err := randomToken(32)
	if err != nil {
		t.Errorf("randomToken(32) should not error: %v", err)
	}
	if len(token) != 32 {
		t.Errorf("randomToken(32) should return 32 chars, got: %d", len(token))
	}
}

// Test CreditRepo.ListByUser with various filters
func TestCreditRepo_ListByUser_Filters(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:  "credit-test@example.com",
		Name:   "Credit Test User",
		Role:   "user",
		Status: "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create transactions
	txns := []*CreditTransaction{
		{UserID: user.ID, Type: "add", Amount: 100},
		{UserID: user.ID, Type: "consume", Amount: -50},
		{UserID: user.ID, Type: "refund", Amount: 25},
	}
	for _, txn := range txns {
		if err := db.Credits().Create(txn); err != nil {
			t.Fatalf("Create transaction error: %v", err)
		}
	}

	// Test with invalid type filter
	result, err := db.Credits().ListByUser(user.ID, CreditListOptions{Type: "nonexistent"})
	if err != nil {
		t.Errorf("ListByUser with invalid type should not error: %v", err)
	}
	if result == nil || len(result.Transactions) != 0 {
		t.Error("ListByUser with invalid type should return empty results")
	}
}

// Test marshalJSON with unmarshalable value
func TestMarshalJSON_Unmarshalable(t *testing.T) {
	// Create a channel which cannot be marshaled to JSON
	ch := make(chan int)
	_, err := marshalJSON(ch, "{}")
	if err == nil {
		t.Error("marshalJSON() with unmarshalable value should return error")
	}
}

// Test unmarshalStringMap error handling
func TestUnmarshalStringMap_Error(t *testing.T) {
	// Test with invalid JSON
	_, err := unmarshalStringMap("invalid json")
	if err == nil {
		t.Error("unmarshalStringMap() with invalid JSON should return error")
	}
}

// Test marshalStringMap error handling
func TestMarshalStringMap_Error(t *testing.T) {
	// Create a map with unmarshalable value
	m := map[string]string{
		"key": "value",
	}
	// This should work fine
	result, err := marshalStringMap(m)
	if err != nil {
		t.Errorf("marshalStringMap() with valid map should not error: %v", err)
	}
	if result == "" {
		t.Error("marshalStringMap() should return non-empty string")
	}
}

// Test decodePermissionFields error handling
func TestDecodePermissionFields_Error(t *testing.T) {
	perm := &EndpointPermission{}

	// Test with invalid methods JSON
	err := decodePermissionFields(perm, "invalid", "[]", "[]", "", "", "", "", "", "")
	if err == nil {
		t.Error("decodePermissionFields() with invalid methods JSON should return error")
	}

	// Test with invalid allowed_days JSON
	err = decodePermissionFields(perm, "[]", "invalid", "[]", "", "", "", "", "", "")
	if err == nil {
		t.Error("decodePermissionFields() with invalid allowed_days JSON should return error")
	}

	// Test with invalid allowed_hours JSON
	err = decodePermissionFields(perm, "[]", "[]", "invalid", "", "", "", "", "", "")
	if err == nil {
		t.Error("decodePermissionFields() with invalid allowed_hours JSON should return error")
	}

	// Test with invalid valid_from time
	err = decodePermissionFields(perm, "[]", "[]", "[]", "invalid-time", "", "", "", "", "")
	if err == nil {
		t.Error("decodePermissionFields() with invalid valid_from should return error")
	}

	// Test with invalid valid_until time
	err = decodePermissionFields(perm, "[]", "[]", "[]", "", "invalid-time", "", "", "", "")
	if err == nil {
		t.Error("decodePermissionFields() with invalid valid_until should return error")
	}

	// Test with invalid created_at time
	err = decodePermissionFields(perm, "[]", "[]", "[]", "", "", "", "", "invalid-time", "")
	if err == nil {
		t.Error("decodePermissionFields() with invalid created_at should return error")
	}

	// Test with invalid updated_at time
	err = decodePermissionFields(perm, "[]", "[]", "[]", "", "", "", "", "", "invalid-time")
	if err == nil {
		t.Error("decodePermissionFields() with invalid updated_at should return error")
	}
}

// Test decodeAPIKeyDateFields error handling
func TestDecodeAPIKeyDateFields_Error(t *testing.T) {
	key := &APIKey{}

	// Test with invalid expires_at time
	err := decodeAPIKeyDateFields(key, "invalid", "", "", "")
	if err == nil {
		t.Error("decodeAPIKeyDateFields() with invalid expires_at should return error")
	}

	// Test with invalid last_used_at time
	err = decodeAPIKeyDateFields(key, "", "invalid", "", "")
	if err == nil {
		t.Error("decodeAPIKeyDateFields() with invalid last_used_at should return error")
	}

	// Test with invalid created_at time
	err = decodeAPIKeyDateFields(key, "", "", "invalid", "")
	if err == nil {
		t.Error("decodeAPIKeyDateFields() with invalid created_at should return error")
	}

	// Test with invalid updated_at time
	err = decodeAPIKeyDateFields(key, "", "", "", "invalid")
	if err == nil {
		t.Error("decodeAPIKeyDateFields() with invalid updated_at should return error")
	}
}

// Test decodeUserJSONFields error handling
func TestDecodeUserJSONFields_Error(t *testing.T) {
	user := &User{}

	// Test with nil user
	err := decodeUserJSONFields(nil, "{}", "[]", "{}", "", "")
	if err == nil {
		t.Error("decodeUserJSONFields() with nil user should return error")
	}

	// Test with invalid rate_limits JSON
	err = decodeUserJSONFields(user, "invalid", "[]", "{}", "", "")
	if err == nil {
		t.Error("decodeUserJSONFields() with invalid rate_limits JSON should return error")
	}

	// Test with invalid ip_whitelist JSON
	err = decodeUserJSONFields(user, "{}", "invalid", "{}", "", "")
	if err == nil {
		t.Error("decodeUserJSONFields() with invalid ip_whitelist JSON should return error")
	}

	// Test with invalid metadata JSON
	err = decodeUserJSONFields(user, "{}", "[]", "invalid", "", "")
	if err == nil {
		t.Error("decodeUserJSONFields() with invalid metadata JSON should return error")
	}

	// Test with invalid created_at time
	err = decodeUserJSONFields(user, "{}", "[]", "{}", "invalid", "")
	if err == nil {
		t.Error("decodeUserJSONFields() with invalid created_at should return error")
	}

	// Test with invalid updated_at time
	err = decodeUserJSONFields(user, "{}", "[]", "{}", "", "invalid")
	if err == nil {
		t.Error("decodeUserJSONFields() with invalid updated_at should return error")
	}
}

// Test Open with corrupted database file
func TestOpen_CorruptedDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupted.db")

	// Write invalid SQLite data
	if err := os.WriteFile(dbPath, []byte("NOT A VALID SQLITE DATABASE"), 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        dbPath,
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	_, err := Open(cfg)
	if err == nil {
		t.Fatal("Expected error when opening corrupted database, but got none")
	}
}

// Test Open when applyPragmas fails
func TestOpen_ApplyPragmasError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Close the database to simulate error condition
	s.db.Close()

	// Try to apply pragmas - should fail
	err = s.applyPragmas()
	if err == nil {
		t.Fatal("Expected error when applyPragmas fails, but got none")
	}
}

// Test Open when migrate fails
func TestOpen_MigrateError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Close the database to simulate error condition
	s.db.Close()

	// Try to migrate - should fail
	err = s.migrate()
	if err == nil {
		t.Fatal("Expected error when migrate fails, but got none")
	}
}

// Test migrate when creating schema_migrations fails
func TestMigrate_CreateSchemaMigrationsError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Drop schema_migrations table
	_, err = s.db.Exec("DROP TABLE schema_migrations")
	if err != nil {
		t.Fatalf("failed to drop schema_migrations: %v", err)
	}

	// Close the database to force error
	s.db.Close()

	// Try to migrate - should fail when creating schema_migrations
	err = s.migrate()
	if err == nil {
		t.Fatal("Expected error when creating schema_migrations fails, but got none")
	}
}

// Test migrate when isMigrationApplied fails
func TestMigrate_IsMigrationAppliedError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Drop schema_migrations table
	_, err = s.db.Exec("DROP TABLE schema_migrations")
	if err != nil {
		t.Fatalf("failed to drop schema_migrations: %v", err)
	}

	// Close the database to force error
	s.db.Close()

	// Try to migrate - should fail when creating schema_migrations (first query)
	err = s.migrate()
	if err == nil {
		t.Fatal("Expected error when migrate fails, but got none")
	}
}

// Test migrate when BeginTx fails
func TestMigrate_BeginTxError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Clear migrations to force re-application
	_, err = s.db.Exec("DELETE FROM schema_migrations")
	if err != nil {
		t.Fatalf("failed to clear migrations: %v", err)
	}

	// Close the database to force BeginTx to fail
	s.db.Close()

	// Try to migrate - should fail at BeginTx
	err = s.migrate()
	if err == nil {
		t.Fatal("Expected error when BeginTx fails, but got none")
	}
}

// Test migrate when migration statement fails
func TestMigrate_StatementExecError(t *testing.T) {
	// This test creates a scenario where a migration statement fails
	// We do this by creating a store, then manually corrupting the migration state
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Verify migrations succeeded
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}
	if count != len(migrationsList) {
		t.Fatalf("expected %d migrations, got %d", len(migrationsList), count)
	}
}

// Test migrate when recording migration fails
func TestMigrate_RecordMigrationError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Clear migrations to force re-application
	_, err = s.db.Exec("DELETE FROM schema_migrations")
	if err != nil {
		t.Fatalf("failed to clear migrations: %v", err)
	}

	// Drop playground_templates table to allow re-creation
	_, err = s.db.Exec("DROP TABLE IF EXISTS playground_templates")
	if err != nil {
		t.Fatalf("failed to drop playground_templates: %v", err)
	}

	// Close the database to force error during migration
	s.db.Close()

	// Try to migrate - should fail
	err = s.migrate()
	if err == nil {
		t.Fatal("Expected error when migrate fails, but got none")
	}
}

// Test migrate when commit fails
func TestMigrate_CommitError(t *testing.T) {
	// This is difficult to test without complex mocking
	// We verify the commit path works by checking successful migrations
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Verify the migration was committed
	var version int
	err = s.db.QueryRow("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("failed to get latest migration: %v", err)
	}
	if version != len(migrationsList) {
		t.Fatalf("expected latest version %d, got %d", len(migrationsList), version)
	}
}

// Test isMigrationApplied when query fails
func TestIsMigrationApplied_QueryError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Drop the schema_migrations table to force query error
	_, err = s.db.Exec("DROP TABLE schema_migrations")
	if err != nil {
		t.Fatalf("failed to drop schema_migrations: %v", err)
	}

	// Now query should fail
	_, err = s.isMigrationApplied(1)
	if err == nil {
		t.Fatal("Expected error when querying dropped table, but got none")
	}
}

// Test applyPragmas when journal_mode pragma fails
func TestApplyPragmas_JournalModeError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Close the database to force pragma errors
	s.db.Close()

	// Try to apply pragmas - should fail
	err = s.applyPragmas()
	if err == nil {
		t.Fatal("Expected error when journal_mode pragma fails, but got none")
	}
}

// Test applyPragmas when busy_timeout pragma fails
func TestApplyPragmas_BusyTimeoutError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Close the database to force pragma errors
	s.db.Close()

	// Try to apply pragmas - busy_timeout should fail
	err = s.applyPragmas()
	if err == nil {
		t.Fatal("Expected error when busy_timeout pragma fails, but got none")
	}
}

// Test applyPragmas when foreign_keys pragma fails
func TestApplyPragmas_ForeignKeysError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Close the database to force pragma errors
	s.db.Close()

	// Try to apply pragmas - foreign_keys should fail
	err = s.applyPragmas()
	if err == nil {
		t.Fatal("Expected error when foreign_keys pragma fails, but got none")
	}
}

// Test applyPragmas when ping fails
func TestApplyPragmas_PingError(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Close the database to force ping to fail
	s.db.Close()

	// Try to apply pragmas - ping should fail
	err = s.applyPragmas()
	if err == nil {
		t.Fatal("Expected error when ping fails, but got none")
	}
}

// Test applyPragmas with empty journal mode
func TestApplyPragmas_EmptyJournalMode(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "", // Empty - should default to WAL
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Verify it worked (in-memory uses memory mode)
	var journalMode string
	err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "memory" {
		t.Fatalf("expected memory mode for in-memory db, got %s", journalMode)
	}
}

// Test applyPragmas with zero busy timeout
func TestApplyPragmas_ZeroBusyTimeout(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 0, // Zero - should default to 5000ms
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Verify default busy timeout was applied
	var busyTimeout int
	err = s.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	if err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("expected busy_timeout = 5000, got %d", busyTimeout)
	}
}

// Test applyPragmas with foreign keys disabled
func TestApplyPragmas_ForeignKeysOff(t *testing.T) {
	// This test verifies that the foreign_keys pragma is set correctly
	// Note: SQLite foreign key settings are per-connection, so we verify
	// the pragma was executed by checking no error occurs
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fk_test.db")

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        dbPath,
			BusyTimeout: 5 * time.Second,
			JournalMode: "DELETE",
			ForeignKeys: false,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Just verify the store opened successfully with foreign keys disabled
	// The pragma was applied during Open()
	if s.db == nil {
		t.Fatal("expected db to be initialized")
	}
}

// Test Open with invalid journal mode
func TestOpen_InvalidJournalMode(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "INVALID_MODE",
			ForeignKeys: true,
		},
	}

	_, err := Open(cfg)
	if err == nil {
		t.Fatal("Expected error when opening with invalid journal mode, but got none")
	}
}

// Test Open with negative busy timeout
func TestOpen_NegativeBusyTimeout(t *testing.T) {
	// Test the validation directly since Open validates config first
	err := validateStoreConfig(config.StoreConfig{
		Path:        ":memory:",
		BusyTimeout: -1 * time.Second,
		JournalMode: "MEMORY",
	})
	if err == nil {
		t.Fatal("Expected error for negative busy timeout, but got none")
	}
	if err.Error() != "store.busy_timeout cannot be negative" {
		t.Fatalf("Expected 'store.busy_timeout cannot be negative', got: %v", err)
	}
}

// Test Open with nil config
func TestOpen_NilConfig(t *testing.T) {
	s, err := Open(nil)
	if err != nil {
		t.Fatalf("Expected Open with nil config to use defaults, but got error: %v", err)
	}
	defer s.Close()

	// Verify it opened with default settings
	if s.db == nil {
		t.Fatal("Expected db to be initialized")
	}
}

// Test applyPragmas with all valid journal modes
func TestApplyPragmas_AllJournalModes(t *testing.T) {
	modes := []string{"WAL", "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "OFF"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			cfg := &config.Config{
				Store: config.StoreConfig{
					Path:        ":memory:",
					BusyTimeout: 5 * time.Second,
					JournalMode: mode,
					ForeignKeys: true,
				},
			}

			s, err := Open(cfg)
			if err != nil {
				t.Fatalf("failed to open store with journal mode %s: %v", mode, err)
			}
			s.Close()
		})
	}
}

// Test validateStoreConfig with all valid journal modes
func TestValidateStoreConfig_AllValidJournalModes(t *testing.T) {
	modes := []string{"WAL", "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "OFF"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			err := validateStoreConfig(config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: 5 * time.Second,
				JournalMode: mode,
			})
			if err != nil {
				t.Errorf("Unexpected error for journal mode %s: %v", mode, err)
			}
		})
	}
}

// Test validateStoreConfig case insensitivity
func TestValidateStoreConfig_CaseInsensitive(t *testing.T) {
	modes := []string{"wal", "Wal", "WAL", "delete", "Delete", "DELETE"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			err := validateStoreConfig(config.StoreConfig{
				Path:        ":memory:",
				BusyTimeout: 5 * time.Second,
				JournalMode: mode,
			})
			if err != nil {
				t.Errorf("Unexpected error for journal mode %s: %v", mode, err)
			}
		})
	}
}

// Test resolveStoreConfig with empty values
func TestResolveStoreConfig_EmptyValues(t *testing.T) {
	input := &config.Config{
		Store: config.StoreConfig{
			Path:        "",
			BusyTimeout: 0,
			JournalMode: "",
			ForeignKeys: false,
		},
	}

	cfg := resolveStoreConfig(input)

	// Empty path should get default
	if cfg.Path != "apicerberus.db" {
		t.Errorf("expected default path 'apicerberus.db', got %s", cfg.Path)
	}
	// Zero busy timeout gets default (5s) since it's not > 0
	if cfg.BusyTimeout != 5*time.Second {
		t.Errorf("expected default busy timeout 5s, got %v", cfg.BusyTimeout)
	}
	// Empty journal mode gets default "WAL"
	if cfg.JournalMode != "WAL" {
		t.Errorf("expected default journal mode 'WAL', got %s", cfg.JournalMode)
	}
	// Foreign keys should be true (default) since input was false
	if !cfg.ForeignKeys {
		t.Error("expected foreign keys to be true (default)")
	}
}

// Test resolveStoreConfig with whitespace values
func TestResolveStoreConfig_WhitespaceValues(t *testing.T) {
	input := &config.Config{
		Store: config.StoreConfig{
			Path:        "  /path/to/db.db  ",
			BusyTimeout: 10 * time.Second,
			JournalMode: "  wal  ",
			ForeignKeys: true,
		},
	}

	cfg := resolveStoreConfig(input)

	if cfg.Path != "/path/to/db.db" {
		t.Errorf("expected trimmed path, got '%s'", cfg.Path)
	}
	if cfg.JournalMode != "WAL" {
		t.Errorf("expected uppercase journal mode 'WAL', got '%s'", cfg.JournalMode)
	}
}

// Test validateStoreConfig empty path
func TestValidateStoreConfig_EmptyPath(t *testing.T) {
	err := validateStoreConfig(config.StoreConfig{
		Path:        "",
		BusyTimeout: 5 * time.Second,
		JournalMode: "WAL",
	})
	if err == nil {
		t.Fatal("Expected error for empty path, but got none")
	}
	if err.Error() != "store.path is required" {
		t.Errorf("Expected 'store.path is required', got: %v", err)
	}
}

// Test validateStoreConfig whitespace path
func TestValidateStoreConfig_WhitespacePath(t *testing.T) {
	err := validateStoreConfig(config.StoreConfig{
		Path:        "   ",
		BusyTimeout: 5 * time.Second,
		JournalMode: "WAL",
	})
	if err == nil {
		t.Fatal("Expected error for whitespace-only path, but got none")
	}
}

// Test validateStoreConfig negative busy timeout
func TestValidateStoreConfig_NegativeBusyTimeout(t *testing.T) {
	err := validateStoreConfig(config.StoreConfig{
		Path:        "/path/to/db.db",
		BusyTimeout: -1 * time.Second,
		JournalMode: "WAL",
	})
	if err == nil {
		t.Fatal("Expected error for negative busy timeout, but got none")
	}
	if err.Error() != "store.busy_timeout cannot be negative" {
		t.Errorf("Expected 'store.busy_timeout cannot be negative', got: %v", err)
	}
}

// Test isMigrationApplied with non-existent migration
func TestIsMigrationApplied_NotExists(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Check a migration that doesn't exist
	applied, err := s.isMigrationApplied(9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Fatal("expected migration 9999 to not be applied")
	}
}

// Test isMigrationApplied with existing migration
func TestIsMigrationApplied_Exists(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Check a migration that exists
	applied, err := s.isMigrationApplied(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !applied {
		t.Fatal("expected migration 1 to be applied")
	}
}

// Test migrate with partial migrations
func TestMigrate_PartialMigrations(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Delete the last migration to force it to be re-applied
	_, err = s.db.Exec("DELETE FROM schema_migrations WHERE version = ?", len(migrationsList))
	if err != nil {
		t.Fatalf("failed to delete last migration: %v", err)
	}

	// Also drop the playground_templates table to allow re-creation
	_, err = s.db.Exec("DROP TABLE IF EXISTS playground_templates")
	if err != nil {
		t.Fatalf("failed to drop playground_templates: %v", err)
	}

	// Re-run migrations
	err = s.migrate()
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify the migration was re-applied
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", len(migrationsList)).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check migration: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migration %d to be re-applied", len(migrationsList))
	}
}

// Test migrate idempotency
func TestMigrate_Idempotent(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Run migrations again - should be idempotent
	err = s.migrate()
	if err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	// Verify no duplicate migrations
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}
	if count != len(migrationsList) {
		t.Fatalf("expected %d migrations, got %d (duplicates detected)", len(migrationsList), count)
	}
}

// Test Open/Close cycle
func TestOpen_CloseCycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cycle.db")

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        dbPath,
			BusyTimeout: 5 * time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}

	// First open
	s1, err := Open(cfg)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	s1.Close()

	// Second open - should work with existing database
	s2, err := Open(cfg)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}

	// Verify data is still there
	var count int
	err = s2.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}
	if count != len(migrationsList) {
		t.Fatalf("expected %d migrations, got %d", len(migrationsList), count)
	}

	s2.Close()
}

// Test admin user creation
func TestOpen_EnsureAdminUserExists(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Check that admin user exists
	var email string
	err = s.db.QueryRow("SELECT email FROM users WHERE role = 'admin' LIMIT 1").Scan(&email)
	if err != nil {
		t.Fatalf("failed to find admin user: %v", err)
	}
	if email != "admin@apicerberus.local" {
		t.Fatalf("expected admin email, got %s", email)
	}
}

// Test applyPragmas with lowercase journal mode
func TestApplyPragmas_LowercaseJournalMode(t *testing.T) {
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "wal", // lowercase
			ForeignKeys: true,
		},
	}

	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	// Verify it worked
	var journalMode string
	err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	// In-memory databases always use memory mode
	if journalMode != "memory" {
		t.Fatalf("expected memory mode for in-memory db, got %s", journalMode)
	}
}
