# Putting It All Together

> **Level**: 🔴 Advanced
>
> **What You'll Learn**:
>
> - How all MCP concepts work together in a real-world scenario
> - A complete end-to-end GitLab workflow traced through the protocol
> - How the protocol handles errors and edge cases
> - Architecture patterns for production MCP servers

## A Complete Scenario

Let's trace a realistic workflow through every protocol layer: **"Find open bugs in my project, create a fix branch, and draft a merge request."**

This scenario touches: initialization, capabilities, tools, resources, prompts, sampling, progress, completions, logging, and security — all working together.

### The Setup

- **Host**: VS Code with GitHub Copilot
- **MCP Server**: gitlab-mcp-server (this project)
- **Transport**: stdio (local binary)
- **User workspace**: A GitLab project

## Step 1: Connection and Initialization

```mermaid
sequenceDiagram
    participant U as 👤 User
    participant H as 🖥️ VS Code
    participant C as 🔌 MCP Client
    participant S as ⚙️ gitlab-mcp-server

    Note over H: User opens VS Code with<br/>MCP server configured

    H->>S: Spawn process (stdio)
    C->>S: initialize (protocol + capabilities)
    S-->>C: Server capabilities + info
    C->>S: initialized

    Note over C,S: Connection ready
```

**What happens behind the scenes:**

1. VS Code reads the MCP server configuration
2. Spawns `gitlab-mcp-server` as a child process
3. The MCP client sends `initialize` with client capabilities (roots, sampling, elicitation)
4. The server responds with its capabilities (tools, resources, prompts, logging, completions)
5. The client confirms with `initialized`

## Step 2: User Makes a Request

The user types in the chat:

> "Find all open bugs in my project and create a fix branch for the most critical one."

```mermaid
flowchart TD
    U["User: 'Find all open bugs and fix the critical one'"]
    H["Host analyzes intent"]
    H --> D1["Step A: List issues (bugs, open)"]
    H --> D2["Step B: Identify most critical"]
    H --> D3["Step C: Create fix branch"]
    H --> D4["Step D: Draft merge request"]

    style U fill:#3498DB,color:#fff
```

## Step 3: Tool Discovery

The AI first needs to know what tools are available:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

The server returns hundreds of tools. The AI identifies the relevant ones:

- `gitlab_list_issues` — find bugs
- `gitlab_create_branch` — create fix branch
- `gitlab_create_merge_request` — draft the MR

## Step 4: Finding Bugs (Tools + Completions)

The AI calls the list issues tool with filters:

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "gitlab_list_issues",
    "arguments": {
      "project_id": "my-project",
      "labels": "bug",
      "state": "opened",
      "order_by": "priority"
    },
    "_meta": {
      "progressToken": "find-bugs"
    }
  }
}
```

The server:

1. Sends a **log message**: `info` — "Querying GitLab API for open bugs"
2. Sends **progress** updates as it fetches paginated results
3. Returns the list of matching issues

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/progress",
  "params": {
    "progressToken": "find-bugs",
    "progress": 1,
    "total": 1,
    "message": "Fetching issues from GitLab..."
  }
}
```

## Step 5: Creating the Fix Branch (Security + Consent)

The AI identifies bug #142 as the most critical and wants to create a branch:

```mermaid
sequenceDiagram
    participant AI as 🤖 AI
    participant H as 🖥️ Host
    participant U as 👤 User
    participant S as ⚙️ Server

    AI->>H: tools/call: gitlab_create_branch<br/>branch: "fix/critical-bug-142"
    Note over H: Tool has readOnlyHint: false<br/>Requires user approval
    H->>U: "Create branch 'fix/critical-bug-142'?"
    U-->>H: ✅ Approve
    H->>S: Forward tools/call
    S-->>H: Branch created
    H-->>AI: Success result
```

The Host checks tool annotations:

- `readOnlyHint: false` — this modifies data
- `destructiveHint: false` — not destructive
- `idempotentHint: false` — creates a new resource

Based on the security policy, it asks the user for approval before executing.

## Step 6: Drafting the Merge Request (Sampling)

Now the AI wants to write a good MR description. The server uses **sampling** to ask the AI for help composing it:

