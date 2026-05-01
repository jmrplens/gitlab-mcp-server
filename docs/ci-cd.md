# CI/CD Usage

How to use gitlab-mcp-server in CI/CD pipelines for automated GitLab operations — with or without an LLM.

> **Diátaxis type**: How-to Guide
> **Audience**: DevOps engineers, CI/CD maintainers
> **Prerequisites**: [Getting Started](getting-started.md), [Configuration](configuration.md)
> 📖 **User documentation**: See the [CI/CD Usage](https://jmrplens.github.io/gitlab-mcp-server/operations/ci-cd/) on the documentation site for a user-friendly version.

---

## Overview

gitlab-mcp-server can run inside CI/CD jobs just like any other CLI tool. Two usage modes are available:

| Mode | LLM Required | Use Case | Determinism |
| --- | :---: | --- | :---: |
| **Deterministic** (JSON-RPC) | No | Scripted operations: list issues, post comments, create releases | ✅ Fully deterministic |
| **LLM-driven** (headless MCP client) | Yes | Intelligent workflows: code review, issue triage, MR analysis | ❌ Non-deterministic |

Both modes authenticate with a **Personal Access Token** (PAT) or **Project Access Token**. The server supports the full 1006-tool surface when using a token with `api` scope.

---

## Prerequisites

### 1. Download the Binary

Add a step to download the server binary from [GitHub Releases](https://github.com/jmrplens/gitlab-mcp-server/releases):

```bash
# Download latest release for Linux amd64
curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64" \
  -o gitlab-mcp-server
chmod +x gitlab-mcp-server
```

For pinned versions, replace `latest` with a specific tag:

```bash
curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/download/v1.0.0/gitlab-mcp-server-linux-amd64" \
  -o gitlab-mcp-server
chmod +x gitlab-mcp-server
```

### 2. Create an Access Token

Create a **Project Access Token** (recommended over personal PATs for CI):

1. Go to **Settings > Access Tokens** in your GitLab project.
2. Create a token with the required scope:
   - `api` — full read/write access to all 1006 tools
   - `read_api` — read-only operations (list, get, search tools)
3. Set an expiration date (90 days maximum recommended).

### 3. Store the Token as a CI Variable

**GitLab CI**: Go to **Settings > CI/CD > Variables** and add:

| Variable | Value | Properties |
| --- | --- | --- |
| `MCP_PAT` | `glpat-xxxx...` | Masked, Protected (optional) |

**GitHub Actions**: Go to **Settings > Secrets and variables > Actions** and add a repository secret named `MCP_PAT`.

> **Security**: Never hardcode tokens in pipeline files. Always use masked CI/CD variables.

---

## Mode 1: Deterministic (No LLM)

Send JSON-RPC messages directly to the server via stdio. This is fully deterministic — no LLM or external API needed.

### How It Works

The server communicates via the [MCP protocol](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports#stdio) over stdin/stdout using JSON-RPC 2.0. Each interaction requires:

1. An `initialize` handshake
2. An `initialized` notification
3. One or more `tools/call` requests

### Basic Example

```bash
#!/bin/bash
set -euo pipefail

export GITLAB_URL="${CI_SERVER_URL}"
export GITLAB_TOKEN="${MCP_PAT}"

# Initialize MCP session + call a tool in a single pipeline
{
  # 1. Initialize handshake
  echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"ci-script","version":"1.0"}},"id":1}'
  # 2. Initialized notification
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  # 3. Call a tool
  echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"gitlab_list_issues","arguments":{"project_id":"'"${CI_PROJECT_ID}"'","state":"opened","per_page":10}},"id":2}'
} | ./gitlab-mcp-server 2>/dev/null | jq -s '.[1]'
```

The `jq -s '.[1]'` selects the second JSON response (the tool result, skipping the initialize response).

### GitLab CI Example

```yaml
# .gitlab-ci.yml
mcp-list-issues:
  stage: test
  image: alpine:latest
  variables:
    GITLAB_URL: ${CI_SERVER_URL}
    GITLAB_TOKEN: ${MCP_PAT}
  before_script:
    - apk add --no-cache curl jq
    - curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64"
        -o gitlab-mcp-server
    - chmod +x gitlab-mcp-server
  script:
    - |
      RESULT=$({
        echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"ci","version":"1.0"}},"id":1}'
        echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
        echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"gitlab_list_issues","arguments":{"project_id":"'"${CI_PROJECT_ID}"'","state":"opened","per_page":5}},"id":2}'
      } | ./gitlab-mcp-server 2>/dev/null | jq -s '.[1].result.content[0].text')
    - echo "${RESULT}"
```

### Multi-Tool Chaining

Chain multiple tool calls in a single session:

```bash
#!/bin/bash
set -euo pipefail

export GITLAB_URL="${CI_SERVER_URL}"
export GITLAB_TOKEN="${MCP_PAT}"

{
  echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"ci","version":"1.0"}},"id":1}'
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'

  # List open MRs
  echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"gitlab_list_merge_requests","arguments":{"project_id":"'"${CI_PROJECT_ID}"'","state":"opened"}},"id":2}'

  # Get project details
  echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"gitlab_get_project","arguments":{"project_id":"'"${CI_PROJECT_ID}"'"}},"id":3}'
} | ./gitlab-mcp-server 2>/dev/null | jq -s '.'
```

### Helper Function

For pipelines with many tool calls, use a helper function:

```bash
#!/bin/bash
set -euo pipefail

export GITLAB_URL="${CI_SERVER_URL}"
export GITLAB_TOKEN="${MCP_PAT}"

mcp_call() {
  local tool="$1"
  local args="$2"
  local id="${3:-2}"

  {
    echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"ci","version":"1.0"}},"id":1}'
    echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
    echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"'"${tool}"'","arguments":'"${args}"'},"id":'"${id}"'}'
  } | ./gitlab-mcp-server 2>/dev/null | jq -s '.[1].result.content[0].text' -r
}

# Usage
ISSUES=$(mcp_call "gitlab_list_issues" '{"project_id":"'"${CI_PROJECT_ID}"'","state":"opened"}')
echo "Open issues: ${ISSUES}"

MR_DETAILS=$(mcp_call "gitlab_get_merge_request" '{"project_id":"'"${CI_PROJECT_ID}"'","merge_request_iid":"'"${CI_MERGE_REQUEST_IID}"'"}')
echo "MR details: ${MR_DETAILS}"
```

---

## Mode 2: LLM-Driven (Headless MCP Client)

Use a headless MCP client to let an LLM drive tool selection and orchestration. This mode is ideal for intelligent workflows like code review, issue triage, and MR analysis.

### Recommended Client: IBM mcp-cli

[IBM mcp-cli](https://github.com/IBM/mcp-cli) is the most mature headless MCP client:

- Command mode (`mcp-cli cmd`) for scriptable LLM-driven workflows
- Direct tool execution: `mcp-cli cmd --tool <name> --tool-args '{...}'`
- Natural language prompts: `mcp-cli cmd --prompt "..."`
- Supports OpenAI, Anthropic, Azure, Gemini, Groq, and local Ollama
- DAG-based execution plans for multi-step orchestration

### Server Configuration

Create a `server_config.json` for mcp-cli:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "./gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "${GITLAB_URL}",
        "GITLAB_TOKEN": "${GITLAB_TOKEN}"
      }
    }
  }
}
```

### GitLab CI: Automated MR Review

```yaml
# .gitlab-ci.yml
auto-review:
  stage: review
  image: python:3.12-slim
  variables:
    GITLAB_URL: ${CI_SERVER_URL}
    GITLAB_TOKEN: ${MCP_PAT}
    OPENAI_API_KEY: ${OPENAI_KEY}
  before_script:
    - apt-get update && apt-get install -y curl jq
    - curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64"
        -o gitlab-mcp-server
    - chmod +x gitlab-mcp-server
    - pip install --quiet mcp-cli
  script:
    - |
      cat > server_config.json << 'EOF'
      {
        "mcpServers": {
          "gitlab": {
            "command": "./gitlab-mcp-server",
            "env": {
              "GITLAB_URL": "${GITLAB_URL}",
              "GITLAB_TOKEN": "${GITLAB_TOKEN}"
            }
          }
        }
      }
      EOF
    - |
      mcp-cli cmd \
        --config-file server_config.json \
        --server gitlab \
        --provider openai \
        --model gpt-4o \
        --prompt "Review merge request !${CI_MERGE_REQUEST_IID} in project ${CI_PROJECT_ID}. Check for code quality issues, security concerns, and missing tests. Post your review as a note on the MR." \
        --raw
  rules:
    - if: $CI_MERGE_REQUEST_IID
