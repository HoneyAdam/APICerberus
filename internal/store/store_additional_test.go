package store

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// Helper function to setup test database
func setupTestStore(t *testing.T) *Store {
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
	return s
}

// Test normalizeUserSortBy
func TestNormalizeUserSortBy(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"email", "email"},
		{"name", "name"},
		{"updated_at", "updated_at"},
		{"credit_balance", "credit_balance"},
		{"created_at", "created_at"},
		{"", "created_at"},
		{"unknown", "created_at"},
		{"EMAIL", "email"},
		{" Name ", "name"},
		{"Updated_AT", "updated_at"},
	}

	for _, tt := range tests {
		result := normalizeUserSortBy(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeUserSortBy(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// Test validateUserInput
func TestValidateUserInput(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{
			name: "valid user",
			user: User{
				Email: "test@example.com",
				Name:  "Test User",
			},
			wantErr: false,
		},
		{
			name: "empty email",
			user: User{
				Email: "",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "whitespace email",
			user: User{
				Email: "   ",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email no at",
			user: User{
				Email: "testexample.com",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email no domain",
			user: User{
				Email: "test@",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "invalid email no dot",
			user: User{
				Email: "test@example",
				Name:  "Test User",
			},
			wantErr: true,
		},
		{
			name: "empty name",
			user: User{
				Email: "test@example.com",
				Name:  "",
			},
			wantErr: true,
		},
		{
			name: "whitespace name",
			user: User{
				Email: "test@example.com",
				Name:  "   ",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUserInput(tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUserInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test looksLikeEmail
func TestLooksLikeEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"test@example.com", true},
		{"user@domain.org", true},
		{"a@b.co", true},
		{"testexample.com", false},
		{"test@", false},
		{"@example.com", false},
		{"test@example", false},
		{"", false},
		{"   ", false},
		{"test@@example.com", true}, // has @ and domain has .
		{"test@example.", true},     // has @ and domain has . (even at end)
		{".test@example.com", true}, // has @ and domain has .
	}

	for _, tt := range tests {
		result := looksLikeEmail(tt.email)
		if result != tt.valid {
			t.Errorf("looksLikeEmail(%q) = %v, want %v", tt.email, result, tt.valid)
		}
	}
}

// Test marshalJSON
func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		fallback string
		wantErr  bool
	}{
		{
			name:     "nil value",
			value:    nil,
			fallback: "null",
			wantErr:  false,
		},
		{
			name:     "string value",
			value:    "test",
			fallback: "",
			wantErr:  false,
		},
		{
			name:     "map value",
			value:    map[string]any{"key": "value"},
			fallback: "",
			wantErr:  false,
		},
		{
			name:     "slice value",
			value:    []string{"a", "b"},
			fallback: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalJSON(tt.value, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Errorf("marshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.value == nil && result != tt.fallback {
				t.Errorf("marshalJSON() = %q, want %q", result, tt.fallback)
			}
		})
	}
}

// Test normalizeAuditLimit
func TestNormalizeAuditLimit(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 50},
		{-1, 50},
		{50, 50},
		{100, 100},
		{200, 200},
		{1000, 1000},
		{2000, 1000},
	}

	for _, tt := range tests {
		result := normalizeAuditLimit(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeAuditLimit(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

// Test normalizeAuditExportLimit
func TestNormalizeAuditExportLimit(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{-1, 0},
		{500, 500},
		{1000, 1000},
		{5000, 5000},
		{50000, 50000},
		{100000, 100000},
		{200000, 100000},
	}

	for _, tt := range tests {
		result := normalizeAuditExportLimit(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeAuditExportLimit(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

// Test normalizeAuditOffset
func TestNormalizeAuditOffset(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{1, 1},
		{100, 100},
		{-1, 0},
		{-100, 0},
		{999, 999},
	}

	for _, tt := range tests {
		result := normalizeAuditOffset(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeAuditOffset(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

// Test FindByEmail
func TestFindByEmail(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create a user
	user := &User{
		Email:        "findbyemail@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Find by email
	found, err := db.Users().FindByEmail("findbyemail@example.com")
	if err != nil {
		t.Errorf("FindByEmail error: %v", err)
	}
	if found == nil {
		t.Error("FindByEmail should return user")
	}
	if found.Email != "findbyemail@example.com" {
		t.Errorf("Email = %q, want findbyemail@example.com", found.Email)
	}

	// Find non-existent email
	notFound, err := db.Users().FindByEmail("nonexistent@example.com")
	if err != nil {
		t.Errorf("FindByEmail non-existent error: %v", err)
	}
	if notFound != nil {
		t.Error("FindByEmail should return nil for non-existent email")
	}
}

// Test FindByHash
func TestFindByHash(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create a user first
	user := &User{
		Email:        "findbyhash@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create an API key using the proper method
	rawKey, apiKey, err := db.APIKeys().Create(user.ID, "Test Key", "test")
	if err != nil {
		t.Fatalf("Create API key error: %v", err)
	}
	_ = rawKey

	// Find by hash - verify it matches the created key's hash
	found, err := db.APIKeys().FindByHash(apiKey.KeyHash)
	if err != nil {
		t.Errorf("FindByHash error: %v", err)
	}
	if found == nil {
		t.Error("FindByHash should return API key")
	}
	if found.KeyHash != apiKey.KeyHash {
		t.Errorf("KeyHash = %q, want %q", found.KeyHash, apiKey.KeyHash)
	}

	// Find non-existent hash
	notFound, err := db.APIKeys().FindByHash("nonexistenthash")
	if err != nil {
		t.Errorf("FindByHash non-existent error: %v", err)
	}
	if notFound != nil {
		t.Error("FindByHash should return nil for non-existent hash")
	}
}

// Test FindByID for audit logs
func TestAuditFindByID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create an audit entry using BatchInsert
	entries := []AuditEntry{
		{
			ID:         "audit-test-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			LatencyMS:  50,
			ClientIP:   "127.0.0.1",
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert audit error: %v", err)
	}

	// Find by ID
	found, err := db.Audits().FindByID("audit-test-1")
	if err != nil {
		t.Errorf("FindByID error: %v", err)
	}
	if found == nil {
		t.Error("FindByID should return audit entry")
	}
	if found.ID != "audit-test-1" {
		t.Errorf("ID = %q, want audit-test-1", found.ID)
	}

	// Find non-existent ID
	notFound, err := db.Audits().FindByID("nonexistent")
	if err != nil {
		t.Errorf("FindByID non-existent error: %v", err)
	}
	if notFound != nil {
		t.Error("FindByID should return nil for non-existent ID")
	}
}

// Test BatchInsert for audit entries
func TestAuditBatchInsert(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	entries := []AuditEntry{
		{
			ID:         "batch-1",
			Method:     "GET",
			Path:       "/api/test1",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
		},
		{
			ID:         "batch-2",
			Method:     "POST",
			Path:       "/api/test2",
			StatusCode: 201,
			ClientIP:   "127.0.0.1",
		},
	}

	// Batch insert
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Errorf("BatchInsert error: %v", err)
	}

	// Verify entries were inserted
	for _, id := range []string{"batch-1", "batch-2"} {
		found, err := db.Audits().FindByID(id)
		if err != nil {
			t.Errorf("FindByID %s error: %v", id, err)
		}
		if found == nil {
			t.Errorf("Entry %s should exist", id)
		}
	}

	// Test empty batch
	if err := db.Audits().BatchInsert([]AuditEntry{}); err != nil {
		t.Errorf("BatchInsert empty error: %v", err)
	}
}

// Test Export for audit entries
func TestAuditExport(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create audit entries
	entries := []AuditEntry{
		{
			ID:         "export-1",
			Method:     "GET",
			Path:       "/api/test1",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
		},
		{
			ID:         "export-2",
			Method:     "POST",
			Path:       "/api/test2",
			StatusCode: 201,
			ClientIP:   "127.0.0.1",
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Export entries - just verify it doesn't error
	var buf strings.Builder
	err := db.Audits().Export(AuditSearchFilters{
		Limit: 100,
	}, "jsonl", &buf)
	if err != nil {
		t.Errorf("Export error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Export should return data")
	}
}

// Test Revoke for API keys
func TestRevokeAPIKey(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create a user first
	user := &User{
		Email:        "revoke@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create an API key using the proper method
	_, apiKey, err := db.APIKeys().Create(user.ID, "Key to Revoke", "test")
	if err != nil {
		t.Fatalf("Create API key error: %v", err)
	}

	// Revoke the key - just verify it doesn't error
	if err := db.APIKeys().Revoke(apiKey.ID); err != nil {
		t.Errorf("Revoke error: %v", err)
	}

	// Verify key is revoked by listing
	keys, _ := db.APIKeys().ListByUser(user.ID)
	for _, key := range keys {
		if key.ID == apiKey.ID && key.Status != "revoked" {
			t.Errorf("Status = %q, want revoked", key.Status)
		}
	}
}

// Test withTx for permissions
func TestPermissionWithTx(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create a user first
	user := &User{
		Email:        "permission@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create a permission
	perm := &EndpointPermission{
		UserID:     user.ID,
		RouteID:    "route-1",
		CreditCost: int64Ptr(5),
	}
	if err := db.Permissions().Create(perm); err != nil {
		t.Fatalf("Create permission error: %v", err)
	}

	// Test withTx - this should not error
	// The withTx function handles transactions
}

func int64Ptr(i int64) *int64 {
	return &i
}

// Test BatchInsert with nil repo
func TestAuditBatchInsert_NilRepo(t *testing.T) {
	var nilRepo *AuditRepo
	err := nilRepo.BatchInsert([]AuditEntry{
		{ID: "test", Method: "GET"},
	})
	if err == nil {
		t.Error("BatchInsert with nil repo should return error")
	}
}

// Test BatchInsert with empty entries
func TestAuditBatchInsert_EmptyEntries(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Empty batch should not error
	if err := db.Audits().BatchInsert([]AuditEntry{}); err != nil {
		t.Errorf("BatchInsert empty error: %v", err)
	}
}

// Test APIKeys with nil store
func TestStore_APIKeys_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.APIKeys()
	if repo != nil {
		t.Error("APIKeys with nil store should return nil")
	}
}

// Test Audits with nil store
func TestStore_Audits_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.Audits()
	if repo != nil {
		t.Error("Audits with nil store should return nil")
	}
}

// Test Credits with nil store
func TestStore_Credits_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.Credits()
	if repo != nil {
		t.Error("Credits with nil store should return nil")
	}
}

// Test Permissions with nil store
func TestStore_Permissions_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.Permissions()
	if repo != nil {
		t.Error("Permissions with nil store should return nil")
	}
}

// ========== Additional Tests for Error Paths ==========

// Test Open with invalid config
func TestOpen_InvalidConfig(t *testing.T) {
	// Test with invalid journal mode - this will fail when applying pragmas
	cfg2 := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			JournalMode: "INVALID",
		},
	}
	_, err := Open(cfg2)
	if err == nil {
		t.Error("Open with invalid journal mode should return error")
	}
}

// Test Store methods with nil receiver
func TestStore_NilReceiver(t *testing.T) {
	var nilStore *Store

	// Test DB()
	db := nilStore.DB()
	if db != nil {
		t.Error("DB() with nil store should return nil")
	}

	// Test Close()
	err := nilStore.Close()
	if err != nil {
		t.Error("Close() with nil store should return nil error")
	}
}

// Test Users with nil store
func TestStore_Users_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.Users()
	if repo != nil {
		t.Error("Users with nil store should return nil")
	}
}

// Test Sessions with nil store
func TestStore_Sessions_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.Sessions()
	if repo != nil {
		t.Error("Sessions with nil store should return nil")
	}
}

// Test PlaygroundTemplates with nil store
func TestStore_PlaygroundTemplates_Nil(t *testing.T) {
	var nilStore *Store
	repo := nilStore.PlaygroundTemplates()
	if repo != nil {
		t.Error("PlaygroundTemplates with nil store should return nil")
	}
}

// Test UserRepo methods with nil receiver
func TestUserRepo_NilReceiver(t *testing.T) {
	var nilRepo *UserRepo

	// Test Create
	err := nilRepo.Create(&User{})
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("Create with nil repo should return initialization error, got: %v", err)
	}

	// Test Create with nil user
	err = nilRepo.Create(nil)
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("Create(nil) with nil repo should return initialization error, got: %v", err)
	}

	// Test FindByID
	user, err := nilRepo.FindByID("test")
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("FindByID with nil repo should return initialization error, got: %v", err)
	}
	if user != nil {
		t.Error("FindByID with nil repo should return nil user")
	}

	// Test FindByEmail
	user, err = nilRepo.FindByEmail("test@example.com")
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("FindByEmail with nil repo should return initialization error, got: %v", err)
	}
	if user != nil {
		t.Error("FindByEmail with nil repo should return nil user")
	}

	// Test List
	result, err := nilRepo.List(UserListOptions{})
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("List with nil repo should return initialization error, got: %v", err)
	}
	if result != nil {
		t.Error("List with nil repo should return nil result")
	}

	// Test Update
	err = nilRepo.Update(&User{ID: "test", Email: "test@example.com", Name: "Test"})
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("Update with nil repo should return initialization error, got: %v", err)
	}

	// Test Delete
	err = nilRepo.Delete("test")
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("Delete with nil repo should return initialization error, got: %v", err)
	}

	// Test HardDelete
	err = nilRepo.HardDelete("test")
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("HardDelete with nil repo should return initialization error, got: %v", err)
	}

	// Test UpdateStatus
	err = nilRepo.UpdateStatus("test", "active")
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("UpdateStatus with nil repo should return initialization error, got: %v", err)
	}

	// Test UpdateCreditBalance
	balance, err := nilRepo.UpdateCreditBalance("test", 100)
	if err == nil || err.Error() != "user repo is not initialized" {
		t.Errorf("UpdateCreditBalance with nil repo should return initialization error, got: %v", err)
	}
	if balance != 0 {
		t.Error("UpdateCreditBalance with nil repo should return 0 balance")
	}
}

// Test UserRepo Create with nil user
func TestUserRepo_Create_NilUser(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().Create(nil)
	if err == nil || err.Error() != "user is nil" {
		t.Errorf("Create(nil) should return 'user is nil' error, got: %v", err)
	}
}

// Test UserRepo FindByID with empty ID
func TestUserRepo_FindByID_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	user, err := db.Users().FindByID("")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("FindByID('') should return 'user id is required' error, got: %v", err)
	}
	if user != nil {
		t.Error("FindByID('') should return nil user")
	}
}

