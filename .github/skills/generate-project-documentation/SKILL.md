---
name: generate-project-documentation
description: 'Generate comprehensive project documentation including architecture overview, package/component docs, MCP tools/resources/prompts reference, developer onboarding guide, and configuration/deployment guide. Uses Diátaxis framework and Mermaid diagrams.'
---

# Generate Project Documentation

## Primary Directive

Generate comprehensive, well-organized documentation for the project. Analyze the entire codebase and produce a complete documentation suite covering architecture, components, APIs, developer guides, and deployment configuration.

The documentation must be:

- Accurate (absolute parity with source code)
- Organized (Diátaxis framework: Tutorials, How-to, Reference, Explanation)
- Navigable (index with cross-references between documents)
- Visual (Mermaid diagrams for architecture and data flow)
- Self-contained (no external context required to understand)

## Execution Context

This skill is designed for the `Documentation Writer` agent or any agent tasked with producing project documentation. All output must be in English per the project language policy.

## Analysis Phase

Before generating any documentation, complete a thorough analysis:

### Step 1: Repository Structure Discovery

1. Explore the complete project directory structure
2. Identify all Go packages and their responsibilities
3. Map the dependency graph between internal packages
4. Locate existing documentation and assess gaps
5. List all configuration files, build scripts, and CI/CD pipelines

### Step 2: Architecture Understanding

1. Read the entry point (`cmd/server/main.go`) to understand initialization flow
2. Identify the configuration loading mechanism
3. Map how the GitLab client is initialized and injected
4. Understand the MCP server setup and tool registration pattern
5. Trace a complete request flow from MCP client to GitLab API and back

### Step 3: API Surface Inventory

1. List all MCP tools with their input/output types
2. List all MCP resources with their URI templates
3. List all MCP prompts with their parameters
4. Document pagination patterns and shared types
5. Identify tool annotations (readOnlyHint, destructiveHint, etc.)

### Step 4: Gap Analysis

1. Compare current documentation against the coverage matrix below
2. Identify missing, outdated, or incomplete documentation
3. Prioritize generation of missing critical documentation first

## Coverage Matrix

The following documents must be generated or updated:

| Document | Diátaxis Type | Path | Priority |
|----------|---------------|------|----------|
| Documentation Index | Navigation | `docs/README.md` | Critical |
| Architecture Overview | Explanation | `docs/architecture.md` | Critical |
| Package Reference: config | Reference | `docs/packages/config.md` | High |
| Package Reference: gitlab | Reference | `docs/packages/gitlab.md` | High |
| Package Reference: tools | Reference | `docs/packages/tools.md` | High |
| Package Reference: resources | Reference | `docs/packages/resources.md` | High |
| Package Reference: prompts | Reference | `docs/packages/prompts.md` | High |
| Tools Reference | Reference | `docs/tools/README.md` | Critical |
| Resources Reference | Reference | `docs/resources-reference.md` | High |
| Prompts Reference | Reference | `docs/prompts-reference.md` | High |
| Developer Onboarding Guide | Tutorial | `docs/onboarding.md` | High |
| Configuration Guide | Reference | `docs/configuration.md` | High |
| Deployment Guide | How-to | `docs/deployment.md` | Medium |
| Development Guide | How-to | `docs/development/development.md` | High |
| Testing Guide | How-to | `docs/development/testing.md` | Medium |
| Contributing Guide | How-to | `docs/contributing.md` | Medium |

## Document Templates

### Documentation Index (`docs/README.md`)

```markdown
# gitlab-mcp-server Documentation

> GitLab MCP Server — comprehensive documentation index

## Quick Navigation

| Document | Type | Description |
|----------|------|-------------|
| [Architecture](architecture.md) | Explanation | System architecture and design decisions |
| [Configuration](configuration.md) | Reference | Environment variables and transport modes |
| [Tools Reference](tools/README.md) | Reference | All MCP tools with parameters and examples |
| [Resources Reference](resources-reference.md) | Reference | MCP resources and URI templates |
| [Prompts Reference](prompts-reference.md) | Reference | MCP prompts and their parameters |
| [Development Guide](development.md) | How-to | Building, testing, and contributing |
| [Onboarding](onboarding.md) | Tutorial | Getting started for new developers |
| [Deployment](deployment.md) | How-to | Production deployment guide |

## Package Documentation

| Package | Path | Description |
|---------|------|-------------|
| [config](packages/config.md) | `internal/config` | Configuration loading |
| [gitlab](packages/gitlab.md) | `internal/gitlab` | GitLab API client wrapper |
| [tools](packages/tools.md) | `internal/tools` | MCP tool implementations |
| [resources](packages/resources.md) | `internal/resources` | MCP resource handlers |
| [prompts](packages/prompts.md) | `internal/prompts` | MCP prompt handlers |
```

