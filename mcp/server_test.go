package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"

	"io"
	"log/slog"

	"github.com/loopwork/emcee/internal"
	"github.com/loopwork/emcee/jsonrpc"
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
						{
							"name":        "fields",
							"in":          "query",
							"description": "Fields to return",
							"schema": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"type": "string",
								},
							},
						},
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
			"/pets/{petId}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "getPet",
					"summary":     "Get a specific pet",
					"description": "Returns a specific pet by ID",
					"parameters": []map[string]interface{}{
						{
							"name":        "petId",
							"in":          "path",
							"required":    true,
							"description": "The ID of the pet to retrieve",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "A pet",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"id": map[string]interface{}{
												"type": "integer",
											},
											"name": map[string]interface{}{
												"type": "string",
											},
										},
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

	// Track if auth header was checked
	authHeaderChecked := false

	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header if present
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			assert.Equal(t, "Bearer test-token", authHeader, "Authorization header should match")
			authHeaderChecked = true
		}

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

				// For auth test case, verify the auth header was checked
				if r.Header.Get("Authorization") != "" {
					assert.True(t, authHeaderChecked, "Auth header should have been checked")
				}

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
		case "/pets/special%20pet":
			w.Header().Set("Content-Type", "application/json")
			pet := map[string]interface{}{
				"id":   1,
				"name": "Special Pet",
			}
			json.NewEncoder(w).Encode(pet)
		default:
			http.NotFound(w, r)
		}
	}))

	client := ts.Client()
	client.Transport = &internal.HeaderTransport{
		Base:    client.Transport,
		Headers: http.Header{"Authorization": []string{"Bearer test-token"}},
	}

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

	assert.Len(t, toolsResp.Tools, 4) // GET and POST /pets, plus GET /pets/image

	// Verify GET operation
	var getOp, postOp, imageOp, getPetOp Tool
	for _, tool := range toolsResp.Tools {
		switch tool.Name {
		case "listPets":
			getOp = tool
		case "createPet":
			postOp = tool
		case "getPetImage":
			imageOp = tool
		case "getPet":
			getPetOp = tool
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

	// Verify GET /pets/special pet operation
	assert.Equal(t, "getPet", getPetOp.Name)
	assert.Equal(t, "Returns a specific pet by ID", getPetOp.Description)
	assert.Contains(t, getPetOp.InputSchema.Properties, "petId")
}

func TestHandleToolsCall(t *testing.T) {
	server, ts := setupTestServer(t)
	defer ts.Close()

	tests := []struct {
		name     string
		server   *Server
		setup    func(*testing.T, *httptest.Server) http.HandlerFunc
		request  jsonrpc.Request
		validate func(*testing.T, jsonrpc.Response, string)
	}{
		{
			name:   "GET request with query parameters",
			server: server,
			setup: func(t *testing.T, ts *httptest.Server) http.HandlerFunc {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify query parameters are present
					limit := r.URL.Query().Get("limit")
					petType := r.URL.Query().Get("type")
					assert.Equal(t, "5", limit)
					assert.Equal(t, "dog", petType)

					w.Header().Set("Content-Type", "application/json")
					pets := []map[string]interface{}{
						{"id": 1, "name": "Fluffy", "type": "dog"},
						{"id": 2, "name": "Rover", "type": "dog"},
					}
					json.NewEncoder(w).Encode(pets)
				})
			},
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "listPets", "arguments": {"limit": 5, "type": "dog"}}`), 1),
			validate: func(t *testing.T, response jsonrpc.Response, url string) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 1, response.ID.Value())
				assert.Nil(t, response.Error)

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

				var pets []interface{}
				err = json.Unmarshal([]byte(textContent.Text), &pets)
				assert.NoError(t, err)
				assert.Len(t, pets, 2)

				for _, pet := range pets {
					petMap := pet.(map[string]interface{})
					assert.Equal(t, "dog", petMap["type"])
				}
			},
		},
		{
			name:   "GET request with array query parameters",
			server: server,
			setup: func(t *testing.T, ts *httptest.Server) http.HandlerFunc {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify the query parameters
					assert.Equal(t, "dog", r.URL.Query().Get("type"))
					// The fields parameter should be a comma-separated list
					assert.Equal(t, "name,age,breed", r.URL.Query().Get("fields"))

					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"success": true,
						"query": map[string]string{
							"type":   r.URL.Query().Get("type"),
							"fields": r.URL.Query().Get("fields"),
						},
					})
				})
			},
			request: jsonrpc.NewRequest("tools/call", json.RawMessage(`{
				"name": "listPets", 
				"arguments": {
					"type": "dog",
					"fields": ["name", "age", "breed"]
				}
			}`), 7),
			validate: func(t *testing.T, response jsonrpc.Response, requestURL string) {
				assert.Equal(t, "2.0", response.Version)
				assert.Equal(t, 7, response.ID.Value())
				assert.Nil(t, response.Error)

				var result ToolCallResponse
				resultBytes, err := json.Marshal(response.Result)
				require.NoError(t, err)
				err = json.Unmarshal(resultBytes, &result)
				require.NoError(t, err)

				assert.Len(t, result.Content, 1)
				assert.False(t, result.IsError)

				// Parse the URL to verify parameters
				parsedURL, err := url.Parse(requestURL)
				require.NoError(t, err)

				params := parsedURL.Query()
				assert.Equal(t, "dog", params.Get("type"))
				assert.Equal(t, "name,age,breed", params.Get("fields"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedURL string
			if tt.setup != nil {
				handler := tt.setup(t, ts)
				// Wrap the handler to capture the URL
				wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					capturedURL = r.URL.String()
					handler.ServeHTTP(w, r)
				})
				ts.Config.Handler = wrappedHandler
			}

			var response jsonrpc.Response
			if tt.server != nil {
				response = *tt.server.HandleRequest(tt.request)
			} else {
				response = *server.HandleRequest(tt.request)
			}

			tt.validate(t, response, capturedURL)
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

func TestWithAuth(t *testing.T) {
	tests := []struct {
		name    string
		auth    string
		wantErr bool
		assert  func(*testing.T, *Server)
	}{
		{
			name: "valid bearer token",
			auth: "Bearer mytoken123",
			assert: func(t *testing.T, s *Server) {
				assert.Equal(t, "Bearer mytoken123", s.auth)
			},
		},
		{
			name: "valid basic auth",
			auth: "Basic dXNlcjpwYXNz",
			assert: func(t *testing.T, s *Server) {
				assert.Equal(t, "Basic dXNlcjpwYXNz", s.auth)
			},
		},
		{
			name:    "missing auth type",
			auth:    "mytoken123",
			wantErr: true,
		},
		{
			name:    "empty auth",
			auth:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			auth:    "   ",
			wantErr: true,
		},
	}

	// Create a minimal valid spec for server initialization
	validSpec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {}
	}`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(
				WithSpecData([]byte(validSpec)),
				WithAuth(tt.auth),
			)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, server)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, server)

			if tt.assert != nil {
				tt.assert(t, server)
			}

			// Verify the auth header is properly set in the client transport
			transport, ok := server.client.Transport.(*internal.HeaderTransport)
			assert.True(t, ok)
			assert.Equal(t, tt.auth, transport.Headers.Get("Authorization"))
		})
	}
}

