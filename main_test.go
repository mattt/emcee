package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration(t *testing.T) {
	// Build the emcee binary for testing
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "emcee")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "cmd/emcee/main.go")
	require.NoError(t, buildCmd.Run(), "Failed to build emcee binary")

	// Start emcee with the embedded test OpenAPI spec
	specPath := "testdata/api.weather.gov/openapi.json"
	cmd := exec.Command(binaryPath, specPath)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	// Ensure cleanup
	defer func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Give the process a moment to initialize
	time.Sleep(100 * time.Millisecond)

	// Perform MCP handshake (initialize + initialized), then list tools
	scanner := bufio.NewScanner(stdout)

	// initialize
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "emcee-test",
				"version": "dev",
			},
		},
	}
	initJSON, err := json.Marshal(initReq)
	require.NoError(t, err)
	initJSON = append(initJSON, '\n')
	_, err = stdin.Write(initJSON)
	require.NoError(t, err)
	require.True(t, scanner.Scan(), "Expected initialize response")

	// notifications/initialized
	initialized := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}
	initdJSON, err := json.Marshal(initialized)
	require.NoError(t, err)
	initdJSON = append(initdJSON, '\n')
	_, err = stdin.Write(initdJSON)
	require.NoError(t, err)

	// tools/list
	listReqID := 2
	listReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      listReqID,
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	listJSON, err := json.Marshal(listReq)
	require.NoError(t, err)
	listJSON = append(listJSON, '\n')
	_, err = stdin.Write(listJSON)
	require.NoError(t, err)
	require.True(t, scanner.Scan(), "Expected tools/list response")

	var response struct {
		JSONRPC string `json:"jsonrpc"`
		Result  struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
		ID int `json:"id"`
	}

	err = json.Unmarshal(scanner.Bytes(), &response)
	require.NoError(t, err, "Failed to parse JSON response")

	// Verify response
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, listReqID, response.ID)
	assert.NotEmpty(t, response.Result.Tools, "Expected at least one tool in response")

	// Find and verify the point tool
	var pointTool struct {
		Name        string
		Description string
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		}
	}

	foundPointTool := false
	for _, tool := range response.Result.Tools {
		if tool.Name == "point" {
			foundPointTool = true
			err := json.Unmarshal(tool.InputSchema, &pointTool.InputSchema)
			require.NoError(t, err)
			pointTool.Name = tool.Name
			pointTool.Description = tool.Description
			break
		}
	}

	require.True(t, foundPointTool, "Expected to find point tool")
	assert.Equal(t, "point", pointTool.Name)
	assert.Contains(t, pointTool.Description, "Returns metadata about a given latitude/longitude point")

	// Verify point tool has proper parameter schema
	assert.Equal(t, "object", pointTool.InputSchema.Type)
	assert.Contains(t, pointTool.InputSchema.Properties, "point", "Point tool should have 'point' parameter")

	pointParam := pointTool.InputSchema.Properties["point"].(map[string]interface{})
	assert.Equal(t, "string", pointParam["type"])
	assert.Contains(t, pointParam["description"].(string), "Point (latitude, longitude)")
	assert.Contains(t, pointTool.InputSchema.Required, "point", "Point parameter should be required")

	var zoneTool struct {
		Name        string
		Description string
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		}
	}

	foundZoneTool := false
	for _, tool := range response.Result.Tools {
		if tool.Name == "zone" {
			foundZoneTool = true
			err := json.Unmarshal(tool.InputSchema, &zoneTool.InputSchema)
			require.NoError(t, err)
			zoneTool.Name = tool.Name
			zoneTool.Description = tool.Description
			break
		}
	}

	require.True(t, foundZoneTool, "Expected to find zone tool")
	assert.Equal(t, "zone", zoneTool.Name)
	assert.Contains(t, zoneTool.Description, "Returns metadata about a given zone")

	// Verify zone tool has proper parameter schema
	assert.Equal(t, "object", zoneTool.InputSchema.Type)
	assert.Contains(t, zoneTool.InputSchema.Properties, "zoneId", "Zone tool should have 'zoneId' parameter")

	typeParam := zoneTool.InputSchema.Properties["type"].(map[string]interface{})
	assert.Equal(t, "string", typeParam["type"])
	assert.Contains(t, typeParam["description"].(string), "Zone type")
	assert.Contains(t, typeParam["description"].(string), "Allowed values: land, marine, ")
	assert.Contains(t, zoneTool.InputSchema.Required, "type", "type parameter should be required")

	zoneIdParam := zoneTool.InputSchema.Properties["zoneId"].(map[string]interface{})
	assert.Equal(t, "string", zoneIdParam["type"])
	assert.Contains(t, zoneIdParam["description"].(string), "NWS public zone/county identifier")
	assert.Contains(t, zoneIdParam["description"].(string), "UGC identifier for a NWS")
	assert.Contains(t, zoneTool.InputSchema.Required, "zoneId", "zoneId parameter should be required")
}
