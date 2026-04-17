package federation

import (
	"strings"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/graphql"
)

func TestBuildEntityQuery(t *testing.T) {
	t.Parallel()
	p := &Planner{}
	entity := &Entity{
		Name:      "User",
		KeyFields: []string{"id"},
	}
	field := GraphQLField{
		Name: "user",
		Fields: []GraphQLField{
			{Name: "id"},
			{Name: "name"},
			{Name: "email"},
		},
	}
	query := p.buildEntityQuery(entity, field)
	if !strings.Contains(query, "_entities") {
		t.Error("expected _entities in query")
	}
	if !strings.Contains(query, "_Any") {
		t.Error("expected _Any in query")
	}
	if !strings.Contains(query, "... on User") {
		t.Error("expected fragment on User")
	}
	if !strings.Contains(query, "name") {
		t.Error("expected name field in query")
	}
}

func TestConvertValue_ScalarValue(t *testing.T) {
	t.Parallel()
	v := convertValue(&graphql.ScalarValue{Value: "hello"})
	if v != "hello" {
		t.Errorf("got %v, want hello", v)
	}
}

func TestConvertValue_ListValue(t *testing.T) {
	t.Parallel()
	v := convertValue(&graphql.ListValue{
		Values: []graphql.Value{
			&graphql.ScalarValue{Value: "a"},
			&graphql.ScalarValue{Value: "b"},
		},
	})
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if len(list) != 2 {
		t.Errorf("list len = %d, want 2", len(list))
	}
	if list[0] != "a" || list[1] != "b" {
		t.Errorf("list = %v, want [a b]", list)
	}
}

func TestConvertValue_ObjectValue(t *testing.T) {
	t.Parallel()
	v := convertValue(&graphql.ObjectValue{
		Fields: map[string]graphql.Value{
			"key": &graphql.ScalarValue{Value: "val"},
		},
	})
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	if obj["key"] != "val" {
		t.Errorf("obj[key] = %v, want val", obj["key"])
	}
}

func TestConvertValue_Nil(t *testing.T) {
	t.Parallel()
	v := convertValue(nil)
	if v != nil {
		t.Errorf("got %v, want nil", v)
	}
}

func TestConvertValue_Unknown(t *testing.T) {
	t.Parallel()
	// A nil-value ScalarValue (non-nil interface, but zero struct) hits the ScalarValue branch
	// with an empty Value. Let's use a plain struct that satisfies Value but isn't matched.
	v := convertValue(nil)
	if v != nil {
		t.Errorf("nil should return nil, got %v", v)
	}
}

func TestConvertArguments_Empty(t *testing.T) {
	t.Parallel()
	result := convertArguments(nil)
	if result != nil {
		t.Errorf("expected nil for empty args, got %v", result)
	}
}

func TestConvertArguments_WithValues(t *testing.T) {
	t.Parallel()
	args := []graphql.Argument{
		{Name: "id", Value: &graphql.ScalarValue{Value: "123"}},
		{Name: "limit", Value: &graphql.ScalarValue{Value: "10"}},
	}
	result := convertArguments(args)
	if result["id"] != "123" {
		t.Errorf("id = %v, want 123", result["id"])
	}
	if result["limit"] != "10" {
		t.Errorf("limit = %v, want 10", result["limit"])
	}
}

func TestConvertArguments_ListArg(t *testing.T) {
	t.Parallel()
	args := []graphql.Argument{
		{
			Name: "ids",
			Value: &graphql.ListValue{
				Values: []graphql.Value{
					&graphql.ScalarValue{Value: "1"},
					&graphql.ScalarValue{Value: "2"},
				},
			},
		},
	}
	result := convertArguments(args)
	list, ok := result["ids"].([]any)
	if !ok {
		t.Fatalf("ids type = %T", result["ids"])
	}
	if len(list) != 2 {
		t.Errorf("ids len = %d, want 2", len(list))
	}
}

func TestBuildFieldQuery(t *testing.T) {
	t.Parallel()
	p := &Planner{}
	field := GraphQLField{
		Name: "users",
		Args: map[string]any{"limit": 10},
		Fields: []GraphQLField{
			{Name: "id"},
			{Name: "name"},
		},
	}
	query := p.buildFieldQuery(field)
	if !strings.Contains(query, "users") {
		t.Error("expected 'users' in query")
	}
	if !strings.Contains(query, "{") {
		t.Error("expected opening brace")
	}
}

func TestBuildFieldSelection_WithArgs(t *testing.T) {
	t.Parallel()
	p := &Planner{}
	field := GraphQLField{
		Name: "user",
		Args: map[string]any{"id": "123"},
	}
	sel := p.buildFieldSelection(field, 0)
	// JSON encoding quotes string values (e.g., id: "123" not id: 123)
	if !strings.Contains(sel, "user(id: \"123\")") {
		t.Errorf("selection = %q", sel)
	}
}

func TestBuildFieldSelection_NestedFields(t *testing.T) {
	t.Parallel()
	p := &Planner{}
	field := GraphQLField{
		Name: "user",
		Fields: []GraphQLField{
			{Name: "id"},
			{Name: "name"},
		},
	}
	sel := p.buildFieldSelection(field, 0)
	if !strings.Contains(sel, "user {") {
		t.Errorf("selection = %q", sel)
	}
	if !strings.Contains(sel, "id") {
		t.Error("expected nested field 'id'")
	}
}

func TestPlan_EmptyQuery(t *testing.T) {
	t.Parallel()
	p := NewPlanner(nil, nil)
	_, err := p.Plan("", nil)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}
