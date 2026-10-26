package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterToolsSupportsNativeQueryOperation(t *testing.T) {
	testRegisterToolsSupportsQuery(t, "3.2.0", "query")
}

func TestRegisterToolsSupportsQueryExtension(t *testing.T) {
	testRegisterToolsSupportsQuery(t, "3.1.0", "x-query")
}

func testRegisterToolsSupportsQuery(t *testing.T, openAPIVersion, operationKey string) {
	type observedRequest struct {
		method string
		body   map[string]any
		err    error
	}
	observed := make(chan observedRequest, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obs := observedRequest{method: r.Method}
		body, err := io.ReadAll(r.Body)
		if err == nil {
			err = json.Unmarshal(body, &obs.body)
		}
		obs.err = err
		observed <- obs

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()

	spec := fmt.Sprintf(`{
  "openapi": %q,
  "info": {"title": "QUERY API", "version": "1.0.0"},
  "servers": [{"url": %q}],
  "paths": {
    "/search": {
      %q: {
        "operationId": "searchRecords",
        "summary": "Search records",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "q": {"type": "string", "description": "Search query"}
                },
                "required": ["q"]
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`, openAPIVersion, api.URL, operationKey)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "dev"}, nil)
	require.NoError(t, RegisterTools(server, []byte(spec), api.Client()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "dev"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	var queryTool *mcp.Tool
	for i := range tools.Tools {
		if tools.Tools[i].Name == "searchRecords" {
			queryTool = tools.Tools[i]
			break
		}
	}
	require.NotNil(t, queryTool)
	require.NotNil(t, queryTool.Annotations)
	assert.True(t, queryTool.Annotations.ReadOnlyHint)
	assert.True(t, queryTool.Annotations.IdempotentHint)

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "searchRecords",
		Arguments: map[string]any{"q": "emcee"},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	obs := <-observed
	require.NoError(t, obs.err)
	assert.Equal(t, "QUERY", obs.method)
	assert.Equal(t, map[string]any{"q": "emcee"}, obs.body)
}

// readTestRailSpec reads a subset of the TestRail specification.
func readTestRailSpec(t *testing.T) string {
	t.Helper()
	spec, err := os.ReadFile("../testdata/testrail/openapi.yaml")
	require.NoError(t, err)
	return string(spec)
}

// recordRequests starts a test server that records the requests it receives.
func recordRequests(t *testing.T) (*httptest.Server, <-chan *http.Request) {
	t.Helper()
	requests := make(chan *http.Request, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)
	return api, requests
}

// callTool registers tools from spec and calls the named tool with args.
func callTool(t *testing.T, spec string, client *http.Client, opts []RegisterToolsOption, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "dev"}, nil)
	require.NoError(t, RegisterTools(server, []byte(spec), client, opts...))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { serverSession.Close() })

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "dev"}, nil)
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { clientSession.Close() })

	return clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}

func TestRegisterToolsExpandsServerVariables(t *testing.T) {
	api, requests := recordRequests(t)
	spec := fmt.Sprintf(`
openapi: 3.1.0
info: { title: Variables API, version: "1.0.0" }
servers:
  - url: "{scheme}://{host}/{version}"
    variables:
      scheme: { default: http }
      host: { default: %q }
      version: { default: v1, enum: [v1, v2] }
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200": { description: OK }
`, strings.TrimPrefix(api.URL, "http://"))

	result, err := callTool(t, spec, api.Client(), nil, "listItems", nil)
	require.NoError(t, err)
	require.False(t, result.IsError)

	r := <-requests
	assert.Equal(t, "/v1/items", r.URL.Path)
}

func TestRegisterToolsWithServerURL(t *testing.T) {
	api, requests := recordRequests(t)
	opts := []RegisterToolsOption{WithServerURL(api.URL + "/index.php?/api/v2")}

	result, err := callTool(t, readTestRailSpec(t), api.Client(), opts, "getCase", map[string]any{"case_id": 30245416})
	require.NoError(t, err)
	require.False(t, result.IsError)

	r := <-requests
	assert.Equal(t, "/index.php", r.URL.Path)
	assert.Equal(t, "/api/v2/get_case/30245416", r.URL.RawQuery)
}

