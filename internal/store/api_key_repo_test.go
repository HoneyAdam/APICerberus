package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestAPIKeyRepoCreateFindListRevokeResolve(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	user := &User{
		Email:        "key-owner@example.com",
		Name:         "Key Owner",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}

	fullKey, created, err := apiKeys.Create(user.ID, "primary", "live")
	if err != nil {
		t.Fatalf("Create api key error: %v", err)
	}
	if fullKey == "" || created == nil {
		t.Fatalf("expected full key and metadata")
	}
	if len(fullKey) < len("ck_live_")+32 || fullKey[:8] != "ck_live_" {
		t.Fatalf("unexpected full key format %q", fullKey)
	}
	if len(created.KeyPrefix) != 12 {
		t.Fatalf("expected key prefix length 12 got %d", len(created.KeyPrefix))
	}

	foundByHash, err := apiKeys.FindByHash(created.KeyHash)
	if err != nil {
		t.Fatalf("FindByHash error: %v", err)
	}
	if foundByHash == nil || foundByHash.ID != created.ID {
		t.Fatalf("unexpected FindByHash result %#v", foundByHash)
	}

	list, err := apiKeys.ListByUser(user.ID)
	if err != nil {
		t.Fatalf("ListByUser error: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("unexpected ListByUser result %#v", list)
	}

	resolvedUser, resolvedKey, err := apiKeys.ResolveUserByRawKey(fullKey)
	if err != nil {
		t.Fatalf("ResolveUserByRawKey error: %v", err)
	}
	if resolvedUser == nil || resolvedUser.ID != user.ID {
		t.Fatalf("unexpected resolved user %#v", resolvedUser)
	}
	if resolvedKey == nil || resolvedKey.ID != created.ID {
		t.Fatalf("unexpected resolved key %#v", resolvedKey)
	}

	if err := apiKeys.Revoke(created.ID); err != nil {
		t.Fatalf("Revoke error: %v", err)
	}
	_, _, err = apiKeys.ResolveUserByRawKey(fullKey)
	if err != ErrAPIKeyRevoked {
		t.Fatalf("expected ErrAPIKeyRevoked got %v", err)
	}
}

func TestAPIKeyRepoUpdateLastUsed(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, _ := HashPassword("pw")
	user := &User{
		Email:        "last-used@example.com",
		Name:         "Last Used",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}
	_, created, err := apiKeys.Create(user.ID, "secondary", "test")
	if err != nil {
		t.Fatalf("create api key error: %v", err)
	}

	apiKeys.UpdateLastUsed(created.ID, "198.51.100.5")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		found, err := apiKeys.FindByHash(created.KeyHash)
		if err != nil {
			t.Fatalf("FindByHash error: %v", err)
		}
		if found != nil && found.LastUsedIP == "198.51.100.5" && found.LastUsedAt != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected async UpdateLastUsed to update key usage fields")
}

func TestAPIKeyRepoCreateFailsForUnknownUser(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	_, _, err := s.APIKeys().Create("missing-user", "x", "live")
	if err == nil {
		t.Fatalf("expected error for missing user")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows got %v", err)
	}
}

func TestResolveUserByRawKeyWithExpiredAndInactiveUser(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, _ := HashPassword("pw")
	user := &User{
		Email:        "exp@example.com",
		Name:         "Exp User",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}
	raw, created, err := apiKeys.Create(user.ID, "exp-key", "live")
	if err != nil {
		t.Fatalf("create api key error: %v", err)
	}

	expiredAt := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	_, err = s.DB().Exec(`UPDATE api_keys SET expires_at = ? WHERE id = ?`, expiredAt, created.ID)
	if err != nil {
		t.Fatalf("set expires_at error: %v", err)
	}
	_, _, err = apiKeys.ResolveUserByRawKey(raw)
	if err != ErrAPIKeyExpired {
		t.Fatalf("expected ErrAPIKeyExpired got %v", err)
	}

	_, err = s.DB().Exec(`UPDATE api_keys SET expires_at = '' WHERE id = ?`, created.ID)
	if err != nil {
		t.Fatalf("clear expires_at error: %v", err)
	}
	if err := users.UpdateStatus(user.ID, "suspended"); err != nil {
		t.Fatalf("suspend user error: %v", err)
	}
	_, _, err = apiKeys.ResolveUserByRawKey(raw)
	if err != ErrAPIKeyUserDown {
		t.Fatalf("expected ErrAPIKeyUserDown got %v", err)
	}
}

func TestAPIKeyHashHelpers(t *testing.T) {
	t.Parallel()

	h1 := hashAPIKey("abc")
	h2 := hashAPIKey("abc")
	if h1 == "" || h1 != h2 {
		t.Fatalf("expected deterministic non-empty hash")
	}
	token, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken error: %v", err)
	}
	if len(token) != 32 {
		t.Fatalf("expected token length 32 got %d", len(token))
	}
}

