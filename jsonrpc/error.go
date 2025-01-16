package jsonrpc

import (
	"fmt"
)

// ErrorCode represents a JSON-RPC error code
type ErrorCode int

// JSON-RPC 2.0 error codes as defined in https://www.jsonrpc.org/specification
const (
	// Parse error (-32700)
	// Invalid JSON was received by the server.
	// An error occurred on the server while parsing the JSON text.
	ErrParse ErrorCode = -32700

	// Invalid Request (-32600)
	// The JSON sent is not a valid Request object.
	ErrInvalidRequest ErrorCode = -32600

	// Method not found (-32601)
	// The method does not exist / is not available.
	ErrMethodNotFound ErrorCode = -32601

	// Invalid params (-32602)
	// Invalid method parameter(s).
	ErrInvalidParams ErrorCode = -32602

	// Internal error (-32603)
	// Internal JSON-RPC error.
	ErrInternal ErrorCode = -32603

	// Server error (-32000 to -32099)
	// Reserved for implementation-defined server-errors.
	ErrServer ErrorCode = -32000
)

// errorDetails maps error codes to their standard messages
var errorDetails = map[ErrorCode]string{
	ErrParse:          "Parse error",
	ErrInvalidRequest: "Invalid Request",
	ErrMethodNotFound: "Method not found",
	ErrInvalidParams:  "Invalid params",
	ErrInternal:       "Internal error",
	ErrServer:         "Server error",
}

// Error represents a JSON-RPC error object
type Error struct {
	Code    ErrorCode   `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

var _ error = &Error{}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// NewError creates a new JSON-RPC error with the given code and optional data
func NewError(code ErrorCode, data interface{}) *Error {
	msg, ok := errorDetails[code]
	if !ok {
		if code >= -32099 && code <= -32000 {
			msg = "Server error"
		} else {
			msg = "Unknown error"
		}
	}

	return &Error{
		Code:    code,
		Message: msg,
		Data:    data,
	}
}
