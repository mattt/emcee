package jsonrpc

import "encoding/json"

// Request represents a JSON-RPC request object
type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Id      interface{}     `json:"id"`
}

// NewRequest creates a new Request object
func NewRequest(method string, params json.RawMessage, id interface{}) Request {
	return Request{
		Version: Version,
		Method:  method,
		Params:  params,
		Id:      id,
	}
}
