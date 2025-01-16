package mcp

// ToolsListResponse represents the response for the tools/list method
type ToolsListResponse struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a single tool in the tools/list response
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema represents the JSON Schema for tool parameters
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// ToolCallParams represents the parameters for the tools/call method
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// Role represents the sender or recipient of messages and data in a conversation
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Annotations represents optional annotations for objects
type Annotations struct {
	// Describes who the intended customer of this object or data is
	Audience []Role `json:"audience,omitempty"`
	// Describes how important this data is for operating the server (0-1)
	Priority *float64 `json:"priority,omitempty"`
}

// Annotated represents objects that can have annotations
type Annotated struct {
	Annotations *Annotations `json:"annotations,omitempty"`
}

// Content represents the base content type
type Content struct {
	Annotated
	Type string `json:"type"`
}

// TextContent represents text provided to or from an LLM
type TextContent struct {
	Content
	Text string `json:"text"`
}

// ImageContent represents an image provided to or from an LLM
type ImageContent struct {
	Content
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// ResourceContents represents the contents of a specific resource or sub-resource
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
}

// EmbeddedResource represents the contents of a resource embedded into a prompt or tool call result
type EmbeddedResource struct {
	Content
	Resource ResourceContents `json:"resource"`
}

// CallToolResult represents the server's response to a tool call
type CallToolResult struct {
	Content []interface{} `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
