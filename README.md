# emcee

emcee is a CLI tool that provides an [MCP] stdio transport for a given [OpenAPI] specification.

```console
Usage:
  emcee [openapi-spec-url] [flags]

Flags:
      --auth string   Authorization header value (e.g. 'Bearer token123' or 'Basic dXNlcjpwYXNz')
  -h, --help          help for emcee
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
