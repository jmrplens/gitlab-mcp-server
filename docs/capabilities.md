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
    Logging:   &mcp.LoggingCapabilities{},
    Tools:     &mcp.ToolCapabilities{ListChanged: true},
    Resources: &mcp.ResourceCapabilities{ListChanged: true},
    Prompts:   &mcp.PromptCapabilities{ListChanged: true},
}
```

The three `ListChanged: true` flags advertise that the server will emit
`notifications/tools/list_changed`, `notifications/resources/list_changed`, and
`notifications/prompts/list_changed` whenever the corresponding catalog
changes. The Go SDK debounces these notifications (10 ms window) and sends
them automatically when `AddTool`, `AddResource`, `AddPrompt`, or their
`Remove*` counterparts are invoked at runtime — no manual emission is
required from handler code.

In practice this server emits list-changed notifications only on dynamic
tool exclusion via `removeExcludedTools` at startup; the catalog is
otherwise immutable for the lifetime of a session. Auto-update replaces
the binary process entirely, so the MCP session is reinitialised rather
than mutated. Declaring the capability is still valuable for spec
compliance and lets clients keep their UI in sync without polling
`tools/list`, `resources/list`, or `prompts/list`.

Client capabilities (Roots, Sampling, Elicitation) are negotiated during the MCP `initialize` handshake. The server checks for their presence before using them.

## External References

- [MCP Specification — Capabilities](https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/logging)
- [MCP Go SDK — ServerCapabilities](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#ServerCapabilities)
- [MCP Specification](https://modelcontextprotocol.io/specification/) — official protocol documentation
