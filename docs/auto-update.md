# Auto-Update

gitlab-mcp-server can automatically detect, download, and apply new releases from a GitHub repository. Updates are fetched from the GitHub Releases API, validated with a `checksums.txt` file, and applied using a **rename trick** — the running binary is renamed to `.old`, the new binary is placed at the original path, and on Unix the process re-executes itself seamlessly via `syscall.Exec`. On Windows, the new binary is ready at the original path and takes effect on the next restart. Old `.old` files are cleaned up automatically at startup.

> **Diátaxis type**: Explanation
> **Audience**: 👤🔧 All users
> **Prerequisites**: gitlab-mcp-server installed, GitHub repository with releases
> 📖 **User documentation**: See the [Auto-update](https://jmrplens.github.io/gitlab-mcp-server/operations/auto-update/) on the documentation site for a user-friendly version.

---

## How It Works

```mermaid
sequenceDiagram
    participant Server as gitlab-mcp-server
    participant GitHub as GitHub Releases API
    participant FS as File System

    Server->>Server: CleanupOldBinary() — remove .old from previous update
    Server->>Server: JustUpdated()? Skip check (re-exec guard)
    Server->>GitHub: GET /repos/:owner/:repo/releases/latest
    GitHub-->>Server: Release metadata + asset URLs
    alt Update available
        Server->>GitHub: Download binary asset
        GitHub-->>Server: Binary data
        Server->>FS: Write to .tmp staging file
        Server->>FS: Rename current → .old (rename trick)
        Server->>FS: Rename .tmp → current path
        alt Unix (Linux/macOS)
            Server->>Server: Set PE_MCP_JUST_UPDATED=1
            Server->>Server: syscall.Exec(self) — same PID, same pipes
            Note right of Server: Process image replaced,<br/>new code running instantly
        else Windows
            Server->>Server: Log "update will take effect on next restart"
        end
    else Up to date
        Server->>Server: Log "server is up to date"
    end
```

The update mechanism uses [creativeprojects/go-selfupdate](https://github.com/creativeprojects/go-selfupdate) v1.5.2 with a GitHub source backend. Key properties:

- **GitHub Releases**: Auto-update fetches releases from the GitHub repository configured via `AUTO_UPDATE_REPO` (default: `jmrplens/gitlab-mcp-server`).
- **Rename trick**: The running binary is renamed to `.old` (allowed even on Windows where executables are locked), and the new binary is placed at the original path. This eliminates the need for deferred scripts or manual intervention.
- **Unix seamless re-exec**: On Linux/macOS, after replacing the binary, the process calls `syscall.Exec()` which replaces the process image in-place — the PID stays the same, stdin/stdout file descriptors are preserved, and the MCP client sees no interruption.
- **Windows next-restart**: On Windows, `syscall.Exec` is not available (it creates a new process, losing stdio pipes). The new binary is ready at the original path and activates on the next server restart.
- **External updater integration (`--shutdown`)**: An external store app (pe-agnostic-store) can invoke `gitlab-mcp-server --shutdown` to terminate all running instances before replacing the binary on disk. This flag finds matching processes by name, sends a graceful termination signal, waits up to 5 seconds, and force-kills any remaining. Uses [gopsutil](https://github.com/shirou/gopsutil) for cross-platform process listing (Linux, macOS, Windows). No admin/root permissions required.
- **Re-exec loop prevention**: An environment variable `PE_MCP_JUST_UPDATED=1` is set before re-exec and cleared on the new process startup, preventing infinite update loops.
- **Old binary cleanup**: At startup, `CleanupOldBinary()` removes any leftover `.old` file from a previous update.
- **Checksum validation**: Each release must include a `checksums.txt` asset (goreleaser format) containing SHA-256 hashes for all platform binaries.
- **Platform detection**: The library automatically selects the correct binary for the running OS and architecture (`{name}-{goos}-{goarch}`).

## Update Modes

The `AUTO_UPDATE` variable controls behaviour:

| Value | Mode | Behaviour |
| --- | --- | --- |
| `true` (default) | Auto | Detect and apply updates automatically |
| `check` | Check-only | Detect updates and log availability, but do not apply |
| `false` | Disabled | Skip all update checks entirely |

Accepted aliases: `1`/`yes` for true, `0`/`no` for false. The value is case-insensitive.

## Transport-Specific Behaviour

### Stdio Mode

When running as a stdio server (the default), auto-update runs as a **pre-start update** with a 15-second timeout, **before** the MCP server logic begins:

1. `CleanupOldBinary()` — remove leftover `.old` file from a previous update.
2. Parse `AUTO_UPDATE` mode from environment variables.
3. Check `PE_MCP_JUST_UPDATED` — if set, skip the update check (re-exec guard) and clear the variable.
4. Create an `Updater` with the GitHub source and `AUTO_UPDATE_REPO`.
5. Call `PreStartUpdate()`:
   - If mode is `true` and a newer release exists → download to `.tmp`, rename current to `.old`, move `.tmp` to original path.
   - **Unix**: Set `PE_MCP_JUST_UPDATED=1` → call `syscall.Exec(self)` → the process image is replaced with the new binary, same PID, same stdin/stdout pipes. The new binary starts, sees the guard, skips the update check, and proceeds to serve.
   - **Windows**: Log "update will take effect on next restart". The new binary is already at the original path.
   - If mode is `check` → log the available version without applying.
6. Continue with normal server startup regardless of update outcome.

The startup check is **non-fatal**: any error (network timeout, invalid token, missing releases) is logged as a warning and does not prevent the server from starting.

```text
INFO autoupdate: server is up to date  version=1.1.7
```

or (Unix — seamless re-exec):

```text
INFO autoupdate: new version available  current_version=1.1.7 latest_version=1.2.0
INFO autoupdate: binary updated on disk  new_version=1.2.0
INFO autoupdate: re-executing with new binary  new_version=1.2.0
INFO autoupdate: skipping update check (just re-executed after update)
INFO starting MCP server  transport=stdio version=1.2.0
```

or (Windows — next restart):

```text
INFO autoupdate: new version available  current_version=1.1.7 latest_version=1.2.0
INFO autoupdate: binary updated on disk  new_version=1.2.0
INFO autoupdate: update will take effect on next restart  new_version=1.2.0
INFO starting MCP server  transport=stdio version=1.1.7
```

### HTTP Mode

When running as an HTTP server (`--http`), auto-update runs as a **background periodic check**:

1. Parse `--auto-update` flag (defaults to `true`).
2. Create an `Updater` with the GitHub source, `--auto-update-repo`, and `--auto-update-interval`.
3. Launch `StartPeriodicCheck()` — a goroutine that runs every `--auto-update-interval` (default: 1 hour).
4. Each cycle:
   - Check for a newer release (30-second timeout per check).
   - If mode is `true` → apply the update and log a restart advisory.
   - If mode is `check` → log availability only.
5. The goroutine stops when the server context is cancelled (graceful shutdown).

> **Note**: Auto-update uses the GitHub Releases API, completely independent of the user's GitLab configuration.

## Configuration Reference

### Environment Variables (Stdio Mode)

| Variable | Default | Description |
| --- | --- | --- |
| `AUTO_UPDATE` | `true` | Update mode: `true`, `check`, or `false` |
| `AUTO_UPDATE_REPO` | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for release assets |
| `AUTO_UPDATE_INTERVAL` | `1h` | Check interval (used by HTTP mode periodic checks) |

Auto-update uses the GitHub Releases API via `AUTO_UPDATE_REPO`. It does **not** use the user's `GITLAB_URL`, `GITLAB_TOKEN`, or `GITLAB_SKIP_TLS_VERIFY`.

### CLI Flags (HTTP Mode)

| Flag | Default | Description |
| --- | --- | --- |
| `--auto-update` | `true` | Update mode: `true`, `check`, or `false` |
| `--auto-update-repo` | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for release assets |
| `--auto-update-interval` | `1h` | Interval between periodic update checks |

Auto-update uses the GitHub Releases API, so `--gitlab-url` and `--skip-tls-verify` do **not** affect auto-update behaviour.

### Configuration Examples

Disable auto-update entirely:

```env
AUTO_UPDATE=false
```

Check-only mode (log available updates without applying):

```env
AUTO_UPDATE=check
```

Custom repository and fast check interval:

```env
AUTO_UPDATE=true
AUTO_UPDATE_REPO=my-group/my-project
AUTO_UPDATE_INTERVAL=15m
```

HTTP mode with custom settings:

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --auto-update=check \
  --auto-update-interval=30m
```

## MCP Tools

When auto-update is enabled, update tools are registered as part of the `gitlab_mcp` meta-tool (or as individual tools in non-meta mode). These allow AI assistants to check for and apply updates on demand:

### `gitlab_mcp_check_update`

Check if a newer version of the MCP server is available.

**Input**: None (empty object `{}`).

**Output**:

| Field | Type | Description |
| --- | --- | --- |
| `update_available` | boolean | Whether a newer version exists |
| `current_version` | string | Currently running version |
| `latest_version` | string | Latest release version (if available) |
| `release_url` | string | URL to the release page |
| `release_notes` | string | Release notes content |
| `mode` | string | Current auto-update mode |

**Annotations**: Read-only (`readOnlyHint: true`, `idempotentHint: true`).

**Example response** (Markdown):

```markdown
## ⬆️ Update Available

- **Current Version**: 1.1.7
- **Latest Version**: 1.2.0
- **Release URL**: https://github.com/jmrplens/gitlab-mcp-server/releases/tag/v1.2.0

### Release Notes

- Added new pipeline tools
- Fixed merge request approval handling
```

### `gitlab_mcp_apply_update`

Download and apply the latest MCP server update. The binary is replaced using the rename trick (rename current → `.old`, place new binary at original path). On all platforms a server restart is required to use the new version.

**Input**: None (empty object `{}`).

**Output**:

| Field | Type | Description |
| --- | --- | --- |
| `applied` | boolean | Whether the update was applied (binary replaced on disk) |
| `previous_version` | string | Version before the update |
| `new_version` | string | Version after applying the update |
| `message` | string | Human-readable status message |

**Annotations**: Destructive (`destructiveHint: true`) — replaces the server binary.

**Example response** (Markdown):

```markdown
## ✅ Update Applied

- **Previous Version**: 1.1.7
- **New Version**: 1.2.0

> **Note**: Restart the server to use the new version.
```

> **Important**: These tools are only registered when auto-update is enabled (`AUTO_UPDATE` is not `false`) and the binary was built with version information (`-ldflags`). Development builds (`version=dev`) disable auto-update.

## Architecture

```mermaid
graph TD
    subgraph "cmd/server/main.go"
        A0[CleanupOldBinary] --> A
        A[runStdio] -->|pre-start| B[preStartAutoUpdate]
        A -->|creates| C[newUpdaterForTools]
        D0[CleanupOldBinary] --> D
        D[runHTTP] -->|background| E[startAutoUpdate]
    end

    subgraph "internal/autoupdate"
        B --> F[PreStartUpdate]
        F --> H[CheckForUpdate]
        F --> P[downloadToStaging]
        F --> Q[replaceExecutable]
        F -->|Unix| R[ExecSelf — syscall.Exec]
        E --> G[Updater.StartPeriodicCheck]
        G --> H
        G --> I[ApplyUpdate]
        H --> J[go-selfupdate<br/>DetectLatest]
        I --> K[go-selfupdate<br/>UpdateSelf]
        I -->|Windows fallback| P
        I -->|Windows fallback| Q
    end

    subgraph "internal/tools/serverupdate"
        C --> L[RegisterTools]
        L --> M[gitlab_mcp_check_update]
        L --> N[gitlab_mcp_apply_update]
        M --> H
        N --> I
        N -->|Windows fallback| S[DownloadAndReplace]
    end

    J -->|GitHub API| O[(GitHub Releases)]
    K -->|Download + Checksum| O
    P -->|Download| O
```

### Package Responsibilities

| Package | Role |
| --- | --- |
| `internal/autoupdate` | Core update logic: detect releases, download, rename trick replacement, re-exec (Unix), old binary cleanup. Transport-agnostic. |
| `internal/autoupdate/exec_unix.go` | `ExecSelf()` — `syscall.Exec` to re-exec the process (same PID, same FDs). Build tag: `!windows`. |
| `internal/autoupdate/exec_windows.go` | `ExecSelf()` — stub returning an error (exec not supported). Build tag: `windows`. |
| `internal/autoupdate/prestart.go` | `PreStartUpdate()` — pre-start flow: check → download → rename → exec/log. |
| `internal/tools/serverupdate` | MCP tool wrappers exposing `Check` and `Apply` as MCP tools with Markdown formatting. |
| `cmd/server/main.go` | Wiring: calls `CleanupOldBinary()` and `preStartAutoUpdate` (stdio), `startAutoUpdate` (HTTP), `newUpdaterForTools` (MCP tools). |

## Release Requirements

For auto-update to work, GitHub releases must follow this structure:

1. **Tag format**: Semantic version with `v` prefix (e.g., `v1.1.0`).
2. **Binary assets**: One per supported platform, named `gitlab-mcp-server-{goos}-{goarch}[.exe]`:
   - `gitlab-mcp-server-linux-amd64`
   - `gitlab-mcp-server-linux-arm64`
   - `gitlab-mcp-server-darwin-amd64`
   - `gitlab-mcp-server-darwin-arm64`
   - `gitlab-mcp-server-windows-amd64.exe`
   - `gitlab-mcp-server-windows-arm64.exe`
3. **Checksum file**: A `checksums.txt` asset containing SHA-256 hashes in goreleaser format:

   ```text
   abcdef1234567890...  gitlab-mcp-server-linux-amd64
   fedcba0987654321...  gitlab-mcp-server-windows-amd64.exe
   ```

4. **Signature file** (optional): A `checksums.txt.asc` asset containing the detached PGP/GPG signature of `checksums.txt`. Required only when GPG verification is enabled via `AUTO_UPDATE_GPG_KEY`. Generate with: `gpg --detach-sign --armor checksums.txt` (the build scripts do this automatically when `GPG_SIGN=1`).

The Makefile `release` target generates all of these automatically.

## Security Considerations

- **Token scope**: Auto-update uses a dedicated built-in token (injected at build time), separate from the user's `GITLAB_TOKEN`. The built-in token is a GitHub token that needs read access to releases. No additional permissions are required.
- **TLS verification**: Auto-update always uses TLS verification (hardcoded `SkipTLS: false`) since it connects to GitHub (`github.com`) with a valid certificate. The user's `GITLAB_SKIP_TLS_VERIFY` setting does not affect auto-update.
- **Binary integrity**: Each downloaded binary is validated against the `checksums.txt` file before replacement. If the checksum does not match, the update is rejected.
- **GPG signature verification** (optional): When `AUTO_UPDATE_GPG_KEY` (or `--auto-update-gpg-key` in HTTP mode) points to an armored PGP public key file, the server additionally verifies that `checksums.txt` is signed by the expected key. The library downloads `checksums.txt.asc` from the release and validates the signature before trusting the checksums. If the GPG key is malformed or the `.asc` file is missing, the server logs a warning and falls back to checksum-only validation.
- **Rename-and-rollback**: The old binary is renamed to `.old` before replacement. If the new binary fails to be placed, the `.old` is renamed back to the original path as a rollback.
- **Development builds**: Binaries built without `-ldflags -X main.version=...` report `version=dev` and auto-update is disabled to prevent accidental overwrites during development.

## Troubleshooting

| Symptom | Cause | Solution |
| --- | --- | --- |
| `autoupdate: current version is required (binary built without -ldflags?)` | Binary built without version injection | Build with `make build` or add `-ldflags "-X main.version=1.2.0"` |
| `autoupdate: repository is required` | `AUTO_UPDATE_REPO` is empty | Set `AUTO_UPDATE_REPO` or use the default |
| `autoupdate: creating GitHub source` | Network error reaching GitHub API | Verify network connectivity to `github.com` |
| `autoupdate: detecting latest release` | No releases in repository, or token lacks permissions | Create a release or check token permissions |
| `autoupdate: startup check failed` | Network timeout (10s limit at startup) | Check network connectivity; the server starts anyway |
| `autoupdate: could not initialize periodic updater` | Missing required config in HTTP mode | Verify `--auto-update-repo` flag and network connectivity |
| Update detected but not applied | Mode is `check` | Set `AUTO_UPDATE=true` to enable automatic application |
| Server still runs old version after update (Windows) | Binary replaced but process not restarted | Restart the server process (Windows only — Unix re-execs automatically) |
| `autoupdate: exec-self failed` | `syscall.Exec` failed on Unix | Server continues with old code; restart manually |
| `autoupdate: skipping update check (just re-executed after update)` | Normal: re-exec guard preventing loop | No action needed — this is expected after a successful update |
| `autoupdate: invalid GPG public key, falling back to checksum-only validation` | Malformed or corrupted PGP key file | Verify the key file is a valid armored PGP public key (`gpg --show-keys key.pub`) |
| `autoupdate: GPG key path is not a regular file` | Path points to directory, symlink, or device | Ensure `AUTO_UPDATE_GPG_KEY` points to a regular file |
| GPG verification configured but updates still work without `.asc` file | `checksums.txt.asc` missing in release → falls back to checksum-only | Upload signed checksums to the release (`GPG_SIGN=1 make release`) |

## Disabling Auto-Update

To completely disable all update-related functionality:

```env
AUTO_UPDATE=false
```

Or in HTTP mode:

```bash
gitlab-mcp-server --http --auto-update=false ...
```

This disables:

- Startup update check (stdio mode)
- Periodic background checks (HTTP mode)
- MCP tool registration (`gitlab_mcp_check_update` and `gitlab_mcp_apply_update` are not registered)
