# IDE Configuration

Per-IDE MCP client configuration examples for gitlab-mcp-server, covering both stdio and HTTP modes (legacy and OAuth).

> **Diátaxis type**: Reference
> **Audience**: 👤 End users, AI assistant users
> **Prerequisites**: gitlab-mcp-server installed; for OAuth: a GitLab OAuth Application created (see [OAuth App Setup](oauth-app-setup.md))

---

## OAuth Support Matrix

| IDE / Client               | Stdio | HTTP Legacy | HTTP OAuth | Official MCP Docs                                                                                 |
| -------------------------- | :---: | :---------: | :--------: | ------------------------------------------------------------------------------------------------- |
| VS Code (GitHub Copilot)   |   ✅   |      ✅      |     ✅      | [VS Code MCP docs](https://code.visualstudio.com/docs/copilot/chat/mcp-servers)                   |
| Claude Code                |   ✅   |      ✅      |     ✅      | [Claude Code MCP docs](https://docs.anthropic.com/en/docs/claude-code/mcp)                        |
| Claude Desktop / claude.ai |   ✅   |      ✅      |     ✅      | [Claude Desktop MCP docs](https://modelcontextprotocol.io/quickstart/user)                        |
| Cursor                     |   ✅   |      ✅      |     ✅      | [Cursor MCP docs](https://docs.cursor.com/context/model-context-protocol)                         |
| OpenAI Codex CLI           |   ✅   |      ✅      |     ✅      | [Codex MCP docs](https://developers.openai.com/codex/mcp)                                         |
| Gemini CLI                 |   ✅   |      ✅      |     ✅      | [Gemini CLI MCP docs](https://google-gemini.github.io/gemini-cli/docs/tools/mcp-server.html)      |
| LM Studio                  |   ✅   |      ✅      |     ✅      | [LM Studio MCP docs](https://lmstudio.ai/docs/app/plugins/mcp)                                    |
| mcp-remote (proxy)         |   ✅   |      ✅      |     ✅      | [mcp-remote](https://github.com/geelen/mcp-remote)                                                |
| GitLab Duo Agent Platform  |   —   |      ✅      |     —      | [GitLab MCP clients](https://docs.gitlab.com/user/gitlab_duo/model_context_protocol/mcp_clients/) |
| Windsurf                   |   ✅   |      ✅      |     —      | [Windsurf MCP docs](https://docs.windsurf.com/windsurf/cascade/mcp)                               |
| JetBrains IDEs             |   ✅   |      ✅      |     —      | [JetBrains MCP docs](https://www.jetbrains.com/help/ai-assistant/mcp.html)                        |
| Zed                        |   ✅   |      ✅      |     —      | [Zed MCP docs](https://zed.dev/docs/ai/mcp)                                                       |
| Kiro                       |   ✅   |      ✅      |     —      | [Kiro MCP docs](https://kiro.dev/docs/mcp/)                                                       |
| OpenCode                   |   ✅   |      ✅      |     —      | [OpenCode GitHub](https://github.com/anomalyco/opencode)                                          |
| Cline                      |   ✅   |      ✅      |     —      | [Cline MCP docs](https://docs.cline.bot/mcp-servers/overview)                                     |

> **Note**: "—" in the OAuth column means the client cannot present a **pre-registered client ID**. Those clients fall back to Dynamic Client Registration, which GitLab restricts to the `mcp` scope — a credential that cannot drive this server's REST-backed actions (see [OAuth App Setup — DCR and the `mcp` scope](oauth-app-setup.md#dynamic-client-registration-and-the-mcp-scope)). They are not locked out: HTTP OAuth mode also accepts a personal access token sent as `Authorization: Bearer <glpat-...>`, and HTTP legacy accepts `PRIVATE-TOKEN`.
>
> Verified as of 2026-08-27.

---

## Configuration Modes

| Mode            | Transport    | Token Management                                                                                 | Best For                                   |
| --------------- | ------------ | ------------------------------------------------------------------------------------------------ | ------------------------------------------ |
| **Stdio**       | stdin/stdout | `GITLAB_TOKEN` env var or `~/.gitlab-mcp-server.env`                                             | Single user, local development             |
| **HTTP Legacy** | HTTP         | `PRIVATE-TOKEN` header per-request                                                               | Multi-user, simple setup                   |
| **HTTP OAuth**  | HTTP         | Automatic OAuth 2.1 flow via [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) discovery | Multi-user, production, zero-config tokens |

> **Tip**: In HTTP modes, what a `GITLAB-URL` header does depends on how many instances the deployment publishes. A deployment publishes at least one or it refuses to start, unless it was launched with `--allow-any-gitlab-url`, in which case the header chooses the instance per request. With exactly one, that instance is authoritative and the header is ignored and logged — this is the public endpoint's case, fixed to `https://gitlab.com`. With several, they form an allow-list and the header is required: a request without it is refused with `400` naming the published instances, and a value outside the list is refused rather than ignored, `403` in OAuth mode and `400` in legacy mode.
> **Docker note**: The published Docker image infers its transport from stdin. An IDE that launches Docker as a stdio MCP process needs `docker run -i` and no transport flag; without `-i` the container gets `/dev/null` instead of a pipe and starts an HTTP listener. Do not publish port 8080 in that mode.

---

## Public Hosted Endpoint (mcp.jmrp.io)

Every example in this file configures a server you run yourself. To use the public instance at `https://mcp.jmrp.io/gitlab` instead, the shapes are identical with two differences: the URL is fixed, and the GitLab OAuth Application already exists — you do not create one, you point your client at its Application ID. The [server card](https://mcp.jmrp.io/servers/gitlab/) publishes that ID next to a copy button in each snippet; it is not repeated here, because this repository publishes artifacts only and does not operate that deployment.

Claude Code:

```bash
claude mcp add gitlab \
  --transport http \
  --client-id CLIENT_ID_FROM_THE_SERVER_CARD \
  --callback-port 8090 \
  https://mcp.jmrp.io/gitlab
```

VS Code (`.vscode/mcp.json`) — the same object with `servers` in place of `mcpServers`:

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "https://mcp.jmrp.io/gitlab",
      "oauth": {
        "clientId": "CLIENT_ID_FROM_THE_SERVER_CARD",
        "scopes": ["api"]
      }
    }
  }
}
```

Cursor (`.cursor/mcp.json`) spells the same thing differently — `auth.CLIENT_ID`, not `oauth.clientId`:

```json
{
  "mcpServers": {
    "gitlab": {
      "type": "http",
      "url": "https://mcp.jmrp.io/gitlab",
      "auth": {
        "CLIENT_ID": "CLIENT_ID_FROM_THE_SERVER_CARD",
        "scopes": ["api"]
      }
    }
  }
}
```

Use `"scopes": ["read_api"]` for a credential that cannot change anything: it is admitted and served a read-only tool surface, not refused.

Claude Desktop has no JSON path for remote OAuth servers — add `https://mcp.jmrp.io/gitlab` as a Custom Connector exactly as described under **Claude Desktop → HTTP OAuth Mode** below, using the card's Application ID under **Advanced settings**.

For a client with no OAuth flow, or for headless use, send the credential yourself; a gitlab.com personal access token is verified exactly like an OAuth one:

```bash
claude mcp add --transport http gitlab https://mcp.jmrp.io/gitlab \
  --header "Authorization: Bearer <your token>"
```

`PRIVATE-TOKEN` is the legacy-mode header and is not accepted there, and `GITLAB-URL` is ignored: the deployment fixes the instance to `https://gitlab.com`. Its full property table and caveats are in [HTTP Server Mode — Public Hosted Endpoint](http-server-mode.md#public-hosted-endpoint).

---

## VS Code / GitHub Copilot

### Stdio Mode

`GITLAB_URL` defaults to `https://gitlab.com`; add it to `env` only for self-managed GitLab instances.

Add to `.vscode/mcp.json`:

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "${input:gitlab-token}"
      }
    }
  },
  "inputs": [
    {
      "type": "promptString",
      "id": "gitlab-token",
      "description": "GitLab Personal Access Token",
      "password": true
    }
  ]
}
```

Docker stdio variant:

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e",
        "GITLAB_TOKEN",
        "-e",
        "GITLAB_MCP_SKIP_TLS_VERIFY",
        "ghcr.io/jmrplens/gitlab-mcp-server:latest"
      ],
      "env": {
        "GITLAB_TOKEN": "${input:gitlab-token}",
        "GITLAB_MCP_SKIP_TLS_VERIFY": "false"
      }
    }
  },
  "inputs": [
    {
      "type": "promptString",
      "id": "gitlab-token",
      "description": "GitLab Personal Access Token",
      "password": true
    }
  ]
}
```

### HTTP Legacy Mode

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

### HTTP OAuth Mode

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-server:8080/mcp",
      "oauth": {
        "clientId": "YOUR_GITLAB_APPLICATION_ID",
        "scopes": ["api"]
      }
    }
  }
}
```

- **`clientId`**: The Application ID from your GitLab OAuth Application (see [OAuth App Setup](oauth-app-setup.md))
- **`scopes`**: Must include `api` for full tool functionality (`read_api` for read-only)

VS Code discovers the GitLab authorization server automatically via `/.well-known/oauth-protected-resource` and initiates the OAuth 2.1 PKCE flow. The user authorizes in the browser and VS Code stores the token securely.

> **Important**: Without `clientId`, VS Code falls back to OAuth Dynamic Client Registration (DCR). GitLab's DCR assigns the `mcp` scope instead of `api`, which causes most server operations to fail. Always configure `clientId` explicitly.

---

## Claude Desktop

> **Easiest path**: install the one-click [.mcpb desktop extension](claude-desktop-extension.md)
> instead — no JSON editing, token stored in the OS keychain.

### Stdio Mode

Edit `claude_desktop_config.json`:

- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

### HTTP OAuth Mode

Claude Desktop supports remote MCP servers with OAuth via the **Custom Connectors** UI:

1. Go to [claude.ai/settings/connectors](https://claude.ai/settings/connectors)
2. Click **Add Connector** and enter the server URL: `https://mcp.example.com/mcp` — which must be the value the server was started with as `--public-url`, not merely the same origin. RFC 9728 makes a client discard metadata whose `resource` is not identical to the URL it used, so `--public-url=https://mcp.example.com` with clients on `.../mcp` fails discovery. See [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode)
3. Under **Advanced settings**, set the **Client ID** to your GitLab Application ID — without it Claude falls back to Dynamic Client Registration, which on GitLab yields an `mcp`-scoped token this server cannot use
4. Claude handles OAuth discovery and authorization through the browser

> **HTTPS required**: hosted Claude surfaces reach the server from Anthropic's infrastructure, so a plain `http://` or loopback URL cannot work. The callback to register in GitLab is `https://claude.ai/api/mcp/auth_callback`.
>
> **Note**: Claude Desktop does not support JSON-based OAuth configuration for remote MCP servers. Use the Custom Connectors UI for OAuth, or use stdio mode with a local binary.

---

## Claude Code (CLI)

### Stdio Mode

Write the token where the server reads it, then register the command:

```bash
echo 'GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx' > ~/.gitlab-mcp-server.env
chmod 600 ~/.gitlab-mcp-server.env

claude mcp add gitlab \
  --transport stdio \
  -- /path/to/gitlab-mcp-server
```

Add `GITLAB_URL=https://gitlab.example.com` to that same file only for self-managed instances. The registration command names no token, so nothing puts it in `argv` or in Claude Code's own configuration file.

### HTTP OAuth Mode

With pre-configured OAuth credentials (recommended):

```bash
claude mcp add gitlab \
  --transport http \
  --client-id YOUR_GITLAB_APPLICATION_ID \
  --callback-port 8090 \
  http://your-server:8080/mcp
```

Or via JSON configuration (`.mcp.json` or `~/.claude.json`):

```json
{
  "mcpServers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-server:8080/mcp",
      "oauth": {
        "clientId": "YOUR_GITLAB_APPLICATION_ID",
        "callbackPort": 8090,
        "scopes": "api"
      }
    }
  }
}
```

- **`clientId`**: The Application ID from your GitLab OAuth Application (see [OAuth App Setup](oauth-app-setup.md))
- **`callbackPort`**: Must match the redirect URI registered in the GitLab OAuth Application (`http://localhost:8090/callback`)
- **`scopes`**: Space-separated string (Claude Code format), must include `api`

Claude Code discovers the GitLab authorization server via `/.well-known/oauth-protected-resource`, opens the browser for authorization, and stores the token securely.

> **Without `--client-id`**: Claude Code falls back to Dynamic Client Registration (DCR). GitLab's DCR assigns the `mcp` scope instead of `api`, causing most operations to fail.

---

## Cursor

### Stdio Mode

Create or edit `.cursor/mcp.json` in your project root:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

### HTTP OAuth Mode

Cursor is a VS Code fork, but its static OAuth credentials do **not** use VS Code's `oauth.clientId` key — they go under `auth`, with an upper-case field name. Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-server:8080/mcp",
      "auth": {
        "CLIENT_ID": "YOUR_GITLAB_APPLICATION_ID",
        "scopes": ["api"]
      }
    }
  }
}
```

Set `scopes` explicitly. Cursor otherwise discovers them from the authorization server's own metadata, which for GitLab advertises every scope it supports rather than the one this server needs.

> **Note**: an `oauth: { clientId }` block — the VS Code spelling — is silently ignored by Cursor. It then falls back to Dynamic Client Registration, and GitLab's DCR mints an `mcp`-scoped token that this server's identity check rejects, so the failure looks like a bad credential rather than a misplaced key. Cursor does not support `${input:...}` variables either; OAuth support varies by version, so check the Cursor changelog for the one you have.

---

## Windsurf

### Stdio Mode

Edit `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "serverUrl": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

> **Tip**: Windsurf supports `${env:VAR_NAME}` interpolation in `serverUrl` and `headers`. Use `"PRIVATE-TOKEN": "${env:GITLAB_TOKEN}"` to avoid hardcoding secrets.

---

## JetBrains IDEs

### Stdio Mode

1. Open **Settings → Tools → AI Assistant → MCP Servers**
2. Click **+ Add** and select **stdio**
3. Set the command to `/path/to/gitlab-mcp-server`
4. Add `GITLAB_TOKEN` = `glpat-xxxxxxxxxxxxxxxxxxxx`
5. Add `GITLAB_URL` only for self-managed instances
6. Click **OK** and restart the IDE

### HTTP Legacy Mode

1. Open **Settings → Tools → AI Assistant → MCP Servers**
2. Click **+ Add** and select **HTTP**
3. Provide the JSON configuration shown below, set the server level, and click **OK** then **Apply**

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

> **Note**: JetBrains IDEs support Streamable HTTP and SSE transports but do not yet support the MCP OAuth 2.1 / RFC 9728 flow. Use `PRIVATE-TOKEN` header for authentication.

---

## Zed

### Stdio Mode

Edit your Zed `settings.json`:

```json
{
  "context_servers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "args": [],
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "context_servers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

---

## Kiro

### Stdio Mode

Create or edit `.kiro/settings/mcp.json` in your project root (or `~/.kiro/settings/mcp.json` for global config):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "args": [],
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

---

## OpenCode

### Stdio Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

---

## Cline

### Stdio Mode

Open the Cline sidebar in VS Code → click the MCP servers icon → **Edit Global MCP**, or edit the settings file directly:

- **macOS**: `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Linux**: `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Windows**: `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json`

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Legacy Mode

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

---

## Gemini CLI

Add the server to `~/.gemini/settings.json`. Either authentication path works:

```json
{
  "mcpServers": {
    "gitlab": {
      "httpUrl": "https://mcp.example.com/mcp",
      "oauth": {
        "enabled": true,
        "clientId": "YOUR_GITLAB_APPLICATION_ID",
        "redirectUri": "http://localhost:7777/oauth/callback"
      }
    }
  }
}
```

Register the exact `redirectUri` you pin in the GitLab application. For a token-based setup instead, drop the `oauth` block and use `"headers": { "Authorization": "Bearer glpat-..." }`.

## GitLab Duo Agent Platform

Duo's MCP client (VS Code, VSCodium, JetBrains via the GitLab Language Server) reads the shared `mcp.json`:

```json
{
  "mcpServers": {
    "gitlab-extended": {
      "url": "https://mcp.example.com/mcp",
      "headers": { "Authorization": "Bearer glpat-xxxxxxxxxxxx" }
    }
  }
}
```

Header authentication is the reliable path here; Duo does not drive a browser OAuth flow against a third-party authorization server. Pointing Duo at this server is not redundant with its built-in GitLab tools — it adds the full action catalog on top of them.

## OpenAI Codex

### Stdio Mode

Add the server to `~/.codex/config.toml`. The `default_tools_approval_mode = "approve"` line pre-approves tool calls; without it, Codex asks for approval on every tool whose annotations are not read-only (`gitlab_execute_action` and every mutating meta-tool), and non-interactive `codex exec` runs cancel those calls outright.

```toml
[mcp_servers.gitlab]
command = "/path/to/gitlab-mcp-server"
args = ["--transport", "stdio"]
startup_timeout_sec = 60
default_tools_approval_mode = "approve"

[mcp_servers.gitlab.env]
GITLAB_URL = "https://gitlab.example.com"
GITLAB_TOKEN = "glpat-xxxxxxxxxxxxxxxxxxxx"
```

### HTTP Mode (Streamable HTTP)

Codex reaches a remote server over Streamable HTTP with either a bearer token or a full OAuth flow:

```toml
# Bearer token — the simplest path, no redirect URI to register
[mcp_servers.gitlab]
url = "https://mcp.example.com/mcp"
bearer_token_env_var = "GITLAB_TOKEN"      # a glpat-... personal access token
default_tools_approval_mode = "approve"

# OAuth with a pre-registered GitLab application (Codex CLI 2026-05-22+)
[mcp_servers.gitlab.oauth]
client_id = "YOUR_GITLAB_APPLICATION_ID"
```

Without `client_id`, `codex mcp login` falls back to Dynamic Client Registration, which GitLab restricts to the `mcp` scope — see [OAuth App Setup](oauth-app-setup.md#dynamic-client-registration-and-the-mcp-scope).

Codex-specific notes:

- The server detects Codex from its `clientInfo` (`codex-mcp-client`) and automatically rounds the float `priority` in content annotations to 0 or 1 — the Codex builds bundled with ChatGPT.app reject non-integer priorities and mark every affected call as `Unexpected response type`. All other fields (audience, `structuredContent`, `outputSchema`, icons) are delivered unchanged. Set `GITLAB_MCP_CLIENT_COMPAT=off` to disable this. See [Client Compatibility](client-compatibility.md).
- When a result carries `structuredContent`, Codex forwards only that JSON to its model and discards the markdown content blocks ([openai/codex#10334](https://github.com/openai/codex/issues/10334)).
- Keep `GITLAB_MCP_META_PARAM_SCHEMA` at its `opaque` default: Codex silently strips descriptions from any tool input schema larger than ~5 KB.
- In its default protocol mode Codex reads only the first page of `tools/list`; the server sizes its page above the largest catalog so every surface fits in one page.

---

## See Also

### Internal

- [Getting Started](../getting-started.md) — install, configure a client, first call
- [OAuth App Setup](oauth-app-setup.md) — creating GitLab OAuth applications
- [Configuration](../reference/configuration.md) — environment variables and config loading order
- [HTTP Server Mode](http-server-mode.md) — HTTP transport architecture and deployment
- [HTTP Server Mode — Public Hosted Endpoint](http-server-mode.md#public-hosted-endpoint) — the public instance's properties and caveats

### External

- [RFC 9728: OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728) — the specification behind `--auth-mode=oauth`
- [MCP Specification: Authorization](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization) — MCP protocol authorization requirements
- [GitLab: Configure GitLab as an OAuth 2.0 provider](https://docs.gitlab.com/ee/integration/oauth_provider.html) — GitLab OAuth Application docs
