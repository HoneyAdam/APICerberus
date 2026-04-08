package admin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"log"
	"strings"
	"sync"
	"time"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// WebhookEvent represents a webhook event type
type WebhookEvent struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// WebhookManager manages webhooks and their delivery
type WebhookManager struct {
	mu         sync.RWMutex
	webhooks   map[string]*store.Webhook
	deliveryCh chan *store.WebhookDelivery
	client     *http.Client
	store      WebhookStore
}

// WebhookStore defines the interface for webhook persistence
type WebhookStore interface {
	CreateWebhook(webhook *store.Webhook) error
	GetWebhook(id string) (*store.Webhook, error)
	UpdateWebhook(webhook *store.Webhook) error
	DeleteWebhook(id string) error
	ListWebhooks() ([]*store.Webhook, error)
	ListWebhooksByEvent(eventType string) ([]*store.Webhook, error)
	CreateDelivery(delivery *store.WebhookDelivery) error
	UpdateDelivery(delivery *store.WebhookDelivery) error
	GetDeliveries(webhookID string, limit int) ([]*store.WebhookDelivery, error)
	GetPendingDeliveries(limit int) ([]*store.WebhookDelivery, error)
}

// Available webhook events
var WebhookEvents = []WebhookEvent{
	{Type: "route.created", Description: "Route created", Category: "route"},
	{Type: "route.updated", Description: "Route updated", Category: "route"},
	{Type: "route.deleted", Description: "Route deleted", Category: "route"},
	{Type: "service.created", Description: "Service created", Category: "service"},
	{Type: "service.updated", Description: "Service updated", Category: "service"},
	{Type: "service.deleted", Description: "Service deleted", Category: "service"},
	{Type: "upstream.created", Description: "Upstream created", Category: "upstream"},
	{Type: "upstream.updated", Description: "Upstream updated", Category: "upstream"},
	{Type: "upstream.deleted", Description: "Upstream deleted", Category: "upstream"},
	{Type: "consumer.created", Description: "Consumer created", Category: "consumer"},
	{Type: "consumer.updated", Description: "Consumer updated", Category: "consumer"},
	{Type: "consumer.deleted", Description: "Consumer deleted", Category: "consumer"},
	{Type: "user.created", Description: "User created", Category: "user"},
	{Type: "user.updated", Description: "User updated", Category: "user"},
	{Type: "user.deleted", Description: "User deleted", Category: "user"},
	{Type: "api_key.created", Description: "API key created", Category: "api_key"},
	{Type: "api_key.revoked", Description: "API key revoked", Category: "api_key"},
	{Type: "rate_limit.triggered", Description: "Rate limit triggered", Category: "security"},
	{Type: "block.triggered", Description: "Request blocked", Category: "security"},
	{Type: "alert.triggered", Description: "Alert triggered", Category: "alert"},
	{Type: "config.reloaded", Description: "Configuration reloaded", Category: "system"},
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(webhookStore WebhookStore) *WebhookManager {
	return &WebhookManager{
		webhooks:   make(map[string]*store.Webhook),
		deliveryCh: make(chan *store.WebhookDelivery, 1000),
		client:     &http.Client{Timeout: 30 * time.Second},
		store:      webhookStore,
	}
}

// Start starts the webhook delivery worker
func (m *WebhookManager) Start() {
	go m.deliveryWorker()
}

// Stop stops the webhook manager
func (m *WebhookManager) Stop() {
	close(m.deliveryCh)
}

// deliveryWorker processes webhook deliveries
func (m *WebhookManager) deliveryWorker() {
	for delivery := range m.deliveryCh {
		m.processDelivery(delivery)
	}
}

// processDelivery processes a single webhook delivery
func (m *WebhookManager) processDelivery(delivery *store.WebhookDelivery) {
	m.mu.RLock()
	webhook, exists := m.webhooks[delivery.WebhookID]
	m.mu.RUnlock()

	if !exists {
		delivery.Status = "failed"
		delivery.Error = "webhook not found"
		now := time.Now().UTC()
		delivery.CompletedAt = &now
		if err := m.store.UpdateDelivery(delivery); err != nil {
			log.Printf("[WARN] webhook: failed to update delivery %s: %v", delivery.ID, err)
		}
		return
	}

	if !webhook.Active {
		delivery.Status = "failed"
		delivery.Error = "webhook is inactive"
		now := time.Now().UTC()
		delivery.CompletedAt = &now
		if err := m.store.UpdateDelivery(delivery); err != nil {
			log.Printf("[WARN] webhook: failed to update delivery %s: %v", delivery.ID, err)
		}
		return
	}

	// Prepare request
	payload, err := json.Marshal(delivery.Payload)
	if err != nil {
		delivery.Error = fmt.Sprintf("failed to marshal payload: %v", err)
		m.retryOrFail(delivery, webhook)
		return
	}

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewReader(payload))
	if err != nil {
		delivery.Error = fmt.Sprintf("failed to create request: %v", err)
		m.retryOrFail(delivery, webhook)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-ID", delivery.WebhookID)
	req.Header.Set("X-Webhook-Event", delivery.EventType)
	req.Header.Set("X-Webhook-Delivery", delivery.ID)
	req.Header.Set("X-Webhook-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	// Add custom headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Sign payload if secret is configured
	if webhook.Secret != "" {
		signature := m.signPayload(payload, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Set timeout
	timeout := time.Duration(webhook.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		delivery.Error = fmt.Sprintf("request failed: %v", err)
		m.retryOrFail(delivery, webhook)
		return
	}
	defer resp.Body.Close()

	delivery.StatusCode = resp.StatusCode

	// Read response
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	delivery.Response = buf.String()

	// Check status
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Status = "success"
		now := time.Now().UTC()
		delivery.CompletedAt = &now
		if err := m.store.UpdateDelivery(delivery); err != nil {
			log.Printf("[WARN] webhook: failed to update delivery %s: %v", delivery.ID, err)
		}

		// Update last triggered time
		m.mu.Lock()
		webhook.LastTriggered = time.Now().UTC()
		m.mu.Unlock()
	} else {
		delivery.Error = fmt.Sprintf("received status code %d", resp.StatusCode)
		m.retryOrFail(delivery, webhook)
	}
}

// retryOrFail either retries the delivery or marks it as failed
func (m *WebhookManager) retryOrFail(delivery *store.WebhookDelivery, webhook *store.Webhook) {
	delivery.Attempt++

	if delivery.Attempt < delivery.MaxAttempts {
		// Schedule retry
		go func() {
			interval := time.Duration(webhook.RetryInterval) * time.Second
			if interval == 0 {
				interval = 60 * time.Second
			}
			time.Sleep(interval)
			m.deliveryCh <- delivery
		}()
	} else {
		delivery.Status = "failed"
		now := time.Now().UTC()
		delivery.CompletedAt = &now
		if err := m.store.UpdateDelivery(delivery); err != nil {
			log.Printf("[WARN] webhook: failed to update delivery %s: %v", delivery.ID, err)
		}
	}
}

// signPayload creates an HMAC signature for the payload
func (m *WebhookManager) signPayload(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// Trigger triggers a webhook event
func (m *WebhookManager) Trigger(eventType string, payload any) {
	webhooks, err := m.store.ListWebhooksByEvent(eventType)
	if err != nil {
		log.Printf("[WARN] webhook: failed to list webhooks for event %s: %v", eventType, err)
		return
	}

	payloadBytes, _ := json.Marshal(payload)

	for _, webhook := range webhooks {
		if !webhook.Active {
			continue
		}

		// Check if webhook is subscribed to this event
		subscribed := false
		for _, event := range webhook.Events {
			if event == eventType || event == "*" {
				subscribed = true
				break
			}
		}
		if !subscribed {
			continue
		}

		delivery := &store.WebhookDelivery{
			ID:          generateDeliveryID(),
			WebhookID:   webhook.ID,
			EventType:   eventType,
			Payload:     payloadBytes,
			Status:      "pending",
			Attempt:     0,
			MaxAttempts: webhook.RetryCount + 1,
			CreatedAt:   time.Now().UTC(),
		}

		if err := m.store.CreateDelivery(delivery); err != nil {
			log.Printf("[WARN] webhook: failed to create delivery for webhook %s: %v", delivery.WebhookID, err)
		}

		select {
		case m.deliveryCh <- delivery:
		default:
			// Channel full, delivery will be picked up by retry worker
		}
	}
}

// generateDeliveryID generates a unique delivery ID
func generateDeliveryID() string {
	id, _ := uuid.NewString()
	return "del_" + id
}

// HTTP Handlers

// handleListWebhooks lists all webhooks
func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	webhooks, err := st.ListWebhooks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}

	// Clear secrets before returning
	for _, webhook := range webhooks {
		webhook.Secret = ""
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, webhooks)
}

// handleCreateWebhook creates a new webhook
func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req store.Webhook
	if err := jsonutil.ReadJSON(r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	// Validate
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "Name is required")
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "URL is required")
		return
	}
	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "At least one event is required")
		return
	}

	// Generate ID if not provided
	if strings.TrimSpace(req.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		req.ID = id
	}

	// Set defaults
	if req.RetryCount == 0 {
		req.RetryCount = 3
	}
	if req.RetryInterval == 0 {
		req.RetryInterval = 60
	}
	if req.Timeout == 0 {
		req.Timeout = 30
	}
	req.Active = true
	req.CreatedAt = time.Now().UTC()
	req.UpdatedAt = req.CreatedAt

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	if err := st.CreateWebhook(&req); err != nil {
		writeError(w, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}

	// Return webhook without secret
	req.Secret = ""
	_ = jsonutil.WriteJSON(w, http.StatusCreated, req)
}

