package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Transport handles the communication between stdin/stdout and the MCP server
type Transport struct {
	handler RequestHandler
	reader  *bufio.Reader
	writer  *json.Encoder
	errOut  io.Writer
}

// NewTransport creates a new Transport instance
func NewTransport(handler RequestHandler, in io.Reader, out io.Writer, errOut io.Writer) *Transport {
	return &Transport{
		handler: handler,
		reader:  bufio.NewReader(in),
		writer:  json.NewEncoder(out),
		errOut:  errOut,
	}
}

// Run starts the transport loop, reading from stdin and writing to stdout
func (t *Transport) Run() error {
	for {
		line, err := t.reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(t.errOut, "Error reading input: %v\n", err)
			continue
		}

		var request JsonRpcRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			response := JsonRpcResponse{
				JsonRpc: "2.0",
				Error: &JsonRpcError{
					Code:    -32700,
					Message: "Parse error",
					Data:    err,
				},
			}
			t.writer.Encode(response)
			continue
		}

		response := t.handler.HandleRequest(request)
		if err := t.writer.Encode(response); err != nil {
			fmt.Fprintf(t.errOut, "Error encoding response: %v\n", err)
		}
	}
	return nil
}
