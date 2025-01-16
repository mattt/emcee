package main

import "encoding/json"

// JsonRpcHandler defines the interface for handling JSON-RPC requests
type JsonRpcHandler interface {
	HandleRequest(request JsonRpcRequest) JsonRpcResponse
}

// JsonRpcRequest represents a JSON-RPC request
type JsonRpcRequest struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Id      interface{}     `json:"id"`
}

// JsonRpcResponse represents a JSON-RPC response
type JsonRpcResponse struct {
	JsonRpc string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JsonRpcError `json:"error,omitempty"`
	Id      interface{}   `json:"id"`
}

// JsonRpcError represents a JSON-RPC error
type JsonRpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolsListResponse represents a response to the tools/list method
type ToolsListResponse struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a callable tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolCallParams represents the parameters for the tools/call method
type ToolCallParams struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}
