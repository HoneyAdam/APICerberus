package federation

import (
	"fmt"
	"strings"
)

// Composer composes multiple subgraph schemas into a unified supergraph.
type Composer struct {
	supergraph *Schema
	entities   map[string]*Entity
}

// Entity represents a federated entity.
type Entity struct {
	Name      string
	KeyFields []string
	Subgraphs map[string]*Subgraph // subgraph ID -> subgraph
	Resolvers map[string]*Resolver // subgraph ID -> resolver
}

// Resolver represents an entity resolver.
type Resolver struct {
	Subgraph   *Subgraph
	Query      string
	KeyField   string
	ReturnType string
}

// NewComposer creates a new schema composer.
func NewComposer() *Composer {
	return &Composer{
		supergraph: &Schema{
			Types:      make(map[string]*Type),
			Directives: make(map[string]*Directive),
		},
		entities: make(map[string]*Entity),
	}
}

// Compose composes multiple subgraph schemas into a unified supergraph.
func (c *Composer) Compose(subgraphs []*Subgraph) (*Schema, error) {
	if len(subgraphs) == 0 {
		return nil, fmt.Errorf("no subgraphs provided")
	}

	// First pass: collect all types
	for _, sg := range subgraphs {
		if sg.Schema == nil {
			continue
		}

		for typeName, t := range sg.Schema.Types {
			// Skip introspection types
			if strings.HasPrefix(typeName, "__") {
				continue
			}

			// Check if type already exists
			if existing, ok := c.supergraph.Types[typeName]; ok {
				// Merge types
				if err := c.mergeTypes(existing, t, sg); err != nil {
					return nil, fmt.Errorf("failed to merge type %s: %w", typeName, err)
				}
			} else {
				// Add new type
				c.supergraph.Types[typeName] = c.copyType(t)
			}

			// Check for @key directive
			if c.isEntity(t) {
				c.addEntity(typeName, sg)
			}
		}
	}

	// Second pass: add federation directives
	c.addFederationDirectives()

	// Build SDL
	c.supergraph.SDL = c.buildSDL()

	return c.supergraph, nil
}

// mergeTypes merges two types.
func (c *Composer) mergeTypes(existing *Type, new *Type, subgraph *Subgraph) error {
	// Merge fields
	for fieldName, field := range new.Fields {
		if _, ok := existing.Fields[fieldName]; !ok {
			existing.Fields[fieldName] = c.copyField(field)
		}
	}

	// Merge interfaces
	for _, iface := range new.Interfaces {
		found := false
		for _, existingIface := range existing.Interfaces {
			if existingIface == iface {
				found = true
				break
			}
		}
		if !found {
			existing.Interfaces = append(existing.Interfaces, iface)
		}
	}

	// Merge possible types (for unions)
	for _, possibleType := range new.PossibleTypes {
		found := false
		for _, existingType := range existing.PossibleTypes {
			if existingType == possibleType {
				found = true
				break
			}
		}
		if !found {
			existing.PossibleTypes = append(existing.PossibleTypes, possibleType)
		}
	}

	return nil
}

// isEntity checks if a type is a federated entity (has @key directive).
func (c *Composer) isEntity(t *Type) bool {
	// Check for @key directive on the type
	for _, dir := range t.Directives {
		if dir.Name == "key" {
			return true
		}
	}

	// Fall back to checking for "id" field as heuristic
	for _, field := range t.Fields {
		if field.Name == "id" {
			return true
		}
	}
	return false
}

// addEntity adds an entity to the federation.
func (c *Composer) addEntity(name string, subgraph *Subgraph) {
	if _, ok := c.entities[name]; !ok {
		// Extract key fields from @key directive if present
		keyFields := []string{"id"}
		if t, ok := subgraph.Schema.Types[name]; ok {
			for _, dir := range t.Directives {
				if dir.Name == "key" {
					if fields, ok := dir.Args["fields"]; ok && fields != "" {
						keyFields = strings.Fields(fields)
					}
				}
			}
		}

		c.entities[name] = &Entity{
			Name:      name,
			KeyFields: keyFields,
			Subgraphs: make(map[string]*Subgraph),
			Resolvers: make(map[string]*Resolver),
		}
	}

	c.entities[name].Subgraphs[subgraph.ID] = subgraph
}

// addFederationDirectives adds federation directives to the schema.
func (c *Composer) addFederationDirectives() {
	// Add @key directive
	c.supergraph.Directives["key"] = &Directive{
		Name:        "key",
		Description: "Indicates a key field for an entity",
		Locations:   []string{"FIELD_DEFINITION", "OBJECT"},
		Args: map[string]*Argument{
			"fields": {
				Name: "fields",
				Type: "String!",
			},
		},
	}

	// Add @external directive
	c.supergraph.Directives["external"] = &Directive{
		Name:        "external",
		Description: "Indicates a field is owned by another service",
		Locations:   []string{"FIELD_DEFINITION"},
	}

	// Add @requires directive
	c.supergraph.Directives["requires"] = &Directive{
		Name:        "requires",
		Description: "Indicates required fields from other services",
		Locations:   []string{"FIELD_DEFINITION"},
		Args: map[string]*Argument{
			"fields": {
				Name: "fields",
				Type: "String!",
			},
		},
	}

	// Add @provides directive
	c.supergraph.Directives["provides"] = &Directive{
		Name:        "provides",
		Description: "Indicates fields provided by this service",
		Locations:   []string{"FIELD_DEFINITION"},
		Args: map[string]*Argument{
			"fields": {
				Name: "fields",
				Type: "String!",
			},
		},
	}
}

