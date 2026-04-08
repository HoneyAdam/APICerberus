package federation

import (
	"fmt"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/graphql"
)

// Planner plans federated GraphQL queries.
type Planner struct {
	subgraphs []*Subgraph
	entities  map[string]*Entity
}

// Plan represents an execution plan for a federated query.
type Plan struct {
	Steps     []*PlanStep
	DependsOn map[string][]string // step ID -> dependencies
}

// PlanStep represents a single step in the execution plan.
type PlanStep struct {
	ID         string
	Subgraph   *Subgraph
	Query      string
	Variables  map[string]any
	DependsOn  []string
	ResultType string
	Path       []string
}

// NewPlanner creates a new query planner.
func NewPlanner(subgraphs []*Subgraph, entities map[string]*Entity) *Planner {
	return &Planner{
		subgraphs: subgraphs,
		entities:  entities,
	}
}

// Plan plans the execution of a GraphQL query.
func (p *Planner) Plan(query string, variables map[string]any) (*Plan, error) {
	// Parse the query
	doc, err := ParseGraphQLQuery(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	plan := &Plan{
		Steps:     make([]*PlanStep, 0),
		DependsOn: make(map[string][]string),
	}

	// Analyze query and create steps
	for _, op := range doc.Operations {
		steps, err := p.planOperation(op, variables)
		if err != nil {
			return nil, err
		}
		plan.Steps = append(plan.Steps, steps...)
	}

	// Build dependency graph
	for _, step := range plan.Steps {
		plan.DependsOn[step.ID] = step.DependsOn
	}

	return plan, nil
}

// planOperation plans a single operation.
func (p *Planner) planOperation(op GraphQLOperation, variables map[string]any) ([]*PlanStep, error) {
	steps := make([]*PlanStep, 0)

	// For each field in the operation, determine which subgraph can resolve it
	for _, field := range op.Fields {
		fieldSteps, err := p.planField(field, variables, []string{})
		if err != nil {
			return nil, err
		}
		steps = append(steps, fieldSteps...)
	}

	return steps, nil
}

// planField plans a single field.
func (p *Planner) planField(field GraphQLField, variables map[string]any, path []string) ([]*PlanStep, error) {
	steps := make([]*PlanStep, 0)
	currentPath := append(path, field.Name)

	// Find which subgraph can resolve this field
	subgraph := p.findSubgraphForField(field.Name)
	if subgraph == nil {
		return nil, fmt.Errorf("no subgraph can resolve field: %s", field.Name)
	}

	// Check if this is an entity field that requires resolution
	if entity, ok := p.entities[field.Name]; ok {
		// Create entity resolution step
		step := &PlanStep{
			ID:         fmt.Sprintf("step_%s", strings.Join(currentPath, "_")),
			Subgraph:   subgraph,
			Query:      p.buildEntityQuery(entity, field),
			Variables:  variables,
			ResultType: field.Name,
			Path:       currentPath,
		}
		steps = append(steps, step)

		// Plan nested fields
		for _, nestedField := range field.Fields {
			nestedSteps, err := p.planField(nestedField, variables, currentPath)
			if err != nil {
				return nil, err
			}
			// Mark dependency
			for _, nestedStep := range nestedSteps {
				nestedStep.DependsOn = append(nestedStep.DependsOn, step.ID)
			}
			steps = append(steps, nestedSteps...)
		}
	} else {
		// Regular field query
		step := &PlanStep{
			ID:         fmt.Sprintf("step_%s", strings.Join(currentPath, "_")),
			Subgraph:   subgraph,
			Query:      p.buildFieldQuery(field),
			Variables:  variables,
			ResultType: "scalar",
			Path:       currentPath,
		}
		steps = append(steps, step)

		// Plan nested fields on the same subgraph if possible
		for _, nestedField := range field.Fields {
			nestedSteps, err := p.planField(nestedField, variables, currentPath)
			if err != nil {
				return nil, err
			}
			steps = append(steps, nestedSteps...)
		}
	}

	return steps, nil
}

// findSubgraphForField finds a subgraph that can resolve the given field.
func (p *Planner) findSubgraphForField(fieldName string) *Subgraph {
	// Check if any entity has this field
	for _, entity := range p.entities {
		for _, sg := range entity.Subgraphs {
			if sg.Schema != nil {
				if _, ok := sg.Schema.Types[fieldName]; ok {
					return sg
				}
			}
		}
	}

	// Otherwise, find any subgraph that has this field in its Query type
	for _, sg := range p.subgraphs {
		if sg.Schema != nil && sg.Schema.QueryType != "" {
			if queryType, ok := sg.Schema.Types[sg.Schema.QueryType]; ok {
				if _, ok := queryType.Fields[fieldName]; ok {
					return sg
				}
			}
		}
	}

	return nil
}

// buildEntityQuery builds a query for entity resolution.
func (p *Planner) buildEntityQuery(entity *Entity, field GraphQLField) string {
	var sb strings.Builder

	// Build the entity representation query
	sb.WriteString("query ($representations: [_Any!]!) {\n")
	sb.WriteString(fmt.Sprintf("  _entities(representations: $representations) {\n"))
	sb.WriteString(fmt.Sprintf("    ... on %s {\n", entity.Name))

	// Add fields
	for _, f := range field.Fields {
		sb.WriteString(fmt.Sprintf("      %s\n", f.Name))
	}

	sb.WriteString("    }\n")
	sb.WriteString("  }\n")
	sb.WriteString("}")

	return sb.String()
}

// buildFieldQuery builds a query for a regular field.
func (p *Planner) buildFieldQuery(field GraphQLField) string {
	var sb strings.Builder

	sb.WriteString("{\n")
	sb.WriteString(p.buildFieldSelection(field, 1))
	sb.WriteString("}")

	return sb.String()
}

// buildFieldSelection builds a selection for a field.
func (p *Planner) buildFieldSelection(field GraphQLField, indent int) string {
	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	sb.WriteString(fmt.Sprintf("%s%s", prefix, field.Name))

	// Add arguments if any
	if len(field.Args) > 0 {
		sb.WriteString("(")
		first := true
		for name, value := range field.Args {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s: %v", name, value))
			first = false
		}
		sb.WriteString(")")
	}

	// Add subfields if any
	if len(field.Fields) > 0 {
		sb.WriteString(" {\n")
		for _, f := range field.Fields {
			sb.WriteString(p.buildFieldSelection(f, indent+1))
		}
		sb.WriteString(fmt.Sprintf("%s}", prefix))
	}

	sb.WriteString("\n")

	return sb.String()
}

// GraphQLDocument represents a parsed GraphQL document.
type GraphQLDocument struct {
	Operations []GraphQLOperation
}

// GraphQLOperation represents a GraphQL operation.
type GraphQLOperation struct {
	Type      string // query, mutation, subscription
	Name      string
	Fields    []GraphQLField
	Variables map[string]string
}

// GraphQLField represents a GraphQL field.
type GraphQLField struct {
	Name   string
	Alias  string
	Args   map[string]any
	Fields []GraphQLField
}

// ParseGraphQLQuery parses a GraphQL query string using the proper AST parser.
func ParseGraphQLQuery(query string) (*GraphQLDocument, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}

	node, err := graphql.ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return convertDocument(node)
}