// handleGetWebhook gets a specific webhook
func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	webhook, err := st.GetWebhook(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}

	// Clear secret
	webhook.Secret = ""
	_ = jsonutil.WriteJSON(w, http.StatusOK, webhook)
}

// handleUpdateWebhook updates a webhook
func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req store.Webhook
	if err := jsonutil.ReadJSON(r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	existing, err := st.GetWebhook(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}

	// Update fields
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.URL != "" {
		existing.URL = req.URL
	}
	if req.Secret != "" {
		existing.Secret = req.Secret
	}
	if len(req.Events) > 0 {
		existing.Events = req.Events
	}
	if len(req.Headers) > 0 {
		existing.Headers = req.Headers
	}
	if req.RetryCount > 0 {
		existing.RetryCount = req.RetryCount
	}
	if req.RetryInterval > 0 {
		existing.RetryInterval = req.RetryInterval
	}
	if req.Timeout > 0 {
		existing.Timeout = req.Timeout
	}
	existing.Active = req.Active
	existing.UpdatedAt = time.Now().UTC()

	if err := st.UpdateWebhook(existing); err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	// Return without secret
	existing.Secret = ""
	_ = jsonutil.WriteJSON(w, http.StatusOK, existing)
}

// handleDeleteWebhook deletes a webhook
func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	if err := st.DeleteWebhook(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListWebhookEvents lists available webhook events
func (s *Server) handleListWebhookEvents(w http.ResponseWriter, r *http.Request) {
	_ = jsonutil.WriteJSON(w, http.StatusOK, WebhookEvents)
}

// handleListWebhookDeliveries lists delivery history for a webhook
func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	deliveries, err := st.GetDeliveries(id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, deliveries)
}

