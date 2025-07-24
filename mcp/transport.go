package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattt/emcee/jsonrpc"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

// Transport handles the communication between stdin/stdout and the MCP server
type Transport struct {
	reader io.Reader
	writer io.Writer
	logger io.Writer
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(in io.Reader, out io.Writer, logger io.Writer) *Transport {
	return &Transport{
		reader: in,
		writer: out,
		logger: logger,
	}
}

// setupNonBlockingFd duplicates a file descriptor and sets it to non-blocking mode
func setupNonBlockingFd(f interface{}) (fd int, cleanup func() error, err error) {
	file, ok := f.(*os.File)
	if !ok {
		return -1, func() error { return nil }, nil
	}

	fd, err = unix.Dup(int(file.Fd()))
	if err != nil {
		return -1, func() error { return nil }, fmt.Errorf("failed to duplicate file descriptor: %w", err)
	}

	cleanup = func() error { return unix.Close(fd) }

	if err := unix.SetNonblock(fd, true); err != nil {
		cleanup()
		return -1, func() error { return nil }, fmt.Errorf("failed to set non-blocking mode: %w", err)
	}

	return fd, cleanup, nil
}

// Run starts the transport loop, reading from stdin and writing to stdout
func (t *Transport) Run(ctx context.Context, handler func(jsonrpc.Request) *jsonrpc.Response) error {
	g, ctx := errgroup.WithContext(ctx)
	lines := make(chan string)
	responses := make(chan *jsonrpc.Response)

	// Writer goroutine
	g.Go(func() error {
		fd, cleanup, err := setupNonBlockingFd(t.writer)
		if err != nil {
			return err
		}
		defer cleanup()

		var buf bytes.Buffer
		buf.Grow(4096)
		for {
			select {
			case <-ctx.Done():
				return nil
			case response, ok := <-responses:
				if !ok {
					return nil
				}

				buf.Reset()
				enc := json.NewEncoder(&buf)
				if err := enc.Encode(response); err != nil {
					fmt.Fprintf(t.logger, "Error marshaling response: %v\n", err)
					continue
				}

				data := buf.Bytes()
				for len(data) > 0 {
					select {
					case <-ctx.Done():
						return nil
					default:
						var n int
						var err error

						if fd != -1 {
							n, err = unix.Write(fd, data)
						} else {
							n, err = t.writer.Write(data)
						}

						if err != nil {
							if fd != -1 && err == unix.EAGAIN {
								time.Sleep(time.Millisecond)
								continue
							}
							if err == io.EOF {
								return nil
							}
							return err
						}
						if n == 0 {
							return nil
						}

						data = data[n:]
					}
				}
			}
		}
	})

	// Reader goroutine
	g.Go(func() error {
		fd, cleanup, err := setupNonBlockingFd(t.reader)
		if err != nil {
			return err
		}
		defer cleanup()
		defer close(lines)

		buf := make([]byte, 1)
		line := make([]byte, 0, 4096)
		for {
			select {
			case <-ctx.Done():
				return nil
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
						time.Sleep(time.Millisecond)
						continue
					}
					if err == io.EOF {
						return nil
					}
					return err
				}
				if n == 0 {
					return nil
				}

				if buf[0] == '\n' || buf[0] == '\r' {
					if len(line) > 0 {
						select {
						case <-ctx.Done():
							return nil
						case lines <- string(line):
							line = line[:0]
						}
					}
					continue
				}

				line = append(line, buf[0])
			}
		}
	})

	// Handler goroutine
	g.Go(func() error {
		defer close(responses)
		for {
			select {
			case <-ctx.Done():
				return nil
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
					select {
					case <-ctx.Done():
						return nil
					case responses <- &response:
					}
					continue
				}

				response := handler(request)
				if response != nil {
					select {
					case <-ctx.Done():
						return nil
					case responses <- response:
					}
				}
			}
		}
	})

	return g.Wait()
}
