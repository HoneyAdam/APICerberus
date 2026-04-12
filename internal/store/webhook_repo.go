package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

// Webhook represents a configured webhook endpoint
type Webhook struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Secret        string            `json:"secret,omitempty"`
	Events        []string          `json:"events"`
	Headers       map[string]string `json:"headers,omitempty"`
	Active        bool              `json:"active"`
	RetryCount    int               `json:"retry_count"`
	RetryInterval int               `json:"retry_interval"` // seconds
	Timeout       int               `json:"timeout"`        // seconds
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastTriggered time.Time         `json:"last_triggered,omitempty"`
}

// WebhookDelivery represents a webhook delivery attempt
type WebhookDelivery struct {
	ID          string          `json:"id"`
	WebhookID   string          `json:"webhook_id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"` // "pending", "success", "failed"
	StatusCode  int             `json:"status_code,omitempty"`
	Response    string          `json:"response,omitempty"`
	Error       string          `json:"error,omitempty"`
	Attempt     int             `json:"attempt"`
	MaxAttempts int             `json:"max_attempts"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// WebhookRepo provides webhook persistence
type WebhookRepo struct {
	db  *sql.DB
	now func() time.Time
}

// Webhooks returns the webhook repository
func (s *Store) Webhooks() *WebhookRepo {
	if s == nil || s.db == nil {
		return nil
	}
	return &WebhookRepo{
		db:  s.db,
		now: time.Now,
	}
}

// CreateWebhook creates a new webhook
func (r *WebhookRepo) CreateWebhook(webhook *Webhook) error {
	if r == nil || r.db == nil {
		return errors.New("webhook repo is not initialized")
	}

	eventsJSON, _ := json.Marshal(webhook.Events)
	headersJSON, _ := json.Marshal(webhook.Headers)

	_, err := r.db.Exec(`
		INSERT INTO webhooks (id, name, url, secret, events, headers, active, retry_count, retry_interval, timeout, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, webhook.ID, webhook.Name, webhook.URL, webhook.Secret, string(eventsJSON), string(headersJSON),
		webhook.Active, webhook.RetryCount, webhook.RetryInterval, webhook.Timeout,
		webhook.CreatedAt.Format(time.RFC3339), webhook.UpdatedAt.Format(time.RFC3339))

	return err
}

// GetWebhook retrieves a webhook by ID
func (r *WebhookRepo) GetWebhook(id string) (*Webhook, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("webhook repo is not initialized")
	}

	row := r.db.QueryRow(`
		SELECT id, name, url, secret, events, headers, active, retry_count, retry_interval, timeout, created_at, updated_at, last_triggered
		FROM webhooks WHERE id = ?
	`, id)

	return r.scanWebhook(row)
}

