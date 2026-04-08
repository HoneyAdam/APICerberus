package federation

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Subgraph represents a federated GraphQL service.
type Subgraph struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Schema      *Schema           `json:"schema,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Health      HealthStatus      `json:"health"`
	LastUpdated time.Time         `json:"last_updated"`
	mu          sync.RWMutex      `json:"-"`
}

// HealthStatus represents the health of a subgraph.
type HealthStatus int

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthUnhealthy
)

// String returns the string representation of the health status.
func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// Schema represents a GraphQL schema.
type Schema struct {
	SDL              string                `json:"sdl"`
	Types            map[string]*Type      `json:"types"`
	QueryType        string                `json:"query_type"`
	MutationType     string                `json:"mutation_type,omitempty"`
	SubscriptionType string                `json:"subscription_type,omitempty"`
	Directives       map[string]*Directive `json:"directives"`
}

// Type represents a GraphQL type.
type Type struct {
	Kind          string                 `json:"kind"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description,omitempty"`
	Fields        map[string]*Field      `json:"fields,omitempty"`
	Interfaces    []string               `json:"interfaces,omitempty"`
	PossibleTypes []string               `json:"possible_types,omitempty"`
	EnumValues    []string               `json:"enum_values,omitempty"`
	InputFields   map[string]*InputField `json:"input_fields,omitempty"`
	OfType        string                 `json:"of_type,omitempty"`
	Directives    []TypeDirective        `json:"directives,omitempty"`
}

// TypeDirective represents a directive applied to a type.
type TypeDirective struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args,omitempty"`
}

// Field represents a GraphQL field.
type Field struct {
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	Type              string               `json:"type"`
	Args              map[string]*Argument `json:"args,omitempty"`
	IsDeprecated      bool                 `json:"is_deprecated"`
	DeprecationReason string               `json:"deprecation_reason,omitempty"`
}

// InputField represents a GraphQL input field.
type InputField struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value,omitempty"`
}

// Argument represents a GraphQL argument.
type Argument struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value,omitempty"`
}

// Directive represents a GraphQL directive.
type Directive struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Locations   []string             `json:"locations"`
	Args        map[string]*Argument `json:"args,omitempty"`
}

// SubgraphManager manages federated subgraphs.
type SubgraphManager struct {
	subgraphs map[string]*Subgraph
	mu        sync.RWMutex
	client    *http.Client
}

