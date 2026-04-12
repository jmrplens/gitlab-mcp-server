# MCP Capabilities

> 👤🔧 **Audience**: All users

<!-- -->

> **Moved**: Capability documentation has been expanded into individual documents in the [`capabilities/`](capabilities/) folder for detailed coverage of each capability.

## Quick Reference

| # | Capability | Direction | Package | Document |
| --: | ---------- | --------- | ------- | -------- |
| 1 | Logging | Server → Client | `internal/logging/` | [logging.md](capabilities/logging.md) |
| 2 | Progress | Server → Client | `internal/progress/` | [progress.md](capabilities/progress.md) |
| 3 | Completions | Client → Server | `internal/completions/` | [completions.md](capabilities/completions.md) |
| 4 | Roots | Client → Server | `internal/roots/` | [roots.md](capabilities/roots.md) |
| 5 | Sampling | Server → Client | `internal/sampling/` | [sampling.md](capabilities/sampling.md) |
| 6 | Elicitation | Server → Client | `internal/elicitation/` | [elicitation.md](capabilities/elicitation.md) |

## Detailed Documentation

- **[capabilities/README.md](capabilities/README.md)** — overview of all 6 capabilities with design principles
- **[capabilities/logging.md](capabilities/logging.md)** — structured log messages, SessionLogger API, security rules
- **[capabilities/progress.md](capabilities/progress.md)** — step-by-step progress tracker, tools that use it
- **[capabilities/completions.md](capabilities/completions.md)** — 17 argument types, per-project and global completers
- **[capabilities/roots.md](capabilities/roots.md)** — workspace discovery, Git detection, project discovery via `gitlab://workspace/roots` resource
- **[capabilities/sampling.md](capabilities/sampling.md)** — LLM analysis delegation, 11 tools, credential stripping, hardened prompt
- **[capabilities/elicitation.md](capabilities/elicitation.md)** — interactive creation wizards, 4 tools, JSON Schema validation

## Capability Declaration

Capabilities are declared in `cmd/server/main.go` when constructing the MCP server:

```go
mcp.ServerCapabilities{
    Logging:     &mcp.LoggingCapability{},
    Completions: &mcp.CompletionCapability{},
}
```

Client capabilities (Roots, Sampling, Elicitation) are negotiated during the MCP `initialize` handshake. The server checks for their presence before using them.

## External References

- [MCP Specification — Capabilities](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/logging)
- [MCP Go SDK — ServerCapabilities](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#ServerCapabilities)
- [MCP Protocol Guide](mcp-protocol/) — comprehensive MCP learning resource
