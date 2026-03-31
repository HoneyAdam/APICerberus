package observability

import (
	"testing"
	"time"
)

func TestNewWebhookManager(t *testing.T) {
	wm := NewWebhookManager()
	if wm == nil {
		t.Fatal("NewWebhookManager() returned nil")
	}
}

func TestWebhookManagerRegister(t *testing.T) {
	wm := NewWebhookManager()

	webhook := &Webhook{
		ID:     "webhook-1",
		URL:    "http://example.com/webhook",
		Events: []string{EventLowBalance, EventUserCreated},
	}

	err := wm.Register(webhook)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify it was registered
	got, ok := wm.Get("webhook-1")
	if !ok {
		t.Error("Expected webhook to be registered")
	}
	if got.URL != webhook.URL {
		t.Errorf("URL = %v, want %v", got.URL, webhook.URL)
	}
}

func TestWebhookManagerRegisterMissingID(t *testing.T) {
	wm := NewWebhookManager()

	webhook := &Webhook{
		URL:    "http://example.com/webhook",
		Events: []string{EventLowBalance},
	}

	err := wm.Register(webhook)
	if err == nil {
		t.Error("Register() expected error for missing ID")
	}
}

func TestWebhookManagerRegisterMissingURL(t *testing.T) {
	wm := NewWebhookManager()

	webhook := &Webhook{
		ID:     "webhook-1",
		Events: []string{EventLowBalance},
	}

	err := wm.Register(webhook)
	if err == nil {
		t.Error("Register() expected error for missing URL")
	}
}

func TestWebhookManagerRegisterNoEvents(t *testing.T) {
	wm := NewWebhookManager()

	webhook := &Webhook{
		ID:  "webhook-1",
		URL: "http://example.com/webhook",
	}

	err := wm.Register(webhook)
	if err == nil {
		t.Error("Register() expected error for no events")
	}
}

func TestWebhookManagerUnregister(t *testing.T) {
	wm := NewWebhookManager()

	webhook := &Webhook{
		ID:     "webhook-1",
		URL:    "http://example.com/webhook",
		Events: []string{EventLowBalance},
	}

	wm.Register(webhook)

	// Unregister
	wm.Unregister("webhook-1")

	// Verify it was removed
	_, ok := wm.Get("webhook-1")
	if ok {
		t.Error("Expected webhook to be unregistered")
	}
}

func TestWebhookManagerList(t *testing.T) {
	wm := NewWebhookManager()

	wm.Register(&Webhook{
		ID:     "webhook-1",
		URL:    "http://example.com/webhook1",
		Events: []string{EventLowBalance},
	})

	wm.Register(&Webhook{
		ID:     "webhook-2",
		URL:    "http://example.com/webhook2",
		Events: []string{EventUserCreated},
	})

	list := wm.List()
	if len(list) != 2 {
		t.Errorf("List() returned %d webhooks, want 2", len(list))
	}
}

func TestWebhookManagerDefaultRetries(t *testing.T) {
	wm := NewWebhookManager()

	webhook := &Webhook{
		ID:     "webhook-1",
		URL:    "http://example.com/webhook",
		Events: []string{EventLowBalance},
	}

	wm.Register(webhook)

	got, _ := wm.Get("webhook-1")
	if got.Retries != 3 {
		t.Errorf("Retries = %v, want 3", got.Retries)
	}
}

func TestWebhookEventStruct(t *testing.T) {
	event := &WebhookEvent{
		ID:        "evt_123",
		Type:      EventLowBalance,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id": "user-1",
			"balance": 10.0,
		},
	}

	if event.ID != "evt_123" {
		t.Errorf("ID = %v, want evt_123", event.ID)
	}

	if event.Type != EventLowBalance {
		t.Errorf("Type = %v, want %v", event.Type, EventLowBalance)
	}
}

func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !contains(slice, "a") {
		t.Error("Expected contains(slice, 'a') to be true")
	}

	if !contains(slice, "b") {
		t.Error("Expected contains(slice, 'b') to be true")
	}

	if contains(slice, "d") {
		t.Error("Expected contains(slice, 'd') to be false")
	}
}
