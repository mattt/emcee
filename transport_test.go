package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockServer struct {
	handleRequestFunc func(JsonRpcRequest) JsonRpcResponse
}

func (m *mockServer) HandleRequest(req JsonRpcRequest) JsonRpcResponse {
	return m.handleRequestFunc(req)
}

func TestTransport_Run(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		mockResponse  JsonRpcResponse
		expectedOut   string
		expectedErr   string
		expectSuccess bool
	}{
		{
			name: "successful request",
			input: `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}
`,
			mockResponse: JsonRpcResponse{
				JsonRpc: "2.0",
				Result: map[string]interface{}{
					"tools": []interface{}{},
				},
				Id: 1,
			},
			expectedOut: `{"jsonrpc":"2.0","result":{"tools":[]},"id":1}
`,
			expectSuccess: true,
		},
		{
			name: "invalid JSON request",
			input: `{"invalid json
`,
			expectedOut: `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error","data":{"Offset":15}},"id":null}
`,
			expectSuccess: true,
		},
		{
			name: "multiple requests",
			input: `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}
{"jsonrpc": "2.0", "method": "tools/call", "id": 2}
`,
			mockResponse: JsonRpcResponse{
				JsonRpc: "2.0",
				Result:  "success",
				Id:      0,
			},
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
				handleRequestFunc: func(JsonRpcRequest) JsonRpcResponse {
					return tt.mockResponse
				},
			}

			// Setup input and output buffers
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			errOut := &bytes.Buffer{}

			// Create and run transport
			transport := NewTransport(mockServer, in, out, errOut)
			err := transport.Run()

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

	// Create a real server instance
	doc, err := libopenapi.NewDocument(specData)
	require.NoError(t, err)
	server := NewServer(doc, "http://test.api")

	// Test tools/list request
	input := `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}
`
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	transport := NewTransport(server, in, out, errOut)
	err = transport.Run()
	require.NoError(t, err)

	// Verify the response
	var response JsonRpcResponse
	err = json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.JsonRpc)
	assert.Equal(t, float64(1), response.Id)
	assert.Nil(t, response.Error)

	// Verify the response contains a tools list
	result, ok := response.Result.(map[string]interface{})
	require.True(t, ok)
	tools, ok := result["tools"].([]interface{})
	require.True(t, ok)
	assert.NotNil(t, tools)
}
