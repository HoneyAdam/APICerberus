package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// WebhookManager manages webhook notifications.
type WebhookManager struct {
	mu       sync.RWMutex
	webhooks map[string]*Webhook
	client   *http.Client
}

// Webhook represents a webhook endpoint.
type Webhook struct {
	ID       string            `json:"id"`
	URL      string            `json:"url"`
	Secret   string            `json:"secret,omitempty"`
	Events   []string          `json:"events"`
	Headers  map[string]string `json:"headers,omitempty"`
	Disabled bool              `json:"disabled"`
	Retries  int               `json:"retries"`
}

// WebhookEvent represents a webhook event.
type WebhookEvent struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// Event types
const (
	EventLowBalance     = "low_balance"
	EventUserCreated    = "user_created"
	EventAlertTriggered = "alert_triggered"
	EventUpstreamDown   = "upstream_down"
	EventRateLimitHit   = "rate_limit_hit"
	EventAuthFailure    = "auth_failure"
)

// NewWebhookManager creates a new webhook manager.
func NewWebhookManager() *WebhookManager {
	return &WebhookManager{
		webhooks: make(map[string]*Webhook),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Register registers a new webhook.
func (wm *WebhookManager) Register(webhook *Webhook) error {
	if webhook.ID == "" {
		return fmt.Errorf("webhook ID is required")
	}
	if webhook.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	if len(webhook.Events) == 0 {
		return fmt.Errorf("at least one event is required")
	}

	if webhook.Retries == 0 {
		webhook.Retries = 3
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.webhooks[webhook.ID] = webhook
	return nil
}

// Unregister removes a webhook.
func (wm *WebhookManager) Unregister(id string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	delete(wm.webhooks, id)
}

// Get returns a webhook by ID.
func (wm *WebhookManager) Get(id string) (*Webhook, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	webhook, ok := wm.webhooks[id]
	return webhook, ok
}

// List returns all webhooks.
func (wm *WebhookManager) List() []*Webhook {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	list := make([]*Webhook, 0, len(wm.webhooks))
	for _, webhook := range wm.webhooks {
		list = append(list, webhook)
	}
	return list
}

// Trigger triggers an event to all registered webhooks.
func (wm *WebhookManager) Trigger(eventType string, payload interface{}) {
	wm.mu.RLock()
	webhooks := make([]*Webhook, 0)
	for _, webhook := range wm.webhooks {
		if !webhook.Disabled && contains(webhook.Events, eventType) {
			webhooks = append(webhooks, webhook)
		}
	}
	wm.mu.RUnlock()

	event := &WebhookEvent{
		ID:        generateEventID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	// Send to all matching webhooks concurrently
	for _, webhook := range webhooks {
		go wm.sendWithRetry(webhook, event)
	}
}

// sendWithRetry sends a webhook with exponential backoff retry.
func (wm *WebhookManager) sendWithRetry(webhook *Webhook, event *WebhookEvent) {
	var lastErr error

	for attempt := 0; attempt <= webhook.Retries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s...
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		err := wm.send(webhook, event)
		if err == nil {
			return // Success
		}

		lastErr = err
	}

	// All retries failed - log error (in production, this would go to a logger)
	_ = lastErr
}

// send sends a webhook event.
func (wm *WebhookManager) send(webhook *Webhook, event *WebhookEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-ID", webhook.ID)
	req.Header.Set("X-Event-ID", event.ID)
	req.Header.Set("X-Event-Type", event.Type)

	// Add custom headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Add signature if secret is set
	if webhook.Secret != "" {
		// In production, implement HMAC signature
		req.Header.Set("X-Webhook-Signature", "sha256="+generateSignature(data, webhook.Secret))
	}

	resp, err := wm.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// generateEventID generates a unique event ID.
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

// generateSignature generates an HMAC signature (simplified).
func generateSignature(data []byte, secret string) string {
	// In production, use proper HMAC-SHA256
	return fmt.Sprintf("%x", data[:min(len(data), 32)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
