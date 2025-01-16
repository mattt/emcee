package mcp

import (
	"bufio"
	"context"
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
	bufOut  *bufio.Writer
	errOut  io.Writer
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(handler jsonrpc.Handler, in io.Reader, out io.Writer, errOut io.Writer) *Transport {
	scanner := bufio.NewScanner(in)
	// Set a reasonable max size for each line
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	bufOut := bufio.NewWriter(out)
	return &Transport{
		handler: handler,
		scanner: scanner,
		writer:  json.NewEncoder(bufOut),
		bufOut:  bufOut,
		errOut:  errOut,
	}
}

// Run starts the transport loop, reading from stdin and writing to stdout
func (t *Transport) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if !t.scanner.Scan() {
				if err := t.scanner.Err(); err != nil {
					return fmt.Errorf("scanner error: %v", err)
				}
				return nil
			}

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
				t.bufOut.Flush()
				continue
			}

			response := t.handler.Handle(request)
			if err := t.writer.Encode(response); err != nil {
				fmt.Fprintf(t.errOut, "Error encoding response: %v\n", err)
			}
			t.bufOut.Flush()
		}
	}
}
