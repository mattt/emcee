package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loopwork-ai/emcee/jsonrpc"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

// Transport handles the communication between stdin/stdout and the MCP server
type Transport struct {
	reader io.Reader
	writer *json.Encoder
	logger io.Writer
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(in io.Reader, out io.Writer, logger io.Writer) *Transport {
	return &Transport{
		reader: in,
		writer: json.NewEncoder(out),
		logger: logger,
	}
}

// Run starts the transport loop, reading from stdin and writing to stdout
func (t *Transport) Run(ctx context.Context, handler func(jsonrpc.Request) jsonrpc.Response) error {
	g, ctx := errgroup.WithContext(ctx)
	lines := make(chan string)

	g.Go(func() error {
		defer close(lines)

		// Setup non-blocking mode for input if we have a file descriptor
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
					if err := t.writeWithRetry(response); err != nil {
						fmt.Fprintf(t.logger, "Error encoding response: %v\n", err)
					}
					continue
				}

				response := handler(request)
				if err := t.writeWithRetry(response); err != nil {
					fmt.Fprintf(t.logger, "Error encoding response: %v\n", err)
				}
			}
		}
	})

	return g.Wait()
}

func (t *Transport) writeWithRetry(response jsonrpc.Response) error {
	for {
		err := t.writer.Encode(response)
		if err == nil {
			return nil
		}

		if err != unix.EAGAIN {
			return err
		}

		// If we get EAGAIN, sleep briefly and retry
		time.Sleep(time.Millisecond)
	}
}
