package plugin

import (
	"net/http"
	"strconv"

	"github.com/APICerberus/APICerebrus/internal/graphql"
)

// GraphQLGuard is a plugin that provides GraphQL security features.
type GraphQLGuard struct {
	maxDepth           int
	maxComplexity      int
	blockIntrospection bool
	analyzer           *graphql.QueryAnalyzer
}

// GraphQLGuardConfig configures the GraphQLGuard plugin.
type GraphQLGuardConfig struct {
	MaxDepth           int            `json:"max_depth" yaml:"max_depth"`
	MaxComplexity      int            `json:"max_complexity" yaml:"max_complexity"`
	BlockIntrospection bool           `json:"block_introspection" yaml:"block_introspection"`
	FieldCosts         map[string]int `json:"field_costs" yaml:"field_costs"`
}

// NewGraphQLGuard creates a new GraphQLGuard plugin.
func NewGraphQLGuard(config *GraphQLGuardConfig) *GraphQLGuard {
	if config == nil {
		config = &GraphQLGuardConfig{}
	}

	g := &GraphQLGuard{
		maxDepth:           config.MaxDepth,
		maxComplexity:      config.MaxComplexity,
		blockIntrospection: config.BlockIntrospection,
	}

	if g.maxDepth == 0 {
		g.maxDepth = 15
	}
	if g.maxComplexity == 0 {
		g.maxComplexity = 1000
	}

	g.analyzer = graphql.NewQueryAnalyzer(&graphql.AnalyzerConfig{
		MaxDepth:      g.maxDepth,
		MaxComplexity: g.maxComplexity,
		FieldCosts:    config.FieldCosts,
		DefaultCost:   1,
	})

	return g
}

// Name returns the plugin name.
func (g *GraphQLGuard) Name() string {
	return "graphql_guard"
}

// Phase returns the plugin phase.
func (g *GraphQLGuard) Phase() Phase {
	return PhasePreAuth
}

// Priority returns the plugin priority.
func (g *GraphQLGuard) Priority() int {
	return 2
}

// Handle applies GraphQL guard logic. Returns true when request is blocked.
func (g *GraphQLGuard) Handle(w http.ResponseWriter, r *http.Request) bool {
	if g == nil || w == nil || r == nil {
		return false
	}

	// Check if this is a GraphQL request
	if !graphql.IsGraphQLRequest(r) {
		return false
	}

	// Parse the GraphQL request
	gqlReq, err := graphql.ParseRequest(r)
	if err != nil {
		graphql.WriteError(w, "failed to parse GraphQL request: "+err.Error(), http.StatusBadRequest)
		return true
	}

	// Check introspection
	if g.blockIntrospection && graphql.IsIntrospectionQuery(gqlReq.Query) {
		graphql.WriteError(w, "GraphQL introspection is disabled", http.StatusForbidden)
		return true
	}

	// Analyze query depth and complexity
	result, err := g.analyzer.Analyze(gqlReq.Query)
	if err != nil {
		graphql.WriteError(w, "failed to analyze query: "+err.Error(), http.StatusBadRequest)
		return true
	}

	if !result.IsValid {
		errors := ""
		for _, e := range result.Errors {
			errors += e + "; "
		}
		graphql.WriteError(w, errors, http.StatusBadRequest)
		return true
	}

	// Store analysis results in request headers for later use
	r.Header.Set("X-GraphQL-Depth", strconv.Itoa(result.Depth))
	r.Header.Set("X-GraphQL-Complexity", strconv.Itoa(result.Complexity))

	return false
}
