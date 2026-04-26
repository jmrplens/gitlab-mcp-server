# Troubleshooting

Common issues and solutions for gitlab-mcp-server.

> **Diátaxis type**: How-to
> **Audience**: 👤 End users, AI assistant users
> **Prerequisites**: gitlab-mcp-server installed and configured
> 📖 **User documentation**: See the [Troubleshooting](https://jmrplens.github.io/gitlab-mcp-server/operations/troubleshooting/) on the documentation site for a user-friendly version.

---

## Connection and Authentication

| Symptom | Cause | Solution |
| --- | --- | --- |
| `GITLAB_TOKEN is required` at startup | Token not set | Set `GITLAB_TOKEN` in `.env` or environment |
| `GITLAB_URL is required` at startup | URL not set | Set `GITLAB_URL` in `.env` or use `--gitlab-url` flag. In HTTP mode, `--gitlab-url` is optional if clients send the `GITLAB-URL` header |
| `401 Unauthorized` from GitLab API | Invalid or expired PAT | Generate a new token with `api` scope in GitLab → User Settings → Access Tokens |
| `403 Forbidden` on specific operations | Token lacks required scope | Ensure the token has `api` scope (not just `read_api`) |
| Connection refused or timeout | GitLab instance unreachable | Verify `GITLAB_URL` is reachable: `curl -s $GITLAB_URL/api/v4/version` |

## TLS and Certificates

| Symptom | Cause | Solution |
| --- | --- | --- |
| `x509: certificate signed by unknown authority` | Self-signed certificate on GitLab instance | Set `GITLAB_SKIP_TLS_VERIFY=true` in `.env` (or `--skip-tls-verify` in HTTP mode) |
| `x509: certificate has expired` | Expired TLS certificate | Renew the certificate on the GitLab server, or use `GITLAB_SKIP_TLS_VERIFY=true` as a temporary workaround |

## Tool Discovery

| Symptom | Cause | Solution |
| --- | --- | --- |
| MCP client shows 1000 tools instead of 32 | Meta-tools disabled | Set `META_TOOLS=true` (default) to use 32 base meta-tools instead of 1000 individual tools |
| Tool not found in `tools/list` | Tool not registered, or meta-tools mode mismatch | Check if the tool exists in individual mode (`META_TOOLS=false`) or meta-tool mode (`META_TOOLS=true`) — they expose different tool names |
| `unknown action` in meta-tool call | Invalid `action` parameter | List valid actions by calling the meta-tool with `action: "list"` or check [Meta-Tools Reference](meta-tools.md) |

## Pagination

| Symptom | Cause | Solution |
| --- | --- | --- |
| List results truncated (missing items) | Default `per_page` limit | Pass `per_page` (max 100) and `page` parameters to paginate through results |
| `nextPage` field missing in response | You are on the last page | No more results available — this is expected behavior |

## Auto-Update

| Symptom | Cause | Solution |
| --- | --- | --- |
| `autoupdate: current version is required` | Binary built without version injection | Build with `make build` or add `-ldflags "-X main.version=1.2.0"` |
| `autoupdate: repository is required` | `AUTO_UPDATE_REPO` is empty | Set `AUTO_UPDATE_REPO` or use the default value |
| `autoupdate: creating GitHub source` | Network error reaching GitHub API | Verify network connectivity to `github.com` |
| `autoupdate: detecting latest release` | No releases in repository, or token lacks permissions | Create a release or check token permissions |
| Update detected but not applied | Mode is `check` only | Set `AUTO_UPDATE=true` to enable automatic application |
| Server still runs old version after update | Binary replaced but process not restarted | Restart the server process or use `gitlab-mcp-server --shutdown` to terminate all instances |
| Cannot replace binary (file locked) | Running instances hold the file | Run `gitlab-mcp-server --shutdown` to terminate all instances first |

See [Auto-Update](auto-update.md) for full details on update modes and configuration.

## HTTP Server Mode

| Symptom | Cause | Solution |
| --- | --- | --- |
| `400 Bad Request` | Missing or empty token header | Send `PRIVATE-TOKEN` or `Authorization: Bearer <token>` header |
| Pool eviction too frequent | Too many unique tokens | Increase `--max-http-clients` (default: 100) |
| Sessions expiring unexpectedly | Idle timeout too short | Increase `--session-timeout` (default: 30m) |

See [HTTP Server Mode](http-server-mode.md) for architecture and configuration details.

## OAuth Mode (`--auth-mode=oauth`)

| Symptom | Cause | Solution |
| --- | --- | --- |
| `401 Unauthorized` immediately on all requests | Token verification failed against GitLab API | Verify the token is valid: `curl -H "Authorization: Bearer $TOKEN" $GITLAB_URL/api/v4/user` |
| `401` after working for a while | Token expired or revoked after cache entry | Token will be re-verified on next request after cache TTL expires. If persistent, generate a new token |
| High latency on first request | OAuth cache miss — token verified against GitLab API | Expected on cold start. Subsequent requests within `--oauth-cache-ttl` (default 15m) use cache |
| Frequent re-verifications despite cache | `--oauth-cache-ttl` set too low | Increase `--oauth-cache-ttl` (default 15m, max 2h). Check that the value was parsed correctly with `LOG_LEVEL=debug` |
| `/.well-known/oauth-protected-resource` returns 404 | Server not running in OAuth mode | Start the server with `--auth-mode=oauth`. The metadata endpoint is only served in OAuth mode |
| MCP client does not initiate OAuth flow | Client does not support RFC 9728 discovery, or OAuth app not configured | Configure a GitLab OAuth Application and set `clientId` in the MCP client config. See [OAuth App Setup](oauth-app-setup.md) |
| Operations fail with insufficient `mcp` scope | DCR fallback assigned `mcp` scope instead of `api` | Configure `clientId` explicitly in the MCP client config so the correct OAuth Application (with `api` scope) is used. See [OAuth App Setup](oauth-app-setup.md) |
| `PRIVATE-TOKEN` header rejected | Not rejected — it is auto-converted to `Authorization: Bearer` | This is expected behavior. The `NormalizeAuthHeader` middleware handles the conversion transparently |

See [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode) for the full OAuth architecture and flow diagram.

## MCP Transport (Stdio)

| Symptom | Cause | Solution |
| --- | --- | --- |
| No output from server | MCP client not sending JSON-RPC to stdin | Verify the client is configured for stdio transport and sends `initialize` as the first message |
| Garbled output or parse errors | Debug logs mixed with JSON-RPC on stdout | Ensure `LOG_LEVEL` is not `debug` in production; logs go to stderr, JSON-RPC to stdout |
| Server exits immediately | Stdin closed prematurely | The server exits when stdin is closed — ensure the MCP client keeps the pipe open |

## IDE-Specific Issues

### VS Code / GitHub Copilot

| Symptom | Cause | Solution |
| --- | --- | --- |
| "Tool not found" in Copilot Chat | Server not started or MCP configuration error | Check the Output panel → **MCP Logs** for errors. Verify `.vscode/mcp.json` has the correct `command` path |
| Server does not appear in MCP status | Configuration not loaded | Run `Ctrl+Shift+P` → **MCP: List Servers** to verify. Check that the binary path is absolute and the file exists |
| "Permission denied" on startup (Linux/macOS) | Binary not executable | Run `chmod +x /path/to/gitlab-mcp-server` |
| Token prompt does not appear | `${input:...}` misconfigured | Ensure the `inputs` array is at the top level of `mcp.json`, not inside `servers` |
| Server restarts repeatedly | Crash loop due to missing env vars | Check Output panel → **MCP Logs** for `GITLAB_URL is required` or `GITLAB_TOKEN is required` |

### Cursor

| Symptom | Cause | Solution |
| --- | --- | --- |
| Tools not listed | Configuration file not found | Verify `.cursor/mcp.json` exists and uses the `mcpServers` key (not `servers`) |
| `${input:...}` not working | Not supported by Cursor | Use system environment variables or hardcode the token in the config file |

### General IDE Tips

- **View server logs**: Most MCP clients show server output in a log panel. In VS Code: `Ctrl+Shift+P` → **MCP: List Servers** → click the server → **Show Output**
- **Restart the server**: After changing configuration, restart the MCP server from the IDE. In VS Code: `Ctrl+Shift+P` → **MCP: Restart Server**
- **Test connectivity**: If the server starts but tools fail, the GitLab URL or token may be wrong. Check the [Connection and Authentication](#connection-and-authentication) section above

## Output Format

| Symptom | Cause | Solution |
| --- | --- | --- |
| Links not clickable in IDE | Your IDE does not render Markdown links from tool responses | The `next_steps` hints are also available in the JSON `structuredContent`. Your AI assistant reads these and can present clickable links in its response |
| Raw Markdown displayed alongside formatted output | Client shows both `content` and `structuredContent` | Content is annotated `audience: ["assistant"]` — MCP clients that support annotations will hide the raw Markdown. Update your MCP client to the latest version |
| No "Next steps" in response | Tool is used in individual mode (not meta-tool) | Next steps appear in meta-tool mode (`META_TOOLS=true`, default). Individual tools include hints in Markdown content only |
| Error message lacks corrective suggestion | Not all errors have known corrective actions | Errors with known fixes include a `💡 Suggestion` section. The server uses `WrapErrWithHint` / `WrapErrWithStatusHint` for status-specific guidance. See [Error Handling](error-handling.md) |

See [Output Format](output-format.md) for the complete response format specification.

## Diagnostic Commands

Verify your GitLab connection and token:

```bash
# Test GitLab API connectivity
curl -s --header "PRIVATE-TOKEN: $GITLAB_TOKEN" "$GITLAB_URL/api/v4/version"

# Run the server with debug logging
LOG_LEVEL=debug ./gitlab-mcp-server 2>debug.log

# Test in HTTP mode with curl (legacy)
./gitlab-mcp-server --http --http-addr=localhost:8080 --gitlab-url=$GITLAB_URL
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# Test in OAuth mode
./gitlab-mcp-server --http --http-addr=localhost:8080 --gitlab-url=$GITLAB_URL --auth-mode=oauth
curl -s http://localhost:8080/.well-known/oauth-protected-resource | jq .
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GITLAB_TOKEN" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

## See Also

- [Configuration](configuration.md) — environment variables and transport modes
- [Security](security.md) — authentication, TLS, and input validation
- [Error Handling](error-handling.md) — error types and classification logic
- [HTTP Server Mode](http-server-mode.md) — multi-user HTTP transport
- [Auto-Update](auto-update.md) — self-update mechanism
