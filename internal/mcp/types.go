package mcp

import "encoding/json"

// JSONRPCRequest represents a JSON-RPC 2.0 request envelope.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response envelope.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	jsonRPCVersion = "2.0"
)

type toolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type resourceDefinition struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

func successResponse(id any, result any) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}
}

func errorResponse(id any, code int, message string, data any) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}
