# emcee

emcee is a CLI tool that provides an [MCP] stdio transport for a given [OpenAPI] specification.

```console
$ emcee https://api.weather.gov/openapi.json
```

<details>

<summary>Request</summary>

```json
{"jsonrpc": "2.0", "method": "tools/list", "params": {}, "id": 1}
```

<details>

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
$ go build -o emcee cmd/emcee/main.go
```

[MCP]: https://modelcontextprotocol.io/
[OpenAPI]: https://www.openapis.org