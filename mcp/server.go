package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pb33f/libopenapi"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/loopwork-ai/openapi-mcp/jsonrpc"
)

// Server represents an MCP server that processes JSON-RPC requests
type Server struct {
	doc     libopenapi.Document
	baseURL string
	client  *http.Client
}

// NewServer creates a new MCP server instance
func NewServer(doc libopenapi.Document, baseURL string) *Server {
	return &Server{
		doc:     doc,
		baseURL: strings.TrimSuffix(baseURL, "/openapi.json"),
		client:  &http.Client{},
	}
}

// Handle processes a single JSON-RPC request and returns a response
func (s *Server) Handle(request jsonrpc.Request) jsonrpc.Response {
	switch request.Method {
	case "tools/list":
		return s.handleToolsList(request)
	case "tools/call":
		return s.handleToolsCall(request)
	default:
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrMethodNotFound, nil))
	}
}

func (s *Server) handleToolsList(request jsonrpc.Request) jsonrpc.Response {
	model, err := s.doc.BuildV3Model()
	if err != nil {
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
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

	return jsonrpc.NewResponse(request.Id, ToolsListResponse{Tools: tools}, nil)
}

func (s *Server) handleToolsCall(request jsonrpc.Request) jsonrpc.Response {
	var params ToolCallParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInvalidParams, err))
	}

	model, errs := s.doc.BuildV3Model()
	if errs != nil {
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInternal, errs))
	}

	method, path, found := s.findOperation(&model.Model, params.Name)
	if !found {
		parts := strings.SplitN(params.Name, " ", 2)
		if len(parts) != 2 {
			return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInvalidParams, "Invalid tool name format"))
		}
		method, path = parts[0], parts[1]
	}

	url := s.baseURL + path

	var body io.Reader
	if len(params.Parameters) > 0 && (method == "POST" || method == "PUT" || method == "PATCH") {
		jsonBody, err := json.Marshal(params.Parameters)
		if err != nil {
			return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
		}
		body = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return jsonrpc.NewResponse(request.Id, nil, jsonrpc.NewError(jsonrpc.ErrInternal, err))
	}

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		result = string(respBody)
	}

	return jsonrpc.NewResponse(request.Id, result, nil)
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

	parameters := make(map[string]interface{})
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
								parameters[propName] = map[string]interface{}{
									"type": schemaType,
								}
							}
						}
					}
				}
			}
		}
	}

	return Tool{
		Name:        name,
		Description: description,
		Parameters:  parameters,
	}
}