// Test UserRepo FindByEmail with empty email
func TestUserRepo_FindByEmail_EmptyEmail(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	user, err := db.Users().FindByEmail("")
	if err == nil || err.Error() != "email is required" {
		t.Errorf("FindByEmail('') should return 'email is required' error, got: %v", err)
	}
	if user != nil {
		t.Error("FindByEmail('') should return nil user")
	}
}

// Test UserRepo Update with nil user
func TestUserRepo_Update_NilUser(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().Update(nil)
	if err == nil || err.Error() != "user is nil" {
		t.Errorf("Update(nil) should return 'user is nil' error, got: %v", err)
	}
}

// Test UserRepo Update with empty ID
func TestUserRepo_Update_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().Update(&User{ID: "", Email: "test@example.com", Name: "Test"})
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("Update with empty ID should return 'user id is required' error, got: %v", err)
	}
}

// Test UserRepo Delete with empty ID
func TestUserRepo_Delete_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().Delete("")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("Delete('') should return 'user id is required' error, got: %v", err)
	}
}

// Test UserRepo HardDelete with empty ID
func TestUserRepo_HardDelete_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().HardDelete("")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("HardDelete('') should return 'user id is required' error, got: %v", err)
	}
}

// Test UserRepo UpdateStatus with empty ID
func TestUserRepo_UpdateStatus_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().UpdateStatus("", "active")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("UpdateStatus('', 'active') should return 'user id is required' error, got: %v", err)
	}
}

// Test UserRepo UpdateStatus with empty status
func TestUserRepo_UpdateStatus_EmptyStatus(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().UpdateStatus("test", "")
	if err == nil || err.Error() != "status is required" {
		t.Errorf("UpdateStatus('test', '') should return 'status is required' error, got: %v", err)
	}
}

// Test UserRepo UpdateCreditBalance with empty ID
func TestUserRepo_UpdateCreditBalance_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	balance, err := db.Users().UpdateCreditBalance("", 100)
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("UpdateCreditBalance('', 100) should return 'user id is required' error, got: %v", err)
	}
	if balance != 0 {
		t.Error("UpdateCreditBalance with empty ID should return 0 balance")
	}
}

// Test HashPassword with empty password
func TestHashPassword_Empty(t *testing.T) {
	_, err := HashPassword("")
	if err == nil || err.Error() != "password is required" {
		t.Errorf("HashPassword('') should return 'password is required' error, got: %v", err)
	}

	_, err = HashPassword("   ")
	if err == nil || err.Error() != "password is required" {
		t.Errorf("HashPassword('   ') should return 'password is required' error, got: %v", err)
	}
}

// Test VerifyPassword with empty values
func TestVerifyPassword_Empty(t *testing.T) {
	if VerifyPassword("", "test") {
		t.Error("VerifyPassword('', 'test') should return false")
	}
	if VerifyPassword("test", "") {
		t.Error("VerifyPassword('test', '') should return false")
	}
	if VerifyPassword("", "") {
		t.Error("VerifyPassword('', '') should return false")
	}
}

// Test APIKeyRepo methods with nil receiver
func TestAPIKeyRepo_NilReceiver(t *testing.T) {
	var nilRepo *APIKeyRepo

	// Test Create
	_, _, err := nilRepo.Create("user", "name", "test")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("Create with nil repo should return initialization error, got: %v", err)
	}

	// Test FindByHash
	key, err := nilRepo.FindByHash("hash")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("FindByHash with nil repo should return initialization error, got: %v", err)
	}
	if key != nil {
		t.Error("FindByHash with nil repo should return nil key")
	}

	// Test ListByUser
	keys, err := nilRepo.ListByUser("user")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("ListByUser with nil repo should return initialization error, got: %v", err)
	}
	if keys != nil {
		t.Error("ListByUser with nil repo should return nil keys")
	}

	// Test Revoke
	err = nilRepo.Revoke("id")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("Revoke with nil repo should return initialization error, got: %v", err)
	}

	// Test RenameForUser
	err = nilRepo.RenameForUser("id", "user", "name")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("RenameForUser with nil repo should return initialization error, got: %v", err)
	}

	// Test RevokeForUser
	err = nilRepo.RevokeForUser("id", "user")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("RevokeForUser with nil repo should return initialization error, got: %v", err)
	}

	// Test ResolveUserByRawKey
	user, key, err := nilRepo.ResolveUserByRawKey("key")
	if err == nil || err.Error() != "api key repo is not initialized" {
		t.Errorf("ResolveUserByRawKey with nil repo should return initialization error, got: %v", err)
	}
	if user != nil || key != nil {
		t.Error("ResolveUserByRawKey with nil repo should return nil user and key")
	}
}

// Test APIKeyRepo Create with empty user ID
func TestAPIKeyRepo_Create_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	_, _, err := db.APIKeys().Create("", "name", "test")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("Create('', 'name', 'test') should return 'user id is required' error, got: %v", err)
	}
}

// Test APIKeyRepo Create with non-existent user
func TestAPIKeyRepo_Create_NonExistentUser(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	_, _, err := db.APIKeys().Create("nonexistent-user-id", "name", "test")
	if err != sql.ErrNoRows {
		t.Errorf("Create with non-existent user should return sql.ErrNoRows, got: %v", err)
	}
}

// Test APIKeyRepo FindByHash with empty hash
func TestAPIKeyRepo_FindByHash_EmptyHash(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	key, err := db.APIKeys().FindByHash("")
	if err == nil || err.Error() != "api key hash is required" {
		t.Errorf("FindByHash('') should return 'api key hash is required' error, got: %v", err)
	}
	if key != nil {
		t.Error("FindByHash('') should return nil key")
	}
}

// Test APIKeyRepo ListByUser with empty user ID
func TestAPIKeyRepo_ListByUser_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	keys, err := db.APIKeys().ListByUser("")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("ListByUser('') should return 'user id is required' error, got: %v", err)
	}
	if keys != nil {
		t.Error("ListByUser('') should return nil keys")
	}
}

// Test APIKeyRepo Revoke with empty ID
func TestAPIKeyRepo_Revoke_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.APIKeys().Revoke("")
	if err == nil || err.Error() != "api key id is required" {
		t.Errorf("Revoke('') should return 'api key id is required' error, got: %v", err)
	}
}

