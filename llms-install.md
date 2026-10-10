# gitlab-mcp-server — AI agent installation guide

This guide is for AI agents (Cline, Claude, Cursor, etc.) installing the
GitLab MCP server on behalf of a user. Follow the steps in order. Every step
is deterministic — no build from source is required.

## What this server is

A single-binary MCP server (Go) exposing the GitLab REST API v4 and GraphQL
as MCP tools: ~868 tools on Free/CE, ~1022 on Premium, ~1088 on self-managed
Ultimate and ~1094 on GitLab.com Ultimate, with three selectable tool
surfaces, 45 MCP resources, and 37 prompts. It talks to GitLab.com or any
self-managed GitLab instance over stdio.

## Step 1 — Collect required information from the user

Ask the user for:

1. **GitLab Personal Access Token** (required) — a `glpat-...` token with
   `api` scope. The user can create one at
   `https://gitlab.com/-/user_settings/personal_access_tokens` (or the same
   path on their self-managed instance).
2. **GitLab URL** (optional) — only needed for self-managed instances.
   Defaults to `https://gitlab.com`.

Never echo the token back in plain text after receiving it.

## Step 2 — Choose an install method

### Method A: Docker (recommended — no download, always up to date)

Requires Docker installed and running. Verify with `docker --version`.
No other setup: the image is pulled automatically on first run.

MCP configuration (Cline `cline_mcp_settings.json`, Claude Desktop
`claude_desktop_config.json`, Cursor `mcp.json` — all use `mcpServers`):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "GITLAB_URL",
        "-e", "GITLAB_TOKEN",
        "ghcr.io/jmrplens/gitlab-mcp-server:latest"
      ],
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "<USER_TOKEN_HERE>"
      }
    }
  }
}
```

### Method B: Native binary (no Docker)

1. Download the binary for the user's platform from the latest release at
   `https://github.com/jmrplens/gitlab-mcp-server/releases/latest`.
   Asset names are exact:
   - `gitlab-mcp-server-linux-amd64`
   - `gitlab-mcp-server-linux-arm64`
   - `gitlab-mcp-server-darwin-amd64`
   - `gitlab-mcp-server-darwin-arm64`
   - `gitlab-mcp-server-windows-amd64.exe`
   - `gitlab-mcp-server-windows-arm64.exe`
2. Make it executable (`chmod +x`) on Linux/macOS and place it somewhere
   stable, e.g. `~/.local/bin/gitlab-mcp-server`.
3. Configure:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/absolute/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "<USER_TOKEN_HERE>"
      }
    }
  }
}
```

Run with no `GITLAB_TOKEN` in a terminal and the binary prints what it needs
and waits, which confirms the install without configuring anything.

### Method C: npm / npx (no Docker, no download step)

Use this when Docker is unavailable and you would rather not manage a binary.
The package carries prebuilt binaries for Linux, macOS and Windows on x64 and
arm64; npm installs only the one matching the user's platform and nothing runs
at install time.

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "npx",
      "args": ["-y", "@jmrp.io/gitlab-mcp-server"],
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "<USER_TOKEN_HERE>"
      }
    }
  }
}
```

Node.js 18 or newer is required. The server never replaces its own binary, so
updates come from `npm update -g @jmrp.io/gitlab-mcp-server`. The
Linux packages need glibc: on musl systems such as Alpine, use Method A.

## Step 3 — Optional environment variables

Add these to the `env` block (and, for Docker, a matching `-e NAME` in
`args`) only when the user asks for the behavior:

