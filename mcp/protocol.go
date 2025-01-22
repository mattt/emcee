package mcp

// Version is the Model Context Protocol version
const Version = "2024-11-05"

// Role represents the sender or recipient of messages and data in a conversation
type Role string

const (
	// RoleUser represents the user
	RoleUser Role = "user"

	// RoleAssistant represents the assistant
	RoleAssistant Role = "assistant"
)

// Content types
type (
	// Annotations represents optional annotations for objects
	Annotations struct {
		// Describes who the intended customer of this object or data is
		Audience []Role `json:"audience,omitempty"`
		// Describes how important this data is for operating the server (0-1)
		Priority *float64 `json:"priority,omitempty"`
	}

	// Content represents the base content type
	Content struct {
		Type        string       `json:"type"`
		Text        string       `json:"text,omitempty"`
		Data        string       `json:"data,omitempty"`
		MimeType    string       `json:"mimeType,omitempty"`
		Annotations *Annotations `json:"annotations,omitempty"`
	}
)

// NewTextContent creates a new TextContent with the given text and optional annotations
func NewTextContent(text string, audience []Role, priority *float64) Content {
	return Content{
		Type: "text",
		Text: text,
		Annotations: &Annotations{
			Audience: audience,
			Priority: priority,
		},
	}
}

// NewImageContent creates a new ImageContent with the given data and optional annotations
func NewImageContent(data string, mimeType string, audience []Role, priority *float64) Content {
	return Content{
		Type:     "image",
		Data:     data,
		MimeType: mimeType,
		Annotations: &Annotations{
			Audience: audience,
			Priority: priority,
		},
	}
}

// Initialize
type (
	// ServerCapabilities represents the server's supported capabilities
	ServerCapabilities struct {
		Experimental map[string]interface{} `json:"experimental,omitempty"`
		Logging      *struct{}              `json:"logging,omitempty"`
		Prompts      *struct {
			ListChanged bool `json:"listChanged"`
		} `json:"prompts,omitempty"`
		Resources *struct {
			Subscribe   bool `json:"subscribe"`
			ListChanged bool `json:"listChanged"`
		} `json:"resources,omitempty"`
		Tools *struct {
			ListChanged bool `json:"listChanged"`
		} `json:"tools,omitempty"`
	}

	// ServerInfo represents information about an MCP implementation
	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	// InitializeRequest represents a request to initialize the server
	InitializeRequest struct{}

	// InitializeResponse represents the server's response to an initialize request
	InitializeResponse struct {
		ProtocolVersion string             `json:"protocolVersion"`
		Capabilities    ServerCapabilities `json:"capabilities"`
		ServerInfo      ServerInfo         `json:"serverInfo"`
		Instructions    string             `json:"instructions,omitempty"`
	}

	// InitializedNotification represents a notification that initialization is complete
	InitializedNotification struct{}
)

// Resources
type (
	// Resource represents a known resource that the server can read
	Resource struct {
		URI         string                 `json:"uri"`
		Name        string                 `json:"name"`
		Description string                 `json:"description,omitempty"`
		MimeType    string                 `json:"mimeType,omitempty"`
		Size        int64                  `json:"size,omitempty"`
		Annotations map[string]interface{} `json:"annotations,omitempty"`
	}

	// ResourceContents represents the contents of a specific resource
	ResourceContents struct {
		URI      string `json:"uri"`
		MimeType string `json:"mimeType,omitempty"`
		Text     string `json:"text,omitempty"`
		Blob     string `json:"blob,omitempty"`
	}

	// ResourceTemplate represents a template for resources
	ResourceTemplate struct {
		URITemplate string                 `json:"uriTemplate"`
		Name        string                 `json:"name"`
		Description string                 `json:"description,omitempty"`
		MimeType    string                 `json:"mimeType,omitempty"`
		Annotations map[string]interface{} `json:"annotations,omitempty"`
	}

	// ListResourcesRequest represents a request to list available resources
	ListResourcesRequest struct {
		Cursor string `json:"cursor,omitempty"`
	}

	// ListResourcesResponse represents the response for resources/list
	ListResourcesResponse struct {
		Resources  []Resource `json:"resources"`
		NextCursor string     `json:"nextCursor,omitempty"`
	}

	// ListResourceTemplatesRequest represents a request to list resource templates
	ListResourceTemplatesRequest struct {
		Cursor string `json:"cursor,omitempty"`
	}

	// ListResourceTemplatesResponse represents the response for resources/templates/list
	ListResourceTemplatesResponse struct {
		ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
		NextCursor        string             `json:"nextCursor,omitempty"`
	}

	// ReadResourceRequest represents a request to read a resource
	ReadResourceRequest struct {
		URI string `json:"uri"`
	}

	// ReadResourceResponse represents the response for resources/read
	ReadResourceResponse struct {
		Contents []ResourceContents `json:"contents"`
	}

	// SubscribeRequest represents a request to subscribe to resource updates
	SubscribeRequest struct {
		URI string `json:"uri"`
	}

	// UnsubscribeRequest represents a request to unsubscribe from resource updates
	UnsubscribeRequest struct {
		URI string `json:"uri"`
	}

	// ResourceListChangedNotification represents a notification that the resource list has changed
	ResourceListChangedNotification struct{}

	// ResourceUpdatedNotification represents a notification that a resource has been updated
	ResourceUpdatedNotification struct {
		URI string `json:"uri"`
	}
)

