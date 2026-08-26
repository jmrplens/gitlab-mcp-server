# Security

Security considerations for gitlab-mcp-server deployment and development.

> **Diátaxis type**: Explanation
> **Audience**: 👤🔧 All users
> **Prerequisites**: Familiarity with GitLab PATs and TLS configuration
> 📖 **User documentation**: See the [Security](https://jmrp.io/docs/gitlab-mcp-server/operations/security/) on the documentation site for a user-friendly version.
>
> 🔧 **Looking for security *tools*?** This page explains the deployment security model. For the feature-flag, secure-file, error-tracking, and alerting tool schemas, see the [Security & Monitoring Tool Reference](../reference/tools/security.md).

---

## Authentication

gitlab-mcp-server authenticates to GitLab using a Personal Access Token (PAT) passed via the `GITLAB_TOKEN` environment variable. The token requires the `api` scope for full tool functionality.

### Token Security

- **Never commit tokens** — `.env` is gitignored; use environment variables in CI/production
- **Wizard-managed secrets** — The Setup Wizard stores `GITLAB_TOKEN`, `GITLAB_URL`, and `GITLAB_SKIP_TLS_VERIFY` in `~/.gitlab-mcp-server.env` with restricted permissions (`0600` on Unix). Client config files only contain non-secret preferences. The server loads this file automatically at startup as a fallback
- **Never hardcode tokens in JSON** — MCP client configuration files (`.vscode/mcp.json`, `.cursor/mcp.json`) are often committed to version control. Use [input variables](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_input-variables-for-sensitive-data) (`${input:gitlab-token}`), [environment files](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_standard-io-stdio-servers) (`envFile`), or system environment variables instead. See [Configuration — Secure Token Configuration](../reference/configuration.md#secure-token-configuration) for examples
- **Minimum scope** — Use `api` scope only; avoid `admin` scope unless required
- **Token rotation** — Rotate tokens regularly; use expiring tokens when possible. Re-run `gitlab-mcp-server --setup` and enter a new token — leaving the field blank keeps the previously stored value
- **Error output minimization** — Error Markdown includes diagnostic fields such as operation name, error class, HTTP status, request ID, and actionable hints. Tool input parameters are not copied into error output

### Setup Wizard Token Storage

The Setup Wizard (`gitlab-mcp-server --setup`) writes a single env file and a per-client JSON config. Both contain sensitive material that operators must protect.

#### `~/.gitlab-mcp-server.env`

- Created on the first successful run with mode `0600` (owner read/write only) on Unix and `0644` on Windows.
- Contains `GITLAB_TOKEN` plus non-secret launch preferences (`GITLAB_URL`, `GITLAB_SKIP_TLS_VERIFY`, `TOOL_SURFACE`, `CAPABILITY_SURFACE`, `META_PARAM_SCHEMA`, `GITLAB_TIER`, `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE`, `EMBEDDED_RESOURCES`, `EXCLUDE_TOOLS`, `GITLAB_IGNORE_SCOPES`, `UPLOAD_MAX_FILE_SIZE`, `AUTO_UPDATE`, `AUTO_UPDATE_REPO`, `AUTO_UPDATE_TIMEOUT`, `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`, `YOLO_MODE`, `LOG_LEVEL`).
- The server loads this file at startup as a fallback after the local `.env` and the shell environment.
- The Web UI's existing-token placeholder shows only the first 8 characters followed by `*` (see `wizard.MaskToken`). The full token never leaves the local browser session.

#### Per-client JSON files

Most MCP clients (VS Code, Claude Desktop, Claude Code, Cursor, Windsurf, OpenCode, Crush, Zed) reference the env file rather than embedding `GITLAB_TOKEN`. Their JSON only contains the binary path, optional `envFile` reference, and non-secret launch preferences.

**JetBrains IDEs are the documented exception**: the JetBrains AI Assistant cannot reference an external env file, so the wizard prints the full entry (including `GITLAB_TOKEN`, `GITLAB_URL`, and `GITLAB_SKIP_TLS_VERIFY`) as a JSON snippet to paste into *Settings > Tools > AI Assistant > MCP Servers*. If you choose this path:

- Paste the snippet into a machine-local IDE config (not a workspace-shared file).
- Treat the snippet as a secret — anyone with read access to the IDE config has the token.
- Rotate `GITLAB_TOKEN` immediately if the snippet is exposed (commit, backup, screen share, etc.).
- Prefer a client that supports `envFile` or system environment variables whenever possible.

### OAuth mode and audience binding (documented deviation)

The MCP auth specification says a resource server MUST validate that access
tokens were issued for it as the intended audience (RFC 8707 resource
indicators) and MUST NOT transit other tokens. This server cannot satisfy
that literally: GitLab's authorization server does not support resource
indicators, so every token it issues is a GitLab-audience token by design —
the server is a thin resource proxy that forwards the presented credential
to the one upstream it fronts (the same shape GitHub's remote MCP server
uses). What IS enforced in oauth mode: the token is verified against the
configured GitLab instance before any use, its **real granted scopes are
introspected** (`/personal_access_tokens/self` for PATs,
`/oauth/token/info` for OAuth tokens) rather than assumed, only the
standard `Authorization: Bearer` scheme is accepted (the legacy
`PRIVATE-TOKEN` alias is not rewritten in this mode), and the RFC 9728
protected-resource metadata is served from the identifier derived from
`--public-url`. Deployers should treat the token as what it is: a GitLab
credential entrusted to this proxy, scoped as narrowly as the workload
allows.

## Cross-Origin Protection

Every non-safe request (`POST`, `DELETE`) that a **browser** makes from another origin is rejected with `403` before authentication or MCP dispatch:

```text
HTTP/1.1 403 Forbidden
Content-Type: text/plain; charset=utf-8

cross-origin request detected from Sec-Fetch-Site header
```

Non-browser clients are unaffected: a request carrying neither `Origin` nor `Sec-Fetch-Site` — every CLI, IDE and SDK client — always passes. Safe methods (`GET`, `HEAD`) are exempt too, so the server card, `/health` and the OAuth metadata endpoint are readable cross-origin.

| Request                                                           | Result                             |
| ----------------------------------------------------------------- | ---------------------------------- |
| No `Origin` and no `Sec-Fetch-Site` (non-browser client)          | Allowed                            |
| `Sec-Fetch-Site: none` or `same-origin`                           | Allowed                            |
| `Sec-Fetch-Site: same-site` or `cross-site`                       | `403` unless the origin is trusted |
| `Origin` present, no `Sec-Fetch-Site`, origin equals `Host`       | Allowed                            |
| `Origin` present, no `Sec-Fetch-Site`, origin differs from `Host` | `403` unless the origin is trusted |

### Allowing specific origins

`--trusted-origins` takes a comma-separated list of absolute origins (`scheme://host[:port]`). A listed origin may make cross-origin browser requests; every other origin is still refused, so the DNS-rebinding requirement remains satisfied — an explicit allowlist **is** validation.

```bash
# Browser clients served from the site itself
gitlab-mcp-server --http --trusted-origins=https://mcp.example.com

# A local deployment reached by IP
gitlab-mcp-server --http --trusted-origins=http://192.168.1.50:8080

# Accept any origin (disables the protection) — only on a trusted network or
# behind a same-origin proxy that is the sole ingress
gitlab-mcp-server --http --trusted-origins='*'
```

Two behaviors are worth knowing:

- **`--public-url` seeds its own origin.** The flag already declares the externally reachable origin of the deployment, so that origin is trusted automatically. In OAuth mode `--public-url` is required, which means the origin RFC 9728 discovery points at is trusted without extra configuration.
- **A malformed origin fails startup.** A deployment that believes an origin is trusted when it is not is worse than one that refuses to start, so entries are validated before the server is built.

## TLS

- All GitLab API communication uses HTTPS by default
- Self-signed certificates: set `GITLAB_SKIP_TLS_VERIFY=true` (development only)
- Production deployments should use valid TLS certificates

## Input Validation

All tool handlers validate inputs before making API calls:

- **Required fields** — Checked before hitting the GitLab API
- **Schema lockdown** — All tool input schemas set `additionalProperties: false`, rejecting unexpected fields at the MCP SDK level before the handler runs
- **String sanitization** — `NormalizeText()` handles double-escaped sequences from MCP transport
- **Markdown escaping** — `EscapeMdTableCell()` and `EscapeMdHeading()` prevent injection in formatted output
- **File validation** — `OpenAndValidateFile()` checks file existence, type (regular files only), and size limits
- **Package names** — `ValidatePackageName()` and `ValidatePackageFileName()` enforce GitLab naming rules

## Destructive Action Protection

Operations that modify or delete data use a confirmation flow (see [Error Handling](error-handling.md)):

1. **YOLO_MODE / AUTOPILOT** — Environment variable bypass for automated pipelines
2. **Explicit confirm parameter** — `"confirm": true` in tool input
3. **MCP elicitation** — Interactive user confirmation when supported by the client
4. **Fail-safe** — If no confirmation mechanism is available, the operation is cancelled

## Read-Only and Safe Mode

Two opt-in modes narrow what the server can do to a GitLab instance, independently of the token's own permissions:

| Mode      | Variable                                | Effect                                                                                                                                       |
| --------- | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Read-only | `GITLAB_READ_ONLY=true` (`--read-only`) | Mutating operations are removed from the catalog. Attempting one fails as an unknown or unadvertised action                                  |
| Safe mode | `GITLAB_SAFE_MODE=true` (`--safe-mode`) | Mutating operations stay visible but return a JSON preview (`{"status":"blocked","mode":"safe","tool":"<action>",...}`) instead of executing |

Both policies are enforced **per action**, not per tool. This matters because two of the three tool surfaces expose dispatcher tools that cover many actions at once: a meta-tool such as `gitlab_issue` serves `list` and `create` alike, and the dynamic surface routes every action through `gitlab_execute_action`. Classifying those tools as mutating and removing or intercepting them whole would take their read operations down with them, leaving the server unable to read anything in either mode.

Instead the policies are applied to the action catalog that meta-tools and the dynamic surface are both built from:

- Read-only keeps the read-only actions inside each domain and drops the rest, so `gitlab_issue` survives with its read actions and `gitlab_execute_action` stays available (annotated read-only) for the reads it can still route.
- Safe mode replaces each mutating action's handler with one returning a preview naming that canonical action, so `issue.create` previews while `issue.list` executes.

The individual surface is unaffected by this distinction: one tool is one action there, so read-only removes mutating tools and safe mode wraps them directly.

When both are enabled, read-only takes precedence: mutating operations are absent rather than previewable.

## Transport Security

### stdio (Default)

Communication occurs over stdin/stdout within the local process. No network exposure.

### HTTP Mode

When running with `--http`:

- Binds to `localhost` by default — not exposed to the network
- No built-in authentication on the HTTP endpoint
- For production use, place behind a reverse proxy with proper TLS and auth
- **Cross-origin request protection** — HTTP mode applies middleware created with the Go standard library `net/http` function `http.NewCrossOriginProtection().Handler`. Browser-originated non-safe cross-site requests are rejected before MCP dispatch, while non-browser MCP clients without `Origin` or `Sec-Fetch-Site` headers continue to work. This satisfies the 2026-07-28 streamable-HTTP requirement that servers validate the `Origin` header against DNS rebinding. Use `--trusted-origins` to allow specific browser origins — see below
- **Host validation** — When listening on a specific local host, requests with unexpected `Host` headers are rejected to mitigate DNS rebinding attacks. Binding to all interfaces (`0.0.0.0` or `::`) leaves Host validation to the reverse proxy deployment
- **`GITLAB-URL` header validation** — In multi-instance mode, the server validates client-provided `GITLAB-URL` values and rejects malformed URLs with HTTP 400. When `--gitlab-url` is configured, it is authoritative and any client-provided `GITLAB-URL` value is ignored and logged
- **Rate limiting** — A per-IP authentication failure rate limiter (10 failures/min) protects against brute-force token guessing. When running behind a reverse proxy, configure `--trusted-proxy-header` (e.g. `CF-Connecting-IP`, `X-Real-IP`, `X-Forwarded-For`) so the rate limiter sees real client IPs. Only enable this flag when the server is reachable exclusively through a trusted proxy that overwrites or strips incoming copies of the header — otherwise clients can spoof it and bypass per-IP rate limiting. For multi-value headers like `X-Forwarded-For` the server uses the rightmost entry (the hop appended by the trusted proxy) to avoid trusting client-supplied values

### OAuth Mode (`--auth-mode=oauth`)

When running with `--auth-mode=oauth`, the server validates every request's Bearer token against the GitLab `/api/v4/user` endpoint before processing:

- **Token verification** — Each token is validated by calling GitLab's user API. Invalid or expired tokens receive HTTP 401
- **Identity caching** — Verified token identities are cached in-memory using SHA-256 hashed keys (raw tokens are never stored). Cache TTL is configurable via `--oauth-cache-ttl` (default 15m, range 1m–2h)
- **Bearer only** — only the standard `Authorization: Bearer` scheme is accepted; the legacy `PRIVATE-TOKEN` header is rejected with HTTP 401 in this mode (it remains accepted in legacy mode)
- **[RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) metadata** — The `/.well-known/oauth-protected-resource` endpoint advertises the GitLab authorization server URL, enabling compliant OAuth clients to discover the token issuer
- **PKCE** — The OAuth 2.1 flow uses Proof Key for Code Exchange (PKCE) to protect against authorization code interception attacks. MCP clients generate a code verifier/challenge pair for each authorization request
- **Cache eviction** — A background goroutine runs every 30 seconds to clean up expired entries. The cache is bounded by TTL, not by size

See [HTTP Server Mode — OAuth Mode](../guides/http-server-mode.md#oauth-mode) for the full architecture and flow diagram, and [OAuth App Setup](../guides/oauth-app-setup.md) for creating GitLab OAuth applications.

| Threat            | Mitigation                                                           |
| ----------------- | -------------------------------------------------------------------- |
| Token replay      | TTL-based expiration; tokens re-verified after cache expires         |
| Cache key leakage | SHA-256 hashing of raw tokens; original tokens never stored          |
| Brute force       | GitLab API rate limiting applies to verification requests            |
| Memory dump       | Only SHA-256 hashes and user metadata stored; no raw tokens in cache |

## PAT Scope-Based Tool Filtering

The server automatically detects the scopes of the Personal Access Token (PAT) at startup and removes tools that require scopes the token does not have. This follows the principle of least privilege — only tools the token can actually execute are exposed to the LLM.

- **Detection**: Uses the GitLab `GET /personal_access_tokens/self` endpoint
- **Graceful degradation**: If scope detection fails (e.g. older GitLab versions), all tools remain registered
- **Opt-out**: Set `GITLAB_IGNORE_SCOPES=true` or `--ignore-scopes` to skip detection
- **Scope map**: Defined in `internal/tools/scope_filter.go` (`MetaToolScopes`)

Tools requiring `admin_mode` (e.g. `gitlab_admin`, `gitlab_geo`, `gitlab_storage_move`) are filtered when the token lacks that scope.

## Prompt Injection Protection

MCP tool output contains user-generated content (UGC) from GitLab — issue descriptions, commit messages, wiki pages, MR notes, labels, etc. Malicious UGC could attempt to manipulate LLM behavior through prompt injection.

### Escaping Strategy

All Markdown formatters apply context-appropriate escaping to UGC fields:

| Context                  | Escape Function       | Purpose                                                   |
| ------------------------ | --------------------- | --------------------------------------------------------- |
| Table cells              | `EscapeMdTableCell()` | Prevents pipe characters from breaking table structure    |
| Headings                 | `EscapeMdHeading()`   | Prevents `#` injection that would break heading hierarchy |
| Multi-line body content  | `WrapGFMBody()`       | Wraps in blockquote (`>`) to contain structural Markdown  |
| List items (single-line) | `EscapeMdTableCell()` | Strips newlines and pipes from inline values              |

### UGC Boundary Markers

Explicit boundary markers (e.g., `<user_content>...</user_content>`) were evaluated and deemed unnecessary because:

1. **MCP protocol separation** — Tool results are delivered as structured JSON with `content` arrays, providing inherent boundary isolation between tool output and system/user prompts
2. **Escaping is sufficient** — The three escape functions above neutralize structural Markdown injection without needing delimiter tokens
3. **No cross-tool contamination** — Each tool result is a separate `CallToolResult` object; content cannot leak between tool calls

### Coverage

Escaping is applied to UGC fields across the 175 packages under `internal/tools`. Key field types:

- **Titles/names**: `EscapeMdTableCell()` in table contexts, `EscapeMdHeading()` in heading contexts
- **Descriptions/bodies**: `WrapGFMBody()` for multi-line GFM content
- **Author names**: `EscapeMdHeading()` when interpolated into headings, `EscapeMdTableCell()` in tables
- **Notes/comments**: `WrapGFMBody()` for standalone display, `EscapeMdTableCell()` in table summaries

## Error Information Disclosure

The error handling system is designed to be informative for LLMs while avoiding information leakage:

- **ClassifyError** returns semantic descriptions, not raw stack traces
- **DetailedError.Markdown** includes HTTP status and request ID for diagnostics
- Tool input parameters are not copied into standard error Markdown
- Internal implementation details are not exposed in error messages

## Dependencies

| Dependency                               | Security Notes                                                        |
| ---------------------------------------- | --------------------------------------------------------------------- |
| `gitlab.com/gitlab-org/api/client-go/v2` | Official GitLab client; uses `retryablehttp` with exponential backoff |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP SDK; handles JSON-RPC transport                          |
| `github.com/joho/godotenv`               | Loads `.env` files (CWD and `~/.gitlab-mcp-server.env` fallback)      |

Run `go list -m all` to see all transitive dependencies. Use `govulncheck` for vulnerability scanning:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Auto-Update Token Security

### Threat Model

The auto-update subsystem embeds a GitHub API token in the
compiled binary to check for and download new releases. Attack vectors:

| Vector                                 | Description                                     | Mitigation                                                   |
| -------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------ |
| Traffic capture (HTTP)                 | Intercept token on the wire                     | HTTPS enforcement in `NewUpdater()`                          |
| Proxy interception (`HTTPS_PROXY`)     | Token sent through attacker proxy               | `Proxy: nil` in HTTP transport                               |
| HTTP redirect to external host         | GitHub redirects to S3/CDN leaking token        | `CheckRedirect` strips `Authorization` on cross-host         |
| Protocol downgrade (HTTPS→HTTP)        | Redirect from HTTPS to HTTP exposes token       | `CheckRedirect` refuses HTTPS→HTTP redirects                 |
| Redirect chain abuse                   | Infinite redirects / open redirect exploitation | Max 10 redirects enforced                                    |
| Token to external hosts                | Asset URL points to non-GitHub host             | `sameHost()` check before attaching header                   |
| Memory dump (`gcore`, `/proc/PID/mem`) | Read token from process memory                  | Intermediate `[]byte` zeroed; globals zeroed after first use |
| Accidental logging (`%v`, panic)       | Token printed in logs or stack traces           | `Config.String()` / `GoString()` redact to `***`             |
| `GetConfig()` API                      | Token exposed via MCP tool                      | Returns copy with `Token: "***"`                             |

### Network Hardening

The `newGitHubSource` HTTP client (`internal/autoupdate/github_source.go`):

- `Proxy: nil` — disables system proxy
- `TLSClientConfig.MinVersion: tls.VersionTLS12`
- `InsecureSkipVerify` conditional on `SkipTLS` parameter
- `CheckRedirect`:
  - Strips `Authorization` on cross-host redirects
  - Refuses HTTPS→HTTP protocol downgrades
  - Limits redirect chain to 10 hops

### File Reference

| File                                   | Purpose                                           |
| -------------------------------------- | ------------------------------------------------- |
| `internal/autoupdate/github_source.go` | `newGitHubSource` with hardened HTTP client       |
| `internal/autoupdate/autoupdate.go`    | HTTPS enforcement, `Config.String()`/`GoString()` |
| `cmd/server/main.go`                   | Update initialization                             |

---

## Rate Limiting Model

The server ships an **optional** token-bucket rate limiter that gates `tools/call`
invocations. It is **disabled by default** because GitLab itself is the canonical
rate-limit authority — the limiter exists to protect operators against runaway
agents and noisy clients, not to replace upstream throttling.

### Configuration

| Setting         | Env var            | Flag (HTTP mode)     | Default        |
| --------------- | ------------------ | -------------------- | -------------- |
| Requests/second | `RATE_LIMIT_RPS`   | `--rate-limit-rps`   | `0` (disabled) |
| Burst capacity  | `RATE_LIMIT_BURST` | `--rate-limit-burst` | `40`           |

When `RATE_LIMIT_RPS = 0` the middleware is not attached and there is zero
overhead on the hot path. Setting any value `> 0` activates a `golang.org/x/time/rate`
limiter scoped to **one MCP server instance**:

- **stdio mode** — one process, one bucket → effectively per-user.
- **HTTP mode** — the server pool maintains one MCP server and server
  configuration snapshot per token+URL, so each authenticated client gets its
  own bucket. Multi-tenant deployments do not share quota across users.

### Recommended values

| Deployment                      | `--rate-limit-rps` | Rationale                                                                                      |
| ------------------------------- | ------------------ | ---------------------------------------------------------------------------------------------- |
| GitLab.com (authenticated user) | `20`               | Stays well under the published ~33 rps authenticated quota with headroom for pagination loops. |
| Self-hosted (default config)    | `8`                | Matches the typical 600 req/min default in `application_settings`.                             |
| CI / batch automation           | `2`–`4`            | Conservative; pipelines that invoke many tools per job.                                        |
| Disabled (default)              | `0`                | Trust GitLab's own throttle; useful when you have not measured traffic patterns yet.           |

### Behavior on excess

When the bucket is empty the middleware short-circuits the call and returns a
`CallToolResult` with `IsError: true` and a human-readable hint:

```text
Rate limit exceeded for `gitlab_list_merge_requests`. Wait a moment and retry, or raise --rate-limit-rps if this is sustained traffic.
```

The error is returned as a tool result (not a JSON-RPC error) so the LLM can
parse it and decide whether to back off, batch differently, or surface the
problem to the user. `tools/list`, `resources/*`, and `prompts/*` are **not**
gated.

### Defense-in-depth

The local limiter complements but does not replace:

- GitLab's per-user rate limiter (primary defense).
- HTTP-mode bounded server pool (`MAX_HTTP_CLIENTS`) which caps concurrency.
- Reverse-proxy/WAF policies in front of public deployments.

Disable it again by setting `RATE_LIMIT_RPS=0` (or omitting the flag). No state
is persisted between restarts.

---

## See Also

### Internal

- [HTTP Server Mode — OAuth Mode](../guides/http-server-mode.md#oauth-mode) — OAuth architecture and flow diagram
- [OAuth App Setup](../guides/oauth-app-setup.md) — creating GitLab OAuth applications
- [Troubleshooting — OAuth Mode](../guides/troubleshooting.md#oauth-mode---auth-modeoauth) — OAuth-specific troubleshooting

### External

- [RFC 9728: OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728) — the specification behind `--auth-mode=oauth`
- [OAuth 2.1 Authorization Framework (draft)](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-12) — mandates PKCE for all clients
- [GitLab: Configure GitLab as an OAuth 2.0 provider](https://docs.gitlab.com/ee/integration/oauth_provider.html) — GitLab OAuth Application docs
