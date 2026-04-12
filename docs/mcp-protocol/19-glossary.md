# Glossary

> **Level**: 🟢 All levels
>
> Quick reference for all MCP terminology used throughout this guide. Terms link to their detailed explanations.

## A

**Annotations**
: Metadata attached to [tools](04-tools.md) that describe their behavior: `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`. Used by Hosts to make approval decisions.

**Audience Annotation**
: A content-level annotation that indicates who should see a piece of response content — `["user"]` for display in the IDE, `["assistant"]` for the AI to process internally. Helps MCP clients decide whether to show or hide parts of a tool response. See [Output Format](../output-format.md).

## C

**Capabilities**
: Feature declarations exchanged during [initialization](11-lifecycle.md). Clients declare support for roots, sampling, and elicitation. Servers declare support for tools, resources, prompts, logging, and completions. See [Capabilities](12-capabilities.md).

**Client**
: The protocol-level component inside a Host that maintains a 1:1 connection with an MCP server. Handles protocol messages, capability negotiation, and message routing. See [Key Concepts](02-key-concepts.md).

**Completions**
: Auto-complete suggestions for tool, prompt, and resource arguments. Requested via `completion/complete`. See [Completions](14-completions.md).

**Content Annotation**
: Metadata on individual content items within a tool response, specifying `audience` and `priority`. For example, Markdown content annotated with `audience: ["assistant"]` is intended for the AI only and should not be shown directly to the user. See [Output Format](../output-format.md).

## D

**Destructive Hint**
: A tool [annotation](04-tools.md) (`destructiveHint`) that indicates whether the tool performs irreversible operations like deleting files or dropping tables.

## E

**Elicitation**
: A mechanism for servers to request structured input from users via JSON Schema forms. The user can accept, decline, or cancel. See [Elicitation](08-elicitation.md).

## H

**Host**
: The application the user interacts with directly (e.g., VS Code, Claude Desktop). Manages one or more MCP clients, controls security policies, enforces user consent. See [Key Concepts](02-key-concepts.md).

**Human-in-the-Loop**
: A design pattern where the Host requires user approval before executing sensitive operations. Applied to tool execution, sampling, and data sharing. See [Security](16-security.md).

## I

**Idempotent Hint**
: A tool [annotation](04-tools.md) (`idempotentHint`) that indicates whether calling the tool multiple times with the same arguments produces the same result.

**Initialization**
: The first phase of the MCP connection [lifecycle](11-lifecycle.md). Client sends `initialize`, server responds with capabilities, client confirms with `initialized`.

## J

**JSON-RPC 2.0**
: The underlying message format for all MCP communication. Defines requests (with `id`), responses, and notifications (without `id`). See [Transport](10-transport.md).

## L

**Lifecycle**
: The three phases of an MCP connection: Initialization → Operation → Shutdown. See [Lifecycle](11-lifecycle.md).

**listChanged**
: A capability sub-option that enables dynamic update notifications. When `listChanged: true`, the server can notify the client when its tools, resources, or prompts list changes. See [Capabilities](12-capabilities.md).

**Logging**
: Structured server-to-client diagnostic messages sent via `notifications/message`. Uses syslog severity levels (debug through emergency). See [Logging](15-logging.md).

## M

**MCP (Model Context Protocol)**
: An open standard for connecting AI assistants to external data sources and tools. Provides a universal interface so any AI can work with any tool. See [What is MCP?](01-what-is-mcp.md).

**Meta-tool**
: A design pattern where multiple related tools are grouped under a single tool that dispatches based on an `action` parameter. Reduces the number of tools exposed to the AI while maintaining all functionality.

## N

**Next Steps**
: Contextual suggestions included in tool responses that tell the AI (or user) what actions can be taken after receiving results. In meta-tool mode, these are injected into the JSON `structuredContent` as a `next_steps` array. See [Output Format](../output-format.md).

