# Security Policy

## Reporting a Vulnerability

> **Do NOT open a public issue for security vulnerabilities.**

The preferred channel is **[GitHub's private vulnerability reporting](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/new)**. It opens a draft security advisory that only you and the maintainer can read, and keeps the report private until a coordinated fix is published.

Please include:

- A clear description of the vulnerability and its potential impact.
- Steps to reproduce (proof-of-concept code, request samples, or affected configuration).
- Affected version(s) — `gitlab-mcp-server --version` output and commit hash if built from source.
- Environment (OS, GitLab version, transport: stdio or HTTP, MCP client).
- Any suggested mitigation, if known.

If you cannot use it, for instance because you have no GitHub account, email the same information to the maintainer at **[mail@jmrp.io](mailto:mail@jmrp.io)**. Email is not encrypted on its own, so to keep the report private on its way, encrypt it if you can to the maintainer's OpenPGP key, published at <https://github.com/jmrplens.gpg> with the fingerprint `0A99 3B26 8654 DBBA 52B7 E8D3 FCF6 5339 1E2C 91FC`, which carries that address. A report by email goes through the same process as one through GitHub. Do not send details over any public channel.

### Preferred Languages

English (preferred) or Spanish.

## Response SLA

| Stage                       | Target                                                                        |
| --------------------------- | ----------------------------------------------------------------------------- |
| Acknowledgement of report   | within **48 hours**                                                           |
| Initial triage and severity | within **7 days**                                                             |
| Fix for **Critical / High** | within **30 days** of confirmation                                            |
| Fix for **Medium / Low**    | within **90 days** of confirmation                                            |
| Public advisory             | once the release that carries the fix is published, which can be the same day |

These targets are best-effort for a maintainer-driven open-source project. We will keep you informed of progress and any expected delay.

Two advisories have been published so far:

- [GHSA-fcj2-hj27-hj26](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/GHSA-fcj2-hj27-hj26),
  rated High, was a server-side request forgery in legacy HTTP mode: with no
  fixed GitLab URL, a caller chose the host the server sent requests to through
  the `GITLAB-URL` header. It was reported on 12 June 2026 and fixed in 3.0.0,
  released on 10 September 2026: 90 days after the report, where the target for
  a High is 30 days from confirmation. Its advisory was published on
  14 September.
- [GHSA-rxjw-584q-hxjr](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/GHSA-rxjw-584q-hxjr),
  rated Medium, was a tool argument placed into the path of a request to GitLab
  without a check for path separators or dot segments, so a crafted value could
  make the request name a different GitLab API endpoint than the action
  intended, under the user's own token. The maintainer found it in the
  [October 2026 security review](docs/development/security-review-2026-10.md#findings),
  where it is finding 8, and opened its advisory on 28 October 2026. It was
  fixed in 3.4.0, released on 31 October 2026, and its advisory was published
  the same day.

## Threat model

`gitlab-mcp-server` sits between an AI client and a GitLab instance. It holds a
credential that usually carries the full `api` scope, it takes instructions from
a model, and it relays content written by other people — issue titles, MR
descriptions, commit messages, file contents, job logs — back to that model.
That shape is what determines whether something counts as a vulnerability here.

**Trust boundaries.** Four of them matter:

1. **Model → server.** Tool arguments arrive from a model that may have been
   influenced by content it read earlier. The server must not treat an argument
   as authority to exceed the token's own permissions, and must not let an
   argument redirect a request to an unintended host or path.
2. **GitLab → server → model.** GitLab content is untrusted input even when the
   GitLab instance is trusted infrastructure. An issue title, a repository file,
   a job log or a discussion note is written by whoever can open an issue or
   push a branch, which on a public project is anybody, and none of it is the
   caller. It is rendered into tool results through the escaping helpers in
   `internal/toolutil/text.go`, so a way to break out of a table cell, a code
   fence, a link label or a heading is a vulnerability, as is any way to forge
   the guidance the server writes for the model in its own voice.
3. **Tenant → tenant (HTTP mode).** One process serves many callers. The bounded
   per-token+URL server pool in `internal/serverpool/` is what keeps one caller's
   token, client, and cached tier from reaching another caller's session.
4. **Working directory → server.** The process starts wherever a client chose to
   start it, which for a stdio server is whatever repository somebody just
   opened. A file that arrives by `git clone` must not be able to configure this
   server, so a dotenv file is read only from `~/.gitlab-mcp-server.env` or from
   the path `GITLAB_MCP_ENV_FILE` names, and that variable is read from the
   process environment alone, so a file the server declines to load cannot
   nominate itself. A `./.env` is still looked for and reported at WARN with its
   absolute path, because a developer whose repository-local file stopped taking
   effect deserves to be told why rather than left debugging it.

**What we consider a vulnerability, specifically:**

- **Credential disclosure.** A PAT or OAuth token reaching a log line, an error
  message, a tool result, an MCP resource, or a crash dump. Tokens are meant to
  stay in memory and in the operator's `.env`, never in model-visible output.
- **Prompt injection escaping its labelling.** Externally controlled GitLab text
  that can be made to read to the calling model as an instruction rather than as
  quoted data. The payload originating on GitLab does not put it out of scope —
  how this server frames that text is our responsibility.
- **Cross-tenant leakage in HTTP mode.** Any path by which one bearer token
  observes another's GitLab client, cached identity, tier, or tool results, or by
  which pool eviction serves a session the wrong client.
- **Privilege escalation past the configured mode.** `GITLAB_MCP_READ_ONLY=true` and
  `GITLAB_MCP_SAFE_MODE=true` are enforcement, not hints. A mutating call that
  executes despite either flag is a vulnerability, including through the dynamic
  `gitlab_execute_action` surface.
- **Path traversal on file operations.** Upload and file-content tools take
  caller-supplied paths; one that escapes its intended directory or reads
  something the operator did not expose is a vulnerability.
- **Path traversal in a request path.** A tool argument placed into the path of
  a request to GitLab that, with a separator or a dot segment, makes the
  request name a different GitLab endpoint from the one its action names. It
  is a vulnerability even though the request still carries the user's own token,
  as [GHSA-rxjw-584q-hxjr](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/GHSA-rxjw-584q-hxjr)
  was.
- **Bypassing a per-action control.** The tool exclusion list
  (`GITLAB_MCP_EXCLUDE_TOOLS`), the tier filter, the chosen tool surface and the
  confirmation a destructive action asks for are enforcement, like the read-only
  and safe modes. A call that reaches a GitLab endpoint past any of them, or
  runs a destructive action without its confirmation, is a vulnerability.
- **Server-side request forgery.** A caller-controlled value that redirects an
  API request away from the configured `GITLAB_URL` to an internal address.

**Assumptions we make.** The operator is trusted and chooses the token and its
scope; the GitLab instance enforces its own authorization, and this server never
tries to widen what the token already allows; the host running the server is not
already compromised. Reports resting on breaking one of those assumptions fall
under *Out of scope* below.

**Security review.** The October 2026 security review of these requirements
and boundaries is recorded in
[Security Review, October 2026](docs/development/security-review-2026-10.md):
what was checked against the code, with which tools, what was found, and where
each finding is tracked.

**Assurance case.** The argument that these requirements are met, from the
boundaries above, the secure design principles the server applies and the
common implementation weaknesses it counters, each with its evidence and with
what does not hold yet, is the
[assurance case](https://docs.jmrp.io/gitlab-mcp-server/operations/assurance-case/).

## Scope

### In scope

- The `gitlab-mcp-server` source code in this repository (Go server, MCP tools, transports, prompts, resources).
- Authentication and authorization handling (token storage, OAuth flows, HTTP session isolation).
- Input validation in MCP tool handlers.
- Path handling for file uploads and downloads.
- TLS configuration handling and `GITLAB_MCP_SKIP_TLS_VERIFY` semantics.
- Error messages and logs that could leak credentials or sensitive metadata.
- Released binaries and Docker images published from this repository.

### Out of scope

- Vulnerabilities in the **GitLab server** itself — please report those to [GitLab](https://about.gitlab.com/security/disclosure/).
- Vulnerabilities in **upstream dependencies** that have already been disclosed and patched upstream — open a regular issue or PR to bump the dependency instead.
- Misconfigurations of the operator's environment (leaked PATs, world-readable `.env`, exposed HTTP port without authentication, etc.) that are explicitly warned against in the documentation.
- Issues that require the attacker to already control the host running the server (kernel exploits, container escapes, side-channel attacks on memory).
- Denial-of-service via legitimate but expensive GitLab API queries (rate limiting is the operator's responsibility).
- Findings from automated scanners without a demonstrated impact (please include a working PoC).

## Supported Versions

Security fixes are issued for the latest stable release line on `main`. Older releases do not receive backports.

| Version                | Supported           |
| ---------------------- | ------------------- |
| Latest `3.x` release   | :white_check_mark:  |
| Older `3.x` releases   | :x: (please update) |
| `2.x`, `1.x` and `0.x` | :x:                 |

We strongly recommend running the most recent release. Updates arrive through whichever channel you installed from (npm, Homebrew, the container image, the Claude Desktop extension, winget, or a fresh download); the server never replaces its own binary.

## Coordinated Disclosure

We follow a **coordinated disclosure** model aligned with [ISO/IEC 29147](https://www.iso.org/standard/72311.html) and the [OWASP Vulnerability Disclosure Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Vulnerability_Disclosure_Cheat_Sheet.html):

1. You report privately, through GitHub's private vulnerability reporting or by email, as [above](#reporting-a-vulnerability).
2. We acknowledge the report, triage it, and confirm the issue.
3. We develop and test the fix while the report itself stays private, in one of two ways. Either in pull requests of this repository, which run the same CI as any other change: the fix of GHSA-fcj2-hj27-hj26 was developed that way, in [#479](https://github.com/jmrplens/gitlab-mcp-server/pull/479) and [#645](https://github.com/jmrplens/gitlab-mcp-server/pull/645). Or in the advisory's temporary private fork, which is merged into `main` from the advisory: GitHub runs no CI and no status checks in such a fork, so the repository's CI first runs on the fix once it is on `main`. The fix of GHSA-rxjw-584q-hxjr was developed that way and merged as [`39b7093af`](https://github.com/jmrplens/gitlab-mcp-server/commit/39b7093afffede232184c1e385b1ef4a215b0258).
4. We release the fix, and request a CVE through GitHub when applicable.
5. Once the release that carries the fix is published, we publish the GitHub Security Advisory with the full technical details, crediting you as you choose. That can be the same day: GHSA-rxjw-584q-hxjr was published on 31 October 2026, the day 3.4.0 was released, once the release workflow had finished, with its fix already public on `main`. GHSA-fcj2-hj27-hj26 was published four days after 3.0.0.

If a vulnerability is being actively exploited in the wild, we may publish the advisory sooner to protect users.

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
- **A credential does not leave the instance it was issued for.** A redirect that
  crosses hosts, or downgrades https to http, loses `PRIVATE-TOKEN`,
  `Authorization`, `Sudo`, `Job-Token` and `Deploy-Token` before the next hop,
  and on every hop after it, a hop back onto the instance included.
  Go's own client strips `Authorization` and cannot strip GitLab's header, and
  no adversary is needed to reach this: GitLab answers artifact, trace and
  package downloads with a redirect to object storage whenever object storage
  is configured. Hosts are compared with ASCII letters folded and nothing else,
  so a name that matches the instance only once a character outside ASCII is
  lowercased (a dotted capital `İ` becomes `i`) is a different host, as it is
  to the dialer. The OAuth verifier refuses such a hop outright rather than
  following it stripped, because there the response body is the caller's
  identity rather than an asset.
- **HTTP mode names the instances it serves.** It refuses to start without
  `--gitlab-url`, because a deployment that names none makes requests to
  whatever host a caller puts in a header, with whatever token that caller
  supplied, and returns the answer to them. `--allow-any-gitlab-url` accepts
  that for a single-user local deployment, and only there: it is refused unless
  `--http-addr` binds a loopback address or a unix socket, because on a
  listener anyone else can reach it is that same request forgery, opted into.
  Where several instances are published the header is required, since choosing
  for the caller would send their token to an instance they never named.
- The `.env` file containing credentials is excluded from version control via `.gitignore`.

### File System Access (caller-supplied local paths)

Eleven actions take a path on the machine running the server: the project and
wiki uploads, secure files, alert metric images, the three avatars, package
publish and publish-directory, the import archives, and `package.download`,
which writes rather than reads.

- **Paths are confined, not merely cleaned.** Every one of them resolves
  symlinks and must land inside the working directory, the operating system
  temporary directory, or a directory the operator named. A path that resolves
  outside those roots is refused. `package.download` resolves its destination
  again after creating parent directories, so a symlink planted underneath a
  directory the call just made cannot decide where the bytes go.
- **A download never writes through a link planted at its destination.**
  `package.download` writes to a temporary file it creates exclusively
  beside the file `output_path` resolves to, under a random name, and
  renames it over that file once the whole body has arrived. When
  `output_path` is already a link to a regular file inside the roots, that
  file is what gets replaced and the link stays. A rename replaces the name
  rather than following it, so a link planted after the check is replaced
  instead of written through, on Windows as well, where Go offers no
  `O_NOFOLLOW` and opening a path that already exists follows a link planted
  there. A download that fails or is cancelled removes its temporary file
  and leaves `output_path` as it was, so nothing that looks like a download
  is left for the next reader to trust.
- **The operator widens the roots, the caller never does.**
  `GITLAB_MCP_ALLOWED_UPLOAD_DIRS` for reads, `GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS`
  for writes, and `GITLAB_MCP_ALLOWED_IMPORT_DIRS` for import archives.
- **The working directory is dropped from the roots when it is the filesystem
  root or the user's home directory.** A stdio server usually starts in the
  workspace somebody just opened, which is why it is a root at all. Some clients
  start their servers in `/`, where keeping it would allow-list the whole disk
  and undo the confinement without saying so. The home directory is the same
  argument one level down: it holds `~/.ssh`, `~/.aws`, the browser profiles and
  this server's own `~/.gitlab-mcp-server.env`, so allow-listing it implicitly
  would put the `GITLAB_TOKEN` the confinement exists to protect within reach of
  a `file_path`. Both are dropped as *implicit* roots only. An operator whose
  workspace really is their home directory names it in one of the variables
  above and gets it back, and the server logs a warning naming that remedy the
  first time it resolves a caller-supplied path there.
- **A local path may not be named at all over HTTP.** This is a property of the
  transport rather than a setting. A stdio server shares a machine with the
  person driving it and `file_path` is the point of it; a caller reached over
  HTTP has no files here, so every path they could name belongs to somebody
  else. Such a request is refused with an error naming `content_base64`, which
  is what a remote caller can actually supply.
- **Reads are bounded while reading**, not by the size the filesystem reported
  beforehand. A procfs entry is a regular file that reports zero bytes and then
  returns many, which is how this process's own environment could be streamed
  past a limit it had already satisfied.

`file_path` is caller-supplied input. Treating it as trusted was the earlier
position here and it was wrong even for stdio, because the argument arrives
from a model that may have read attacker-influenced content.

### TLS Configuration

- TLS certificate verification is enabled by default.
- Self-signed certificates can be accepted via `GITLAB_MCP_SKIP_TLS_VERIFY=true`.
- This setting should **only** be used in trusted internal networks.

### Read-Only and Safe Modes

- Set `GITLAB_MCP_READ_ONLY=true` to remove every mutating action (create, update, delete) from the served surface, on every surface; reads (list, get, search) keep working.
- Set `GITLAB_MCP_SAFE_MODE=true` to have a mutating action return a preview card naming the action and echoing its arguments instead of running the change; reads keep working.
- Both flags provide additional protection for sensitive GitLab instances.

### Input Validation

- All tool inputs are validated before GitLab API calls.
- Required parameters are checked explicitly — missing values produce clear error messages.
- Integer IDs are validated to prevent injection.
- String parameters are sanitized where applicable.
- A tool argument placed into a request path is refused before any request is
  built if it carries a `.` or `..` segment, and is escaped so that a separator
  stays inside it; the GitLab transport also refuses any request whose path
  carries a dot segment, as it stands or once decoded (from 3.4.0).

### Error Handling

- Error messages never expose internal server details or stack traces.
- API errors from GitLab are wrapped with context but sensitive headers are stripped.
- Authentication failures return generic messages without revealing credential details.

### Dependencies

- Minimal dependency footprint.
- Dependencies are tracked in `go.sum` with cryptographic checksums.
- Regular dependency updates are performed to address known vulnerabilities (`govulncheck` runs in CI).
- The release binaries are also checked at module grain, the way scanners that read a binary or an image SBOM check them: every target the release configuration names is built from the tree and scanned, and an advisory against any module a binary links fails the check unless a reviewed declaration accepts it (`make check-binary-vulns`). CI runs it on every pull request and every push to main, and the release workflow runs it again on the tagged tree before anything is published.
- The container image's Alpine packages are held to Trivy's vulnerability database before the image is pushed: the release workflow scans both platforms after the smoke test, and an advisory with a fixed package rated HIGH or CRITICAL stops the release unless a reviewed declaration accepts it (`make check-image-vulns`). A weekly run asks the same of the published `latest` image.
- Automated dependency scanning via Dependabot.

### Release Integrity

- Release binaries are built from tagged commits and published via GoReleaser.
- Checksums (`checksums.txt`) and a Cosign/Sigstore signature bundle (`checksums.txt.sigstore.json`) are attached to every GitHub Release. Verify with [`cosign`](https://docs.sigstore.dev/cosign/installation/) using the keyless GitHub OIDC identity of this repository.
- Every release tag from v2.7.0 on is an annotated tag signed with the maintainer's OpenPGP key, and no tag on the repository can be moved or deleted. [Verify a release tag](https://docs.jmrp.io/gitlab-mcp-server/operations/security/#verify-a-release-tag) shows how to check one with `git verify-tag`.
- Some release-integrity controls are GitHub repository settings rather than files, so no commit or CI gate can apply them. They are tracked as a maintainer checklist in [Repository Settings](docs/development/repository-settings.md).

## Security Best Practices for Deployment

1. **Use a dedicated GitLab token** with minimal required scopes (prefer `read_api` for read-only use cases).
2. **Run the server as a non-privileged user** — avoid root/administrator.
3. **Enable TLS** between the MCP server and GitLab instance in production.
4. **Keep the `.env` file permissions restrictive** (`chmod 600` on Unix systems).
5. **Run the server with least privilege** so `file_path` uploads cannot read sensitive files (or use `content_base64`).
6. **Use read-only or safe mode** (`GITLAB_MCP_READ_ONLY=true` or `GITLAB_MCP_SAFE_MODE=true`) when mutation is not needed or must be reviewed.
7. **Monitor token usage** via GitLab's admin panel.
8. **Rotate tokens periodically** according to your organization's policy.
9. **In HTTP mode**, restrict network access to trusted clients only and consider running behind a TLS-terminating reverse proxy.
10. **Keep `gitlab-mcp-server` updated** by updating through your install channel, or subscribe to repository releases.
