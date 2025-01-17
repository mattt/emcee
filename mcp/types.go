package mcp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pb33f/libopenapi"
)

// ServerInfo represents information about the server implementation
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities represents the server's supported capabilities
type ServerCapabilities struct {
	Tools struct {
		ListChanged bool `json:"listChanged"`
	} `json:"tools"`
}

// InitializeResponse represents the server's response to an initialize request
type InitializeResponse struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

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

// CallToolResult represents the server's response to a tool call
type CallToolResult struct {
	Content []interface{} `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// NewTextContent creates a new TextContent with the given text and optional annotations
func NewTextContent(text string, audience []Role, priority *float64) TextContent {
	return TextContent{
		Content: Content{
			Type: "text",
			Annotated: Annotated{
				Annotations: &Annotations{
					Audience: audience,
					Priority: priority,
				},
			},
		},
		Text: text,
	}
}

// NewImageContent creates a new ImageContent with the given data and optional annotations
func NewImageContent(data string, mimeType string, audience []Role, priority *float64) ImageContent {
	return ImageContent{
		Content: Content{
			Type: "image",
			Annotated: Annotated{
				Annotations: &Annotations{
					Audience: audience,
					Priority: priority,
				},
			},
		},
		Data:     data,
		MimeType: mimeType,
	}
}

// ServerOption configures a Server
type ServerOption func(*Server) error

// WithSpecURL sets the OpenAPI spec URL and downloads the spec
func WithSpecURL(url string) ServerOption {
	return func(s *Server) error {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("error downloading spec: %w", err)
		}
		defer resp.Body.Close()

		specData, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading spec: %w", err)
		}

		doc, err := libopenapi.NewDocument(specData)
		if err != nil {
			return fmt.Errorf("error parsing spec: %w", err)
		}

		model, errs := doc.BuildV3Model()
		if errs != nil {
			return fmt.Errorf("error building model: %w", errors.Join(errs...))
		}

		s.doc = doc
		s.model = &model.Model
		s.baseURL = strings.TrimSuffix(url, "/openapi.json")

		// Apply model info if available
		if model.Model.Info != nil {
			if model.Model.Info.Title != "" {
				s.info.Name = model.Model.Info.Title
			}
			if model.Model.Info.Version != "" {
				s.info.Version = model.Model.Info.Version
			}
		}

		return nil
	}
}

// WithAuth sets the authorization header
func WithAuth(auth string) ServerOption {
	return func(s *Server) error {
		s.authHeader = auth
		return nil
	}
}

// WithClient sets the HTTP client
func WithClient(client *http.Client) ServerOption {
	return func(s *Server) error {
		s.client = client
		return nil
	}
}

// WithServerInfo sets custom server info
func WithServerInfo(name, version string) ServerOption {
	return func(s *Server) error {
		s.info = ServerInfo{
			Name:    name,
			Version: version,
		}
		return nil
	}
}