// UpdateWebhook updates an existing webhook
func (r *WebhookRepo) UpdateWebhook(webhook *Webhook) error {
	if r == nil || r.db == nil {
		return errors.New("webhook repo is not initialized")
	}

	eventsJSON, _ := json.Marshal(webhook.Events)
	headersJSON, _ := json.Marshal(webhook.Headers)

	result, err := r.db.Exec(`
		UPDATE webhooks SET
			name = ?, url = ?, secret = ?, events = ?, headers = ?,
			active = ?, retry_count = ?, retry_interval = ?, timeout = ?, updated_at = ?
		WHERE id = ?
	`, webhook.Name, webhook.URL, webhook.Secret, string(eventsJSON), string(headersJSON),
		webhook.Active, webhook.RetryCount, webhook.RetryInterval, webhook.Timeout,
		webhook.UpdatedAt.Format(time.RFC3339), webhook.ID)

	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// DeleteWebhook deletes a webhook by ID
func (r *WebhookRepo) DeleteWebhook(id string) error {
	if r == nil || r.db == nil {
		return errors.New("webhook repo is not initialized")
	}

	result, err := r.db.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// ListWebhooks returns all webhooks
func (r *WebhookRepo) ListWebhooks() ([]*Webhook, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("webhook repo is not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, name, url, secret, events, headers, active, retry_count, retry_interval, timeout, created_at, updated_at, last_triggered
		FROM webhooks ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanWebhooks(rows)
}

// ListWebhooksByEvent returns webhooks subscribed to a specific event
func (r *WebhookRepo) ListWebhooksByEvent(eventType string) ([]*Webhook, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("webhook repo is not initialized")
	}

	// This is a simplified implementation - in production, you'd want to use JSON queries
	rows, err := r.db.Query(`
		SELECT id, name, url, secret, events, headers, active, retry_count, retry_interval, timeout, created_at, updated_at, last_triggered
		FROM webhooks WHERE active = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	webhooks, err := r.scanWebhooks(rows)
	if err != nil {
		return nil, err
	}

	// Filter by event type
	var filtered []*Webhook
	for _, webhook := range webhooks {
		for _, event := range webhook.Events {
			if event == eventType || event == "*" {
				filtered = append(filtered, webhook)
				break
			}
		}
	}

	return filtered, nil
}

// CreateDelivery creates a new delivery record
func (r *WebhookRepo) CreateDelivery(delivery *WebhookDelivery) error {
	if r == nil || r.db == nil {
		return errors.New("webhook repo is not initialized")
	}

	_, err := r.db.Exec(`
		INSERT INTO webhook_deliveries (id, webhook_id, event_type, payload, status, status_code, response, error, attempt, max_attempts, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, delivery.ID, delivery.WebhookID, delivery.EventType, string(delivery.Payload),
		delivery.Status, delivery.StatusCode, delivery.Response, delivery.Error,
		delivery.Attempt, delivery.MaxAttempts, delivery.CreatedAt.Format(time.RFC3339))

	return err
}

// UpdateDelivery updates a delivery record
func (r *WebhookRepo) UpdateDelivery(delivery *WebhookDelivery) error {
	if r == nil || r.db == nil {
		return errors.New("webhook repo is not initialized")
	}

	completedAt := ""
	if delivery.CompletedAt != nil {
		completedAt = delivery.CompletedAt.Format(time.RFC3339)
	}

	_, err := r.db.Exec(`
		UPDATE webhook_deliveries SET
			status = ?, status_code = ?, response = ?, error = ?, attempt = ?, completed_at = ?
		WHERE id = ?
	`, delivery.Status, delivery.StatusCode, delivery.Response, delivery.Error,
		delivery.Attempt, completedAt, delivery.ID)

	return err
}

// GetDeliveries returns delivery history for a webhook
func (r *WebhookRepo) GetDeliveries(webhookID string, limit int) ([]*WebhookDelivery, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("webhook repo is not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.Query(`
		SELECT id, webhook_id, event_type, payload, status, status_code, response, error, attempt, max_attempts, created_at, completed_at
		FROM webhook_deliveries
		WHERE webhook_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanDeliveries(rows)
}

// GetPendingDeliveries returns pending deliveries for retry
func (r *WebhookRepo) GetPendingDeliveries(limit int) ([]*WebhookDelivery, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("webhook repo is not initialized")
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.Query(`
		SELECT id, webhook_id, event_type, payload, status, status_code, response, error, attempt, max_attempts, created_at, completed_at
		FROM webhook_deliveries
		WHERE status = 'pending' OR (status = 'failed' AND attempt < max_attempts)
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanDeliveries(rows)
}

// scanWebhook scans a single webhook from a row
func (r *WebhookRepo) scanWebhook(row *sql.Row) (*Webhook, error) {
	var webhook Webhook
	var eventsJSON, headersJSON string
	var createdAt, updatedAt, lastTriggered string

	err := row.Scan(
		&webhook.ID, &webhook.Name, &webhook.URL, &webhook.Secret,
		&eventsJSON, &headersJSON, &webhook.Active, &webhook.RetryCount,
		&webhook.RetryInterval, &webhook.Timeout, &createdAt, &updatedAt, &lastTriggered,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(eventsJSON), &webhook.Events)
	_ = json.Unmarshal([]byte(headersJSON), &webhook.Headers)
	webhook.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	webhook.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	webhook.LastTriggered, _ = time.Parse(time.RFC3339, lastTriggered)

	return &webhook, nil
}

// scanWebhooks scans multiple webhooks from rows
func (r *WebhookRepo) scanWebhooks(rows *sql.Rows) ([]*Webhook, error) {
	var webhooks []*Webhook

	for rows.Next() {
		var webhook Webhook
		var eventsJSON, headersJSON string
		var createdAt, updatedAt, lastTriggered string

		err := rows.Scan(
			&webhook.ID, &webhook.Name, &webhook.URL, &webhook.Secret,
			&eventsJSON, &headersJSON, &webhook.Active, &webhook.RetryCount,
			&webhook.RetryInterval, &webhook.Timeout, &createdAt, &updatedAt, &lastTriggered,
		)
		if err != nil {
			return nil, err
		}

	if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
			log.Printf("[WARN] webhook_repo: failed to unmarshal events for webhook %s: %v", webhook.ID, err)
		}
		if err := json.Unmarshal([]byte(headersJSON), &webhook.Headers); err != nil {
			log.Printf("[WARN] webhook_repo: failed to unmarshal headers for webhook %s: %v", webhook.ID, err)
		}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			webhook.CreatedAt = t
		} else {
			log.Printf("[WARN] webhook_repo: failed to parse created_at for webhook %s: %v", webhook.ID, err)
		}
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			webhook.UpdatedAt = t
		} else {
			log.Printf("[WARN] webhook_repo: failed to parse updated_at for webhook %s: %v", webhook.ID, err)
		}
		if t, err := time.Parse(time.RFC3339, lastTriggered); err == nil {
			webhook.LastTriggered = t
		}

		webhooks = append(webhooks, &webhook)
	}

	return webhooks, rows.Err()
}

// scanDeliveries scans multiple deliveries from rows
func (r *WebhookRepo) scanDeliveries(rows *sql.Rows) ([]*WebhookDelivery, error) {
	var deliveries []*WebhookDelivery

	for rows.Next() {
		var delivery WebhookDelivery
		var payloadJSON string
		var createdAt, completedAt string

		err := rows.Scan(
			&delivery.ID, &delivery.WebhookID, &delivery.EventType, &payloadJSON,
			&delivery.Status, &delivery.StatusCode, &delivery.Response, &delivery.Error,
			&delivery.Attempt, &delivery.MaxAttempts, &createdAt, &completedAt,
		)
		if err != nil {
			return nil, err
		}

		delivery.Payload = json.RawMessage(payloadJSON)
		delivery.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if completedAt != "" {
			t, pErr := time.Parse(time.RFC3339, completedAt)
			if pErr != nil {
				log.Printf("[WARN] webhook_repo: failed to parse completed_at for delivery %s: %v", delivery.ID, pErr)
			}
			delivery.CompletedAt = &t
		}

		deliveries = append(deliveries, &delivery)
	}

	return deliveries, rows.Err()
}

// CreateWebhookTables creates the webhook tables if they don't exist
func (s *Store) CreateWebhookTables() error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}

	webhooksTable := `
		CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			secret TEXT NOT NULL DEFAULT '',
			events TEXT NOT NULL DEFAULT '[]',
			headers TEXT NOT NULL DEFAULT '{}',
			active INTEGER NOT NULL DEFAULT 1,
			retry_count INTEGER NOT NULL DEFAULT 3,
			retry_interval INTEGER NOT NULL DEFAULT 60,
			timeout INTEGER NOT NULL DEFAULT 30,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_triggered TEXT NOT NULL DEFAULT ''
		)
	`

	deliveriesTable := `
		CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id TEXT PRIMARY KEY,
			webhook_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			status_code INTEGER NOT NULL DEFAULT 0,
			response TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			completed_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(webhook_id) REFERENCES webhooks(id)
		)
	`

	if _, err := s.db.Exec(webhooksTable); err != nil {
		return fmt.Errorf("create webhooks table: %w", err)
	}

	if _, err := s.db.Exec(deliveriesTable); err != nil {
		return fmt.Errorf("create webhook_deliveries table: %w", err)
	}

	// Create indexes
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_webhooks_active ON webhooks(active)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created_at ON webhook_deliveries(created_at)`,
	}

	for _, idx := range indexes {
		if _, err := s.db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

// Helper methods to satisfy the WebhookStore interface

// CreateWebhook creates a webhook (satisfies WebhookStore interface)
func (s *Store) CreateWebhook(webhook *Webhook) error {
	return s.Webhooks().CreateWebhook(webhook)
}

// GetWebhook gets a webhook (satisfies WebhookStore interface)
func (s *Store) GetWebhook(id string) (*Webhook, error) {
	return s.Webhooks().GetWebhook(id)
}

// UpdateWebhook updates a webhook (satisfies WebhookStore interface)
func (s *Store) UpdateWebhook(webhook *Webhook) error {
	return s.Webhooks().UpdateWebhook(webhook)
}

// DeleteWebhook deletes a webhook (satisfies WebhookStore interface)
func (s *Store) DeleteWebhook(id string) error {
	return s.Webhooks().DeleteWebhook(id)
}

// ListWebhooks lists all webhooks (satisfies WebhookStore interface)
func (s *Store) ListWebhooks() ([]*Webhook, error) {
	return s.Webhooks().ListWebhooks()
}

// ListWebhooksByEvent lists webhooks by event (satisfies WebhookStore interface)
func (s *Store) ListWebhooksByEvent(eventType string) ([]*Webhook, error) {
	return s.Webhooks().ListWebhooksByEvent(eventType)
}

// CreateDelivery creates a delivery (satisfies WebhookStore interface)
func (s *Store) CreateDelivery(delivery *WebhookDelivery) error {
	return s.Webhooks().CreateDelivery(delivery)
}

// UpdateDelivery updates a delivery (satisfies WebhookStore interface)
func (s *Store) UpdateDelivery(delivery *WebhookDelivery) error {
	return s.Webhooks().UpdateDelivery(delivery)
}

// GetDeliveries gets deliveries for a webhook (satisfies WebhookStore interface)
func (s *Store) GetDeliveries(webhookID string, limit int) ([]*WebhookDelivery, error) {
	return s.Webhooks().GetDeliveries(webhookID, limit)
}

// GetPendingDeliveries gets pending deliveries (satisfies WebhookStore interface)
func (s *Store) GetPendingDeliveries(limit int) ([]*WebhookDelivery, error) {
	return s.Webhooks().GetPendingDeliveries(limit)
}

// Helper function to generate webhook ID
func GenerateWebhookID() string {
	id, _ := uuid.NewString()
	return "wh_" + strings.ToLower(id[:8])
}
