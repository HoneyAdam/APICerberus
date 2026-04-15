package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/store"
)

func newMockWebhookStore() *mockWebhookStore {
	return &mockWebhookStore{
		webhooks:   make(map[string]*store.Webhook),
		deliveries: make(map[string]*store.WebhookDelivery),
	}
}

// setupManager creates a WebhookManager with mockStore and registers webhooks
// directly in the manager's in-memory map (where processDelivery looks them up).
func setupManager(mockStore *mockWebhookStore) *WebhookManager {
	manager := NewWebhookManager(mockStore)
	// Copy webhooks from store into manager's in-memory map
	for id, wh := range mockStore.webhooks {
		manager.webhooks[id] = wh
	}
	return manager
}

func TestProcessDelivery_WebhookNotFound(t *testing.T) {
	t.Parallel()
	mockStore := newMockWebhookStore()
	manager := NewWebhookManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:        "del-1",
		WebhookID: "nonexistent",
		EventType: "route.created",
		Payload:   json.RawMessage(`{"id":"r1"}`),
		Status:    "pending",
	}

	manager.processDelivery(delivery)

	if delivery.Status != "failed" {
		t.Errorf("status = %q, want failed", delivery.Status)
	}
	if delivery.Error != "webhook not found" {
		t.Errorf("error = %q, want webhook not found", delivery.Error)
	}
	if delivery.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestProcessDelivery_InactiveWebhook(t *testing.T) {
	t.Parallel()
	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{ID: "wh-1", URL: "http://localhost:1", Active: false}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:        "del-1",
		WebhookID: "wh-1",
		EventType: "route.created",
		Payload:   json.RawMessage(`{}`),
		Status:    "pending",
	}

	manager.processDelivery(delivery)

	if delivery.Status != "failed" {
		t.Errorf("status = %q, want failed", delivery.Status)
	}
	if delivery.Error != "webhook is inactive" {
		t.Errorf("error = %q, want webhook is inactive", delivery.Error)
	}
}

func TestProcessDelivery_SuccessWith2xx(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{ID: "wh-1", URL: server.URL, Active: true}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		EventType:   "route.created",
		Payload:     json.RawMessage(`{"test":"data"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}

	manager.processDelivery(delivery)

	if delivery.Status != "success" {
		t.Errorf("status = %q, want success", delivery.Status)
	}
	if delivery.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", delivery.StatusCode)
	}
	if delivery.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestProcessDelivery_FailureWith5xx(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer server.Close()

	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{ID: "wh-1", URL: server.URL, Active: true}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		EventType:   "route.created",
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 1,
	}

	manager.processDelivery(delivery)

	if delivery.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", delivery.StatusCode)
	}
	if delivery.Status != "failed" {
		t.Errorf("status = %q, want failed", delivery.Status)
	}
}

func TestProcessDelivery_RequestError(t *testing.T) {
	t.Parallel()
	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{ID: "wh-1", URL: "http://127.0.0.1:1/unreachable", Active: true}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		EventType:   "route.created",
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 1,
	}

	manager.processDelivery(delivery)

	if delivery.Status != "failed" {
		t.Errorf("status = %q, want failed", delivery.Status)
	}
	if delivery.Error == "" {
		t.Error("Error should be set")
	}
}

func TestProcessDelivery_WithSignature(t *testing.T) {
	t.Parallel()
	var receivedSig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{ID: "wh-1", URL: server.URL, Active: true, Secret: "my-secret"}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		EventType:   "route.created",
		Payload:     json.RawMessage(`{"test":"sig"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}

	manager.processDelivery(delivery)

	if !strings.HasPrefix(receivedSig, "sha256=") {
		t.Errorf("signature = %q, want sha256= prefix", receivedSig)
	}
	if delivery.Status != "success" {
		t.Errorf("status = %q, want success", delivery.Status)
	}
}

func TestProcessDelivery_CustomHeaders(t *testing.T) {
	t.Parallel()
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{
		ID: "wh-1", URL: server.URL, Active: true,
		Headers: map[string]string{"X-Custom": "custom-value"},
	}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		EventType:   "route.created",
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}

	manager.processDelivery(delivery)

	if receivedHeader != "custom-value" {
		t.Errorf("X-Custom = %q, want custom-value", receivedHeader)
	}
}

func TestProcessDelivery_SetsStandardHeaders(t *testing.T) {
	t.Parallel()
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := newMockWebhookStore()
	mockStore.webhooks["wh-1"] = &store.Webhook{ID: "wh-1", URL: server.URL, Active: true}
	manager := setupManager(mockStore)

	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		EventType:   "route.updated",
		Payload:     json.RawMessage(`{}`),
		Status:      "pending",
		MaxAttempts: 3,
	}

	manager.processDelivery(delivery)

	if headers.Get("X-Webhook-ID") != "wh-1" {
		t.Errorf("X-Webhook-ID = %q, want wh-1", headers.Get("X-Webhook-ID"))
	}
	if headers.Get("X-Webhook-Event") != "route.updated" {
		t.Errorf("X-Webhook-Event = %q, want route.updated", headers.Get("X-Webhook-Event"))
	}
	if headers.Get("X-Webhook-Delivery") != "del-1" {
		t.Errorf("X-Webhook-Delivery = %q, want del-1", headers.Get("X-Webhook-Delivery"))
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", headers.Get("Content-Type"))
	}
}

func TestRetryOrFail_MaxAttemptsExceeded(t *testing.T) {
	t.Parallel()
	mockStore := newMockWebhookStore()
	manager := setupManager(mockStore)

	webhook := &store.Webhook{ID: "wh-1", URL: "http://localhost:1", Active: true}
	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		Attempt:     2,
		MaxAttempts: 2,
		Status:      "pending",
	}

	manager.retryOrFail(delivery, webhook)

	if delivery.Status != "failed" {
		t.Errorf("status = %q, want failed", delivery.Status)
	}
	if delivery.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestRetryOrFail_StillHasAttempts(t *testing.T) {
	t.Parallel()
	mockStore := newMockWebhookStore()
	manager := setupManager(mockStore)
	manager.Start()
	defer manager.Stop()

	webhook := &store.Webhook{ID: "wh-1", RetryInterval: 0}
	delivery := &store.WebhookDelivery{
		ID:          "del-1",
		WebhookID:   "wh-1",
		Attempt:     0,
		MaxAttempts: 3,
		Status:      "pending",
	}

	manager.retryOrFail(delivery, webhook)

	if delivery.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", delivery.Attempt)
	}
	if delivery.Status == "failed" {
		t.Error("should not be failed yet, still has attempts")
	}
}

func TestNewWebhookManager_NilStore(t *testing.T) {
	manager := NewWebhookManager(nil)
	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
	if manager.webhooks == nil {
		t.Error("webhooks map should be initialized")
	}
}

func TestWebhookManager_TriggerWithDelivery(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockStore := newMockWebhookStore()
	wh := &store.Webhook{
		ID: "wh-1", URL: server.URL, Active: true,
		Events: []string{"route.created"}, Secret: "test-secret",
	}
	mockStore.webhooks["wh-1"] = wh
	manager := setupManager(mockStore)
	manager.Start()
	defer manager.Stop()

	manager.Trigger("route.created", map[string]string{"route_id": "r-1"})
	time.Sleep(100 * time.Millisecond)

	if len(mockStore.deliveries) == 0 {
		t.Error("expected at least one delivery")
	}
}
