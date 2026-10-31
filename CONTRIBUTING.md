# Contributing to gitlab-mcp-server

Thank you for your interest in contributing to gitlab-mcp-server! This guide covers the process for submitting changes, reporting issues, and following project conventions.

By participating, you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).
For security issues, please follow the [Security Policy](SECURITY.md) instead of opening a public issue.
How the project makes decisions, who holds which role and what it depends on to continue are in [GOVERNANCE.md](GOVERNANCE.md), and where it is going is on the [roadmap](https://docs.jmrp.io/gitlab-mcp-server/roadmap/).

## Table of Contents

- [Getting Started](#getting-started)
- [Finding a First Task](#finding-a-first-task)
- [Development Setup](#development-setup)
- [Branch Naming](#branch-naming)
- [Commit Messages](#commit-messages)
- [Developer Certificate of Origin](#developer-certificate-of-origin)
- [Pull Requests](#pull-requests)
- [Code Review](#code-review)
- [Code Standards](#code-standards)
- [Dependencies](#dependencies)
- [Testing](#testing)
- [Documentation](#documentation)
- [Release Process](#release-process)
- [Issue Reporting](#issue-reporting)
- [Labels](#labels)

## Getting Started

1. Clone the repository
2. Run `make install-tools` to install the pinned `golangci-lint`, `govulncheck` and `gotestsum`; the Go module dependencies are fetched by the first build
3. To run the end-to-end suite against your own GitLab, create a `.env` at the repository root with `GITLAB_URL` and `GITLAB_TOKEN`: `make test-e2e` reads it. The server itself never loads a `.env` from its working directory (see [Configuration](https://docs.jmrp.io/gitlab-mcp-server/configuration/) for how it is configured)
4. Run `make build` to verify the setup
5. Run `make test` to ensure all tests pass

## Finding a First Task

Issues small enough for a first contribution carry the [`good first issue`](https://github.com/jmrplens/gitlab-mcp-server/labels/good%20first%20issue) label. The label goes on an issue whose change stays in one place in the code or the documentation, needs no GitLab license to test, and is not part of work already under way, and the issue says what done looks like. Larger issues anyone is welcome to take carry [`help wanted`](https://github.com/jmrplens/gitlab-mcp-server/labels/help%20wanted).

Say in the issue that you are taking it, so nobody else starts the same work, and ask there about anything the issue leaves unclear. When you open the pull request, link the issue with `Closes #N`.

## Development Setup

### Prerequisites

- **Go 1.27+** — [Download](https://go.dev/dl/)
- **Node.js 24.18+ with Corepack**, required for the documentation site and MCP Inspector. The `packageManager` field in `site/package.json` names the pnpm version (`pnpm@12.16.1` today) and is authoritative; pnpm settings live in `site/pnpm-workspace.yaml`.
- **Git** — configured with push access
- **GitLab instance** — with a Personal Access Token (`api` scope)

### Build and Test

```bash
# Install the pinned linters and test runner (once, and after the Makefile pins move)
make install-tools

# Install the documentation site's dependencies (the site's checks run from site/)
(cd site && pnpm install --frozen-lockfile)

# Build
make build

# Run all tests
make test

# Run tests with race detector
make test-race

# Run end-to-end tests (requires .env with real GitLab credentials)
make test-e2e

# Run end-to-end tests against an ephemeral GitLab CE with a runner and fixtures in Docker
# (GitLab peaks at about 7.5 GiB, capped at 12 GiB)
make test-e2e-ce

# Check test coverage
make coverage

# Lint
make lint

# Launch MCP Inspector (interactive tool testing UI)
make inspector

# Stop MCP Inspector
make inspector-stop
```

## Branch Naming

Use the following naming convention for branches:

| Prefix      | Purpose                 | Example                       |
| ----------- | ----------------------- | ----------------------------- |
| `feature/`  | New functionality       | `feature/gitlab-wiki-tools`   |
| `fix/`      | Bug fixes               | `fix/pagination-off-by-one`   |
| `docs/`     | Documentation only      | `docs/add-wiki-reference`     |
| `test/`     | Test additions          | `test/increase-mr-coverage`   |
| `refactor/` | Code restructuring      | `refactor/extract-pagination` |
| `chore/`    | Build, CI, dependencies | `chore/upgrade-go-sdk`        |

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type       | Description                                             |
| ---------- | ------------------------------------------------------- |
| `feat`     | New feature or tool                                     |
| `fix`      | Bug fix                                                 |
| `docs`     | Documentation changes                                   |
| `test`     | Adding or updating tests                                |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `chore`    | Build process, CI, dependency updates                   |
| `perf`     | Performance improvement                                 |
| `style`    | Code formatting (no logic change)                       |

### Scopes

Use the package name as scope when applicable:

```text
feat(tools): add gitlab_wiki_page_create tool
fix(config): handle empty GITLAB_URL gracefully
test(branches): increase coverage to 90%
docs(site): describe the wiki tools in English and Spanish
```

## Developer Certificate of Origin

Every commit in a pull request is signed off: its message ends with a `Signed-off-by:` trailer naming the commit's author, with the name and email the commit is authored under. The sign-off is how the author certifies the [Developer Certificate of Origin](https://developercertificate.org/) 1.1 for that commit, which in short says that the contribution is theirs to submit under the project's [MIT license](LICENSE): they wrote it, or it is based on earlier work under a compatible open source license that they may submit with their changes, or it was given to them by somebody who certified one of those and they have not changed it; and that the contribution and its sign-off are public and kept, and may be redistributed under the license. The certificate's own text is the one that applies.

```bash
git commit -s -m "fix(config): handle empty GITLAB_URL gracefully"
```

`-s` (`--signoff`) writes the trailer from your git identity, so it names you when you are the author:

```text
Signed-off-by: Jane Doe <jane@example.org>
```

To sign off commits already on your branch where they stand, rebase it on its own base and push it again: `git rebase --keep-base --signoff origin/main`, then `git push --force-with-lease`. A commit somebody else authored is signed off by that person. A sign-off is not a signature: signing commits with a GPG or SSH key is welcome, and it does not replace the trailer.

`make check-dco` judges the commits of your branch against `origin/main` before you push, and the `✍️ DCO sign-off` job of CI judges every pull request the same way, from the base commit its event names to its head; for a stacked pull request that base is the head of the layer below, so each layer's pull request judges that layer's commits. It reads the trailer the way git does, and passes a commit when one of its `Signed-off-by` trailers names its author, the name and the email compared without regard to case. A merge commit that brings no change of its own, such as the one GitHub's Update branch button writes, is not judged; a merge that resolves a conflict or changes anything while merging is judged like any other commit, so conclude it with `git commit -s`. Dependabot's commits are exempt in the pull requests Dependabot opens, because an app cannot certify anything and what it commits is a dependency bump; the same commit in a pull request somebody else opened is judged like any other. The release workflow's version stamp reaches `main` without a pull request, so nothing judges it. A fix developed in a security advisory's temporary private fork reaches `main` from a pull request of that fork, merged from the advisory, and GitHub runs no check on it there ([SECURITY.md](SECURITY.md#coordinated-disclosure)), so nothing in CI judges it either: run `make check-dco` on the fork's branch before the advisory is merged. The [command's reference](docs/development/cmd-utilities.md#audit_dco) says what else it refuses.

GitHub signs off a commit made in its web interface, such as an applied review suggestion, because the repository requires sign-off for web commits ([Repository Settings](docs/development/repository-settings.md#web-commit-sign-off)), so such a commit needs no trailer typed into its message. The squash commit that lands on `main` is not judged: the `✍️ DCO sign-off` job judges every commit of the pull request, and those are what you certify, so the description carries no trailer of its own. A squash merged with GitHub's default message keeps the `Signed-off-by` lines of the pull request's commits below the description, while a message written by hand at the merge carries only the trailers it holds, so it should end with those lines.

## Pull Requests

### Before Submitting

- [ ] Code compiles: `go build ./...`
- [ ] All tests pass: `go test ./... -count=1`
- [ ] Static analysis is clean: `make analyze` (run `make analyze-fix` first to apply supported Go and Markdown fixes)
- [ ] New tools have tests, and total coverage stays at or above the 90% CI enforces
- [ ] Every new source file opens with the [copyright and license header](#copyright-and-license-headers): `make check-file-headers`, or `make fix-file-headers` to write it
- [ ] Documentation is updated where the change shows: the site page for user-facing behavior (English and Spanish), `docs/development/` for contributor-facing changes (see [Documentation](#documentation))
- [ ] Commit messages follow conventional commits
- [ ] Every commit is signed off by its author (`git commit -s`), which certifies the [Developer Certificate of Origin](#developer-certificate-of-origin); `make check-dco` says which are not

### PR Process

1. Create a feature branch from `main`
2. Make your changes in small, focused commits
3. Push the branch and open a pull request; [CODEOWNERS](CODEOWNERS) requests the maintainer's review
4. Fill in the PR template (auto-populated from `.github/pull_request_template.md`)
5. Answer every review thread, with a change or the reason for none, as [Code Review](#code-review) describes
6. The maintainer squash-merges the pull request once it meets [Code Review](#code-review); squash is the only merge method the repository allows. The squash commit's message is the pull request description, so the description carries no skip command GitHub reads (a bracketed skip ci or one of the four other commands GitHub documents, or a skip-checks trailer), which would turn off every workflow on main for that commit; `make check-pr-description` refuses one

### PR Size Guidelines

- **Small** (<200 lines): Preferred — faster review, fewer conflicts
- **Medium** (200–500 lines): Acceptable for feature additions
- **Large** (>500 lines): Split into smaller PRs when possible

## Code Review

Every change reaches `main` through a pull request, except the version stamp the release workflow pushes after a release ([GOVERNANCE.md](GOVERNANCE.md#roles-and-responsibilities) says how), and every pull request is reviewed before it is merged. This section is the standard the review holds a change to; [GOVERNANCE.md](GOVERNANCE.md) says who holds the role.

### How a review is conducted

- **Who reviews.** The maintainer reviews every pull request a contributor opens, and is the one who merges. Automated reviewers also comment on most pull requests. Their threads are read like a person's: each one is answered with a change, or with the reason the change is not made, and then resolved. The maintainer's own pull requests go through the same checks and automated reviews; until a second maintainer joins, no person other than the author reviews them.
- **Where.** In the pull request's review threads, on the lines they concern. A question is answered in its thread rather than in a new comment, and a thread is resolved once it is answered.
- **What the reviewer reads.** The diff, the description's account of what changed and how it was checked, and the checks' results. The description has to say enough to judge the change without running it: what was wrong or missing, what the change does about it, and what was run to show it works.

### What is checked

- **Correctness.** The change does what its description says, against what GitLab documents and sends, including its errors, its empty answers and the paths a test does not take by default.
- **Tests.** New or changed behavior has tests in the automated suite and a fix has a regression test, as [Testing](#testing) requires, and the packages the change touches stay covered. A new catalog action has an end-to-end scenario that names it.
- **Security.** The change holds the [threat model](SECURITY.md#threat-model): no tool argument reaches beyond the token's own permissions or redirects a request to another host or path, GitLab-authored text is escaped wherever a result interpolates it, no secret reaches code, fixtures or logs, and in HTTP mode nothing one caller holds reaches another. A limit, a budget or what a setting's zero means is decided in `internal/tenancy`.
- **The published surface.** Tool and action names, input and output schemas, annotations and descriptions are deliberate, and what a model reads names only actions and tools the server serves.
- **Code standards.** The change follows [Code Standards](#code-standards), and any exception the linters allow is marked where it occurs with the reason.
- **Documentation.** A change a user can see updates its site page in English and Spanish together, a change to how the repository works updates `docs/development/`, and every generated artifact is regenerated rather than edited (see [Documentation](#documentation)).
- **History.** Commit messages follow [Commit Messages](#commit-messages), every commit is signed off by its author as [Developer Certificate of Origin](#developer-certificate-of-origin) requires, and the description, which becomes the squash commit's message, carries no skip command and no block a review bot injected.

### What a change needs to be accepted

- The required `✅ CI` check passes. It aggregates the build, the tests on Linux, macOS and Windows, the 90% coverage floor, the linters and the repository's own gates, `make check-pr-description` and the sign-off check among them.
- Every review thread is resolved. The branch rules refuse the merge otherwise, except to the repository admin role, whose place on their bypass list lets it pass over them only when merging a pull request, never on a push to `main`; the maintainer, who holds that role, merges a pull request only once this item and the one above hold.
- A contributor's pull request completes the template's checklist, with any item that does not apply marked so. The maintainer's own pull requests carry the same information in the description's account of what was run.
- The maintainer agrees the change belongs in the project. A correct change can still be declined when it falls outside the [roadmap](https://docs.jmrp.io/gitlab-mcp-server/roadmap/) or a recorded decision; the reason is given in the pull request.

The branch rules require no approval count while the project has one maintainer, so acceptance is the maintainer's merge itself, made only when everything above holds.

## Code Standards

### Style Guides

Contributions are expected to follow these guides. The tools listed beside each enforce them in CI, and an exception is rare and marked at its location with the reason, or, where it covers a whole scope, written into the tool's configuration beside its reason; [SECURITY.md](SECURITY.md#static-analysis-findings) lists the ones that touch a security rule and says what happens to a static analysis finding.

| Language                                   | Style guide                                                                                                                                                                               | Enforced by                                                                                                                                     |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Go                                         | [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) and [Google's Go Style Guide](https://google.github.io/styleguide/go/) | `golangci-lint` with the linters and formatters in `.golangci.yml`; `nolintlint` requires every `//nolint` to name its linter and give a reason |
| TypeScript, JavaScript and Astro (`site/`) | The rules of `site/eslint.config.mjs` and the formatting of `site/.prettierrc`                                                                                                            | `pnpm run lint` in `site/` (ESLint and Prettier among its checks)                                                                               |
| Markdown and MDX                           | The rules of `.markdownlint-cli2.jsonc`, with pipe tables padded                                                                                                                          | `markdownlint-cli2` on every Markdown and MDX file; `go run ./cmd/format_md_tables/ --check` on `README.md`, `docs/` and the site's pages       |

`.github/instructions/go.instructions.md` turns the Go guides into the rules this repository applies, and the conventions below are the ones specific to it.

### Go Conventions

- Follow idiomatic Go and the repository's consolidated `golangci-lint` configuration (`goimports`, `gofumpt`, `gci`, `govet`, `staticcheck`, `gosec`, and related checks)
- All exported types and functions must have doc comments
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Use `context.Context` consistently for cancellation/timeouts
- Table-driven tests with `t.Run()` subtests

### MCP Tool Patterns

- Each GitLab operation = one canonical action with typed input/output structs
- Use `jsonschema` struct tags for tool input documentation
- Register runtime surfaces from the canonical action catalog
- Set appropriate annotations (readOnlyHint, destructiveHint, etc.)
- Return both structured JSON and human-readable Markdown

### File Organization

```text
internal/tools/
├── action_specs.go          # CollectActionSpecs(): aggregates every domain's ActionSpecs
├── action_catalog.go        # BuildActionCatalog(): the canonical action catalog built from them
├── register.go              # RegisterAll(): projects individual tools from the canonical catalog
├── meta_catalog.go          # RegisterMetaCatalog(): one meta-tool per catalog group
├── register_meta.go         # RegisterMetaStandaloneTools(): the standalone utilities (gitlab_discover_project and the gitlab_interactive_* flows) on the meta and individual surfaces
├── meta_tool.go             # Route wrappers and the meta parameter-schema mode
├── markdown.go              # Delegates to the type-based Markdown registry in toolutil
└── <domain>/                # 181 sub-packages
    ├── doc.go               # Package comment
    ├── action_specs.go      # Canonical ActionSpecs for catalog-backed tool surfaces
    ├── <domain>.go          # Typed input/output structs + handlers
    ├── <domain>_test.go     # Table-driven unit tests
    └── markdown.go          # Markdown formatters (self-registered via init())
```

### Copyright and License Headers

Every source file opens with these two lines, written as comments of its language:

```go
// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT
```

A shebang, an encoding declaration, a Dockerfile parser directive or a CSS `@charset` rule stays above them, and an Astro component carries them inside its frontmatter. `make fix-file-headers` writes them into a new file, `make check-file-headers` fails a file without them, and a generator that writes source writes them itself. A file somebody else wrote keeps its own notice and is not stamped with this one. [Copyright and License Headers](docs/development/file-headers.md) lists the files and where each kind carries the lines.

### Repository Tooling

Tooling this repository runs, its auditors, generators and release steps, is a Go command under `cmd/`, sharing what it needs through `cmd/internal/`, or under `test/e2e/` for the end-to-end orchestration. A Go command runs natively on every platform a contributor uses, is tested with the rest of the module, and needs no interpreter or package installed at run time in CI, so new tooling is written as one.

A script in another language is the exception, and `make check-supply-chain` holds the tree to a table of the ones allowed to exist (`declaredScripts` in `cmd/audit_supply_chain/declarations.go`), each with the reason it has to stay: an installer that runs before any binary of ours exists, the entry point a distribution hands its own runtime, Ruby GitLab evaluates, thin glue around another program's command line, run by a job where setting up Go would cost more than it saves or by a make target, or fixture content. The scripts still being ported are declared as pending, each with the item of issue 1114 that ports it. A new script fails the gate until it is declared, and the [command-line utilities reference](docs/development/cmd-utilities.md#audit_supply_chain) says what each category means.

## Dependencies

This is how the project chooses, obtains and keeps current what it depends on. What a known vulnerability in a dependency, or a license outside the accepted ones, sets off is in [SECURITY.md](SECURITY.md#dependencies).

### Choosing a dependency

A dependency is added in the pull request that needs it, which says why, and the review holds it to these points:

- **Needed.** The standard library and the modules `go.mod` already requires come first; a module is added for what they do not do.
- **The official client where one exists.** GitLab is reached through `gitlab.com/gitlab-org/api/client-go`, GitLab's own Go client, MCP through `github.com/modelcontextprotocol/go-sdk`, the protocol's official Go SDK, and telemetry through the OpenTelemetry Go SDK. An endpoint client-go has no method for is a request made through client-go's own request builder, not a second GitLab client.
- **Maintained.** It publishes releases, answers its issues and its security reports, and is neither archived nor deprecated.
- **A license the release can carry.** It is one of the licenses [SECURITY.md](SECURITY.md#licenses) accepts in what a release binary links.
- **No known vulnerability.** `govulncheck` and `make check-binary-vulns` pass with it, which the required `✅ CI` check holds anyway.

### How each kind is obtained

Each dependency is named at an exact version, and almost every one is checked against a recorded checksum when it is fetched, so a build takes the bytes that were reviewed:

| Dependency                                                                              | Where its version is recorded                                                                                | How it is fetched and checked                                                                                                                                                                                                                                    |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go modules of the server, of the commands under `cmd/` and of the tests                 | `go.mod`, the repository's one module, with each module's checksum in `go.sum`                               | `go` downloads them through the Go module proxy and refuses a module whose checksum differs from `go.sum` or from the checksum database (`sum.golang.org`); a build never edits either file                                                                      |
| Go tools (`govulncheck`, `gotestsum`)                                                   | The `tool` block of `go.mod`                                                                                 | As the modules                                                                                                                                                                                                                                                   |
| `golangci-lint`                                                                         | `GOLANGCI_LINT_VERSION` in the `Makefile`                                                                    | `make install-tools` builds that version with `go install`, which checks it as it checks a module; CI installs that version's release through the linter's own action                                                                                            |
| The documentation site's packages                                                       | `site/package.json` and `site/pnpm-lock.yaml`; pnpm itself in the `packageManager` field, with its SHA-512   | `pnpm install --frozen-lockfile`, which refuses a lockfile out of step with `package.json` and checks every package against the integrity hash the lockfile records; install scripts run only for the packages `allowBuilds` names in `site/pnpm-workspace.yaml` |
| The tools the release's PyPI job installs                                               | `.github/requirements/pypi-publish.txt`, compiled from `pypi-publish.in` with a hash for every file          | `pip install --require-hashes`                                                                                                                                                                                                                                   |
| GitHub Actions                                                                          | Each `uses:`, pinned to a full commit SHA with its version in a comment                                      | `make check-supply-chain` fails on a tag or a branch                                                                                                                                                                                                             |
| Container images                                                                        | The `Dockerfile`'s two base images, by tag and digest; an image a workflow step or an action runs, by digest | The digest pins the bytes, and `make check-supply-chain` holds the workflows to it, with the one exception it declares and gives the reason for, the image of the Scorecard action                                                                               |
| Programs a release job downloads (Syft, Trivy, ORAS, the MCP Registry publisher, komac) | A version and a SHA-256 in `release.yml`                                                                     | The job checks the download against the SHA-256 before running it                                                                                                                                                                                                |

The images the end-to-end stack starts and the GitLab the Enterprise schema check boots are the other exception, on purpose: GitLab and its runner follow the tags they name, because those runs exist to hold the server to the GitLab a user installs today, and the fixture services beside them decide nothing a scenario asserts ([`test/e2e/README.md`](test/e2e/README.md)). Nothing from them reaches a release.

### Keeping dependencies current

- **Dependabot** (`.github/dependabot.yml`) checks every Monday the Go modules, the site's packages, the PyPI job's tools, the GitHub Actions, the `Dockerfile` and the two compose files, and opens a pull request for what moved. A version is proposed only after a cooldown: 7 days, and for the Go modules, the site's packages and the PyPI tools 14 days for a new minor release and 30 for a new major one. Minor and patch updates of the Go modules and of the site's packages arrive grouped, one pull request per ecosystem (`go-minor-patch`, `npm-minor-patch`), and every update of the Actions and of the PyPI tools in one group each (`actions-all`, `pypi-publish`). Dependabot's security updates are on and wait for no cooldown. The npm launcher under `npm/gitlab-mcp-server` has no entry, since the release stamps its pins, and the OpenTelemetry collector image of the telemetry example moves by hand, with the constant its end-to-end module holds.
- **By hand.** The maintainer also raises the dependencies to their newest releases in a sweep pull request, the versions Dependabot does not manage included (the tool versions pinned in the `Makefile` and `release.yml`, the site's Node.js and pnpm). A sweep takes the newest release without waiting for the cooldown, and holds a version back only for a technical reason it states: a peer range that refuses the release, a release a rehearsal shows broken, a release withdrawn upstream. client-go is taken as soon as a release of it appears, since the workarounds [`docs/development/upstream-bugs.md`](docs/development/upstream-bugs.md) records are retired by it.
- **Tracking.** `govulncheck` over the source, `make check-binary-vulns` over every release binary and Trivy over the image's Alpine packages watch for known vulnerabilities, as [SECURITY.md](SECURITY.md#dependencies) describes, and every release publishes an SPDX SBOM of each binary and `THIRD_PARTY_NOTICES`, the license texts of every module the binaries link.

## Testing

### Policy

- **New or changed behavior must come with tests.** A pull request that adds functionality or changes what existing functionality does adds tests for it to the automated suite in the same pull request, and CI runs them on every pull request and every push to `main`. This holds for the server, the commands under `cmd/` and the scripts under `scripts/` alike.
- **A bug fix adds a regression test** that fails without the fix and passes with it, unless the defect cannot be reproduced in a test (a race in a third-party service, a platform the runners lack); the pull request then says why.
- **A new catalog action needs an end-to-end scenario** under `test/e2e/gitlab/` that names it, or a declared exemption in `cmd/audit_e2e_coverage/exemptions.go` with its reason; `go run ./cmd/audit_e2e_coverage/ -static` fails CI otherwise.
- **A test is not weakened to make a change pass.** A test that has to change because the behavior it holds changed says so in the pull request.

### Requirements

- **Unit tests** for every tool handler: use `httptest` to mock GitLab API responses
- **Table-driven tests** with `t.Run()` subtests
- **Test naming**: `TestToolName_Scenario_ExpectedResult`
- **Coverage target**: CI fails when total coverage of `./cmd/...` and `./internal/...` drops below 90% (`COVERAGE_MIN` in `.github/workflows/ci.yml`); the `increase-test-coverage` skill aims at 100% for every package a change touches
- **No external dependencies**: Unit tests must not call real GitLab APIs

### Running Tests

```bash
# All unit tests
go test ./... -count=1

# Specific package
go test ./internal/tools/... -count=1 -v

# With coverage
go test ./internal/tools/... -coverprofile=cover.out
go tool cover -func=cover.out

# E2E tests (requires real GitLab)
go test -tags e2e -p 1 -timeout 2700s ./test/e2e/gitlab/...
```

## Documentation

### AI-Assisted Development

This project ships with **7 AI agents** and **18 skills** for GitHub Copilot and compatible assistants. Key workflows for contributors:

- **Adding new tools**: Use the `create-mcp-tool` skill — it scaffolds the full tool lifecycle (struct, handler, registration, tests, docs).
- **Improving test coverage**: Use the `increase-test-coverage` skill to identify gaps and cover every package you touch.
- **Documenting a change**: Use the `update-project-documentation` skill, and `update-starlight-docs` for the site pages it leads to.
- **Code quality reviews**: Use the `review-and-refactor` skill for code quality + OWASP security + MCP pattern checks.

See [AGENTS.md](AGENTS.md) for the complete catalog of agents, skills, and instruction files.

### Snapshot Testing (Golden Files)

Tool definitions are snapshot-tested to detect unintentional changes. Golden files live in `internal/tools/testdata/`:

- `tools_individual.json`: all individual tool definitions
- `tools_meta.json`: all meta-tool definitions
- `tools_meta_compact.json` and `tools_meta_full.json`: the meta-tool definitions under the `compact` and `full` parameter schemas

When you intentionally change a tool definition (name, description, schema, annotations), update the golden files:

```bash
UPDATE_TOOLSNAPS=true go test ./internal/tools/ -run TestToolSnapshots -count=1
```

Then commit the updated golden files alongside your code changes. The CI will fail if snapshots are out of date.

### Where Documentation Lives

The user documentation lives only on the [documentation site](https://docs.jmrp.io/gitlab-mcp-server/). Its pages are under `site/src/content/docs/` in English, each with a Spanish twin at the same path under `site/src/content/docs/es/`, and an English page and its twin change in the same pull request: `pnpm run i18n:check` (in `site/`) fails when a page has no twin or when a Spanish code block differs from its English one outside its comments (commands, configuration and placeholders stay as in English; only comments and prose are translated, and a `text` or `markdown` block holding prose carries `translated` in both fences), and the translation itself is the author's job. `docs/` keeps contributor documentation only, under [`docs/development/`](docs/development/README.md), and the README is a short landing page that links the site. ADR-0025, in the [ADR index](docs/development/adr/README.md), records the decision.

A site page is named by its slug: `operations/http-server` is `site/src/content/docs/operations/http-server.mdx`, published at `https://docs.jmrp.io/gitlab-mcp-server/operations/http-server/`. Link it from a repository file by that URL; `make check-doc-links` resolves the URL and its anchor to the page.

A page that names the release a reader installs today (an install command, an image tag, a package pin, sample `--version` output, "the current release is") writes `%%VERSION%%` instead of the number, and the build replaces it with the published release, the top-level `version` of `server.json`, which the release workflow stamps once the release is out, so a release needs no page edit. `VERSION` is not used, because it names the release being prepared before anyone can download it. A number that is history ("removed in 3.1.0", "checked on 3.1.0") stays literal. `pnpm run version:check` (in `site/`) refuses the token in frontmatter, where the build does not replace it, and a release written by hand where the token belongs; the [`update-starlight-docs` skill](.github/skills/update-starlight-docs/SKILL.md) has the rule and its exceptions.

### What Is Generated

Some documentation is written by a generator and never edited by hand. Change what the generator reads, run it, and commit the result; `make update-all` runs every command below (the benchmark through its redraw target, which measures nothing), and CI compares each committed result with what the tree generates.

| Content                                                                                                                       | Command                                                                                                                                |
| ----------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| The per-domain tool reference, `site/src/content/docs/reference/tools/` (its index included) and its `es/` twin               | `make gen-tool-reference` (a new catalog group first needs its overview and sample questions in `cmd/gen_tool_reference/domains.json`) |
| The fine-grained permissions reference, `reference/fine-grained-permissions.mdx` and its twin                                 | `make gen-action-grants`                                                                                                               |
| The counts the site prints (tools, meta-tools, actions, resources, prompts), `site/src/data/stats.json`                       | `make gen-site-stats`                                                                                                                  |
| The token footprint: the README's token claim, `site/src/data/token-footprint.json` and `docs/development/token-footprint.md` | `make gen-footprint`                                                                                                                   |
| The figures and charts of the `performance/resource-benchmark` page                                                           | `make bench-resources` measures; `make bench-resources-render` redraws from the committed record                                       |
| `llms.txt` and the other `llms*.txt` files at the repository root                                                             | `make gen-llms`                                                                                                                        |
| The managed block of `docs/development/testing/testing.md`                                                                    | `make gen-testing-docs`                                                                                                                |
| The capability arrays of `lhm.plugin.json`                                                                                    | `make gen-lhm-manifest`                                                                                                                |

### When to Update

- Adding a new tool → run `make gen-tool-reference`, `make gen-site-stats` and `make gen-action-grants` (or `make update-all`, which runs all three); a new catalog group needs its overview and sample questions in `cmd/gen_tool_reference/domains.json` first
- Adding a new meta-tool action → update the action count table of the site page `tools/meta-tools`
- Adding a new resource or prompt → update the site page `tools/resources-prompts`
- Adding a new capability → update the site page `capabilities/overview` and the capability's own page under `capabilities/`, and [`docs/development/capabilities.md`](docs/development/capabilities.md) for the API a contributor calls
- Changing configuration → update the site pages `configuration` and `reference/environment`, `reference/cli` for a flag, the variable and flag lists in [`docs/development/configuration.md`](docs/development/configuration.md) and the summary table in `CLAUDE.md`, and the environment variable and flag tables in `.github/copilot-instructions.md`
- Changing how the code is put together (a new package, the path a call takes) → update [`docs/development/architecture.md`](docs/development/architecture.md); a change to error classification or wrapping → [`docs/development/error-handling.md`](docs/development/error-handling.md); a change to how a bundle or channel is built or published → [`docs/development/distribution.md`](docs/development/distribution.md)
- Changing how the repository works (a gate, a generator, a convention) → update `docs/development/` or `CLAUDE.md`, never the site
- Adding or modifying tests → run `make gen-testing-docs` to refresh `docs/development/testing/testing.md`

Every site page in that list changes in English and Spanish together. The `update-starlight-docs` skill maps the rest of the changes to their pages and lists the site's own checks (`cd site && pnpm run build && pnpm run lint`).

### Language Policy

All project artifacts must be written in **English**:

- Source code, comments, doc comments
- Commit messages, branch names
- Documentation, ADRs, specs
- MCP tool names, descriptions, error messages
- Test names and assertions

There are two exceptions. The first is the Spanish half of the user documentation: the Spanish twin of each site page under `site/src/content/docs/es/`, which translates its English page and changes with it, and the Spanish text kept beside its English source (the `es` fields of `cmd/gen_tool_reference/domains.json`, the Spanish strings of the generators that write both languages, `labelEs` in `site/scripts/gen-llms.mjs`, `site/src/content/i18n/es.json`). The second is the server's Spanish message catalog, `internal/i18n/catalog_es.go`, which translates the English catalog key by key and is what a deployment with `GITLAB_MCP_LANGUAGE=es` shows a person, and the Spanish its tests compare a render against (issue 1547). A new entry of the English catalog gets its Spanish in the same change, which the catalog's completeness test enforces.

## Release Process

A release is cut by a tag. Pushing a `v*` tag runs `.github/workflows/release.yml`, which builds every binary with GoReleaser, signs and attests what it publishes, creates the GitHub release and publishes the other channels (container images, npm, PyPI, NuGet, Homebrew, winget, the MCP Registry). `gh workflow run release.yml --ref <branch>` rehearses the whole run without publishing anything.

The pull request that moves `VERSION` also adds `.github/release-notes/vX.Y.Z.md`, the written summary the GitHub release opens with, above the commit list GoReleaser generates. It has four sections, `## Summary`, `## Breaking changes`, `## Upgrading` and `## Security`, in that order, with `None.` under one that has nothing to say, and the Security section names every advisory or CVE the release fixes. `make check-supply-chain` fails without the file, and the release workflow refuses the tag.

`make release` builds the same binaries locally as a GoReleaser snapshot and flattens `dist/` to the release asset names, which is a way to inspect them, not to publish them. [`docs/development/release-process.md`](docs/development/release-process.md) carries the details.

## Issue Reporting

Open an issue at <https://github.com/jmrplens/gitlab-mcp-server/issues/new/choose> and pick a template:

- **Bug Report** — reproducible defects
- **Feature Request** — new functionality / new MCP tool
- **Enhancement** — improvement to existing behavior
- **Documentation** — missing, outdated or incorrect docs

For **security issues**, do not open a public issue: report privately via [GitHub Security Advisories](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/new) (see [SECURITY.md](SECURITY.md)). If you cannot use them, email the same information to the maintainer at [mail@jmrp.io](mailto:mail@jmrp.io), encrypted if you can to the OpenPGP key [SECURITY.md](SECURITY.md#reporting-a-vulnerability) names.

Templates auto-apply the relevant labels listed in [Labels](#labels).

## Labels

Labels arrive four ways, none of which removes one placed by hand. Issue templates apply their kind label on submission, and their "Area" dropdown is read by `.github/workflows/issue-area.yml`, which applies the area label the answer names. Pull requests are labeled from the paths they touch by `.github/workflows/labeler.yml` against the map in `.github/labeler.yml`, and from the title's conventional prefix by the same workflow: `fix:` is a `bug`, `feat:` a `feature`, and nothing else is mapped. Milestones pair with a version label both ways (`v2.8.0` for milestone `2.8.0`), kept in step by `.github/workflows/version-labels.yml`. The repo uses a flat label set (no `type::`/`priority::` namespaces — those are GitLab conventions), and the area labels use the same words on issues and on pull requests so the two can be filtered together:

| Label              | Color     | Used by                                                                         |
| ------------------ | --------- | ------------------------------------------------------------------------------- |
| `bug`              | `#d73a4a` | Bug Report template; `fix:` titles                                              |
| `feature`          | `#a2eeef` | Feature Request template; `feat:` titles                                        |
| `enhancement`      | `#a2eeef` | Enhancement template (GitHub default)                                           |
| `documentation`    | `#0075ca` | Documentation template; path labeler on docs-only PRs                           |
| `security`         | `#d73a4a` | Security Advisories; path labeler on security paths; Area "Authentication"      |
| `ci`               | `#bfdadc` | Path labeler: pipeline, workflows, lint configuration                           |
| `distribution`     | `#fbca04` | Path labeler: release artifacts and install channels; Area "Installation"       |
| `dependencies`     | `#0052cc` | Path labeler and Dependabot: dependency files                                   |
| `transport`        | `#5c8dd6` | Path labeler: `cmd/server`, the transport e2e modules; Area "Transport"         |
| `mcp`              | `#a371f7` | Path labeler: resources, prompts, completions, subscriptions; Area "MCP"        |
| `tools`            | `#1f883d` | Area "GitLab tools" only: two thirds of PRs touch `internal/tools/`, so no path |
| `telemetry`        | `#6fbf9a` | Path labeler: `internal/telemetry`, `internal/mcpotel`; Area "Telemetry"        |
| `site`             | `#f0a35c` | Path labeler: the site's build and components, not its content; Area "Site"     |
| `tooling`          | `#9aa5b1` | Path labeler: the auditors and generators under `cmd/`; Area "CI and tooling"   |
| `platform`         | `#e9c46a` | Path labeler: `*_windows.go`, `*_darwin.go`, `*_unix.go`; Area "Platform"       |
| `e2e`              | `#8bd3c7` | Path labeler: the e2e suite and its Docker fixtures                             |
| `release`          | `#0e8a16` | Manual: release tracking issues                                                 |
| `high-priority`    | `#b60205` | Manual: critical bugs and security advisories                                   |
| `needs-triage`     | `#c2e0c6` | All issue templates; retired by `triage.yml` on milestone or close              |
| `v<version>`       | `#1d76db` | `version-labels.yml`, paired with the milestone of the same number              |
| `good first issue` | `#7057ff` | Manual: newcomer-friendly issues                                                |
| `help wanted`      | `#008672` | Manual: community contributions welcome                                         |
| `question`         | `#d876e3` | Manual: questions / discussions                                                 |
| `duplicate`        | `#cfd3d7` | Manual: duplicates of existing issues                                           |
| `invalid`          | `#e4e669` | Manual: out of scope                                                            |
| `wontfix`          | `#ffffff` | Manual: accepted but won't implement                                            |

The Area dropdown's options are the strings `issue-area.yml` matches, so a change to one has to land in the three templates and that workflow together; an answer the workflow does not know fails its run rather than passing quietly, which is how the drift is noticed.

The path labeler never removes a label (`sync-labels: false`), so anything applied by hand survives a later push. It also cannot create one, because the workflow grants only `pull-requests: write`. A label named in `.github/labeler.yml` that does not exist in the repository is therefore not skipped: the action sends the whole set in a single request, so that one name costs the pull request every label it should have had, and the job says so rather than passing quietly. Create the label first, with `gh label list` / `gh label create`.
