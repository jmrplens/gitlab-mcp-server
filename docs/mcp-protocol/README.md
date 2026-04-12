# MCP Protocol Guide

A comprehensive guide to the **Model Context Protocol (MCP)** — the open standard that connects AI applications to external tools, data, and services.

This guide explains MCP from the ground up: what it is, how it works, and why it matters. No prior AI or protocol experience required.

## Learning Paths

Choose the path that matches your experience level:

### 🟢 Beginner Path — "I'm new to AI and MCP"

Start here if you've never used MCP or have minimal experience with AI tools.

| Order | Document | Topic |
|-------|----------|-------|
| 1 | [What is MCP?](01-what-is-mcp.md) | The problem MCP solves and why it exists |
| 2 | [Key Concepts](02-key-concepts.md) | Host, Client, Server — the three participants |
| 3 | [How Prompts Reach MCP](03-how-prompts-reach-mcp.md) | The invisible journey from your prompt to an MCP tool |
| 4 | [The MCP Ecosystem](18-ecosystem.md) | Popular hosts, servers, SDKs, and tools |
| 5 | [Glossary](19-glossary.md) | A-Z definitions of all MCP terms |

### 🟡 Intermediate Path — "I use AI tools and want to understand MCP deeply"

After completing the Beginner Path, dive into the protocol's building blocks.

| Order | Document | Topic |
|-------|----------|-------|
| 1 | [Tools](04-tools.md) | Executable functions the AI can invoke |
| 2 | [Resources](05-resources.md) | Read-only data sources for AI context |
| 3 | [Prompts](06-prompts.md) | Reusable interaction templates |
| 4 | [Sampling](07-sampling.md) | Servers asking the AI for help |
| 5 | [Elicitation](08-elicitation.md) | Servers asking the user for input |
| 6 | [Roots](09-roots.md) | Defining filesystem boundaries |
| 7 | [Transport](10-transport.md) | How client and server communicate |
| 8 | [Lifecycle](11-lifecycle.md) | Connection initialization and shutdown |
| 9 | [Capabilities](12-capabilities.md) | What client and server can do |
| 10 | [Notifications and Progress](13-notifications-and-progress.md) | Real-time updates and progress tracking |
| 11 | [Completions](14-completions.md) | Autocomplete for arguments |
| 12 | [Logging](15-logging.md) | Server messages to clients |

### 🔴 Advanced Path — "I want protocol-level and security details"

Deep-dive into security, end-to-end scenarios, and complete protocol walkthroughs.

| Order | Document | Topic |
|-------|----------|-------|
| 1 | [Security](16-security.md) | Transport security, authentication, trust boundaries |
| 2 | [Putting It All Together](17-putting-it-all-together.md) | Complete multi-server interaction walkthrough |

## Full Table of Contents

| # | Document | Level | Description |
|---|----------|-------|-------------|
| 01 | [What is MCP?](01-what-is-mcp.md) | 🟢 | Introduction to MCP: the problem, the analogy, the solution |
| 02 | [Key Concepts](02-key-concepts.md) | 🟢 | Host, Client, Server — the three participants |
| 03 | [How Prompts Reach MCP](03-how-prompts-reach-mcp.md) | 🟢 | Step-by-step flow from user prompt to MCP tool execution |
| 04 | [Tools](04-tools.md) | 🟡 | Executable functions: discovery, execution, annotations |
| 05 | [Resources](05-resources.md) | 🟡 | Read-only data sources: URIs, templates, subscriptions |
| 06 | [Prompts](06-prompts.md) | 🟡 | Reusable templates: arguments, workflows, UI patterns |
| 07 | [Sampling](07-sampling.md) | 🟡 | Server-initiated LLM completions with human oversight |
| 08 | [Elicitation](08-elicitation.md) | 🟡 | Structured user input requests with form schemas |
| 09 | [Roots](09-roots.md) | 🟡 | Filesystem boundaries and access coordination |
| 10 | [Transport](10-transport.md) | 🟡🔴 | stdio and Streamable HTTP transport mechanisms |
| 11 | [Lifecycle](11-lifecycle.md) | 🟡 | Initialization handshake and capability negotiation |
| 12 | [Capabilities](12-capabilities.md) | 🟡 | Server and client capability declarations |
| 13 | [Notifications and Progress](13-notifications-and-progress.md) | 🟡 | Real-time notifications, progress tracking, cancellation |
| 14 | [Completions](14-completions.md) | 🟡 | Argument autocomplete for prompts and resource templates |
| 15 | [Logging](15-logging.md) | 🟡 | Log levels and server-to-client message logging |
| 16 | [Security](16-security.md) | 🔴 | Authentication, TLS, trust boundaries, threat model |
| 17 | [Putting It All Together](17-putting-it-all-together.md) | 🟡🔴 | Complete end-to-end multi-server interaction |
| 18 | [The MCP Ecosystem](18-ecosystem.md) | 🟢🟡 | Hosts, servers, SDKs, inspector, governance |
| 19 | [Glossary](19-glossary.md) | 🟢 | A-Z definitions of all MCP terms |

## About This Guide

- **Source of truth**: [MCP Specification](https://modelcontextprotocol.io/specification/latest) (protocol version 2025-11-25)
- **Framework**: [Diátaxis](https://diataxis.fr/) — Tutorials + Explanations + Reference
- **Diagrams**: [Mermaid](https://mermaid.js.org/) — renders natively in GitHub, GitLab, and VS Code
- **Governance**: MCP is an open standard under [LF Projects, LLC](https://lfprojects.org/)

## References

- [MCP Specification (latest)](https://modelcontextprotocol.io/specification/latest)
- [MCP Introduction](https://modelcontextprotocol.io/introduction)
- [MCP Architecture Overview](https://modelcontextprotocol.io/docs/learn/architecture)
- [MCP GitHub Repository](https://github.com/modelcontextprotocol/modelcontextprotocol)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
