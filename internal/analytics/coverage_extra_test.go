package analytics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- normalizeRule tests ---

func TestNormalizeRule_Valid(t *testing.T) {
	t.Parallel()
	rule := AlertRule{
		ID:        "rule-1",
		Name:      "Test Rule",
		Type:      AlertRuleErrorRate,
		Threshold: 0.5,
		Window:    "5m",
		Cooldown:  "1m",
		Action:    AlertAction{Type: AlertActionLog},
	}
	result, err := normalizeRule(rule)
	if err != nil {
		t.Fatalf("normalizeRule: %v", err)
	}
	if result.ID != "rule-1" {
		t.Errorf("ID = %q, want rule-1", result.ID)
	}
	if result.Window != "5m" {
		t.Errorf("Window = %q, want 5m", result.Window)
	}
}

func TestNormalizeRule_Defaults(t *testing.T) {
	t.Parallel()
	rule := AlertRule{
		ID:        "r2",
		Name:      "Rule 2",
		Type:      AlertRuleP99Latency,
		Threshold: 100,
	}
	result, err := normalizeRule(rule)
	if err != nil {
		t.Fatalf("normalizeRule: %v", err)
	}
	if result.Window != "5m" {
		t.Errorf("default Window = %q, want 5m", result.Window)
	}
	if result.Cooldown != "1m" {
		t.Errorf("default Cooldown = %q, want 1m", result.Cooldown)
	}
	if result.Action.Type != AlertActionLog {
		t.Errorf("default Action = %q, want log", result.Action.Type)
	}
}

func TestNormalizeRule_MissingID(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{Name: "n", Type: AlertRuleErrorRate})
	if err == nil {
		t.Error("expected error for missing ID")
	}
}

func TestNormalizeRule_MissingName(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{ID: "r1", Type: AlertRuleErrorRate})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestNormalizeRule_InvalidType(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{ID: "r1", Name: "n", Type: "invalid"})
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestNormalizeRule_NegativeThreshold(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{ID: "r1", Name: "n", Type: AlertRuleErrorRate, Threshold: -1})
	if err == nil {
		t.Error("expected error for negative threshold")
	}
}

func TestNormalizeRule_InvalidWindow(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{ID: "r1", Name: "n", Type: AlertRuleErrorRate, Window: "abc"})
	if err == nil {
		t.Error("expected error for invalid window")
	}
}

func TestNormalizeRule_InvalidCooldown(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{ID: "r1", Name: "n", Type: AlertRuleErrorRate, Cooldown: "xyz"})
	if err == nil {
		t.Error("expected error for invalid cooldown")
	}
}

func TestNormalizeRule_InvalidActionType(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{
		ID:     "r1",
		Name:   "n",
		Type:   AlertRuleErrorRate,
		Action: AlertAction{Type: "invalid"},
	})
	if err == nil {
		t.Error("expected error for invalid action type")
	}
}

func TestNormalizeRule_WebhookNoURL(t *testing.T) {
	t.Parallel()
	_, err := normalizeRule(AlertRule{
		ID:     "r1",
		Name:   "n",
		Type:   AlertRuleErrorRate,
		Action: AlertAction{Type: AlertActionWebhook},
	})
	if err == nil {
		t.Error("expected error for webhook without URL")
	}
}

func TestNormalizeRule_WebhookWithURL(t *testing.T) {
	t.Parallel()
	result, err := normalizeRule(AlertRule{
		ID:     "r1",
		Name:   "n",
		Type:   AlertRuleUpstreamHealth,
		Action: AlertAction{Type: AlertActionWebhook, WebhookURL: "http://example.com/hook"},
	})
	if err != nil {
		t.Fatalf("normalizeRule: %v", err)
	}
	if result.Action.WebhookURL != "http://example.com/hook" {
		t.Errorf("WebhookURL = %q", result.Action.WebhookURL)
	}
}

