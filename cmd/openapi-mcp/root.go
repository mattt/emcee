package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pb33f/libopenapi"
	"github.com/spf13/cobra"

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
		specURL := args[0]
		doc, err := loadOpenAPISpec(specURL)
		if err != nil {
			return fmt.Errorf("error loading OpenAPI spec: %v", err)
		}

		server := mcp.NewServer(doc, specURL)
		transport := mcp.NewStdioTransport(server, os.Stdin, os.Stdout, os.Stderr)
		return transport.Run()
	},
}

func loadOpenAPISpec(url string) (libopenapi.Document, error) {
	resp, err := http.Get(url)
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
