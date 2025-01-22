package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pb33f/libopenapi"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/loopwork-ai/emcee/jsonrpc"
)

const ModelContextProtocolVersion = "2024-11-05"

// ServerOption configures a Server
type ServerOption func(*Server) error

// WithAuth sets the HTTP Authorization header
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

// WithServerInfo sets server info
func WithServerInfo(name, version string) ServerOption {
	return func(s *Server) error {
		s.info = ServerInfo{
			Name:    name,
			Version: version,
		}
		return nil
	}
}

// WithSpecData sets the OpenAPI spec from a byte slice
func WithSpecData(data []byte) ServerOption {
	return func(s *Server) error {
		if len(data) == 0 {
			return fmt.Errorf("no OpenAPI spec data provided")
		}

		doc, err := libopenapi.NewDocument(data)
		if err != nil {
			return fmt.Errorf("error parsing OpenAPI spec: %v", err)
		}

		s.doc = doc
		model, errs := doc.BuildV3Model()
		if len(errs) > 0 {
			return fmt.Errorf("error building OpenAPI model: %v", errs[0])
		}

		s.model = &model.Model

		// Require server URL information
		if len(model.Model.Servers) == 0 || model.Model.Servers[0].URL == "" {
			return fmt.Errorf("OpenAPI spec must include at least one server URL")
		}
		s.baseURL = strings.TrimSuffix(model.Model.Servers[0].URL, "/")

		return nil
	}
}

// WithLogger sets the logger for the server
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) error {
		s.logger = logger
		return nil
	}
}

// Server represents an MCP server that processes JSON-RPC requests
type Server struct {
	doc        libopenapi.Document
	model      *v3.Document
	baseURL    string
	client     *http.Client
	info       ServerInfo
	authHeader string
	logger     *slog.Logger
}

// NewServer creates a new MCP server instance
func NewServer(opts ...ServerOption) (*Server, error) {
	s := &Server{
		client: http.DefaultClient,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	// Validate required fields
	if s.doc == nil {
		return nil, fmt.Errorf("OpenAPI spec URL is required")
	}

	if s.logger != nil {
		s.logger.Info("server initialized with OpenAPI spec")
	}

	return s, nil
}

// Handle processes a single JSON-RPC request and returns a response
func (s *Server) Handle(request jsonrpc.Request) jsonrpc.Response {
	if s.logger != nil {
		reqJSON, _ := json.MarshalIndent(request, "", "  ")
		s.logger.Info("incoming request",
			"request", string(reqJSON))
	}

	response := s.handleRequest(request)

	if s.logger != nil {
		respJSON, _ := json.MarshalIndent(response, "", "  ")
		s.logger.Info("outgoing response",
			"response", string(respJSON))
	}

	return response
}

func (s *Server) handleRequest(request jsonrpc.Request) jsonrpc.Response {
	if s.logger != nil && request.Method != "" {
		s.logger.Info("processing request",
			"method", request.Method)
	}

	switch request.Method {
	case "initialize":
		return s.handleInitialize(request)
	case "tools/list":
		return s.handleToolsList(request)
	case "tools/call":
		return s.handleToolsCall(request)
	default:
		if s.logger != nil {
			s.logger.Warn("unknown method requested",
				"method", request.Method)
		}
		return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil))
	}
}

func (s *Server) handleInitialize(request jsonrpc.Request) jsonrpc.Response {
	response := InitializeResponse{
		ProtocolVersion: ModelContextProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: struct {
				ListChanged bool `json:"listChanged"`
			}{
				ListChanged: false,
			},
		},
		ServerInfo: s.info,
	}
	return jsonrpc.NewResponse(request.ID, response, nil)
}

func (s *Server) handleToolsList(request jsonrpc.Request) jsonrpc.Response {
	if s.logger != nil {
		s.logger.Info("building tools list from OpenAPI spec")
	}

	model, err := s.doc.BuildV3Model()
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to build OpenAPI model",
				"error", err)
		}
		return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
	}

	tools := []Tool{}
	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		path := pair.Key()
		pathItem := pair.Value()
		if pathItem.Get != nil {
			tools = append(tools, createTool("GET", path, pathItem.Get))
		}
		if pathItem.Post != nil {
			tools = append(tools, createTool("POST", path, pathItem.Post))
		}
		if pathItem.Put != nil {
			tools = append(tools, createTool("PUT", path, pathItem.Put))
		}
		if pathItem.Delete != nil {
			tools = append(tools, createTool("DELETE", path, pathItem.Delete))
		}
		if pathItem.Patch != nil {
			tools = append(tools, createTool("PATCH", path, pathItem.Patch))
		}
	}

	if s.logger != nil {
		s.logger.Info("tools list built",
			"count", len(tools))
	}

	return jsonrpc.NewResponse(request.ID, ToolsListResponse{Tools: tools}, nil)
}

// applyAuthHeaders applies authentication headers to the request based on the server's auth configuration
func (s *Server) applyAuthHeaders(req *http.Request) {
	if s.authHeader != "" {
		req.Header.Set("Authorization", s.authHeader)
	}
}

