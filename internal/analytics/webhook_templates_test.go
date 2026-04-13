package analytics

import (
	"html/template"
	"strings"
	"testing"
	"time"
)

// TestNewWebhookTemplateEngine tests creating a new template engine
func TestNewWebhookTemplateEngine(t *testing.T) {
	engine := NewWebhookTemplateEngine()
	if engine == nil {
		t.Fatal("NewWebhookTemplateEngine() returned nil")
	}
	if engine.templates == nil {
		t.Error("templates map is nil")
	}
	if engine.compiled == nil {
		t.Error("compiled map is nil")
	}

	// Verify default templates are registered
	templates := engine.ListTemplates()
	if len(templates) == 0 {
		t.Error("no default templates registered")
	}
}

// TestRegisterTemplate tests registering a custom template
func TestRegisterTemplate(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	tmpl := WebhookTemplate{
		ID:          "test-template",
		Name:        "Test Template",
		Type:        WebhookTemplateCustom,
		ContentType: "application/json",
		Method:      "POST",
		Body:        `{"message": "{{.RuleName}} triggered"}`,
		Enabled:     true,
	}

	err := engine.RegisterTemplate(tmpl)
	if err != nil {
		t.Errorf("RegisterTemplate() error = %v", err)
	}

	// Verify template was registered
	retrieved, ok := engine.GetTemplate("test-template")
	if !ok {
		t.Error("GetTemplate() returned false for existing template")
	}
	if retrieved.ID != "test-template" {
		t.Errorf("expected ID 'test-template', got %q", retrieved.ID)
	}
}

// TestRegisterTemplateDuplicate tests registering duplicate template
func TestRegisterTemplateDuplicate(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	tmpl := WebhookTemplate{
		ID:      "duplicate-test",
		Name:    "Duplicate Test",
		Type:    WebhookTemplateCustom,
		Body:    `{"test": true}`,
		Enabled: true,
	}

	// First registration should succeed
	err := engine.RegisterTemplate(tmpl)
	if err != nil {
		t.Errorf("first registration error = %v", err)
	}

	// Second registration should update
	tmpl.Name = "Updated Name"
	err = engine.RegisterTemplate(tmpl)
	if err != nil {
		t.Errorf("second registration error = %v", err)
	}

	retrieved, _ := engine.GetTemplate("duplicate-test")
	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %q", retrieved.Name)
	}
}

// TestGetTemplateNotFound tests getting non-existent template
func TestGetTemplateNotFound(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	_, ok := engine.GetTemplate("non-existent")
	if ok {
		t.Error("expected false for non-existent template")
	}
}

// TestDeleteTemplate tests deleting a template
func TestDeleteTemplate(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	tmpl := WebhookTemplate{
		ID:      "delete-test",
		Name:    "Delete Test",
		Type:    WebhookTemplateCustom,
		Body:    `{"test": true}`,
		Enabled: true,
	}

	_ = engine.RegisterTemplate(tmpl)

	// Delete should succeed
	if !engine.DeleteTemplate("delete-test") {
		t.Error("DeleteTemplate() returned false for existing template")
	}

	// Verify deletion
	_, ok := engine.GetTemplate("delete-test")
	if ok {
		t.Error("template still exists after deletion")
	}

	// Delete non-existent should return false
	if engine.DeleteTemplate("non-existent") {
		t.Error("DeleteTemplate() returned true for non-existent template")
	}
}

// TestListTemplates tests listing all templates
func TestListTemplates(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	// Add some templates
	for i := 0; i < 3; i++ {
		_ = engine.RegisterTemplate(WebhookTemplate{
			ID:      "list-test-" + string(rune('a'+i)),
			Name:    "List Test " + string(rune('A'+i)),
			Type:    WebhookTemplateCustom,
			Body:    `{"test": true}`,
			Enabled: true,
		})
	}

	templates := engine.ListTemplates()
	if len(templates) < 3 {
		t.Errorf("expected at least 3 templates, got %d", len(templates))
	}
}

