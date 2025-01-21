package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/loopwork-ai/emcee/jsonrpc"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

// Transport handles the communication between stdin/stdout and the MCP server
type Transport struct {
	handler jsonrpc.Handler
	reader  io.Reader
	writer  *json.Encoder
	bufOut  *bufio.Writer
	errOut  io.Writer
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(handler jsonrpc.Handler, in io.Reader, out io.Writer, errOut io.Writer) *Transport {
	bufOut := bufio.NewWriter(out)
	return &Transport{
		handler: handler,
		reader:  in,
		writer:  json.NewEncoder(bufOut),
		bufOut:  bufOut,
		errOut:  errOut,
	}
}

// Run starts the transport loop, reading from stdin and writing to stdout
func (t *Transport) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	lines := make(chan string)

	g.Go(func() error {
		defer close(lines)

		// Setup non-blocking mode if we have a file descriptor
		var fd = -1
		if f, ok := t.reader.(*os.File); ok {
			fd = int(f.Fd())
			if err := unix.SetNonblock(fd, true); err != nil {
				return fmt.Errorf("failed to set non-blocking mode: %v", err)
			}
		}

		var line []byte
		buf := make([]byte, 1)

		for {
			select {
			case <-ctx.Done():
				return nil // Return nil on context cancellation
			default:
				var n int
				var err error

				if fd != -1 {
					n, err = unix.Read(fd, buf)
				} else {
					n, err = t.reader.Read(buf)
				}

				if err != nil {
					if fd != -1 && err == unix.EAGAIN {
						continue
					}
					if err == io.EOF {
						return nil // Return nil on EOF
					}
					return err
				}
				if n == 0 {
					return nil // Return nil on EOF (n == 0)
				}

				// Handle newlines
				if buf[0] == '\n' || buf[0] == '\r' {
					if len(line) > 0 {
						select {
						case <-ctx.Done():
							return nil // Return nil on context cancellation
						case lines <- string(line):
							line = line[:0]
						}
					}
					continue
				}

				// Build the line
				line = append(line, buf[0])
			}
		}
	})

	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil // Return nil on context cancellation
			case line, ok := <-lines:
				if !ok {
					return nil
				}
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
	})

	return g.Wait()
}