// handleTestWebhook tests a webhook by sending a test event
func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	webhook, err := st.GetWebhook(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}

	// Create test payload
	testPayload := map[string]any{
		"event":      "test",
		"webhook_id": id,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"message":    "This is a test event",
	}

	payloadBytes, _ := json.Marshal(testPayload)

	delivery := &store.WebhookDelivery{
		ID:          generateDeliveryID(),
		WebhookID:   webhook.ID,
		EventType:   "test",
		Payload:     payloadBytes,
		Status:      "pending",
		Attempt:     0,
		MaxAttempts: 1,
		CreatedAt:   time.Now().UTC(),
	}

	_ = st.CreateDelivery(delivery)

	// Send test request synchronously
	// #nosec G704 -- webhook.URL is administrator-configured by design; SSRF protection is an admin-level responsibility.
	req, _ := http.NewRequest("POST", webhook.URL, bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-ID", webhook.ID)
	req.Header.Set("X-Webhook-Event", "test")
	req.Header.Set("X-Webhook-Delivery", delivery.ID)
	req.Header.Set("X-Webhook-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	if webhook.Secret != "" {
		h := hmac.New(sha256.New, []byte(webhook.Secret))
		h.Write(payloadBytes)
		signature := hex.EncodeToString(h.Sum(nil))
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	timeout := time.Duration(webhook.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	// #nosec G704 -- webhook URL is intentionally administrator-configured.
	resp, err := client.Do(req)
	if err != nil {
		delivery.Status = "failed"
		delivery.Error = err.Error()
		now := time.Now().UTC()
		delivery.CompletedAt = &now
		_ = st.UpdateDelivery(delivery)

		writeError(w, http.StatusBadGateway, "test_failed", err.Error())
		return
	}
	defer resp.Body.Close()

	delivery.StatusCode = resp.StatusCode
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	delivery.Response = buf.String()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Status = "success"
	} else {
		delivery.Status = "failed"
		delivery.Error = fmt.Sprintf("received status code %d", resp.StatusCode)
	}

	now := time.Now().UTC()
	delivery.CompletedAt = &now
	_ = st.UpdateDelivery(delivery)

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success":     delivery.Status == "success",
		"status_code": resp.StatusCode,
		"response":    delivery.Response,
	})
}

