# Security Policy

## Reporting a Vulnerability

> **Do NOT create a public issue for security vulnerabilities.**

If you discover a security vulnerability in gitlab-mcp-server, please report it responsibly:

1. **Contact**: Send a detailed report to `@jmrplens` via direct message
2. **Include**: Description of the vulnerability, steps to reproduce, potential impact
3. **Expectation**: You will receive an acknowledgment within 48 hours

## Supported Versions

| Version | Supported |
| ------- | --------- |
| latest  | Yes       |
| < 1.0.0 | No        |

## Security Considerations

### Token Handling

- The GitLab Personal Access Token is provided via `GITLAB_TOKEN` environment variable (stdio mode) or per-request HTTP header (HTTP mode)
- Tokens are never logged, displayed in tool output, or included in error messages
- In HTTP mode, each client authenticates via `PRIVATE-TOKEN` or `Authorization: Bearer` header — tokens are isolated per session
- The `.env` file containing credentials is excluded from version control via `.gitignore`

### File System Access (MCP Roots)

- File upload via `file_path` is restricted to directories declared as MCP Roots by the client
- Path traversal attacks are prevented by validating absolute paths against allowed root directories
- Symlinks and relative paths (`..`) are resolved before validation
- If no MCP Roots are configured, file_path uploads are denied (fail-safe)

### TLS Configuration

- TLS certificate verification is enabled by default
- Self-signed certificates can be accepted via `GITLAB_SKIP_TLS_VERIFY=true`
- This setting should **only** be used in trusted internal networks

### Read-Only Mode

- Set `GITLAB_READ_ONLY=true` to disable all mutating tools (create, update, delete)
- Only read-only tools (list, get, search) are registered in this mode
- Provides an additional layer of protection for sensitive GitLab instances

### Input Validation

- All tool inputs are validated before GitLab API calls
- Required parameters are checked explicitly — missing values produce clear error messages
- Integer IDs are validated to prevent injection
- String parameters are sanitized where applicable

### Error Handling

- Error messages never expose internal server details or stack traces
- API errors from GitLab are wrapped with context but sensitive headers are stripped
- Authentication failures return generic messages without revealing credential details

### Dependencies

- Minimal dependency footprint
- Dependencies are tracked in `go.sum` with cryptographic checksums
- Regular dependency updates are performed to address known vulnerabilities
- Automated dependency scanning via Dependabot

## Security Best Practices for Deployment

1. **Use a dedicated GitLab token** with minimal required scopes (prefer `read_api` for read-only use cases)
2. **Run the server as a non-privileged user** — avoid root/administrator
3. **Enable TLS** between the MCP server and GitLab instance in production
4. **Keep the .env file permissions restrictive** (`chmod 600` on Unix systems)
5. **Use MCP Roots** to limit file system access to specific directories
6. **Use read-only mode** (`GITLAB_READ_ONLY=true`) when mutation is not needed
7. **Monitor token usage** via GitLab's admin panel
8. **Rotate tokens periodically** according to your organization's policy
9. **In HTTP mode**, restrict network access to trusted clients only
