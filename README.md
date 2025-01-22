# Getting started

## Download the `emcee` CLI
```sh
curl <url>
```


## Update your `claude_desktop_config.json`

Start the Claude desktop app then `Claude > Settings... > Developer > Edit Config`. The file might be here: `~/Library/Application Support/Claude/claude_desktop_config.json` but you should check the path in your app settings.  


Edit your `claude_desktop_config.json` to add a new `mcpServers` entry. In this example, we're adding a new `weather` server that uses the `emcee` CLI to fetch weather data from the [National Weather Service](https://api.weather.gov/openapi.json).

```json
{
    "mcpServers": {
        "weather": {
            "command": "~/emcee",
            "args": [
                "https://api.weather.gov/openapi.json",
            ]
        }
    }
}
```

### API with authentication
To use an API that requires authentication, use the `--auth` flag. 

```json
    "mcpServers": {
        "x (formerly twitter)": {
            "command": "~/emcee",
            "args": [
                "https://api.x.com/2/openapi.json",
                "--auth",
                "Bearer <YOUR_X_BEARER_TOKEN>"
            ]
        }
    }
```


### Getting an OpenAPI spec from an API

```sh
curl -s \
  -H "Authorization: Bearer <YOUR_REPLICATE_API_KEY>" \
  https://api.replicate.com/v1/models/black-forest-labs/flux-schnell \
  | jq .latest_version.openapi_schema
```



# emcee

emcee is a CLI tool that provides an [MCP] stdio transport for a given [OpenAPI] specification.

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
```

```console
emcee https://api.weather.gov/openapi.json
```

<details open>

<summary>Request</summary>

```json
{"jsonrpc": "2.0", "method": "tools/list", "params": {}, "id": 1}
```

</details>

</details>

<details open>

<summary>Response</summary>

```jsonc
{ 
  "jsonrpc":"2.0", 
  "result": {
    "tools": [
        {"name":"alerts_query", "description":"Returns all alerts", "inputSchema": {"type":"object"}}
        // ...
    ]
  }
}
```

</details>

## Requirements

- Go 1.23+

## Installation

Run the following command to build from source:

```console
go build -o emcee cmd/emcee/main.go
```

Once built, you can run in place (`./emcee`) or 
move it somewhere in your `PATH`, like `/usr/local/bin`.

[MCP]: https://modelcontextprotocol.io/
[OpenAPI]: https://www.openapis.org