```

### GitLab CI: Issue Triage

```yaml
# .gitlab-ci.yml
triage-issues:
  stage: deploy
  image: python:3.12-slim
  variables:
    GITLAB_URL: ${CI_SERVER_URL}
    GITLAB_TOKEN: ${MCP_PAT}
    OPENAI_API_KEY: ${OPENAI_KEY}
  before_script:
    - apt-get update && apt-get install -y curl
    - curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64"
        -o gitlab-mcp-server
    - chmod +x gitlab-mcp-server
    - pip install --quiet mcp-cli
  script:
    - |
      cat > server_config.json << 'EOF'
      {
        "mcpServers": {
          "gitlab": {
            "command": "./gitlab-mcp-server",
            "env": {
              "GITLAB_URL": "${GITLAB_URL}",
              "GITLAB_TOKEN": "${GITLAB_TOKEN}"
            }
          }
        }
      }
      EOF
    - |
      mcp-cli cmd \
        --config-file server_config.json \
        --server gitlab \
        --provider openai \
        --model gpt-4o \
        --prompt "List all open issues in project ${CI_PROJECT_ID} without labels. For each issue, analyze its content and add appropriate labels (bug, feature, documentation, etc.)." \
        --raw
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
```

### Using a Local LLM (Ollama)

For pipelines that cannot use external LLM APIs:

```yaml
# .gitlab-ci.yml
local-llm-review:
  stage: review
  image: python:3.12-slim
  services:
    - name: ollama/ollama:latest
      alias: ollama
  variables:
    GITLAB_URL: ${CI_SERVER_URL}
    GITLAB_TOKEN: ${MCP_PAT}
    OLLAMA_HOST: http://ollama:11434
  before_script:
    - apt-get update && apt-get install -y --no-install-recommends curl
    - curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64"
        -o gitlab-mcp-server
    - chmod +x gitlab-mcp-server
    - pip install --quiet mcp-cli
    # Pull a model (once, cached in CI)
    - curl -s "${OLLAMA_HOST}/api/pull" -d '{"name":"qwen2.5-coder:7b"}'
  script:
    - |
      cat > server_config.json << 'EOF'
      {
        "mcpServers": {
          "gitlab": {
            "command": "./gitlab-mcp-server",
            "env": {
              "GITLAB_URL": "${GITLAB_URL}",
              "GITLAB_TOKEN": "${GITLAB_TOKEN}"
            }
          }
        }
      }
      EOF
    - |
      mcp-cli cmd \
        --config-file server_config.json \
        --server gitlab \
        --provider ollama \
        --model qwen2.5-coder:7b \
        --prompt "Summarize the latest 5 merge requests in project ${CI_PROJECT_ID}." \
        --raw
