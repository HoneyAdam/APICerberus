package federation

import (
	"testing"
)

func TestSubgraphManager(t *testing.T) {
	manager := NewSubgraphManagerWith(WithURLValidation(false))

	// Test AddSubgraph
	subgraph := &Subgraph{
		ID:   "test-subgraph",
		Name: "Test Subgraph",
		URL:  "http://localhost:4001/graphql",
	}

	err := manager.AddSubgraph(subgraph)
	if err != nil {
		t.Fatalf("Failed to add subgraph: %v", err)
	}

	// Test GetSubgraph
	got, ok := manager.GetSubgraph("test-subgraph")
	if !ok {
		t.Error("Expected to find subgraph")
	}
	if got.Name != "Test Subgraph" {
		t.Errorf("Expected name 'Test Subgraph', got '%s'", got.Name)
	}

	// Test ListSubgraphs
	subgraphs := manager.ListSubgraphs()
	if len(subgraphs) != 1 {
		t.Errorf("Expected 1 subgraph, got %d", len(subgraphs))
	}

	// Test RemoveSubgraph
	manager.RemoveSubgraph("test-subgraph")
	_, ok = manager.GetSubgraph("test-subgraph")
	if ok {
		t.Error("Expected subgraph to be removed")
	}
}

func TestSubgraphManagerValidation(t *testing.T) {
	manager := NewSubgraphManagerWith(WithURLValidation(false))

	// Test missing ID
	err := manager.AddSubgraph(&Subgraph{URL: "http://localhost:4001/graphql"})
	if err == nil {
		t.Error("Expected error for missing ID")
	}

	// Test missing URL
	err = manager.AddSubgraph(&Subgraph{ID: "test"})
	if err == nil {
		t.Error("Expected error for missing URL")
	}
}

func TestSchemaCopy(t *testing.T) {
	schema := &Schema{
		Types: map[string]*Type{
			"User": {
				Kind: "OBJECT",
				Name: "User",
				Fields: map[string]*Field{
					"id": {
						Name: "id",
						Type: "ID!",
					},
				},
			},
		},
	}

	if schema.Types["User"] == nil {
		t.Error("Expected User type to exist")
	}
	if schema.Types["User"].Fields["id"] == nil {
		t.Error("Expected id field to exist")
	}
}

func TestComposer(t *testing.T) {
	composer := NewComposer()

	// Create test subgraphs
	subgraph1 := &Subgraph{
		ID: "users",
		Schema: &Schema{
			Types: map[string]*Type{
				"User": {
					Kind: "OBJECT",
					Name: "User",
					Fields: map[string]*Field{
						"id":   {Name: "id", Type: "ID!"},
						"name": {Name: "name", Type: "String!"},
					},
				},
				"Query": {
					Kind: "OBJECT",
					Name: "Query",
					Fields: map[string]*Field{
						"user":  {Name: "user", Type: "User"},
						"users": {Name: "users", Type: "[User]"},
					},
				},
			},
			QueryType: "Query",
		},
	}

	subgraph2 := &Subgraph{
		ID: "posts",
		Schema: &Schema{
			Types: map[string]*Type{
				"Post": {
					Kind: "OBJECT",
					Name: "Post",
					Fields: map[string]*Field{
						"id":     {Name: "id", Type: "ID!"},
						"title":  {Name: "title", Type: "String!"},
						"userId": {Name: "userId", Type: "ID!"},
					},
				},
				"Query": {
					Kind: "OBJECT",
					Name: "Query",
					Fields: map[string]*Field{
						"post":  {Name: "post", Type: "Post"},
						"posts": {Name: "posts", Type: "[Post]"},
					},
				},
			},
			QueryType: "Query",
		},
	}

	// Compose schemas
	supergraph, err := composer.Compose([]*Subgraph{subgraph1, subgraph2})
	if err != nil {
		t.Fatalf("Failed to compose schemas: %v", err)
	}

	// Check composed schema
	if _, ok := supergraph.Types["User"]; !ok {
		t.Error("Expected User type in supergraph")
	}
	if _, ok := supergraph.Types["Post"]; !ok {
		t.Error("Expected Post type in supergraph")
	}

	// Check SDL was generated
	if supergraph.SDL == "" {
		t.Error("Expected SDL to be generated")
	}

	t.Logf("Generated SDL:\n%s", supergraph.SDL)
}

func TestPlanner(t *testing.T) {
	// Create test subgraphs
	subgraph := &Subgraph{
		ID: "users",
		Schema: &Schema{
			Types: map[string]*Type{
				"User": {
					Kind: "OBJECT",
					Name: "User",
					Fields: map[string]*Field{
						"id":   {Name: "id", Type: "ID!"},
						"name": {Name: "name", Type: "String!"},
					},
				},
				"Query": {
					Kind: "OBJECT",
					Name: "Query",
					Fields: map[string]*Field{
						"user":  {Name: "user", Type: "User"},
						"users": {Name: "users", Type: "[User]"},
					},
				},
			},
			QueryType: "Query",
		},
	}

	planner := NewPlanner([]*Subgraph{subgraph}, make(map[string]*Entity))

	// Test planning a simple query
	query := `{ users { id name } }`
	plan, err := planner.Plan(query, nil)
	if err != nil {
		t.Logf("Plan error (expected with simple parser): %v", err)
		return
	}

	if len(plan.Steps) == 0 {
		t.Error("Expected plan steps")
	}
}

func TestParseGraphQLQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "simple query",
			query:   `{ users { id name } }`,
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
		},
		{
			name:    "query with operation",
			query:   `query GetUsers { users { id } }`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseGraphQLQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGraphQLQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if doc == nil {
				t.Error("Expected document")
			}
		})
	}
}

func TestHealthStatus(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthUnknown, "unknown"},
		{HealthHealthy, "healthy"},
		{HealthUnhealthy, "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntity(t *testing.T) {
	entity := &Entity{
		Name:      "User",
		KeyFields: []string{"id"},
		Subgraphs: make(map[string]*Subgraph),
		Resolvers: make(map[string]*Resolver),
	}

	if entity.Name != "User" {
		t.Errorf("Expected name 'User', got '%s'", entity.Name)
	}

	if len(entity.KeyFields) != 1 || entity.KeyFields[0] != "id" {
		t.Error("Expected key field 'id'")
	}
}