// Prompts
type (
	// Prompt represents a prompt or prompt template
	Prompt struct {
		Name        string           `json:"name"`
		Description string           `json:"description,omitempty"`
		Arguments   []PromptArgument `json:"arguments,omitempty"`
	}

	// PromptArgument represents an argument for a prompt
	PromptArgument struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Required    bool   `json:"required,omitempty"`
	}

	// PromptMessage represents a message in a prompt
	PromptMessage struct {
		Role    Role    `json:"role"`
		Content Content `json:"content"`
	}

	// ListPromptsRequest represents a request to list available prompts
	ListPromptsRequest struct {
		Cursor string `json:"cursor,omitempty"`
	}

	// ListPromptsResponse represents the response for prompts/list
	ListPromptsResponse struct {
		Prompts    []Prompt `json:"prompts"`
		NextCursor string   `json:"nextCursor,omitempty"`
	}

	// GetPromptRequest represents a request to get a specific prompt
	GetPromptRequest struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}

	// GetPromptResponse represents the response for prompts/get
	GetPromptResponse struct {
		Description string          `json:"description,omitempty"`
		Messages    []PromptMessage `json:"messages"`
	}

	// PromptListChangedNotification represents a notification that the prompt list has changed
	PromptListChangedNotification struct{}
)

// Tools
type (
	// Tool represents a single tool in the tools/list response
	Tool struct {
		Name        string      `json:"name"`
		Description string      `json:"description,omitempty"`
		InputSchema InputSchema `json:"inputSchema"`
	}

	// InputSchema represents the JSON Schema for tool parameters
	InputSchema struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties,omitempty"`
		Required   []string               `json:"required,omitempty"`
	}

	// ToolsListRequest represents a request to list available tools
	ToolsListRequest struct {
		Cursor string `json:"cursor,omitempty"`
	}

	// ToolsListResponse represents the response for the tools/list method
	ToolsListResponse struct {
		Tools      []Tool `json:"tools"`
		NextCursor string `json:"nextCursor,omitempty"`
	}

	// ToolCallRequest represents a request to call a specific tool
	ToolCallRequest struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}

	// ToolCallResponse represents the response from a tool call
	ToolCallResponse struct {
		Content []Content `json:"content"`
		IsError bool      `json:"isError,omitempty"`
	}

	// ToolsChangedNotification represents a notification that the tools list has changed
	ToolsChangedNotification struct{}
)

// Sampling-related types
type (
	// SamplingMessage represents a message for LLM sampling
	SamplingMessage struct {
		Role    Role    `json:"role"`
		Content Content `json:"content"`
	}

	// ModelPreferences represents preferences for model selection
	ModelPreferences struct {
		Hints                []ModelHint `json:"hints,omitempty"`
		CostPriority         float64     `json:"costPriority,omitempty"`
		SpeedPriority        float64     `json:"speedPriority,omitempty"`
		IntelligencePriority float64     `json:"intelligencePriority,omitempty"`
	}

	// ModelHint represents hints for model selection
	ModelHint struct {
		Name string `json:"name,omitempty"`
	}

	// CreateMessageRequest represents a request to create a message via sampling
	CreateMessageRequest struct {
		Messages         []SamplingMessage      `json:"messages"`
		ModelPreferences *ModelPreferences      `json:"modelPreferences,omitempty"`
		SystemPrompt     string                 `json:"systemPrompt,omitempty"`
		IncludeContext   string                 `json:"includeContext,omitempty"`
		Temperature      float64                `json:"temperature,omitempty"`
		MaxTokens        int                    `json:"maxTokens"`
		StopSequences    []string               `json:"stopSequences,omitempty"`
		Metadata         map[string]interface{} `json:"metadata,omitempty"`
	}

	// CreateMessageResponse represents the response for sampling/createMessage
	CreateMessageResponse struct {
		Role       Role    `json:"role"`
		Content    Content `json:"content"`
		Model      string  `json:"model"`
		StopReason string  `json:"stopReason,omitempty"`
	}
)

// Completions
type (
	// CompleteRequest represents a request for completion options
	CompleteRequest struct {
		Ref      interface{} `json:"ref"`
		Argument struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"argument"`
	}

	// CompleteResponse represents the response for completion/complete
	CompleteResponse struct {
		Completion struct {
			Values  []string `json:"values"`
			Total   int      `json:"total,omitempty"`
			HasMore bool     `json:"hasMore,omitempty"`
		} `json:"completion"`
	}
)

// Roots
type (
	// Root represents a root directory or file
	Root struct {
		URI  string `json:"uri"`
		Name string `json:"name,omitempty"`
	}

	// ListRootsRequest represents a request to list root directories
	ListRootsRequest struct{}

	// ListRootsResponse represents the response for roots/list
	ListRootsResponse struct {
		Roots []Root `json:"roots"`
	}

	// RootsListChangedNotification represents a notification that the roots list has changed
	RootsListChangedNotification struct{}
)

// Logging
type (
	// SetLevelRequest represents a request to set logging level
	SetLevelRequest struct {
		Level string `json:"level"`
	}

	// LogNotification represents a log message from the server
	LogNotification struct {
		Level  string      `json:"level"`
		Logger string      `json:"logger,omitempty"`
		Data   interface{} `json:"data"`
	}
)

// Progress
type (
	// ProgressNotification represents a progress update for a long-running request
	ProgressNotification struct {
		ProgressToken string  `json:"progressToken"`
		Progress      float64 `json:"progress"`
		Total         float64 `json:"total,omitempty"`
	}
)

// Ping
type (
	// PingRequest represents a ping request
	PingRequest struct{}

	// PingResponse represents the response for ping/ping
	PingResponse struct{}
)
