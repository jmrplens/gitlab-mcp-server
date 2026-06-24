# MCP Capabilities

> 👤🔧 **Audience**: All users

<!-- -->

> **Moved**: Capability documentation has been expanded into individual documents in the [`capabilities/`](capabilities/) folder for detailed coverage of each capability.

## Quick Reference

| # | Capability | Direction | Package | Document |
| --: | ---------- | --------- | ------- | -------- |
| 2 | Progress | Server → Client | `internal/progress/` | [progress.md](capabilities/progress.md) |
| 3 | Completions | Client → Server | `internal/completions/` | [completions.md](capabilities/completions.md) |
| 6 | Elicitation | Server → Client | `internal/elicitation/` | [elicitation.md](capabilities/elicitation.md) |

## Detailed Documentation

- **[capabilities/README.md](capabilities/README.md)** — overview of all 3 capabilities with design principles
- **[capabilities/progress.md](capabilities/progress.md)** — step-by-step progress tracker, tools that use it
- **[capabilities/completions.md](capabilities/completions.md)** — 17 argument types, per-project and global completers
- **[capabilities/elicitation.md](capabilities/elicitation.md)** — interactive creation wizards, 4 tools, JSON Schema validation

## Capability Declaration

Capabilities are declared in `cmd/server/main.go` when constructing the MCP server:

```go
serverCapabilities := &mcp.ServerCapabilities{
    Tools:     &mcp.ToolCapabilities{ListChanged: true},
    Resources: &mcp.ResourceCapabilities{ListChanged: true},
}
if capabilitySurface == config.CapabilitySurfaceFull {
    serverCapabilities.Prompts = &mcp.PromptCapabilities{ListChanged: true}
}
```

The `tools` and `resources` `ListChanged: true` flags are always advertised.
The `prompts` capability is advertised only when `CAPABILITY_SURFACE=full`.
`CAPABILITY_SURFACE=minimal` keeps tool execution, completions, and
progress handling, but
omits optional prompts, static GitLab resources, and workflow guides. It also
keeps `gitlab://tools` and `gitlab://tools/{id}` so callers can retrieve exact
action call shapes without expanding `tools/list`.

The configuration intentionally has only two modes today. `full` is the broad
compatibility surface; `minimal` is the low-token surface for Dynamic clients
that can use `gitlab_find_action` for schemas inline or `gitlab://tools` for
explicit action enumeration.
The latest token audit measured shared resources plus prompts at about 18.2k
tokens in full mode and about 184 tokens in minimal mode. Intermediate modes
such as schemas-only or resources-only are not exposed because they add another
configuration branch without improving the recommended Dynamic low-token path.

The Go SDK debounces list-changed notifications (10 ms window) and sends
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

Client capabilities (Elicitation) are negotiated during the MCP `initialize` handshake. The server checks for their presence before using them.

## External References

- [MCP Specification — Capabilities](https://modelcontextprotocol.io/specification/2025-11-25/server)
- [MCP Go SDK — ServerCapabilities](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#ServerCapabilities)
- [MCP Specification](https://modelcontextprotocol.io/specification/) — official protocol documentation