// convertDocument converts a graphql.Node (expected to be *graphql.Document)
// into the federation GraphQLDocument used by the planner.
func convertDocument(node graphql.Node) (*GraphQLDocument, error) {
	doc, ok := node.(*graphql.Document)
	if !ok {
		return nil, fmt.Errorf("expected *graphql.Document, got %T", node)
	}

	fedDoc := &GraphQLDocument{
		Operations: make([]GraphQLOperation, 0),
	}

	for _, def := range doc.Definitions {
		switch d := def.(type) {
		case *graphql.Operation:
			op := GraphQLOperation{
				Type:   d.Type,
				Name:   d.Name,
				Fields: convertSelections(d.Selections),
			}
			fedDoc.Operations = append(fedDoc.Operations, op)
		}
		// Fragment definitions are ignored for now; they would be
		// resolved at a higher level before planning.
	}

	return fedDoc, nil
}

// convertSelections converts a slice of graphql.Node selections into
// federation GraphQLField slices.
func convertSelections(selections []graphql.Node) []GraphQLField {
	fields := make([]GraphQLField, 0, len(selections))
	for _, sel := range selections {
		switch s := sel.(type) {
		case *graphql.Field:
			f := GraphQLField{
				Name:   s.Name,
				Alias:  s.Alias,
				Args:   convertArguments(s.Arguments),
				Fields: convertSelections(s.Selections),
			}
			fields = append(fields, f)
		}
	}
	return fields
}

// convertArguments converts graphql.Argument slices into a map used by
// the federation planner.
func convertArguments(args []graphql.Argument) map[string]any {
	if len(args) == 0 {
		return nil
	}
	result := make(map[string]any, len(args))
	for _, arg := range args {
		result[arg.Name] = convertValue(arg.Value)
	}
	return result
}

// convertValue converts a graphql.Value into a plain Go value.
func convertValue(v graphql.Value) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case *graphql.ScalarValue:
		return val.Value
	case *graphql.ListValue:
		list := make([]any, 0, len(val.Values))
		for _, item := range val.Values {
			list = append(list, convertValue(item))
		}
		return list
	case *graphql.ObjectValue:
		obj := make(map[string]any, len(val.Fields))
		for k, fv := range val.Fields {
			obj[k] = convertValue(fv)
		}
		return obj
	default:
		return fmt.Sprintf("%v", v)
	}
}
