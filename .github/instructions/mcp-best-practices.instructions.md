---
description: 'MCP protocol-level best practices for tool design, annotations, response formats, pagination, and security. Applies to all Go MCP server code.'
applyTo: "**/*.go"
---

# MCP Best Practices

Protocol-level guidelines for building high-quality MCP servers that enable LLMs to accomplish real-world tasks effectively.

## Tool Design Principles

### Naming

- **snake_case with service prefix**: `gitlab_create_issue`, `gitlab_list_projects`
- **Action-oriented verbs**: get, list, search, create, update, delete
- **Specific names**: Avoid generic names that conflict with other MCP servers
- **Descriptions must match behavior exactly**: Narrow, unambiguous descriptions

### Annotations

Every tool MUST declare annotations to help clients understand behavior:

```go
Annotations: &mcp.ToolAnnotations{
    ReadOnlyHint:    boolPtr(true),
    DestructiveHint: boolPtr(false),
    IdempotentHint:  boolPtr(true),
    OpenWorldHint:   boolPtr(true),
}
```

| Tool Type | ReadOnly | Destructive | Idempotent | OpenWorld |
|-----------|:---:|:---:|:---:|:---:|
| list / get / search | true | false | true | true |
| create | false | false | false | true |
| update | false | false | true | true |
| delete | false | true | true | true |

### Atomic Operations

Keep tools focused on a single operation. Avoid multi-step tools that combine unrelated actions.

## Response Formats

- **JSON**: Structured data for programmatic processing (include all fields)
- **Markdown**: Human-readable display (use headers, lists, formatting)
- Convert timestamps to human-readable format in Markdown
- Show display names with IDs in parentheses when useful
- Omit verbose metadata in Markdown output

## Pagination

For list operations:

- Default to 20-50 items per page
- Always respect the `per_page` / `limit` parameter
- Return metadata: `has_more`, `total_count`, `page`, `total_pages`
- Never load all results into memory for large datasets
- Use cursor-based pagination when offset-based becomes expensive

## Error Handling

- **Actionable messages**: Tell the LLM what went wrong AND what to try next
- **Report tool errors in results**: Use `IsError: true` in `CallToolResult` for recoverable errors
- **Don't expose internals**: Hide stack traces, internal paths, database details
- **Wrap with context**: `fmt.Errorf("gitlab_list_projects: %w", err)`
- **Clean up resources**: Always release connections, files on errors

## Security

### Authentication

- Store tokens in environment variables, never in code
- Validate credentials on startup with clear error messages
- Use `context.Context` to pass auth through call chains

### Input Validation

- Sanitize file paths against directory traversal
- Validate URLs and external identifiers
- Check parameter sizes and ranges
- Use JSON schema struct tags for constraints

### HTTP Transport Security

- Bind to `127.0.0.1` for local servers, not `0.0.0.0`
- Validate `Origin` header on incoming connections
- Enable DNS rebinding protection
- Use TLS for remote deployments

## Transport Selection

| Criterion | stdio | Streamable HTTP |
|-----------|-------|-----------------|
| Deployment | Local | Remote |
| Clients | Single | Multiple |
| Complexity | Low | Medium |
| Real-time | No | Yes |

- **stdio**: Log to stderr, never stdout
- **Streamable HTTP**: Preferred over deprecated SSE
- Support graceful shutdown via signal handling

## Testing Requirements

- **Functional**: Verify correct execution with valid and invalid inputs
- **Integration**: Test against real external systems (with proper .env config)
- **Security**: Validate auth, input sanitization, error message safety
- **Evaluation**: Create Q&A pairs to test LLM's ability to use the server effectively

## MCP Protocol 2025-11-25 Features

### Structured Output (OutputSchema)

Use typed output structs to auto-generate `OutputSchema` and `StructuredContent`. This enables clients to process successful tool results programmatically. If a tool result sets `isError: true`, omit `structuredContent` unless it still conforms to the declared success schema.

```go
// Triple-return signature auto-generates OutputSchema from Out type
mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
    return markdownResult, typedOutput, nil
})
```

### Tool Icons

Provide domain-specific SVG icons on all tools, resources, and prompts for visual identification in client UIs.

### Sampling

Tools can request LLM assistance via `server.CreateMessage()` for analysis, summarization, and content generation tasks. Strip credentials from sampling prompts.

### Elicitation

Tools can request user input via `server.Elicit()` for confirmation of destructive actions or collecting missing parameters.

### Completions

Provide argument autocompletion for tool parameters using `mcp.AddCompletionProvider()`. Register completions for arguments that have a finite, discoverable set of values (project IDs, branch names, user names).

### Discovery and Meta-Tools

For servers with many tools, provide meta-tools that group operations by domain:

- Reduces tool count for LLM context window efficiency
- Uses `action` enum field routed to specific handlers
- Each meta-tool has a discovery description listing available actions
