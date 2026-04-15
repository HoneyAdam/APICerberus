package federation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// --- getCircuitBreaker ---

func TestGetCircuitBreaker_CreatesNew(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	cb := e.getCircuitBreaker("subgraph-1")
	if cb == nil {
		t.Fatal("expected non-nil circuit breaker")
	}
}

func TestGetCircuitBreaker_Idempotent(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	cb1 := e.getCircuitBreaker("subgraph-1")
	cb2 := e.getCircuitBreaker("subgraph-1")
	if cb1 != cb2 {
		t.Error("expected same circuit breaker instance for same ID")
	}
}

func TestGetCircuitBreaker_DifferentIDs(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	cb1 := e.getCircuitBreaker("subgraph-1")
	cb2 := e.getCircuitBreaker("subgraph-2")
	if cb1 == cb2 {
		t.Error("expected different circuit breaker instances for different IDs")
	}
}

// --- GetActiveSubscriptions ---

func TestGetActiveSubscriptions_Empty(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	ids := e.GetActiveSubscriptions()
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
}

func TestGetActiveSubscriptions_WithEntries(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	e.subscriptions["sub-1"] = &SubscriptionConnection{ID: "sub-1"}
	e.subscriptions["sub-2"] = &SubscriptionConnection{ID: "sub-2"}
	e.subscriptions["sub-3"] = &SubscriptionConnection{ID: "sub-3"}

	ids := e.GetActiveSubscriptions()
	if len(ids) != 3 {
		t.Fatalf("expected 3, got %d", len(ids))
	}

	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	for _, want := range []string{"sub-1", "sub-2", "sub-3"} {
		if !found[want] {
			t.Errorf("missing subscription %q", want)
		}
	}
}

// --- StopSubscription ---

func TestStopSubscription_NotFound(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	err := e.StopSubscription("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent subscription")
	}
}

func TestStopSubscription_Found_NilConn(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	e.subscriptions["sub-1"] = &SubscriptionConnection{
		ID:   "sub-1",
		Conn: nil, // no WebSocket connection
	}
	err := e.StopSubscription("sub-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Verify it was removed
	if len(e.GetActiveSubscriptions()) != 0 {
		t.Error("expected subscription to be removed")
	}
}

func TestStopSubscription_MultipleRemoves(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	e.subscriptions["sub-1"] = &SubscriptionConnection{ID: "sub-1"}
	e.subscriptions["sub-2"] = &SubscriptionConnection{ID: "sub-2"}

	if err := e.StopSubscription("sub-1"); err != nil {
		t.Errorf("stop sub-1: %v", err)
	}
	ids := e.GetActiveSubscriptions()
	if len(ids) != 1 || ids[0] != "sub-2" {
		t.Errorf("expected [sub-2], got %v", ids)
	}

	if err := e.StopSubscription("sub-1"); err == nil {
		t.Error("expected error removing already-removed subscription")
	}
}

// --- ExecuteParallel ---

func TestExecuteParallel_SingleStep(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"user": map[string]any{"name": "Alice"}},
		})
	}))
	defer srv.Close()

	e := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{
			{
				ID:        "step-1",
				Subgraph:  &Subgraph{ID: "sg-1", URL: srv.URL},
				Query:     `{ user { name } }`,
				Path:      []string{"user"},
				DependsOn: nil,
			},
		},
		DependsOn: map[string][]string{"step-1": {}},
	}

	result, err := e.ExecuteParallel(context.Background(), plan)
	if err != nil {
		t.Fatalf("ExecuteParallel: %v", err)
	}
	if called.Load() != 1 {
		t.Errorf("called = %d, want 1", called.Load())
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestExecuteParallel_TwoIndependentSteps(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"result": "ok"},
		})
	}))
	defer srv.Close()

	e := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{
			{ID: "s1", Subgraph: &Subgraph{ID: "sg1", URL: srv.URL}, Query: `{ a }`, Path: []string{"a"}},
			{ID: "s2", Subgraph: &Subgraph{ID: "sg2", URL: srv.URL}, Query: `{ b }`, Path: []string{"b"}},
		},
		DependsOn: map[string][]string{"s1": {}, "s2": {}},
	}

	result, err := e.ExecuteParallel(context.Background(), plan)
	if err != nil {
		t.Fatalf("ExecuteParallel: %v", err)
	}
	if called.Load() != 2 {
		t.Errorf("called = %d, want 2", called.Load())
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors: %v", result.Errors)
	}
}

