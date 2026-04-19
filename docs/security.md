# Security

Security considerations for gitlab-mcp-server deployment and development.

> **Diátaxis type**: Explanation
> **Audience**: 👤🔧 All users
> **Prerequisites**: Familiarity with GitLab PATs and TLS configuration
> 📖 **User documentation**: See the [Security](https://jmrplens.github.io/gitlab-mcp-server/operations/security/) on the documentation site for a user-friendly version.

---

## Authentication

gitlab-mcp-server authenticates to GitLab using a Personal Access Token (PAT) passed via the `GITLAB_TOKEN` environment variable. The token requires the `api` scope for full tool functionality.

### Token Security

- **Never commit tokens** — `.env` is gitignored; use environment variables in CI/production
- **Wizard-managed secrets** — The Setup Wizard stores `GITLAB_TOKEN`, `GITLAB_URL`, and `GITLAB_SKIP_TLS_VERIFY` in `~/.gitlab-mcp-server.env` with restricted permissions (`0600` on Unix). Client config files only contain non-secret preferences. The server loads this file automatically at startup as a fallback
- **Never hardcode tokens in JSON** — MCP client configuration files (`.vscode/mcp.json`, `.cursor/mcp.json`) are often committed to version control. Use [input variables](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_input-variables-for-sensitive-data) (`${input:gitlab-token}`), [environment files](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_standard-io-stdio-servers) (`envFile`), or system environment variables instead. See [Configuration — Secure Token Configuration](configuration.md#secure-token-configuration) for examples
- **Minimum scope** — Use `api` scope only; avoid `admin` scope unless required
- **Token rotation** — Rotate tokens regularly; use expiring tokens when possible
- **Secret redaction** — The error reporting system (`issue_report.go`) automatically redacts fields containing: `token`, `password`, `secret`, `key`, `credential`, `auth`, `cookie`, `session`, `private`. Issue report generation is opt-in (`ISSUE_REPORTS=true`); when disabled, errors use the standard Markdown format without input parameter details

## TLS

- All GitLab API communication uses HTTPS by default
- Self-signed certificates: set `GITLAB_SKIP_TLS_VERIFY=true` (development only)
- Production deployments should use valid TLS certificates

## Input Validation

All tool handlers validate inputs before making API calls:

- **Required fields** — Checked before hitting the GitLab API
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

## Transport Security

### stdio (Default)

Communication occurs over stdin/stdout within the local process. No network exposure.

### HTTP Mode

When running with `--http`:

- Binds to `localhost` by default — not exposed to the network
- No built-in authentication on the HTTP endpoint
- For production use, place behind a reverse proxy with proper TLS and auth

### OAuth Mode (`--auth-mode=oauth`)

When running with `--auth-mode=oauth`, the server validates every request's Bearer token against the GitLab `/api/v4/user` endpoint before processing:

- **Token verification** — Each token is validated by calling GitLab's user API. Invalid or expired tokens receive HTTP 401
- **Identity caching** — Verified token identities are cached in-memory using SHA-256 hashed keys (raw tokens are never stored). Cache TTL is configurable via `--oauth-cache-ttl` (default 15m, range 1m–2h)
- **Header normalization** — `PRIVATE-TOKEN` headers are automatically converted to `Authorization: Bearer` for backward compatibility with existing clients
- **[RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) metadata** — The `/.well-known/oauth-protected-resource` endpoint advertises the GitLab authorization server URL, enabling compliant OAuth clients to discover the token issuer
- **PKCE** — The OAuth 2.1 flow uses Proof Key for Code Exchange (PKCE) to protect against authorization code interception attacks. MCP clients generate a code verifier/challenge pair for each authorization request
- **Cache eviction** — A background goroutine runs every 30 seconds to clean up expired entries. The cache is bounded by TTL, not by size

See [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode) for the full architecture and flow diagram, and [OAuth App Setup](oauth-app-setup.md) for creating GitLab OAuth applications.

| Threat | Mitigation |
| --- | --- |
| Token replay | TTL-based expiration; tokens re-verified after cache expires |
| Cache key leakage | SHA-256 hashing of raw tokens; original tokens never stored |
| Brute force | GitLab API rate limiting applies to verification requests |
| Memory dump | Only SHA-256 hashes and user metadata stored; no raw tokens in cache |

## Error Information Disclosure

The error handling system is designed to be informative for LLMs while avoiding information leakage:

- **ClassifyError** returns semantic descriptions, not raw stack traces
- **DetailedError.Markdown** includes HTTP status and request ID for diagnostics
- **FormatIssueReport** redacts sensitive input fields before generating bug reports
- Internal implementation details are not exposed in error messages

## Dependencies

| Dependency | Security Notes |
| --- | --- |
| `gitlab.com/gitlab-org/api/client-go/v2` | Official GitLab client; uses `retryablehttp` with exponential backoff |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP SDK; handles JSON-RPC transport |
| `github.com/joho/godotenv` | Loads `.env` files (CWD and `~/.gitlab-mcp-server.env` fallback) |

Run `go list -m all` to see all transitive dependencies. Use `govulncheck` for vulnerability scanning:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Auto-Update Token Security

### Threat Model

The auto-update subsystem embeds a GitHub API token in the
compiled binary to check for and download new releases. Attack vectors:

| Vector | Description | Mitigation |
|--------|-------------|------------|
| Traffic capture (HTTP) | Intercept token on the wire | HTTPS enforcement in `NewUpdater()` |
| Proxy interception (`HTTPS_PROXY`) | Token sent through attacker proxy | `Proxy: nil` in HTTP transport |
| HTTP redirect to external host | GitHub redirects to S3/CDN leaking token | `CheckRedirect` strips `Authorization` on cross-host |
| Protocol downgrade (HTTPS→HTTP) | Redirect from HTTPS to HTTP exposes token | `CheckRedirect` refuses HTTPS→HTTP redirects |
| Redirect chain abuse | Infinite redirects / open redirect exploitation | Max 10 redirects enforced |
| Token to external hosts | Asset URL points to non-GitHub host | `sameHost()` check before attaching header |
| Memory dump (`gcore`, `/proc/PID/mem`) | Read token from process memory | Intermediate `[]byte` zeroed; globals zeroed after first use |
| Accidental logging (`%v`, panic) | Token printed in logs or stack traces | `Config.String()` / `GoString()` redact to `***` |
| `GetConfig()` API | Token exposed via MCP tool | Returns copy with `Token: "***"` |

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

| File | Purpose |
|------|---------|
| `internal/autoupdate/github_source.go` | `newGitHubSource` with hardened HTTP client |
| `internal/autoupdate/autoupdate.go` | HTTPS enforcement, `Config.String()`/`GoString()` |
| `cmd/server/main.go` | Update initialization |

---

## See Also

### Internal

- [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode) — OAuth architecture and flow diagram
- [OAuth App Setup](oauth-app-setup.md) — creating GitLab OAuth applications
- [Troubleshooting — OAuth Mode](troubleshooting.md#oauth-mode---auth-modeoauth) — OAuth-specific troubleshooting

### External

- [RFC 9728: OAuth 2.0 Protected Resource Metadata](https://datatracker.ietf.org/doc/html/rfc9728) — the specification behind `--auth-mode=oauth`
- [OAuth 2.1 Authorization Framework (draft)](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-12) — mandates PKCE for all clients
- [GitLab: Configure GitLab as an OAuth 2.0 provider](https://docs.gitlab.com/ee/integration/oauth_provider.html) — GitLab OAuth Application docs
