package federation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSubgraphManager(t *testing.T) {
	manager := NewSubgraphManagerWith(WithURLValidation(false))
	if manager == nil {
		t.Fatal("NewSubgraphManager() returned nil")
	}
	if manager.subgraphs == nil {
		t.Error("subgraphs map not initialized")
	}
	if manager.client == nil {
		t.Error("client not initialized")
	}
}

func TestSubgraphManager_AddSubgraph(t *testing.T) {
	// Use a manager with URL validation disabled since test URLs use localhost
	manager := NewSubgraphManagerWith(WithURLValidation(false))

	t.Run("Valid subgraph", func(t *testing.T) {
		subgraph := &Subgraph{
			ID:  "users",
			URL: "http://localhost:4001/graphql",
		}
		err := manager.AddSubgraph(subgraph)
		if err != nil {
			t.Errorf("AddSubgraph() error = %v", err)
		}

		retrieved, ok := manager.GetSubgraph("users")
		if !ok {
			t.Error("GetSubgraph() returned false for existing subgraph")
		}
		if retrieved.ID != "users" {
			t.Errorf("ID = %v, want users", retrieved.ID)
		}
	})

	t.Run("Missing ID", func(t *testing.T) {
		subgraph := &Subgraph{
			URL: "http://localhost:4001/graphql",
		}
		err := manager.AddSubgraph(subgraph)
		if err == nil {
			t.Error("AddSubgraph() should return error for missing ID")
		}
	})

	t.Run("Missing URL", func(t *testing.T) {
		subgraph := &Subgraph{
			ID: "posts",
		}
		err := manager.AddSubgraph(subgraph)
		if err == nil {
			t.Error("AddSubgraph() should return error for missing URL")
		}
	})
}

func TestSubgraphManager_RemoveSubgraph(t *testing.T) {
	manager := NewSubgraphManagerWith(WithURLValidation(false))

	subgraph := &Subgraph{
		ID:  "users",
		URL: "http://localhost:4001/graphql",
	}
	_ = manager.AddSubgraph(subgraph)

	manager.RemoveSubgraph("users")

	_, ok := manager.GetSubgraph("users")
	if ok {
		t.Error("GetSubgraph() should return false after removal")
	}
}

func TestSubgraphManager_ListSubgraphs(t *testing.T) {
	manager := NewSubgraphManagerWith(WithURLValidation(false))

	// Add multiple subgraphs
	_ = manager.AddSubgraph(&Subgraph{ID: "users", URL: "http://localhost:4001/graphql"})
	_ = manager.AddSubgraph(&Subgraph{ID: "posts", URL: "http://localhost:4002/graphql"})

	subgraphs := manager.ListSubgraphs()
	if len(subgraphs) != 2 {
		t.Errorf("len(subgraphs) = %v, want 2", len(subgraphs))
	}
}

func TestSubgraphManager_CheckHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	manager := NewSubgraphManagerWith(WithURLValidation(false))
	subgraph := &Subgraph{
		ID:  "users",
		URL: server.URL,
	}

	err := manager.CheckHealth(subgraph)
	if err != nil {
		t.Errorf("CheckHealth() error = %v", err)
	}

	if subgraph.Health != HealthHealthy {
		t.Errorf("Health = %v, want HealthHealthy", subgraph.Health)
	}
}

func TestSubgraphManager_CheckHealth_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	manager := NewSubgraphManagerWith(WithURLValidation(false))
	subgraph := &Subgraph{
		ID:  "users",
		URL: server.URL,
	}

	err := manager.CheckHealth(subgraph)
	if err == nil {
		t.Error("CheckHealth() should return error for unhealthy subgraph")
	}

	if subgraph.Health != HealthUnhealthy {
		t.Errorf("Health = %v, want HealthUnhealthy", subgraph.Health)
	}
}

