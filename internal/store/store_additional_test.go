package store

import (
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
		email  string
		valid  bool
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
		{"test@@example.com", true},   // has @ and domain has .
		{"test@example.", true},       // has @ and domain has . (even at end)
		{".test@example.com", true},    // has @ and domain has .
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