### Architecture Overview (`docs/architecture.md`)

```markdown
# Architecture Overview

## System Context

[Describe what gitlab-mcp-server does at the highest level]

(mermaid) C4 Context diagram showing:
- MCP Clients (VS Code, Cursor, Copilot CLI, OpenCode)
- gitlab-mcp-server MCP Server
- GitLab REST API v4 and GraphQL API
- GitLab Instance

## Container View

(mermaid) Container diagram showing:
- stdio/HTTP transport layer
- MCP Server (go-sdk)
- Tool handlers
- GitLab client wrapper
- Configuration loader

## Component View

(mermaid) Component diagram showing all internal packages:
- cmd/server → config → gitlab client
- config → environment loading
- tools → tool handlers by domain (projects, MRs, branches, tags, releases)
- tools → pagination, resources, prompts

## Data Flow

(mermaid) Sequence diagram for a typical tool invocation:
MCP Client → MCP Server → Tool Handler → GitLab Client → GitLab API → response chain

## Key Design Decisions

[Reference ADRs from docs/adr/ directory]
[Explain: why go-sdk, why single-package tools, why pagination pattern, etc.]
```

### Package Documentation Template

```markdown
# Package: [name]

**Import path**: `github.com/[org]/gitlab-mcp-server/internal/[name]`
**Responsibility**: [one-sentence description]

## Overview

[What this package does and its role in the system]

## Architecture

(mermaid) Class/component diagram showing exported types and their relationships

## Exported Types

| Type | Kind | Description |
|------|------|-------------|
| `TypeName` | struct/interface | Purpose |

## Exported Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `FuncName` | `func(ctx, params) (result, error)` | Purpose |

## Configuration

[If applicable: environment variables, flags, or initialization parameters]

## Error Handling

[How errors are created, wrapped, and propagated]

## Dependencies

| Package | Purpose |
|---------|---------|
| `internal/gitlab` | GitLab API access |

## Usage Example

```go
// Minimal working example
```

```text

### Developer Onboarding Guide Template

```markdown
# Developer Onboarding Guide

## Welcome

[Brief project description and your role as a contributor]

## Prerequisites

- Go 1.24+
- GitLab instance with PAT
- VS Code with Go extension (recommended)

## Step 1: Clone and Build

[Commands and expected output]

## Step 2: Configure Environment

[.env setup with explanations]

## Step 3: Run the Server

[How to start and verify it works]

## Step 4: Test with an MCP Client

[How to connect VS Code, Cursor, or CLI tools (Copilot CLI, OpenCode)]

## Step 5: Understand the Codebase

[Guided tour of key files and packages with cross-references to package docs]

## Step 6: Make Your First Change

[Walk through adding a simple tool or modifying an existing one]

## Common Tasks

| Task | Guide |
|------|-------|
| Add a new MCP tool | [Development Guide](development.md#adding-tools) |
| Run tests | [Testing Guide](testing.md) |
| Build for production | [Deployment Guide](deployment.md) |

## Getting Help

[Where to find answers: docs, issues, team contacts]
```

## Generation Rules

1. **Source code is truth**: Read every Go file before documenting it; never guess
2. **Complete coverage**: Every exported type, function, and constant must appear in reference docs
3. **Working examples**: All code examples must be syntactically valid Go
4. **Mermaid diagrams**: Include at least one diagram per architecture/package document
5. **Cross-references**: Link between documents using relative paths
6. **No placeholders**: Final output must have no TBD, TODO, or placeholder text
7. **Consistent terminology**: Use the same names as the source code
8. **Pagination**: Document pagination patterns once and reference from each list tool

## Quality Checklist

Before considering documentation complete, verify:

- [ ] All items in coverage matrix are documented
- [ ] Documentation matches current implementation (parity check)
- [ ] All Mermaid diagrams render correctly
- [ ] All cross-reference links are valid
- [ ] No TBD/TODO placeholders remain
- [ ] Code examples are syntactically valid
- [ ] Tables are properly formatted
- [ ] English language used throughout
- [ ] Progressive disclosure: overview → details → advanced topics