```

---

## Using HTTP Transport in CI

For pipelines that make many tool calls, the HTTP transport avoids process startup overhead per call:

```yaml
# .gitlab-ci.yml
http-mode-pipeline:
  stage: test
  image: alpine:latest
  variables:
    MCP_PAT: ${MCP_PAT}
  before_script:
    - apk add --no-cache curl jq
    - curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64"
        -o gitlab-mcp-server
    - chmod +x gitlab-mcp-server
  script:
    # Start HTTP server in background
    - |
      ./gitlab-mcp-server --http \
        --gitlab-url="${CI_SERVER_URL}" \
        --http-addr=127.0.0.1:8080 &
      sleep 2

    # Initialize session
    - |
      SESSION=$(curl -s -X POST http://127.0.0.1:8080/mcp \
        -H "Content-Type: application/json" \
        -H "PRIVATE-TOKEN: ${MCP_PAT}" \
        -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"ci","version":"1.0"}},"id":1}')

    # Call tools via HTTP
    - |
      curl -s -X POST http://127.0.0.1:8080/mcp \
        -H "Content-Type: application/json" \
        -H "PRIVATE-TOKEN: ${MCP_PAT}" \
        -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"gitlab_list_issues","arguments":{"project_id":"'"${CI_PROJECT_ID}"'","state":"opened"}},"id":2}' \
        | jq '.result.content[0].text'
```

See [HTTP Server Mode](http-server-mode.md) for full details on flags and authentication.

---

## GitHub Actions Examples

### Deterministic Mode

```yaml
# .github/workflows/mcp-query.yml
name: MCP Query
on:
  workflow_dispatch:

