package federation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestEnforceFieldAuth_DeniesRestrictedField pins SEC-GQL-006: when a checker
// is attached to the request context, executeStep must refuse to issue the
// outbound subgraph call for a field the caller lacks the required role for.
func TestEnforceFieldAuth_DeniesRestrictedField(t *testing.T) {
	t.Parallel()

	var upstreamHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"user": map[string]any{"email": "leaked@example.com"}},
		})
	}))
	defer srv.Close()

	exec := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{{
			ID:         "s1",
			Subgraph:   &Subgraph{URL: srv.URL},
			Query:      "{ user(id: 1) { email } }",
			ResultType: "User",
		}},
		DependsOn: map[string][]string{},
	}

	checker := NewExecutionAuthChecker(
		map[string][]string{"User.email": {"admin"}},
		[]string{"viewer"}, // caller is NOT admin
	)
	ctx := WithAuthChecker(context.Background(), checker)

	res, err := exec.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if upstreamHits.Load() != 0 {
		t.Fatalf("subgraph was called despite @authorized denial (hits=%d); protected field must not reach the subgraph",
			upstreamHits.Load())
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected a structured ExecutionError for denied field, got none")
	}
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e.Message, "forbidden") && strings.Contains(e.Message, "User.email") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected forbidden error mentioning User.email, got %+v", res.Errors)
	}
}

// TestEnforceFieldAuth_AllowsWhenRoleMatches verifies we don't over-block:
// a caller with the required role must pass through and hit the subgraph.
func TestEnforceFieldAuth_AllowsWhenRoleMatches(t *testing.T) {
	t.Parallel()

	var upstreamHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"user": map[string]any{"email": "ok@example.com"}},
		})
	}))
	defer srv.Close()

	exec := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{{
			ID:         "s1",
			Subgraph:   &Subgraph{URL: srv.URL},
			Query:      "{ user(id: 1) { email } }",
			ResultType: "User",
		}},
		DependsOn: map[string][]string{},
	}

	checker := NewExecutionAuthChecker(
		map[string][]string{"User.email": {"admin"}},
		[]string{"admin"},
	)
	ctx := WithAuthChecker(context.Background(), checker)

	res, err := exec.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if upstreamHits.Load() != 1 {
		t.Fatalf("expected subgraph to be called exactly once for authorized caller, got hits=%d", upstreamHits.Load())
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected errors for authorized caller: %+v", res.Errors)
	}
}

// TestEnforceFieldAuth_NoCheckerInContextIsPermissive verifies the fallback
// path: if no checker is attached (e.g. the gateway hasn't wired auth yet),
// Execute preserves existing behavior and does not randomly deny traffic.
func TestEnforceFieldAuth_NoCheckerInContextIsPermissive(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"user": map[string]any{"email": "ok@example.com"}},
		})
	}))
	defer srv.Close()

	exec := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{{
			ID:         "s1",
			Subgraph:   &Subgraph{URL: srv.URL},
			Query:      "{ user(id: 1) { email } }",
			ResultType: "User",
		}},
		DependsOn: map[string][]string{},
	}

	res, err := exec.Execute(context.Background(), plan) // no checker
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("no checker should pass through, got errors: %+v", res.Errors)
	}
}

// TestEnforceFieldAuth_TypeLevelDeny verifies @authorized at the type level
// (User.* → admin) is enforced too, not just per-field.
func TestEnforceFieldAuth_TypeLevelDeny(t *testing.T) {
	t.Parallel()

	var upstreamHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
	}))
	defer srv.Close()

	exec := NewExecutorWith(WithExecutorURLValidation(false))
	plan := &Plan{
		Steps: []*PlanStep{{
			ID:         "s1",
			Subgraph:   &Subgraph{URL: srv.URL},
			Query:      "{ user(id: 1) { name } }",
			ResultType: "User",
		}},
		DependsOn: map[string][]string{},
	}

	checker := NewExecutionAuthChecker(
		map[string][]string{"User.*": {"admin"}},
		[]string{"viewer"},
	)
	ctx := WithAuthChecker(context.Background(), checker)

	res, _ := exec.Execute(ctx, plan)
	if upstreamHits.Load() != 0 {
		t.Fatalf("type-level @authorized denial must not reach the subgraph (hits=%d)", upstreamHits.Load())
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected forbidden error for type-level denial")
	}
}