// NewSubgraphManager creates a new subgraph manager.
func NewSubgraphManager() *SubgraphManager {
	return &SubgraphManager{
		subgraphs: make(map[string]*Subgraph),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AddSubgraph adds a subgraph to the manager.
func (m *SubgraphManager) AddSubgraph(subgraph *Subgraph) error {
	if subgraph.ID == "" {
		return fmt.Errorf("subgraph ID is required")
	}
	if subgraph.URL == "" {
		return fmt.Errorf("subgraph URL is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.subgraphs[subgraph.ID] = subgraph
	return nil
}

// RemoveSubgraph removes a subgraph from the manager.
func (m *SubgraphManager) RemoveSubgraph(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.subgraphs, id)
}

// GetSubgraph returns a subgraph by ID.
func (m *SubgraphManager) GetSubgraph(id string) (*Subgraph, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sg, ok := m.subgraphs[id]
	return sg, ok
}

// ListSubgraphs returns all subgraphs.
func (m *SubgraphManager) ListSubgraphs() []*Subgraph {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subgraphs := make([]*Subgraph, 0, len(m.subgraphs))
	for _, sg := range m.subgraphs {
		subgraphs = append(subgraphs, sg)
	}
	return subgraphs
}

// FetchSchema fetches the schema from a subgraph using introspection.
func (m *SubgraphManager) FetchSchema(subgraph *Subgraph) (*Schema, error) {
	introspectionQuery := `
	query IntrospectionQuery {
		__schema {
			queryType { name }
			mutationType { name }
			subscriptionType { name }
			types {
				name
				kind
				description
				fields {
					name
					description
					type { name kind }
					args {
						name
						description
						type { name kind }
					}
				}
			}
		}
	}`

	reqBody := map[string]string{
		"query": introspectionQuery,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", subgraph.URL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range subgraph.Headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		subgraph.setHealth(HealthUnhealthy)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		subgraph.setHealth(HealthUnhealthy)
		return nil, fmt.Errorf("introspection failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	var introspectionResp struct {
		Data struct {
			Schema struct {
				QueryType        *TypeRef  `json:"queryType"`
				MutationType     *TypeRef  `json:"mutationType"`
				SubscriptionType *TypeRef  `json:"subscriptionType"`
				Types            []TypeDef `json:"types"`
			} `json:"__schema"`
		} `json:"data"`
		Errors []GraphQLError `json:"errors"`
	}

	if err := json.Unmarshal(body, &introspectionResp); err != nil {
		return nil, err
	}

	if len(introspectionResp.Errors) > 0 {
		subgraph.setHealth(HealthUnhealthy)
		return nil, fmt.Errorf("introspection errors: %v", introspectionResp.Errors)
	}

	schema := &Schema{
		Types:      make(map[string]*Type),
		Directives: make(map[string]*Directive),
	}

	if introspectionResp.Data.Schema.QueryType != nil {
		schema.QueryType = introspectionResp.Data.Schema.QueryType.Name
	}
	if introspectionResp.Data.Schema.MutationType != nil {
		schema.MutationType = introspectionResp.Data.Schema.MutationType.Name
	}
	if introspectionResp.Data.Schema.SubscriptionType != nil {
		schema.SubscriptionType = introspectionResp.Data.Schema.SubscriptionType.Name
	}

	for _, typeDef := range introspectionResp.Data.Schema.Types {
		t := &Type{
			Kind:        typeDef.Kind,
			Name:        typeDef.Name,
			Description: typeDef.Description,
			Fields:      make(map[string]*Field),
		}

		for _, field := range typeDef.Fields {
			f := &Field{
				Name:        field.Name,
				Description: field.Description,
				Type:        typeToString(field.Type),
				Args:        make(map[string]*Argument),
			}
			for _, arg := range field.Args {
				f.Args[arg.Name] = &Argument{
					Name:        arg.Name,
					Description: arg.Description,
					Type:        typeToString(arg.Type),
				}
			}
			t.Fields[field.Name] = f
		}

		schema.Types[typeDef.Name] = t
	}

	subgraph.setSchema(schema)
	subgraph.setHealth(HealthHealthy)

	return schema, nil
}

// CheckHealth checks the health of a subgraph.
func (m *SubgraphManager) CheckHealth(subgraph *Subgraph) error {
	req, err := http.NewRequest("GET", subgraph.URL, nil)
	if err != nil {
		subgraph.setHealth(HealthUnhealthy)
		return err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		subgraph.setHealth(HealthUnhealthy)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		subgraph.setHealth(HealthUnhealthy)
		return fmt.Errorf("subgraph returned status %d", resp.StatusCode)
	}

	subgraph.setHealth(HealthHealthy)
	return nil
}

// setSchema sets the schema for the subgraph.
func (s *Subgraph) setSchema(schema *Schema) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Schema = schema
	s.LastUpdated = time.Now()
}

// setHealth sets the health status for the subgraph.
func (s *Subgraph) setHealth(health HealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Health = health
}

// GetSchema returns the schema for the subgraph.
func (s *Subgraph) GetSchema() *Schema {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Schema
}

// Helper types for introspection

type TypeRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type TypeDef struct {
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Fields      []FieldDef `json:"fields"`
}
type FieldDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        *TypeRef `json:"type"`
	Args        []ArgDef `json:"args"`
}
type ArgDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        *TypeRef `json:"type"`
}
type GraphQLError struct {
	Message string `json:"message"`
}

func typeToString(t *TypeRef) string {
	if t == nil {
		return ""
	}
	return t.Name
}