func TestSubgraphManager_FetchSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"data": map[string]interface{}{
				"__schema": map[string]interface{}{
					"queryType": map[string]interface{}{"name": "Query"},
					"types": []map[string]interface{}{
						{
							"kind": "OBJECT",
							"name": "User",
							"fields": []map[string]interface{}{
								{
									"name":        "id",
									"description": "User ID",
									"type":        map[string]interface{}{"name": "ID", "kind": "SCALAR"},
									"args":        []map[string]interface{}{},
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	manager := NewSubgraphManagerWith(WithURLValidation(false))
	subgraph := &Subgraph{
		ID:  "users",
		URL: server.URL,
	}

	schema, err := manager.FetchSchema(subgraph)
	if err != nil {
		t.Errorf("FetchSchema() error = %v", err)
	}
	if schema == nil {
		t.Fatal("FetchSchema() returned nil schema")
	}

	if schema.QueryType != "Query" {
		t.Errorf("QueryType = %v, want Query", schema.QueryType)
	}

	if len(schema.Types) == 0 {
		t.Error("Types should not be empty")
	}

	if subgraph.Health != HealthHealthy {
		t.Errorf("Health = %v, want HealthHealthy", subgraph.Health)
	}
}

func TestSubgraphManager_FetchSchema_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"errors": []map[string]interface{}{
				{"message": "Introspection disabled"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	manager := NewSubgraphManagerWith(WithURLValidation(false))
	subgraph := &Subgraph{
		ID:  "users",
		URL: server.URL,
	}

	_, err := manager.FetchSchema(subgraph)
	if err == nil {
		t.Error("FetchSchema() should return error for introspection errors")
	}

	if subgraph.Health != HealthUnhealthy {
		t.Errorf("Health = %v, want HealthUnhealthy", subgraph.Health)
	}
}

func TestSubgraphManager_FetchSchema_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	manager := NewSubgraphManagerWith(WithURLValidation(false))
	subgraph := &Subgraph{
		ID:  "users",
		URL: server.URL,
	}

	_, err := manager.FetchSchema(subgraph)
	if err == nil {
		t.Error("FetchSchema() should return error for non-200 status")
	}

	if subgraph.Health != HealthUnhealthy {
		t.Errorf("Health = %v, want HealthUnhealthy", subgraph.Health)
	}
}

func TestHealthStatus_String(t *testing.T) {
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

func TestSubgraph_SchemaMethods(t *testing.T) {
	subgraph := &Subgraph{
		ID:  "users",
		URL: "http://localhost:4001/graphql",
	}

	schema := &Schema{
		QueryType: "Query",
		Types: map[string]*Type{
			"User": {
				Kind: "OBJECT",
				Name: "User",
			},
		},
	}

	subgraph.setSchema(schema)

	retrieved := subgraph.GetSchema()
	if retrieved == nil {
		t.Fatal("GetSchema() returned nil")
	}
	if retrieved.QueryType != "Query" {
		t.Errorf("QueryType = %v, want Query", retrieved.QueryType)
	}
}

func TestSubgraph_HealthMethods(t *testing.T) {
	subgraph := &Subgraph{
		ID:  "users",
		URL: "http://localhost:4001/graphql",
	}

	if subgraph.Health != HealthUnknown {
		t.Errorf("Initial Health = %v, want HealthUnknown", subgraph.Health)
	}

	subgraph.setHealth(HealthHealthy)
	if subgraph.Health != HealthHealthy {
		t.Errorf("Health after set = %v, want HealthHealthy", subgraph.Health)
	}
}

func TestSchema_Types(t *testing.T) {
	schema := &Schema{
		SDL:       "type Query { users: [User] }",
		QueryType: "Query",
		Types: map[string]*Type{
			"Query": {
				Kind: "OBJECT",
				Name: "Query",
				Fields: map[string]*Field{
					"users": {
						Name: "users",
						Type: "[User]",
						Args: map[string]*Argument{
							"limit": {
								Name: "limit",
								Type: "Int",
							},
						},
					},
				},
			},
			"User": {
				Kind: "OBJECT",
				Name: "User",
				Fields: map[string]*Field{
					"id": {
						Name:              "id",
						Type:              "ID",
						IsDeprecated:      true,
						DeprecationReason: "Use userId instead",
					},
				},
			},
		},
		MutationType:     "Mutation",
		SubscriptionType: "Subscription",
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Errorf("Failed to marshal Schema: %v", err)
	}

	var decoded Schema
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal Schema: %v", err)
	}

	if decoded.QueryType != "Query" {
		t.Errorf("QueryType = %v, want Query", decoded.QueryType)
	}
	if decoded.MutationType != "Mutation" {
		t.Errorf("MutationType = %v, want Mutation", decoded.MutationType)
	}
}

func TestType_FieldAccess(t *testing.T) {
	typ := &Type{
		Kind:        "OBJECT",
		Name:        "User",
		Description: "A user in the system",
		Fields: map[string]*Field{
			"id": {
				Name: "id",
				Type: "ID",
			},
		},
		Interfaces:    []string{"Node"},
		PossibleTypes: []string{"User", "Admin"},
		EnumValues:    []string{"ACTIVE", "INACTIVE"},
		Directives: []TypeDirective{
			{Name: "deprecated", Args: map[string]string{"reason": "Use new field"}},
		},
	}

	if typ.Fields["id"] == nil {
		t.Error("Field 'id' not found")
	}
	if len(typ.Interfaces) != 1 {
		t.Errorf("Interfaces length = %v, want 1", len(typ.Interfaces))
	}
}

func TestField_WithArgs(t *testing.T) {
	field := &Field{
		Name:        "users",
		Description: "Get all users",
		Type:        "[User]",
		Args: map[string]*Argument{
			"limit": {
				Name:         "limit",
				Type:         "Int",
				DefaultValue: "10",
			},
		},
		IsDeprecated:      true,
		DeprecationReason: "Use usersV2 instead",
	}

	if field.Args["limit"] == nil {
		t.Error("Argument 'limit' not found")
	}
}

func TestArgument(t *testing.T) {
	arg := &Argument{
		Name:         "limit",
		Description:  "Maximum number of items",
		Type:         "Int",
		DefaultValue: "10",
	}

	data, err := json.Marshal(arg)
	if err != nil {
		t.Errorf("Failed to marshal Argument: %v", err)
	}

	var decoded Argument
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal Argument: %v", err)
	}

	if decoded.Name != "limit" {
		t.Errorf("Name = %v, want limit", decoded.Name)
	}
}

func TestInputField(t *testing.T) {
	field := &InputField{
		Name:         "name",
		Description:  "User name",
		Type:         "String",
		DefaultValue: "Anonymous",
	}

	data, err := json.Marshal(field)
	if err != nil {
		t.Errorf("Failed to marshal InputField: %v", err)
	}

	var decoded InputField
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal InputField: %v", err)
	}

	if decoded.DefaultValue != "Anonymous" {
		t.Errorf("DefaultValue = %v, want Anonymous", decoded.DefaultValue)
	}
}

func TestDirective(t *testing.T) {
	directive := &Directive{
		Name:        "auth",
		Description: "Requires authentication",
		Locations:   []string{"FIELD_DEFINITION", "OBJECT"},
		Args: map[string]*Argument{
			"role": {
				Name: "role",
				Type: "String",
			},
		},
	}

	data, err := json.Marshal(directive)
	if err != nil {
		t.Errorf("Failed to marshal Directive: %v", err)
	}

	var decoded Directive
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal Directive: %v", err)
	}

	if len(decoded.Locations) != 2 {
		t.Errorf("Locations length = %v, want 2", len(decoded.Locations))
	}
}

func TestTypeToString(t *testing.T) {
	tests := []struct {
		input *TypeRef
		want  string
	}{
		{&TypeRef{Name: "String", Kind: "SCALAR"}, "String"},
		{&TypeRef{Name: "User", Kind: "OBJECT"}, "User"},
		{nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := typeToString(tt.input)
			if got != tt.want {
				t.Errorf("typeToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubgraph_LastUpdated(t *testing.T) {
	subgraph := &Subgraph{
		ID:  "users",
		URL: "http://localhost:4001/graphql",
	}

	before := time.Now()
	subgraph.setSchema(&Schema{QueryType: "Query"})
	after := time.Now()

	if subgraph.LastUpdated.Before(before) || subgraph.LastUpdated.After(after) {
		t.Error("LastUpdated should be set to current time")
	}
}
