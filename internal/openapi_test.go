package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
