package store

import (
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// openTestStoreForWebhooks opens a test store with webhook tables
func openTestStoreForWebhooks(t *testing.T) *Store {
	t.Helper()

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	}

	st, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Create webhook tables
	if err := st.CreateWebhookTables(); err != nil {
		st.Close()
		t.Fatalf("failed to create webhook tables: %v", err)
	}

	return st
}

// TestWebhookRepo_NilStore tests nil store behavior
func TestWebhookRepo_NilStore(t *testing.T) {
	t.Parallel()

	t.Run("Webhooks returns nil for nil store", func(t *testing.T) {
		var s *Store
		repo := s.Webhooks()
		if repo != nil {
			t.Error("expected nil repo for nil store")
		}
	})

	t.Run("Webhooks returns nil for store with nil db", func(t *testing.T) {
		s := &Store{db: nil}
		repo := s.Webhooks()
		if repo != nil {
			t.Error("expected nil repo for store with nil db")
		}
	})
}

// TestWebhookRepo_NilRepo tests nil repo behavior
func TestWebhookRepo_NilRepo(t *testing.T) {
	t.Parallel()

	var r *WebhookRepo

	t.Run("CreateWebhook returns error for nil repo", func(t *testing.T) {
		wh := &Webhook{ID: "test", Name: "Test", URL: "http://example.com"}
		err := r.CreateWebhook(wh)
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("GetWebhook returns error for nil repo", func(t *testing.T) {
		_, err := r.GetWebhook("test")
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("UpdateWebhook returns error for nil repo", func(t *testing.T) {
		wh := &Webhook{ID: "test", Name: "Test", URL: "http://example.com"}
		err := r.UpdateWebhook(wh)
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("DeleteWebhook returns error for nil repo", func(t *testing.T) {
		err := r.DeleteWebhook("test")
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("ListWebhooks returns error for nil repo", func(t *testing.T) {
		_, err := r.ListWebhooks()
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("ListWebhooksByEvent returns error for nil repo", func(t *testing.T) {
		_, err := r.ListWebhooksByEvent("test.event")
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("CreateDelivery returns error for nil repo", func(t *testing.T) {
		delivery := &WebhookDelivery{ID: "test", WebhookID: "wh1"}
		err := r.CreateDelivery(delivery)
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("UpdateDelivery returns error for nil repo", func(t *testing.T) {
		delivery := &WebhookDelivery{ID: "test", Status: "success"}
		err := r.UpdateDelivery(delivery)
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("GetDeliveries returns error for nil repo", func(t *testing.T) {
		_, err := r.GetDeliveries("test", 10)
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})

	t.Run("GetPendingDeliveries returns error for nil repo", func(t *testing.T) {
		_, err := r.GetPendingDeliveries(10)
		if err == nil {
			t.Error("expected error for nil repo")
		}
	})
}

// TestWebhookRepo_CreateAndGet tests creating and retrieving webhooks
func TestWebhookRepo_CreateAndGet(t *testing.T) {
	t.Parallel()

	st := openTestStoreForWebhooks(t)
	defer st.Close()

	repo := st.Webhooks()

	webhook := &Webhook{
		ID:            "wh-test-1",
		Name:          "Test Webhook",
		URL:           "https://example.com/webhook",
		Secret:        "secret123",
		Events:        []string{"user.created", "user.updated"},
		Headers:       map[string]string{"X-Custom": "value"},
		Active:        true,
		RetryCount:    3,
		RetryInterval: 60,
		Timeout:       30,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	t.Run("Create webhook", func(t *testing.T) {
		err := repo.CreateWebhook(webhook)
		if err != nil {
			t.Errorf("failed to create webhook: %v", err)
		}
	})

	t.Run("Get webhook", func(t *testing.T) {
		retrieved, err := repo.GetWebhook(webhook.ID)
		if err != nil {
			t.Fatalf("failed to get webhook: %v", err)
		}

		if retrieved.ID != webhook.ID {
			t.Errorf("expected ID %s, got %s", webhook.ID, retrieved.ID)
		}
		if retrieved.Name != webhook.Name {
			t.Errorf("expected Name %s, got %s", webhook.Name, retrieved.Name)
		}
		if retrieved.URL != webhook.URL {
			t.Errorf("expected URL %s, got %s", webhook.URL, retrieved.URL)
		}
		if !retrieved.Active {
			t.Error("expected Active to be true")
		}
	})

	t.Run("Get non-existent webhook", func(t *testing.T) {
		_, err := repo.GetWebhook("non-existent")
		if err == nil {
			t.Error("expected error for non-existent webhook")
		}
	})
}

// TestWebhookRepo_Update tests updating webhooks
func TestWebhookRepo_Update(t *testing.T) {
	t.Parallel()

	st := openTestStoreForWebhooks(t)
	defer st.Close()

	repo := st.Webhooks()

	webhook := &Webhook{
		ID:        "wh-update-1",
		Name:      "Original Name",
		URL:       "https://example.com/original",
		Events:    []string{"event1"},
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := repo.CreateWebhook(webhook); err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	t.Run("Update webhook", func(t *testing.T) {
		webhook.Name = "Updated Name"
		webhook.URL = "https://example.com/updated"
		webhook.Events = []string{"event1", "event2"}
		webhook.Active = false
		webhook.UpdatedAt = time.Now().UTC()

		err := repo.UpdateWebhook(webhook)
		if err != nil {
			t.Errorf("failed to update webhook: %v", err)
		}

		// Verify update
		retrieved, err := repo.GetWebhook(webhook.ID)
		if err != nil {
			t.Fatalf("failed to get webhook: %v", err)
		}

		if retrieved.Name != "Updated Name" {
			t.Errorf("expected Name 'Updated Name', got %s", retrieved.Name)
		}
		if retrieved.URL != "https://example.com/updated" {
			t.Errorf("expected URL 'https://example.com/updated', got %s", retrieved.URL)
		}
		if retrieved.Active {
			t.Error("expected Active to be false")
		}
	})
}

// TestWebhookRepo_Delete tests deleting webhooks
func TestWebhookRepo_Delete(t *testing.T) {
	t.Parallel()

	st := openTestStoreForWebhooks(t)
	defer st.Close()

	repo := st.Webhooks()

	webhook := &Webhook{
		ID:        "wh-delete-1",
		Name:      "To Delete",
		URL:       "https://example.com/delete",
		Events:    []string{"event1"},
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := repo.CreateWebhook(webhook); err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	t.Run("Delete webhook", func(t *testing.T) {
		err := repo.DeleteWebhook(webhook.ID)
		if err != nil {
			t.Errorf("failed to delete webhook: %v", err)
		}

		// Verify deletion
		_, err = repo.GetWebhook(webhook.ID)
		if err == nil {
			t.Error("expected error when getting deleted webhook")
		}
	})

	t.Run("Delete non-existent webhook", func(t *testing.T) {
		err := repo.DeleteWebhook("non-existent")
		if err == nil {
			t.Error("expected error when deleting non-existent webhook")
		}
	})
}

// TestWebhookRepo_List tests listing webhooks
func TestWebhookRepo_List(t *testing.T) {
	t.Parallel()

	st := openTestStoreForWebhooks(t)
	defer st.Close()

	repo := st.Webhooks()

	// Create multiple webhooks
	for i := 0; i < 3; i++ {
		webhook := &Webhook{
			ID:        "wh-list-" + string(rune('a'+i)),
			Name:      "Webhook " + string(rune('A'+i)),
			URL:       "https://example.com/webhook" + string(rune('a'+i)),
			Events:    []string{"event1"},
			Active:    i%2 == 0, // Alternate active/inactive
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := repo.CreateWebhook(webhook); err != nil {
			t.Fatalf("failed to create webhook: %v", err)
		}
	}

	t.Run("List all webhooks", func(t *testing.T) {
		webhooks, err := repo.ListWebhooks()
		if err != nil {
			t.Fatalf("failed to list webhooks: %v", err)
		}

		if len(webhooks) != 3 {
			t.Errorf("expected 3 webhooks, got %d", len(webhooks))
		}
	})

	t.Run("List by event", func(t *testing.T) {
		webhooks, err := repo.ListWebhooksByEvent("event1")
		if err != nil {
			t.Fatalf("failed to list webhooks by event: %v", err)
		}

		// Only active webhooks are returned (2 out of 3, since we alternate active/inactive)
		if len(webhooks) != 2 {
			t.Errorf("expected 2 active webhooks, got %d", len(webhooks))
		}
	})

	t.Run("List by non-matching event", func(t *testing.T) {
		webhooks, err := repo.ListWebhooksByEvent("non.existent")
		if err != nil {
			t.Fatalf("failed to list webhooks by event: %v", err)
		}

		if len(webhooks) != 0 {
			t.Errorf("expected 0 webhooks, got %d", len(webhooks))
		}
	})
}

// TestWebhookRepo_Delivery tests webhook delivery operations
func TestWebhookRepo_Delivery(t *testing.T) {
	t.Parallel()

	st := openTestStoreForWebhooks(t)
	defer st.Close()

	repo := st.Webhooks()

	// Create a webhook first
	webhook := &Webhook{
		ID:        "wh-delivery-1",
		Name:      "Delivery Test",
		URL:       "https://example.com/delivery",
		Events:    []string{"test.event"},
		Active:    true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.CreateWebhook(webhook); err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	delivery := &WebhookDelivery{
		ID:          "wd-1",
		WebhookID:   webhook.ID,
		EventType:   "test.event",
		Payload:     []byte(`{"test":"data"}`),
		Status:      "pending",
		Attempt:     1,
		MaxAttempts: 3,
		CreatedAt:   time.Now().UTC(),
	}

	t.Run("Create delivery", func(t *testing.T) {
		err := repo.CreateDelivery(delivery)
		if err != nil {
			t.Errorf("failed to create delivery: %v", err)
		}
	})

	t.Run("Get deliveries", func(t *testing.T) {
		deliveries, err := repo.GetDeliveries(webhook.ID, 10)
		if err != nil {
			t.Fatalf("failed to get deliveries: %v", err)
		}

		if len(deliveries) != 1 {
			t.Errorf("expected 1 delivery, got %d", len(deliveries))
		}
	})

	t.Run("Get pending deliveries", func(t *testing.T) {
		pending, err := repo.GetPendingDeliveries(10)
		if err != nil {
			t.Fatalf("failed to get pending deliveries: %v", err)
		}

		if len(pending) != 1 {
			t.Errorf("expected 1 pending delivery, got %d", len(pending))
		}
	})

	t.Run("Update delivery", func(t *testing.T) {
		delivery.Status = "success"
		delivery.StatusCode = 200
		delivery.Response = "OK"
		now := time.Now().UTC()
		delivery.CompletedAt = &now

		err := repo.UpdateDelivery(delivery)
		if err != nil {
			t.Errorf("failed to update delivery: %v", err)
		}

		// Verify update - pending should now be 0
		pending, err := repo.GetPendingDeliveries(10)
		if err != nil {
			t.Fatalf("failed to get pending deliveries: %v", err)
		}

		if len(pending) != 0 {
			t.Errorf("expected 0 pending deliveries after update, got %d", len(pending))
		}
	})
}

// TestGenerateWebhookID tests the webhook ID generation
func TestGenerateWebhookID(t *testing.T) {
	t.Parallel()

	id1 := GenerateWebhookID()
	id2 := GenerateWebhookID()

	if id1 == "" {
		t.Error("expected non-empty ID")
	}

	if id1 == id2 {
		t.Error("expected unique IDs")
	}
}
