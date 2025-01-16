package main

import "encoding/json"

const JsonRpcVersion = "2.0"

// JsonRpcErrorCode represents a JSON-RPC error code
type JsonRpcErrorCode int

// JSON-RPC 2.0 error codes as defined in https://www.jsonrpc.org/specification
const (
	// Parse error (-32700)
	// Invalid JSON was received by the server.
	// An error occurred on the server while parsing the JSON text.
	ErrParse JsonRpcErrorCode = -32700

	// Invalid Request (-32600)
	// The JSON sent is not a valid Request object.
	ErrInvalidRequest JsonRpcErrorCode = -32600

	// Method not found (-32601)
	// The method does not exist / is not available.
	ErrMethodNotFound JsonRpcErrorCode = -32601

	// Invalid params (-32602)
	// Invalid method parameter(s).
	ErrInvalidParams JsonRpcErrorCode = -32602

	// Internal error (-32603)
	// Internal JSON-RPC error.
	ErrInternal JsonRpcErrorCode = -32603

	// Server error (-32000 to -32099)
	// Reserved for implementation-defined server-errors.
	ErrServer JsonRpcErrorCode = -32000
)

// ErrorDetails maps error codes to their standard messages
var ErrorDetails = map[JsonRpcErrorCode]string{
	ErrParse:          "Parse error",
	ErrInvalidRequest: "Invalid Request",
	ErrMethodNotFound: "Method not found",
	ErrInvalidParams:  "Invalid params",
	ErrInternal:       "Internal error",
	ErrServer:         "Server error",
}

// RequestHandler defines the interface for handling JSON-RPC requests
type RequestHandler interface {
	HandleRequest(request JsonRpcRequest) JsonRpcResponse
}

// JsonRpcRequest represents a JSON-RPC request object
type JsonRpcRequest struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Id      interface{}     `json:"id"`
}

// JsonRpcResponse represents a JSON-RPC response object
type JsonRpcResponse struct {
	JsonRpc string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JsonRpcError `json:"error,omitempty"`
	Id      interface{}   `json:"id"`
}

// JsonRpcError represents a JSON-RPC error object
type JsonRpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewJsonRpcError creates a new JsonRpcError with the given code and optional data
func NewJsonRpcError(code JsonRpcErrorCode, data interface{}) *JsonRpcError {
	msg, ok := ErrorDetails[code]
	if !ok {
		if code >= -32099 && code <= -32000 {
			msg = "Server error"
		} else {
			msg = "Unknown error"
		}
	}

	return &JsonRpcError{
		Code:    int(code), // Convert back to int for JSON marshaling
		Message: msg,
		Data:    data,
	}
}
