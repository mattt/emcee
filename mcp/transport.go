package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/loopwork-ai/openapi-mcp/jsonrpc"
)

// Transport handles the communication between stdin/stdout and the MCP server
type Transport struct {
	handler jsonrpc.Handler
	scanner *bufio.Scanner
	writer  *json.Encoder
	errOut  io.Writer
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(handler jsonrpc.Handler, in io.Reader, out io.Writer, errOut io.Writer) *Transport {
	scanner := bufio.NewScanner(in)
	// Set a reasonable max size for each line
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	return &Transport{
		handler: handler,
		scanner: scanner,
		writer:  json.NewEncoder(out),
		errOut:  errOut,
	}
}

// Run starts the transport loop, reading from stdin and writing to stdout
func (t *Transport) Run() error {
	for t.scanner.Scan() {
		line := t.scanner.Text()
		if line == "" {
			continue
		}

		var request jsonrpc.Request
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			response := jsonrpc.NewResponse(nil, nil, jsonrpc.NewError(jsonrpc.ErrParse, err))
			if err := t.writer.Encode(response); err != nil {
				fmt.Fprintf(t.errOut, "Error encoding response: %v\n", err)
			}
			continue
		}

		response := t.handler.Handle(request)
		if err := t.writer.Encode(response); err != nil {
			fmt.Fprintf(t.errOut, "Error encoding response: %v\n", err)
		}
	}

	if err := t.scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %v", err)
	}
	return nil
}
