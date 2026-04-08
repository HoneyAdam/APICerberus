package analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// WebhookTemplateType represents predefined webhook template types.
type WebhookTemplateType string

const (
	WebhookTemplateCustom    WebhookTemplateType = "custom"
	WebhookTemplateSlack     WebhookTemplateType = "slack"
	WebhookTemplateDiscord   WebhookTemplateType = "discord"
	WebhookTemplatePagerDuty WebhookTemplateType = "pagerduty"
	WebhookTemplateTeams     WebhookTemplateType = "teams"
	WebhookTemplateGeneric   WebhookTemplateType = "generic"
	WebhookTemplateTelegram  WebhookTemplateType = "telegram"
)

// WebhookTemplateData contains variables available in webhook templates.
type WebhookTemplateData struct {
	// Rule information
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	RuleType    string `json:"rule_type"`
	Description string `json:"description"`

	// Alert values
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Unit      string  `json:"unit"`
	Condition string  `json:"condition"`

	// Timing
	Timestamp   time.Time `json:"timestamp"`
	TriggeredAt time.Time `json:"triggered_at"`

	// Gateway information
	Gateway string `json:"gateway"`
	NodeID  string `json:"node_id"`
	Cluster string `json:"cluster"`

	// Additional context
	Details      map[string]any `json:"details"`
	URL          string                 `json:"url"`
	DashboardURL string                 `json:"dashboard_url"`

	// Computed fields
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// WebhookTemplate represents a webhook notification template.
type WebhookTemplate struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Type        WebhookTemplateType `json:"type"`
	ContentType string              `json:"content_type"`
	Headers     map[string]string   `json:"headers"`
	Body        string              `json:"body"`
	Method      string              `json:"method"`
	Enabled     bool                `json:"enabled"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// WebhookTemplateEngine manages webhook templates and rendering.
type WebhookTemplateEngine struct {
	templates map[string]WebhookTemplate
	compiled  map[string]*template.Template
}

// NewWebhookTemplateEngine creates a new template engine with default templates.
func NewWebhookTemplateEngine() *WebhookTemplateEngine {
	engine := &WebhookTemplateEngine{
		templates: make(map[string]WebhookTemplate),
		compiled:  make(map[string]*template.Template),
	}

	// Register default templates
	engine.RegisterDefaultTemplates()

	return engine
}

// RegisterDefaultTemplates registers all built-in templates.
func (e *WebhookTemplateEngine) RegisterDefaultTemplates() {
	// Slack template
	_ = e.RegisterTemplate(WebhookTemplate{ // #nosec G104 // Built-in template, failure is impossible.
		ID:          "slack-default",
		Name:        "Slack (Default)",
		Type:        WebhookTemplateSlack,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Body: `{
  "text": "API Cerberus Alert: {{.RuleName}}",
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "🚨 {{.RuleName}}",
        "emoji": true
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Rule Type:*\n{{.RuleType}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Severity:*\n{{.Severity}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Current Value:*\n{{.Value}}"
        },
        {
          "type": "mrkdwn",
          "text": "*Threshold:*\n{{.Threshold}}"
        }
      ]
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Condition:* {{.Condition}}\n*Time:* {{.TriggeredAt.Format "2006-01-02 15:04:05 UTC"}}"
      }
    },
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {
            "type": "plain_text",
            "text": "View Dashboard",
            "emoji": true
          },
          "url": "{{.DashboardURL}}",
          "style": "primary"
        }
      ]
    }
  ]
}`,
		Enabled: true,
	})

	// Discord template
	_ = e.RegisterTemplate(WebhookTemplate{ // #nosec G104
		ID:          "discord-default",
		Name:        "Discord (Default)",
		Type:        WebhookTemplateDiscord,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Body: `{
  "username": "API Cerberus",
  "avatar_url": "https://apicerberus.com/logo.png",
  "embeds": [
    {
      "title": "🚨 {{.RuleName}}",
      "description": "Alert triggered for {{.Gateway}}",
      "color": 15158332,
      "fields": [
        {
          "name": "Rule Type",
          "value": "{{.RuleType}}",
          "inline": true
        },
        {
          "name": "Severity",
          "value": "{{.Severity}}",
          "inline": true
        },
        {
          "name": "Current Value",
          "value": "{{.Value}}",
          "inline": true
        },
        {
          "name": "Threshold",
          "value": "{{.Threshold}}",
          "inline": true
        },
        {
          "name": "Condition",
          "value": "{{.Condition}}"
        },
        {
          "name": "Time",
          "value": "{{.TriggeredAt.Format "2006-01-02 15:04:05 UTC"}}"
        }
      ],
      "timestamp": "{{.TriggeredAt.Format "2006-01-02T15:04:05Z"}}"
    }
  ]
}`,
		Enabled: true,
	})

	// PagerDuty template
	_ = e.RegisterTemplate(WebhookTemplate{ // #nosec G104
		ID:          "pagerduty-default",
		Name:        "PagerDuty (Default)",
		Type:        WebhookTemplatePagerDuty,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Headers: map[string]string{
			"Authorization": "Token token={{.Details.pagerduty_token}}",
		},
		Body: `{
  "routing_key": "{{.Details.routing_key}}",
  "event_action": "trigger",
  "dedup_key": "{{.RuleID}}-{{.TriggeredAt.Format "20060102"}}",
  "payload": {
    "summary": "{{.RuleName}}: {{.Condition}}",
    "severity": "{{if eq .Severity "critical"}}critical{{else if eq .Severity "warning"}}warning{{else}}info{{end}}",
    "source": "{{.Gateway}}",
    "component": "{{.RuleType}}",
    "custom_details": {
      "current_value": {{.Value}},
      "threshold": {{.Threshold}},
      "rule_id": "{{.RuleID}}",
      "node_id": "{{.NodeID}}"
    }
  }
}`,
		Enabled: true,
	})

	// Microsoft Teams template
	_ = e.RegisterTemplate(WebhookTemplate{ // #nosec G104
		ID:          "teams-default",
		Name:        "Microsoft Teams (Default)",
		Type:        WebhookTemplateTeams,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Body: `{
  "@type": "MessageCard",
  "@context": "https://schema.org/extensions",
  "themeColor": "FF0000",
  "summary": "API Cerberus Alert: {{.RuleName}}",
  "sections": [
    {
      "activityTitle": "🚨 {{.RuleName}}",
      "activitySubtitle": "{{.Gateway}} - {{.TriggeredAt.Format "2006-01-02 15:04:05 UTC"}}",
      "facts": [
        {
          "name": "Rule Type:",
          "value": "{{.RuleType}}"
        },
        {
          "name": "Severity:",
          "value": "{{.Severity}}"
        },
        {
          "name": "Current Value:",
          "value": "{{.Value}}"
        },
        {
          "name": "Threshold:",
          "value": "{{.Threshold}}"
        },
        {
          "name": "Condition:",
          "value": "{{.Condition}}"
        }
      ],
      "markdown": true
    }
  ],
  "potentialAction": [
    {
      "@type": "OpenUri",
      "name": "View Dashboard",
      "targets": [
        {
          "os": "default",
          "uri": "{{.DashboardURL}}"
        }
      ]
    }
  ]
}`,
		Enabled: true,
	})

	// Telegram template
	_ = e.RegisterTemplate(WebhookTemplate{ // #nosec G104
		ID:          "telegram-default",
		Name:        "Telegram (Default)",
		Type:        WebhookTemplateTelegram,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Body: `{
  "chat_id": "{{.Details.chat_id}}",
  "text": "🚨 *{{.RuleName}}*\n\n*Type:* {{.RuleType}}\n*Severity:* {{.Severity}}\n*Value:* {{.Value}}\n*Threshold:* {{.Threshold}}\n*Time:* {{.TriggeredAt.Format "2006-01-02 15:04:05 UTC"}}\n\n[View Dashboard]({{.DashboardURL}})",
  "parse_mode": "Markdown",
  "disable_web_page_preview": true
}`,
		Enabled: true,
	})

	// Generic JSON template
	_ = e.RegisterTemplate(WebhookTemplate{ // #nosec G104
		ID:          "generic-default",
		Name:        "Generic JSON (Default)",
		Type:        WebhookTemplateGeneric,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Body: `{
  "alert": {
    "id": "{{.RuleID}}",
    "name": "{{.RuleName}}",
    "type": "{{.RuleType}}",
    "severity": "{{.Severity}}",
    "status": "{{.Status}}",
    "value": {{.Value}},
    "threshold": {{.Threshold}},
    "unit": "{{.Unit}}",
    "condition": "{{.Condition}}",
    "timestamp": "{{.TriggeredAt.Format "2006-01-02T15:04:05Z"}}",
    "gateway": "{{.Gateway}}",
    "node_id": "{{.NodeID}}",
    "cluster": "{{.Cluster}}",
    "url": "{{.DashboardURL}}"
  }
}`,
		Enabled: true,
	})
}

// RegisterTemplate registers a custom template.
func (e *WebhookTemplateEngine) RegisterTemplate(tpl WebhookTemplate) error {
	if e == nil {
		return fmt.Errorf("template engine is nil")
	}

	// Validate template
	if tpl.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	if tpl.Body == "" {
		return fmt.Errorf("template body is required")
	}

	// Set defaults
	if tpl.Method == "" {
		tpl.Method = http.MethodPost
	}
	if tpl.ContentType == "" {
		tpl.ContentType = "application/json"
	}
	if tpl.Type == "" {
		tpl.Type = WebhookTemplateCustom
	}

	now := time.Now()
	if tpl.CreatedAt.IsZero() {
		tpl.CreatedAt = now
	}
	tpl.UpdatedAt = now

	// Compile template
	compiled, err := template.New(tpl.ID).Parse(tpl.Body)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	e.templates[tpl.ID] = tpl
	e.compiled[tpl.ID] = compiled

	return nil
}

// GetTemplate retrieves a template by ID.
func (e *WebhookTemplateEngine) GetTemplate(id string) (WebhookTemplate, bool) {
	if e == nil {
		return WebhookTemplate{}, false
	}

	tpl, ok := e.templates[id]
	return tpl, ok
}

// DeleteTemplate removes a template.
func (e *WebhookTemplateEngine) DeleteTemplate(id string) bool {
	if e == nil {
		return false
	}

	if _, ok := e.templates[id]; !ok {
		return false
	}

	delete(e.templates, id)
	delete(e.compiled, id)
	return true
}

// ListTemplates returns all registered templates.
func (e *WebhookTemplateEngine) ListTemplates() []WebhookTemplate {
	if e == nil {
		return nil
	}

	templates := make([]WebhookTemplate, 0, len(e.templates))
	for _, tpl := range e.templates {
		templates = append(templates, tpl)
	}
	return templates
}

// ListTemplatesByType returns templates of a specific type.
func (e *WebhookTemplateEngine) ListTemplatesByType(tplType WebhookTemplateType) []WebhookTemplate {
	if e == nil {
		return nil
	}

	var templates []WebhookTemplate
	for _, tpl := range e.templates {
		if tpl.Type == tplType {
			templates = append(templates, tpl)
		}
	}
	return templates
}

// Render renders a template with the given data.
func (e *WebhookTemplateEngine) Render(templateID string, data WebhookTemplateData) (string, error) {
	if e == nil {
		return "", fmt.Errorf("template engine is nil")
	}

	compiled, ok := e.compiled[templateID]
	if !ok {
		return "", fmt.Errorf("template not found: %s", templateID)
	}

	// Compute severity if not set
	if data.Severity == "" {
		data.Severity = e.computeSeverity(data.RuleType, data.Value, data.Threshold)
	}

	// Set status
	if data.Status == "" {
		data.Status = "firing"
	}

	var buf bytes.Buffer
	if err := compiled.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// RenderWithTemplate renders using a provided template string.
func (e *WebhookTemplateEngine) RenderWithTemplate(templateBody string, data WebhookTemplateData) (string, error) {
	if e == nil {
		return "", fmt.Errorf("template engine is nil")
	}

	// Compute severity if not set
	if data.Severity == "" {
		data.Severity = e.computeSeverity(data.RuleType, data.Value, data.Threshold)
	}

	if data.Status == "" {
		data.Status = "firing"
	}

	compiled, err := template.New("inline").Parse(templateBody)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := compiled.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// BuildRequest builds an HTTP request from a template.
func (e *WebhookTemplateEngine) BuildRequest(templateID string, webhookURL string, data WebhookTemplateData) (*http.Request, error) {
	if e == nil {
		return nil, fmt.Errorf("template engine is nil")
	}

	tpl, ok := e.templates[templateID]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}

	// Render body
	body, err := e.Render(templateID, data)
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequest(tpl.Method, webhookURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", tpl.ContentType)
	for key, value := range tpl.Headers {
		// Render header value as template
		headerValue, err := e.RenderWithTemplate(value, data)
		if err != nil {
			// If rendering fails, use raw value
			headerValue = value
		}
		req.Header.Set(key, headerValue)
	}

	return req, nil
}

// computeSeverity determines the severity based on rule type and values.
func (e *WebhookTemplateEngine) computeSeverity(ruleType string, value, threshold float64) string {
	ratio := value / threshold

	switch AlertRuleType(ruleType) {
	case AlertRuleErrorRate:
		if ratio >= 2.0 {
			return "critical"
		} else if ratio >= 1.5 {
			return "warning"
		}
		return "info"

	case AlertRuleP99Latency:
		if ratio >= 2.0 {
			return "critical"
		} else if ratio >= 1.3 {
			return "warning"
		}
		return "info"

	case AlertRuleUpstreamHealth:
		if value <= 0.5 {
			return "critical"
		} else if value <= 0.75 {
			return "warning"
		}
		return "info"

	default:
		if ratio >= 2.0 {
			return "critical"
		} else if ratio >= 1.5 {
			return "warning"
		}
		return "info"
	}
}

// ValidateTemplate validates a template body.
func (e *WebhookTemplateEngine) ValidateTemplate(body string) error {
	if e == nil {
		return fmt.Errorf("template engine is nil")
	}

	_, err := template.New("validate").Parse(body)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	return nil
}

// TestTemplate renders a template with sample data for testing.
func (e *WebhookTemplateEngine) TestTemplate(templateID string) (string, error) {
	if e == nil {
		return "", fmt.Errorf("template engine is nil")
	}

	sampleData := WebhookTemplateData{
		RuleID:      "rule-123",
		RuleName:    "High Error Rate",
		RuleType:    "error_rate",
		Description: "Error rate exceeds threshold",
		Value:       5.5,
		Threshold:   5.0,
		Unit:        "%",
		Condition:   "error_rate > 5%",
		Timestamp:   time.Now(),
		TriggeredAt: time.Now(),
		Gateway:     "apicerberus-prod-01",
		NodeID:      "node-1",
		Cluster:     "production",
		Details: map[string]any{
			"route":        "/api/users",
			"status_codes": "500,502,503",
		},
		URL:          "https://apicerberus.com/alerts/rule-123",
		DashboardURL: "https://apicerberus.com/dashboard",
	}

	return e.Render(templateID, sampleData)
}

// GetDefaultTemplateID returns the default template ID for a template type.
func GetDefaultTemplateID(tplType WebhookTemplateType) string {
	switch tplType {
	case WebhookTemplateSlack:
		return "slack-default"
	case WebhookTemplateDiscord:
		return "discord-default"
	case WebhookTemplatePagerDuty:
		return "pagerduty-default"
	case WebhookTemplateTeams:
		return "teams-default"
	case WebhookTemplateTelegram:
		return "telegram-default"
	case WebhookTemplateGeneric:
		return "generic-default"
	default:
		return "generic-default"
	}
}

// TemplateVariableDocs returns documentation for available template variables.
func TemplateVariableDocs() map[string]string {
	return map[string]string{
		"{{.RuleID}}":       "Unique identifier for the alert rule",
		"{{.RuleName}}":     "Human-readable name of the alert rule",
		"{{.RuleType}}":     "Type of alert (error_rate, p99_latency, upstream_health)",
		"{{.Description}}":  "Description of the alert rule",
		"{{.Value}}":        "Current metric value that triggered the alert",
		"{{.Threshold}}":    "Threshold value for the alert",
		"{{.Unit}}":         "Unit of measurement (%, ms, etc.)",
		"{{.Condition}}":    "Human-readable condition description",
		"{{.Timestamp}}":    "Timestamp when the alert was evaluated",
		"{{.TriggeredAt}}":  "Timestamp when the alert was triggered",
		"{{.Gateway}}":      "Gateway instance name",
		"{{.NodeID}}":       "Node identifier in cluster",
		"{{.Cluster}}":      "Cluster name",
		"{{.Details}}":      "Map of additional details (access with {{.Details.key}})",
		"{{.URL}}":          "Direct URL to alert details",
		"{{.DashboardURL}}": "URL to the dashboard",
		"{{.Severity}}":     "Computed severity (critical, warning, info)",
		"{{.Status}}":       "Alert status (firing, resolved)",
	}
}

// CreateCustomTemplate creates a custom template with validation.
func CreateCustomTemplate(id, name, body string, headers map[string]string) (WebhookTemplate, error) {
	// Validate template body
	_, err := template.New("validate").Parse(body)
	if err != nil {
		return WebhookTemplate{}, fmt.Errorf("invalid template body: %w", err)
	}

	// Validate JSON if content type is JSON
	var jsonTest map[string]any
	if err := json.Unmarshal([]byte(body), &jsonTest); err != nil {
		return WebhookTemplate{}, fmt.Errorf("template body is not valid JSON: %w", err)
	}

	return WebhookTemplate{
		ID:          id,
		Name:        name,
		Type:        WebhookTemplateCustom,
		ContentType: "application/json",
		Method:      http.MethodPost,
		Headers:     headers,
		Body:        body,
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}
