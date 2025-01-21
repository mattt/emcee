package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/loopwork-ai/emcee/mcp"
)

var rootCmd = &cobra.Command{
	Use:   "emcee [openapi-spec-url]",
	Short: "An MCP server for a given OpenAPI specification",
	Long: `emcee is a CLI tool that provides an MCP stdio transport for a given OpenAPI specification.
It takes an OpenAPI specification URL as input and processes JSON-RPC requests
from stdin, making corresponding API calls and returning JSON-RPC responses to stdout.

If "-" is provided as the argument, the OpenAPI specification will be read from stdin.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			var opts []mcp.ServerOption

			client := &http.Client{}
			opts = append(opts, mcp.WithClient(client))

			var specData []byte
			var err error
			var rpcInput io.Reader = os.Stdin

			if args[0] == "-" {
				// When reading spec from stdin, we need to use /dev/tty for RPC input
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
			} else {
				// When reading spec from a URL, we need to download it
				req, err := http.NewRequest(http.MethodGet, args[0], nil)
				if err != nil {
					return fmt.Errorf("error creating request: %w", err)
				}

				if auth != "" {
					req.Header.Set("Authorization", auth)
				}

				resp, err := client.Do(req)
				if err != nil {
					return fmt.Errorf("error downloading spec: %w", err)
				}
				defer resp.Body.Close()

				specData, err = io.ReadAll(resp.Body)
				if err != nil {
					return fmt.Errorf("error reading spec from %s: %w", args[0], err)
				}
			}

			if len(specData) == 0 {
				return fmt.Errorf("no OpenAPI spec data provided")
			}
			opts = append(opts, mcp.WithSpecData(specData))

			if auth != "" {
				opts = append(opts, mcp.WithAuth(auth))
			}

			if verbose {
				logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))
				opts = append(opts, mcp.WithLogger(logger))
			}

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
