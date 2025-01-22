package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/pb33f/libopenapi"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/loopwork-ai/emcee/jsonrpc"
)

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

// HandleRequest processes a single JSON-RPC request and returns a response
func (s *Server) HandleRequest(request jsonrpc.Request) jsonrpc.Response {
	if s.logger != nil {
		reqJSON, _ := json.MarshalIndent(request, "", "  ")
		s.logger.Info("incoming request",
			"request", string(reqJSON))
	}

	if s.logger != nil && request.Method != "" {
		s.logger.Info("processing request",
			"method", request.Method)
	}

	var response jsonrpc.Response
	switch request.Method {
	case "initialize":
		response = handleMethod(request, s.handleInitialize)
	case "tools/list":
		response = handleMethod(request, s.handleToolsList)
	case "tools/call":
		response = handleMethod(request, s.handleToolsCall)
	case "ping/ping":
		response = handleMethod(request, s.handlePing)
	default:
		if s.logger != nil {
			s.logger.Warn("unknown method requested",
				"method", request.Method)
		}
		response = jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil))
	}

	if s.logger != nil {
		respJSON, _ := json.MarshalIndent(response, "", "  ")
		s.logger.Info("outgoing response",
			"response", string(respJSON))
	}

	return response
}

// handleMethod is a helper to unmarshal params and call a handler with proper error handling
func handleMethod[Req, Resp any](request jsonrpc.Request, handler func(*Req) (*Resp, error)) jsonrpc.Response {
	var req Req
	if request.Params != nil {
		if err := json.Unmarshal(request.Params, &req); err != nil {
			return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrInvalidParams, err))
		}
	}
	resp, err := handler(&req)
	if err != nil {
		if rpcErr, ok := err.(*jsonrpc.Error); ok {
			return jsonrpc.NewResponse(request.ID, nil, rpcErr)
		}
		return jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
	}

	// Convert response to interface{} to ensure proper JSON serialization
	var result interface{} = resp
	if resp != nil {
		// If it's a pointer, get the underlying value
		if rv := reflect.ValueOf(resp); rv.Kind() == reflect.Ptr && !rv.IsNil() {
			result = rv.Elem().Interface()
		}
	}

	return jsonrpc.NewResponse(request.ID, result, nil)
}

func (s *Server) handleInitialize(request *InitializeRequest) (*InitializeResponse, error) {
	response := &InitializeResponse{
		ProtocolVersion: Version,
		Capabilities: ServerCapabilities{
			Tools: &struct {
				ListChanged bool `json:"listChanged"`
			}{
				ListChanged: false,
			},
		},
		ServerInfo: s.info,
	}
	return response, nil
}