| Variable                 | Default   | Purpose                                                                                          |
| ------------------------ | --------- | ------------------------------------------------------------------------------------------------ |
| `GITLAB_MCP_TOOL_SURFACE`           | `dynamic` | Tool surface: `dynamic` (2 find/execute tools, lowest token use), `meta` (one tool per domain group: 34 on Free, up to 52 on GitLab.com Ultimate, fewer for a token without `admin_mode`; see Step 4), `individual` (one tool per action) |
| `GITLAB_MCP_TIER`            | detected  | Force `free`, `premium`, or `ultimate`, used as written. Unset, the tier is detected from the instance license, then from the plans of the namespaces the token administers, then on GitLab.com from the top-level groups the token is a member of, and falls back to `free` |
| `GITLAB_MCP_READ_ONLY`       | `false`   | Remove every mutating action; reads keep working                                                  |
| `GITLAB_MCP_SAFE_MODE`       | `false`   | Mutating actions answer with a preview card (the action and the arguments it would send) instead of executing |
| `GITLAB_MCP_SKIP_TLS_VERIFY` | `false`   | Allow self-signed certificates on self-managed instances                                          |
| `GITLAB_MCP_LOG_LEVEL`              | `info`    | `debug`, `info`, `warn`, `error`                                                                  |

Full reference: <https://jmrp.io/docs/gitlab-mcp-server/configuration/>

## Step 4 — Verify the installation

1. Restart or reload the MCP client so it picks up the new configuration.
2. The server should appear as connected, listing 2 tools on the default
   `dynamic` surface. On the `meta` surface it lists 34 tools on Free/CE, 40
   on Premium, 51 on self-managed Ultimate, 41 on GitLab.com Premium and 52
   on GitLab.com Ultimate. With a token that lacks the `admin_mode` scope,
   such as the one Step 1 asks for, it lists fewer: the server removes the
   `gitlab_admin` and `gitlab_storage_move` groups, and on Premium and above
   also `gitlab_enterprise_user`, `gitlab_geo` and `gitlab_project_alias`,
   which leaves 32, 35, 46, 36 and 47 in the same order. All of these
   counts are correct.
3. Smoke test: call `gitlab_find_action` with `query: "get current user"`,
   then `gitlab_execute_action` with the returned action id
   (`user.current`). On the `meta` surface call `gitlab_user` with
   `action: "current"` instead. A successful response returns the
   authenticated user's username.

## Troubleshooting

- **401 Unauthorized on the smoke test**: GitLab refused the token itself,
  because it is mistyped, expired or revoked, or belongs to an instance other
  than the one `GITLAB_URL` names. Ask the user for a new token. A 401 from only some actions,
  while the smoke test works, is GitLab refusing a permission on those
  routes rather than the token.
- **Every tool, resource and prompt request fails with JSON-RPC error
  `-40300`**: GitLab accepted the token, but it carries neither `read_api`
  nor `api`, the minimum this server needs. A token's scopes cannot be
  changed after it is created: ask the user for a new token with `api` (or
  `read_api` for reads only), then restart or reload the MCP client.
- **Reads work but most create, update and delete actions are missing**:
  the token carries `read_api` and not `api`, so the server serves it only
  the actions GitLab accepts from `read_api`. Ask for a token with `api` to
  write.
- **TLS errors on self-managed instances** — set
  `GITLAB_MCP_SKIP_TLS_VERIFY=true` (and `-e GITLAB_MCP_SKIP_TLS_VERIFY` in the
  Docker args).
- **Only two tools visible** — expected: the default `dynamic` surface
  exposes `gitlab_find_action` and `gitlab_execute_action`, which route to
  every action. Set `GITLAB_MCP_TOOL_SURFACE=meta` for visible per-domain tools.
- **Docker: `docker: command not found`** — fall back to Method C (npm/npx,
  needs only Node 18+) or Method B (native binary).
- Other symptoms and their fixes are under
  [Connection and authentication](https://jmrp.io/docs/gitlab-mcp-server/operations/troubleshooting/#connection-and-authentication)
  on the troubleshooting page.

## More documentation

- Project README: <https://github.com/jmrplens/gitlab-mcp-server>
- Docs site: <https://jmrp.io/docs/gitlab-mcp-server/>
- Tool reference: <https://jmrp.io/docs/gitlab-mcp-server/tools/overview/>