func TestPathJoining(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		expected string
	}{
		{
			name:     "simple paths",
			baseURL:  "https://api.example.com",
			path:     "/pets",
			expected: "https://api.example.com/pets",
		},
		{
			name:     "base URL with trailing slash",
			baseURL:  "https://api.example.com/",
			path:     "/pets",
			expected: "https://api.example.com/pets",
		},
		{
			name:     "base URL with path",
			baseURL:  "https://api.example.com/v1",
			path:     "/pets",
			expected: "https://api.example.com/v1/pets",
		},
		{
			name:     "base URL with path and trailing slash",
			baseURL:  "https://api.example.com/v1/",
			path:     "/pets",
			expected: "https://api.example.com/v1/pets",
		},
		{
			name:     "path without leading slash",
			baseURL:  "https://api.example.com/v1",
			path:     "pets",
			expected: "https://api.example.com/v1/pets",
		},
		{
			name:     "multiple path segments",
			baseURL:  "https://api.example.com/v1",
			path:     "/pets/dogs",
			expected: "https://api.example.com/v1/pets/dogs",
		},
		{
			name:     "multiple slashes in path",
			baseURL:  "https://api.example.com/v1/",
			path:     "//pets///dogs",
			expected: "https://api.example.com/v1/pets/dogs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock HTTP server first
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Parse the request URL and compare with expected
				actualPath := path.Clean(r.URL.Path)
				expectedPath := path.Clean(tt.path)
				if !strings.HasPrefix(expectedPath, "/") {
					expectedPath = "/" + expectedPath
				}
				assert.Equal(t, expectedPath, actualPath)

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
			}))
			defer ts.Close()

			// Create a test spec with the test server URL
			spec := fmt.Sprintf(`{
				"openapi": "3.0.0",
				"servers": [{"url": "%s"}],
				"paths": {
					"%s": {
						"get": {
							"operationId": "testOperation"
						}
					}
				}
			}`, ts.URL, tt.path)

			server, err := NewServer(
				WithSpecData([]byte(spec)),
				WithClient(ts.Client()),
			)
			require.NoError(t, err)

			// Make a test request
			request := jsonrpc.NewRequest("tools/call", json.RawMessage(`{"name": "testOperation"}`), 1)
			response := server.HandleRequest(request)

			// Verify the request was successful
			assert.Nil(t, response.Error)
		})
	}
}

