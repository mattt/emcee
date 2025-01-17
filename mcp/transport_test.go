package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"net/http/httptest"

	"github.com/loopwork-ai/openapi-mcp/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockServer struct {
	handleRequestFunc func(jsonrpc.Request) jsonrpc.Response
}

func (m *mockServer) Handle(req jsonrpc.Request) jsonrpc.Response {
	return m.handleRequestFunc(req)
}

func TestTransport_Run(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		mockResponse  jsonrpc.Response
		expectedOut   string
		expectedErr   string
		expectSuccess bool
	}{
		{
			name:  "successful request",
			input: `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}`,
			mockResponse: jsonrpc.NewResponse(1, map[string]interface{}{
				"tools": []interface{}{},
			}, nil),
			expectedOut: `{"jsonrpc":"2.0","result":{"tools":[]},"id":1}
`,
			expectSuccess: true,
		},
		{
			name:  "invalid JSON request",
			input: `{"jsonrpc": "2.0" method: invalid}`,
			expectedOut: `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error","data":{"Offset":19}},"id":0}
`,
			expectSuccess: true,
		},
		{
			name: "multiple requests",
			input: `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}
{"jsonrpc": "2.0", "method": "tools/call", "id": 2}`,
			mockResponse: jsonrpc.NewResponse(0, "success", nil),
			expectedOut: `{"jsonrpc":"2.0","result":"success","id":0}
{"jsonrpc":"2.0","result":"success","id":0}
`,
			expectSuccess: true,
		},
		{
			name:          "empty input",
			input:         "",
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock server
			mockServer := &mockServer{
				handleRequestFunc: func(jsonrpc.Request) jsonrpc.Response {
					return tt.mockResponse
				},
			}

			// Ensure input ends with newline
			input := tt.input
			if input != "" && !strings.HasSuffix(input, "\n") {
				input += "\n"
			}

			// Setup input and output buffers
			in := strings.NewReader(input)
			out := &bytes.Buffer{}
			errOut := &bytes.Buffer{}

			// Create and run transport
			transport := NewStdioTransport(mockServer, in, out, errOut)
			err := transport.Run(context.Background())

			if tt.expectSuccess {
				assert.NoError(t, err)
				if tt.expectedOut != "" {
					assert.Equal(t, tt.expectedOut, out.String())
				}
				if tt.expectedErr != "" {
					assert.Equal(t, tt.expectedErr, errOut.String())
				}
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestTransport_Integration(t *testing.T) {
	// Create a minimal OpenAPI spec for testing
	specData := []byte(`{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {}
	}`)

	// Create a test HTTP server to serve the spec
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(specData)
	}))
	defer ts.Close()

	// Create a real server instance
	server, err := NewServer(
		WithClient(http.DefaultClient),
		WithSpecURL(ts.URL),
	)
	require.NoError(t, err)

	// Test tools/list request
	input := `{"jsonrpc": "2.0", "method": "tools/list", "params": {}, "id": 1}
`
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	transport := NewStdioTransport(server, in, out, errOut)
	err = transport.Run(context.Background())
	require.NoError(t, err)

	// Verify the response
	var response jsonrpc.Response
	err = json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.Nil(t, response.Error)

	// Verify the response contains a tools list
	result, ok := response.Result.(map[string]interface{})
	require.True(t, ok)
	tools, ok := result["tools"].([]interface{})
	require.True(t, ok)
	assert.NotNil(t, tools)

	// Test tools/call request
	input = `{"jsonrpc": "2.0", "method": "tools/call", "params": {}, "id": 2}
`
	in = strings.NewReader(input)
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}

	transport = NewStdioTransport(server, in, out, errOut)
	err = transport.Run(context.Background())
	require.NoError(t, err)

	// Verify the response
	err = json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 2, response.ID.Value())
	assert.Equal(t, jsonrpc.ErrMethodNotFound, response.Error.Code)
	assert.Equal(t, "Method not found", response.Error.Message)

	// Test tools/call request
	input = `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "test"}, "id": 4}
`
	in = strings.NewReader(input)
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}

	transport = NewStdioTransport(server, in, out, errOut)
	err = transport.Run(context.Background())
	require.NoError(t, err)

	// Verify the response
	err = json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 4, response.ID.Value())

	assert.Equal(t, jsonrpc.ErrMethodNotFound, response.Error.Code)
	assert.Equal(t, "Method not found", response.Error.Message)

	// Test tools/call request
	input = `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "test", "arguments": {}}, "id": 3}
`
	in = strings.NewReader(input)
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}

	transport = NewStdioTransport(server, in, out, errOut)
	err = transport.Run(context.Background())
	require.NoError(t, err)

	// Verify the response
	err = json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 3, response.ID.Value())
	assert.NotNil(t, response.Error)

	// Test tools/list request
	input = `{"jsonrpc": "2.0", "method": "tools/list", "params": {}, "id": 1}
`
	in = strings.NewReader(input)
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}

	transport = NewStdioTransport(server, in, out, errOut)
	err = transport.Run(context.Background())
	require.NoError(t, err)

	// Verify the response
	err = json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.Version)
	assert.Equal(t, 1, response.ID.Value())
	assert.NotNil(t, response.Error)
}
