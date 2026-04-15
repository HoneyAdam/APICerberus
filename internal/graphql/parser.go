package graphql

import (
	"errors"
	"fmt"
	"strings"
)

// Node is the interface for all AST nodes.
type Node interface {
	NodeKind() string
	Depth() int
}

// Document represents a GraphQL document.
type Document struct {
	Definitions []Node
}

// NodeKind returns the node kind.
func (d *Document) NodeKind() string { return "Document" }

// Depth returns the maximum depth of the document.
func (d *Document) Depth() int {
	maxDepth := 0
	for _, def := range d.Definitions {
		if depth := def.Depth(); depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

// Operation represents a GraphQL operation (query, mutation, subscription).
type Operation struct {
	Type       string // "query", "mutation", "subscription"
	Name       string
	Selections []Node
}

// NodeKind returns the node kind.
func (o *Operation) NodeKind() string { return "Operation" }

// Depth returns the depth of the operation.
func (o *Operation) Depth() int {
	maxDepth := 0
	for _, sel := range o.Selections {
		if depth := sel.Depth(); depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

// Field represents a field selection.
type Field struct {
	Name       string
	Alias      string
	Arguments  []Argument
	Selections []Node
}

// NodeKind returns the node kind.
func (f *Field) NodeKind() string { return "Field" }

// Depth returns the depth of the field.
func (f *Field) Depth() int {
	if len(f.Selections) == 0 {
		return 1
	}
	maxDepth := 0
	for _, sel := range f.Selections {
		if depth := sel.Depth(); depth+1 > maxDepth {
			maxDepth = depth + 1
		}
	}
	return maxDepth
}

// Argument represents a field argument.
type Argument struct {
	Name  string
	Value Value
}

// Value is the interface for argument values.
type Value interface {
	ValueKind() string
}

// ScalarValue represents a scalar value.
type ScalarValue struct {
	Value string
}

// ValueKind returns the value kind.
func (s *ScalarValue) ValueKind() string { return "Scalar" }

// ListValue represents a list value.
type ListValue struct {
	Values []Value
}

// ValueKind returns the value kind.
func (l *ListValue) ValueKind() string { return "List" }

// ObjectValue represents an object value.
type ObjectValue struct {
	Fields map[string]Value
}

// ValueKind returns the value kind.
func (o *ObjectValue) ValueKind() string { return "Object" }

// FragmentSpread represents a fragment spread.
type FragmentSpread struct {
	Name string
}

// NodeKind returns the node kind.
func (f *FragmentSpread) NodeKind() string { return "FragmentSpread" }

// Depth returns the depth of the fragment spread.
func (f *FragmentSpread) Depth() int { return 1 }

// InlineFragment represents an inline fragment.
type InlineFragment struct {
	TypeCondition string
	Selections    []Node
}

// NodeKind returns the node kind.
func (i *InlineFragment) NodeKind() string { return "InlineFragment" }

// Depth returns the depth of the inline fragment.
func (i *InlineFragment) Depth() int {
	if len(i.Selections) == 0 {
		return 1
	}
	maxDepth := 0
	for _, sel := range i.Selections {
		if depth := sel.Depth(); depth+1 > maxDepth {
			maxDepth = depth + 1
		}
	}
	return maxDepth
}

// FragmentDefinition represents a fragment definition.
type FragmentDefinition struct {
	Name       string
	Type       string
	Selections []Node
}

// NodeKind returns the node kind.
func (f *FragmentDefinition) NodeKind() string { return "FragmentDefinition" }

// Depth returns the depth of the fragment definition.
func (f *FragmentDefinition) Depth() int {
	if len(f.Selections) == 0 {
		return 1
	}
	maxDepth := 0
	for _, sel := range f.Selections {
		if depth := sel.Depth(); depth+1 > maxDepth {
			maxDepth = depth + 1
		}
	}
	return maxDepth
}

// ParseQuery parses a GraphQL query string into an AST.
// This is a simplified parser for depth and complexity analysis.
func ParseQuery(query string) (Node, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("empty query")
	}

	// M-018: Limit maximum query depth to prevent resource exhaustion.
	// A depth limit of 50 is sufficient for most legitimate queries while
	// preventing deeply nested malicious queries (e.g., 1000 levels).
	const defaultMaxDepth = 50

	// Simple recursive descent parser
	parser := &queryParser{
		input:    query,
		pos:      0,
		maxDepth: defaultMaxDepth,
	}

	return parser.parseDocument()
}

// queryParser is a simple GraphQL query parser.
type queryParser struct {
	input     string
	pos       int
	maxDepth  int  // M-018: Maximum nesting depth to prevent resource exhaustion
	currentDepth int
}

// parseDocument parses a document.
func (p *queryParser) parseDocument() (*Document, error) {
	doc := &Document{
		Definitions: make([]Node, 0),
	}

	for !p.isEOF() {
		p.skipWhitespace()
		if p.isEOF() {
			break
		}

		// Check for fragment definition
		if p.peek() == 'f' && p.peekWord() == "fragment" {
			frag, err := p.parseFragmentDefinition()
			if err != nil {
				return nil, err
			}
			doc.Definitions = append(doc.Definitions, frag)
			continue
		}

		// Parse operation
		op, err := p.parseOperation()
		if err != nil {
			return nil, err
		}
		doc.Definitions = append(doc.Definitions, op)
	}

	return doc, nil
}

// parseOperation parses an operation.
func (p *queryParser) parseOperation() (*Operation, error) {
	op := &Operation{
		Selections: make([]Node, 0),
	}

	p.skipWhitespace()

	// Check for operation type
	word := p.peekWord()
	switch word {
	case "query":
		op.Type = "query"
		p.advance(5)
	case "mutation":
		op.Type = "mutation"
		p.advance(8)
	case "subscription":
		op.Type = "subscription"
		p.advance(12)
	default:
		// Implicit query
		op.Type = "query"
	}

	p.skipWhitespace()

	// Check for operation name
	if p.peek() != '{' && !p.isEOF() {
		op.Name = p.parseName()
	}

	p.skipWhitespace()

	// Parse variable definitions if present
	if p.peek() == '(' {
		p.skipUntil('{')
	}

	p.skipWhitespace()

	// Expect opening brace
	if p.peek() != '{' {
		return nil, errors.New("expected '{' to start operation")
	}

	// Parse selections
	selections, err := p.parseSelections()
	if err != nil {
		return nil, err
	}
	op.Selections = selections

	return op, nil
}

// parseFragmentDefinition parses a fragment definition.
func (p *queryParser) parseFragmentDefinition() (*FragmentDefinition, error) {
	frag := &FragmentDefinition{
		Selections: make([]Node, 0),
	}

	p.advance(8) // "fragment"
	p.skipWhitespace()

	frag.Name = p.parseName()
	if frag.Name == "" {
		return nil, errors.New("expected fragment name")
	}
	p.skipWhitespace()

	// Expect "on"
	if p.peekWord() != "on" {
		return nil, errors.New("expected 'on' in fragment definition")
	}
	p.advance(2)
	p.skipWhitespace()

	frag.Type = p.parseName()
	if frag.Type == "" {
		return nil, errors.New("expected type condition in fragment definition")
	}
	p.skipWhitespace()

	// Parse directives if present
	if p.peek() == '@' {
		p.skipUntil('{')
	}

	p.skipWhitespace()

	// Expect opening brace
	if p.peek() != '{' {
		return nil, errors.New("expected '{' in fragment definition")
	}

	// Parse selections
	selections, err := p.parseSelections()
	if err != nil {
		return nil, err
	}
	frag.Selections = selections

	return frag, nil
}

// parseSelections parses a selection set.
func (p *queryParser) parseSelections() ([]Node, error) {
	selections := make([]Node, 0)

	if p.peek() != '{' {
		return nil, errors.New("expected '{'")
	}
	p.advance(1)
	p.skipWhitespace()

	// M-018: Track depth. Each `{...}` block increases nesting depth.
	p.currentDepth++
	if p.currentDepth > p.maxDepth {
		p.currentDepth--
		return nil, fmt.Errorf("query depth %d exceeds maximum allowed depth %d", p.currentDepth, p.maxDepth)
	}
	defer func() { p.currentDepth-- }()

	for p.peek() != '}' && !p.isEOF() {
		sel, err := p.parseSelection()
		if err != nil {
			return nil, err
		}
		selections = append(selections, sel)
		p.skipWhitespace()
	}

	if p.isEOF() && p.peek() != '}' {
		return nil, errors.New("expected '}' to close selection set")
	}

	if p.peek() == '}' {
		p.advance(1)
	}

	return selections, nil
}

// parseSelection parses a single selection.
func (p *queryParser) parseSelection() (Node, error) {
	p.skipWhitespace()

	// Check for inline fragment first (... on Type)
	if p.peek() == '.' && p.peekN(1) == '.' && p.peekN(2) == '.' {
		// Check if this is an inline fragment (... on) or fragment spread (...Name)
		if p.peekN(3) == ' ' {
			// Could be inline fragment, need to check further
			p.advance(3)
			p.skipWhitespace()
			if p.peekWord() == "on" {
				return p.parseInlineFragment()
			}
			// It's a fragment spread with name after ...
			return &FragmentSpread{Name: p.parseName()}, nil
		}
		// Fragment spread: ...Name
		p.advance(3)
		p.skipWhitespace()
		return &FragmentSpread{Name: p.parseName()}, nil
	}

	// Must be a field
	return p.parseField()
}

// parseInlineFragment parses an inline fragment.
func (p *queryParser) parseInlineFragment() (*InlineFragment, error) {
	frag := &InlineFragment{
		Selections: make([]Node, 0),
	}

	// Skip "..." if present (for direct calls)
	if p.peek() == '.' && p.peekN(1) == '.' && p.peekN(2) == '.' {
		p.advance(3)
		p.skipWhitespace()
	}

	// Check for "on" keyword (optional for type condition)
	if p.peekWord() == "on" {
		p.advance(2) // "on"
		p.skipWhitespace()
		frag.TypeCondition = p.parseName()
		if frag.TypeCondition == "" {
			return nil, errors.New("expected type condition in inline fragment")
		}
		p.skipWhitespace()
	}

	// Parse directives if present
	if p.peek() == '@' {
		p.skipUntil('{')
	}

	p.skipWhitespace()

	// Expect opening brace
	if p.peek() != '{' {
		return nil, errors.New("expected '{' in inline fragment")
	}

	// Parse selections
	selections, err := p.parseSelections()
	if err != nil {
		return nil, err
	}
	frag.Selections = selections

	return frag, nil
}

// parseField parses a field.
func (p *queryParser) parseField() (*Field, error) {
	field := &Field{
		Arguments:  make([]Argument, 0),
		Selections: make([]Node, 0),
	}

	// Parse name or alias
	name := p.parseName()
	p.skipWhitespace()

	// Check for valid field name
	if name == "" {
		return nil, errors.New("expected field name")
	}

	// Check for alias
	if p.peek() == ':' {
		field.Alias = name
		p.advance(1)
		p.skipWhitespace()
		field.Name = p.parseName()
		p.skipWhitespace()
		if field.Name == "" {
			return nil, errors.New("expected field name after alias")
		}
	} else {
		field.Name = name
	}

	// Parse arguments if present
	if p.peek() == '(' {
		args, err := p.parseArguments()
		if err != nil {
			return nil, err
		}
		field.Arguments = args
		p.skipWhitespace()
	}

	// Parse directives if present
	if p.peek() == '@' {
		p.skipUntil('{')
	}

	// Parse selections if present
	if p.peek() == '{' {
		selections, err := p.parseSelections()
		if err != nil {
			return nil, err
		}
		field.Selections = selections
	}

	return field, nil
}

// parseArguments parses field arguments.
func (p *queryParser) parseArguments() ([]Argument, error) {
	args := make([]Argument, 0)

	if p.peek() != '(' {
		return args, nil
	}
	p.advance(1)
	p.skipWhitespace()

	for p.peek() != ')' && !p.isEOF() {
		name := p.parseName()
		if name == "" {
			return nil, errors.New("expected argument name")
		}
		p.skipWhitespace()

		if p.peek() != ':' {
			return nil, errors.New("expected ':' in argument")
		}
		p.advance(1)
		p.skipWhitespace()

		value := p.parseValue()

		args = append(args, Argument{
			Name:  name,
			Value: value,
		})

		p.skipWhitespace()
		if p.peek() == ',' {
			p.advance(1)
			p.skipWhitespace()
		}
	}

	if p.isEOF() && p.peek() != ')' {
		return nil, errors.New("expected ')' to close arguments")
	}

	if p.peek() == ')' {
		p.advance(1)
	}

	return args, nil
}

// parseValue parses a value.
func (p *queryParser) parseValue() Value {
	p.skipWhitespace()

	// Check for list
	if p.peek() == '[' {
		return p.parseListValue()
	}

	// Check for object
	if p.peek() == '{' {
		return p.parseObjectValue()
	}

	// Scalar value
	return p.parseScalarValue()
}

// parseListValue parses a list value.
func (p *queryParser) parseListValue() *ListValue {
	list := &ListValue{
		Values: make([]Value, 0),
	}

	p.advance(1) // '['
	p.skipWhitespace()

	for p.peek() != ']' && !p.isEOF() {
		value := p.parseValue()
		list.Values = append(list.Values, value)
		p.skipWhitespace()
		if p.peek() == ',' {
			p.advance(1)
			p.skipWhitespace()
		}
	}

	if p.peek() == ']' {
		p.advance(1)
	}

	return list
}

// parseObjectValue parses an object value.
func (p *queryParser) parseObjectValue() *ObjectValue {
	obj := &ObjectValue{
		Fields: make(map[string]Value),
	}

	p.advance(1) // '{'
	p.skipWhitespace()

	for p.peek() != '}' && !p.isEOF() {
		name := p.parseName()
		p.skipWhitespace()

		if p.peek() == ':' {
			p.advance(1)
			p.skipWhitespace()
		}

		value := p.parseValue()
		obj.Fields[name] = value

		p.skipWhitespace()
		if p.peek() == ',' {
			p.advance(1)
			p.skipWhitespace()
		}
	}

	if p.peek() == '}' {
		p.advance(1)
	}

	return obj
}

// parseScalarValue parses a scalar value.
func (p *queryParser) parseScalarValue() *ScalarValue {
	start := p.pos

	// Handle string
	if p.peek() == '"' {
		return p.parseStringValue()
	}

	// Handle number, boolean, null, or enum
	for !p.isEOF() && !isWhitespace(p.peek()) && p.peek() != ',' && p.peek() != ')' && p.peek() != '}' && p.peek() != ']' {
		p.advance(1)
	}

	return &ScalarValue{Value: p.input[start:p.pos]}
}

// parseStringValue parses a string value.
func (p *queryParser) parseStringValue() *ScalarValue {
	p.advance(1) // opening quote
	start := p.pos

	for p.peek() != '"' && !p.isEOF() {
		if p.peek() == '\\' {
			p.advance(2) // skip escaped character
		} else {
			p.advance(1)
		}
	}

	value := p.input[start:p.pos]
	if p.peek() == '"' {
		p.advance(1) // closing quote
	}

	return &ScalarValue{Value: value}
}

// parseName parses an identifier name.
func (p *queryParser) parseName() string {
	start := p.pos

	for !p.isEOF() && (isLetter(p.peek()) || isDigit(p.peek()) || p.peek() == '_') {
		p.advance(1)
	}

	return p.input[start:p.pos]
}

// Helper methods

func (p *queryParser) peek() byte {
	if p.isEOF() {
		return 0
	}
	return p.input[p.pos]
}

func (p *queryParser) peekN(n int) byte {
	if p.pos+n >= len(p.input) {
		return 0
	}
	return p.input[p.pos+n]
}

func (p *queryParser) peekWord() string {
	start := p.pos
	for !p.isEOF() && isLetter(p.peek()) {
		p.advance(1)
	}
	word := p.input[start:p.pos]
	p.pos = start
	return word
}

func (p *queryParser) advance(n int) {
	p.pos += n
	if p.pos > len(p.input) {
		p.pos = len(p.input)
	}
}

func (p *queryParser) isEOF() bool {
	return p.pos >= len(p.input)
}

func (p *queryParser) skipWhitespace() {
	for !p.isEOF() && isWhitespace(p.peek()) {
		p.advance(1)
	}
}

func (p *queryParser) skipUntil(char byte) {
	for !p.isEOF() && p.peek() != char {
		p.advance(1)
	}
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ','
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
