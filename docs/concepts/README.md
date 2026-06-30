# Concepts

**Explanation** — the ideas and design decisions behind gitlab-mcp-server.

These pages answer *why* and *how it works* rather than *how to do X*. Read them
when you want to understand the system's shape: its architecture, the three tool
surfaces and how they're projected, when it reaches for GraphQL, how errors are
made actionable, what security model it assumes, and how it scales. None of these
are step-by-step instructions — for those, see [Guides](../guides/README.md).

> **Diátaxis type**: Explanation · **Audience**: 🔧 Operators & 🛠️ contributors

| Topic                                           | What it explains                                                         |
| ----------------------------------------------- | ------------------------------------------------------------------------ |
| [Architecture](architecture.md)                 | System architecture with C4 diagrams, components, and data flow          |
| [Meta-Tools](meta-tools.md)                     | The domain-level meta-tool surface and its action mappings               |
| [Dynamic Toolset](dynamic-tools.md)             | The low-token find/execute surface and the canonical action catalog      |
| [GraphQL Integration](graphql.md)               | When and why the server uses GitLab's GraphQL API instead of REST        |
| [Security](security.md)                         | Authentication, TLS, input validation, and transport security model      |
| [Resource Consumption](resource-consumption.md) | Memory footprint, scaling limits, and optimization strategies            |
| [Error Handling](error-handling.md)             | Error types, classification, and how diagnostics are made LLM-actionable |

**Looking for something else?**
[Reference](../reference/README.md) for exact tool/flag details ·
[Development](../development/README.md) to change the system yourself.