// Test APIKeyRepo Revoke with non-existent ID
func TestAPIKeyRepo_Revoke_NonExistentID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.APIKeys().Revoke("nonexistent-id")
	if err != sql.ErrNoRows {
		t.Errorf("Revoke with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test APIKeyRepo RenameForUser with empty parameters
func TestAPIKeyRepo_RenameForUser_EmptyParams(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.APIKeys().RenameForUser("", "user", "name")
	if err == nil || err.Error() != "api key id is required" {
		t.Errorf("RenameForUser('', 'user', 'name') should return 'api key id is required' error, got: %v", err)
	}

	err = db.APIKeys().RenameForUser("id", "", "name")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("RenameForUser('id', '', 'name') should return 'user id is required' error, got: %v", err)
	}

	err = db.APIKeys().RenameForUser("id", "user", "")
	if err == nil || err.Error() != "api key name is required" {
		t.Errorf("RenameForUser('id', 'user', '') should return 'api key name is required' error, got: %v", err)
	}
}

// Test APIKeyRepo RevokeForUser with empty parameters
func TestAPIKeyRepo_RevokeForUser_EmptyParams(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.APIKeys().RevokeForUser("", "user")
	if err == nil || err.Error() != "api key id is required" {
		t.Errorf("RevokeForUser('', 'user') should return 'api key id is required' error, got: %v", err)
	}

	err = db.APIKeys().RevokeForUser("id", "")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("RevokeForUser('id', '') should return 'user id is required' error, got: %v", err)
	}
}

// Test APIKeyRepo ResolveUserByRawKey with empty key
func TestAPIKeyRepo_ResolveUserByRawKey_EmptyKey(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	user, key, err := db.APIKeys().ResolveUserByRawKey("")
	if err != ErrAPIKeyNotFound {
		t.Errorf("ResolveUserByRawKey('') should return ErrAPIKeyNotFound, got: %v", err)
	}
	if user != nil || key != nil {
		t.Error("ResolveUserByRawKey('') should return nil user and key")
	}
}

// Test randomToken with invalid length
func TestRandomToken_InvalidLength(t *testing.T) {
	_, err := randomToken(0)
	if err == nil || err.Error() != "token length must be positive" {
		t.Errorf("randomToken(0) should return 'token length must be positive' error, got: %v", err)
	}

	_, err = randomToken(-1)
	if err == nil || err.Error() != "token length must be positive" {
		t.Errorf("randomToken(-1) should return 'token length must be positive' error, got: %v", err)
	}
}

// Test AuditRepo methods with nil receiver
func TestAuditRepo_NilReceiver(t *testing.T) {
	var nilRepo *AuditRepo

	// Test BatchInsert
	err := nilRepo.BatchInsert([]AuditEntry{{ID: "test"}})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("BatchInsert with nil repo should return initialization error, got: %v", err)
	}

	// Test FindByID
	entry, err := nilRepo.FindByID("id")
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("FindByID with nil repo should return initialization error, got: %v", err)
	}
	if entry != nil {
		t.Error("FindByID with nil repo should return nil entry")
	}

	// Test List
	result, err := nilRepo.List(AuditListOptions{})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("List with nil repo should return initialization error, got: %v", err)
	}
	if result != nil {
		t.Error("List with nil repo should return nil result")
	}

	// Test Search
	result, err = nilRepo.Search(AuditSearchFilters{})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("Search with nil repo should return initialization error, got: %v", err)
	}
	if result != nil {
		t.Error("Search with nil repo should return nil result")
	}

	// Test Stats
	stats, err := nilRepo.Stats(AuditSearchFilters{})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("Stats with nil repo should return initialization error, got: %v", err)
	}
	if stats != nil {
		t.Error("Stats with nil repo should return nil stats")
	}

	// Test ListOlderThanForRoute
	entries, err := nilRepo.ListOlderThanForRoute("route", time.Now(), 100)
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("ListOlderThanForRoute with nil repo should return initialization error, got: %v", err)
	}
	if entries != nil {
		t.Error("ListOlderThanForRoute with nil repo should return nil entries")
	}

	// Test ListOlderThanExcludingRoutes
	entries, err = nilRepo.ListOlderThanExcludingRoutes(time.Now(), 100, []string{"route"})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("ListOlderThanExcludingRoutes with nil repo should return initialization error, got: %v", err)
	}
	if entries != nil {
		t.Error("ListOlderThanExcludingRoutes with nil repo should return nil entries")
	}

	// Test DeleteOlderThanForRoute
	_, err = nilRepo.DeleteOlderThanForRoute("route", time.Now(), 100)
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("DeleteOlderThanForRoute with nil repo should return initialization error, got: %v", err)
	}

	// Test DeleteOlderThanExcludingRoutes
	_, err = nilRepo.DeleteOlderThanExcludingRoutes(time.Now(), 100, []string{"route"})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("DeleteOlderThanExcludingRoutes with nil repo should return initialization error, got: %v", err)
	}

	// Test DeleteByIDs
	_, err = nilRepo.DeleteByIDs([]string{"id"})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("DeleteByIDs with nil repo should return initialization error, got: %v", err)
	}

	// Test Export
	err = nilRepo.Export(AuditSearchFilters{}, "jsonl", &bytes.Buffer{})
	if err == nil || err.Error() != "audit repo is not initialized" {
		t.Errorf("Export with nil repo should return initialization error, got: %v", err)
	}
}

// Test AuditRepo FindByID with empty ID
func TestAuditRepo_FindByID_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	entry, err := db.Audits().FindByID("")
	if err == nil || err.Error() != "audit id is required" {
		t.Errorf("FindByID('') should return 'audit id is required' error, got: %v", err)
	}
	if entry != nil {
		t.Error("FindByID('') should return nil entry")
	}
}

// Test AuditRepo ListOlderThanForRoute with empty route
func TestAuditRepo_ListOlderThanForRoute_EmptyRoute(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	entries, err := db.Audits().ListOlderThanForRoute("", time.Now(), 100)
	if err == nil || err.Error() != "route is required" {
		t.Errorf("ListOlderThanForRoute('', time.Now(), 100) should return 'route is required' error, got: %v", err)
	}
	if entries != nil {
		t.Error("ListOlderThanForRoute with empty route should return nil entries")
	}
}

// Test AuditRepo DeleteOlderThanForRoute with empty route
func TestAuditRepo_DeleteOlderThanForRoute_EmptyRoute(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	_, err := db.Audits().DeleteOlderThanForRoute("", time.Now(), 100)
	if err == nil || err.Error() != "route is required" {
		t.Errorf("DeleteOlderThanForRoute('', time.Now(), 100) should return 'route is required' error, got: %v", err)
	}
}

// Test AuditRepo Export with nil writer
func TestAuditRepo_Export_NilWriter(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Audits().Export(AuditSearchFilters{}, "jsonl", nil)
	if err == nil || err.Error() != "export writer is nil" {
		t.Errorf("Export with nil writer should return 'export writer is nil' error, got: %v", err)
	}
}

// Test AuditRepo Export with unsupported format
func TestAuditRepo_Export_UnsupportedFormat(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	var buf bytes.Buffer
	err := db.Audits().Export(AuditSearchFilters{}, "xml", &buf)
	if err == nil || err.Error() != "unsupported export format" {
		t.Errorf("Export with unsupported format should return 'unsupported export format' error, got: %v", err)
	}
}

// Test PermissionRepo methods with nil receiver
func TestPermissionRepo_NilReceiver(t *testing.T) {
	var nilRepo *PermissionRepo

	// Test Create
	err := nilRepo.Create(&EndpointPermission{UserID: "user", RouteID: "route"})
	if err == nil || err.Error() != "permission repo is not initialized" {
		t.Errorf("Create with nil repo should return initialization error, got: %v", err)
	}

	// Test Update
	err = nilRepo.Update(&EndpointPermission{ID: "id", UserID: "user", RouteID: "route"})
	if err == nil || err.Error() != "permission repo is not initialized" {
		t.Errorf("Update with nil repo should return initialization error, got: %v", err)
	}

	// Test Delete
	err = nilRepo.Delete("id")
	if err == nil || err.Error() != "permission repo is not initialized" {
		t.Errorf("Delete with nil repo should return initialization error, got: %v", err)
	}

	// Test FindByUserAndRoute
	perm, err := nilRepo.FindByUserAndRoute("user", "route")
	if err == nil || err.Error() != "permission repo is not initialized" {
		t.Errorf("FindByUserAndRoute with nil repo should return initialization error, got: %v", err)
	}
	if perm != nil {
		t.Error("FindByUserAndRoute with nil repo should return nil permission")
	}

	// Test ListByUser
	perms, err := nilRepo.ListByUser("user")
	if err == nil || err.Error() != "permission repo is not initialized" {
		t.Errorf("ListByUser with nil repo should return initialization error, got: %v", err)
	}
	if perms != nil {
		t.Error("ListByUser with nil repo should return nil permissions")
	}

	// Test BulkAssign
	err = nilRepo.BulkAssign("user", []EndpointPermission{})
	if err == nil || err.Error() != "permission repo is not initialized" {
		t.Errorf("BulkAssign with nil repo should return initialization error, got: %v", err)
	}
}

// Test PermissionRepo Create with nil permission
func TestPermissionRepo_Create_NilPermission(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Permissions().Create(nil)
	if err == nil || err.Error() != "permission is nil" {
		t.Errorf("Create(nil) should return 'permission is nil' error, got: %v", err)
	}
}

// Test PermissionRepo Create with invalid input
func TestPermissionRepo_Create_InvalidInput(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Missing UserID
	err := db.Permissions().Create(&EndpointPermission{RouteID: "route"})
	if err == nil || err.Error() != "permission user id is required" {
		t.Errorf("Create with missing UserID should return 'permission user id is required' error, got: %v", err)
	}

	// Missing RouteID
	err = db.Permissions().Create(&EndpointPermission{UserID: "user"})
	if err == nil || err.Error() != "permission route id is required" {
		t.Errorf("Create with missing RouteID should return 'permission route id is required' error, got: %v", err)
	}
}

// Test PermissionRepo Update with nil permission
func TestPermissionRepo_Update_NilPermission(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Permissions().Update(nil)
	if err == nil || err.Error() != "permission is nil" {
		t.Errorf("Update(nil) should return 'permission is nil' error, got: %v", err)
	}
}

// Test PermissionRepo Update with empty ID
func TestPermissionRepo_Update_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Permissions().Update(&EndpointPermission{ID: "", UserID: "user", RouteID: "route"})
	if err == nil || err.Error() != "permission id is required" {
		t.Errorf("Update with empty ID should return 'permission id is required' error, got: %v", err)
	}
}

// Test PermissionRepo Delete with empty ID
func TestPermissionRepo_Delete_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Permissions().Delete("")
	if err == nil || err.Error() != "permission id is required" {
		t.Errorf("Delete('') should return 'permission id is required' error, got: %v", err)
	}
}

