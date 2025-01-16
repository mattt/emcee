package main

// ToolsListResponse represents the response for the tools/list method
type ToolsListResponse struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a single tool in the tools/list response
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
