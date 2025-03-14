package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"

	"github.com/pb33f/libopenapi"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/loopwork-ai/emcee/internal"
	"github.com/loopwork-ai/emcee/jsonrpc"
)

// Server represents an MCP server that processes JSON-RPC requests
type Server struct {
	auth    string
	doc     libopenapi.Document
	model   *v3.Document
	baseURL string
	client  *http.Client
	info    ServerInfo
	logger  *slog.Logger
}

// ServerOption configures a Server
type ServerOption func(*Server) error

// WithAuth sets the authentication header for the server
func WithAuth(auth string) ServerOption {
	return func(s *Server) error {
		auth = strings.TrimSpace(auth)
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid auth header format: %s", auth)
		}
		s.auth = fmt.Sprintf("%s %s", parts[0], parts[1])
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

// WithLogger sets the logger for the server
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) error {
		s.logger = logger
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

// NewServer creates a new MCP server instance
func NewServer(opts ...ServerOption) (*Server, error) {
	s := &Server{
		client: &http.Client{
			Transport: http.DefaultTransport,
		},
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	// Apply custom transport to inject auth header, if provided
	if s.auth != "" {
		headers := http.Header{}
		headers.Add("Authorization", s.auth)

		s.client.Transport = &internal.HeaderTransport{
			Base:    s.client.Transport,
			Headers: headers,
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
		s.logger.Debug("incoming request",
			"request", string(reqJSON),
			"method", request.Method)
		s.logger.Info("handling request", "method", request.Method)
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
			s.logger.Warn("unknown method requested", "method", request.Method)
		}
		response = jsonrpc.NewResponse(request.ID, nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil))
	}

	if s.logger != nil {
		if response.Error != nil {
			s.logger.Error("request failed",
				"method", request.Method,
				"error", response.Error)
		}
		respJSON, _ := json.MarshalIndent(response, "", "  ")
		s.logger.Debug("outgoing response",
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

// Update the tools list generation to use the helper
func (s *Server) handleToolsList(request *ToolsListRequest) (*ToolsListResponse, error) {
	tools := []Tool{}
	if s.model.Paths == nil || s.model.Paths.PathItems == nil {
		if s.logger != nil {
			s.logger.Info("no tools found in OpenAPI spec")
		}
		return &ToolsListResponse{Tools: tools}, nil
	}

	toolCount := 0
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
			if s.logger != nil {
				s.logger.Debug("discovered tool",
					"operation_id", op.op.OperationId,
					"method", op.method,
					"description", op.op.Description)
			}
			toolCount++

			// Create input schema
			inputSchema := InputSchema{
				Type:       "object",
				Properties: make(map[string]interface{}),
				Required:   []string{},
			}

			// Add path parameters
			if pathItem.Parameters != nil {
				for _, param := range pathItem.Parameters {
					if param != nil && param.Schema != nil {
						schema := make(map[string]interface{})
						if paramSchema := param.Schema.Schema(); paramSchema != nil {
							schemaType := "string" // default to string if not specified
							if len(paramSchema.Type) > 0 {
								schemaType = paramSchema.Type[0]
							}
							schema["type"] = schemaType
							if paramSchema.Pattern != "" {
								schema["pattern"] = paramSchema.Pattern
							}
						}
						schema["description"] = param.Description
						inputSchema.Properties[param.Name] = schema
						if param.Required != nil && *param.Required {
							inputSchema.Required = append(inputSchema.Required, param.Name)
						}
					}
				}
			}

			// Add operation parameters
			if op.op.Parameters != nil {
				for _, param := range op.op.Parameters {
					if param != nil && param.Schema != nil {
						schema := make(map[string]interface{})
						if paramSchema := param.Schema.Schema(); paramSchema != nil {
							schemaType := "string" // default to string if not specified
							if len(paramSchema.Type) > 0 {
								schemaType = paramSchema.Type[0]
							}
							schema["type"] = schemaType
							if paramSchema.Pattern != "" {
								schema["pattern"] = paramSchema.Pattern
							}
						}
						schema["description"] = param.Description
						inputSchema.Properties[param.Name] = schema
						if param.Required != nil && *param.Required {
							inputSchema.Required = append(inputSchema.Required, param.Name)
						}
					}
				}
			}

			// Add request body if present
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
									}
								}
							}
							if schema.Required != nil {
								inputSchema.Required = append(inputSchema.Required, schema.Required...)
							}
						}
					}
				}
			}

			description := op.op.Description
			if description == "" {
				description = op.op.Summary
			}

			// Handle operation ID length with hash for uniqueness
			toolName := getToolName(op.op.OperationId)
			tools = append(tools, Tool{
				Name:        toolName,
				Description: description,
				InputSchema: inputSchema,
			})
		}
	}

	if s.logger != nil {
		s.logger.Info("tools discovery completed", "count", toolCount)
	}

	return &ToolsListResponse{Tools: tools}, nil
}