**Notification**
: A JSON-RPC message without an `id` field. Fire-and-forget — no response is expected. Used for events like `notifications/tools/list_changed` and `notifications/progress`. See [Notifications and Progress](13-notifications-and-progress.md).

## O

**Open World Hint**
: A tool [annotation](04-tools.md) (`openWorldHint`) that indicates whether the tool interacts with systems outside the local environment (external APIs, internet services).

## P

**Pagination**
: A mechanism for retrieving large result sets in chunks. Tool responses include a `nextCursor` field when more results are available. The client sends the cursor in the next request.

**Progress**
: Real-time operation tracking via `notifications/progress`. Includes a `progressToken`, current value, optional total, and optional message. See [Notifications and Progress](13-notifications-and-progress.md).

**Prompts**
: Reusable message templates defined by servers with optional arguments. Discovered via `prompts/list`, retrieved via `prompts/get`. See [Prompts](06-prompts.md).

**Protocol Version**
: The MCP specification version negotiated during initialization. Current version: `2025-11-25`. Both sides must agree on a compatible version.

## R

**Read-Only Hint**
: A tool [annotation](04-tools.md) (`readOnlyHint`) that indicates whether the tool only reads data without modifying anything.

**Request**
: A JSON-RPC message with an `id` field that expects a response. Examples: `tools/list`, `tools/call`, `initialize`.

**Resources**
: Data sources exposed by servers via URI-based addressing. Discovered via `resources/list`, read via `resources/read`, with optional subscriptions for change notifications. See [Resources](05-resources.md).

**Resource Template**
: A parameterized resource URI like `gitlab://project/{project_id}/info` where `{project_id}` is filled at runtime. See [Resources](05-resources.md).

**Roots**
: URIs provided by clients that define advisory workspace boundaries for servers. Typically `file://` paths representing open project folders. See [Roots](09-roots.md).

## S

**Sampling**
: A mechanism for servers to request LLM completions through the client. The Host mediates the request, applying human oversight. See [Sampling](07-sampling.md).

**Server**
: An MCP component that exposes tools, resources, and prompts. May run locally (stdio) or remotely (HTTP). See [Key Concepts](02-key-concepts.md).

**Server-Sent Events (SSE)**
: An HTTP-based streaming mechanism used by the Streamable HTTP transport for server-to-client notifications and progress updates.

**Session**
: A single connection between a client and a server, from initialization through shutdown. In HTTP mode, tracked by `Mcp-Session-Id`.

**Streamable HTTP**
: An MCP [transport](10-transport.md) that uses HTTP POST for requests and Server-Sent Events for streaming. Designed for remote or multi-client deployments.

**structuredContent**
: A typed JSON object in a tool response that carries machine-readable data alongside the human-readable Markdown `content`. MCP clients like VS Code read `structuredContent` to render formatted results. Meta-tools enrich this object with a `next_steps` array. See [Output Format](../output-format.md).

**stdio**
: An MCP [transport](10-transport.md) where the client launches the server as a child process and communicates via standard input/output streams. See [Transport](10-transport.md).

## T

**Tools**
: Functions exposed by servers that the AI can call to perform actions. Have typed input schemas, return structured results, and include annotations for safety. See [Tools](04-tools.md).

**Transport**
: The physical communication channel between client and server. MCP defines two: stdio (local) and Streamable HTTP (remote). See [Transport](10-transport.md).

**Trust Boundary**
: The security perimeter enforced by the Host between the user and MCP servers. All sensitive operations must cross this boundary with user consent. See [Security](16-security.md).

## U

**URI (Uniform Resource Identifier)**
: The addressing scheme used for resources (`gitlab://project/42/info`) and roots (`file:///home/user/project`).

## References

- [MCP Specification (Complete)](https://modelcontextprotocol.io/specification/latest)
- [MCP Concepts — Architecture](https://modelcontextprotocol.io/docs/concepts/architecture)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
