package analytics

import (
	"strings"
	"testing"
	"time"
)

func TestWebhookTemplateEngine_RenderWithTemplate(t *testing.T) {
	engine := NewWebhookTemplateEngine()

	tests := []struct {
		name         string
		templateBody string
		data         WebhookTemplateData
		wantErr      bool
		contains     string
	}{
		{
			name:         "simple template with rule name",
			templateBody: "Alert: {{.RuleName}}",
			data:         WebhookTemplateData{RuleName: "High Latency Alert"},
			wantErr:      false,
			contains:     "Alert: High Latency Alert",
		},
		{
			name:         "template with values",
			templateBody: "Value {{.Value}} exceeds threshold {{.Threshold}}",
			data: WebhookTemplateData{
				Value:     150.5,
				Threshold: 100.0,
			},
			wantErr:  false,
			contains: "Value 150.5 exceeds threshold 100",
		},
		{
			name:         "template with gateway info",
			templateBody: "Gateway: {{.Gateway}}, Node: {{.NodeID}}",
			data: WebhookTemplateData{
				Gateway: "prod-gateway-1",
				NodeID:  "node-123",
			},
			wantErr:  false,
			contains: "Gateway: prod-gateway-1, Node: node-123",
		},
		{
			name:         "template with timestamp",
			templateBody: "Triggered at: {{.TriggeredAt.Format \"2006-01-02\"}}",
			data: WebhookTemplateData{
				TriggeredAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			},
			wantErr:  false,
			contains: "Triggered at: 2026-01-15",
		},
		{
			name:         "invalid template syntax",
			templateBody: "{{.Unclosed",
			data:         WebhookTemplateData{},
			wantErr:      true,
		},
		{
			name:         "template with condition",
			templateBody: "Condition: {{.Condition}}, Unit: {{.Unit}}",
			data: WebhookTemplateData{
				Condition: "greater_than",
				Unit:      "ms",
			},
			wantErr:  false,
			contains: "Condition: greater_than, Unit: ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.RenderWithTemplate(tt.templateBody, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderWithTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !strings.Contains(result, tt.contains) {
				t.Errorf("RenderWithTemplate() result = %q, should contain %q", result, tt.contains)
			}
		})
	}
}