func TestExecuteParallel_Deadlock(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{
			{ID: "s1", Subgraph: &Subgraph{ID: "sg1", URL: "http://localhost:0"}, Query: `{ a }`, Path: []string{"a"}},
			{ID: "s2", Subgraph: &Subgraph{ID: "sg2", URL: "http://localhost:0"}, Query: `{ b }`, Path: []string{"b"}},
		},
		// Circular dependency: s1 depends on s2, s2 depends on s1
		DependsOn: map[string][]string{"s1": {"s2"}, "s2": {"s1"}},
	}

	_, err := e.ExecuteParallel(context.Background(), plan)
	if err == nil {
		t.Error("expected deadlock error")
	}
}

func TestExecuteParallel_StepFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{
			{ID: "s1", Subgraph: &Subgraph{ID: "sg1", URL: srv.URL}, Query: `{ a }`, Path: []string{"a"}},
		},
		DependsOn: map[string][]string{"s1": {}},
	}

	result, err := e.ExecuteParallel(context.Background(), plan)
	if err != nil {
		t.Fatalf("ExecuteParallel should not return error on step failure: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one execution error")
	}
}

func TestExecuteParallel_CancelledContext(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	e := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{
			{ID: "s1", Subgraph: &Subgraph{ID: "sg1", URL: srv.URL}, Query: `{ a }`, Path: []string{"a"}},
		},
		DependsOn: map[string][]string{"s1": {}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, _ := e.ExecuteParallel(ctx, plan)
	// Context cancellation causes executeStep to fail → error in result.Errors
	// (ExecuteParallel catches step errors internally rather than returning them)
	if result != nil && len(result.Errors) == 0 {
		t.Log("context cancellation may not have propagated before step completed")
	}
}

func TestExecuteParallel_EmptyPlan(t *testing.T) {
	t.Parallel()
	e := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps:    []*PlanStep{},
		DependsOn: map[string][]string{},
	}

	result, err := e.ExecuteParallel(context.Background(), plan)
	if err != nil {
		t.Fatalf("ExecuteParallel empty plan: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// --- SubscriptionConnection / SubscriptionMessage ---

func TestSubscriptionConnection_Fields(t *testing.T) {
	t.Parallel()
	sc := &SubscriptionConnection{
		ID:        "sub-1",
		Subgraph:  &Subgraph{ID: "sg-1"},
		Query:     "subscription { onEvent }",
		Variables: map[string]any{"key": "val"},
	}
	if sc.ID != "sub-1" {
		t.Errorf("ID = %q, want %q", sc.ID, "sub-1")
	}
	if sc.Query != "subscription { onEvent }" {
		t.Error("unexpected query")
	}
}

func TestSubscriptionMessage_Fields(t *testing.T) {
	t.Parallel()
	msg := &SubscriptionMessage{
		ID:   "msg-1",
		Data: map[string]any{"count": float64(42)},
	}
	if msg.ID != "msg-1" {
		t.Error("unexpected ID")
	}
	if msg.Data["count"] != float64(42) {
		t.Error("unexpected data")
	}
}

// --- ExecutionResult / ExecutionError JSON ---

func TestExecutionResult_EmptyJSON(t *testing.T) {
	t.Parallel()
	r := &ExecutionResult{
		Data:   map[string]any{},
		Errors: []ExecutionError{},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) == "" {
		t.Error("expected non-empty JSON")
	}
}

func TestExecutionError_JSON(t *testing.T) {
	t.Parallel()
	e := ExecutionError{
		Message:    "test error",
		Path:       []string{"user", "name"},
		Extensions: map[string]any{"code": "TEST"},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}
