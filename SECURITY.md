# Security Policy

## Reporting a Vulnerability

> **Do NOT open a public issue for security vulnerabilities.**

The preferred channel is **[GitHub Security Advisories](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/new)**, which keeps the report private until a coordinated fix is published.

Please include:

- A clear description of the vulnerability and its potential impact.
- Steps to reproduce (proof-of-concept code, request samples, or affected configuration).
- Affected version(s) — `gitlab-mcp-server --version` output and commit hash if built from source.
- Environment (OS, GitLab version, transport: stdio or HTTP, MCP client).
- Any suggested mitigation, if known.

If GitHub Security Advisories is unavailable to you, contact the maintainer privately via direct message on GitHub (`@jmrplens`). Do not send sensitive details over public channels.

### Preferred Languages

English (preferred) or Spanish.

## Response SLA

| Stage                          | Target                                         |
| ------------------------------ | ---------------------------------------------- |
| Acknowledgement of report      | within **48 hours**                            |
| Initial triage and severity    | within **7 days**                              |
| Fix for **Critical / High**    | within **30 days** of confirmation             |
| Fix for **Medium / Low**       | within **90 days** of confirmation             |
| Public disclosure / advisory   | after a fix is released, with a typical **7-day embargo** so users can update |

These targets are best-effort for a maintainer-driven open-source project. We will keep you informed of progress and any expected delay.

## Scope

### In scope

- The `gitlab-mcp-server` source code in this repository (Go server, MCP tools, transports, prompts, resources).
- Authentication and authorization handling (token storage, OAuth flows, HTTP session isolation).
- Input validation in MCP tool handlers.
- Path handling and MCP Roots enforcement (file uploads, downloads).
- TLS configuration handling and `GITLAB_SKIP_TLS_VERIFY` semantics.
- Error messages and logs that could leak credentials or sensitive metadata.
- Released binaries and Docker images published from this repository.
- Auto-update mechanism (signature verification, integrity checks).

### Out of scope