func (s *Server) handleToolsCall(request jsonrpc.Request) jsonrpc.Response {
	var params ToolCallParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		if s.logger != nil {
			s.logger.Error("failed to unmarshal tool call params",
				"error", err)
		}
		return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrInvalidParams, err))
	}

	model, errs := s.doc.BuildV3Model()
	if errs != nil {
		if s.logger != nil {
			s.logger.Error("failed to build OpenAPI model",
				"error", errs)
		}
		return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrInternal, errs))
	}

	method, path, found := s.findOperation(&model.Model, params.Name)
	if !found {
		if s.logger != nil {
			s.logger.Warn("operation not found",
				"operation", params.Name)
		}
		return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil))
	}

	url := s.baseURL + path

	var body io.Reader
	if len(params.Arguments) > 0 && (method == "POST" || method == "PUT" || method == "PATCH") {
		jsonBody, err := json.Marshal(params.Arguments)
		if err != nil {
			if s.logger != nil {
				s.logger.Error("failed to marshal request body",
					"error", err)
			}
			return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
		}
		body = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to create HTTP request",
				"error", err)
		}
		return toolError(request.ID, fmt.Sprintf("Error making request: %v", err))
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	s.applyAuthHeaders(req)

	if s.logger != nil {
		s.logger.Info("making HTTP request",
			"method", method,
			"url", url,
			"has_body", body != nil)
		if body != nil {
			s.logger.Debug("request body",
				"body", params.Arguments)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("HTTP request error",
				"error", err)
		}
		return toolError(request.ID, fmt.Sprintf("Error making request: %v", err))
	}
	defer resp.Body.Close()

	if s.logger != nil {
		s.logger.Info("HTTP response status",
			"status", resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolError(request.ID, fmt.Sprintf("Error reading response: %v", err))
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "image/") {
		// For image responses, base64 encode the data
		if resp.StatusCode >= 400 {
			return toolError(request.ID, fmt.Sprintf("Image request failed with status %d", resp.StatusCode))
		}
		return toolSuccess(request.ID, NewImageContent(base64.StdEncoding.EncodeToString(respBody), contentType, []Role{RoleAssistant}, nil))
	}

	// Try to parse as JSON first
	var jsonResult interface{}
	if err := json.Unmarshal(respBody, &jsonResult); err != nil {
		// If not JSON, return as plain text
		return toolSuccess(request.ID, NewTextContent(string(respBody), []Role{RoleAssistant}, nil))
	}

	// For JSON responses, convert to string for better readability
	jsonStr, err := json.MarshalIndent(jsonResult, "", "  ")
	if err != nil {
		jsonStr = respBody
	}

	if resp.StatusCode >= 400 {
		return toolError(request.ID, string(jsonStr))
	}
	return toolSuccess(request.ID, NewTextContent(string(jsonStr), []Role{RoleAssistant}, nil))
}

// toolSuccess creates a successful tool response with the given content
func toolSuccess(id interface{}, content interface{}) jsonrpc.Response {
	return jsonrpc.NewResponse(id, CallToolResult{
		Content: []interface{}{content},
		IsError: false,
	}, nil)
}

// toolError creates an error tool response with the given message
func toolError(id interface{}, message string) jsonrpc.Response {
	return jsonrpc.NewResponse(id, CallToolResult{
		Content: []interface{}{
			NewTextContent(message, []Role{RoleAssistant}, nil),
		},
		IsError: true,
	}, nil)
}

func (s *Server) findOperation(model *v3.Document, operationId string) (method, path string, found bool) {
	for pair := model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathStr := pair.Key()
		pathItem := pair.Value()

		if pathItem.Get != nil && pathItem.Get.OperationId == operationId {
			return "GET", pathStr, true
		}
		if pathItem.Post != nil && pathItem.Post.OperationId == operationId {
			return "POST", pathStr, true
		}
		if pathItem.Put != nil && pathItem.Put.OperationId == operationId {
			return "PUT", pathStr, true
		}
		if pathItem.Delete != nil && pathItem.Delete.OperationId == operationId {
			return "DELETE", pathStr, true
		}
		if pathItem.Patch != nil && pathItem.Patch.OperationId == operationId {
			return "PATCH", pathStr, true
		}
	}
	return "", "", false
}

func createTool(method string, path string, operation *v3.Operation) Tool {
	name := operation.OperationId
	if name == "" {
		name = fmt.Sprintf("%s %s", method, path)
	}

	description := operation.Description
	if description == "" {
		description = operation.Summary
	}

	inputSchema := InputSchema{
		Type:       "object",
		Properties: make(map[string]interface{}),
	}

	if operation.RequestBody != nil && operation.RequestBody.Content != nil {
		if mediaType, ok := operation.RequestBody.Content.Get("application/json"); ok && mediaType != nil {
			if mediaType.Schema != nil {
				if schema := mediaType.Schema.Schema(); schema != nil {
					// Extract properties from the schema
					if schema.Properties != nil {
						for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
							propName := pair.Key()
							propSchema := pair.Value()
							if innerSchema := propSchema.Schema(); innerSchema != nil {
								schemaType := "object"
								if len(innerSchema.Type) > 0 {
									schemaType = innerSchema.Type[0]
								}
								inputSchema.Properties[propName] = map[string]interface{}{
									"type": schemaType,
								}
							}
						}
					}
					if schema.Required != nil {
						inputSchema.Required = schema.Required
					}
				}
			}
		}
	}

	return Tool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
	}
}