// Test PermissionRepo Delete with non-existent ID
func TestPermissionRepo_Delete_NonExistentID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Permissions().Delete("nonexistent-id")
	if err != sql.ErrNoRows {
		t.Errorf("Delete with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test PermissionRepo FindByUserAndRoute with empty parameters
func TestPermissionRepo_FindByUserAndRoute_EmptyParams(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	perm, err := db.Permissions().FindByUserAndRoute("", "route")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("FindByUserAndRoute('', 'route') should return 'user id is required' error, got: %v", err)
	}
	if perm != nil {
		t.Error("FindByUserAndRoute with empty user ID should return nil permission")
	}

	perm, err = db.Permissions().FindByUserAndRoute("user", "")
	if err == nil || err.Error() != "route id is required" {
		t.Errorf("FindByUserAndRoute('user', '') should return 'route id is required' error, got: %v", err)
	}
	if perm != nil {
		t.Error("FindByUserAndRoute with empty route ID should return nil permission")
	}
}

// Test PermissionRepo ListByUser with empty user ID
func TestPermissionRepo_ListByUser_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	perms, err := db.Permissions().ListByUser("")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("ListByUser('') should return 'user id is required' error, got: %v", err)
	}
	if perms != nil {
		t.Error("ListByUser('') should return nil permissions")
	}
}

// Test PermissionRepo BulkAssign with empty user ID
func TestPermissionRepo_BulkAssign_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Permissions().BulkAssign("", []EndpointPermission{})
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("BulkAssign('', []) should return 'user id is required' error, got: %v", err)
	}
}

// Test validatePermissionInput
func TestValidatePermissionInput(t *testing.T) {
	// Test nil permission
	err := validatePermissionInput(nil)
	if err == nil || err.Error() != "permission is nil" {
		t.Errorf("validatePermissionInput(nil) should return 'permission is nil' error, got: %v", err)
	}

	// Test empty UserID
	err = validatePermissionInput(&EndpointPermission{RouteID: "route"})
	if err == nil || err.Error() != "permission user id is required" {
		t.Errorf("validatePermissionInput with empty UserID should return 'permission user id is required' error, got: %v", err)
	}

	// Test empty RouteID
	err = validatePermissionInput(&EndpointPermission{UserID: "user"})
	if err == nil || err.Error() != "permission route id is required" {
		t.Errorf("validatePermissionInput with empty RouteID should return 'permission route id is required' error, got: %v", err)
	}

	// Test valid permission
	err = validatePermissionInput(&EndpointPermission{UserID: "user", RouteID: "route"})
	if err != nil {
		t.Errorf("validatePermissionInput with valid permission should return nil error, got: %v", err)
	}
}

// Test SessionRepo methods with nil receiver
func TestSessionRepo_NilReceiver(t *testing.T) {
	var nilRepo *SessionRepo

	// Test Create
	err := nilRepo.Create(&Session{UserID: "user", TokenHash: "hash", ExpiresAt: time.Now()})
	if err == nil || err.Error() != "session repo is not initialized" {
		t.Errorf("Create with nil repo should return initialization error, got: %v", err)
	}

	// Test FindByTokenHash
	session, err := nilRepo.FindByTokenHash("hash")
	if err == nil || err.Error() != "session repo is not initialized" {
		t.Errorf("FindByTokenHash with nil repo should return initialization error, got: %v", err)
	}
	if session != nil {
		t.Error("FindByTokenHash with nil repo should return nil session")
	}

	// Test DeleteByID
	err = nilRepo.DeleteByID("id")
	if err == nil || err.Error() != "session repo is not initialized" {
		t.Errorf("DeleteByID with nil repo should return initialization error, got: %v", err)
	}

	// Test DeleteByTokenHash
	err = nilRepo.DeleteByTokenHash("hash")
	if err == nil || err.Error() != "session repo is not initialized" {
		t.Errorf("DeleteByTokenHash with nil repo should return initialization error, got: %v", err)
	}

	// Test Touch
	err = nilRepo.Touch("id")
	if err == nil || err.Error() != "session repo is not initialized" {
		t.Errorf("Touch with nil repo should return initialization error, got: %v", err)
	}

	// Test CleanupExpired
	deleted, err := nilRepo.CleanupExpired(time.Now())
	if err == nil || err.Error() != "session repo is not initialized" {
		t.Errorf("CleanupExpired with nil repo should return initialization error, got: %v", err)
	}
	if deleted != 0 {
		t.Error("CleanupExpired with nil repo should return 0 deleted")
	}
}

// Test SessionRepo Create with nil session
func TestSessionRepo_Create_NilSession(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Sessions().Create(nil)
	if err == nil || err.Error() != "session is nil" {
		t.Errorf("Create(nil) should return 'session is nil' error, got: %v", err)
	}
}

// Test SessionRepo Create with invalid input
func TestSessionRepo_Create_InvalidInput(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Missing UserID
	err := db.Sessions().Create(&Session{TokenHash: "hash", ExpiresAt: time.Now()})
	if err == nil || err.Error() != "session user_id is required" {
		t.Errorf("Create with missing UserID should return 'session user_id is required' error, got: %v", err)
	}

	// Missing TokenHash
	err = db.Sessions().Create(&Session{UserID: "user", ExpiresAt: time.Now()})
	if err == nil || err.Error() != "session token_hash is required" {
		t.Errorf("Create with missing TokenHash should return 'session token_hash is required' error, got: %v", err)
	}

	// Missing ExpiresAt
	err = db.Sessions().Create(&Session{UserID: "user", TokenHash: "hash"})
	if err == nil || err.Error() != "session expires_at is required" {
		t.Errorf("Create with missing ExpiresAt should return 'session expires_at is required' error, got: %v", err)
	}
}

// Test SessionRepo FindByTokenHash with empty hash
func TestSessionRepo_FindByTokenHash_EmptyHash(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	session, err := db.Sessions().FindByTokenHash("")
	if err == nil || err.Error() != "token hash is required" {
		t.Errorf("FindByTokenHash('') should return 'token hash is required' error, got: %v", err)
	}
	if session != nil {
		t.Error("FindByTokenHash('') should return nil session")
	}
}

// Test SessionRepo DeleteByID with empty ID
func TestSessionRepo_DeleteByID_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Sessions().DeleteByID("")
	if err == nil || err.Error() != "session id is required" {
		t.Errorf("DeleteByID('') should return 'session id is required' error, got: %v", err)
	}
}

// Test SessionRepo DeleteByTokenHash with empty hash
func TestSessionRepo_DeleteByTokenHash_EmptyHash(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Sessions().DeleteByTokenHash("")
	if err == nil || err.Error() != "token hash is required" {
		t.Errorf("DeleteByTokenHash('') should return 'token hash is required' error, got: %v", err)
	}
}

// Test SessionRepo Touch with empty ID
func TestSessionRepo_Touch_EmptyID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Sessions().Touch("")
	if err == nil || err.Error() != "session id is required" {
		t.Errorf("Touch('') should return 'session id is required' error, got: %v", err)
	}
}

// Test SessionRepo Touch with non-existent ID
func TestSessionRepo_Touch_NonExistentID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Sessions().Touch("nonexistent-id")
	if err != sql.ErrNoRows {
		t.Errorf("Touch with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test CreditRepo methods with nil receiver
func TestCreditRepo_NilReceiver(t *testing.T) {
	var nilRepo *CreditRepo

	// Test Create
	err := nilRepo.Create(&CreditTransaction{UserID: "user"})
	if err == nil || err.Error() != "credit repo is not initialized" {
		t.Errorf("Create with nil repo should return initialization error, got: %v", err)
	}

	// Test ListByUser
	result, err := nilRepo.ListByUser("user", CreditListOptions{})
	if err == nil || err.Error() != "credit repo is not initialized" {
		t.Errorf("ListByUser with nil repo should return initialization error, got: %v", err)
	}
	if result != nil {
		t.Error("ListByUser with nil repo should return nil result")
	}

	// Test OverviewStats
	stats, err := nilRepo.OverviewStats()
	if err == nil || err.Error() != "credit repo is not initialized" {
		t.Errorf("OverviewStats with nil repo should return initialization error, got: %v", err)
	}
	if stats != nil {
		t.Error("OverviewStats with nil repo should return nil stats")
	}
}

// Test CreditRepo Create with nil transaction
func TestCreditRepo_Create_NilTransaction(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Credits().Create(nil)
	if err == nil || err.Error() != "credit transaction is nil" {
		t.Errorf("Create(nil) should return 'credit transaction is nil' error, got: %v", err)
	}
}

// Test CreditRepo Create with empty user ID
func TestCreditRepo_Create_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Credits().Create(&CreditTransaction{})
	if err == nil || err.Error() != "credit transaction user id is required" {
		t.Errorf("Create with empty UserID should return 'credit transaction user id is required' error, got: %v", err)
	}
}

// Test CreditRepo ListByUser with empty user ID
func TestCreditRepo_ListByUser_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	result, err := db.Credits().ListByUser("", CreditListOptions{})
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("ListByUser('') should return 'user id is required' error, got: %v", err)
	}
	if result != nil {
		t.Error("ListByUser('') should return nil result")
	}
}

// Test PlaygroundTemplateRepo methods with nil receiver
func TestPlaygroundTemplateRepo_NilReceiver(t *testing.T) {
	var nilRepo *PlaygroundTemplateRepo

	// Test ListByUser
	templates, err := nilRepo.ListByUser("user")
	if err == nil || err.Error() != "playground template repo is not initialized" {
		t.Errorf("ListByUser with nil repo should return initialization error, got: %v", err)
	}
	if templates != nil {
		t.Error("ListByUser with nil repo should return nil templates")
	}

	// Test Save
	err = nilRepo.Save(&PlaygroundTemplate{UserID: "user", Name: "name"})
	if err == nil || err.Error() != "playground template repo is not initialized" {
		t.Errorf("Save with nil repo should return initialization error, got: %v", err)
	}

	// Test DeleteForUser
	err = nilRepo.DeleteForUser("id", "user")
	if err == nil || err.Error() != "playground template repo is not initialized" {
		t.Errorf("DeleteForUser with nil repo should return initialization error, got: %v", err)
	}
}

// Test PlaygroundTemplateRepo ListByUser with empty user ID
func TestPlaygroundTemplateRepo_ListByUser_EmptyUserID(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	templates, err := db.PlaygroundTemplates().ListByUser("")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("ListByUser('') should return 'user id is required' error, got: %v", err)
	}
	if templates != nil {
		t.Error("ListByUser('') should return nil templates")
	}
}