- Vulnerabilities in the **GitLab server** itself — please report those to [GitLab](https://about.gitlab.com/security/disclosure/).
- Vulnerabilities in **upstream dependencies** that have already been disclosed and patched upstream — open a regular issue or PR to bump the dependency instead.
- Misconfigurations of the operator's environment (leaked PATs, world-readable `.env`, exposed HTTP port without authentication, etc.) that are explicitly warned against in the documentation.
- Issues that require the attacker to already control the host running the server (kernel exploits, container escapes, side-channel attacks on memory).
- Denial-of-service via legitimate but expensive GitLab API queries (rate limiting is the operator's responsibility).
- Findings from automated scanners without a demonstrated impact (please include a working PoC).

## Supported Versions

Security fixes are issued for the latest stable release line on `main`. Older releases do not receive backports.

| Version                | Supported          |
| ---------------------- | ------------------ |
| Latest `1.x` release   | :white_check_mark: |
| Older `1.x` releases   | :x: (please update) |
| `0.x` (pre-1.0)        | :x:                |

We strongly recommend running the most recent release. The auto-update mechanism (`AUTO_UPDATE=true`, default) keeps the binary current.

## Coordinated Disclosure

We follow a **coordinated disclosure** model aligned with [ISO/IEC 29147](https://www.iso.org/standard/72311.html) and the [OWASP Vulnerability Disclosure Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Vulnerability_Disclosure_Cheat_Sheet.html):

1. You report privately via GitHub Security Advisories.
2. We acknowledge, triage, and confirm the issue.
3. We develop and test a fix in a private fork.
4. We release the fix and publish a GitHub Security Advisory (and request a CVE when applicable).
5. After a typical **7-day embargo**, full technical details may be disclosed publicly.

If a vulnerability is being actively exploited in the wild, we may shorten or skip the embargo to protect users.

## Safe Harbor

We support **good-faith security research** on this project. If you make a reasonable effort to comply with this policy, we will:

- Consider your research authorized under our terms of use.
- Work with you to understand and resolve the issue quickly.
- Not pursue or support legal action against you, or report you to law enforcement, for accidental or good-faith violations.
- Recognise your contribution publicly (see *Credit*) unless you prefer to remain anonymous.

To stay within safe harbor, you must:

- Only test against your own deployment of `gitlab-mcp-server` (do not target third-party hosts or organizations).
- Avoid privacy violations, data destruction, and service degradation of others.
- Stop testing and report immediately if you encounter user data, credentials, or PII.
- Give us reasonable time to remediate before any public disclosure.

This safe-harbor language is inspired by [disclose.io](https://disclose.io/) core terms.

## Credit and CVE

- We are happy to **credit** reporters in the published advisory and release notes (handle, real name, or anonymous — your choice).
- For qualifying issues we will **request a CVE** through GitHub's CNA and reference it in the advisory.
- There is currently **no monetary bug bounty**.

## Security Considerations

The remainder of this document describes how the server handles security-sensitive concerns. Operators should review these to harden their deployments.

### Token Handling

- The GitLab Personal Access Token is provided via `GITLAB_TOKEN` environment variable (stdio mode) or per-request HTTP header (HTTP mode).
- Tokens are never logged, displayed in tool output, or included in error messages.
- In HTTP mode, each client authenticates via `PRIVATE-TOKEN` or `Authorization: Bearer` header — tokens are isolated per session.
- The `.env` file containing credentials is excluded from version control via `.gitignore`.

### File System Access (MCP Roots)

- File upload via `file_path` is restricted to directories declared as MCP Roots by the client.
- Path traversal attacks are prevented by validating absolute paths against allowed root directories.
- Symlinks and relative paths (`..`) are resolved before validation.
- If no MCP Roots are configured, `file_path` uploads are denied (fail-safe).

### TLS Configuration

- TLS certificate verification is enabled by default.
- Self-signed certificates can be accepted via `GITLAB_SKIP_TLS_VERIFY=true`.
- This setting should **only** be used in trusted internal networks.

### Read-Only and Safe Modes

- Set `GITLAB_READ_ONLY=true` to disable all mutating tools (create, update, delete). Only read-only tools (list, get, search) are registered.
- Set `GITLAB_SAFE_MODE=true` to intercept mutating tools and return a JSON preview instead of executing the change.
- Both flags provide additional protection for sensitive GitLab instances.

### Input Validation

- All tool inputs are validated before GitLab API calls.
- Required parameters are checked explicitly — missing values produce clear error messages.
- Integer IDs are validated to prevent injection.
- String parameters are sanitized where applicable.

### Error Handling

- Error messages never expose internal server details or stack traces.
- API errors from GitLab are wrapped with context but sensitive headers are stripped.
- Authentication failures return generic messages without revealing credential details.

### Dependencies

- Minimal dependency footprint.
- Dependencies are tracked in `go.sum` with cryptographic checksums.
- Regular dependency updates are performed to address known vulnerabilities (`govulncheck` runs in CI).
- Automated dependency scanning via Dependabot.

### Release Integrity

- Release binaries are built from tagged commits and published via GoReleaser.
- Checksums (`checksums.txt`) and a GPG-signed `checksums.txt.asc` are attached to every GitHub Release.
- The auto-update mechanism verifies integrity against published checksums before replacing the running binary.

## Security Best Practices for Deployment

1. **Use a dedicated GitLab token** with minimal required scopes (prefer `read_api` for read-only use cases).
2. **Run the server as a non-privileged user** — avoid root/administrator.
3. **Enable TLS** between the MCP server and GitLab instance in production.
4. **Keep the `.env` file permissions restrictive** (`chmod 600` on Unix systems).
5. **Use MCP Roots** to limit file system access to specific directories.
6. **Use read-only or safe mode** (`GITLAB_READ_ONLY=true` or `GITLAB_SAFE_MODE=true`) when mutation is not needed or must be reviewed.
7. **Monitor token usage** via GitLab's admin panel.
8. **Rotate tokens periodically** according to your organization's policy.
9. **In HTTP mode**, restrict network access to trusted clients only and consider running behind a TLS-terminating reverse proxy.
10. **Keep `gitlab-mcp-server` updated** — enable `AUTO_UPDATE=true` (default) or subscribe to repository releases.
