---
description: 'Best practices and patterns for building Model Context Protocol (MCP) servers in Go using the official github.com/modelcontextprotocol/go-sdk package (v1.5.0+).'
applyTo: "**/*.go, **/go.mod, **/go.sum"
---

# Go MCP Server Development Guidelines

When building MCP servers in Go, follow these best practices and patterns using the official Go SDK (v1.5.0+).

## Server Setup

Create an MCP server using `mcp.NewServer`:

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

server := mcp.NewServer(
    &mcp.Implementation{
        Name:    "my-server",
        Version: "v1.5.0",
    },
    nil, // or provide mcp.Options
)
```

## Tool Naming Conventions

Use snake_case with a service prefix to avoid conflicts when multiple MCP servers run together:

- **Format**: `{service}_{action}_{resource}` (e.g., `gitlab_create_issue`, `gitlab_list_projects`)
- **Be action-oriented**: Start with verbs (`get`, `list`, `search`, `create`, `update`, `delete`)
- **Be specific**: Avoid generic names that could conflict with other servers
- **Descriptions**: Must narrowly and unambiguously describe functionality, matching actual behavior

## Adding Tools

Use `mcp.AddTool` with struct-based input and output for type safety:

```go
type ToolInput struct {
    Query string `json:"query" jsonschema:"the search query"`
    Limit int    `json:"limit,omitempty" jsonschema:"maximum results to return"`
}

type ToolOutput struct {
    Results []string `json:"results" jsonschema:"list of search results"`
    Count   int      `json:"count" jsonschema:"number of results found"`
}

func SearchTool(ctx context.Context, req *mcp.CallToolRequest, input ToolInput) (
    *mcp.CallToolResult,
    ToolOutput,
    error,
) {
    // Implement tool logic
    results := performSearch(ctx, input.Query, input.Limit)

    return nil, ToolOutput{
        Results: results,
        Count:   len(results),
    }, nil
}

// Register the tool with annotations
mcp.AddTool(server,
    &mcp.Tool{
        Name:        "gitlab_search",
        Description: "Search for information across GitLab resources",
        Annotations: &mcp.ToolAnnotations{
            ReadOnlyHint:    boolPtr(true),
            DestructiveHint: boolPtr(false),
            IdempotentHint:  boolPtr(true),
            OpenWorldHint:   boolPtr(true),
        },
    },
    SearchTool,
)

func boolPtr(b bool) *bool { return &b }
```

## Tool Annotations

Provide annotations to help clients understand tool behavior:

| Annotation | Type | Default | Description |
|-----------|------|---------|-------------|
| `ReadOnlyHint` | *bool | false | Tool does not modify its environment |
| `DestructiveHint` | *bool | true | Tool may perform destructive updates |
| `IdempotentHint` | *bool | false | Repeated calls with same args have no additional effect |
| `OpenWorldHint` | *bool | true | Tool interacts with external entities |

**Important**: Annotations are hints, not security guarantees. Clients should not make security-critical decisions based solely on annotations.

Guidelines for setting annotations:

- **Read-only tools** (list, get, search): `ReadOnlyHint=true, DestructiveHint=false, IdempotentHint=true`
- **Create tools**: `ReadOnlyHint=false, DestructiveHint=false, IdempotentHint=false`
- **Update tools**: `ReadOnlyHint=false, DestructiveHint=false, IdempotentHint=true`
- **Delete tools**: `ReadOnlyHint=false, DestructiveHint=true, IdempotentHint=true`

```text

## Adding Resources

Use `mcp.AddResource` for providing accessible data:

```go
func GetResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
    content, err := loadResourceContent(ctx, req.URI)
    if err != nil {
        return nil, err
    }

    return &mcp.ReadResourceResult{
        Contents: []any{
            &mcp.TextResourceContents{
                ResourceContents: mcp.ResourceContents{
                    URI:      req.URI,
                    MIMEType: "text/plain",
                },
                Text: content,
            },
        },
    }, nil
}

mcp.AddResource(server,
    &mcp.Resource{
        URI:         "file:///data/example.txt",
        Name:        "Example Data",
        Description: "Example resource data",
        MIMEType:    "text/plain",
    },
    GetResource,
)
```

## Adding Prompts

Use `mcp.AddPrompt` for reusable prompt templates:

```go
type PromptInput struct {
    Topic string `json:"topic" jsonschema:"the topic to analyze"`
}

func AnalyzePrompt(ctx context.Context, req *mcp.GetPromptRequest, input PromptInput) (
    *mcp.GetPromptResult,
    error,
) {
    return &mcp.GetPromptResult{
        Description: "Analyze the given topic",
        Messages: []mcp.PromptMessage{
            {
                Role: mcp.RoleUser,
                Content: mcp.TextContent{
                    Text: fmt.Sprintf("Analyze this topic: %s", input.Topic),
                },
            },
        },
    }, nil
}

mcp.AddPrompt(server,
    &mcp.Prompt{
        Name:        "analyze",
        Description: "Analyze a topic",
        Arguments: []mcp.PromptArgument{
            {
                Name:        "topic",
                Description: "The topic to analyze",
                Required:    true,
            },
        },
    },
    AnalyzePrompt,
)
```

## Transport Configuration

### Stdio Transport

For communication over stdin/stdout (local integrations, desktop apps, single-user):

```go
if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
    log.Fatal(err)
}
```

**Important**: stdio servers must NOT log to stdout — use stderr or structured logging to a file.

### HTTP Transport

For remote servers, multi-client scenarios, web service deployments:

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

transport := &mcp.HTTPTransport{
    Addr: ":8080",
}

if err := server.Run(ctx, transport); err != nil {
    log.Fatal(err)
}
```

### Transport Selection Guide

| Criterion | stdio | HTTP |
|-----------|-------|------|
| **Deployment** | Local | Remote |
| **Clients** | Single | Multiple |
| **Complexity** | Low | Medium |
| **Real-time** | No | Yes |

Prefer streamable HTTP for remote/multi-client. Avoid SSE (deprecated in favor of streamable HTTP).

## Error Handling

Return actionable errors that guide LLMs toward solutions:

```go
func MyTool(ctx context.Context, req *mcp.CallToolRequest, input MyInput) (
    *mcp.CallToolResult,
    MyOutput,
    error,
) {
    if ctx.Err() != nil {
        return nil, MyOutput{}, ctx.Err()
    }

    if input.Query == "" {
        return nil, MyOutput{}, fmt.Errorf("query cannot be empty, provide a search term")
    }

    result, err := performOperation(ctx, input)
    if err != nil {
        return nil, MyOutput{}, fmt.Errorf("operation failed for query %q: %w", input.Query, err)
    }

    return nil, result, nil
}
```

Error handling guidelines:

- **Actionable messages**: Include what went wrong AND what to try next
- **Don't expose internals**: Hide stack traces, internal paths, credentials
- **Wrap with context**: Use `fmt.Errorf("context: %w", err)` for error chains
- **Report tool errors in results**: Use `IsError: true` in `CallToolResult` for recoverable errors, not protocol-level errors
- **Clean up resources**: Always release connections, files, etc. on errors

## Response Formats

Support both structured and human-readable output:

```go
// JSON for programmatic processing
type ProjectOutput struct {
    ID          int    `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
    WebURL      string `json:"web_url"`
}

// Markdown for human readability via CallToolResult
func formatMarkdown(project *gitlab.Project) *mcp.CallToolResult {
    md := fmt.Sprintf("## %s\n\n%s\n\n**URL**: %s",
        project.Name, project.Description, project.WebURL)
    return &mcp.CallToolResult{
        Content: []any{mcp.TextContent{Text: md}},
    }
}
```

Guidelines:

- JSON for structured data that clients can process programmatically
- Markdown for human-readable display with headers, lists, formatting
- Include all relevant fields in JSON; omit verbose metadata in Markdown
- Convert timestamps to human-readable format in Markdown output

## Pagination

For tools that list resources, implement proper pagination:

```go
type ListInput struct {
    Page    int `json:"page,omitempty" jsonschema:"page number (1-based), default 1"`
    PerPage int `json:"per_page,omitempty" jsonschema:"items per page (1-100), default 20"`
}

type ListOutput struct {
    Items      []Item `json:"items"`
    TotalCount int    `json:"total_count"`
    Page       int    `json:"page"`
    PerPage    int    `json:"per_page"`
    TotalPages int    `json:"total_pages"`
    HasMore    bool   `json:"has_more"`
}
```

Guidelines:

- Always respect the `limit`/`per_page` parameter
- Default to 20-50 items per page
- Return pagination metadata: `has_more`, `total_count`, `page`, `total_pages`
- Never load all results into memory for large datasets

## JSON Schema Tags

Use `jsonschema` tags to document your structs for better client integration:

```go
type Input struct {
    Name     string   `json:"name" jsonschema:"required,description=User's name"`
    Age      int      `json:"age" jsonschema:"minimum=0,maximum=150"`
    Email    string   `json:"email,omitempty" jsonschema:"format=email"`
    Tags     []string `json:"tags,omitempty" jsonschema:"uniqueItems=true"`
    Active   bool     `json:"active" jsonschema:"default=true"`
}
```

## Context Usage

Always respect context cancellation and deadlines:

```go
func LongRunningTool(ctx context.Context, req *mcp.CallToolRequest, input Input) (
    *mcp.CallToolResult,
    Output,
    error,
) {
    select {
    case <-ctx.Done():
        return nil, Output{}, ctx.Err()
    case result := <-performWork(ctx, input):
        return nil, result, nil
    }
}
```

## Server Options

Configure server behavior with options:

```go
options := &mcp.Options{
    Capabilities: &mcp.ServerCapabilities{
        Tools:     &mcp.ToolsCapability{},
        Resources: &mcp.ResourcesCapability{
            Subscribe: true,
        },
        Prompts: &mcp.PromptsCapability{},
    },
}

server := mcp.NewServer(
    &mcp.Implementation{Name: "my-server", Version: "v1.5.0"},
    options,
)
```

## Testing

Test your MCP tools using standard Go testing patterns:

```go
func TestSearchTool(t *testing.T) {
    ctx := context.Background()
    input := ToolInput{Query: "test", Limit: 10}

    result, output, err := SearchTool(ctx, nil, input)
    if err != nil {
        t.Fatalf("SearchTool failed: %v", err)
    }

    if len(output.Results) == 0 {
        t.Error("Expected results, got none")
    }
}
```

## Module Setup

Initialize your Go module properly:

```bash
go mod init github.com/yourusername/yourserver
go get github.com/modelcontextprotocol/go-sdk@latest
```

Your `go.mod` should include:

```go
module github.com/yourusername/yourserver

go 1.23

require github.com/modelcontextprotocol/go-sdk v1.5.0
```

## Common Patterns

### Logging

Use structured logging:

```go
import "log/slog"

logger := slog.Default()
logger.Info("tool called", "name", req.Params.Name, "args", req.Params.Arguments)
```

### Configuration

Use environment variables or config files:

```go
type Config struct {
    ServerName string
    Version    string
    Port       int
}

func LoadConfig() *Config {
    return &Config{
        ServerName: getEnv("SERVER_NAME", "my-server"),
        Version:    getEnv("VERSION", "v1.0.0"),
        Port:       getEnvInt("PORT", 8080),
    }
}
```

### Graceful Shutdown

Handle shutdown signals properly:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigCh
    cancel()
}()

