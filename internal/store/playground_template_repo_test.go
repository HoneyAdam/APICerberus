package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestPlaygroundTemplateRepoSaveListDelete(t *testing.T) {
	t.Parallel()

	st := openPlaygroundTemplateTestStore(t)
	defer st.Close()
	user := createPlaygroundTemplateTestUser(t, st, "template-user@example.com")

	repo := st.PlaygroundTemplates()
	item := &PlaygroundTemplate{
		UserID: user.ID,
		Name:   "First Request",
		Method: "POST",
		Path:   "/v1/chat/completions",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Query: map[string]string{
			"trace": "1",
		},
		Body: `{"input":"hello"}`,
	}
	if err := repo.Save(item); err != nil {
		t.Fatalf("Save(create) error: %v", err)
	}
	if item.ID == "" {
		t.Fatalf("expected created template id")
	}

	items, err := repo.ListByUser(user.ID)
	if err != nil {
		t.Fatalf("ListByUser error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 template got %d", len(items))
	}
	if items[0].Name != "First Request" {
		t.Fatalf("unexpected template name %q", items[0].Name)
	}

	item.Name = "Updated Request"
	if err := repo.Save(item); err != nil {
		t.Fatalf("Save(update) error: %v", err)
	}
	items, err = repo.ListByUser(user.ID)
	if err != nil {
		t.Fatalf("ListByUser after update error: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Updated Request" {
		t.Fatalf("expected updated template, got %#v", items)
	}

	if err := repo.DeleteForUser(item.ID, user.ID); err != nil {
		t.Fatalf("DeleteForUser error: %v", err)
	}
	items, err = repo.ListByUser(user.ID)
	if err != nil {
		t.Fatalf("ListByUser after delete error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 templates after delete, got %d", len(items))
	}
}

func openPlaygroundTemplateTestStore(t *testing.T) *Store {
	t.Helper()
	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        filepath.Join(t.TempDir(), "playground-template.db"),
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

func createPlaygroundTemplateTestUser(t *testing.T, st *Store, email string) *User {
	t.Helper()
	hash, err := HashPassword("pw")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	user := &User{
		Email:        email,
		Name:         "Template User",
		PasswordHash: hash,
		Role:         "user",
		Status:       "active",
	}
	if err := st.Users().Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}
	return user
}
