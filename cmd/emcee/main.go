package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/loopwork-ai/emcee/mcp"
)

var rootCmd = &cobra.Command{
	Use:   "emcee [spec-path-or-url]",
	Short: "An MCP server for a given OpenAPI specification",
	Long: `emcee is a CLI tool that provides an MCP stdio transport for a given OpenAPI specification.
It takes an OpenAPI specification path or URL as input and processes JSON-RPC requests
from stdin, making corresponding API calls and returning JSON-RPC responses to stdout.

The spec-path-or-url argument can be:
- A local file path
- An HTTP(S) URL
- "-" to read from stdin`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		g, ctx := errgroup.WithContext(ctx)

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		if !verbose {
			logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		}

		g.Go(func() error {
			var opts []mcp.ServerOption

			opts = append(opts, mcp.WithLogger(logger))

			client := &http.Client{}
			opts = append(opts, mcp.WithClient(client))

			var specData []byte
			var err error
			var rpcInput io.Reader = os.Stdin

			if args[0] == "-" {
				logger.Info("reading spec from stdin")

				// When reading spec from stdin, we need to use /dev/tty for RPC input
				// because stdin isn't a TTY when reading from a pipe
				tty, err := os.Open("/dev/tty")
				if err != nil {
					return fmt.Errorf("error opening /dev/tty: %w", err)
				}
				defer tty.Close()
				rpcInput = tty

				// Read spec from stdin
				specData, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("error reading OpenAPI spec from stdin: %w", err)
				}
			} else if strings.HasPrefix(args[0], "http://") || strings.HasPrefix(args[0], "https://") {
				logger.Info("reading spec from URL", "url", args[0])

				req, err := http.NewRequest(http.MethodGet, args[0], nil)
				if err != nil {
					return fmt.Errorf("error creating request: %w", err)
				}

				// Apply auth header if provided
				if auth != "" {
					req.Header.Set("Authorization", auth)
				}

				resp, err := client.Do(req)
				if err != nil {
					return fmt.Errorf("error downloading spec: %w", err)
				}
				if resp.Body == nil {
					return fmt.Errorf("no response body from %s", args[0])
				}
				defer resp.Body.Close()

				specData, err = io.ReadAll(resp.Body)
				if err != nil {
					return fmt.Errorf("error reading spec from %s: %w", args[0], err)
				}
			} else {
				logger.Info("reading spec from file", "file", args[0])

				// Clean the file path to remove any . or .. segments and ensure consistent separators
				cleanPath := filepath.Clean(args[0])

				// Check if file exists and is readable before attempting to read
				info, err := os.Stat(cleanPath)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("spec file does not exist: %s", cleanPath)
					}
					return fmt.Errorf("error accessing spec file %s: %w", cleanPath, err)
				}

				// Ensure it's a regular file, not a directory
				if info.IsDir() {
					return fmt.Errorf("specified path is a directory, not a file: %s", cleanPath)
				}

				// Check file size to prevent loading extremely large files
				if info.Size() > 100*1024*1024 { // 100MB limit
					return fmt.Errorf("spec file too large (max 100MB): %s", cleanPath)
				}

				specData, err = os.ReadFile(cleanPath)
				if err != nil {
					return fmt.Errorf("error reading spec file %s: %w", cleanPath, err)
				}
			}

			if len(specData) == 0 {
				return fmt.Errorf("no OpenAPI spec data provided")
			}
			opts = append(opts, mcp.WithSpecData(specData))

			if auth != "" {
				opts = append(opts, mcp.WithAuth(auth))
			}

			opts = append(opts, mcp.WithLogger(logger))

			server, err := mcp.NewServer(opts...)
			if err != nil {
				return fmt.Errorf("error creating server: %w", err)
			}

			transport := mcp.NewStdioTransport(rpcInput, os.Stdout, os.Stderr)
			return transport.Run(ctx, server.Handle)
		})

		return g.Wait()
	},
}

var (
	auth    string
	verbose bool
)

func init() {
	rootCmd.Flags().StringVar(&auth, "auth", "", "Authorization header value (e.g. 'Bearer token123' or 'Basic dXNlcjpwYXNz')")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging to stderr")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
