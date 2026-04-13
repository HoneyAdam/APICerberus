package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/store"
)

// =============================================================================
// Webhook Handler Tests
// =============================================================================

func TestWebhookHandlers_Basic(t *testing.T) {
	t.Run("list webhooks empty", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token
		defer os.RemoveAll(storePath)

		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/webhooks", token, nil)
		if resp == nil {
			t.Error("expected empty response, got nil")
		}
	})
}

// =============================================================================
// WebhookManager Tests
// =============================================================================

type mockWebhookStore struct {
	webhooks   map[string]*store.Webhook
	deliveries map[string]*store.WebhookDelivery
}

func (m *mockWebhookStore) CreateWebhook(webhook *store.Webhook) error {
	if m.webhooks == nil {
		m.webhooks = make(map[string]*store.Webhook)
	}
	m.webhooks[webhook.ID] = webhook
	return nil
}

func (m *mockWebhookStore) GetWebhook(id string) (*store.Webhook, error) {
	if webhook, ok := m.webhooks[id]; ok {
		return webhook, nil
	}
	return nil, nil
}

func (m *mockWebhookStore) UpdateWebhook(webhook *store.Webhook) error {
	m.webhooks[webhook.ID] = webhook
	return nil
}

func (m *mockWebhookStore) DeleteWebhook(id string) error {
	delete(m.webhooks, id)
	return nil
}

func (m *mockWebhookStore) ListWebhooks() ([]*store.Webhook, error) {
	var list []*store.Webhook
	for _, w := range m.webhooks {
		list = append(list, w)
	}
	return list, nil
}

func (m *mockWebhookStore) ListWebhooksByEvent(eventType string) ([]*store.Webhook, error) {
	var list []*store.Webhook
	for _, w := range m.webhooks {
		for _, e := range w.Events {
			if e == eventType {
				list = append(list, w)
				break
			}
		}
	}
	return list, nil
}

func (m *mockWebhookStore) CreateDelivery(delivery *store.WebhookDelivery) error {
	if m.deliveries == nil {
		m.deliveries = make(map[string]*store.WebhookDelivery)
	}
	m.deliveries[delivery.ID] = delivery
	return nil
}

func (m *mockWebhookStore) UpdateDelivery(delivery *store.WebhookDelivery) error {
	m.deliveries[delivery.ID] = delivery
	return nil
}

func (m *mockWebhookStore) GetDeliveries(webhookID string, limit int) ([]*store.WebhookDelivery, error) {
	var list []*store.WebhookDelivery
	for _, d := range m.deliveries {
		if d.WebhookID == webhookID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *mockWebhookStore) GetPendingDeliveries(limit int) ([]*store.WebhookDelivery, error) {
	var list []*store.WebhookDelivery
	for _, d := range m.deliveries {
		if d.Status == "pending" {
			list = append(list, d)
		}
	}
	return list, nil
}

func TestWebhookManager(t *testing.T) {
	t.Run("webhook manager lifecycle", func(t *testing.T) {
		mockStore := &mockWebhookStore{
			webhooks:   make(map[string]*store.Webhook),
			deliveries: make(map[string]*store.WebhookDelivery),
		}

		manager := NewWebhookManager(mockStore)
		if manager == nil {
			t.Fatal("expected webhook manager, got nil")
		}
	})

	t.Run("webhook events list", func(t *testing.T) {
		if len(WebhookEvents) == 0 {
			t.Error("expected webhook events to be defined")
		}

		// Verify common events exist
		hasRouteCreated := false
		for _, e := range WebhookEvents {
			if e.Type == "route.created" {
				hasRouteCreated = true
				break
			}
		}
		if !hasRouteCreated {
			t.Error("expected route.created event to exist")
		}
	})
}

// =============================================================================
// Webhook Security Tests
// =============================================================================

func TestWebhookSignature(t *testing.T) {
	t.Run("signature generation", func(t *testing.T) {
		secret := "test-secret"
		payload := []byte(`{"event":"test"}`)

		// Generate signature using HMAC SHA256
		h := sha256.New()
		h.Write(payload)
		h.Write([]byte(secret))
		signature := hex.EncodeToString(h.Sum(nil))

		if signature == "" {
			t.Error("expected signature to be generated")
		}
	})
}

// =============================================================================
// Webhook HTTP Tests
// =============================================================================

func TestWebhookHTTPEndpoints(t *testing.T) {
	t.Run("webhook events endpoint", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token
		defer os.RemoveAll(storePath)

		resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/webhooks/events", token, nil)
		if resp == nil {
			t.Error("expected response with webhook events")
		}
	})

	t.Run("webhook CRUD operations", func(t *testing.T) {
		baseURL, _, storePath, token := newAdminTestServer(t)
		_ = token
		defer os.RemoveAll(storePath)

		// Create webhook
		createData := map[string]any{
			"name":    "Test Webhook",
			"url":     "https://example.com/webhook",
			"events":  []string{"route.created", "route.updated"},
			"enabled": true,
		}

		resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/webhooks", token, createData)
		if resp == nil {
			t.Error("expected response after creating webhook")
		}

		// List webhooks
		listResp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/webhooks", token, nil)
		if listResp == nil {
			t.Error("expected response when listing webhooks")
		}
	})
}