func TestRegisterToolsExtendsServerURLQuery(t *testing.T) {
	api, requests := recordRequests(t)
	opts := []RegisterToolsOption{WithServerURL(api.URL + "/index.php?/api/v2")}

	result, err := callTool(t, readTestRailSpec(t), api.Client(), opts, "getCases", map[string]any{
		"project_id":  83,
		"suite_id":    68297,
		"section_id":  2046975,
		"priority_id": []any{1, 2},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	r := <-requests
	assert.Equal(t, "/index.php", r.URL.Path)
	assert.Equal(t, "/api/v2/get_cases/83&priority_id=1%2C2&section_id=2046975&suite_id=68297", r.URL.RawQuery)
}

func TestRegisterToolsFormatsIntegerParameters(t *testing.T) {
	api, requests := recordRequests(t)
	spec := fmt.Sprintf(`
openapi: 3.1.0
info: { title: Integer API, version: "1.0.0" }
servers:
  - url: %q
paths:
  /cases/{case_id}:
    get:
      operationId: getCases
      parameters:
        - { name: case_id, in: path, required: true, schema: { type: integer } }
        - { name: section_id, in: query, schema: { type: integer } }
        - { name: ids, in: query, schema: { type: array, items: { type: integer } } }
        - { name: X-Request-Id, in: header, schema: { type: integer } }
      responses:
        "200": { description: OK }
`, api.URL)

	result, err := callTool(t, spec, api.Client(), nil, "getCases", map[string]any{
		"case_id":      30245416,
		"section_id":   2046975,
		"ids":          []any{30245416, 2046975},
		"X-Request-Id": 1234567890,
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	r := <-requests
	assert.Equal(t, "/cases/30245416", r.URL.Path)
	assert.Equal(t, "2046975", r.URL.Query().Get("section_id"))
	assert.Equal(t, "30245416,2046975", r.URL.Query().Get("ids"))
	assert.Equal(t, "1234567890", r.Header.Get("X-Request-Id"))
}

func TestRegisterToolsRejectsFractionalIntegerParameters(t *testing.T) {
	api, requests := recordRequests(t)
	opts := []RegisterToolsOption{WithServerURL(api.URL + "/index.php?/api/v2")}

	_, err := callTool(t, readTestRailSpec(t), api.Client(), opts, "getCase", map[string]any{"case_id": 1.5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `want "integer"`)
	assert.Empty(t, requests)
}

func TestRegisterToolsEscapesPathParametersInServerURLQuery(t *testing.T) {
	spec := `
openapi: 3.1.0
info: { title: TestRail API, version: "2.0.0" }
servers:
  - url: https://testrail.example.com/index.php?/api/v2
paths:
  /get_user/{user}:
    get:
      operationId: getUser
      parameters:
        - { name: user, in: path, required: true, schema: { type: string } }
        - { name: limit, in: query, schema: { type: integer } }
      responses:
        "200": { description: OK }
`
	tests := []struct {
		value string
		want  string
	}{
		{"a&limit=1", "/api/v2/get_user/a%26limit%3D1&limit=50"},
		{"a=b", "/api/v2/get_user/a%3Db&limit=50"},
		{"a+b", "/api/v2/get_user/a%2Bb&limit=50"},
		{"a b", "/api/v2/get_user/a%20b&limit=50"},
		{"100%", "/api/v2/get_user/100%25&limit=50"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			api, requests := recordRequests(t)
			opts := []RegisterToolsOption{WithServerURL(api.URL + "/index.php?/api/v2")}

			result, err := callTool(t, spec, api.Client(), opts, "getUser", map[string]any{"user": tt.value, "limit": 50})
			require.NoError(t, err)
			require.False(t, result.IsError)

			r := <-requests
			assert.Equal(t, "/index.php", r.URL.Path)
			assert.Equal(t, tt.want, r.URL.RawQuery)
			assert.Equal(t, []string{"50"}, r.URL.Query()["limit"])
		})
	}
}

func TestRegisterToolsAppendsPathToServerURLQuery(t *testing.T) {
	api, requests := recordRequests(t)
	spec := fmt.Sprintf(`
openapi: 3.1.0
info: { title: Versioned API, version: "1.0.0" }
servers:
  - url: %q
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200": { description: OK }
`, api.URL+"/api?api-version=2")

	// OpenAPI appends the operation path to the server URL as written,
	// so the path follows an existing query rather than the server URL's path.
	result, err := callTool(t, spec, api.Client(), nil, "listItems", nil)
	require.NoError(t, err)
	require.False(t, result.IsError)

	r := <-requests
	assert.Equal(t, "/api", r.URL.Path)
	assert.Equal(t, "api-version=2/items", r.URL.RawQuery)
}
