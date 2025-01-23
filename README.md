**emcee** is a tool that provides a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for any web application with an [OpenAPI] specification. You can use emcee to connect [Claude Desktop](https://claude.ai/download) and [other apps ](https://modelcontextprotocol.info/docs/clients/) to external tools and data services, similar to [ChatGPT plugins](https://openai.com/index/chatgpt-plugins/).

## Quickstart

If you're on macOS 15 and have Claude, `brew` and `jq` installed, 
you can get up-and-running with a few commands:

```console
# Install emcee
brew install emcee

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

> 🌤️ Partly sunny in Portland, with a light NNW breeze at 2-6 mph. 
> Currently 7°C, falling to 6°C this afternoon. 
> Watch out for that frost before 7am!

!!! TK Screenshot of it working !!!

---

## Installation

### Installer Script

```console
sh <(curl -fsSL http://emcee.sh)
```

### Homebrew

```console
brew install loopwork-ai/tap/emcee
```

### Build From Source

Requires [go 1.23](https://go.dev) or later.

```console
git clone https://github.com/loopwork-ai/emcee.git
cd emcee
go build -o emcee cmd/emcee/main.go
```

Once built, you can run in place (`./emcee`) or move it somewhere in your `PATH`, like `/usr/local/bin`.

## Setup

Claude > Settings... (<kbd>⌘</kbd><kbd>,</kbd>)
Navigate to "Developer" section in sidebar.
Click "Edit Config" button to reveal config file in Finder.

At the time of writing, this file is located in a subdirectory of Application Support.
You can edit it in VSCode with the following command:  

```
code ~/Library/Application\ Support/Claude/claude_desktop_config.json
```

Enter the following:

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

MCP uses [JSON-RPC 2.0](https://www.jsonrpc.org/) as its wire format.
emcee supports [Standard Input/Output (stdio)](https://modelcontextprotocol.io/docs/concepts/transports#standard-input-output-stdio) transport.

When you run the `emcee` tool from the command-line, 
it starts a program that listens on stdin, 
outputs to stdout, 
and logs to stderr.

You can interact directly with the provided MCP server 
by sending JSON-RPC requests.

**List Tools**

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

**Call Tool**

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