// buildSDL builds the SDL string from the schema.
func (c *Composer) buildSDL() string {
	var sb strings.Builder

	// Write directives
	for _, dir := range c.supergraph.Directives {
		sb.WriteString(fmt.Sprintf("directive @%s", dir.Name))
		if len(dir.Args) > 0 {
			sb.WriteString("(")
			first := true
			for _, arg := range dir.Args {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%s: %s", arg.Name, arg.Type))
				first = false
			}
			sb.WriteString(")")
		}
		sb.WriteString(fmt.Sprintf(" on %s\n", strings.Join(dir.Locations, " | ")))
	}
	sb.WriteString("\n")

	// Write types
	for _, t := range c.supergraph.Types {
		if strings.HasPrefix(t.Name, "__") {
			continue
		}

		switch t.Kind {
		case "OBJECT":
			sb.WriteString(c.buildObjectSDL(t))
		case "INTERFACE":
			sb.WriteString(c.buildInterfaceSDL(t))
		case "UNION":
			sb.WriteString(c.buildUnionSDL(t))
		case "ENUM":
			sb.WriteString(c.buildEnumSDL(t))
		case "INPUT_OBJECT":
			sb.WriteString(c.buildInputSDL(t))
		case "SCALAR":
			sb.WriteString(c.buildScalarSDL(t))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildObjectSDL builds SDL for an object type.
func (c *Composer) buildObjectSDL(t *Type) string {
	var sb strings.Builder

	if t.Description != "" {
		sb.WriteString(fmt.Sprintf("\"%s\"\n", t.Description))
	}

	sb.WriteString(fmt.Sprintf("type %s", t.Name))

	// Write implements
	if len(t.Interfaces) > 0 {
		sb.WriteString(fmt.Sprintf(" implements %s", strings.Join(t.Interfaces, " & ")))
	}

	sb.WriteString(" {\n")

	// Write fields
	for _, field := range t.Fields {
		if field.Description != "" {
			sb.WriteString(fmt.Sprintf("  \"%s\"\n", field.Description))
		}
		sb.WriteString(fmt.Sprintf("  %s", field.Name))

		// Write args
		if len(field.Args) > 0 {
			sb.WriteString("(")
			first := true
			for _, arg := range field.Args {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%s: %s", arg.Name, arg.Type))
				first = false
			}
			sb.WriteString(")")
		}

		sb.WriteString(fmt.Sprintf(": %s", field.Type))

		if field.IsDeprecated && field.DeprecationReason != "" {
			sb.WriteString(fmt.Sprintf(" @deprecated(reason: \"%s\")", field.DeprecationReason))
		}

		sb.WriteString("\n")
	}

	sb.WriteString("}")

	return sb.String()
}

// buildInterfaceSDL builds SDL for an interface type.
func (c *Composer) buildInterfaceSDL(t *Type) string {
	var sb strings.Builder

	if t.Description != "" {
		sb.WriteString(fmt.Sprintf("\"%s\"\n", t.Description))
	}

	sb.WriteString(fmt.Sprintf("interface %s {\n", t.Name))

	for _, field := range t.Fields {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", field.Name, field.Type))
	}

	sb.WriteString("}")

	return sb.String()
}

// buildUnionSDL builds SDL for a union type.
func (c *Composer) buildUnionSDL(t *Type) string {
	return fmt.Sprintf("union %s = %s", t.Name, strings.Join(t.PossibleTypes, " | "))
}

// buildEnumSDL builds SDL for an enum type.
func (c *Composer) buildEnumSDL(t *Type) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("enum %s {\n", t.Name))
	for _, value := range t.EnumValues {
		sb.WriteString(fmt.Sprintf("  %s\n", value))
	}
	sb.WriteString("}")
	return sb.String()
}

// buildInputSDL builds SDL for an input type.
func (c *Composer) buildInputSDL(t *Type) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("input %s {\n", t.Name))
	for _, field := range t.InputFields {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", field.Name, field.Type))
	}
	sb.WriteString("}")
	return sb.String()
}

// buildScalarSDL builds SDL for a scalar type.
func (c *Composer) buildScalarSDL(t *Type) string {
	return fmt.Sprintf("scalar %s", t.Name)
}

// copyType creates a copy of a type.
func (c *Composer) copyType(t *Type) *Type {
	return &Type{
		Kind:          t.Kind,
		Name:          t.Name,
		Description:   t.Description,
		Fields:        make(map[string]*Field),
		Interfaces:    append([]string(nil), t.Interfaces...),
		PossibleTypes: append([]string(nil), t.PossibleTypes...),
		EnumValues:    append([]string(nil), t.EnumValues...),
		InputFields:   make(map[string]*InputField),
		OfType:        t.OfType,
	}
}

// copyField creates a copy of a field.
func (c *Composer) copyField(f *Field) *Field {
	return &Field{
		Name:              f.Name,
		Description:       f.Description,
		Type:              f.Type,
		Args:              make(map[string]*Argument),
		IsDeprecated:      f.IsDeprecated,
		DeprecationReason: f.DeprecationReason,
	}
}

// GetEntities returns all federated entities.
func (c *Composer) GetEntities() map[string]*Entity {
	return c.entities
}
