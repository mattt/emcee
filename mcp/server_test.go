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

func newTestSpec(serverURL string) []byte {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "Test API",
			"version": "1.0.0",
		},
		"servers": []map[string]interface{}{
			{"url": serverURL},
		},
		"paths": map[string]interface{}{
			"/pets": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listPets",
					"summary":     "List all pets",
					"description": "Returns all pets from the system",
					"parameters": []map[string]interface{}{
						{"name": "limit", "in": "query", "description": "Maximum number of pets to return", "schema": map[string]interface{}{"type": "integer"}},
						{"name": "type", "in": "query", "description": "Type of pets to filter by", "schema": map[string]interface{}{"type": "string"}},
					},
				},
				"post": map[string]interface{}{
					"operationId": "createPet",
					"summary":     "Create a pet",
					"description": "Creates a new pet in the system",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"name": map[string]interface{}{
											"type": "string",
										},
										"age": map[string]interface{}{
											"type": "integer",
										},
									},
								},
							},
						},
					},
				},
			},
			"/pets/image": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "getPetImage",
					"summary":     "Get a pet's image",
					"description": "Returns a pet's image in PNG format",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "A pet image",
							"content": map[string]interface{}{
								"image/png": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":   "string",
										"format": "binary",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		panic(err)
	}
	return data
}

func setupTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	// Create a small test image
	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header

	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi.json":
			w.Header().Set("Content-Type", "application/json")
			w.Write(newTestSpec(ts.URL))
		case "/pets":
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case "GET":
				// Verify query parameters are present
				limit := r.URL.Query().Get("limit")
				petType := r.URL.Query().Get("type")
				assert.Equal(t, "5", limit)
				assert.Equal(t, "dog", petType)

				pets := []map[string]interface{}{
					{"id": 1, "name": "Fluffy", "type": "dog"},
					{"id": 2, "name": "Rover", "type": "dog"},
				}
				err := json.NewEncoder(w).Encode(pets)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			case "POST":
				// Verify Content-Type header
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var pet map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&pet); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				// Verify request body parameters
				assert.Equal(t, "Whiskers", pet["name"])
				assert.Equal(t, float64(5), pet["age"])

				// Add ID and return
				pet["id"] = 3
				err := json.NewEncoder(w).Encode(pet)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		case "/pets/image":
			w.Header().Set("Content-Type", "image/png")
			_, err := w.Write(imgData)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))

	// Create a server instance with the test server URL and spec
	server, err := NewServer(
		WithClient(ts.Client()),
		WithServerInfo("Test API", "1.0.0"),
		WithSpecData(newTestSpec(ts.URL)),
	)
	require.NoError(t, err)

	return server, ts
}

func TestServer_HandleInitialize(t *testing.T) {
	// Create a test HTTP server to serve the spec
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(newTestSpec(r.Host))
	}))
	defer ts.Close()

	// Test with OpenAPI spec that includes server info
	server, err := NewServer(
		WithClient(http.DefaultClient),
		WithServerInfo("Test API", "1.0.0"),
		WithSpecData(newTestSpec(ts.URL)),
	)
	require.NoError(t, err)

	// Create a basic initialize request
	request := jsonrpc.NewRequest("initialize", json.RawMessage(`{}`), 1)

	// Get the response
	response := server.HandleRequest(request)

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
	assert.Equal(t, "Test API", result.ServerInfo.Name)
	assert.Equal(t, "1.0.0", result.ServerInfo.Version)
	assert.False(t, result.Capabilities.Tools.ListChanged)

	// Test with empty OpenAPI spec
	emptySpec := map[string]interface{}{
		"openapi": "3.0.0",
		"servers": []map[string]interface{}{
			{"url": ts.URL},
		},
		"paths": map[string]interface{}{},
	}
	emptySpecData, err := json.Marshal(emptySpec)
	require.NoError(t, err)

	tsEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(emptySpecData)
	}))
	defer tsEmpty.Close()

	serverEmpty, err := NewServer(
		WithClient(http.DefaultClient),
		WithSpecData(emptySpecData),
	)
	require.NoError(t, err)

	// Get response from empty spec server
	responseEmpty := serverEmpty.HandleRequest(request)
	var resultEmpty InitializeResponse
	resultBytes, err = json.Marshal(responseEmpty.Result)
	require.NoError(t, err)
	err = json.Unmarshal(resultBytes, &resultEmpty)
	require.NoError(t, err)
}