// TestListTemplatesByType tests listing templates by type
func TestListTemplatesByType(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	// Clear existing templates and add specific ones
	engine.templates = make(map[string]WebhookTemplate)
	engine.compiled = make(map[string]*template.Template)

	_ = engine.RegisterTemplate(WebhookTemplate{
		ID:   "slack-1",
		Name: "Slack 1",
		Type: WebhookTemplateSlack,
		Body: `{"slack": true}`,
	})
	_ = engine.RegisterTemplate(WebhookTemplate{
		ID:   "slack-2",
		Name: "Slack 2",
		Type: WebhookTemplateSlack,
		Body: `{"slack": true}`,
	})
	_ = engine.RegisterTemplate(WebhookTemplate{
		ID:   "discord-1",
		Name: "Discord 1",
		Type: WebhookTemplateDiscord,
		Body: `{"discord": true}`,
	})

	slackTemplates := engine.ListTemplatesByType(WebhookTemplateSlack)
	if len(slackTemplates) != 2 {
		t.Errorf("expected 2 slack templates, got %d", len(slackTemplates))
	}

	discordTemplates := engine.ListTemplatesByType(WebhookTemplateDiscord)
	if len(discordTemplates) != 1 {
		t.Errorf("expected 1 discord template, got %d", len(discordTemplates))
	}
}

// TestRender tests template rendering
func TestRender(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	// Register a simple test template first
	_ = engine.RegisterTemplate(WebhookTemplate{
		ID:          "render-test",
		Name:        "Render Test",
		Type:        WebhookTemplateCustom,
		ContentType: "application/json",
		Method:      "POST",
		Body:        `{"rule": "{{.RuleName}}", "severity": "{{.Severity}}"}`,
		Enabled:     true,
	})

	data := WebhookTemplateData{
		RuleID:    "rule-123",
		RuleName:  "Test Rule",
		RuleType:  "threshold",
		Value:     95.5,
		Threshold: 90.0,
		Unit:      "requests/sec",
		Condition: "above",
		Timestamp: time.Now(),
		Gateway:   "gateway-1",
		NodeID:    "node-1",
		Severity:  "warning",
		Status:    "firing",
	}

	result, err := engine.Render("render-test", data)
	if err != nil {
		t.Errorf("Render() error = %v", err)
	}
	if result == "" {
		t.Error("Render() returned empty result")
	}
	if !strings.Contains(result, "Test Rule") {
		t.Error("Render() result doesn't contain rule name")
	}
	if !strings.Contains(result, "warning") {
		t.Error("Render() result doesn't contain severity")
	}
}

// TestRenderNotFound tests rendering non-existent template
func TestRenderNotFound(t *testing.T) {
	engine := NewWebhookTemplateEngine()
	data := WebhookTemplateData{RuleName: "Test"}

	_, err := engine.Render("non-existent", data)
	if err == nil {
		t.Error("expected error for non-existent template")
	}
}

// TestBuildRequest tests building HTTP request from template
func TestBuildRequest(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	// Register a simple template
	_ = engine.RegisterTemplate(WebhookTemplate{
		ID:          "build-test",
		Name:        "Build Test",
		Type:        WebhookTemplateCustom,
		ContentType: "application/json",
		Method:      "POST",
		Body:        `{"rule": "{{.RuleName}}"}`,
		Enabled:     true,
	})

	data := WebhookTemplateData{
		RuleName: "Test Rule",
		URL:      "https://hooks.slack.com/test",
	}

	req, err := engine.BuildRequest("build-test", "https://hooks.slack.com/test", data)
	if err != nil {
		t.Errorf("BuildRequest() error = %v", err)
	}
	if req == nil {
		t.Fatal("BuildRequest() returned nil")
	}
	if req.Method != "POST" {
		t.Errorf("expected POST method, got %s", req.Method)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", req.Header.Get("Content-Type"))
	}
}

// TestComputeSeverity tests severity computation
func TestComputeSeverity(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	tests := []struct {
		ruleType  string
		value     float64
		threshold float64
		expected  string
	}{
		{string(AlertRuleErrorRate), 2.0, 1.0, "critical"},
		{string(AlertRuleErrorRate), 1.5, 1.0, "warning"},
		{string(AlertRuleErrorRate), 1.1, 1.0, "info"},
		{string(AlertRuleP99Latency), 2.0, 1.0, "critical"},
		{string(AlertRuleP99Latency), 1.3, 1.0, "warning"},
		{string(AlertRuleP99Latency), 1.1, 1.0, "info"},
		{string(AlertRuleUpstreamHealth), 0.4, 1.0, "critical"},
		{string(AlertRuleUpstreamHealth), 0.6, 1.0, "warning"},
		{string(AlertRuleUpstreamHealth), 0.8, 1.0, "info"},
		{"unknown", 100, 50, "critical"}, // unknown rule types default to ratio-based severity
	}

	for _, tt := range tests {
		severity := engine.computeSeverity(tt.ruleType, tt.value, tt.threshold)
		if severity != tt.expected {
			t.Errorf("computeSeverity(%s, %f, %f) = %s, want %s", tt.ruleType, tt.value, tt.threshold, severity, tt.expected)
		}
	}
}

