package federation

import (
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(3, 30*time.Second)
	if cb == nil {
		t.Fatal("expected non-nil circuit breaker")
	}
	if cb.threshold != 3 {
		t.Errorf("threshold = %d, want 3", cb.threshold)
	}
	if cb.state != CircuitClosed {
		t.Error("initial state should be Closed")
	}
}

func TestCircuitBreaker_Closed_Allows(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(3, 30*time.Second)
	if !cb.CanExecute() {
		t.Error("closed circuit should allow execution")
	}
}

func TestCircuitBreaker_Opens_AfterThreshold(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(3, 30*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.CanExecute() {
		t.Error("should still be closed after 2 failures (threshold=3)")
	}
	cb.RecordFailure()
	if cb.CanExecute() {
		t.Error("should be open after 3 failures")
	}
}

func TestCircuitBreaker_HalfOpen_AfterResetTimeout(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.CanExecute() {
		t.Error("should be open")
	}
	time.Sleep(60 * time.Millisecond)
	if !cb.CanExecute() {
		t.Error("should transition to half-open after reset timeout")
	}
}

func TestCircuitBreaker_Closes_OnSuccess(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(2, 50*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.CanExecute() {
		t.Error("should be open")
	}
	time.Sleep(60 * time.Millisecond)
	if !cb.CanExecute() {
		t.Error("should be half-open")
	}
	cb.RecordSuccess()
	if cb.state != CircuitClosed {
		t.Error("should be closed after success in half-open")
	}
	if cb.failures != 0 {
		t.Errorf("failures = %d, want 0 after success", cb.failures)
	}
}

func TestCircuitBreaker_RecordSuccess_ClosedState(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(3, 30*time.Second)
	cb.RecordFailure()
	cb.RecordSuccess()
	if cb.failures != 0 {
		t.Errorf("failures = %d, want 0", cb.failures)
	}
}

func TestNewExecutionAuthChecker(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(map[string][]string{"User.email": {"admin"}}, []string{"admin"})
	if ac == nil {
		t.Fatal("expected non-nil checker")
	}
}

func TestExecutionAuthChecker_Nil(t *testing.T) {
	t.Parallel()
	var ac *ExecutionAuthChecker
	allowed, required := ac.CheckFieldAuth("User", "name")
	if !allowed {
		t.Error("nil checker should allow all")
	}
	if required != nil {
		t.Error("nil checker should return nil required roles")
	}
}

func TestExecutionAuthChecker_NoAuthorizedFields(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(nil, []string{"user"})
	allowed, _ := ac.CheckFieldAuth("User", "name")
	if !allowed {
		t.Error("empty authorized fields should allow all")
	}
}

func TestExecutionAuthChecker_TypeLevelRestriction(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(
		map[string][]string{"User.*": {"admin"}},
		[]string{"user"},
	)
	allowed, required := ac.CheckFieldAuth("User", "name")
	if allowed {
		t.Error("user role should not access admin-only type")
	}
	if len(required) != 1 || required[0] != "admin" {
		t.Errorf("required = %v, want [admin]", required)
	}
}

func TestExecutionAuthChecker_TypeLevelAllowed(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(
		map[string][]string{"User.*": {"admin"}},
		[]string{"admin"},
	)
	allowed, _ := ac.CheckFieldAuth("User", "name")
	if !allowed {
		t.Error("admin should access admin-only type")
	}
}

func TestExecutionAuthChecker_FieldLevelRestriction(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(
		map[string][]string{"User.email": {"admin"}},
		[]string{"user"},
	)
	allowed, required := ac.CheckFieldAuth("User", "email")
	if allowed {
		t.Error("user role should not access admin-only field")
	}
	if len(required) != 1 || required[0] != "admin" {
		t.Errorf("required = %v, want [admin]", required)
	}
}

func TestExecutionAuthChecker_FieldAllowed(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(
		map[string][]string{"User.email": {"admin", "manager"}},
		[]string{"manager"},
	)
	allowed, _ := ac.CheckFieldAuth("User", "email")
	if !allowed {
		t.Error("manager should access field")
	}
}

func TestExecutionAuthChecker_NoRestrictionOnOtherFields(t *testing.T) {
	t.Parallel()
	ac := NewExecutionAuthChecker(
		map[string][]string{"User.email": {"admin"}},
		[]string{"user"},
	)
	allowed, _ := ac.CheckFieldAuth("User", "name")
	if !allowed {
		t.Error("unrestricted field should be allowed")
	}
}

func TestQueryCache_GetMiss(t *testing.T) {
	t.Parallel()
	qc := NewQueryCache(10)
	_, ok := qc.Get("nonexistent")
	if ok {
		t.Error("expected miss")
	}
}

func TestQueryCache_SetAndGet(t *testing.T) {
	t.Parallel()
	qc := NewQueryCache(10)
	plan := &Plan{Steps: []*PlanStep{{ID: "step-1"}}}
	qc.Set("query1", plan)
	got, ok := qc.Get("query1")
	if !ok {
		t.Fatal("expected hit")
	}
	if got.Steps[0].ID != "step-1" {
		t.Errorf("got step ID = %q, want step-1", got.Steps[0].ID)
	}
}

func TestQueryCache_Eviction(t *testing.T) {
	t.Parallel()
	qc := NewQueryCache(2)
	qc.Set("q1", &Plan{Steps: []*PlanStep{{ID: "1"}}})
	time.Sleep(1 * time.Millisecond)
	qc.Set("q2", &Plan{Steps: []*PlanStep{{ID: "2"}}})
	time.Sleep(1 * time.Millisecond)
	qc.Set("q3", &Plan{Steps: []*PlanStep{{ID: "3"}}})

	_, ok := qc.Get("q1")
	if ok {
		t.Error("q1 should have been evicted")
	}
	_, ok = qc.Get("q2")
	if !ok {
		t.Error("q2 should still exist")
	}
	_, ok = qc.Get("q3")
	if !ok {
		t.Error("q3 should exist")
	}
}

func TestQueryCache_HitCount(t *testing.T) {
	t.Parallel()
	qc := NewQueryCache(10)
	qc.Set("q1", &Plan{})
	qc.Get("q1")
	qc.Get("q1")
	entry := qc.entries["q1"]
	if entry.HitCount.Load() != 2 {
		t.Errorf("hit count = %d, want 2", entry.HitCount.Load())
	}
}