jobs:
  list-issues:
    runs-on: ubuntu-latest
    env:
      GITLAB_TOKEN: ${{ secrets.MCP_PAT }}
    steps:
      - name: Download gitlab-mcp-server
        run: |
          curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64" \
            -o gitlab-mcp-server
          chmod +x gitlab-mcp-server

      - name: List open issues
        run: |
          RESULT=$({
            echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"ci","version":"1.0"}},"id":1}'
            echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
            echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"gitlab_list_issues","arguments":{"project_id":"12345","state":"opened","per_page":5}},"id":2}'
          } | ./gitlab-mcp-server 2>/dev/null | jq -s '.[1].result.content[0].text' -r)
          echo "${RESULT}"
```

### LLM-Driven Mode

```yaml
# .github/workflows/auto-review.yml
name: Auto Review
on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    env:
      GITLAB_TOKEN: ${{ secrets.MCP_PAT }}
      OPENAI_API_KEY: ${{ secrets.OPENAI_KEY }}
    steps:
      - name: Setup
        run: |
          curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64" \
            -o gitlab-mcp-server
          chmod +x gitlab-mcp-server
          pip install mcp-cli

      - name: Run review
        run: |
          cat > server_config.json << 'EOF'
          {
            "mcpServers": {
              "gitlab": {
                "command": "./gitlab-mcp-server",
                "env": {
                  "GITLAB_URL": "${{ env.GITLAB_URL }}",
                  "GITLAB_TOKEN": "${{ env.GITLAB_TOKEN }}"
                }
              }
            }
          }
          EOF
          mcp-cli cmd \
            --config-file server_config.json \
            --server gitlab \
            --provider openai \
            --model gpt-4o \
            --prompt "Analyze the latest changes in the GitLab project and summarize them." \
            --raw
```

---

## Security Best Practices

### Token Management

| Practice | Recommendation |
| --- | --- |
| Token type | **Project Access Token** — scoped to a single project, auditable |
| Scope | `api` for full access, `read_api` for read-only workflows |
| Expiration | Set to 90 days maximum, rotate before expiry |
| Storage | **Masked CI/CD variable** — never commit to repository |
| Visibility | Use "Protected" flag if only needed on protected branches |

### Minimal Scope Strategy

Match the token scope to your workflow:

| Workflow | Required Scope |
| --- | --- |
| List issues, MRs, pipelines | `read_api` |
| Post comments, create issues | `api` |
| Manage releases, packages | `api` |
| Full MR review workflow | `api` |

### Group Access Tokens

For multi-project workflows, use a **Group Access Token** instead of creating per-project tokens:

1. Go to the group's **Settings > Access Tokens**.
2. Create a token with the required scope.
3. The token works across all projects in the group.

---

## Troubleshooting

### Binary Not Found or Permission Denied

```text
/bin/sh: ./gitlab-mcp-server: not found
```

Ensure the binary is downloaded for the correct platform and has execute permissions:

```bash
curl -sSL "https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux-amd64" \
  -o gitlab-mcp-server
chmod +x gitlab-mcp-server
./gitlab-mcp-server --version  # verify it runs
```

### Token Authentication Errors

```text
401 Unauthorized
```

- Verify the `MCP_PAT` CI variable is set and not expired.
- Check the token has the required scope (`api` or `read_api`).
- For Project Access Tokens, ensure the token belongs to the correct project.

### TLS Errors with Self-Hosted GitLab

```text
x509: certificate signed by unknown authority
```

Set `GITLAB_SKIP_TLS_VERIFY=true` in your pipeline variables:

```yaml
variables:
  GITLAB_URL: https://gitlab.internal.example.com
  GITLAB_TOKEN: ${MCP_PAT}
  GITLAB_SKIP_TLS_VERIFY: "true"
```

### Timeout on Large Responses

For tools that return large datasets (e.g., listing hundreds of issues), add `per_page` to limit results:

```json
{"name": "gitlab_list_issues", "arguments": {"project_id": "123", "per_page": 20}}
```

### mcp-cli Provider Errors

If mcp-cli fails to connect to the LLM provider:

- Verify the API key variable is set (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`).
- For Ollama, ensure the service is running and accessible at the configured host.
- Check mcp-cli version compatibility: `pip install --upgrade mcp-cli`.

---

## Related Documentation

- [Configuration](configuration.md) — Environment variables and server setup
- [HTTP Server Mode](http-server-mode.md) — HTTP transport for multi-call pipelines
- [IDE Configuration](ide-configuration.md) — Interactive client setup (VS Code, Cursor, etc.)
- [Environment Variables](env-reference.md) — Complete variable reference
- [Security](security.md) — Token management and security model