func (s *Server) handleToolsList(request *ToolsListRequest) (*ToolsListResponse, error) {
	if s.logger != nil {
		s.logger.Info("building tools list from OpenAPI spec")
	}

	tools := []Tool{}
	if s.model.Paths == nil || s.model.Paths.PathItems == nil {
		return &ToolsListResponse{Tools: tools}, nil
	}

	// Iterate through paths and operations
	for pair := s.model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathItem := pair.Value()

		// Process each operation type
		operations := []struct {
			method string
			op     *v3.Operation
		}{
			{"GET", pathItem.Get},
			{"POST", pathItem.Post},
			{"PUT", pathItem.Put},
			{"DELETE", pathItem.Delete},
			{"PATCH", pathItem.Patch},
		}

		for _, op := range operations {
			if op.op == nil || op.op.OperationId == "" {
				continue
			}

			// Create input schema
			inputSchema := InputSchema{
				Type:       "object",
				Properties: make(map[string]interface{}),
			}

			// Add parameters to schema
			if op.op.Parameters != nil {
				for _, param := range op.op.Parameters {
					if param.Schema != nil && param.Schema.Schema() != nil {
						schema := param.Schema.Schema()
						schemaType := "string"
						if len(schema.Type) > 0 {
							schemaType = schema.Type[0]
						}
						inputSchema.Properties[param.Name] = map[string]interface{}{
							"type":        schemaType,
							"description": param.Description,
							"in":          param.In,
						}
					}
				}
			}

			// Add request body to schema if it exists
			if op.op.RequestBody != nil && op.op.RequestBody.Content != nil {
				if mediaType, ok := op.op.RequestBody.Content.Get("application/json"); ok && mediaType != nil {
					if mediaType.Schema != nil && mediaType.Schema.Schema() != nil {
						schema := mediaType.Schema.Schema()
						if schema.Properties != nil {
							for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
								propName := pair.Key()
								propSchema := pair.Value().Schema()
								if propSchema != nil {
									schemaType := "string"
									if len(propSchema.Type) > 0 {
										schemaType = propSchema.Type[0]
									}
									inputSchema.Properties[propName] = map[string]interface{}{
										"type":        schemaType,
										"description": propSchema.Description,
										"in":          "body",
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

			description := op.op.Description
			if description == "" {
				description = op.op.Summary
			}

			tools = append(tools, Tool{
				Name:        op.op.OperationId,
				Description: description,
				InputSchema: inputSchema,
			})
		}
	}

	if s.logger != nil {
		s.logger.Info("tools list built", "count", len(tools))
	}

	return &ToolsListResponse{Tools: tools}, nil
}

func (s *Server) handleToolsCall(request *ToolCallRequest) (*ToolCallResponse, error) {
	method, path, operation, found := s.findOperation(s.model, request.Name)
	if !found {
		if s.logger != nil {
			s.logger.Warn("operation not found", "operation", request.Name)
		}
		return nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil)
	}

	// Build URL from base URL and path
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to parse base URL", "error", err)
		}
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}

	u := url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
		Path:   path,
	}

	if baseURL.Scheme == "" {
		u.Scheme = "http"
	}

	// Process parameters based on their location (path, query, header)
	queryParams := url.Values{}
	headerParams := make(http.Header)
	var bodyParams map[string]interface{}

	// Handle parameters
	if operation.Parameters != nil {
		for _, param := range operation.Parameters {
			if value, ok := request.Arguments[param.Name]; ok {
				switch param.In {
				case "path":
					u.Path = strings.ReplaceAll(u.Path, "{"+param.Name+"}", fmt.Sprint(value))
				case "query":
					queryParams.Set(param.Name, fmt.Sprint(value))
				case "header":
					headerParams.Add(param.Name, fmt.Sprint(value))
				}
			}
		}
	}

	// Handle request body
	var reqBody io.Reader
	if operation.RequestBody != nil && operation.RequestBody.Content != nil {
		if mediaType, ok := operation.RequestBody.Content.Get("application/json"); ok && mediaType != nil {
			bodyParams = make(map[string]interface{})
			if mediaType.Schema != nil && mediaType.Schema.Schema() != nil {
				schema := mediaType.Schema.Schema()
				if schema.Properties != nil {
					for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
						propName := pair.Key()
						if value, ok := request.Arguments[propName]; ok {
							bodyParams[propName] = value
						}
					}
				}
			}
			if len(bodyParams) > 0 {
				jsonBody, err := json.Marshal(bodyParams)
				if err != nil {
					if s.logger != nil {
						s.logger.Error("failed to marshal request body", "error", err)
					}
					return nil, jsonrpc.NewError(jsonrpc.ErrInvalidParams, err)
				}
				reqBody = bytes.NewReader(jsonBody)
			}
		}
	}

	// Add query parameters to URL
	if len(queryParams) > 0 {
		u.RawQuery = queryParams.Encode()
	}

	// Create and send request
	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to create request", "error", err)
		}
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}

	// Add headers
	for key, values := range headerParams {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.authHeader != "" {
		req.Header.Set("Authorization", s.authHeader)
	}

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to send request", "error", err)
		}
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to read response body", "error", err)
		}
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		if s.logger != nil {
			s.logger.Error("request failed",
				"status", resp.StatusCode,
				"body", string(body))
		}
		textContent := NewTextContent(fmt.Sprintf("Request failed with status %d: %s", resp.StatusCode, string(body)), []Role{RoleAssistant}, nil)
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, textContent)
	}

	// Process response based on content type
	contentType := resp.Header.Get("Content-Type")
	var content Content

	if strings.HasPrefix(contentType, "image/") {
		encoded := base64.StdEncoding.EncodeToString(body)
		content = NewImageContent(encoded, contentType, []Role{RoleAssistant}, nil)
	} else if strings.Contains(contentType, "application/json") {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, body, "", "  "); err == nil {
			body = prettyJSON.Bytes()
		}
		content = NewTextContent(string(body), []Role{RoleAssistant}, nil)
	} else {
		content = NewTextContent(string(body), []Role{RoleAssistant}, nil)
	}

	return &ToolCallResponse{
		Content: []Content{content},
		IsError: false,
	}, nil
}

func (s *Server) handlePing(request *PingRequest) (*PingResponse, error) {
	return &PingResponse{}, nil
}

func (s *Server) findOperation(model *v3.Document, operationId string) (method, path string, operation *v3.Operation, found bool) {
	if model.Paths == nil || model.Paths.PathItems == nil {
		return "", "", nil, false
	}

	for pair := model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathStr := pair.Key()
		pathItem := pair.Value()

		operations := []struct {
			method string
			op     *v3.Operation
		}{
			{"GET", pathItem.Get},
			{"POST", pathItem.Post},
			{"PUT", pathItem.Put},
			{"DELETE", pathItem.Delete},
			{"PATCH", pathItem.Patch},
		}

		for _, op := range operations {
			if op.op != nil && op.op.OperationId == operationId {
				return op.method, pathStr, op.op, true
			}
		}
	}
	return "", "", nil, false
}
