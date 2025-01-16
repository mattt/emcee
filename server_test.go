package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
)

const testOpenAPISpec = `{
  "openapi": "3.0.0",
  "info": {
    "title": "Test API",
    "version": "1.0.0"
  },
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "summary": "List all pets",
        "description": "Returns all pets from the system"
      },
      "post": {
        "operationId": "createPet",
        "summary": "Create a pet",
        "description": "Creates a new pet in the system",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "name": {
                    "type": "string"
                  },
                  "age": {
                    "type": "integer"
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

func setupTestServer(t *testing.T) (*Server, *httptest.Server) {
	// Create a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pets":
			switch r.Method {
			case "GET":
				json.NewEncoder(w).Encode([]map[string]interface{}{
					{"id": 1, "name": "Fluffy"},
					{"id": 2, "name": "Rover"},
				})
			case "POST":
				var pet map[string]interface{}
				json.NewDecoder(r.Body).Decode(&pet)
				pet["id"] = 3
				json.NewEncoder(w).Encode(pet)
			}
		}
	}))

	// Create a test OpenAPI document
	doc, err := libopenapi.NewDocument([]byte(testOpenAPISpec))
	if err != nil {
		t.Fatalf("Failed to create test document: %v", err)
	}

	// Create a server instance with the test server URL
	server := NewServer(doc, ts.URL)
	server.client = ts.Client() // Use the test server's client

	return server, ts
}

func TestHandleToolsList(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	request := JsonRpcRequest{
		JsonRpc: "2.0",
		Method:  "tools/list",
		Id:      1,
	}

	response := server.HandleRequest(request)

	// Verify response structure
	assert.Equal(t, "2.0", response.JsonRpc)
	assert.Equal(t, 1, response.Id)
	assert.Nil(t, response.Error)

	// Verify tools list
	toolsResp, ok := response.Result.(ToolsListResponse)
	assert.True(t, ok)
	assert.Len(t, toolsResp.Tools, 2) // GET and POST /pets

	// Verify GET operation
	var getOp, postOp Tool
	for _, tool := range toolsResp.Tools {
		if tool.Name == "listPets" {
			getOp = tool
		} else if tool.Name == "createPet" {
			postOp = tool
		}
	}

	assert.Equal(t, "listPets", getOp.Name)
	assert.Equal(t, "Returns all pets from the system", getOp.Description)

	// Verify POST operation
	assert.Equal(t, "createPet", postOp.Name)
	assert.Equal(t, "Creates a new pet in the system", postOp.Description)
	assert.Contains(t, postOp.Parameters, "name")
	assert.Contains(t, postOp.Parameters, "age")
}

func TestHandleToolsCall(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	tests := []struct {
		name     string
		request  JsonRpcRequest
		validate func(*testing.T, JsonRpcResponse)
	}{
		{
			name: "GET request by operationId",
			request: JsonRpcRequest{
				JsonRpc: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "listPets"}`),
				Id:      1,
			},
			validate: func(t *testing.T, response JsonRpcResponse) {
				assert.Equal(t, "2.0", response.JsonRpc)
				assert.Equal(t, 1, response.Id)
				assert.Nil(t, response.Error)

				result, ok := response.Result.([]interface{})
				assert.True(t, ok)
				assert.Len(t, result, 2)
			},
		},
		{
			name: "POST request by operationId",
			request: JsonRpcRequest{
				JsonRpc: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "createPet", "parameters": {"name": "Whiskers", "age": 5}}`),
				Id:      2,
			},
			validate: func(t *testing.T, response JsonRpcResponse) {
				assert.Equal(t, "2.0", response.JsonRpc)
				assert.Equal(t, 2, response.Id)
				assert.Nil(t, response.Error)

				result, ok := response.Result.(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "Whiskers", result["name"])
				assert.Equal(t, float64(5), result["age"])
				assert.Equal(t, float64(3), result["id"])
			},
		},
		{
			name: "Request with invalid operationId",
			request: JsonRpcRequest{
				JsonRpc: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "nonexistentOperation"}`),
				Id:      3,
			},
			validate: func(t *testing.T, response JsonRpcResponse) {
				assert.Equal(t, "2.0", response.JsonRpc)
				assert.Equal(t, 3, response.Id)
				assert.NotNil(t, response.Error)
				assert.Equal(t, -32602, response.Error.Code)
				assert.Equal(t, "Invalid params", response.Error.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := server.HandleRequest(tt.request)
			tt.validate(t, response)
		})
	}
}

func TestHandleInvalidMethod(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	request := JsonRpcRequest{
		JsonRpc: "2.0",
		Method:  "invalid/method",
		Id:      1,
	}

	response := server.HandleRequest(request)

	assert.Equal(t, "2.0", response.JsonRpc)
	assert.Equal(t, 1, response.Id)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32601, response.Error.Code)
	assert.Equal(t, "Method not found", response.Error.Message)
}