func TestToolNameMapping(t *testing.T) {
	tests := []struct {
		name        string
		operationId string
		wantLen     int
		wantPrefix  string
	}{
		{
			name:        "short operation ID",
			operationId: "listPets",
			wantLen:     8,
			wantPrefix:  "listPets",
		},
		{
			name:        "exactly 64 characters",
			operationId: strings.Repeat("a", 64),
			wantLen:     64,
			wantPrefix:  strings.Repeat("a", 64),
		},
		{
			name:        "long operation ID",
			operationId: "thisIsAVeryLongOperationIdThatExceedsTheSixtyFourCharacterLimitAndNeedsToBeHandledProperly",
			wantLen:     64,
			wantPrefix:  "thisIsAVeryLongOperationIdThatExceedsTheSixtyFourCharac", // 55 chars
		},
		{
			name:        "multiple long IDs generate different names",
			operationId: "anotherVeryLongOperationIdThatExceedsTheSixtyFourCharacterLimitAndNeedsToBeHandledProperly",
			wantLen:     64,
			wantPrefix:  "anotherVeryLongOperationIdThatExceedsTheSixtyFourCharac", // 55 chars
		},
	}

	// Store generated names to check for uniqueness
	generatedNames := make(map[string]string)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get the tool name
			toolName := getToolName(tt.operationId)

			// Check length constraints
			assert.Equal(t, tt.wantLen, len(toolName), "tool name length mismatch")

			// For long names, verify the structure
			if len(tt.operationId) > 64 {
				// Verify the prefix is exactly 55 characters
				prefix := toolName[:55]
				assert.Equal(t, tt.wantPrefix, prefix, "prefix mismatch")

				// Check that there's an underscore separator at position 55
				assert.Equal(t, "_", string(toolName[55]), "underscore separator not found at position 55")

				// Verify hash part length (should be 8 characters)
				hash := toolName[56:]
				assert.Equal(t, 8, len(hash), "hash suffix should be 8 characters")

				// Verify the hash is URL-safe base64
				for _, c := range hash {
					assert.Contains(t,
						"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_",
						string(c),
						"hash should only contain URL-safe base64 characters")
				}
			} else {
				// For short names, verify exact match
				assert.Equal(t, tt.wantPrefix, toolName, "tool name mismatch for short operation ID")
			}

			// Check bijectivity - each operation ID should generate a unique tool name
			if existing, exists := generatedNames[toolName]; exists {
				assert.Equal(t, tt.operationId, existing,
					"tool name collision detected: same name generated for different operation IDs")
			}
			generatedNames[toolName] = tt.operationId
		})
	}
}

func TestFindOperationByToolName(t *testing.T) {
	// Create a test spec with a mix of short and long operation IDs
	longOpId := "thisIsAVeryLongOperationIdThatExceedsTheSixtyFourCharacterLimitAndNeedsToBeHandledProperly"
	spec := fmt.Sprintf(`{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/pets": {
				"get": {
					"operationId": "listPets",
					"description": "List pets"
				}
			},
			"/very/long/path": {
				"post": {
					"operationId": "%s",
					"description": "Operation with long ID"
				}
			}
		}
	}`, longOpId)

	server, err := NewServer(WithSpecData([]byte(spec)))
	require.NoError(t, err)

	tests := []struct {
		name       string
		toolName   string
		wantMethod string
		wantPath   string
		wantFound  bool
	}{
		{
			name:       "find short operation ID",
			toolName:   "listPets",
			wantMethod: "GET",
			wantPath:   "/pets",
			wantFound:  true,
		},
		{
			name:       "find long operation ID",
			toolName:   getToolName(longOpId),
			wantMethod: "POST",
			wantPath:   "/very/long/path",
			wantFound:  true,
		},
		{
			name:      "operation not found",
			toolName:  "nonexistentOperation",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, path, operation, pathItem, found := server.findOperationByToolName(tt.toolName)

			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantMethod, method)
				assert.Equal(t, tt.wantPath, path)
				assert.NotNil(t, operation)
				assert.NotNil(t, pathItem)
			} else {
				assert.Empty(t, method)
				assert.Empty(t, path)
				assert.Nil(t, operation)
				assert.Nil(t, pathItem)
			}
		})
	}
}

func TestHandleInitializedNotification(t *testing.T) {
	// Create a test server with a logger
	server, ts := setupTestServer(t)
	defer ts.Close()

	// Add a logger to the server
	server.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create an initialized notification
	notification := jsonrpc.NewRequest("initialized", nil, 1)

	// Handle the notification
	response := server.HandleRequest(notification)

	// Verify that notifications don't generate a response
	assert.Nil(t, response, "notifications should not generate a response")
}