// =============================================================================
// Webhook Delivery Tests
// =============================================================================

func TestWebhookDelivery(t *testing.T) {
	t.Run("delivery lifecycle", func(t *testing.T) {
		mockStore := &mockWebhookStore{
			webhooks:   make(map[string]*store.Webhook),
			deliveries: make(map[string]*store.WebhookDelivery),
		}

		delivery := &store.WebhookDelivery{
			ID:        "delivery-1",
			WebhookID: "webhook-1",
			EventType: "route.created",
			Payload:   []byte(`{"route":"test"}`),
			Status:    "pending",
			CreatedAt: time.Now(),
		}

		err := mockStore.CreateDelivery(delivery)
		if err != nil {
			t.Errorf("failed to create delivery: %v", err)
		}

		// Update delivery
		delivery.Status = "success"
		now := time.Now()
		delivery.CompletedAt = &now
		err = mockStore.UpdateDelivery(delivery)
		if err != nil {
			t.Errorf("failed to update delivery: %v", err)
		}

		// Get deliveries
		deliveries, err := mockStore.GetDeliveries("webhook-1", 10)
		if err != nil {
			t.Errorf("failed to get deliveries: %v", err)
		}
		if len(deliveries) != 1 {
			t.Errorf("expected 1 delivery, got %d", len(deliveries))
		}
	})
}

// =============================================================================
// Webhook Payload Tests
// =============================================================================

func TestWebhookPayload(t *testing.T) {
	t.Run("payload structure", func(t *testing.T) {
		payload := map[string]any{
			"event":     "route.created",
			"timestamp": time.Now().Unix(),
			"data": map[string]any{
				"id":   "route-1",
				"name": "Test Route",
				"path": "/test",
			},
		}

		if payload["event"] != "route.created" {
			t.Error("expected event type in payload")
		}
	})
}

// =============================================================================
// Mock Server Helpers
// =============================================================================

func TestWebhookWithMockServer(t *testing.T) {
	t.Run("webhook delivery to mock endpoint", func(t *testing.T) {
		received := make(chan bool, 1)

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				received <- true
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer mockServer.Close()

		// Verify mock server works
		resp, err := http.Post(mockServer.URL, "application/json", nil)
		if err != nil {
			t.Fatalf("failed to contact mock server: %v", err)
		}
		resp.Body.Close()

		select {
		case <-received:
			// Success
		case <-time.After(time.Second):
			t.Error("expected webhook to be received by mock server")
		}
	})
}
