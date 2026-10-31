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

How a dependency is chosen, obtained and kept current is in [CONTRIBUTING.md](CONTRIBUTING.md#dependencies). This section is what a finding against one sets off: a known vulnerability, or a license outside the ones accepted below.

#### Vulnerabilities

- `govulncheck` runs in the required `✅ CI` check on every pull request and every push to main, and fails when the project's code calls a vulnerable symbol. It accepts no advisory, so such a finding is fixed before anything else merges, by updating the module or by no longer calling the symbol.
- The release binaries are also checked at module grain, the way scanners that read a binary or an image SBOM check them: every target the release configuration names is built from the tree and scanned, and an advisory against any module a binary links fails the check unless a reviewed declaration accepts it (`make check-binary-vulns`). CI runs it on every pull request and every push to main, and the release workflow runs it again on the tagged tree before anything is published.
- The container image's Alpine packages are held to Trivy's vulnerability database before the image is pushed: the release workflow scans both platforms after the smoke test, and an advisory with a fixed package rated HIGH or CRITICAL stops the release unless a reviewed declaration accepts it (`make check-image-vulns`). A finding with no fix to take, or rated lower, is printed by the scan and does not stop it. A weekly run asks the same of the published `latest` image.
- Every lockfile the repository holds is checked against known vulnerabilities and malicious packages on every pull request and every push to main, as the policy below says (`make check-dependency-vulns`).
- Dependabot security updates are on: an advisory against a dependency opens a pull request that waits for no cooldown.

So the threshold is any severity for a Go module, fixed before the next merge and again checked before a release; any severity for a package a lockfile holds, the documentation site's included, before the merge; and HIGH or CRITICAL with a fix for the image's Alpine packages, fixed before the release. A finding of `make check-binary-vulns` or `make check-image-vulns` can instead be accepted by a reviewed declaration, in the `declarations.go` of `cmd/audit_binary_vulns` or `cmd/audit_image_vulns`, which gives one of the reasons that command defines, code the binaries do not link or the image does not run, or a fix that exists and cannot be taken yet, with the words that back it; a finding of `make check-dependency-vulns` only by an entry in the `osv-scanner.toml` beside its lockfile, as the policy below says; and a finding of `govulncheck` cannot.

#### Malicious and vulnerable dependencies

Every change is evaluated against this policy before it merges. The `🦠 Dependency advisories` job of CI applies it, and `✅ CI`, the one check branch protection requires, fails when that job does, so a violation blocks the merge; `make check-supply-chain` fails if the job ever stops counting toward that check. The repository admin role can bypass the branch rules when it merges a pull request. This policy does not allow that bypass while `🦠 Dependency advisories` is red: an advisory is set aside only by an entry in an `osv-scanner.toml`, as below. The one commit that reaches `main` without a pull request is the release workflow's version stamp, which writes the version and digests of this project's own packages into its manifests and adds no dependency; the push to `main` evaluates it like any other.

- **What is evaluated.** OSV-Scanner, its image pinned by digest, reads every lockfile in the tree at module grain: every `go.mod`, which from Go 1.17 lists every module a build needs (`go.sum` holds checksums and adds no module to a build), `site/pnpm-lock.yaml`, the pip locks under `.github/requirements`, and any lockfile added later that OSV-Scanner recognizes. The check fails if a scan did not read a lockfile git holds. It asks [OSV.dev](https://osv.dev), which carries the Go vulnerability database, GitHub's advisories for npm and PyPI, and the OpenSSF malicious-packages feed (`MAL-` entries). Direct and transitive dependencies count alike, the development tooling the lockfiles hold included. Two tools are installed by exact version outside a lockfile and are not evaluated here: the `@anthropic-ai/mcpb` CLI that `make check-mcpb` runs through `npx`, and the `npm` the release installs to publish with.
- **What has no lockfile.** `npm/gitlab-mcp-server/package.json` names only this project's own six platform packages, at the release version. The `pyproject.toml` files under `test/fixtures/orbit` declare no runtime dependency; their test extra and build requirements are installed only by the fixture projects' own pipelines on GitLab.com, never by this repository's CI or a release. `scripts/build_pypi.py` and `scripts/build_nuget.py` use the Python standard library alone, and the packages they build declare no dependency.
- **What blocks.** A known vulnerability of any severity that no declaration sets aside. A malicious package always blocks and is never declared: an advisory with a `MAL-` id, or one GitHub classifies as embedded malware (CWE-506), fails a second scan that honors no `osv-scanner.toml`, whichever of its ids an entry names, since an entry naming any alias of an advisory sets them all aside. The package is removed and the lockfile regenerated before the change merges. A scan that reaches no verdict, because OSV.dev did not answer or a lockfile could not be read, blocks too, and is run again.
- **Fix first.** A fixed version the project can take is taken rather than declared: a module bump, a Dependabot pull request, or an override in `site/pnpm-workspace.yaml` with the advisory and its reason beside it.
- **What may be declared not exploitable, and where.** Only an advisory one of the OpenVEX `not_affected` justifications holds for: the component or its vulnerable code is not present, the code is not in the execute path, no adversary can control the input that reaches it, or a mitigation already stands in front of it. It is declared in the `osv-scanner.toml` beside the lockfile it is found in, one `[[IgnoredVulns]]` table per advisory, with its `id`, a `reason` that opens with the justification and says why it holds in words a reviewer can check, and `ignoreUntil`, the review date. OSV-Scanner and OpenSSF Scorecard read the same file. An advisory against a Go module is also stated, with the same justification, in `vex/gitlab-mcp-server.openvex.json`, the OpenVEX document every release attaches for a scanner given a release to read; `make check-vex` fails on an advisory the source scan reports, without a call of its function, that the document does not state.
- **By whom.** A declaration is added, renewed or removed by a pull request, which the maintainer reviews like any other change. `make check-supply-chain` refuses an entry without a justification, a reason or a review date, one dated more than six months ahead, one for a `MAL-` advisory, and anything else written in the file.
- **For how long.** Six months at most at a time. OSV-Scanner stops honoring an entry on its `ignoreUntil` date, so the advisory blocks again until the entry is reviewed and renewed or the dependency is fixed, and an entry that no longer sets anything aside fails the check until it is removed.

The source scan (`govulncheck`), the release binaries and the image's Alpine packages are held by the checks listed above; the Alpine packages are evaluated before each release pushes the image and every week, not on each change. The site's `pnpm audit` job reports npm's own advisories for the site's lockfile and does not decide.

#### Licenses

The release binaries link the Go standard library and modules under these licenses, as the SPDX SBOMs of 3.4.0's seven binaries name them (read on 31 October 2026), and these are the licenses a module a release binary or the image's binary links may carry:

- **MIT, MIT-0, BSD-3-Clause and Apache-2.0.** What they ask of a binary distribution, their license and notice texts, travels with every binary in `THIRD_PARTY_NOTICES`, which every package and the image carry.
- **MPL-2.0**, for `github.com/hashicorp/go-cleanhttp` and `github.com/hashicorp/go-retryablehttp`. Its terms bind the files of those modules rather than the program that links them, and the modules are linked as published, unmodified, with their source public at the version `go.mod` requires. A module under MPL-2.0 is accepted on that condition: linked unmodified.
- **CC-BY-4.0** is named in an SBOM for `github.com/modelcontextprotocol/go-sdk` because that module's `LICENSE` puts its documentation under it. The code a binary links from it is Apache-2.0 or MIT.

An SBOM also names `LicenseRef-` entries for client-go, grpc-gateway and this repository's own module. They are files whose names contain `license` or `notices` that the scanner took for license texts, and they carry no license: client-go's `license.go` and `license_templates.go`, its clients of GitLab's license APIs, with their tests and mocks; grpc-gateway's test fixture `license_missing_name.prototext`; and in this module the license tools and their tests, the end-to-end license tests and fixture, the model corpus's `licensed_*.go` cases, and `cmd/gen_third_party_notices/notices.go`, which writes `THIRD_PARTY_NOTICES`, with its test.

A module under any other license, or under none, is not merged. The pull request that adds the module, or updates it to a version under another license, says so, and it is merged only once the module is replaced or the maintainer accepts the license by adding it to the list above in that pull request, with the reason. The threshold is the same for every license outside the list: before the merge. No check fails on a license today, so the review of each pull request that changes `go.mod` is where this is held; what does fail is `cmd/gen_third_party_notices`, which stops the release when a module a binary links publishes no license file at all. Modules only the tests and the repository's commands use reach no release, and are held to the list only when a release binary starts linking them.

### Static analysis findings

Three static analysers read the code, and a finding is handled by the one that raised it:

- **golangci-lint** runs in the required `✅ CI` check on every pull request and every push to main, with `gosec` in audit mode, `staticcheck` with all of its checks, `govet` and the other linters `.golangci.yml` enables. A finding of any severity fails the check, so it is fixed in the pull request that brings it, before the merge. A false positive is suppressed on its own line with its reason, either `//nolint:<linter> // <reason>`, which `nolintlint` refuses without both, or gosec's `// #nosec <rule> -- <reason>`; every `#nosec` in the tree names its rule and its reason, and the 31 in the server's own code are read in [the October 2026 security review](docs/development/security-review-2026-10.md#method).
- **CodeQL** analyses the Go code, the JavaScript and TypeScript and the workflows on every pull request, every push to main and once a week, and reports to code scanning without failing the required check. An alert of any severity is fixed, or dismissed with the reason it does not apply, before the next release is tagged. Read on 31 October 2026, no CodeQL alert is open; of the 21 it has raised, 17 were fixed and 4 dismissed as false positives.
- **SonarCloud** analyses the Go code on every pull request and every push to main, and reports its quality gate beside the required check rather than in it. A vulnerability it reports is fixed, and a security hotspot it raises is fixed or reviewed with its reason, before the next release is tagged. Read on 31 October 2026, the quality gate passes and no vulnerability and no hotspot is open.

`.golangci.yml` also excludes some of gosec's rules across a scope rather than on a line, each for a reason that holds across that scope:

- `G104`, an unhandled error, everywhere, since `errcheck` reports the same finding.
- `G101`, a hard-coded credential, in test files, whose fixtures carry invented tokens on purpose, and in the `action_specs.go` files, whose constants are action IDs.
- `G103`, `G204` and `G304`, the use of `unsafe`, a subprocess started with a variable and a file opened at a variable path, everywhere, through the linter's `common-false-positives` preset. In the server, `unsafe` serves twice in `internal/toolutil`, to read an address as an identity, of a handler's closure and of a schema, and nothing is written through either; the server starts no subprocess; and a path a tool's caller supplies is opened only after the confinement [File System Access](#file-system-access-caller-supplied-local-paths) describes, which gosec cannot follow, the other paths it opens being the operator's (a dotenv file, a certificate). The rest are the repository's commands under `cmd/`, which run `go` and `git` and read the tree they are run in.

The `comments` and `std-error-handling` presets beside it silence rules no security property rests on: missing or misshapen doc comments, which `make check-godocs` judges instead, and the unchecked error of a close, a flush, a print, a removal or a write to the standard streams.

`sonar-project.properties` sets aside three of SonarCloud's security rules in named files, each with its reason written beside it, so no vulnerability or hotspot is reported for them there:

- `S4830` and `S5527`, a TLS connection that verifies neither the certificate nor the host name, in `internal/gitlab/client.go`, whose one such configuration is used only when the operator sets `GITLAB_MCP_SKIP_TLS_VERIFY` for an instance with a self-signed certificate.
- `S4036`, a command found through `PATH`, in `cmd/bench_resources` and `cmd/audit_e2e_coverage`, maintainer tooling that reaches no binary and runs where the same `PATH` has already supplied the compiler.
- `S1313`, a hard-coded IP address, in `internal/gitlab/destination.go`, whose addresses are the cloud metadata endpoints and the RFC 6598 shared address range that the outbound destination guard refuses, not anywhere it connects.

The file's other exclusions concern duplication, complexity, naming, empty test handlers, `TODO` comments, coverage and files that are not code, and set aside no security rule.

To see what is open:

```bash
gh api 'repos/jmrplens/gitlab-mcp-server/code-scanning/alerts?tool_name=CodeQL&state=open' --jq length
curl -s 'https://sonarcloud.io/api/issues/search?componentKeys=jmrplens_gitlab-mcp-server&types=VULNERABILITY&resolved=false' | jq .total
curl -s 'https://sonarcloud.io/api/hotspots/search?projectKey=jmrplens_gitlab-mcp-server&status=TO_REVIEW' | jq .paging.total
```

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
