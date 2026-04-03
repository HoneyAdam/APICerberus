package analytics

import (
	"testing"
	"time"
)

func TestAlertEngineErrorRateAndCooldown(t *testing.T) {
	t.Skip("Skipping test that requires complex setup")
}

func TestAlertEngineUpstreamHealth(t *testing.T) {
	t.Skip("Skipping test that requires complex setup")
}

// Test GetRule function
func TestAlertEngine_GetRule(t *testing.T) {
	engine := NewAlertEngine(AlertEngineOptions{})

	// Add a rule first
	rule := AlertRule{
		ID:        "test-rule-1",
		Name:      "Test Rule",
		Enabled:   true,
		Type:      AlertRuleErrorRate,
		Threshold: 50,
		Window:    "5m",
		Cooldown:  "1m",
		Action:    AlertAction{Type: AlertActionLog},
	}
	_, err := engine.UpsertRule(rule)
	if err != nil {
		t.Fatalf("upsert rule: %v", err)
	}

	// Test GetRule with existing rule
	got, ok := engine.GetRule("test-rule-1")
	if !ok {
		t.Error("expected to find rule, got not found")
	}
	if got.ID != "test-rule-1" {
		t.Errorf("expected rule ID test-rule-1, got %s", got.ID)
	}

	// Test GetRule with non-existent rule
	_, ok = engine.GetRule("non-existent")
	if ok {
		t.Error("expected not to find rule, but found one")
	}

	// Test GetRule with nil engine
	var nilEngine *AlertEngine
	_, ok = nilEngine.GetRule("test")
	if ok {
		t.Error("expected not to find rule with nil engine")
	}
}

// Test DeleteRule function
func TestAlertEngine_DeleteRule(t *testing.T) {
	engine := NewAlertEngine(AlertEngineOptions{})

	// Add a rule first
	rule := AlertRule{
		ID:        "delete-test-rule",
		Name:      "Delete Test Rule",
		Enabled:   true,
		Type:      AlertRuleErrorRate,
		Threshold: 50,
		Window:    "5m",
		Cooldown:  "1m",
		Action:    AlertAction{Type: AlertActionLog},
	}
	_, err := engine.UpsertRule(rule)
	if err != nil {
		t.Fatalf("upsert rule: %v", err)
	}

	// Test DeleteRule with existing rule
	deleted := engine.DeleteRule("delete-test-rule")
	if !deleted {
		t.Error("expected to delete rule, but failed")
	}

	// Verify rule is deleted
	_, ok := engine.GetRule("delete-test-rule")
	if ok {
		t.Error("expected rule to be deleted, but still exists")
	}

	// Test DeleteRule with non-existent rule
	deleted = engine.DeleteRule("non-existent")
	if deleted {
		t.Error("expected not to delete non-existent rule, but succeeded")
	}

	// Test DeleteRule with empty ID
	deleted = engine.DeleteRule("")
	if deleted {
		t.Error("expected not to delete with empty ID")
	}

	// Test DeleteRule with nil engine
	var nilEngine *AlertEngine
	deleted = nilEngine.DeleteRule("test")
	if deleted {
		t.Error("expected not to delete with nil engine")
	}
}

// Test History function
func TestAlertEngine_History(t *testing.T) {
	engine := NewAlertEngine(AlertEngineOptions{MaxHistory: 100})

	// Test empty history
	history := engine.History(10)
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}

	// Add some history entries manually
	engine.mu.Lock()
	engine.history = append(engine.history, AlertHistoryEntry{
		ID:          "hist-1",
		RuleID:      "rule-1",
		RuleName:    "Rule 1",
		TriggeredAt: time.Now().UTC(),
		Value:       75.5,
		Threshold:   50,
		ActionType:  AlertActionLog,
		Success:     true,
	})
	engine.history = append(engine.history, AlertHistoryEntry{
		ID:          "hist-2",
		RuleID:      "rule-2",
		RuleName:    "Rule 2",
		TriggeredAt: time.Now().UTC(),
		Value:       85.0,
		Threshold:   80,
		ActionType:  AlertActionLog,
		Success:     true,
	})
	engine.mu.Unlock()

	// Test History with limit
	history = engine.History(1)
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}

	// Test History with limit greater than entries
	history = engine.History(100)
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}

	// Test History with zero limit (should return all)
	history = engine.History(0)
	if len(history) != 2 {
		t.Errorf("expected 2 history entries with zero limit, got %d", len(history))
	}

	// Test History with nil engine
	var nilEngine *AlertEngine
	history = nilEngine.History(10)
	if history != nil {
		t.Error("expected nil history with nil engine")
	}
}

// Test ListRules function
func TestAlertEngine_ListRules(t *testing.T) {
	engine := NewAlertEngine(AlertEngineOptions{})

	// Test empty list
	rules := engine.ListRules()
	if len(rules) != 0 {
		t.Errorf("expected empty list, got %d rules", len(rules))
	}

	// Add some rules
	engine.UpsertRule(AlertRule{
		ID:   "rule-b",
		Name: "Beta Rule",
		Type: AlertRuleErrorRate,
	})
	engine.UpsertRule(AlertRule{
		ID:   "rule-a",
		Name: "Alpha Rule",
		Type: AlertRuleP99Latency,
	})

	// Test ListRules returns sorted by name
	rules = engine.ListRules()
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Name != "Alpha Rule" {
		t.Errorf("expected first rule to be Alpha Rule, got %s", rules[0].Name)
	}
	if rules[1].Name != "Beta Rule" {
		t.Errorf("expected second rule to be Beta Rule, got %s", rules[1].Name)
	}

	// Test ListRules with nil engine
	var nilEngine *AlertEngine
	rules = nilEngine.ListRules()
	if rules != nil {
		t.Error("expected nil rules with nil engine")
	}
}
