package jsonrpc

import "encoding/json"

// Request represents a JSON-RPC request object
type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      ID              `json:"id"`
}

// NewRequest creates a new Request object
func NewRequest(method string, params json.RawMessage, id interface{}) Request {
	reqID, _ := NewID(id)

	return Request{
		Version: Version,
		Method:  method,
		Params:  params,
		ID:      reqID,
	}
}
