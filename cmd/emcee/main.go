package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/mattt/emcee/internal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var rootCmd = &cobra.Command{
	Use:   "emcee [spec-path-or-url]",
	Short: "Creates an MCP server for an OpenAPI specification",
	Long: `emcee is a CLI tool that provides an Model Context Protocol (MCP) stdio transport for a given OpenAPI specification.
It takes an OpenAPI specification path or URL as input and processes JSON-RPC requests from stdin, making corresponding API calls and returning JSON-RPC responses to stdout.

The spec-path-or-url argument can be:
- A local file path (e.g. ./openapi.json)
- An HTTP(S) URL (e.g. https://api.example.com/openapi.json)
- "-" to read from stdin

By default, a GET request with no additional headers is made to the spec URL to download the OpenAPI specification.

If additional authentication is required to download the specification, you can first download it to a local file using your preferred HTTP client with the necessary authentication headers, and then provide the local file path to emcee.

Authentication values can be provided directly or as 1Password secret references (e.g. op://vault/item/field). When using 1Password references:
- The 1Password CLI (op) must be installed and available in your PATH
- You must be signed in to 1Password
- The reference must be in the format op://vault/item/field
- The secret will be securely retrieved at runtime using the 1Password CLI
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set up context and signal handling
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		// Set up error group
		g, ctx := errgroup.WithContext(ctx)

		// Set up logger
		var logger *slog.Logger
		switch {
		case silent:
			logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		case verbose:
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
		default:
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))
		}

		g.Go(func() error {
			// Read OpenAPI specification data
			var specData []byte
			if args[0] == "-" {
				logger.Info("reading spec from stdin")

				// When reading the OpenAPI spec from stdin, we need to read RPC input from /dev/tty
				// since stdin is being used for the spec data and isn't available for interactive I/O
				origStdin := os.Stdin
				tty, err := os.Open("/dev/tty")
				if err != nil {
					return fmt.Errorf("error opening /dev/tty: %w", err)
				}
				defer tty.Close()

				// Read spec from original stdin
				specData, err = io.ReadAll(origStdin)
				if err != nil {
					return fmt.Errorf("error reading OpenAPI spec from stdin: %w", err)
				}
				// Redirect SDK stdio transport to use /dev/tty for input
				os.Stdin = tty
			} else if strings.HasPrefix(args[0], "http://") || strings.HasPrefix(args[0], "https://") {
				logger.Info("reading spec from URL", "url", args[0])

				// Create HTTP request
				req, err := http.NewRequest(http.MethodGet, args[0], nil)
				if err != nil {
					return fmt.Errorf("error creating request: %w", err)
				}

				// Make HTTP request
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("error downloading spec: %w", err)
				}
				if resp.Body == nil {
					return fmt.Errorf("no response body from %s", args[0])
				}
				defer resp.Body.Close()

				// Read spec from response body
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

				// Read spec from file
				specData, err = os.ReadFile(cleanPath)
				if err != nil {
					return fmt.Errorf("error reading spec file %s: %w", cleanPath, err)
				}
			}

			// Build HTTP client with optional auth header
			client, err := internal.RetryableClient(retries, timeout, rps, logger)
			if err != nil {
				return fmt.Errorf("error creating client: %w", err)
			}
			if bearerAuth != "" {
				resolvedAuth, wasSecret, err := internal.ResolveSecretReference(ctx, bearerAuth)
				if err != nil {
					return fmt.Errorf("error resolving bearer auth: %w", err)
				}
				if wasSecret {
					logger.Debug("resolved bearer auth from 1Password")
				}
				headers := http.Header{}
				headers.Add("Authorization", "Bearer "+resolvedAuth)
				client.Transport = &internal.HeaderTransport{Base: client.Transport, Headers: headers}
			} else if basicAuth != "" {
				resolvedAuth, wasSecret, err := internal.ResolveSecretReference(ctx, basicAuth)
				if err != nil {
					return fmt.Errorf("error resolving basic auth: %w", err)
				}
				if wasSecret {
					logger.Debug("resolved basic auth from 1Password")
				}
				var value string
				if strings.Contains(resolvedAuth, ":") {
					value = base64.StdEncoding.EncodeToString([]byte(resolvedAuth))
				} else {
					value = resolvedAuth
				}
				headers := http.Header{}
				headers.Add("Authorization", "Basic "+value)
				client.Transport = &internal.HeaderTransport{Base: client.Transport, Headers: headers}
			} else if rawAuth != "" {
				resolvedAuth, wasSecret, err := internal.ResolveSecretReference(ctx, rawAuth)
				if err != nil {
					return fmt.Errorf("error resolving raw auth: %w", err)
				}
				if wasSecret {
					logger.Debug("resolved raw auth from 1Password")
				}
				headers := http.Header{}
				headers.Add("Authorization", resolvedAuth)
				client.Transport = &internal.HeaderTransport{Base: client.Transport, Headers: headers}
			}

			// Create SDK server and register tools from OpenAPI
			impl := &mcp.Implementation{Name: cmd.Name(), Version: version}
			server := mcp.NewServer(impl, nil)
			var opts []internal.RegisterToolsOption
			if noAnnotations {
				opts = append(opts, internal.WithoutAnnotations())
			}
			if err := internal.RegisterTools(server, specData, client, opts...); err != nil {
				return fmt.Errorf("error registering tools: %w", err)
			}

			// Run over stdio; when spec was from stdin, we redirected os.Stdin to /dev/tty above.
			return server.Run(ctx, &mcp.StdioTransport{})
		})

		return g.Wait()
	},
}

var (
	bearerAuth string
	basicAuth  string
	rawAuth    string

	retries int
	timeout time.Duration
	rps     int

	verbose       bool
	silent        bool
	noAnnotations bool

	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func init() {
	rootCmd.Flags().StringVar(&bearerAuth, "bearer-auth", "", "Bearer token value (will be prefixed with 'Bearer ')")
	rootCmd.Flags().StringVar(&basicAuth, "basic-auth", "", "Basic auth value (either user:pass or base64 encoded, will be prefixed with 'Basic ')")
	rootCmd.Flags().StringVar(&rawAuth, "raw-auth", "", "Raw value for Authorization header")
	rootCmd.MarkFlagsMutuallyExclusive("bearer-auth", "basic-auth", "raw-auth")

	rootCmd.Flags().IntVar(&retries, "retries", 3, "Maximum number of retries for failed requests")
	rootCmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "HTTP request timeout")
	rootCmd.Flags().IntVarP(&rps, "rps", "r", 0, "Maximum requests per second (0 for no limit)")

	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug level logging to stderr")
	rootCmd.Flags().BoolVarP(&silent, "silent", "s", false, "Disable all logging")
	rootCmd.MarkFlagsMutuallyExclusive("verbose", "silent")

	rootCmd.Flags().BoolVar(&noAnnotations, "no-annotations", false, "Disable generated tool annotations")

	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built at: %s)", version, commit, date)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
