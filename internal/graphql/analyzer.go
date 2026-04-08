package graphql

import (
	"errors"
	"fmt"
)

// QueryAnalyzer analyzes GraphQL queries for depth and complexity.
type QueryAnalyzer struct {
	maxDepth      int
	maxComplexity int
	fieldCosts    map[string]int
	defaultCost   int
}

// AnalyzerConfig configures the query analyzer.
type AnalyzerConfig struct {
	MaxDepth      int
	MaxComplexity int
	FieldCosts    map[string]int
	DefaultCost   int
}

// NewQueryAnalyzer creates a new query analyzer.
func NewQueryAnalyzer(cfg *AnalyzerConfig) *QueryAnalyzer {
	if cfg == nil {
		cfg = &AnalyzerConfig{}
	}

	a := &QueryAnalyzer{
		maxDepth:      cfg.MaxDepth,
		maxComplexity: cfg.MaxComplexity,
		fieldCosts:    cfg.FieldCosts,
		defaultCost:   cfg.DefaultCost,
	}

	if a.maxDepth == 0 {
		a.maxDepth = 15 // Default max depth
	}
	if a.maxComplexity == 0 {
		a.maxComplexity = 1000 // Default max complexity
	}
	if a.defaultCost == 0 {
		a.defaultCost = 1
	}
	if a.fieldCosts == nil {
		a.fieldCosts = make(map[string]int)
	}

	return a
}

// AnalysisResult contains the analysis results.
type AnalysisResult struct {
	Depth      int
	Complexity int
	IsValid    bool
	Errors     []string
}

// Analyze analyzes a GraphQL query.
func (a *QueryAnalyzer) Analyze(query string) (*AnalysisResult, error) {
	result := &AnalysisResult{
		IsValid: true,
		Errors:  make([]string, 0),
	}

	// Parse the query
	ast, err := ParseQuery(query)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("parse error: %v", err))
		return result, err
	}

	// Calculate depth
	result.Depth = a.calculateDepth(ast)
	if a.maxDepth > 0 && result.Depth > a.maxDepth {
		result.IsValid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("query depth %d exceeds maximum %d", result.Depth, a.maxDepth))
	}

	// Calculate complexity
	result.Complexity = a.calculateComplexity(ast)
	if a.maxComplexity > 0 && result.Complexity > a.maxComplexity {
		result.IsValid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("query complexity %d exceeds maximum %d", result.Complexity, a.maxComplexity))
	}

	return result, nil
}

// CalculateDepth calculates only the depth of a query.
func (a *QueryAnalyzer) CalculateDepth(query string) (int, error) {
	ast, err := ParseQuery(query)
	if err != nil {
		return 0, err
	}
	return a.calculateDepth(ast), nil
}

// CalculateComplexity calculates only the complexity of a query.
func (a *QueryAnalyzer) CalculateComplexity(query string) (int, error) {
	ast, err := ParseQuery(query)
	if err != nil {
		return 0, err
	}
	return a.calculateComplexity(ast), nil
}

// calculateDepth recursively calculates the maximum depth of the AST.
func (a *QueryAnalyzer) calculateDepth(node Node) int {
	return node.Depth()
}

// calculateComplexity recursively calculates the complexity of the AST.
func (a *QueryAnalyzer) calculateComplexity(node Node) int {
	switch n := node.(type) {
	case *Document:
		total := 0
		for _, def := range n.Definitions {
			total += a.calculateComplexity(def)
		}
		return total

	case *Operation:
		total := 0
		for _, sel := range n.Selections {
			total += a.calculateComplexity(sel)
		}
		return total

	case *Field:
		cost := a.getFieldCost(n.Name)
		for _, sel := range n.Selections {
			cost += a.calculateComplexity(sel)
		}
		// Multiply by complexity of arguments (arrays add more complexity)
		if len(n.Arguments) > 0 {
			cost *= (1 + len(n.Arguments))
		}
		return cost

	case *FragmentSpread:
		// Fragment spreads are resolved elsewhere
		return a.defaultCost

	case *InlineFragment:
		total := 0
		for _, sel := range n.Selections {
			total += a.calculateComplexity(sel)
		}
		return total

	default:
		return a.defaultCost
	}
}

// getFieldCost returns the cost for a field.
func (a *QueryAnalyzer) getFieldCost(fieldName string) int {
	if cost, ok := a.fieldCosts[fieldName]; ok {
		return cost
	}
	return a.defaultCost
}

// SetFieldCost sets the cost for a specific field.
func (a *QueryAnalyzer) SetFieldCost(fieldName string, cost int) {
	a.fieldCosts[fieldName] = cost
}

// GetMaxDepth returns the configured maximum depth.
func (a *QueryAnalyzer) GetMaxDepth() int {
	return a.maxDepth
}

// GetMaxComplexity returns the configured maximum complexity.
func (a *QueryAnalyzer) GetMaxComplexity() int {
	return a.maxComplexity
}

// ValidateDepth checks if the query depth is within limits.
func (a *QueryAnalyzer) ValidateDepth(query string) error {
	depth, err := a.CalculateDepth(query)
	if err != nil {
		return err
	}
	if a.maxDepth > 0 && depth > a.maxDepth {
		return errors.New("query exceeds maximum depth")
	}
	return nil
}

// ValidateComplexity checks if the query complexity is within limits.
func (a *QueryAnalyzer) ValidateComplexity(query string) error {
	complexity, err := a.CalculateComplexity(query)
	if err != nil {
		return err
	}
	if a.maxComplexity > 0 && complexity > a.maxComplexity {
		return errors.New("query exceeds maximum complexity")
	}
	return nil
}