func TestHandleToolsList(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	request := jsonrpc.NewRequest("tools/list", nil, 1)

	response := server.HandleRequest(request)

	// Verify response structure
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.Nil(t, response.Error)

	// Convert response.Result to ToolsListResponse
	var toolsResp ToolsListResponse
	resultBytes, err := json.Marshal(response.Result)
	require.NoError(t, err)
	err = json.Unmarshal(resultBytes, &toolsResp)
	require.NoError(t, err)

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
			name:    "GET request with query parameters",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "listPets", "arguments": {"limit": 5, "type": "dog"}}`), 1),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 1, response.ID.Value())
				assert.Nil(t, response.Error)

				// Convert response.Result to ToolCallResponse
				var result ToolCallResponse
				resultBytes, err := json.Marshal(response.Result)
				require.NoError(t, err)
				err = json.Unmarshal(resultBytes, &result)
				require.NoError(t, err)

				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				content := result.Content[0]
				assert.Equal(t, "text", content.Type)
				assert.NotNil(t, content.Annotations)
				assert.Contains(t, content.Annotations.Audience, RoleAssistant)

				// Unmarshal the response into a TextContent to get the text
				var textContent Content
				contentBytes, err := json.Marshal(content)
				assert.NoError(t, err)
				err = json.Unmarshal(contentBytes, &textContent)
				assert.NoError(t, err)

				var pets []interface{}
				err = json.Unmarshal([]byte(textContent.Text), &pets)
				assert.NoError(t, err)
				assert.Len(t, pets, 2)

				// Verify the returned pets have the correct type
				for _, pet := range pets {
					petMap := pet.(map[string]interface{})
					assert.Equal(t, "dog", petMap["type"])
				}
			},
		},
		{
			name:    "POST request with body parameters",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "createPet", "arguments": {"name": "Whiskers", "age": 5}}`), 2),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 2, response.ID.Value())
				assert.Nil(t, response.Error)

				// Convert response.Result to ToolCallResponse
				var result ToolCallResponse
				resultBytes, err := json.Marshal(response.Result)
				require.NoError(t, err)
				err = json.Unmarshal(resultBytes, &result)
				require.NoError(t, err)

				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				content := result.Content[0]
				assert.Equal(t, "text", content.Type)
				assert.NotNil(t, content.Annotations)
				assert.Contains(t, content.Annotations.Audience, RoleAssistant)

				var textContent Content
				contentBytes, err := json.Marshal(content)
				assert.NoError(t, err)
				err = json.Unmarshal(contentBytes, &textContent)
				assert.NoError(t, err)

				var pet map[string]interface{}
				err = json.Unmarshal([]byte(textContent.Text), &pet)
				assert.NoError(t, err)
				assert.Equal(t, "Whiskers", pet["name"])
				assert.Equal(t, float64(5), pet["age"])
				assert.Equal(t, float64(3), pet["id"])
			},
		},
		{
			name:    "GET image request",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "getPetImage"}`), 3),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 3, response.ID.Value())
				assert.Nil(t, response.Error)

				// Convert response.Result to ToolCallResponse
				var result ToolCallResponse
				resultBytes, err := json.Marshal(response.Result)
				require.NoError(t, err)
				err = json.Unmarshal(resultBytes, &result)
				require.NoError(t, err)

				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				content := result.Content[0]
				assert.Equal(t, "image", content.Type)
				assert.NotNil(t, content.Annotations)
				assert.Contains(t, content.Annotations.Audience, RoleAssistant)

				var imageContent Content
				contentBytes, err := json.Marshal(content)
				assert.NoError(t, err)
				err = json.Unmarshal(contentBytes, &imageContent)
				assert.NoError(t, err)

				assert.Equal(t, "image/png", imageContent.MimeType)

				decoded, err := base64.StdEncoding.DecodeString(imageContent.Data)
				assert.NoError(t, err)
				assert.Equal(t, imgData, decoded)
			},
		},
		{
			name:    "Request with invalid operationId",
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "nonexistentOperation"}`), 4),
			validate: func(t *testing.T, response jsonrpc.Response) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 4, response.ID.Value())
				assert.Equal(t, jsonrpc.ErrMethodNotFound, response.Error.Code)
				assert.Equal(t, "Method not found", response.Error.Message)
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

	request := jsonrpc.NewRequest("invalid/method", nil, 1)

	response := server.HandleRequest(request)

	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.NotNil(t, response.Error)
	assert.Equal(t, int(jsonrpc.ErrMethodNotFound), int(response.Error.Code))
	assert.Equal(t, "Method not found", response.Error.Message)
}

func TestWithSpecData(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
		assert  func(*testing.T, *Server)
	}{
		{
			name: "valid spec with servers",
			spec: `{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API",
					"version": "1.0.0"
				},
				"servers": [
					{
						"url": "https://api.example.com"
					}
				],
				"paths": {}
			}`,
			assert: func(t *testing.T, s *Server) {
				assert.Equal(t, "https://api.example.com", s.baseURL)
			},
		},
		{
			name:    "invalid spec",
			spec:    `{"openapi": "3.0.0", "invalid": true`,
			wantErr: true,
		},
		{
			name: "spec without servers",
			spec: `{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API",
					"version": "1.0.0"
				},
				"paths": {}
			}`,
			wantErr: true,
		},
		{
			name: "spec with empty server URL",
			spec: `{
				"openapi": "3.0.0",
				"servers": [
					{
						"url": ""
					}
				],
				"paths": {}
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithSpecData([]byte(tt.spec)))
			if tt.wantErr {
				assert.Error(t, err)
				if tt.name == "spec without servers" {
					assert.Contains(t, err.Error(), "must include at least one server URL")
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, server.doc)
			assert.NotNil(t, server.model)

			if tt.assert != nil {
				tt.assert(t, server)
			}
		})
	}
}
