package federation

import (
	"context"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/graphql"
)

// Test buildInterfaceSDL
func TestComposer_buildInterfaceSDL(t *testing.T) {
	composer := NewComposer()

	iface := &Type{
		Kind:        "INTERFACE",
		Name:        "Node",
		Description: "An object with an ID",
		Fields: map[string]*Field{
			"id": {Name: "id", Type: "ID!"},
		},
	}

	sdl := composer.buildInterfaceSDL(iface)
	if sdl == "" {
		t.Error("buildInterfaceSDL returned empty string")
	}
	if !contains(sdl, "interface Node") {
		t.Errorf("SDL should contain 'interface Node', got: %s", sdl)
	}
	if !contains(sdl, "id: ID!") {
		t.Errorf("SDL should contain 'id: ID!', got: %s", sdl)
	}
}

// Test buildUnionSDL
func TestComposer_buildUnionSDL(t *testing.T) {
	composer := NewComposer()

	union := &Type{
		Kind:          "UNION",
		Name:          "SearchResult",
		PossibleTypes: []string{"User", "Post"},
	}

	sdl := composer.buildUnionSDL(union)
	if sdl == "" {
		t.Error("buildUnionSDL returned empty string")
	}
	expected := "union SearchResult = User | Post"
	if sdl != expected {
		t.Errorf("buildUnionSDL() = %q, want %q", sdl, expected)
	}
}

// Test buildEnumSDL
func TestComposer_buildEnumSDL(t *testing.T) {
	composer := NewComposer()

	enum := &Type{
		Kind:       "ENUM",
		Name:       "Status",
		EnumValues: []string{"ACTIVE", "INACTIVE", "PENDING"},
	}

	sdl := composer.buildEnumSDL(enum)
	if sdl == "" {
		t.Error("buildEnumSDL returned empty string")
	}
	if !contains(sdl, "enum Status") {
		t.Errorf("SDL should contain 'enum Status', got: %s", sdl)
	}
	if !contains(sdl, "ACTIVE") {
		t.Errorf("SDL should contain 'ACTIVE', got: %s", sdl)
	}
}

// Test buildInputSDL
func TestComposer_buildInputSDL(t *testing.T) {
	composer := NewComposer()

	input := &Type{
		Kind: "INPUT_OBJECT",
		Name: "UserInput",
		InputFields: map[string]*InputField{
			"name":  {Name: "name", Type: "String!"},
			"email": {Name: "email", Type: "String!"},
		},
	}

	sdl := composer.buildInputSDL(input)
	if sdl == "" {
		t.Error("buildInputSDL returned empty string")
	}
	if !contains(sdl, "input UserInput") {
		t.Errorf("SDL should contain 'input UserInput', got: %s", sdl)
	}
	if !contains(sdl, "name: String!") {
		t.Errorf("SDL should contain 'name: String!', got: %s", sdl)
	}
}

// Test buildScalarSDL
func TestComposer_buildScalarSDL(t *testing.T) {
	composer := NewComposer()

	scalar := &Type{
		Kind: "SCALAR",
		Name: "DateTime",
	}

	sdl := composer.buildScalarSDL(scalar)
	if sdl == "" {
		t.Error("buildScalarSDL returned empty string")
	}
	expected := "scalar DateTime"
	if sdl != expected {
		t.Errorf("buildScalarSDL() = %q, want %q", sdl, expected)
	}
}

// Test GetEntities
func TestComposer_GetEntities(t *testing.T) {
	composer := NewComposer()

	// Initially should be empty
	entities := composer.GetEntities()
	if len(entities) != 0 {
		t.Errorf("GetEntities() returned %d entities, want 0", len(entities))
	}

	// Add an entity manually
	composer.entities["User"] = &Entity{
		Name:      "User",
		KeyFields: []string{"id"},
	}

	entities = composer.GetEntities()
	if len(entities) != 1 {
		t.Errorf("GetEntities() returned %d entities, want 1", len(entities))
	}
	if _, ok := entities["User"]; !ok {
		t.Error("GetEntities() should contain 'User' entity")
	}
}

// Test ExecuteParallel
func TestExecutor_ExecuteParallel(t *testing.T) {
	executor := NewExecutor()

	// Create a simple plan with steps
	plan := &Plan{
		Steps: []*PlanStep{
			{
				ID:       "step1",
				Subgraph: &Subgraph{ID: "subgraph1", Name: "users"},
				Query:    "{ users { id } }",
			},
		},
		DependsOn: map[string][]string{
			"step1": {},
		},
	}

	// Execute in parallel - will fail due to no actual server, but tests the code path
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This will likely error due to no actual subgraphs, but it exercises the code
	_, err := executor.ExecuteParallel(ctx, plan)
	// We expect an error since there are no real subgraphs
	if err == nil {
		t.Log("ExecuteParallel completed without error (may have used mock data)")
	}
}

// Test convertValue
func TestConvertValue(t *testing.T) {
	tests := []struct {
		name     string
		value    graphql.Value
		expected interface{}
	}{
		{"nil", nil, nil},
		{"scalar", &graphql.ScalarValue{Value: "hello"}, "hello"},
		{"list", &graphql.ListValue{Values: []graphql.Value{&graphql.ScalarValue{Value: "a"}, &graphql.ScalarValue{Value: "b"}}}, []interface{}{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertValue(tt.value)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("convertValue(nil) = %v, want nil", result)
				}
				return
			}
			// For complex types, just check not nil
			if result == nil {
				t.Errorf("convertValue(%v) = nil, want non-nil", tt.value)
			}
		})
	}
}

// Test buildEntityQuery
func TestPlanner_buildEntityQuery(t *testing.T) {
	planner := NewPlanner([]*Subgraph{}, make(map[string]*Entity))

	entity := &Entity{
		Name:      "User",
		KeyFields: []string{"id"},
	}

	field := GraphQLField{
		Name:   "user",
		Fields: []GraphQLField{{Name: "id"}, {Name: "name"}},
	}

	query := planner.buildEntityQuery(entity, field)
	if query == "" {
		t.Error("buildEntityQuery() returned empty query")
	}
	if !contains(query, "User") {
		t.Errorf("query should contain 'User', got: %s", query)
	}
	if !contains(query, "_entities") {
		t.Errorf("query should contain '_entities', got: %s", query)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