// Update the tools call handler to use the new finder
func (s *Server) handleToolsCall(request *ToolCallRequest) (*ToolCallResponse, error) {
	method, p, operation, pathItem, found := s.findOperationByToolName(request.Name)
	if !found {
		return nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil)
	}

	// Build URL from base URL and path
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}

	// Ensure the path starts with a slash
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	// Clean the path to handle multiple slashes
	p = path.Clean(p)

	// Create a new URL with the base URL's scheme and host
	u := &url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
	}

	// If the base URL has a path, join it with the operation path
	if baseURL.Path != "" {
		// Clean the base path
		basePath := path.Clean(baseURL.Path)
		// Join paths and ensure leading slash
		u.Path = "/" + strings.TrimPrefix(path.Join(basePath, p), "/")
	} else {
		u.Path = p
	}

	// Set default scheme if not present
	if u.Scheme == "" {
		u.Scheme = "http"
	}

	// Process parameters based on their location (path, query, header)
	queryParams := url.Values{}
	headerParams := make(http.Header)
	var bodyParams map[string]interface{}

	// Handle path item parameters first
	if pathItem.Parameters != nil {
		for _, param := range pathItem.Parameters {
			if param != nil {
				if value, ok := request.Arguments[param.Name]; ok {
					switch param.In {
					case "path":
						// Only escape characters that are invalid in URL path segments
						value := fmt.Sprint(value)
						u.Path = strings.ReplaceAll(u.Path, "{"+param.Name+"}", pathSegmentEscape(value))
					case "query":
						queryParams.Set(param.Name, fmt.Sprint(value))
					case "header":
						headerParams.Add(param.Name, fmt.Sprint(value))
					}
				}
			}
		}
	}

	// Handle operation parameters
	if operation.Parameters != nil {
		for _, param := range operation.Parameters {
			if param != nil {
				if value, ok := request.Arguments[param.Name]; ok {
					switch param.In {
					case "path":
						// Only escape characters that are invalid in URL path segments
						value := fmt.Sprint(value)
						u.Path = strings.ReplaceAll(u.Path, "{"+param.Name+"}", pathSegmentEscape(value))
					case "query":
						// Handle array values for query parameters
						switch v := value.(type) {
						case []interface{}:
							// Join array values with commas for parameters like tweet.fields
							values := make([]string, len(v))
							for i, item := range v {
								values[i] = fmt.Sprint(item)
							}
							queryParams.Set(param.Name, strings.Join(values, ","))
						default:
							queryParams.Set(param.Name, fmt.Sprint(value))
						}
					case "header":
						headerParams.Add(param.Name, fmt.Sprint(value))
					}
				}
			}
		}
	}

	// Handle request body
	if operation.RequestBody != nil && operation.RequestBody.Content != nil {
		if mediaType, ok := operation.RequestBody.Content.Get("application/json"); ok && mediaType != nil {
			if mediaType.Schema != nil && mediaType.Schema.Schema() != nil {
				schema := mediaType.Schema.Schema()
				if schema.Properties != nil {
					bodyParams = make(map[string]interface{})
					for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
						propName := pair.Key()
						if value, ok := request.Arguments[propName]; ok {
							bodyParams[propName] = value
						}
					}
				}
			}
		}
	}

	// Add query parameters to URL
	if len(queryParams) > 0 {
		u.RawQuery = queryParams.Encode()
	}

	// Create and send request
	var reqBody io.Reader
	if len(bodyParams) > 0 {
		jsonBody, err := json.Marshal(bodyParams)
		if err != nil {
			return nil, jsonrpc.NewError(jsonrpc.ErrInvalidParams, err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
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

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, err)
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		textContent := NewTextContent(fmt.Sprintf("Request failed with status %d: %s", resp.StatusCode, string(body)), []Role{RoleAssistant}, nil)
		return nil, jsonrpc.NewError(jsonrpc.ErrInternal, textContent)
	}

	// Process response based on content type
	contentType := resp.Header.Get("Content-Type")
	var content Content

	// Create content based on response content type
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

// pathSegmentEscape escapes invalid URL path segment characters according to RFC 3986.
// It preserves valid path characters including comma, colon, and @ sign.
func pathSegmentEscape(s string) string {
	// RFC 3986 section 3.3 defines path segment characters:
	// segment       = *pchar
	// pchar         = unreserved / pct-encoded / sub-delims / ":" / "@"
	// unreserved    = ALPHA / DIGIT / "-" / "." / "_" / "~"
	// sub-delims    = "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / ";" / "="
	hexCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			hexCount++
		}
	}

	if hexCount == 0 {
		return s
	}

	var buf [3]byte
	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			buf[0] = '%'
			buf[1] = "0123456789ABCDEF"[c>>4]
			buf[2] = "0123456789ABCDEF"[c&15]
			t[j] = buf[0]
			t[j+1] = buf[1]
			t[j+2] = buf[2]
			j += 3
		} else {
			t[j] = c
			j++
		}
	}
	return string(t)
}

