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

- **Never commit tokens** — keep the token file out of version control; use environment variables in CI/production
- **Keep the token out of client JSON** — Put `GITLAB_TOKEN` in `~/.gitlab-mcp-server.env`, which the server loads at startup, and give the client config only non-secret launch preferences. Restrict the file to the owner (`chmod 600`)
- **Never hardcode tokens in JSON** — MCP client configuration files (`.vscode/mcp.json`, `.cursor/mcp.json`) are often committed to version control. Use [input variables](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_input-variables-for-sensitive-data) (`${input:gitlab-token}`), [environment files](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_standard-io-stdio-servers) (`envFile`), or system environment variables instead. See [Configuration — Secure Token Configuration](../reference/configuration.md#secure-token-configuration) for examples
- **Minimum scope** — Use `api` scope only; avoid `admin` scope unless required
- **Token rotation** — Rotate tokens regularly and use expiring tokens where possible. Replace the value wherever it is configured: the env file, the client JSON, or the environment
- **Error output minimization** — Error Markdown includes diagnostic fields such as operation name, error class, HTTP status, request ID, and actionable hints. Tool input parameters are not copied into error output

### Where the token lives

The server reads `GITLAB_TOKEN` from the environment, then from the file
`GITLAB_MCP_ENV_FILE` names, then from `~/.gitlab-mcp-server.env`, in that order
of precedence. A `.env` in the working directory is not one of the sources: that
directory comes from the client, so its contents were chosen by whoever wrote
the repository the client opened rather than by the operator. Whichever file you
use, it is a secret on disk and should be owner-readable only (`chmod 600` on
Unix).

Most MCP clients (VS Code, Claude Desktop, Claude Code, Cursor, Windsurf,
OpenCode, Crush, Zed) can reference an env file rather than embedding the token
in their JSON, and that is the arrangement to prefer: those JSON files are
frequently committed.

**JetBrains IDEs are the documented exception**: the JetBrains AI Assistant
cannot reference an external env file, so its entry carries `GITLAB_TOKEN`
inline. If you use it:

- Put the snippet in a machine-local IDE config, never a workspace-shared file.
- Treat that file as a secret: anyone who can read it has the token.
- Rotate `GITLAB_TOKEN` immediately if it is exposed (a commit, a backup, a screen share).
- Prefer a client that supports `envFile` or system environment variables.

### OAuth mode and audience binding (documented deviation)

The MCP auth specification says a resource server MUST validate that access
tokens were issued for it as the intended audience (RFC 8707 resource
indicators) and MUST NOT transit other tokens. This server cannot satisfy
that literally: GitLab's authorization server does not support resource
indicators, so every token it issues is a GitLab-audience token by design.
The server is a thin resource proxy that forwards the presented credential
to the one upstream it fronts (the same shape GitHub's remote MCP server
uses), which the 2026-07-28 specification prohibits without condition. The
deviation is accepted rather than argued away; the reasoning, and why it does
not generalize, is
[ADR-0019](../development/adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md). What IS enforced in oauth mode: the token is verified against the
configured GitLab instance before any use, its **real granted scopes are
introspected** (`/personal_access_tokens/self` for PATs,
`/oauth/token/info` for OAuth tokens) rather than assumed, only the
standard `Authorization: Bearer` scheme is accepted (the legacy
`PRIVATE-TOKEN` alias is not rewritten in this mode), and the RFC 9728
protected-resource metadata is served from the identifier derived from
`--public-url`. Deployers should treat the token as what it is: a GitLab
credential entrusted to this proxy, scoped as narrowly as the workload
allows.

The specification's alternative to RFC 8707 — "or otherwise verify that they
are the intended recipient of the token" — is available opt-in.
`--oauth-client-uid` (env `GITLAB_MCP_OAUTH_CLIENT_UID`, comma-separated) pins the GitLab
OAuth applications whose tokens the deployment admits, compared against
`application.uid` from `/oauth/token/info`. It is off by default because
turning it on refuses personal access tokens outright: a PAT belongs to no
application, and it is the credential every non-browser client uses. When it is
on, an absent, unreadable or unmatched uid is a refusal, and so is an
introspection that never answered — the surrounding fail-open behaviour is
deliberately inverted there, or breaking introspection would be the way around
the pin. The full reasoning, with the live evidence that GitLab publishes no
`resource_indicators_supported`, is
[ADR-0019](../development/adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md).

## Cross-Origin Protection

Every non-safe request (`POST`, `DELETE`) that a **browser** makes from another origin is rejected with `403` before authentication or MCP dispatch:

```text
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"jsonrpc":"2.0","id":null,"error":{"code":-40300,
 "message":"Cross-origin request refused: the Origin header names an origin this deployment does not trust."}}
```

The body is a JSON-RPC error rather than plain text on purpose: the Streamable
HTTP specification tells a client that receives a `4xx` whose body is not a
recognized JSON-RPC error to conclude the server predates version negotiation,
so an opaque refusal would report a policy decision as a protocol generation.
Host validation answers the same way.

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

### The browser preflight

Allowing an origin past the protection is only half of what a browser needs. Before it sends a cross-origin `POST` carrying `Authorization` or `Content-Type: application/json`, it sends a preflight `OPTIONS` — and that request carries no credentials by definition. In OAuth mode the bearer layer answered it `401`, so the browser never sent the real request and `--trusted-origins` appeared to do nothing unless a reverse proxy in front answered the preflight instead.

A preflight from a trusted origin is now answered directly:

```http
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: https://claude.ai
Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type, Accept, Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-ID, Mcp-Method, Mcp-Name, Mcp-Param-Name, Mcp-Param-Uri, Mcp-Param-Cursor
Access-Control-Max-Age: 86400
Vary: Origin
```

The allowed headers follow the deployment rather than being a constant. `Mcp-Method`, `Mcp-Name` and the `Mcp-Param-*` family are **required** by protocol 2026-07-28 — a `POST` without `Mcp-Method` is rejected before any handler runs — so a preflight that omitted them refused the very headers the server then demanded. `PRIVATE-TOKEN` is added only in legacy mode, which is the only mode that reads it, and `GITLAB-URL` only when the header can actually change which instance a request reaches: never when exactly one instance is pinned, always when several are published.

The actual response then carries `Access-Control-Allow-Origin` and `Access-Control-Expose-Headers: Mcp-Session-Id, Mcp-Protocol-Version, WWW-Authenticate, Retry-After` — a browser cannot read any of them otherwise, since none is CORS-safelisted. `WWW-Authenticate` is what makes automatic discovery work at all: without it a cross-origin client receives the `401` but cannot see the `resource_metadata` URL it points at. `Retry-After` is what lets it honor the backoff a `429` or `503` asks for.

An untrusted origin's preflight is passed down rather than answered here, because some routes serve their own — the RFC 9728 metadata document and the server card are public and answer any origin. What it is never charged with is a failed authentication: a preflight carries no credential by definition, so counting one would let ten routine browser questions lock that address out of the endpoint.

### Do not let a reverse proxy answer CORS as well

A proxy in front that advertises CORS on the server's behalf — the shape most deployments started with, because the server could not answer a preflight — now collides with the server's own answer:

```http
HTTP/1.1 200 OK
Access-Control-Allow-Origin: https://claude.ai
Access-Control-Allow-Origin: *
```

`curl` reports `200` and the headers look generous. A browser refuses the response outright, because [Fetch](https://fetch.spec.whatwg.org/) treats more than one `Access-Control-Allow-Origin` as a failure rather than merging them. Chromium says so plainly:

```text
The 'Access-Control-Allow-Origin' header contains multiple values
'https://claude.ai, *', but only one is allowed.
```

So a deployment that keeps its proxy-level CORS after upgrading is **worse off than before**: previously the proxy's lone `*` at least worked for requests without credentials. Remove the `add_header Access-Control-*` block and the `OPTIONS` short-circuit from the MCP location, and let `--trusted-origins` decide. Both changes belong in the same deploy — between them the endpoint is broken for browsers in a way no `curl` check reveals.

`test/e2e/http` pins this: `TestProxy_ServerAndProxyCORSCollide` runs a real nginx with that block and reports the collision.

Two properties are deliberate:

- **The origin is echoed, never `*`.** These requests carry an `Authorization` header, and a browser rejects the wildcard on a credentialed request. `--trusted-origins='*'` echoes whatever origin asked rather than emitting a literal asterisk.
- **An untrusted origin gets no header.** It is passed straight to the protection above, which refuses it. Nothing here widens the trust decision; it only makes an already-granted one usable.

## TLS

- All GitLab API communication uses HTTPS by default
- A self-signed certificate **on the GitLab side**: install the CA in the system trust store, or set `SSL_CERT_FILE` to a CA bundle. `GITLAB_SKIP_TLS_VERIFY=true` is the blunt alternative, for development only, and `--auth-mode=oauth` refuses it outright for a non-loopback instance
- **Serving HTTPS**: in HTTP mode, `--tls-cert` and `--tls-key` terminate TLS on the listener itself. It is both or neither — a certificate without its key is a deployment that thinks it is encrypting and is not, so the server refuses to start. The pair is loaded at startup too, which turns a wrong path into a startup error naming the file instead of a handshake failure nobody sees until a client reports it. TLS 1.2 is the floor, and 1.3 is used with any client that offers it
- **Or remove the hop instead of encrypting it**: for a reverse proxy on the same machine, `--http-addr=/run/gitlab-mcp.sock` binds a unix socket rather than a TCP port, with `--http-socket-mode` (default `0660`, owner and group only) deciding who may connect. There is no network segment left to read and no certificate to issue or rotate
- Production deployments should use valid TLS certificates, whether this server presents them or a proxy in front of it does

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

- **Binds every interface by default** — `--http-addr` defaults to `:8080`, which is all interfaces, not loopback. Pass `--http-addr=localhost:8080` to keep the listener host-local, or a filesystem path (`--http-addr=/run/gitlab-mcp.sock`, permissions from `--http-socket-mode`, default `0660`) to remove the network hop to a same-machine proxy instead of encrypting it
- **Authentication is per request, not per deployment** — no credential is configured at startup. In legacy mode each client sends its own GitLab token (`PRIVATE-TOKEN` or `Authorization: Bearer`) and the server's only check is that GitLab accepts it; with `--auth-mode=oauth` every Bearer token is verified against GitLab before the request reaches the MCP handler
- **TLS can terminate here** — `--tls-cert` and `--tls-key` serve HTTPS on the listener itself, so a reverse proxy is a deployment choice rather than the only way to encrypt. A proxy remains useful for hostname routing, shared certificates and edge caching
- **Cross-origin request protection** — two layers, because the routes have different jobs. Every route is wrapped in middleware from the Go standard library's `http.NewCrossOriginProtection().Handler`, which rejects browser-originated cross-site requests on non-safe methods while leaving `GET`, `HEAD` and `OPTIONS` alone — that exemption is what keeps `/health` and the server card publicly fetchable. The MCP endpoint itself is additionally wrapped in a guard that refuses an untrusted `Origin` on **every** method with `403`, since the safe-method exemption would otherwise let a cross-origin `GET` open an SSE stream against a stateful session. Together these satisfy the 2026-07-28 streamable-HTTP requirement that servers validate `Origin` on all incoming connections against DNS rebinding. Non-browser clients, which send no `Origin`, are unaffected; a CORS preflight is answered rather than refused, because a browser strips credentials from it. Use `--trusted-origins` to allow specific browser origins — see below
- **Host validation** — When listening on a specific local host, requests with unexpected `Host` headers are rejected to mitigate DNS rebinding attacks. Binding to all interfaces (`0.0.0.0` or `::`) leaves Host validation to the reverse proxy deployment
- **`GITLAB-URL` header validation** — What the header may do follows how many instances the deployment published. With none, it selects the instance per request and a malformed value is rejected with HTTP 400. With exactly one (`--gitlab-url` passed once), that instance is authoritative and the header is ignored and logged. With several (`--gitlab-url` repeated, or one comma-separated value), the header selects **among them**, and a value naming anything else is refused — `403` in OAuth mode, `400` in legacy mode — rather than silently served the first instance. The allow-list is what makes a per-request instance safe under OAuth: the bearer token is verified against the instance the request selected, so a free-form header would let a caller name a host of their own and be handed a live credential
- **Rate limiting** — A per-IP authentication failure rate limiter (10 failures/min) protects against brute-force token guessing. When running behind a reverse proxy, configure `--trusted-proxy-header` (e.g. `CF-Connecting-IP`, `X-Real-IP`, `X-Forwarded-For`) together with `--trusted-proxies`, the addresses or CIDR ranges the proxy connects from, so the rate limiter sees real client IPs. The header is believed only on a connection from a listed address; from any other peer it is ignored and the peer itself is charged, so a client that reaches the listener directly cannot choose the address its failures count against. One flag without the other refuses startup. For multi-value headers like `X-Forwarded-For` the server reads from the right, skipping hops that are themselves listed, and charges the first that is not; a hop that is not an address charges the peer

### OAuth Mode (`--auth-mode=oauth`)

When running with `--auth-mode=oauth`, the server validates every request's Bearer token against the GitLab `/api/v4/user` endpoint before processing:

- **Token verification** — Each token is validated by calling GitLab's user API. Invalid or expired tokens receive HTTP 401
- **Identity caching** — Verified token identities are cached in-memory using SHA-256 hashed keys (raw tokens are never stored). Cache TTL is configurable via `--oauth-cache-ttl` (default 15m, range 1m–2h)
- **Bearer only** — only the standard `Authorization: Bearer` scheme is accepted; the legacy `PRIVATE-TOKEN` header is rejected with HTTP 401 in this mode (it remains accepted in legacy mode)
- **[RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) metadata** — The `/.well-known/oauth-protected-resource` endpoint advertises the GitLab authorization server URL, enabling compliant OAuth clients to discover the token issuer
- **PKCE** — The OAuth 2.1 flow uses Proof Key for Code Exchange (PKCE) to protect against authorization code interception attacks. MCP clients generate a code verifier/challenge pair for each authorization request
- **Cache eviction** — Entries are evicted lazily when read after expiry, and a background sweep removes the ones nothing reads again. The sweep runs at a quarter of `--oauth-cache-ttl`, with a 30-second floor, so the cache is bounded by time rather than by how many distinct credentials have arrived
- **Least-privilege scope** — admission asks only for `read_api`, the least any action needs, and the write check is applied per action instead. A `read_api` token is therefore accepted by a deployment that writes, and is served the read-only tool surface; the only credential refused at the door is one carrying no GitLab API scope at all. The metadata's `scopes_supported` advertises both `api` and `read_api` so a client can deliberately ask for a credential that cannot mutate anything, and a read-only deployment (`--read-only` or `--safe-mode`) advertises `read_api` alone. An `api` token satisfies the minimum everywhere, since `api` is a superset

### Rejections are cheap, and they are bounded

An unauthenticated request is something anyone can generate. Relaying each one to GitLab to ask whether the token is valid turns a public deployment into an amplifier: attacker traffic becomes load on someone else's API, and on gitlab.com it becomes rate-limit pressure charged to the server's own address, where it lands on the legitimate users sharing it.

Three layers sit in front of that, in this order:

1. **Per-address failure budget** — ten authentication failures in one minute block the address for the rest of the window, answered `429` with `Retry-After`. Checked first, so a blocked caller costs nothing at all. Failures at this layer and at the pool share one budget, so exhausting one does not earn a fresh allowance at the other.
2. **Rejected-token cache** — a token GitLab has already refused is refused from memory for five minutes without asking again. Keyed by SHA-256 digest, never the raw credential, and bounded to 4096 entries because the keys come from the caller.
3. **Verified-identity cache** — the existing positive cache, so an accepted token is verified once per TTL rather than once per request.

Neither cache records an upstream failure. A timeout, a `5xx`, or a `429` from GitLab says nothing about the credential, so caching one would lock out a valid token for the whole TTL over a transient outage.

### What a rejection tells the client

The [RFC 6750](https://datatracker.ietf.org/doc/html/rfc6750) error code in `WWW-Authenticate` is the difference between a client reauthorizing, asking for more scope, or simply retrying:

| Condition                       | Status | Challenge                                            |
| ------------------------------- | ------ | ---------------------------------------------------- |
| No credential at all            | `401`  | no `error` code (RFC 6750 §3.1), `resource_metadata` |
| GitLab rejected the token       | `401`  | `error="invalid_token"` with a description           |
| Token lacks the required scope  | `403`  | `error="insufficient_scope"`, `scope="<required>"`   |
| Address over the failure budget | `429`  | none; `Retry-After`                                  |
| GitLab throttled or unreachable | `503`  | **none**; `Retry-After`                              |

The last row matters more than it looks. Reporting a throttled GitLab as `invalid_token` makes a well-behaved MCP client discard a good credential and start a fresh authorization flow — generating more upstream traffic at exactly the moment the instance asked for less, and asking the user to re-approve an application that was never the problem. A `503` carries no challenge, propagates GitLab's own `Retry-After` when it sent one, and says plainly that the token has not been rejected.

See [HTTP Server Mode — OAuth Mode](../guides/http-server-mode.md#oauth-mode) for the full architecture and flow diagram, and [OAuth App Setup](../guides/oauth-app-setup.md) for creating GitLab OAuth applications.

| Threat                 | Mitigation                                                                                                                                      |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Token replay           | TTL-based expiration; tokens re-verified after cache expires                                                                                    |
| Cache key leakage      | SHA-256 hashing of raw tokens; original tokens never stored                                                                                     |
| Brute force            | Per-address failure budget (10/minute) plus a rejected-token cache, so repeated attempts never reach GitLab                                     |
| Upstream amplification | A repeated token is answered from memory, and the failure budget caps the rest: at most ten distinct-token verifications per address per window |
| Memory dump            | Only SHA-256 hashes and user metadata stored; no raw tokens in cache                                                                            |

## PAT Scope-Based Tool Filtering

The server automatically detects the scopes of the Personal Access Token (PAT) at startup and removes tools that require scopes the token does not have. This follows the principle of least privilege — only tools the token can actually execute are exposed to the LLM.

- **Detection**: Uses the GitLab `GET /personal_access_tokens/self` endpoint
- **Graceful degradation**: If scope detection fails (e.g. older GitLab versions), all tools remain registered
- **Opt-out**: Set `GITLAB_IGNORE_SCOPES=true` or `--ignore-scopes` to skip detection
- **Scope map**: Defined in `internal/tools/scope_filter.go` (`MetaToolScopes`)
- **HTTP mode narrows further, in both auth modes**: a pool entry whose token carries no write scope is served the read-only catalog, exactly as if `--read-only` had been set for that client. The narrowing is per pool entry, and an entry is per token, so one client's `read_api` token cannot narrow another client's `api` token. Unknown scopes (detection failed, or `--ignore-scopes`) count as write-capable — a wrong "no" would silently remove tools, while a wrong "yes" simply surfaces as GitLab's own 403 on the call that tried to write

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

| Dependency                               | Security Notes                                                            |
| ---------------------------------------- | ------------------------------------------------------------------------- |
| `gitlab.com/gitlab-org/api/client-go/v2` | Official GitLab client; uses `retryablehttp` with exponential backoff     |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP SDK; handles JSON-RPC transport                              |
| `github.com/joho/godotenv`               | Loads dotenv files (`GITLAB_MCP_ENV_FILE` and `~/.gitlab-mcp-server.env`) |

Run `go list -m all` to see all transitive dependencies. Use `govulncheck` for vulnerability scanning:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Rate Limiting Model

The server ships a token-bucket rate limiter that gates `tools/call`
invocations. It exists to protect operators against runaway agents and noisy
clients, not to replace upstream throttling: GitLab itself remains the canonical
rate-limit authority.

Whether it is on out of the box depends on the transport. **HTTP mode enables it
by default** (`--rate-limit-rps=10`), because that deployment is shared — every
call it forwards is charged to its own egress address, so one looping client's
volume lands on every other tenant. **Stdio leaves it off** (`GITLAB_MCP_RATE_LIMIT_RPS=0`):
a single-user local process has no co-tenant to protect, and a limiter there only
costs latency. Setting `0` explicitly is the opt-out in either mode.

### Configuration

| Setting         | Env var                       | Flag (HTTP mode)     | Default (stdio) | Default (HTTP) |
| --------------- | ----------------------------- | -------------------- | --------------- | -------------- |
| Requests/second | `GITLAB_MCP_RATE_LIMIT_RPS`   | `--rate-limit-rps`   | `0` (disabled)  | `10`           |
| Burst capacity  | `GITLAB_MCP_RATE_LIMIT_BURST` | `--rate-limit-burst` | `40`            | `40`           |

The defaults differ because the deployments differ. The MCP specification
requires a server exposing tools to rate limit their invocation, and an HTTP
deployment is the shared one: every call it forwards is charged to its own
egress address, so one looping client's volume lands on every other tenant and
on the instance's own limits. A stdio process serves one user on their own
machine, has no co-tenant to protect, and a limiter there only costs latency.

`10` is a judgement call rather than a specification value — far above any
human-driven session, and still a bound on a retry loop. Setting `0` explicitly
is the supported opt-out in either mode: the middleware is then not attached and
there is zero overhead on the hot path. Setting any value `> 0` activates a `golang.org/x/time/rate`
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
| HTTP default                    | `10`               | On unless you say otherwise; bounds a looping client without touching normal use.              |
| Disabled (stdio default)        | `0`                | Trust GitLab's own throttle; the right answer for a single-user local process.                 |

### Behavior on excess

When the bucket is empty the middleware short-circuits the call and returns a
`CallToolResult` with `IsError: true` and a human-readable hint:

```text
Rate limit exceeded for `gitlab_mr_list`. Wait a moment and retry, or raise --rate-limit-rps if this is sustained traffic.
```

The error is returned as a tool result (not a JSON-RPC error) so the LLM can
parse it and decide whether to back off, batch differently, or surface the
problem to the user. `tools/list`, `resources/*`, and `prompts/*` are **not**
gated.

### Defense-in-depth

The local limiter complements but does not replace:

- GitLab's per-user rate limiter (primary defense).
- HTTP-mode bounded server pool (`GITLAB_MCP_MAX_HTTP_CLIENTS`) which caps concurrency.
- Reverse-proxy/WAF policies in front of public deployments.

Disable it by setting the value to `0` explicitly — `GITLAB_MCP_RATE_LIMIT_RPS=0` in stdio,
`--rate-limit-rps=0` in HTTP mode. Omitting the flag no longer disables it in
HTTP mode, where `10` is the default. No state is persisted between restarts.

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