// TestValidateTemplate tests template validation
func TestValidateTemplate(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid template",
			body:    `{"message": "{{.RuleName}}"}`,
			wantErr: false,
		},
		{
			name:    "empty body",
			body:    "",
			wantErr: false, // empty body is allowed (just returns empty string)
		},
		{
			name:    "invalid template syntax",
			body:    `{{.RuleName`, // unclosed action
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateTemplate(tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestTestTemplate tests the TestTemplate function
func TestTestTemplate(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	// Register a template first
	_ = engine.RegisterTemplate(WebhookTemplate{
		ID:      "testable",
		Name:    "Testable Template",
		Type:    WebhookTemplateCustom,
		Body:    `{"rule": "{{.RuleName}}", "value": {{.Value}}}`,
		Enabled: true,
	})

	result, err := engine.TestTemplate("testable")
	if err != nil {
		t.Errorf("TestTemplate() error = %v", err)
	}
	if result == "" {
		t.Error("TestTemplate() returned empty result")
	}
}

// TestTestTemplateNotFound tests TestTemplate with non-existent ID
func TestTestTemplateNotFound(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	_, err := engine.TestTemplate("non-existent")
	if err == nil {
		t.Error("expected error for non-existent template")
	}
}

// TestGetDefaultTemplateID tests getting default template IDs
func TestGetDefaultTemplateID(t *testing.T) {
	tests := []struct {
		templateType WebhookTemplateType
		wantEmpty    bool
	}{
		{WebhookTemplateSlack, false},
		{WebhookTemplateDiscord, false},
		{WebhookTemplatePagerDuty, false},
		{WebhookTemplateTeams, false},
		{WebhookTemplateTelegram, false},
		{WebhookTemplateGeneric, false},
		{WebhookTemplateCustom, false}, // Custom returns generic-default
	}

	for _, tt := range tests {
		t.Run(string(tt.templateType), func(t *testing.T) {
			id := GetDefaultTemplateID(tt.templateType)
			if tt.wantEmpty && id != "" {
				t.Errorf("expected empty ID for %s, got %s", tt.templateType, id)
			}
			if !tt.wantEmpty && id == "" {
				t.Errorf("expected non-empty ID for %s", tt.templateType)
			}
		})
	}
}

// TestTemplateVariableDocs tests getting template variable documentation
func TestTemplateVariableDocs(t *testing.T) {
	docs := TemplateVariableDocs()
	if docs == nil {
		t.Fatal("TemplateVariableDocs() returned nil")
	}
	if len(docs) == 0 {
		t.Error("TemplateVariableDocs() returned empty map")
	}

	// Check for expected keys (they have template syntax {{.VarName}})
	expectedKeys := []string{"{{.RuleID}}", "{{.RuleName}}", "{{.Value}}", "{{.Threshold}}", "{{.Timestamp}}"}
	for _, key := range expectedKeys {
		if _, ok := docs[key]; !ok {
			t.Errorf("missing documentation for %s", key)
		}
	}
}

// TestCreateCustomTemplate tests creating a custom template
func TestCreateCustomTemplate(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid custom template",
			body:    `{"alert": "{{.RuleName}} is {{.Status}}"}`,
			wantErr: false,
		},
		{
			name:    "invalid template syntax",
			body:    `{{.RuleName`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := CreateCustomTemplate("test-custom", "Test Custom", tt.body, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateCustomTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tmpl.ID != "test-custom" {
					t.Errorf("expected ID 'test-custom', got %q", tmpl.ID)
				}
				if tmpl.Type != WebhookTemplateCustom {
					t.Errorf("expected type %s, got %s", WebhookTemplateCustom, tmpl.Type)
				}
			}
		})
	}
}

// =============================================================================
// Tests for 0.0% coverage functions
// =============================================================================