```mermaid
sequenceDiagram
    participant AI as 🤖 AI
    participant C as 🔌 Client
    participant S as ⚙️ Server
    participant U as 👤 User

    Note over AI,S: Server creating MR,<br/>needs a good description

    S->>C: sampling/createMessage<br/>"Write MR description for bug #142 fix"
    C->>U: Review sampling request
    U-->>C: Approve
    Note over C: LLM generates description
    C-->>S: "Fix: Resolve critical null pointer in auth module..."
    Note over S: Uses description to<br/>create merge request
```

## Step 7: The Complete Result

After all operations complete, the AI presents the result to the user:

> **Done. Here's what I did:**
>
> 1. Found **8 open bugs** in your project
> 2. Bug #142 (Critical): "Null pointer exception in authentication module"
> 3. Created branch `fix/critical-bug-142` from `main`
> 4. Created Merge Request !89: "Fix: Resolve critical null pointer in auth module"

## How Protocol Layers Interact

Here's how all the pieces connected in this scenario:

```mermaid
flowchart TB
    subgraph "Protocol Foundation"
        T["Transport (stdio)"]
        L["Lifecycle (init → operate → shutdown)"]
        C["Capabilities (tools, sampling, ...)"]
    end

    subgraph "Server Primitives"
        Tools["Tools (list_issues, create_branch, create_mr)"]
        Res["Resources (project info)"]
        Pro["Prompts (analysis templates)"]
    end

    subgraph "Client Primitives"
        Sam["Sampling (MR description)"]
        Roots["Roots (workspace path)"]
    end

    subgraph "Utilities"
        Comp["Completions (project name)"]
        Prog["Progress (operation tracking)"]
        Log["Logging (API calls, warnings)"]
        Notif["Notifications (list changes)"]
    end

    subgraph "Cross-Cutting"
        Sec["Security (consent, annotations, trust)"]
    end

    T --> L
    L --> C
    C --> Tools & Res & Pro & Sam & Roots
    Tools --> Comp & Prog & Log
    Sec -.-> Tools & Sam & Roots
```

## Error Handling Patterns

In a real scenario, things can go wrong. Here's how MCP handles common errors:

| Error | Protocol Response | Recovery |
|-------|------------------|----------|
| GitLab API timeout | `tools/call` returns `isError: true` with timeout message | AI retries or informs user |
| Branch name exists | Server returns descriptive error | AI suggests different name |
| Permission denied | Server returns 403 with explanation | AI explains required permissions |
| Rate limit hit | Server sends `warning` log, delays, retries | Transparent to user with progress updates |
| Network failure | Transport-level error | Client reconnects or reports |

### Error Response Example

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Error: Branch 'fix/critical-bug-142' already exists"
      }
    ],
    "isError": true
  }
}
```

## Architecture Pattern Summary

| Pattern | How It's Used |
|---------|---------------|
| **Capability negotiation** | Server and client agree on features at startup |
| **Progressive disclosure** | AI discovers tools via `tools/list`, uses only what's needed |
| **Human-in-the-loop** | Host requires approval for write operations |
| **Structured errors** | `isError: true` with descriptive messages |
| **Progress tracking** | Long operations report progress with tokens |
| **Layered security** | Transport security + capability restrictions + user consent |
| **Graceful degradation** | Missing capabilities handled with fallbacks |

## Key Takeaways

- MCP is a **layered protocol** — transport, lifecycle, capabilities, primitives, and utilities each play a role
- A single user request can involve **multiple tools**, **sampling**, **completions**, **progress**, and **logging**
- **Security** is a cross-cutting concern — consent checks happen at every write operation
- **Error handling** uses `isError: true` with descriptive messages — not exceptions
- The **Host** orchestrates everything: spawning servers, enforcing policies, presenting results
- Real workflows are **multi-step** — the AI plans and executes a sequence of tool calls

## Next Steps

- [Ecosystem](18-ecosystem.md) — Explore the MCP ecosystem: servers, clients, and community
- [Glossary](19-glossary.md) — Quick reference for all MCP terminology
- [What is MCP?](01-what-is-mcp.md) — Revisit the fundamentals with your new understanding

## References

- [MCP Specification (Complete)](https://modelcontextprotocol.io/specification/latest)
- [MCP Architecture](https://modelcontextprotocol.io/docs/concepts/architecture)
- [MCP Server Concepts](https://modelcontextprotocol.io/docs/concepts/servers)
