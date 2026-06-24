# MCP Capabilities

Detailed documentation for the **3 MCP capabilities** implemented by gitlab-mcp-server.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, contributors, integrators

## What Are Capabilities?

Capabilities are protocol-level features negotiated during the MCP `initialize` handshake. They determine what the server and client can do beyond basic tool calls — progress updates, autocomplete, and interactive user input.

## Server Capabilities

Declared by the server and consumed by connected MCP clients.

| # | Capability | Package | Purpose |
| --: | ---------- | ------- | ------- |
| 2 | [Progress](progress.md) | `internal/progress/` | Step-by-step progress notifications |
| 3 | [Completions](completions.md) | `internal/completions/` | Autocomplete for prompt arguments and resource URIs |

## Client Capabilities

Provided by the MCP client and consumed by the server at tool execution time.

| # | Capability | Package | Purpose |
| --: | ---------- | ------- | ------- |
| 6 | [Elicitation](elicitation.md) | `internal/elicitation/` | Interactive user input forms (4 tools) |

## Capability Declaration

Capabilities are declared in `cmd/server/main.go` when constructing the MCP server:

```go
server := mcp.NewServer(
    &mcp.ServerCapabilities{
        Tools:       &mcp.ToolCapabilities{ListChanged: true},
        Resources:   &mcp.ResourceCapabilities{ListChanged: true},
    },
    &mcp.ServerOptions{
        CompletionHandler:           completionHandler.Complete,
        ProgressNotificationHandler: progressHandler,
    },
)
```

`CAPABILITY_SURFACE=full` also advertises `Prompts` with `ListChanged: true`
and registers the full prompt/resource catalog. `CAPABILITY_SURFACE=minimal`
omits the prompt capability while leaving tool execution, completions, and
progress handling available. Minimal also registers `gitlab://tools` and `gitlab://tools/{id}`
for exact action call shapes across every tool surface.

Client capabilities (Elicitation) are not declared by the server — they are advertised by the client during the `initialize` handshake. The server checks for their presence at tool execution time via `FromRequest()` helpers.

## Features

Additional cross-cutting features implemented alongside capabilities.

| # | Feature | Package | Purpose |
| --: | ------- | ------- | ------- |
| 1 | [Icons](icons.md) | `internal/toolutil/` | 50 SVG icons (base64 data URIs) for tools, resources, and prompts |

## Design Principles

All capability implementations in this project follow consistent patterns:

- **Zero-value safety** — `progress.Tracker` and `elicitation.Client` are value types whose zero values are safe no-ops. Tool handlers never need nil-checks.
- **Graceful degradation** — If a client doesn't support a capability, tools return informational messages instead of errors. The server never crashes due to missing capabilities.
- **Security boundaries** — Elicitation validates all responses against schemas.
- **Nil-safe receivers** — `SessionLogger` methods are safe to call on nil receivers.

## External References

- [MCP Specification — Capabilities](https://modelcontextprotocol.io/specification/2025-11-25/server)
- [MCP Go SDK — ServerCapabilities](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#ServerCapabilities)
- [MCP Specification](https://modelcontextprotocol.io/specification/) — official protocol documentation