// handleRotateWebhookSecret rotates the webhook secret
func (s *Server) handleRotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	st := s.gateway.Store()
	if st == nil {
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "Store not available")
		return
	}

	webhook, err := st.GetWebhook(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook_not_found", "Webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}

	// Generate new secret
	newSecret, err := uuid.NewString()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "secret_generation_failed", err.Error())
		return
	}

	webhook.Secret = newSecret
	webhook.UpdatedAt = time.Now().UTC()

	if err := st.UpdateWebhook(webhook); err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	// Return new secret (only time it's visible)
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"webhook_id": id,
		"secret":     newSecret,
		"message":    "Secret rotated successfully. Store this secret securely as it will not be shown again.",
	})
}

// RegisterWebhookRoutes registers webhook management endpoints
func (s *Server) RegisterWebhookRoutes() {
	s.handle("GET /admin/api/v1/webhooks", s.handleListWebhooks)
	s.handle("POST /admin/api/v1/webhooks", s.handleCreateWebhook)
	s.handle("GET /admin/api/v1/webhooks/events", s.handleListWebhookEvents)
	s.handle("GET /admin/api/v1/webhooks/{id}", s.handleGetWebhook)
	s.handle("PUT /admin/api/v1/webhooks/{id}", s.handleUpdateWebhook)
	s.handle("DELETE /admin/api/v1/webhooks/{id}", s.handleDeleteWebhook)
	s.handle("GET /admin/api/v1/webhooks/{id}/deliveries", s.handleListWebhookDeliveries)
	s.handle("POST /admin/api/v1/webhooks/{id}/test", s.handleTestWebhook)
	s.handle("POST /admin/api/v1/webhooks/{id}/rotate-secret", s.handleRotateWebhookSecret)
}

// ValidateWebhookSignature validates a webhook signature
func ValidateWebhookSignature(payload []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	signature = strings.TrimPrefix(signature, "sha256=")

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expected := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}