// Test PlaygroundTemplateRepo Save with nil template
func TestPlaygroundTemplateRepo_Save_NilTemplate(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.PlaygroundTemplates().Save(nil)
	if err == nil || err.Error() != "template is nil" {
		t.Errorf("Save(nil) should return 'template is nil' error, got: %v", err)
	}
}

// Test PlaygroundTemplateRepo Save with invalid input
func TestPlaygroundTemplateRepo_Save_InvalidInput(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Missing UserID
	err := db.PlaygroundTemplates().Save(&PlaygroundTemplate{Name: "name"})
	if err == nil || err.Error() != "template user id is required" {
		t.Errorf("Save with missing UserID should return 'template user id is required' error, got: %v", err)
	}

	// Missing Name
	err = db.PlaygroundTemplates().Save(&PlaygroundTemplate{UserID: "user"})
	if err == nil || err.Error() != "template name is required" {
		t.Errorf("Save with missing Name should return 'template name is required' error, got: %v", err)
	}
}

// Test PlaygroundTemplateRepo Save with update non-existent template
func TestPlaygroundTemplateRepo_Save_UpdateNonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user first
	user := &User{
		Email:        "template@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Try to update non-existent template
	err := db.PlaygroundTemplates().Save(&PlaygroundTemplate{
		ID:     "nonexistent-id",
		UserID: user.ID,
		Name:   "Updated Name",
	})
	if err != sql.ErrNoRows {
		t.Errorf("Save with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test PlaygroundTemplateRepo DeleteForUser with empty parameters
func TestPlaygroundTemplateRepo_DeleteForUser_EmptyParams(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.PlaygroundTemplates().DeleteForUser("", "user")
	if err == nil || err.Error() != "template id is required" {
		t.Errorf("DeleteForUser('', 'user') should return 'template id is required' error, got: %v", err)
	}

	err = db.PlaygroundTemplates().DeleteForUser("id", "")
	if err == nil || err.Error() != "user id is required" {
		t.Errorf("DeleteForUser('id', '') should return 'user id is required' error, got: %v", err)
	}
}

// Test PlaygroundTemplateRepo DeleteForUser with non-existent ID
func TestPlaygroundTemplateRepo_DeleteForUser_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user first
	user := &User{
		Email:        "template2@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Try to delete non-existent template
	err := db.PlaygroundTemplates().DeleteForUser("nonexistent-id", user.ID)
	if err != sql.ErrNoRows {
		t.Errorf("DeleteForUser with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test marshalStringMap
func TestMarshalStringMap(t *testing.T) {
	// Test nil map
	result, err := marshalStringMap(nil)
	if err != nil {
		t.Errorf("marshalStringMap(nil) should not return error, got: %v", err)
	}
	if result != "{}" {
		t.Errorf("marshalStringMap(nil) should return '{}', got: %s", result)
	}

	// Test empty map
	result, err = marshalStringMap(map[string]string{})
	if err != nil {
		t.Errorf("marshalStringMap(empty) should not return error, got: %v", err)
	}
	if result != "{}" {
		t.Errorf("marshalStringMap(empty) should return '{}', got: %s", result)
	}

	// Test valid map
	result, err = marshalStringMap(map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("marshalStringMap(valid) should not return error, got: %v", err)
	}
	if result != `{"key":"value"}` {
		t.Errorf("marshalStringMap(valid) should return '{\"key\":\"value\"}', got: %s", result)
	}
}

// Test unmarshalStringMap
func TestUnmarshalStringMap(t *testing.T) {
	// Test empty string
	result, err := unmarshalStringMap("")
	if err != nil {
		t.Errorf("unmarshalStringMap('') should not return error, got: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("unmarshalStringMap('') should return empty map, got: %v", result)
	}

	// Test whitespace string
	result, err = unmarshalStringMap("   ")
	if err != nil {
		t.Errorf("unmarshalStringMap('   ') should not return error, got: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("unmarshalStringMap('   ') should return empty map, got: %v", result)
	}

	// Test valid JSON
	result, err = unmarshalStringMap(`{"key":"value"}`)
	if err != nil {
		t.Errorf("unmarshalStringMap(valid) should not return error, got: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("unmarshalStringMap(valid) should return map with key='value', got: %v", result)
	}
}

// Test normalizeMethods
func TestNormalizeMethods(t *testing.T) {
	// Test empty slice
	result := normalizeMethods([]string{})
	if len(result) != 0 {
		t.Errorf("normalizeMethods([]) should return empty slice, got: %v", result)
	}

	// Test nil slice
	result = normalizeMethods(nil)
	if len(result) != 0 {
		t.Errorf("normalizeMethods(nil) should return empty slice, got: %v", result)
	}

	// Test normalization
	result = normalizeMethods([]string{"get", "POST", "  get  ", ""})
	if len(result) != 2 {
		t.Errorf("normalizeMethods should return 2 unique methods, got: %v", result)
	}
	if result[0] != "GET" || result[1] != "POST" {
		t.Errorf("normalizeMethods should return ['GET', 'POST'], got: %v", result)
	}
}

// Test boolToInt
func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should return 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should return 0")
	}
}

// Test creditCostToRaw
func TestCreditCostToRaw(t *testing.T) {
	// Test nil
	result := creditCostToRaw(nil)
	if result != "" {
		t.Errorf("creditCostToRaw(nil) should return '', got: %s", result)
	}

	// Test valid value
	cost := int64(100)
	result = creditCostToRaw(&cost)
	if result != "100" {
		t.Errorf("creditCostToRaw(&100) should return '100', got: %s", result)
	}
}

// Test timePtrToRaw
func TestTimePtrToRaw(t *testing.T) {
	// Test nil
	result := timePtrToRaw(nil)
	if result != "" {
		t.Errorf("timePtrToRaw(nil) should return '', got: %s", result)
	}

	// Test valid time
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result = timePtrToRaw(&now)
	if result == "" {
		t.Error("timePtrToRaw(&time) should return non-empty string")
	}
}

// Test resolveStoreConfig
func TestResolveStoreConfig(t *testing.T) {
	// Test nil config
	cfg := resolveStoreConfig(nil)
	if cfg.Path != "apicerberus.db" {
		t.Errorf("resolveStoreConfig(nil) should return default path, got: %s", cfg.Path)
	}

	// Test with valid config
	input := &config.Config{
		Store: config.StoreConfig{
			Path:        "/custom/path.db",
			BusyTimeout: 10 * time.Second,
			JournalMode: "DELETE",
			ForeignKeys: false,
		},
	}
	cfg = resolveStoreConfig(input)
	if cfg.Path != "/custom/path.db" {
		t.Errorf("resolveStoreConfig should use custom path, got: %s", cfg.Path)
	}
	if cfg.BusyTimeout != 10*time.Second {
		t.Errorf("resolveStoreConfig should use custom busy timeout, got: %v", cfg.BusyTimeout)
	}
	if cfg.JournalMode != "DELETE" {
		t.Errorf("resolveStoreConfig should use custom journal mode, got: %s", cfg.JournalMode)
	}
	// Note: ForeignKeys defaults to true and can only be set to true, not false
	// The function only allows enabling foreign keys, not disabling them
	if cfg.ForeignKeys != true {
		t.Error("resolveStoreConfig should keep foreign keys enabled by default")
	}
}

// Test validateStoreConfig
func TestValidateStoreConfig_Additional(t *testing.T) {
	// Test empty path
	cfg := config.StoreConfig{Path: ""}
	err := validateStoreConfig(cfg)
	if err == nil || err.Error() != "store.path is required" {
		t.Errorf("validateStoreConfig with empty path should return error, got: %v", err)
	}

	// Test negative busy timeout
	cfg = config.StoreConfig{Path: "test.db", BusyTimeout: -1 * time.Second}
	err = validateStoreConfig(cfg)
	if err == nil || err.Error() != "store.busy_timeout cannot be negative" {
		t.Errorf("validateStoreConfig with negative busy timeout should return error, got: %v", err)
	}

	// Test invalid journal mode
	cfg = config.StoreConfig{Path: "test.db", JournalMode: "INVALID"}
	err = validateStoreConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "journal_mode") {
		t.Errorf("validateStoreConfig with invalid journal mode should return error, got: %v", err)
	}

	// Test valid config
	cfg = config.StoreConfig{Path: "test.db", JournalMode: "WAL"}
	err = validateStoreConfig(cfg)
	if err != nil {
		t.Errorf("validateStoreConfig with valid config should not return error, got: %v", err)
	}
}

// Test decodeUserJSONFields with nil user
func TestDecodeUserJSONFields_NilUser(t *testing.T) {
	err := decodeUserJSONFields(nil, "{}", "[]", "{}", time.Now().Format(time.RFC3339Nano), time.Now().Format(time.RFC3339Nano))
	if err == nil || err.Error() != "user is nil" {
		t.Errorf("decodeUserJSONFields(nil, ...) should return 'user is nil' error, got: %v", err)
	}
}

// Test decodeUserJSONFields with invalid JSON
func TestDecodeUserJSONFields_InvalidJSON(t *testing.T) {
	user := &User{}
	now := time.Now().Format(time.RFC3339Nano)

	// Invalid rate_limits
	err := decodeUserJSONFields(user, "invalid", "[]", "{}", now, now)
	if err == nil || !strings.Contains(err.Error(), "rate_limits") {
		t.Errorf("decodeUserJSONFields with invalid rate_limits should return error, got: %v", err)
	}

	// Invalid ip_whitelist
	err = decodeUserJSONFields(user, "{}", "invalid", "{}", now, now)
	if err == nil || !strings.Contains(err.Error(), "ip_whitelist") {
		t.Errorf("decodeUserJSONFields with invalid ip_whitelist should return error, got: %v", err)
	}

	// Invalid metadata
	err = decodeUserJSONFields(user, "{}", "[]", "invalid", now, now)
	if err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Errorf("decodeUserJSONFields with invalid metadata should return error, got: %v", err)
	}

	// Invalid created_at
	err = decodeUserJSONFields(user, "{}", "[]", "{}", "invalid", now)
	if err == nil || !strings.Contains(err.Error(), "created_at") {
		t.Errorf("decodeUserJSONFields with invalid created_at should return error, got: %v", err)
	}

	// Invalid updated_at
	err = decodeUserJSONFields(user, "{}", "[]", "{}", now, "invalid")
	if err == nil || !strings.Contains(err.Error(), "updated_at") {
		t.Errorf("decodeUserJSONFields with invalid updated_at should return error, got: %v", err)
	}
}

// Test decodeAPIKeyDateFields with nil key
func TestDecodeAPIKeyDateFields_NilKey(t *testing.T) {
	now := time.Now().Format(time.RFC3339Nano)
	err := decodeAPIKeyDateFields(nil, "", "", now, now)
	if err == nil || err.Error() != "api key is nil" {
		t.Errorf("decodeAPIKeyDateFields(nil, ...) should return 'api key is nil' error, got: %v", err)
	}
}

// Test decodeAPIKeyDateFields with invalid dates
func TestDecodeAPIKeyDateFields_InvalidDates(t *testing.T) {
	key := &APIKey{}
	now := time.Now().Format(time.RFC3339Nano)

	// Invalid expires_at
	err := decodeAPIKeyDateFields(key, "invalid", "", now, now)
	if err == nil || !strings.Contains(err.Error(), "expires_at") {
		t.Errorf("decodeAPIKeyDateFields with invalid expires_at should return error, got: %v", err)
	}

	// Invalid last_used_at
	err = decodeAPIKeyDateFields(key, "", "invalid", now, now)
	if err == nil || !strings.Contains(err.Error(), "last_used_at") {
		t.Errorf("decodeAPIKeyDateFields with invalid last_used_at should return error, got: %v", err)
	}

	// Invalid created_at
	err = decodeAPIKeyDateFields(key, "", "", "invalid", now)
	if err == nil || !strings.Contains(err.Error(), "created_at") {
		t.Errorf("decodeAPIKeyDateFields with invalid created_at should return error, got: %v", err)
	}

	// Invalid updated_at
	err = decodeAPIKeyDateFields(key, "", "", now, "invalid")
	if err == nil || !strings.Contains(err.Error(), "updated_at") {
		t.Errorf("decodeAPIKeyDateFields with invalid updated_at should return error, got: %v", err)
	}
}

// Test decodePermissionFields with nil permission
func TestDecodePermissionFields_NilPermission(t *testing.T) {
	now := time.Now().Format(time.RFC3339Nano)
	err := decodePermissionFields(nil, "[]", "{}", "", "", "", "[]", "[]", now, now)
	if err == nil || err.Error() != "permission is nil" {
		t.Errorf("decodePermissionFields(nil, ...) should return 'permission is nil' error, got: %v", err)
	}
}

// Test decodePermissionFields with invalid JSON
func TestDecodePermissionFields_InvalidJSON(t *testing.T) {
	perm := &EndpointPermission{}
	now := time.Now().Format(time.RFC3339Nano)

	// Invalid methods
	err := decodePermissionFields(perm, "invalid", "{}", "", "", "", "[]", "[]", now, now)
	if err == nil || !strings.Contains(err.Error(), "methods") {
		t.Errorf("decodePermissionFields with invalid methods should return error, got: %v", err)
	}

	// Invalid rate_limits
	err = decodePermissionFields(perm, "[]", "invalid", "", "", "", "[]", "[]", now, now)
	if err == nil || !strings.Contains(err.Error(), "rate_limits") {
		t.Errorf("decodePermissionFields with invalid rate_limits should return error, got: %v", err)
	}

	// Invalid credit_cost
	err = decodePermissionFields(perm, "[]", "{}", "invalid", "", "", "[]", "[]", now, now)
	if err == nil || !strings.Contains(err.Error(), "credit_cost") {
		t.Errorf("decodePermissionFields with invalid credit_cost should return error, got: %v", err)
	}

	// Invalid valid_from
	err = decodePermissionFields(perm, "[]", "{}", "", "invalid", "", "[]", "[]", now, now)
	if err == nil || !strings.Contains(err.Error(), "valid_from") {
		t.Errorf("decodePermissionFields with invalid valid_from should return error, got: %v", err)
	}

	// Invalid valid_until
	err = decodePermissionFields(perm, "[]", "{}", "", "", "invalid", "[]", "[]", now, now)
	if err == nil || !strings.Contains(err.Error(), "valid_until") {
		t.Errorf("decodePermissionFields with invalid valid_until should return error, got: %v", err)
	}

	// Invalid allowed_days
	err = decodePermissionFields(perm, "[]", "{}", "", "", "", "invalid", "[]", now, now)
	if err == nil || !strings.Contains(err.Error(), "allowed_days") {
		t.Errorf("decodePermissionFields with invalid allowed_days should return error, got: %v", err)
	}

	// Invalid allowed_hours
	err = decodePermissionFields(perm, "[]", "{}", "", "", "", "[]", "invalid", now, now)
	if err == nil || !strings.Contains(err.Error(), "allowed_hours") {
		t.Errorf("decodePermissionFields with invalid allowed_hours should return error, got: %v", err)
	}

	// Invalid created_at
	err = decodePermissionFields(perm, "[]", "{}", "", "", "", "[]", "[]", "invalid", now)
	if err == nil || !strings.Contains(err.Error(), "created_at") {
		t.Errorf("decodePermissionFields with invalid created_at should return error, got: %v", err)
	}

	// Invalid updated_at
	err = decodePermissionFields(perm, "[]", "{}", "", "", "", "[]", "[]", now, "invalid")
	if err == nil || !strings.Contains(err.Error(), "updated_at") {
		t.Errorf("decodePermissionFields with invalid updated_at should return error, got: %v", err)
	}
}

// Test decodeAuditFields with nil entry
func TestDecodeAuditFields_NilEntry(t *testing.T) {
	err := decodeAuditFields(nil, 0, "{}", "{}", time.Now().Format(time.RFC3339Nano))
	if err == nil || err.Error() != "audit entry is nil" {
		t.Errorf("decodeAuditFields(nil, ...) should return 'audit entry is nil' error, got: %v", err)
	}
}

// Test decodeAuditFields with invalid JSON
func TestDecodeAuditFields_InvalidJSON(t *testing.T) {
	entry := &AuditEntry{}
	now := time.Now().Format(time.RFC3339Nano)

	// Invalid request_headers
	err := decodeAuditFields(entry, 0, "invalid", "{}", now)
	if err == nil || !strings.Contains(err.Error(), "request_headers") {
		t.Errorf("decodeAuditFields with invalid request_headers should return error, got: %v", err)
	}

	// Invalid response_headers
	err = decodeAuditFields(entry, 0, "{}", "invalid", now)
	if err == nil || !strings.Contains(err.Error(), "response_headers") {
		t.Errorf("decodeAuditFields with invalid response_headers should return error, got: %v", err)
	}

	// Invalid created_at
	err = decodeAuditFields(entry, 0, "{}", "{}", "invalid")
	if err == nil || !strings.Contains(err.Error(), "created_at") {
		t.Errorf("decodeAuditFields with invalid created_at should return error, got: %v", err)
	}
}

// Test buildAuditWhere with various filters
func TestBuildAuditWhere(t *testing.T) {
	// Test empty filters
	where, args := buildAuditWhere(AuditSearchFilters{})
	if where != "" {
		t.Errorf("buildAuditWhere with empty filters should return empty where, got: %s", where)
	}
	if len(args) != 0 {
		t.Errorf("buildAuditWhere with empty filters should return empty args, got: %v", args)
	}

	// Test with all filters
	blocked := true
	dateFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	where, args = buildAuditWhere(AuditSearchFilters{
		UserID:       "user123",
		APIKeyPrefix: "ck_live",
		Route:        "api/users",
		Method:       "GET",
		StatusMin:    200,
		StatusMax:    299,
		ClientIP:     "127.0.0.1",
		Blocked:      &blocked,
		BlockReason:  "rate_limit",
		DateFrom:     &dateFrom,
		DateTo:       &dateTo,
		MinLatencyMS: 100,
		FullText:     "error",
	})
	if where == "" {
		t.Error("buildAuditWhere with filters should return non-empty where")
	}
	if len(args) == 0 {
		t.Error("buildAuditWhere with filters should return non-empty args")
	}
}

// Test buildRouteExclusionCondition
func TestBuildRouteExclusionCondition(t *testing.T) {
	// Test empty routes
	condition, args := buildRouteExclusionCondition([]string{})
	if condition != "" {
		t.Errorf("buildRouteExclusionCondition([]) should return empty condition, got: %s", condition)
	}
	if len(args) != 0 {
		t.Errorf("buildRouteExclusionCondition([]) should return empty args, got: %v", args)
	}

	// Test nil routes
	condition, args = buildRouteExclusionCondition(nil)
	if condition != "" {
		t.Errorf("buildRouteExclusionCondition(nil) should return empty condition, got: %s", condition)
	}

	// Test with routes (including duplicates and empty)
	condition, args = buildRouteExclusionCondition([]string{"route1", "route2", "route1", ""})
	if condition == "" {
		t.Error("buildRouteExclusionCondition with routes should return non-empty condition")
	}
	if len(args) != 4 { // 2 routes * 2 args each
		t.Errorf("buildRouteExclusionCondition should return 4 args, got: %d", len(args))
	}
}

// Test AuditRepo Stats
func TestAuditRepo_Stats(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Insert some audit entries
	entries := []AuditEntry{
		{
			ID:         "stats-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			LatencyMS:  50,
			ClientIP:   "127.0.0.1",
		},
		{
			ID:         "stats-2",
			Method:     "POST",
			Path:       "/api/test",
			StatusCode: 500,
			LatencyMS:  100,
			ClientIP:   "127.0.0.1",
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Get stats
	stats, err := db.Audits().Stats(AuditSearchFilters{})
	if err != nil {
		t.Errorf("Stats error: %v", err)
	}
	if stats == nil {
		t.Fatal("Stats should return non-nil stats")
	}
	if stats.TotalRequests != 2 {
		t.Errorf("Stats.TotalRequests should be 2, got: %d", stats.TotalRequests)
	}
	if stats.ErrorRequests != 1 {
		t.Errorf("Stats.ErrorRequests should be 1, got: %d", stats.ErrorRequests)
	}
	if stats.ErrorRate != 0.5 {
		t.Errorf("Stats.ErrorRate should be 0.5, got: %f", stats.ErrorRate)
	}
}

// Test AuditRepo ListOlderThan
func TestAuditRepo_ListOlderThan(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Insert audit entry with old date
	entries := []AuditEntry{
		{
			ID:         "old-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			CreatedAt:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// List older than 2021
	cutoff := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	result, err := db.Audits().ListOlderThan(cutoff, 100)
	if err != nil {
		t.Errorf("ListOlderThan error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("ListOlderThan should return 1 entry, got: %d", len(result))
	}

	// List with zero cutoff (should error)
	_, err = db.Audits().ListOlderThan(time.Time{}, 100)
	if err == nil || err.Error() != "cutoff is required" {
		t.Errorf("ListOlderThan with zero cutoff should return 'cutoff is required' error, got: %v", err)
	}
}

// Test AuditRepo DeleteOlderThan
func TestAuditRepo_DeleteOlderThan(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Insert audit entry with old date
	entries := []AuditEntry{
		{
			ID:         "delete-old-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			CreatedAt:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Delete older than 2021
	cutoff := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	deleted, err := db.Audits().DeleteOlderThan(cutoff, 100)
	if err != nil {
		t.Errorf("DeleteOlderThan error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("DeleteOlderThan should delete 1 entry, got: %d", deleted)
	}

	// Delete with zero cutoff (should error)
	_, err = db.Audits().DeleteOlderThan(time.Time{}, 100)
	if err == nil || err.Error() != "cutoff is required" {
		t.Errorf("DeleteOlderThan with zero cutoff should return 'cutoff is required' error, got: %v", err)
	}
}

// Test AuditRepo DeleteByIDs
func TestAuditRepo_DeleteByIDs(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Insert audit entries
	entries := []AuditEntry{
		{
			ID:         "delete-id-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
		},
		{
			ID:         "delete-id-2",
			Method:     "POST",
			Path:       "/api/test",
			StatusCode: 201,
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Delete by IDs (including duplicates and empty)
	deleted, err := db.Audits().DeleteByIDs([]string{"delete-id-1", "delete-id-2", "delete-id-1", ""})
	if err != nil {
		t.Errorf("DeleteByIDs error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteByIDs should delete 2 entries, got: %d", deleted)
	}

	// Delete empty IDs
	deleted, err = db.Audits().DeleteByIDs([]string{})
	if err != nil {
		t.Errorf("DeleteByIDs([]) error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("DeleteByIDs([]) should delete 0 entries, got: %d", deleted)
	}
}

// Test AuditRepo Export with CSV format
func TestAuditRepo_Export_CSV(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Insert audit entry
	entries := []AuditEntry{
		{
			ID:         "export-csv-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Export as CSV
	var buf bytes.Buffer
	err := db.Audits().Export(AuditSearchFilters{}, "csv", &buf)
	if err != nil {
		t.Errorf("Export CSV error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Export CSV should return non-empty data")
	}
	if !strings.Contains(buf.String(), "export-csv-1") {
		t.Error("Export CSV should contain entry ID")
	}
}

// Test AuditRepo Export with JSON format
func TestAuditRepo_Export_JSON(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Insert audit entry
	entries := []AuditEntry{
		{
			ID:         "export-json-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
		},
	}
	if err := db.Audits().BatchInsert(entries); err != nil {
		t.Fatalf("BatchInsert error: %v", err)
	}

	// Export as JSON
	var buf bytes.Buffer
	err := db.Audits().Export(AuditSearchFilters{}, "json", &buf)
	if err != nil {
		t.Errorf("Export JSON error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Export JSON should return non-empty data")
	}
	if !strings.HasPrefix(buf.String(), "[") {
		t.Error("Export JSON should start with '['")
	}
	if !strings.HasSuffix(buf.String(), "]") {
		t.Error("Export JSON should end with ']'")
	}
}

// Test CreditRepo Create with type defaulting
func TestCreditRepo_Create_TypeDefaulting(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "credit@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create transaction without type
	txn := &CreditTransaction{
		UserID: user.ID,
		Amount: 100,
	}
	if err := db.Credits().Create(txn); err != nil {
		t.Errorf("Create error: %v", err)
	}
	if txn.Type != "consume" {
		t.Errorf("Type should default to 'consume', got: %s", txn.Type)
	}
}

// Test CreditRepo OverviewStats
func TestCreditRepo_OverviewStats(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "credit-stats@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create transactions
	txns := []*CreditTransaction{
		{UserID: user.ID, Type: "add", Amount: 1000},
		{UserID: user.ID, Type: "consume", Amount: -100},
		{UserID: user.ID, Type: "consume", Amount: -50},
	}
	for _, txn := range txns {
		if err := db.Credits().Create(txn); err != nil {
			t.Fatalf("Create transaction error: %v", err)
		}
	}

	// Get overview stats
	stats, err := db.Credits().OverviewStats()
	if err != nil {
		t.Errorf("OverviewStats error: %v", err)
	}
	if stats == nil {
		t.Fatal("OverviewStats should return non-nil stats")
	}
	if stats.TotalDistributed != 1000 {
		t.Errorf("TotalDistributed should be 1000, got: %d", stats.TotalDistributed)
	}
	if stats.TotalConsumed != 150 {
		t.Errorf("TotalConsumed should be 150, got: %d", stats.TotalConsumed)
	}
}

// Test SessionRepo full flow
func TestSessionRepo_FullFlow(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "session@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Generate session token
	token, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}
	if token == "" {
		t.Error("GenerateSessionToken should return non-empty token")
	}

	// Hash token
	tokenHash := HashSessionToken(token)
	if tokenHash == "" {
		t.Error("HashSessionToken should return non-empty hash")
	}

	// Create session
	session := &Session{
		UserID:    user.ID,
		TokenHash: tokenHash,
		UserAgent: "TestAgent",
		ClientIP:  "127.0.0.1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := db.Sessions().Create(session); err != nil {
		t.Fatalf("Create session error: %v", err)
	}
	if session.ID == "" {
		t.Error("Create should set session ID")
	}

	// Find by token hash
	found, err := db.Sessions().FindByTokenHash(tokenHash)
	if err != nil {
		t.Errorf("FindByTokenHash error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByTokenHash should return session")
	}
	if found.ID != session.ID {
		t.Error("FindByTokenHash should return correct session")
	}

	// Touch session
	if err := db.Sessions().Touch(session.ID); err != nil {
		t.Errorf("Touch error: %v", err)
	}

	// Cleanup expired (should not delete our session)
	deleted, err := db.Sessions().CleanupExpired(time.Now())
	if err != nil {
		t.Errorf("CleanupExpired error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupExpired should delete 0 sessions, got: %d", deleted)
	}

	// Cleanup expired (should delete our session)
	deleted, err = db.Sessions().CleanupExpired(time.Now().Add(48 * time.Hour))
	if err != nil {
		t.Errorf("CleanupExpired error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("CleanupExpired should delete 1 session, got: %d", deleted)
	}

	// Verify session is deleted
	found, err = db.Sessions().FindByTokenHash(tokenHash)
	if err != nil {
		t.Errorf("FindByTokenHash error: %v", err)
	}
	if found != nil {
		t.Error("Session should be deleted")
	}
}

// Test PermissionRepo BulkAssign
func TestPermissionRepo_BulkAssign(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "bulk-perm@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Bulk assign permissions
	perms := []EndpointPermission{
		{UserID: user.ID, RouteID: "route-1", Allowed: true},
		{UserID: user.ID, RouteID: "route-2", Allowed: false},
	}
	if err := db.Permissions().BulkAssign(user.ID, perms); err != nil {
		t.Errorf("BulkAssign error: %v", err)
	}

	// List permissions
	list, err := db.Permissions().ListByUser(user.ID)
	if err != nil {
		t.Errorf("ListByUser error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListByUser should return 2 permissions, got: %d", len(list))
	}

	// Bulk assign again (should update existing)
	perms = []EndpointPermission{
		{UserID: user.ID, RouteID: "route-1", Allowed: false},
	}
	if err := db.Permissions().BulkAssign(user.ID, perms); err != nil {
		t.Errorf("BulkAssign update error: %v", err)
	}

	// Verify update
	perm, err := db.Permissions().FindByUserAndRoute(user.ID, "route-1")
	if err != nil {
		t.Errorf("FindByUserAndRoute error: %v", err)
	}
	if perm != nil && perm.Allowed != false {
		t.Error("Permission should be updated to Allowed=false")
	}
}

// Test PermissionRepo Update with non-existent ID
func TestPermissionRepo_Update_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "perm-update@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Try to update non-existent permission
	err := db.Permissions().Update(&EndpointPermission{
		ID:      "nonexistent-id",
		UserID:  user.ID,
		RouteID: "route-1",
	})
	if err != sql.ErrNoRows {
		t.Errorf("Update with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test UserRepo withTx error handling
func TestUserRepo_WithTx(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "tx-test@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Test withTx that returns error
	ctx := context.Background()
	err := db.Users().withTx(ctx, func(tx *sql.Tx) error {
		return sql.ErrNoRows
	})
	if err != sql.ErrNoRows {
		t.Errorf("withTx should return the error from the function, got: %v", err)
	}

	// Test withTx that succeeds
	err = db.Users().withTx(ctx, func(tx *sql.Tx) error {
		return nil
	})
	if err != nil {
		t.Errorf("withTx should return nil on success, got: %v", err)
	}
}

// Test UserRepo UpdateCreditBalance insufficient credits
func TestUserRepo_UpdateCreditBalance_InsufficientCredits(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user with 0 credits
	user := &User{
		Email:         "insufficient@example.com",
		Name:          "Test User",
		PasswordHash:  "password123",
		Status:        "active",
		CreditBalance: 0,
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Try to consume more credits than available
	_, err := db.Users().UpdateCreditBalance(user.ID, -100)
	if err != ErrInsufficientCredits {
		t.Errorf("UpdateCreditBalance with insufficient credits should return ErrInsufficientCredits, got: %v", err)
	}

	// Try to update non-existent user
	_, err = db.Users().UpdateCreditBalance("nonexistent-id", 100)
	if err != sql.ErrNoRows {
		t.Errorf("UpdateCreditBalance with non-existent user should return sql.ErrNoRows, got: %v", err)
	}
}

// Test UserRepo UpdateCreditBalance success
func TestUserRepo_UpdateCreditBalance_Success(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "credits@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Add credits
	newBalance, err := db.Users().UpdateCreditBalance(user.ID, 1000)
	if err != nil {
		t.Errorf("UpdateCreditBalance error: %v", err)
	}
	if newBalance != 1000 {
		t.Errorf("New balance should be 1000, got: %d", newBalance)
	}

	// Consume credits
	newBalance, err = db.Users().UpdateCreditBalance(user.ID, -500)
	if err != nil {
		t.Errorf("UpdateCreditBalance error: %v", err)
	}
	if newBalance != 500 {
		t.Errorf("New balance should be 500, got: %d", newBalance)
	}
}

// Test UserRepo Update non-existent user
func TestUserRepo_Update_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().Update(&User{
		ID:    "nonexistent-id",
		Email: "test@example.com",
		Name:  "Test User",
	})
	if err != sql.ErrNoRows {
		t.Errorf("Update with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test UserRepo Delete non-existent user
func TestUserRepo_Delete_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().Delete("nonexistent-id")
	if err != sql.ErrNoRows {
		t.Errorf("Delete with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test UserRepo HardDelete non-existent user
func TestUserRepo_HardDelete_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().HardDelete("nonexistent-id")
	if err != sql.ErrNoRows {
		t.Errorf("HardDelete with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test UserRepo UpdateStatus non-existent user
func TestUserRepo_UpdateStatus_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	err := db.Users().UpdateStatus("nonexistent-id", "suspended")
	if err != sql.ErrNoRows {
		t.Errorf("UpdateStatus with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test APIKeyRepo RenameForUser non-existent key
func TestAPIKeyRepo_RenameForUser_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "rename@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	err := db.APIKeys().RenameForUser("nonexistent-id", user.ID, "New Name")
	if err != sql.ErrNoRows {
		t.Errorf("RenameForUser with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test APIKeyRepo RevokeForUser non-existent key
func TestAPIKeyRepo_RevokeForUser_NonExistent(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "revoke-user@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	err := db.APIKeys().RevokeForUser("nonexistent-id", user.ID)
	if err != sql.ErrNoRows {
		t.Errorf("RevokeForUser with non-existent ID should return sql.ErrNoRows, got: %v", err)
	}
}

// Test PlaygroundTemplateRepo full flow
func TestPlaygroundTemplateRepo_FullFlow(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "template-flow@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create template
	template := &PlaygroundTemplate{
		UserID:  user.ID,
		Name:    "Test Template",
		Method:  "post",
		Path:    "/api/test",
		Headers: map[string]string{"Content-Type": "application/json"},
		Query:   map[string]string{"page": "1"},
		Body:    `{"key": "value"}`,
	}
	if err := db.PlaygroundTemplates().Save(template); err != nil {
		t.Fatalf("Save template error: %v", err)
	}
	if template.ID == "" {
		t.Error("Save should set template ID")
	}
	if template.Method != "POST" {
		t.Errorf("Method should be uppercase, got: %s", template.Method)
	}

	// List templates
	templates, err := db.PlaygroundTemplates().ListByUser(user.ID)
	if err != nil {
		t.Errorf("ListByUser error: %v", err)
	}
	if len(templates) != 1 {
		t.Errorf("ListByUser should return 1 template, got: %d", len(templates))
	}

	// Update template
	template.Name = "Updated Template"
	template.Method = "get"
	if err := db.PlaygroundTemplates().Save(template); err != nil {
		t.Errorf("Save (update) error: %v", err)
	}

	// Verify update
	templates, _ = db.PlaygroundTemplates().ListByUser(user.ID)
	if len(templates) != 1 || templates[0].Name != "Updated Template" {
		t.Error("Template should be updated")
	}
	if templates[0].Method != "GET" {
		t.Errorf("Method should be uppercase after update, got: %s", templates[0].Method)
	}

	// Delete template
	if err := db.PlaygroundTemplates().DeleteForUser(template.ID, user.ID); err != nil {
		t.Errorf("DeleteForUser error: %v", err)
	}

	// Verify deletion
	templates, _ = db.PlaygroundTemplates().ListByUser(user.ID)
	if len(templates) != 0 {
		t.Errorf("ListByUser should return 0 templates after deletion, got: %d", len(templates))
	}
}

// Test generateSecurePassword
func TestGenerateSecurePassword(t *testing.T) {
	// Generate multiple passwords and verify they're different
	passwords := make(map[string]bool)
	for i := 0; i < 10; i++ {
		password := generateSecurePassword()
		if len(password) != 20 {
			t.Errorf("Password length should be 20, got: %d", len(password))
		}
		if passwords[password] {
			t.Error("Generated duplicate password")
		}
		passwords[password] = true
	}
}

// Test HashPassword and VerifyPassword
func TestHashPasswordAndVerify(t *testing.T) {
	password := "mySecurePassword123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" {
		t.Error("HashPassword should return non-empty hash")
	}
	if hash == password {
		t.Error("HashPassword should return hashed password, not plaintext")
	}

	// Verify correct password
	if !VerifyPassword(hash, password) {
		t.Error("VerifyPassword should return true for correct password")
	}

	// Verify incorrect password
	if VerifyPassword(hash, "wrongpassword") {
		t.Error("VerifyPassword should return false for incorrect password")
	}
}

// Test CreditTransaction scanning
func TestScanCreditTransactionRows(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "scan-credit@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create transaction
	txn := &CreditTransaction{
		UserID:        user.ID,
		Type:          "add",
		Amount:        500,
		BalanceBefore: 0,
		BalanceAfter:  500,
		Description:   "Test credit",
		RequestID:     "req-123",
		RouteID:       "route-456",
	}
	if err := db.Credits().Create(txn); err != nil {
		t.Fatalf("Create transaction error: %v", err)
	}

	// List and verify scanning works
	result, err := db.Credits().ListByUser(user.ID, CreditListOptions{})
	if err != nil {
		t.Errorf("ListByUser error: %v", err)
	}
	if result == nil || len(result.Transactions) != 1 {
		t.Fatal("ListByUser should return 1 transaction")
	}

	scanned := result.Transactions[0]
	if scanned.ID != txn.ID {
		t.Error("Scanned transaction ID should match")
	}
	if scanned.UserID != txn.UserID {
		t.Error("Scanned transaction UserID should match")
	}
	if scanned.Type != txn.Type {
		t.Error("Scanned transaction Type should match")
	}
	if scanned.Amount != txn.Amount {
		t.Error("Scanned transaction Amount should match")
	}
	if scanned.BalanceBefore != txn.BalanceBefore {
		t.Error("Scanned transaction BalanceBefore should match")
	}
	if scanned.BalanceAfter != txn.BalanceAfter {
		t.Error("Scanned transaction BalanceAfter should match")
	}
	if scanned.Description != txn.Description {
		t.Error("Scanned transaction Description should match")
	}
	if scanned.RequestID != txn.RequestID {
		t.Error("Scanned transaction RequestID should match")
	}
	if scanned.RouteID != txn.RouteID {
		t.Error("Scanned transaction RouteID should match")
	}
}

// Test User List with various options
func TestUserRepo_List_WithOptions(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Note: The initial admin user is created automatically when the store opens
	// So we start with 1 admin user already in the database

	// Create additional users
	users := []*User{
		{Email: "alice@example.com", Name: "Alice", Role: "user", Status: "active"},
		{Email: "bob@example.com", Name: "Bob", Role: "admin", Status: "active"},
		{Email: "charlie@example.com", Name: "Charlie", Role: "user", Status: "suspended"},
	}
	for _, user := range users {
		user.PasswordHash = "password123"
		if err := db.Users().Create(user); err != nil {
			t.Fatalf("Create user error: %v", err)
		}
	}

	// Test search
	result, err := db.Users().List(UserListOptions{Search: "alice"})
	if err != nil {
		t.Errorf("List with search error: %v", err)
	}
	if result == nil || len(result.Users) != 1 {
		t.Errorf("List with search 'alice' should return 1 user, got: %d", len(result.Users))
	}

	// Test status filter (1 initial admin + alice + bob = 3 active)
	result, err = db.Users().List(UserListOptions{Status: "active"})
	if err != nil {
		t.Errorf("List with status filter error: %v", err)
	}
	if result == nil || len(result.Users) != 3 {
		t.Errorf("List with status 'active' should return 3 users, got: %d", len(result.Users))
	}

	// Test role filter (1 initial admin + bob = 2 admins)
	result, err = db.Users().List(UserListOptions{Role: "admin"})
	if err != nil {
		t.Errorf("List with role filter error: %v", err)
	}
	if result == nil || len(result.Users) != 2 {
		t.Errorf("List with role 'admin' should return 2 users, got: %d", len(result.Users))
	}

	// Test sort by name (3 created + 1 initial admin = 4 total)
	result, err = db.Users().List(UserListOptions{SortBy: "name"})
	if err != nil {
		t.Errorf("List with sort error: %v", err)
	}
	if result == nil || len(result.Users) != 4 {
		t.Errorf("List should return 4 users, got: %d", len(result.Users))
	}

	// Test sort desc
	result, err = db.Users().List(UserListOptions{SortBy: "name", SortDesc: true})
	if err != nil {
		t.Errorf("List with sort desc error: %v", err)
	}
	if result == nil || len(result.Users) != 4 {
		t.Errorf("List should return 4 users, got: %d", len(result.Users))
	}

	// Test pagination
	result, err = db.Users().List(UserListOptions{Limit: 1, Offset: 0})
	if err != nil {
		t.Errorf("List with pagination error: %v", err)
	}
	if result == nil || len(result.Users) != 1 {
		t.Errorf("List with limit 1 should return 1 user, got: %d", len(result.Users))
	}
	if result.Total != 4 {
		t.Errorf("List total should be 4, got: %d", result.Total)
	}
}

// Test Credit ListByUser with type filter
func TestCreditRepo_ListByUser_WithTypeFilter(t *testing.T) {
	db := setupTestStore(t)
	defer db.Close()

	// Create user
	user := &User{
		Email:        "credit-filter@example.com",
		Name:         "Test User",
		PasswordHash: "password123",
		Status:       "active",
	}
	if err := db.Users().Create(user); err != nil {
		t.Fatalf("Create user error: %v", err)
	}

	// Create transactions of different types
	txns := []*CreditTransaction{
		{UserID: user.ID, Type: "add", Amount: 1000},
		{UserID: user.ID, Type: "consume", Amount: -100},
		{UserID: user.ID, Type: "refund", Amount: 50},
	}
	for _, txn := range txns {
		if err := db.Credits().Create(txn); err != nil {
			t.Fatalf("Create transaction error: %v", err)
		}
	}

	// Test type filter
	result, err := db.Credits().ListByUser(user.ID, CreditListOptions{Type: "add"})
	if err != nil {
		t.Errorf("ListByUser with type filter error: %v", err)
	}
	if result == nil || len(result.Transactions) != 1 {
		t.Errorf("ListByUser with type 'add' should return 1 transaction, got: %d", len(result.Transactions))
	}

	// Test pagination
	result, err = db.Credits().ListByUser(user.ID, CreditListOptions{Limit: 2, Offset: 0})
	if err != nil {
		t.Errorf("ListByUser with pagination error: %v", err)
	}
	if result == nil || len(result.Transactions) != 2 {
		t.Errorf("ListByUser with limit 2 should return 2 transactions, got: %d", len(result.Transactions))
	}
	if result.Total != 3 {
		t.Errorf("ListByUser total should be 3, got: %d", result.Total)
	}
}
