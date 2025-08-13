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

		var buffer bytes.Buffer
		var braceCount int // Tracks the balance of curly braces to find a complete JSON object.

		readBuf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				var n int
				var err error

				if fd != -1 {
					n, err = unix.Read(fd, readBuf)
				} else {
					n, err = t.reader.Read(readBuf)
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

				// Append read data into the buffer.
				buffer.Write(readBuf[:n])

				// Process the buffer only if it contains data.
				for buffer.Len() > 0 {
					// Trim leading whitespace from the buffer.
					trimmed := bytes.TrimLeft(buffer.Bytes(), " \t\n\r")
					if len(trimmed) == 0 {
						buffer.Reset()
						continue
					}
					buffer = *bytes.NewBuffer(trimmed)

					// If the buffer doesn't start with a '{', we are not at the beginning of a JSON object.
					// This simple check waits for an object to start. More complex scenarios could involve
					// searching for the next '{', but for now, we'll break and read more data.
					if buffer.Bytes()[0] != '{' {
						if braceCount == 0 {
							break
						}
					}

					// Scan the buffer to find the end of a complete JSON object.
					var end int = -1
					braceCount = 0
					inString := false

					scan := buffer.Bytes()
					for i, char := range scan {
						// Toggle inString flag if a non-escaped quote is found.
						if char == '"' && (i == 0 || scan[i-1] != '\\') {
							inString = !inString
						}

						// Skip brace counting if inside a string literal.
						if inString {
							continue
						}

						if char == '{' {
							braceCount++
						} else if char == '}' {
							braceCount--
							// When braceCount is zero, we've found a complete JSON object.
							if braceCount == 0 {
								end = i + 1
								break
							}
						}
					}

					// If a complete object is found, send it to the lines channel.
					if end != -1 {
						fullMessage := buffer.Next(end)
						select {
						case <-ctx.Done():
							return nil
						case lines <- string(fullMessage):
						}
					} else {
						// If the object is not yet complete, break the loop to read more data.
						break
					}
				}
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
