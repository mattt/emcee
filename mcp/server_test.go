package mcp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loopwork-ai/emcee/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
    },
    "/pets/image": {
      "get": {
        "operationId": "getPetImage",
        "summary": "Get a pet's image",
        "description": "Returns a pet's image in PNG format",
        "responses": {
          "200": {
            "description": "A pet image",
            "content": {
              "image/png": {
                "schema": {
                  "type": "string",
                  "format": "binary"
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
	t.Helper()

	// Create a small test image
	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header

	// Create a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi.json":
			w.Write([]byte(testOpenAPISpec))
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
		case "/pets/image":
			w.Header().Set("Content-Type", "image/png")
			w.Write(imgData)
		}
	}))

	// Create a server instance with the test server URL
	server, err := NewServer(
		WithClient(ts.Client()),
		WithServerInfo("Test API", "1.0.0"),
		WithSpecURL(ts.URL+"/openapi.json"),
	)
	require.NoError(t, err)

	return server, ts
}

func TestServer_HandleInitialize(t *testing.T) {
	// Create a test HTTP server to serve the spec
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testOpenAPISpec))
	}))
	defer ts.Close()

	// Test with OpenAPI spec that includes server info
	server, err := NewServer(
		WithClient(http.DefaultClient),
		WithServerInfo("Test API", "1.0.0"),
		WithSpecURL(ts.URL),
	)
	require.NoError(t, err)

	// Create a basic initialize request
	request := jsonrpc.NewRequest("initialize", json.RawMessage(`{}`), 1)

	// Get the response
	response := server.Handle(request)

	// Assert no error
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.Nil(t, response.Error)

	// Parse the response result
	var result InitializeResponse
	resultBytes, err := json.Marshal(response.Result)
	require.NoError(t, err)
	err = json.Unmarshal(resultBytes, &result)
	require.NoError(t, err)

	// Verify the response structure
	assert.Equal(t, "2024-11-05", result.ProtocolVersion)
	assert.Equal(t, "Test API", result.ServerInfo.Name) // From testOpenAPISpec
	assert.Equal(t, "1.0.0", result.ServerInfo.Version) // From testOpenAPISpec
	assert.False(t, result.Capabilities.Tools.ListChanged)

	// Test with empty OpenAPI spec
	emptySpec := `{
		"openapi": "3.0.0",
		"paths": {}
	}`
	tsEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(emptySpec))
	}))
	defer tsEmpty.Close()

	serverEmpty, err := NewServer(
		WithClient(http.DefaultClient),
		WithSpecURL(tsEmpty.URL),
	)
	require.NoError(t, err)

	// Get response from empty spec server
	responseEmpty := serverEmpty.Handle(request)
	var resultEmpty InitializeResponse
	resultBytes, err = json.Marshal(responseEmpty.Result)
	require.NoError(t, err)
	err = json.Unmarshal(resultBytes, &resultEmpty)
	require.NoError(t, err)

	// Verify default values are used when spec info is missing
	assert.Equal(t, "openapi-mcp", resultEmpty.ServerInfo.Name)
	assert.Equal(t, "0.1.0", resultEmpty.ServerInfo.Version)
}

func TestHandleToolsList(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	request := jsonrpc.NewRequest("tools/list", nil, 1)

	response := server.Handle(request)

	// Verify response structure
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.Nil(t, response.Error)

	// Verify tools list
	toolsResp, ok := response.Result.(ToolsListResponse)
	assert.True(t, ok)
	assert.Len(t, toolsResp.Tools, 3) // GET and POST /pets, plus GET /pets/image

	// Verify GET operation
	var getOp, postOp, imageOp Tool
	for _, tool := range toolsResp.Tools {
		switch tool.Name {
		case "listPets":
			getOp = tool
		case "createPet":
			postOp = tool
		case "getPetImage":
			imageOp = tool
		}
	}

	assert.Equal(t, "listPets", getOp.Name)
	assert.Equal(t, "Returns all pets from the system", getOp.Description)

	// Verify POST operation
	assert.Equal(t, "createPet", postOp.Name)
	assert.Equal(t, "Creates a new pet in the system", postOp.Description)
	assert.Contains(t, postOp.InputSchema.Properties, "name")
	assert.Contains(t, postOp.InputSchema.Properties, "age")

	// Verify Image operation
	assert.Equal(t, "getPetImage", imageOp.Name)
	assert.Equal(t, "Returns a pet's image in PNG format", imageOp.Description)
	assert.Empty(t, imageOp.InputSchema.Properties) // No input parameters needed
}

func TestHandleToolsCall(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	// Create a small test image
	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header

	tests := []struct {
		name     string
		request  jsonrpc.Request
		validate func(*testing.T, jsonrpc.Response)
	}{
		{
			name:    "GET request by operationId",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "listPets"}`), 1),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 1, response.ID.Value())
				assert.Nil(t, response.Error)

				result, ok := response.Result.(CallToolResult)
				assert.True(t, ok)
				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				content, ok := result.Content[0].(TextContent)
				assert.True(t, ok)
				assert.Equal(t, "text", content.Type)
				assert.NotNil(t, content.Annotations)
				assert.Contains(t, content.Annotations.Audience, RoleAssistant)

				var pets []interface{}
				err := json.Unmarshal([]byte(content.Text), &pets)
				assert.NoError(t, err)
				assert.Len(t, pets, 2)
			},
		},
		{
			name:    "POST request by operationId",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "createPet", "arguments": {"name": "Whiskers", "age": 5}}`), 2),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 2, response.ID.Value())
				assert.Nil(t, response.Error)

				result, ok := response.Result.(CallToolResult)
				assert.True(t, ok)
				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				content, ok := result.Content[0].(TextContent)
				assert.True(t, ok)
				assert.Equal(t, "text", content.Type)
				assert.NotNil(t, content.Annotations)
				assert.Contains(t, content.Annotations.Audience, RoleAssistant)

				var pet map[string]interface{}
				err := json.Unmarshal([]byte(content.Text), &pet)
				assert.NoError(t, err)
				assert.Equal(t, "Whiskers", pet["name"])
				assert.Equal(t, float64(5), pet["age"])
				assert.Equal(t, float64(3), pet["id"])
			},
		},
		{
			name:    "GET image request",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "getPetImage"}`), 4),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 4, response.ID.Value())
				assert.Nil(t, response.Error)

				result, ok := response.Result.(CallToolResult)
				assert.True(t, ok)
				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				content, ok := result.Content[0].(ImageContent)
				assert.True(t, ok)
				assert.Equal(t, "image", content.Type)
				assert.NotNil(t, content.Annotations)
				assert.Contains(t, content.Annotations.Audience, RoleAssistant)
				assert.Equal(t, "image/png", content.MimeType)

				// Decode base64 data
				decoded, err := base64.StdEncoding.DecodeString(content.Data)
				assert.NoError(t, err)
				assert.Equal(t, imgData, decoded)
			},
		},
		{
			name:    "Request with invalid operationId",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "nonexistentOperation"}`), 3),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 3, response.ID.Value())
				assert.Equal(t, jsonrpc.ErrMethodNotFound, response.Error.Code)
				assert.Equal(t, "Method not found", response.Error.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := server.Handle(tt.request)
			tt.validate(t, response)
		})
	}
}

func TestHandleInvalidMethod(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	request := jsonrpc.NewRequest("invalid/method", nil, 1)

	response := server.Handle(request)

	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.NotNil(t, response.Error)
	assert.Equal(t, int(jsonrpc.ErrMethodNotFound), int(response.Error.Code))
	assert.Equal(t, "Method not found", response.Error.Message)
}