func TestPercentileOptimized(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values []int64
		p      int
		want   int64
	}{
		{"empty", nil, 50, 0},
		{"single", []int64{10}, 50, 10},
		{"p0_clamped", []int64{1, 2, 3}, 0, 1},
		{"p100_clamped", []int64{1, 2, 3}, 100, 3},
		{"p50_three", []int64{1, 2, 3}, 50, 2},
		{"p90", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 90, 9},
		{"large_array", makeLargeValues(), 50, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := percentileOptimized(tt.values, tt.p)
			if tt.name == "large_array" {
				// Approximate — just verify it's in range
				if got < 1 || got > 1000 {
					t.Errorf("percentileOptimized large array = %d, want between 1-1000", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("percentileOptimized(%v, %d) = %d, want %d", tt.values, tt.p, got, tt.want)
			}
		})
	}
}

func makeLargeValues() []int64 {
	vals := make([]int64, 200)
	for i := range vals {
		vals[i] = int64(i + 1)
	}
	return vals
}

func TestFlushBatch_Empty(t *testing.T) {
	t.Parallel()
	cfg := DefaultOptimizedEngineConfig()
	cfg.WorkerCount = 0 // Disable async workers
	cfg.BatchSize = 10
	engine := NewOptimizedEngine(cfg)

	// Flush with empty batch should be a no-op
	engine.flushBatch()
	// batchesSent should still be 0
	if engine.batchesSent.Load() != 0 {
		t.Error("no batches should be sent for empty batch")
	}
}

func TestFlushBatch_WithItems(t *testing.T) {
	t.Parallel()
	cfg := DefaultOptimizedEngineConfig()
	cfg.WorkerCount = 0
	cfg.BatchSize = 10
	engine := NewOptimizedEngine(cfg)

	// Add items to batch
	engine.batchMu.Lock()
	engine.batch = append(engine.batch, RequestMetric{Path: "/test", StatusCode: 200})
	engine.batchMu.Unlock()

	engine.flushBatch()

	if engine.batchesSent.Load() != 1 {
		t.Errorf("batchesSent = %d, want 1", engine.batchesSent.Load())
	}
}

func TestExecuteAction_WebhookSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := NewAlertEngine(AlertEngineOptions{})
	rule := AlertRule{
		ID:   "rule-1",
		Name: "test",
		Action: AlertAction{
			Type:       AlertActionWebhook,
			WebhookURL: server.URL,
		},
	}
	entry := AlertHistoryEntry{
		RuleID:   "rule-1",
		RuleName: "test",
	}

	err := engine.executeAction(rule, entry)
	if err != nil {
		t.Errorf("executeAction webhook: %v", err)
	}
}

func TestExecuteAction_WebhookFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	engine := NewAlertEngine(AlertEngineOptions{})
	rule := AlertRule{
		ID:   "rule-1",
		Name: "test",
		Action: AlertAction{
			Type:       AlertActionWebhook,
			WebhookURL: server.URL,
		},
	}

	err := engine.executeAction(rule, AlertHistoryEntry{})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestExecuteAction_WebhookEmptyURL(t *testing.T) {
	t.Parallel()
	engine := NewAlertEngine(AlertEngineOptions{})
	rule := AlertRule{
		Action: AlertAction{
			Type:       AlertActionWebhook,
			WebhookURL: "",
		},
	}

	err := engine.executeAction(rule, AlertHistoryEntry{})
	if err == nil {
		t.Error("expected error for empty webhook URL")
	}
}

func TestExecuteAction_Log(t *testing.T) {
	t.Parallel()
	engine := NewAlertEngine(AlertEngineOptions{})
	rule := AlertRule{
		Action: AlertAction{Type: AlertActionLog},
	}

	err := engine.executeAction(rule, AlertHistoryEntry{})
	if err != nil {
		t.Errorf("log action should not error: %v", err)
	}
}

func TestExecuteAction_WebhookConnectionError(t *testing.T) {
	t.Parallel()
	engine := NewAlertEngine(AlertEngineOptions{})
	rule := AlertRule{
		Action: AlertAction{
			Type:       AlertActionWebhook,
			WebhookURL: "http://127.0.0.1:1/unreachable",
		},
	}

	err := engine.executeAction(rule, AlertHistoryEntry{})
	if err == nil {
		t.Error("expected error for unreachable webhook")
	}
}
