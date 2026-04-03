package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestSessionRepoCreateFindTouchAndDelete(t *testing.T) {
	t.Parallel()

	st := openSessionTestStore(t)
	defer st.Close()

	user := createSessionTestUser(t, st, "session-user@example.com")

	rawToken, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}
	hash := HashSessionToken(rawToken)
	if hash == "" {
		t.Fatalf("expected non-empty token hash")
	}

	repo := st.Sessions()
	session := &Session{
		UserID:    user.ID,
		TokenHash: hash,
		UserAgent: "session-repo-test",
		ClientIP:  "127.0.0.1",
		ExpiresAt: time.Now().UTC().Add(2 * time.Hour),
	}
	if err := repo.Create(session); err != nil {
		t.Fatalf("Create session error: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected generated session id")
	}

	found, err := repo.FindByTokenHash(hash)
	if err != nil {
		t.Fatalf("FindByTokenHash error: %v", err)
	}
	if found == nil {
		t.Fatalf("expected session to exist")
	}
	if found.UserID != user.ID {
		t.Fatalf("expected session user_id=%q got %q", user.ID, found.UserID)
	}

	if err := repo.Touch(found.ID); err != nil {
		t.Fatalf("Touch session error: %v", err)
	}

	if err := repo.DeleteByID(found.ID); err != nil {
		t.Fatalf("DeleteByID error: %v", err)
	}
	afterDelete, err := repo.FindByTokenHash(hash)
	if err != nil {
		t.Fatalf("FindByTokenHash after delete error: %v", err)
	}
	if afterDelete != nil {
		t.Fatalf("expected session to be deleted")
	}
}

func TestSessionRepoCleanupExpired(t *testing.T) {
	t.Parallel()

	st := openSessionTestStore(t)
	defer st.Close()
	user := createSessionTestUser(t, st, "cleanup-user@example.com")

	repo := st.Sessions()
	err := repo.Create(&Session{
		UserID:    user.ID,
		TokenHash: HashSessionToken("expired-token"),
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("Create expired session error: %v", err)
	}

	deleted, err := repo.CleanupExpired(time.Now().UTC())
	if err != nil {
		t.Fatalf("CleanupExpired error: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected at least one expired session to be deleted")
	}
}

func openSessionTestStore(t *testing.T) *Store {
	t.Helper()
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        filepath.Join(t.TempDir(), "session-repo.db"),
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}
	st, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open store error: %v", err)
	}
	return st
}

func createSessionTestUser(t *testing.T, st *Store, email string) *User {
	t.Helper()
	passwordHash, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	user := &User{
		Email:        email,
		Name:         "Session Test User",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := st.Users().Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}
	return user
}

// Test DeleteByTokenHash function
func TestSessionRepoDeleteByTokenHash(t *testing.T) {
	t.Parallel()

	st := openSessionTestStore(t)
	defer st.Close()

	user := createSessionTestUser(t, st, "delete-by-token@example.com")

	rawToken, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}
	hash := HashSessionToken(rawToken)

	repo := st.Sessions()
	session := &Session{
		UserID:    user.ID,
		TokenHash: hash,
		UserAgent: "delete-by-token-test",
		ClientIP:  "127.0.0.1",
		ExpiresAt: time.Now().UTC().Add(2 * time.Hour),
	}
	if err := repo.Create(session); err != nil {
		t.Fatalf("Create session error: %v", err)
	}

	// Verify session exists
	found, err := repo.FindByTokenHash(hash)
	if err != nil {
		t.Fatalf("FindByTokenHash error: %v", err)
	}
	if found == nil {
		t.Fatal("expected session to exist")
	}

	// Delete by token hash
	if err := repo.DeleteByTokenHash(hash); err != nil {
		t.Fatalf("DeleteByTokenHash error: %v", err)
	}

	// Verify deleted
	afterDelete, err := repo.FindByTokenHash(hash)
	if err != nil {
		t.Fatalf("FindByTokenHash after delete error: %v", err)
	}
	if afterDelete != nil {
		t.Fatal("expected session to be deleted")
	}
}

// Test DeleteByTokenHash validation errors
func TestSessionRepoDeleteByTokenHash_ValidationErrors(t *testing.T) {
	t.Parallel()

	st := openSessionTestStore(t)
	defer st.Close()

	repo := st.Sessions()

	// Empty token hash
	if err := repo.DeleteByTokenHash(""); err == nil {
		t.Error("expected error for empty token hash")
	}

	// Whitespace only
	if err := repo.DeleteByTokenHash("   "); err == nil {
		t.Error("expected error for whitespace-only token hash")
	}
}

// Test DeleteByTokenHash for non-existent session (should not error)
func TestSessionRepoDeleteByTokenHash_NonExistent(t *testing.T) {
	t.Parallel()

	st := openSessionTestStore(t)
	defer st.Close()

	repo := st.Sessions()

	// Delete non-existent session should not error
	hash := HashSessionToken("non-existent-token")
	if err := repo.DeleteByTokenHash(hash); err != nil {
		t.Errorf("DeleteByTokenHash for non-existent session should not error, got: %v", err)
	}
}

