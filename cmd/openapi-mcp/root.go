package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/pb33f/libopenapi"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/loopwork-ai/openapi-mcp/mcp"
)

var rootCmd = &cobra.Command{
	Use:   "openapi-mcp [openapi-spec-url]",
	Short: "An MCP server for a given OpenAPI specification",
	Long: `OpenAPI MCP is a CLI tool that provides an MCP stdio transport for a given OpenAPI specification.
It takes an OpenAPI specification URL as input and processes JSON-RPC requests
from stdin, making corresponding API calls and returning JSON-RPC responses to stdout.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			client := http.DefaultClient

			specURL := args[0]
			doc, err := loadOpenAPISpec(ctx, specURL, client)
			if err != nil {
				return fmt.Errorf("error loading OpenAPI spec: %v", err)
			}

			server, err := mcp.NewServer(doc, specURL, client)
			if err != nil {
				return fmt.Errorf("error creating server: %v", err)
			}

			transport := mcp.NewStdioTransport(server, os.Stdin, os.Stdout, os.Stderr)
			return transport.Run(ctx)
		})

		return g.Wait()
	},
}

func loadOpenAPISpec(ctx context.Context, url string, client *http.Client) (libopenapi.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	specData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return libopenapi.NewDocument(specData)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
