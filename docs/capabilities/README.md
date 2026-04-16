# MCP Capabilities

Detailed documentation for the **6 MCP capabilities** implemented by gitlab-mcp-server.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, contributors, integrators

## What Are Capabilities?

Capabilities are protocol-level features negotiated during the MCP `initialize` handshake. They determine what the server and client can do beyond basic tool calls — structured logging, progress updates, autocomplete, workspace discovery, LLM delegation, and interactive user input.

## Server Capabilities

Declared by the server and consumed by connected MCP clients.

| # | Capability | Package | Purpose |
| --: | ---------- | ------- | ------- |
| 1 | [Logging](logging.md) | `internal/logging/` | Structured log messages to the client |
| 2 | [Progress](progress.md) | `internal/progress/` | Step-by-step progress notifications |
| 3 | [Completions](completions.md) | `internal/completions/` | Autocomplete for prompt arguments and resource URIs |

## Client Capabilities

Provided by the MCP client and consumed by the server at tool execution time.

| # | Capability | Package | Purpose |
| --: | ---------- | ------- | ------- |
| 4 | [Roots](roots.md) | `internal/roots/` | Workspace directory discovery |
| 5 | [Sampling](sampling.md) | `internal/sampling/` | LLM analysis delegation (11 tools) |
| 6 | [Elicitation](elicitation.md) | `internal/elicitation/` | Interactive user input forms (4 tools) |

## Capability Declaration

Capabilities are declared in `cmd/server/main.go` when constructing the MCP server:

```go
server := mcp.NewServer(
    &mcp.ServerCapabilities{
        Logging:     &mcp.LoggingCapabilities{},
    },
    &mcp.ServerOptions{
        CompletionHandler:           completionHandler.Complete,
        RootsListChangedHandler:     rootsManager.Refresh,
        ProgressNotificationHandler: progressHandler,
    },
)
```

Client capabilities (Roots, Sampling, Elicitation) are not declared by the server — they are advertised by the client during the `initialize` handshake. The server checks for their presence at tool execution time via `FromRequest()` helpers.

## Features

Additional cross-cutting features implemented alongside capabilities.

| # | Feature | Package | Purpose |
| --: | ------- | ------- | ------- |
| 1 | [Icons](icons.md) | `internal/toolutil/` | 44 SVG icons for tools, resources, and prompts |

## Design Principles

All capability implementations in this project follow consistent patterns:

- **Zero-value safety** — `progress.Tracker`, `sampling.Client`, and `elicitation.Client` are value types whose zero values are safe no-ops. Tool handlers never need nil-checks.
- **Graceful degradation** — If a client doesn't support a capability, tools return informational messages instead of errors. The server never crashes due to missing capabilities.
- **Security boundaries** — Logging never includes secrets. Sampling uses a hardened, non-configurable system prompt. Elicitation validates all responses against schemas.
- **Nil-safe receivers** — `SessionLogger` methods are safe to call on nil receivers.

## External References

- [MCP Specification — Capabilities](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/logging)
- [MCP Go SDK — ServerCapabilities](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#ServerCapabilities)
- [MCP Specification](https://modelcontextprotocol.io/specification/) — official protocol documentation