func TestAPIKeyRepoWorksWithFileBackedStore(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/apikey.db"
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        path,
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}
	s, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer s.Close()
	if s.APIKeys() == nil {
		t.Fatalf("expected non-nil APIKeys repo")
	}
}

// Test RenameForUser function
func TestAPIKeyRepoRenameForUser(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, _ := HashPassword("pw")
	user := &User{
		Email:        "rename-test@example.com",
		Name:         "Rename Test",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}

	_, created, err := apiKeys.Create(user.ID, "original-name", "live")
	if err != nil {
		t.Fatalf("create api key error: %v", err)
	}

	// Rename the key
	if err := apiKeys.RenameForUser(created.ID, user.ID, "new-name"); err != nil {
		t.Fatalf("RenameForUser error: %v", err)
	}

	// Verify rename
	found, err := apiKeys.FindByHash(created.KeyHash)
	if err != nil {
		t.Fatalf("FindByHash error: %v", err)
	}
	if found.Name != "new-name" {
		t.Errorf("expected name 'new-name', got %q", found.Name)
	}
}

// Test RenameForUser validation errors
func TestAPIKeyRepoRenameForUser_ValidationErrors(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	apiKeys := s.APIKeys()

	// Empty ID
	if err := apiKeys.RenameForUser("", "user-id", "name"); err == nil {
		t.Error("expected error for empty ID")
	}

	// Empty user ID
	if err := apiKeys.RenameForUser("key-id", "", "name"); err == nil {
		t.Error("expected error for empty user ID")
	}

	// Empty name
	if err := apiKeys.RenameForUser("key-id", "user-id", ""); err == nil {
		t.Error("expected error for empty name")
	}
}

// Test RenameForUser for non-existent key
func TestAPIKeyRepoRenameForUser_NotFound(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, _ := HashPassword("pw")
	user := &User{
		Email:        "rename-notfound@example.com",
		Name:         "Rename NotFound",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}

	// Try to rename non-existent key
	err := apiKeys.RenameForUser("non-existent-key", user.ID, "new-name")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

// Test RevokeForUser function
func TestAPIKeyRepoRevokeForUser(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, _ := HashPassword("pw")
	user := &User{
		Email:        "revoke-test@example.com",
		Name:         "Revoke Test",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}

	fullKey, created, err := apiKeys.Create(user.ID, "to-revoke", "live")
	if err != nil {
		t.Fatalf("create api key error: %v", err)
	}

	// Revoke the key
	if err := apiKeys.RevokeForUser(created.ID, user.ID); err != nil {
		t.Fatalf("RevokeForUser error: %v", err)
	}

	// Verify revoked
	_, _, err = apiKeys.ResolveUserByRawKey(fullKey)
	if err != ErrAPIKeyRevoked {
		t.Errorf("expected ErrAPIKeyRevoked, got %v", err)
	}
}

// Test RevokeForUser validation errors
func TestAPIKeyRepoRevokeForUser_ValidationErrors(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	apiKeys := s.APIKeys()

	// Empty ID
	if err := apiKeys.RevokeForUser("", "user-id"); err == nil {
		t.Error("expected error for empty ID")
	}

	// Empty user ID
	if err := apiKeys.RevokeForUser("key-id", ""); err == nil {
		t.Error("expected error for empty user ID")
	}
}

// Test RevokeForUser for non-existent key
func TestAPIKeyRepoRevokeForUser_NotFound(t *testing.T) {
	t.Parallel()

	s := openTestStore(t)
	defer s.Close()

	users := s.Users()
	apiKeys := s.APIKeys()

	passwordHash, _ := HashPassword("pw")
	user := &User{
		Email:        "revoke-notfound@example.com",
		Name:         "Revoke NotFound",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := users.Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}

	// Try to revoke non-existent key
	err := apiKeys.RevokeForUser("non-existent-key", user.ID)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}
