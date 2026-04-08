package plugin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGraphQLGuard_Priority(t *testing.T) {
	guard := NewGraphQLGuard(nil)
	if guard.Priority() != 2 {
		t.Errorf("Expected priority 2, got %d", guard.Priority())
	}
}

func TestGraphQLGuard_Name(t *testing.T) {
	guard := NewGraphQLGuard(nil)
	if guard.Name() != "graphql_guard" {
		t.Errorf("Expected name 'graphql_guard', got %s", guard.Name())
	}
}

func TestGraphQLGuard_Phase(t *testing.T) {
	guard := NewGraphQLGuard(nil)
	if guard.Phase() != PhasePreAuth {
		t.Errorf("Expected phase PhasePreAuth, got %v", guard.Phase())
	}
}

func TestGraphQLGuard_NilGuard(t *testing.T) {
	var guard *GraphQLGuard
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)

	// Should not panic and return false
	if guard.Handle(w, req) {
		t.Error("Expected nil guard to return false")
	}
}

func TestGraphQLGuard_NonGraphQLRequest(t *testing.T) {
	cfg := &GraphQLGuardConfig{
		BlockIntrospection: true,
	}
	guard := NewGraphQLGuard(cfg)

	// Regular HTTP request (not GraphQL)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)

	if guard.Handle(w, req) {
		t.Error("Expected non-GraphQL request to pass through")
	}
}

func TestGraphQLGuard_InvalidGraphQL(t *testing.T) {
	cfg := &GraphQLGuardConfig{
		BlockIntrospection: false,
		MaxDepth:           5,
		MaxComplexity:      100,
	}
	guard := NewGraphQLGuard(cfg)

	// Invalid GraphQL request body
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	// Should block invalid requests
	if !guard.Handle(w, req) {
		t.Error("Expected invalid GraphQL request to be blocked")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
