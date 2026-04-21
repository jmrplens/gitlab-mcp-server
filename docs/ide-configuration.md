# IDE Configuration

Per-IDE MCP client configuration examples for gitlab-mcp-server, covering both stdio and HTTP modes (legacy and OAuth).

> **Diátaxis type**: Reference
> **Audience**: 👤 End users, AI assistant users
> **Prerequisites**: gitlab-mcp-server installed; for OAuth: a GitLab OAuth Application created (see [OAuth App Setup](oauth-app-setup.md))

---

## OAuth Support Matrix

| IDE / Client | Stdio | HTTP Legacy | HTTP OAuth | Official MCP Docs |
| --- | :---: | :---: | :---: | --- |
| VS Code (GitHub Copilot) | ✅ | ✅ | ✅ | [VS Code MCP docs](https://code.visualstudio.com/docs/copilot/chat/mcp-servers) |
| Claude Desktop | ✅ | ✅ | — | [Claude Desktop MCP docs](https://modelcontextprotocol.io/quickstart/user) |
| Claude Code | ✅ | ✅ | ✅ | [Claude Code MCP docs](https://docs.anthropic.com/en/docs/claude-code/mcp) |
| Cursor | ✅ | ✅ | ✅ | [Cursor MCP docs](https://docs.cursor.com/context/model-context-protocol) |
| Windsurf | ✅ | ✅ | — | [Windsurf MCP docs](https://docs.windsurf.com/windsurf/cascade/mcp) |
| JetBrains IDEs | ✅ | ✅ | — | [JetBrains MCP docs](https://www.jetbrains.com/help/ai-assistant/mcp.html) |
| Zed | ✅ | ✅ | — | [Zed MCP docs](https://zed.dev/docs/ai/mcp) |
| Kiro | ✅ | ✅ | — | [Kiro MCP docs](https://kiro.dev/docs/mcp/) |
| OpenCode | ✅ | ✅ | — | [OpenCode GitHub](https://github.com/anomalyco/opencode) |
| Cline | ✅ | ✅ | — | [Cline MCP docs](https://docs.cline.bot/mcp-servers/overview) |
| Roo Code | ✅ | ✅ | — | [Roo Code MCP docs](https://docs.roocode.com/features/mcp/using-mcp-in-roo) |

> **Note**: "—" indicates the client does not support `clientId` configuration for OAuth. These clients rely on Dynamic Client Registration (DCR), which GitLab assigns the `mcp` scope instead of `api`, making most server operations non-functional. Use stdio or HTTP legacy with `PRIVATE-TOKEN` header for those clients.

---

## Configuration Modes

| Mode | Transport | Token Management | Best For |
| --- | --- | --- | --- |
| **Stdio** | stdin/stdout | `GITLAB_TOKEN` env var or `.env` file | Single user, local development |
| **HTTP Legacy** | HTTP | `PRIVATE-TOKEN` header per-request | Multi-user, simple setup |
| **HTTP OAuth** | HTTP | Automatic OAuth 2.1 flow via [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) discovery | Multi-user, production, zero-config tokens |

---

## VS Code / GitHub Copilot

### Stdio Mode

Add to `.vscode/mcp.json`:

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
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
        "GITLAB_URL": "https://gitlab.example.com",
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
2. Click **Add Connector** and enter the server URL: `http://your-server:8080/mcp`
3. Claude handles OAuth discovery and authorization through the browser

> **Note**: Claude Desktop does not support JSON-based OAuth configuration for remote MCP servers. Use the Custom Connectors UI for OAuth, or use stdio mode with a local binary.

---

## Claude Code (CLI)

### Stdio Mode

```bash
claude mcp add gitlab \
  --transport stdio \
  -- /path/to/gitlab-mcp-server
```

Set environment variables before launching:

```bash
export GITLAB_URL="https://gitlab.example.com"
export GITLAB_TOKEN="glpat-xxxxxxxxxxxxxxxxxxxx"
```

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
        "GITLAB_URL": "https://gitlab.example.com",
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

Cursor is a VS Code fork and uses the same MCP configuration format. Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
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

> **Note**: Cursor does not currently support `${input:...}` variables. OAuth support may vary by Cursor version — verify in the Cursor changelog for your installed version.

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
        "GITLAB_URL": "https://gitlab.example.com",
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
4. Add environment variables:
   - `GITLAB_URL` = `https://gitlab.example.com`
   - `GITLAB_TOKEN` = `glpat-xxxxxxxxxxxxxxxxxxxx`
5. Click **OK** and restart the IDE

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
        "GITLAB_URL": "https://gitlab.example.com",
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
        "GITLAB_URL": "https://gitlab.example.com",
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
        "GITLAB_URL": "https://gitlab.example.com",
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
        "GITLAB_URL": "https://gitlab.example.com",
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

## Roo Code

### Stdio Mode

Open the Roo Code sidebar in VS Code → click the MCP servers icon → **Edit Global MCP** (for global config) or **Edit Project MCP** (creates `.roo/mcp.json`):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
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

## See Also

### Internal

- [Getting Started](getting-started.md) — first-time setup tutorial with Setup Wizard
- [OAuth App Setup](oauth-app-setup.md) — creating GitLab OAuth applications
- [Configuration](configuration.md) — environment variables and config loading order
- [HTTP Server Mode](http-server-mode.md) — HTTP transport architecture and deployment

### External

- [RFC 9728: OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728) — the specification behind `--auth-mode=oauth`
- [MCP Specification: Authorization](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization) — MCP protocol authorization requirements
- [GitLab: Configure GitLab as an OAuth 2.0 provider](https://docs.gitlab.com/ee/integration/oauth_provider.html) — GitLab OAuth Application docs
