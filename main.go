package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

type JsonRpcRequest struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Id      interface{}     `json:"id"`
}

type JsonRpcResponse struct {
	JsonRpc string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JsonRpcError `json:"error,omitempty"`
	Id      interface{}   `json:"id"`
}

type JsonRpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type ToolsListResponse struct {
	Tools []Tool `json:"tools"`
}

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type ToolCallParams struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <openapi-spec-url>\n", os.Args[0])
		os.Exit(1)
	}

	specURL := os.Args[1]
	doc, err := loadOpenAPISpec(specURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			continue
		}

		var request JsonRpcRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			writeError(nil, -32700, "Parse error", err)
			continue
		}

		switch request.Method {
		case "tools/list":
			handleToolsList(request, doc)
		case "tools/call":
			handleToolsCall(request, doc)
		default:
			writeError(request.Id, -32601, "Method not found", nil)
		}
	}
}

func loadOpenAPISpec(url string) (libopenapi.Document, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	specData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return libopenapi.NewDocument(specData)
}

func handleToolsList(request JsonRpcRequest, doc libopenapi.Document) {
	model, err := doc.BuildV3Model()
	if err != nil {
		writeError(request.Id, -32603, "Error building OpenAPI model", err)
		return
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

	response := JsonRpcResponse{
		JsonRpc: "2.0",
		Result:  ToolsListResponse{Tools: tools},
		Id:      request.Id,
	}
	writeResponse(response)
}

func handleToolsCall(request JsonRpcRequest, doc libopenapi.Document) {
	var params ToolCallParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		writeError(request.Id, -32602, "Invalid params", err)
		return
	}

	// Build V3 model to look up the operation
	model, errs := doc.BuildV3Model()
	if errs != nil {
		writeError(request.Id, -32603, "Error building OpenAPI model", errs)
		return
	}

	// Find the operation by operationId
	var method, path string
	var found bool
	for pair := model.Model.Paths.PathItems.First(); pair != nil && !found; pair = pair.Next() {
		pathStr := pair.Key()
		pathItem := pair.Value()

		// Check each operation in the path item
		if pathItem.Get != nil && pathItem.Get.OperationId == params.Name {
			method, path = "GET", pathStr
			found = true
		} else if pathItem.Post != nil && pathItem.Post.OperationId == params.Name {
			method, path = "POST", pathStr
			found = true
		} else if pathItem.Put != nil && pathItem.Put.OperationId == params.Name {
			method, path = "PUT", pathStr
			found = true
		} else if pathItem.Delete != nil && pathItem.Delete.OperationId == params.Name {
			method, path = "DELETE", pathStr
			found = true
		} else if pathItem.Patch != nil && pathItem.Patch.OperationId == params.Name {
			method, path = "PATCH", pathStr
			found = true
		}
	}

	// If not found by operationId, try parsing as method + path
	if !found {
		parts := strings.SplitN(params.Name, " ", 2)
		if len(parts) != 2 {
			writeError(request.Id, -32602, "Invalid tool name format", nil)
			return
		}
		method, path = parts[0], parts[1]
	}

	// Build request URL
	baseURL := os.Args[1]
	baseURL = baseURL[:strings.LastIndex(baseURL, "/")] // Remove /openapi.json or similar
	url := baseURL + path

	// Create request body if needed
	var body io.Reader
	if len(params.Parameters) > 0 && (method == "POST" || method == "PUT" || method == "PATCH") {
		jsonBody, err := json.Marshal(params.Parameters)
		if err != nil {
			writeError(request.Id, -32603, "Error encoding request body", err)
			return
		}
		body = bytes.NewReader(jsonBody)
	}

	// Create and send HTTP request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		writeError(request.Id, -32603, "Error creating request", err)
		return
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		writeError(request.Id, -32603, "Error making request", err)
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(request.Id, -32603, "Error reading response", err)
		return
	}

	// Parse response as JSON if possible
	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// If not JSON, use string
		result = string(respBody)
	}

	response := JsonRpcResponse{
		JsonRpc: "2.0",
		Result:  result,
		Id:      request.Id,
	}
	writeResponse(response)
}

func createTool(method string, path string, operation *v3.Operation) Tool {
	// Use operationId as the tool name, fall back to method + path if not available
	name := operation.OperationId
	if name == "" {
		name = fmt.Sprintf("%s %s", method, path)
	}

	description := operation.Description
	if operation.Summary != "" {
		description = operation.Summary
	}

	// Convert parameters to a map
	params := make(map[string]interface{})
	if operation.RequestBody != nil && operation.RequestBody.Content != nil {
		for pair := operation.RequestBody.Content.First(); pair != nil; pair = pair.Next() {
			contentType := pair.Key()
			mediaType := pair.Value()
			if contentType == "application/json" && mediaType.Schema != nil {
				params["body"] = mediaType.Schema
			}
		}
	}

	if operation.Parameters != nil {
		for _, param := range operation.Parameters {
			if param.Schema != nil {
				params[param.Name] = param.Schema
			}
		}
	}

	return Tool{
		Name:        name,
		Description: description,
		Parameters:  params,
	}
}

func writeResponse(response JsonRpcResponse) {
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding response: %v\n", err)
	}
}

func writeError(id interface{}, code int, message string, data interface{}) {
	response := JsonRpcResponse{
		JsonRpc: "2.0",
		Error: &JsonRpcError{
			Code:    code,
			Message: message,
			Data:    data,
		},
		Id: id,
	}
	writeResponse(response)
}
