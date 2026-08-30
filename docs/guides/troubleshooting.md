# Troubleshooting

Common issues and solutions for gitlab-mcp-server.

> **Diátaxis type**: How-to
> **Audience**: 👤 End users, AI assistant users
> **Prerequisites**: gitlab-mcp-server installed and configured
> 📖 **User documentation**: See the [Troubleshooting](https://jmrp.io/docs/gitlab-mcp-server/operations/troubleshooting/) on the documentation site for a user-friendly version.

---

## Connection and Authentication

| Symptom                                | Cause                       | Solution                                                                        |
| -------------------------------------- | --------------------------- | ------------------------------------------------------------------------------- |
| `GITLAB_TOKEN is required` at startup  | Token not set               | Set `GITLAB_TOKEN` in `.env` or environment                                     |
| `401 Unauthorized` from GitLab API     | Invalid or expired PAT      | Generate a new token with `api` scope in GitLab → User Settings → Access Tokens |
| `403 Forbidden` on specific operations | Token lacks required scope  | Ensure the token has `api` scope (not just `read_api`)                          |
| Connection refused or timeout          | GitLab instance unreachable | Verify `GITLAB_URL` is reachable: `curl -s $GITLAB_URL/api/v4/version`          |

## TLS and Certificates

| Symptom                                         | Cause                                      | Solution                                                                                                                                                                                                                                                                 |
| ----------------------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `x509: certificate signed by unknown authority` | Self-signed certificate on GitLab instance | Install the CA in the system trust store, or point `SSL_CERT_FILE` at a CA bundle. `GITLAB_SKIP_TLS_VERIFY=true` (or `--skip-tls-verify`) also works, but **`--auth-mode=oauth` refuses it** for a non-loopback instance: bearer tokens go to the instance on every call |
| `x509: certificate has expired`                 | Expired TLS certificate                    | Renew the certificate on the GitLab server. `GITLAB_SKIP_TLS_VERIFY=true` is a temporary workaround; `--auth-mode=oauth` accepts it only for a loopback instance                                                                                                         |

## Tool Discovery

| Symptom                                                                 | Cause                                            | Solution                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ----------------------------------------------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Only two tools appear: `gitlab_find_action` and `gitlab_execute_action` | Expected — `dynamic` is the default tool surface | Not a failure: those two reach the whole catalog. `gitlab_find_action` returns the matching canonical `domain.action` ID with its exact schema, `gitlab_execute_action` runs it. Set `TOOL_SURFACE=meta` for domain dispatchers or `TOOL_SURFACE=individual` for one tool per operation. See [Dynamic Toolset](../concepts/dynamic-tools.md)                                                                                               |
| MCP client shows hundreds of individual tools instead of 32             | Individual surface selected                      | Set `TOOL_SURFACE=meta` to use the domain meta-tool catalog instead of the individual one; the per-tier group counts are in [Dynamic Toolset](../concepts/dynamic-tools.md)                                                                                                                                                                                                                                                                |
| Tool not found in `tools/list`                                          | Tool not registered, or tool surface mismatch    | Check the active tool surface. Use `TOOL_SURFACE=individual` for one tool per operation, `TOOL_SURFACE=meta` for consolidated domain meta-tools, and `TOOL_SURFACE=dynamic` for the supported low-token find/execute surface. `META_TOOLS` remains accepted only as deprecated compatibility. See [Configuration](../reference/configuration.md#tool-modes) and [Dynamic Toolset](../concepts/dynamic-tools.md) for mode selection details |
| `unknown action` in meta-tool call                                      | Invalid `action` parameter                       | List valid actions by calling the meta-tool with `action: "list"` or check [Meta-Tools Reference](../concepts/meta-tools.md)                                                                                                                                                                                                                                                                                                               |
| `json: unknown field "<name>"` from a meta-tool                         | Misspelled or stale parameter name in `params`   | Meta-tools reject unknown keys (`DisallowUnknownFields`). Use the exact parameter names listed for the chosen `action` (e.g. `merge_request_iid`, `issue_iid`, `epic_iid`, `work_item_iid`, `snippet_id`) — see [Meta-Tools Reference](../concepts/meta-tools.md)                                                                                                                                                                          |

## Pagination

| Symptom                                | Cause                    | Solution                                                                    |
| -------------------------------------- | ------------------------ | --------------------------------------------------------------------------- |
| List results truncated (missing items) | Default `per_page` limit | Pass `per_page` (max 100) and `page` parameters to paginate through results |
| `nextPage` field missing in response   | You are on the last page | No more results available — this is expected behavior                       |

## HTTP Server Mode

| Symptom                                                                   | Cause                                                                                                                                                         | Solution                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `401 Unauthorized` with `WWW-Authenticate: Bearer`                        | Missing or empty token header                                                                                                                                 | Send `PRIVATE-TOKEN` or `Authorization: Bearer <token>` header. The JSON body names both accepted headers                                                                                                                                                                                                                                                           |
| `401 Unauthorized` saying GitLab rejected the token                       | GitLab answered `401`/`403` to the credential when the pooled session was first built                                                                         | Check the token is valid, unexpired, and issued by the target instance. It is verified once per new pooled session, not on every request                                                                                                                                                                                                                            |
| `400 Bad Request` with `no server available` in plain text                | A build older than 2.6.6 received a request with no token, an unparseable `GITLAB-URL` header, or a backend it could not reach                                | Not a crash and not emitted by this project — it comes from the MCP SDK (`go-sdk/mcp/streamable.go`) whenever the server-selection callback finds no server. Upgrade: those cases are now `401`, `400`, `429`, and `503` with a JSON-RPC body                                                                                                                       |
| `400 Bad Request` naming `GITLAB-URL`                                     | The per-request `GITLAB-URL` header is not a parseable URL                                                                                                    | Send an absolute URL such as `https://gitlab.example.com`, or omit the header when the server was started with `--gitlab-url`                                                                                                                                                                                                                                       |
| `403`/`400` saying the deployment does not serve that GitLab instance     | `--gitlab-url` names more than one instance (repeated, or comma-separated), so it publishes an allow-list and the `GITLAB-URL` header must select one of them | Send one of the instances the server publishes — the refusal names the allowed list, and the same list appears in the RFC 9728 `authorization_servers` field — or omit the header to get the first one, the deployment's default. In `--auth-mode=oauth` the mismatch is a `403` decided before the bearer token is verified anywhere; in legacy mode it is a `400` |
| `429 Too Many Requests` with `Retry-After`                                | More than 10 failed authentications from one client IP within a minute                                                                                        | Wait for the window to pass and retry with a valid token. Behind a reverse proxy, set `--trusted-proxy-header` so the limit counts real client IPs instead of the proxy's                                                                                                                                                                                           |
| `503 Service Unavailable`                                                 | The GitLab instance could not be reached while building the session for that token                                                                            | Check `GITLAB-URL`/`--gitlab-url` and instance reachability; the server logs the underlying error                                                                                                                                                                                                                                                                   |
| `403 Forbidden` on HTTP POST before MCP JSON-RPC handling                 | Browser sent a cross-site `Origin` or `Sec-Fetch-Site: cross-site` header                                                                                     | Allow the origin with `--trusted-origins=https://your-origin` (or `*` to accept any, disabling the protection). Non-browser clients are unaffected. See [Security — Cross-Origin Protection](../concepts/security.md#cross-origin-protection)                                                                                                                       |
| Browser client still fails after `--trusted-origins` was set              | On builds up to 2.7.4 the preflight `OPTIONS` was refused, so the browser never sent the real request                                                         | Upgrade: a preflight from a trusted origin is now answered `204` with the CORS headers. On an older build, put a reverse proxy in front that answers `OPTIONS` itself. See [Security — The browser preflight](../concepts/security.md#the-browser-preflight)                                                                                                        |
| Pool eviction too frequent                                                | Too many unique tokens                                                                                                                                        | Increase `--max-http-clients` (default: 100)                                                                                                                                                                                                                                                                                                                        |
| Sessions expiring unexpectedly                                            | MCP idle timeout too short                                                                                                                                    | Increase `--session-timeout` (default: 30m)                                                                                                                                                                                                                                                                                                                         |
| MCP sessions drop every ~2 min / `keepalive ping failed; closing session` | A low `--http-idle-timeout` (or a proxy timeout) is closing long-lived SSE streams                                                                            | Default `--http-idle-timeout=0` disables HTTP-layer idle closure; if you set a low value, raise it or use `0`. Behind a reverse proxy, also raise its read/idle timeout to at least a few minutes                                                                                                                                                                   |

See [HTTP Server Mode](http-server-mode.md) for architecture and configuration details.

## OAuth Mode (`--auth-mode=oauth`)

| Symptom                                                                      | Cause                                                                                 | Solution                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `401 Unauthorized` immediately on all requests                               | Token verification failed against GitLab API                                          | Verify the token is valid: `curl -H "Authorization: Bearer $TOKEN" $GITLAB_URL/api/v4/user`                                                                                                                                                                                                                                                                                                                                             |
| `401` after working for a while                                              | Token expired or revoked after cache entry                                            | Token will be re-verified on next request after cache TTL expires. If persistent, generate a new token                                                                                                                                                                                                                                                                                                                                  |
| High latency on first request                                                | OAuth cache miss — token verified against GitLab API                                  | Expected on cold start. Subsequent requests within `--oauth-cache-ttl` (default 15m) use cache                                                                                                                                                                                                                                                                                                                                          |
| Frequent re-verifications despite cache                                      | `--oauth-cache-ttl` set too low                                                       | Increase `--oauth-cache-ttl` (default 15m, max 2h). Check that the value was parsed correctly with `LOG_LEVEL=debug`                                                                                                                                                                                                                                                                                                                    |
| `/.well-known/oauth-protected-resource` returns 404                          | Server not running in OAuth mode                                                      | Start the server with `--auth-mode=oauth`. The metadata endpoint is only served in OAuth mode                                                                                                                                                                                                                                                                                                                                           |
| MCP client does not initiate OAuth flow                                      | Client does not support RFC 9728 discovery, or OAuth app not configured               | Configure a GitLab OAuth Application and set `clientId` in the MCP client config. See [OAuth App Setup](oauth-app-setup.md)                                                                                                                                                                                                                                                                                                             |
| Operations fail with insufficient `mcp` scope                                | DCR fallback assigned `mcp` scope instead of `api`                                    | Configure `clientId` explicitly in the MCP client config so the correct OAuth Application (with `api` scope) is used. See [OAuth App Setup](oauth-app-setup.md)                                                                                                                                                                                                                                                                         |
| `PRIVATE-TOKEN` header rejected in OAuth mode                                | By design — OAuth mode is Bearer-only                                                 | Send the token as `Authorization: Bearer <token>`. A personal access token works there and is verified like an OAuth token; `PRIVATE-TOKEN` is legacy-mode only                                                                                                                                                                                                                                                                         |
| `403 Forbidden` with `error="insufficient_scope"`                            | The token is genuine but carries no GitLab API scope at all                           | Reauthorize with the scope named in the challenge's `scope` parameter — `read_api`, the least any action needs. A `read_api` token is accepted everywhere and served the read-only tool surface; grant `api` for the full one                                                                                                                                                                                                           |
| Reads work but every create/update/delete action is missing from the catalog | The token carries `read_api`, not `api`, so the pool built a read-only surface for it | Working as designed (ADR-0018): admission asks for the minimum and write capability is decided per action against the token's real authority — nothing is misconfigured, `--read-only` was not set. Reauthorize with `api` for the full surface. The server logs `token cannot write; serving a read-only tool surface for it` with the detected scopes; the narrowing is per pool entry, so another client's `api` token is unaffected |
| `503 Service Unavailable` with `Retry-After`                                 | GitLab throttled or could not answer the verification request                         | The token has **not** been rejected — do not reauthorize. Retry after the advertised delay. Persistent `503` means the instance is unreachable from the server                                                                                                                                                                                                                                                                          |
| `429 Too Many Requests` after several bad tokens                             | Ten authentication failures from one address inside a minute                          | Wait out the window. Behind a reverse proxy, set `--trusted-proxy-header` so the budget counts real client IPs rather than the proxy's                                                                                                                                                                                                                                                                                                  |
| Same invalid token keeps returning `401` after it was fixed in GitLab        | The rejection is cached for five minutes to keep replays off GitLab                   | Wait for the entry to expire, or restart the server. Only definitive rejections are cached; throttling and outages never are, and a rejection is scoped to the instance that issued it                                                                                                                                                                                                                                                  |

See [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode) for the full OAuth architecture and flow diagram.

## npm / npx Installs

| Symptom                                                             | Cause                                                                                                                                                                                                                                                                                   | Solution                                                                                                                                                                                                                                              |
| ------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `the @jmrp.io/gitlab-mcp-server-linux-x64 package is not installed` | On Alpine this is deliberate: the linux packages declare `libc: glibc` and the released binaries need the glibc dynamic loader, so npm skips them on musl. On a glibc distro it means the install skipped optional dependencies (`--no-optional`, or a lockfile resolved on another OS) | On musl, use the container image `ghcr.io/jmrplens/gitlab-mcp-server` (built against musl) or build from source — the publish is not broken. On glibc, reinstall without `--no-optional`, or delete `node_modules` and the lockfile and install again |

## MCP Transport (Stdio)

| Symptom                                                                               | Cause                                                     | Solution                                                                                                                            |
| ------------------------------------------------------------------------------------- | --------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| No output from server                                                                 | MCP client not sending JSON-RPC to stdin                  | Verify the client is configured for stdio transport and sends `initialize` as the first message                                     |
| Garbled output or parse errors                                                        | Debug logs mixed with JSON-RPC on stdout                  | Ensure `LOG_LEVEL` is not `debug` in production; logs go to stderr, JSON-RPC to stdout                                              |
| Server exits immediately                                                              | Stdin closed prematurely                                  | The server exits when stdin is closed — ensure the MCP client keeps the pipe open                                                   |
| VS Code waits on `initialize` and Docker logs show `starting MCP server in HTTP mode` | Docker image HTTP default is running under a stdio client | Add `--http=false` after the image name in the Docker args, or change the client entry to HTTP mode with URL `http://host:8080/mcp` |

## IDE-Specific Issues

### VS Code / GitHub Copilot

| Symptom                                      | Cause                                         | Solution                                                                                                         |
| -------------------------------------------- | --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| "Tool not found" in Copilot Chat             | Server not started or MCP configuration error | Check the Output panel → **MCP Logs** for errors. Verify `.vscode/mcp.json` has the correct `command` path       |
| Server does not appear in MCP status         | Configuration not loaded                      | Run `Ctrl+Shift+P` → **MCP: List Servers** to verify. Check that the binary path is absolute and the file exists |
| "Permission denied" on startup (Linux/macOS) | Binary not executable                         | Run `chmod +x /path/to/gitlab-mcp-server`                                                                        |
| Token prompt does not appear                 | `${input:...}` misconfigured                  | Ensure the `inputs` array is at the top level of `mcp.json`, not inside `servers`                                |
| Server restarts repeatedly                   | Crash loop due to missing env vars            | Check Output panel → **MCP Logs** for `GITLAB_TOKEN is required` or other startup errors                         |

### Cursor

| Symptom                    | Cause                        | Solution                                                                       |
| -------------------------- | ---------------------------- | ------------------------------------------------------------------------------ |
| Tools not listed           | Configuration file not found | Verify `.cursor/mcp.json` exists and uses the `mcpServers` key (not `servers`) |
| `${input:...}` not working | Not supported by Cursor      | Use system environment variables or hardcode the token in the config file      |

### General IDE Tips

- **View server logs**: Most MCP clients show server output in a log panel. In VS Code: `Ctrl+Shift+P` → **MCP: List Servers** → click the server → **Show Output**
- **Restart the server**: After changing configuration, restart the MCP server from the IDE. In VS Code: `Ctrl+Shift+P` → **MCP: Restart Server**
- **Test connectivity**: If the server starts but tools fail, the GitLab URL or token may be wrong. Check the [Connection and Authentication](#connection-and-authentication) section above

## Resource Subscriptions

**Subscribed but no notifications arrive.** Check, in order: the server must
run `CAPABILITY_SURFACE=full` (the default — `minimal` does not advertise
`resources.subscribe`); the URI must be a single object, not a collection
(`gitlab://project/42/pipeline/99` works, `gitlab://project/42/issues` is
refused); and in HTTP mode with the default `--stateless=true`, the legacy
`resources/subscribe` is refused outright — only `subscriptions/listen`
(protocol 2026-07-28) works there. On protocol 2026-07-28 a refusal may
never reach the client (the Go SDK fires the listen request without awaiting
its response), so a subscription that "succeeded" but never fires is often
one that was refused.

**Notifications arrive slowly.** Two different slowdowns look alike from
outside. A settled resource — a finished pipeline, a closed issue — is
polled at 60 seconds by design, four times the 15-second base. Separately,
the watch may have lease-demoted: 30 minutes without any request on the
session drops it to a 10-minute poll.
Any tool call or resource read on that session restores full speed. The
notification's `_meta` (`io.github.jmrplens/watch`) reports the current
state and cadence.

**Watch stopped by itself.** A 401/403/404 on the resource, the 24-hour
lifetime cap, or eviction at the 10-watcher cap all stop a watch; on
protocol 2026-07-28 the open `subscriptions/listen` request completes when
that happens. Re-subscribe to start fresh. Full details:
[subscriptions reference](../reference/capabilities/subscriptions.md).

## Output Format

| Symptom                                           | Cause                                                       | Solution                                                                                                                                                                                                |
| ------------------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Links not clickable in IDE                        | Your IDE does not render Markdown links from tool responses | The `next_steps` hints are also available in the JSON `structuredContent`. Your AI assistant reads these and can present clickable links in its response                                                |
| Raw Markdown displayed alongside formatted output | Client shows both `content` and `structuredContent`         | Content is annotated `audience: ["assistant"]` — MCP clients that support annotations will hide the raw Markdown. Update your MCP client to the latest version                                          |
| No "Next steps" in response                       | Tool is used in individual or dynamic mode (not meta-tool)  | Next steps appear in meta-tool mode (`TOOL_SURFACE=meta`). Individual and dynamic tools include hints in Markdown content only                                                                          |
| Error message lacks corrective suggestion         | Not all errors have known corrective actions                | Errors with known fixes include a `💡 Suggestion` section. The server uses `WrapErrWithHint` / `WrapErrWithStatusHint` for status-specific guidance. See [Error Handling](../concepts/error-handling.md) |

See [Output Format](../reference/output-format.md) for the complete response format specification.

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
  -H "Accept: application/json, text/event-stream" \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# Test in OAuth mode
./gitlab-mcp-server --http --http-addr=localhost:8080 --gitlab-url=$GITLAB_URL --auth-mode=oauth --public-url=http://localhost:8080
curl -s http://localhost:8080/.well-known/oauth-protected-resource | jq .
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer $GITLAB_TOKEN" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

### Reading a 401 challenge

In OAuth mode an unauthenticated request is answered with an RFC 6750 challenge that tells a client where to go. Ask for the headers alone:

```bash
curl -si -X POST https://mcp.example.com/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}' | head -20
```

The `WWW-Authenticate: Bearer` header carries a `resource_metadata` URL. Fetch it, and `authorization_servers` names the GitLab instance to authorize against:

```bash
curl -s https://mcp.example.com/.well-known/oauth-protected-resource | jq .
```

Read the fields rather than hard-coding them: `resource` is the identifier this deployment is known by, `authorization_servers` is where to authorize, `scopes_supported` lists the scopes a client may authorize with (most capable first), and `resource_documentation` links the setup guide. A client that never fetches that document does not implement RFC 9728 discovery — send it a personal access token as `Authorization: Bearer glpat-…` instead; it is verified the same way. See [OAuth App Setup — The 401 challenge](oauth-app-setup.md#the-401-challenge) for what the `scope` hint in the challenge means.

The public instance at `https://mcp.jmrp.io/gitlab` answers exactly this way, so it is a working reference to compare a deployment against; its metadata lives at `https://mcp.jmrp.io/.well-known/oauth-protected-resource/gitlab`.

## See Also

- [Configuration](../reference/configuration.md) — environment variables and transport modes
- [Security](../concepts/security.md) — authentication, TLS, and input validation
- [Error Handling](../concepts/error-handling.md) — error types and classification logic
- [HTTP Server Mode](http-server-mode.md) — multi-user HTTP transport
