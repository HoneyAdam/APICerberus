package graphql

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Request represents a GraphQL request.
type Request struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

// Response represents a GraphQL response.
type Response struct {
	Data   json.RawMessage `json:"data,omitempty"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// IsGraphQLRequest checks if an HTTP request is a GraphQL request.
func IsGraphQLRequest(r *http.Request) bool {
	// Check GET with query parameter
	if r.Method == http.MethodGet {
		return r.URL.Query().Get("query") != ""
	}

	// Check POST methods
	if r.Method != http.MethodPost {
		return false
	}

	contentType := r.Header.Get("Content-Type")

	// application/graphql content type
	if contentType == "application/graphql" {
		return true
	}

	// application/json with query field
	if strings.HasPrefix(contentType, "application/json") {
		return true
	}

	return false
}

// ParseRequest parses a GraphQL request from an HTTP request.
func ParseRequest(r *http.Request) (*Request, error) {
	if r.Method == http.MethodGet {
		return parseGetRequest(r)
	}

	if r.Method == http.MethodPost {
		return parsePostRequest(r)
	}

	return nil, errors.New("unsupported HTTP method for GraphQL")
}

func parseGetRequest(r *http.Request) (*Request, error) {
	query := r.URL.Query().Get("query")
	if query == "" {
		return nil, errors.New("missing query parameter")
	}

	req := &Request{
		Query: query,
	}

	// Parse variables if present
	if vars := r.URL.Query().Get("variables"); vars != "" {
		var variables map[string]any
		if err := json.Unmarshal([]byte(vars), &variables); err != nil {
			return nil, errors.New("invalid variables parameter")
		}
		req.Variables = variables
	}

	// Parse operation name if present
	if op := r.URL.Query().Get("operationName"); op != "" {
		req.OperationName = op
	}

	return req, nil
}

func parsePostRequest(r *http.Request) (*Request, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	// application/graphql - body is the query
	if contentType == "application/graphql" {
		return &Request{
			Query: string(body),
		}, nil
	}

	// application/json - parse as GraphQL request
	if strings.HasPrefix(contentType, "application/json") {
		var req Request
		// M-019: Reject deeply nested JSON that could cause stack overflow during parsing.
		// Check maximum object/array nesting depth before unmarshaling.
		if err := checkJSONDepth(body, 32); err != nil {
			return nil, fmt.Errorf("JSON depth exceeds limit: %w", err)
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, errors.New("invalid GraphQL request JSON")
		}
		if req.Query == "" {
			return nil, errors.New("missing query field")
		}
		return &req, nil
	}

	return nil, errors.New("unsupported content type for GraphQL")
}

// checkJSONDepth validates that JSON body does not exceed maxNesting depth.
// Uses a simple counter approach without full parsing to avoid DoS during validation.
func checkJSONDepth(data []byte, maxDepth int) error {
	depth := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '{', '[':
			depth++
			if depth > maxDepth {
				return fmt.Errorf("nesting depth %d exceeds maximum %d", depth, maxDepth)
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return errors.New("mismatched brackets")
			}
		case '"':
			// Skip string content to avoid counting brackets inside strings
			i++
			for i < len(data) {
				if data[i] == '\\' {
					i += 2 // skip escaped character
					continue
				}
				if data[i] == '"' {
					break
				}
				i++
			}
		}
	}
	return nil
}

// WriteResponse writes a GraphQL response to the HTTP response writer.
func WriteResponse(w http.ResponseWriter, resp *Response, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp) // #nosec G104
}

// WriteError writes a GraphQL error response.
func WriteError(w http.ResponseWriter, message string, statusCode int) {
	resp := &Response{
		Errors: []GraphQLError{
			{Message: message},
		},
	}
	WriteResponse(w, resp, statusCode)
}

// IsIntrospectionQuery checks if a query contains introspection.
func IsIntrospectionQuery(query string) bool {
	// Quick string check for introspection fields
	return strings.Contains(query, "__schema") ||
		strings.Contains(query, "__type") ||
		strings.Contains(query, "__typename") ||
		strings.Contains(query, "__fields") ||
		strings.Contains(query, "__args")
}
