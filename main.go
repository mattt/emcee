package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pb33f/libopenapi"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "openapi-mcp [openapi-spec-url]",
	Short: "OpenAPI MCP implements the stdio transport for MCP",
	Long: `OpenAPI MCP is a CLI tool that implements the stdio transport for MCP.
It takes an OpenAPI specification URL as input and processes JSON-RPC requests
from stdin, making corresponding API calls and returning JSON-RPC responses to stdout.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		specURL := args[0]
		doc, err := loadOpenAPISpec(specURL)
		if err != nil {
			return fmt.Errorf("error loading OpenAPI spec: %v", err)
		}

		server := NewServer(doc, specURL)
		transport := NewTransport(server, os.Stdin, os.Stdout, os.Stderr)
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
		os.Exit(1)
	}
}