if err := server.Run(ctx, transport); err != nil {
    log.Fatal(err)
}
```

## Security Best Practices

### Authentication

- Store API keys/tokens in environment variables, never in code
- Validate credentials on server startup with clear error messages
- Use `context.Context` to pass auth information through call chains

### Input Validation

- Sanitize file paths to prevent directory traversal
- Validate URLs and external identifiers
- Check parameter sizes and ranges
- Use JSON schema struct tags for input constraints

### DNS Rebinding Protection (HTTP Transport)

For HTTP servers running locally:

- Bind to `127.0.0.1` rather than `0.0.0.0`
- Validate the `Origin` header on all incoming connections
- Enable DNS rebinding protection

### TLS Configuration

```go
// Support self-signed certificates when needed (e.g., internal GitLab)
if skipTLSVerify {
    httpClient.Transport = &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
}
```

## MCP SDK v1.5.0+ Features

### Tool Icons

Provide SVG icons for tools, resources, and prompts:

```go
mcp.AddTool(server, &mcp.Tool{
    Name:  "gitlab_issue_list",
    Icons: []mcp.IconURI{{URI: "data:image/svg+xml;base64,..."}},
}, handler)
```

### Typed Output with OutputSchema

Using the triple-return handler signature auto-generates `OutputSchema` and `StructuredContent`:

```go
mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
    // The Out type generates OutputSchema automatically
    return callToolResult, typedOutput, nil
})
```

### Sampling (LLM Interaction)

Request LLM assistance from within tool handlers:

```go
samplingReq := &mcp.CreateMessageRequest{
    Messages: []mcp.SamplingMessage{{
        Role:    mcp.RoleUser,
        Content: mcp.TextContent{Text: prompt},
    }},
    MaxTokens: 1000,
}
result, err := server.CreateMessage(ctx, samplingReq)
```

### Elicitation (User Input)

Request user input during tool execution:

```go
elicitReq := &mcp.ElicitRequest{
    Message: "Confirm deletion?",
    RequestedSchema: &mcp.ElicitRequestSchema{
        Properties: map[string]mcp.ElicitPropertySchema{
            "confirm": {Type: "boolean", Description: "Confirm deletion"},
        },
    },
}
result, err := server.Elicit(ctx, elicitReq)
```

### Completions

Provide argument autocompletion:

```go
mcp.AddCompletionProvider(server, mcp.NewRef(mcp.RefToolInput, "tool_name", "arg"),
    func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
        return &mcp.CompleteResult{
            Completion: mcp.Completion{Values: suggestions},
        }, nil
    })
```