// shouldEscape reports whether the byte c should be escaped.
// It follows RFC 3986 section 3.3 path segment rules.
func shouldEscape(c byte) bool {
	// RFC 3986 section 2.3 Unreserved Characters (and some sub-delims)
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return false
	}
	switch c {
	case '-', '.', '_', '~': // unreserved
		return false
	case '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=', ':', '@': // sub-delims + ':' + '@'
		return false
	}
	return true
}

// getToolName creates a unique tool name from an operation ID, ensuring it's within the 64-character limit
// while maintaining a bijective mapping between operation IDs and tool names
func getToolName(operationId string) string {
	if len(operationId) <= 64 {
		return operationId
	}
	// Generate a short hash of the full operation ID
	hash := sha256.Sum256([]byte(operationId))
	// Use base64 encoding for shorter hash representation (first 8 chars)
	shortHash := base64.RawURLEncoding.EncodeToString(hash[:])[:8]
	// Create a deterministic name that fits within limits while preserving uniqueness
	return operationId[:55] + "_" + shortHash
}

// findOperationByToolName maps a tool name back to its corresponding OpenAPI operation
func (s *Server) findOperationByToolName(toolName string) (method, path string, operation *v3.Operation, pathItem *v3.PathItem, found bool) {
	if s.model.Paths == nil || s.model.Paths.PathItems == nil {
		return "", "", nil, nil, false
	}

	for pair := s.model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathStr := pair.Key()
		item := pair.Value()

		operations := []struct {
			method string
			op     *v3.Operation
		}{
			{"GET", item.Get},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"DELETE", item.Delete},
			{"PATCH", item.Patch},
		}

		for _, op := range operations {
			if op.op != nil && op.op.OperationId != "" {
				if getToolName(op.op.OperationId) == toolName {
					return op.method, pathStr, op.op, item, true
				}
			}
		}
	}
	return "", "", nil, nil, false
}
