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
	cmd := exec.Command(binaryPath, "testdata/openapi.json")
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

	// Prepare and send JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"params":  map[string]interface{}{},
		"id":      1,
	}

	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)
	requestJSON = append(requestJSON, '\n')

	_, err = stdin.Write(requestJSON)
	require.NoError(t, err)

	// Read response using a scanner
	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan(), "Expected to read a response line")

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
	assert.Equal(t, 1, response.ID)
	assert.NotEmpty(t, response.Result.Tools, "Expected at least one tool in response")
}
