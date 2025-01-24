![emcee flow diagram](https://github.com/user-attachments/assets/bcac98a5-497f-4b34-9e8d-d4bc08852ea1)

# emcee

**emcee** is a tool that provides a [Model Context Protocol (MCP)][mcp] server
for any web application with an [OpenAPI][openapi] specification.
You can use emcee to connect [Claude Desktop][claude] and [other apps][mcp-clients]
to external tools and data services,
similar to [ChatGPT plugins][chatgpt-plugins].

## Quickstart

If you're on macOS 15 and have [Homebrew][homebrew] installed,
you can get up-and-running with a few commands:

```bash
# Install emcee
brew install loopwork-ai/tap/emcee

# Install claude and jq
brew install --cask claude
brew install jq # used to update Claude config in next step

# Add MCP server for weather.gov to Claude Desktop
CLAUDE_CONFIG="~/Library/Application Support/Claude/claude_desktop_config.json" && \
mkdir -p "$(dirname "$CLAUDE_CONFIG")" && \
touch "$CLAUDE_CONFIG" && \
cp "$CLAUDE_CONFIG" <(jq '. * {"mcpServers":{"weather.gov":{"command":"emcee","args":["https://api.weather.gov/openapi.json"]}}}' "$CLAUDE_CONFIG")

# Quit and Re-open Claude app
osascript -e 'tell application "Claude" to quit' && sleep 1 && open -a Claude
```

Start a new chat and ask it about the weather where you are.

> What's the weather in Portland, OR?

Claude will consult the tools made available to it through MCP
and request to use one if deemed to be suitable for answering your question.
You can review this request and either approve or deny it.

<img src="https://github.com/user-attachments/assets/394ac476-17c2-4a29-aaff-9537d42b289b" alt="Allow tool from weather MCP dialog" width="460">

If you allow, Claude will communicate with the MCP
and use the result to inform its response.

![Claude response with MCP tool use](https://github.com/user-attachments/assets/d5b63002-1ed3-4b03-bc71-8357427bb06b)

## Why use emcee?

MCP provides a standardized way to connect AI models to tools and data sources.
It's still early days, but there are already a variety of [available servers][mcp-servers]
for connecting to browsers, developer tools, and other systems.

We think emcee is a convenient way to connect to services
that don't have an existing MCP server implementation —
_especially for services you're building yourself_.
Got a web app with an OpenAPI spec?
You might be surprised how far you can get
without a dashboard or client library.

---

> [!TIP]
> Building agents?
> Want to deploy remote MCPs?
> Reach out to us at emcee@loopwork.com

---

## Installation

### Installer Script

Use the [installer script][installer] to download and install
a [pre-built release][releases] of emcee for your platform (Linux x86-64/i386/arm64 and macOS Intel/Apple Silicon).

```console
# fish
sh (curl -fsSL https://raw.githubusercontent.com/loopwork-ai/emcee/refs/heads/main/tools/install.sh | psub)

# bash, zsh
sh <(curl -fsSL https://raw.githubusercontent.com/loopwork-ai/emcee/refs/heads/main/tools/install.sh)
```

### Homebrew

Install emcee using [Homebrew][homebrew] from [Loopwork's tap][homebrew-tap].

```console
brew install loopwork-ai/tap/emcee
```

### Docker

Prebuilt [Docker images][docker-images] with emcee are available.

```console
docker run -it ghcr.io/loopwork-ai/emcee
```

### Build From Source

Requires [go 1.23][golang] or later.

```console
git clone https://github.com/loopwork-ai/emcee.git
cd emcee
go build -o emcee cmd/emcee/main.go
```

Once built, you can run in place (`./emcee`) or move it somewhere in your `PATH`, like `/usr/local/bin`.

## Setup

To configure Claude Desktop for use with emcee:

1. Open Claude Desktop Settings (<kbd>⌘</kbd><kbd>,</kbd>)
2. Select the "Developer" section in the sidebar
3. Click "Edit Config" to open the configuration file

![Claude Desktop settings Edit Config button](https://github.com/user-attachments/assets/761c6de5-62c2-4c53-83e6-54362040acd5)

The configuration file should be located in the Application Support directory.
You can open it directly in VSCode using:

```console
code ~/Library/Application\ Support/Claude/claude_desktop_config.json
```

Add the following configuration to add the weather.gov MCP server:

```json
{
  "mcpServers": {
    "weather": {
      "command": "emcee",
      "args": [
        "https://api.weather.gov/openapi.json"
      ]
    }
  }
}
```

After saving the file, quit and re-open Claude.
You should now see <kbd>🔨57</kbd> in the bottom right corner of your chat box.
Click on that to see a list of all the tools made available to Claude through MCP.

![Claude Desktop chat box with MCP tool count](https://github.com/user-attachments/assets/fc204032-2c52-4e74-85dc-c9d7687ff25f)

## Usage

```console
Usage:
  emcee [spec-path-or-url] [flags]

Flags:
      --auth string        Authorization header value (e.g. 'Bearer token123' or 'Basic dXNlcjpwYXNz')
  -h, --help               help for emcee
      --retries int        Maximum number of retries for failed requests (default 3)
  -r, --rps int            Maximum requests per second (0 for no limit)
      --timeout duration   HTTP request timeout (default 1m0s)
  -v, --verbose            Enable verbose logging to stderr
      --version            version for emcee
```

emcee implements [Standard Input/Output (stdio)](https://modelcontextprotocol.io/docs/concepts/transports#standard-input-output-stdio) transport for MCP,
which uses [JSON-RPC 2.0](https://www.jsonrpc.org/) as its wire format.

When you run emcee from the command-line,
it starts a program that listens on stdin,
outputs to stdout,
and logs to stderr.

You can interact directly with the provided MCP server
by sending JSON-RPC requests.

> [!NOTE]
> emcee provides only MCP tool capabilities.
> Other features like resources, prompts, and sampling aren't yet supported.

### Example JSON-RPC Calls

#### List Tools

<details open>

<summary>Request</summary>

```json
{"jsonrpc": "2.0", "method": "tools/list", "params": {}, "id": 1}
```

</details>

<details open>

<summary>Response</summary>

```jsonc
{
  "jsonrpc":"2.0",
  "result": {
    "tools": [
      // ...
      {
          "name": "tafs",
          "description": "Returns Terminal Aerodrome Forecasts for the specified airport station.",
          "inputSchema": {
              "type": "object",
              "properties": {
                  "stationId": {
                      "description": "Observation station ID",
                      "type": "string"
                  }
              },
              "required": ["stationId"]
          }
      },
      // ...
    ]
  },
  "id": 1
}
```
</details>

#### Call Tool

<details open>

<summary>Request</summary>

```json
{"jsonrpc": "2.0", "method": "tools/call", "params": { "name": "gridpoint_forecast", "arguments": { "stationId": "KPDX" }}, "id": 1}
```

</details>

<details open>

<summary>Response</summary>

```jsonc
{
  "jsonrpc":"2.0",
  "content": [
    {
      "type": "text",
      "text": /* Weather forecast in GeoJSON format */,
      "annotations": {
        "audience": ["assistant"]
      }
    }
  ]
  "id": 1
}
```

</details>

## Debugging

The [MCP Inspector][mcp-inspector] is a tool for testing and debugging MCP servers.
If Claude and/or emcee aren't working as expected,
the inspector can help you understand what's happening.

```console
npx @modelcontextprotocol/inspector emcee https://api.weather.gov/openapi.json
# 🔍 MCP Inspector is up and running at http://localhost:5173 🚀
```

```console
open http://localhost:5173
```

## License

emcee is licensed under the Apache License, Version 2.0.

[chatgpt-plugins]: https://openai.com/index/chatgpt-plugins/
[claude]: https://claude.ai/download
[docker-images]: https://github.com/loopwork-ai/emcee/pkgs/container/emcee
[golang]: https://go.dev
[homebrew]: https://brew.sh
[homebrew-tap]: https://github.com/loopwork-ai/homebrew-tap
[installer]: https://github.com/loopwork-ai/emcee/blob/main/tools/install.sh
[mcp]: https://modelcontextprotocol.io/
[mcp-clients]: https://modelcontextprotocol.info/docs/clients/
[mcp-inspector]: https://github.com/modelcontextprotocol/inspector
[mcp-servers]: https://modelcontextprotocol.io/examples
[openapi]: https://openapi.org
[releases]: https://github.com/loopwork-ai/emcee/releases
