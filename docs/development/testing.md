# Testing

> **Diátaxis type**: Reference
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go testing basics, understanding of httptest
>
> Comprehensive test documentation for gitlab-mcp-server. Updated: 2026-04-25.
>
> **Maintenance Rule**: Whenever tests are added, modified, or removed, this document must be updated with the new counts and coverage values.

---

## Overview

| Metric                      | Value   |
| --------------------------- | ------- |
| Total test functions        | 8,753   |
| Unit test functions         | 8,460   |
| E2E test functions          | 214     |
| cmd test functions          | 79      |
| Test files (internal/)      | 400     |
| Tool sub-packages tested    | 163     |
| Core packages tested        | 16      |
| Overall coverage (`go test ./...`) | 97.3%  |
| Average package coverage    | ~98.2%  |

### Naming Convention Stats

| Pattern                        | Count | %     |
| ------------------------------ | ----: | ----: |
| `TestFunc_Scenario` (2-part)   | 7,673 | 89.9% |
| `TestFunc` (no-underscore)     |   656 |  7.7% |
| `TestFunc_Sc_Exp` (3+ part)   |   210 |  2.5% |

## Test Distribution

### By Layer

| Layer                    | Test Functions | Test Files | Description                          |
| ------------------------ | -------------: | ---------: | ------------------------------------ |
| Core packages            |          1,310 |         76 | autoupdate, config, gitlab, oauth…   |
| Tools orchestration      |            222 |         17 | register, metatool, markdown, safemode, errors |
| Tool sub-packages (163)  |          6,928 |        307 | Domain-specific tool handlers        |
| E2E integration          |            214 |         96 | Full workflow against real GitLab    |
| cmd/server               |             79 |          1 | Main entry point + OAuth integration |
| **Total**                |      **8,753** |    **497** |                                      |

### Core Packages

| Package        | Tests | Coverage | Description                          |
| -------------- | ----: | -------: | ------------------------------------ |
| autoupdate     |   110 |   85.1%  | Self-update via GitLab releases      |
| completions    |    91 |  100.0%  | Argument auto-completion             |
| config         |    44 |   99.3%  | Configuration loading                |
| elicitation    |    78 |   92.0%  | MCP elicitation capability           |
| gitlab         |    34 |  100.0%  | GitLab API client wrapper            |
| logging        |    16 |  100.0%  | MCP logging capability               |
| progress       |    17 |  100.0%  | MCP progress notifications           |
| prompts        |   202 |   96.3%  | MCP prompt implementations           |
| resources      |    68 |   98.1%  | MCP resource implementations         |
| roots          |    21 |   98.5%  | MCP roots capability                 |
| sampling       |    83 |   99.5%  | MCP sampling capability              |
| serverpool     |    38 |   99.4%  | HTTP mode server pool                |
| testutil       |    21 |   95.5%  | Shared test helpers                  |
| toolutil       |   247 |   96.2%  | Shared tool utilities                |
| wizard         |   205 |   83.1%  | Setup wizard (Web UI, TUI, CLI)      |
| oauth          |    35 |   98.6%  | OAuth HTTP mode (cache, verifier, middleware, metadata) |
| **Subtotal**   |**1,310**|        |                                      |

### Tool Sub-Packages (Top Domains by Test Count)

| Sub-package          | Tests | Coverage | Tools |
| -------------------- | ----: | -------: | ----: |
| projects             |   324 |   96.7%  |    54 |
| mergerequests        |   209 |   97.2%  |    30 |
| issues               |   195 |   98.6%  |    21 |
| users                |   185 |  100.0%  |    28 |
| samplingtools        |   162 |  100.0%  |    11 |
| groups               |   121 |   99.0%  |    18 |
| search               |   106 |  100.0%  |    11 |
| awardemoji           |   106 |   96.2%  |    25 |
| packages             |   104 |   95.2%  |     9 |
| jobs                 |   117 |   97.7%  |    17 |
| resourceevents       |    99 |  100.0%  |    16 |
| runners              |    97 |   96.8%  |    20 |
| commits              |    96 |   97.1%  |    13 |
| groupmilestones      |    87 |  100.0%  |     9 |
| accesstokens         |    86 |  100.0%  |    19 |
| pipelines            |    98 |   97.6%  |    12 |
| pipelineschedules    |    80 |  100.0%  |    11 |
| branches             |    79 |   97.4%  |    10 |
| containerregistry    |    76 |  100.0%  |    14 |
| externalstatuschecks |    75 |  100.0%  |    14 |
| tags                 |    73 |   99.0%  |     9 |
| snippets             |    68 |   98.7%  |    17 |

### Complete Tool Sub-Package Test Counts

<details>
<summary>All 162 sub-packages (click to expand)</summary>

| Sub-package              | Tests |
| ------------------------ | ----: |
| accessrequests           |    42 |
| accesstokens             |    86 |
| alertmanagement          |    30 |
| appearance               |    11 |
| applications             |    15 |
| appstatistics            |     9 |
| attestations             |    17 |
| auditevents              |    42 |
| avatar                   |    10 |
| awardemoji               |   106 |
| badges                   |    47 |
| boards                   |    63 |
| branches                 |    79 |
| branchrules              |    14 |
| broadcastmessages        |    28 |
| bulkimports              |     9 |
| cicatalog                |    19 |
| cilint                   |    27 |
| civariables              |    40 |
| ciyamltemplates          |    21 |
| clusteragents            |    38 |
| commitdiscussions        |    31 |
| commits                  |    96 |
| compliancepolicy         |     5 |
| containerregistry        |    76 |
| customattributes         |    32 |
| customemoji              |    26 |
| dbmigrations             |     7 |
| dependencies             |    14 |
| dependencyproxy          |     6 |
| deploykeys               |    65 |
| deploymentmergerequests  |    20 |
| deployments              |    47 |
| deploytokens             |    65 |
| dockerfiletemplates      |    14 |
| dorametrics              |     9 |
| elicitationtools         |    56 |
| enterpriseusers          |    33 |
| environments             |    46 |
| epicdiscussions          |    14 |
| epicissues               |    14 |
| epicnotes                |    11 |
| epics                    |    45 |
| errortracking            |    26 |
| events                   |    42 |
| externalstatuschecks     |    75 |
| featureflags             |    36 |
| features                 |    19 |
| ffuserlists              |    26 |
| files                    |    74 |
| freezeperiods            |    32 |
| geo                      |    47 |
| gitignoretemplates       |    14 |
| groupanalytics           |     8 |
| groupboards              |    55 |
| groupcredentials         |    35 |
| groupepicboards          |     8 |
| groupimportexport        |    26 |
| groupiterations          |    19 |
| grouplabels              |    48 |
| groupldap                |    10 |
| groupmarkdownuploads     |    35 |
| groupmembers             |    58 |
| groupmilestones          |    87 |
| groupprotectedbranches   |    16 |
| groupprotectedenvs       |    12 |
| grouprelationsexport     |    26 |
| groupreleases            |    14 |
| groups                   |   121 |
| groupsaml                |    23 |
| groupscim                |    27 |
| groupserviceaccounts     |    19 |
| groupsshcerts            |    24 |
| groupstoragemoves        |    34 |
| groupvariables           |    48 |
| groupwikis               |    32 |
| health                   |    17 |
| impersonationtokens      |    38 |
| importservice            |    28 |
| instancevariables        |    38 |
| integrations             |    31 |
| invites                  |    31 |
| issuediscussions         |    41 |
| issuelinks               |    43 |
| issuenotes               |    38 |
| issues                   |   195 |
| issuestatistics          |    41 |
| jobs                     |   117 |
| jobtokenscope            |    49 |
| keys                     |    21 |
| labels                   |    54 |
| license                  |    17 |
| licensetemplates         |    18 |
| markdown                 |     8 |
| memberroles              |    40 |
| members                  |    58 |
| mergerequests            |   209 |
| mergetrains              |    10 |
| metadata                 |    10 |
| milestones               |    64 |
| modelregistry            |     4 |
| mrapprovals              |    60 |
| mrapprovalsettings       |     9 |
| mrchanges                |    32 |
| mrcontextcommits         |    21 |
| mrdiscussions            |    46 |
| mrdraftnotes             |    60 |
| mrnotes                  |    36 |
| namespaces               |    36 |
| notifications            |    30 |
| packages                 |   104 |
| pages                    |    55 |
| pipelines                |    98 |
| pipelineschedules        |    80 |
| pipelinetriggers         |    49 |
| planlimits               |    13 |
| projectaliases           |    25 |
| projectdiscovery         |    19 |
| projectimportexport      |    31 |
| projectiterations        |    18 |
| projectmirrors           |    51 |
| projects                 |   324 |
| projectstatistics        |     9 |
| projectstoragemoves      |    17 |
| projecttemplates         |    18 |
| protectedenvs            |    35 |
| protectedpackages        |    32 |
| releaselinks             |    54 |
| releases                 |    58 |
| repository               |    64 |
| repositorysubmodules     |    48 |
| resourceevents           |    99 |
| resourcegroups           |    18 |
| runnercontrollers        |    29 |
| runnercontrollerscopes   |    30 |
| runnercontrollertokens   |    33 |
| runners                  |    97 |
| samplingtools            |   162 |
| search                   |   106 |
| securefiles              |    27 |
| securityfindings         |    15 |
| securitysettings         |    31 |
| serverupdate             |    22 |
| settings                 |    12 |
| sidekiq                  |    18 |
| snippetdiscussions       |    29 |
| snippetnotes             |    42 |
| snippets                 |    68 |
| snippetstoragemoves      |    38 |
| systemhooks              |    23 |
| tags                     |    73 |
| terraformstates          |    20 |
| todos                    |    29 |
| topics                   |    26 |
| uploads                  |    30 |
| usagedata                |    27 |
| useremails               |    24 |
| usergpgkeys              |    44 |
| users                    |   185 |
| vulnerabilities          |    52 |
| wikis                    |    57 |
| workitems                |    66 |
| **Total** (163 sub-packages) | **6,928** |

</details>

## Coverage Report

### cmd Package Snapshot

| Package    | Coverage |
| ---------- | -------: |
| cmd/server |   56.3%  |

### Core Packages

| Package        | Coverage |
| -------------- | -------: |
| autoupdate     |   85.1%  |
| completions    |  100.0%  |
| config         |   99.3%  |
| elicitation    |   92.0%  |
| gitlab         |  100.0%  |
| logging        |  100.0%  |
| oauth          |   98.6%  |
| progress       |  100.0%  |
| prompts        |   96.3%  |
| resources      |   98.1%  |
| roots          |   98.5%  |
| sampling       |   99.5%  |
| serverpool     |   99.4%  |
| testutil       |   95.5%  |
| toolutil       |   96.2%  |
| wizard         |   83.1%  |

### Tool Sub-Packages

| Package                  | Coverage |
| ------------------------ | -------: |
| tools (orch.)            |   97.2%  |
| accessrequests           |  100.0%  |
| accesstokens             |  100.0%  |
| alertmanagement          |   98.2%  |
| appearance               |  100.0%  |
| applications             |   98.6%  |
| appstatistics            |   97.1%  |
| attestations             |  100.0%  |
| auditevents              |  100.0%  |
| avatar                   |   95.2%  |
| awardemoji               |   96.2%  |
| badges                   |  100.0%  |
| boards                   |  100.0%  |
| branches                 |   97.4%  |
| branchrules              |   95.7%  |
| broadcastmessages        |  100.0%  |
| bulkimports              |  100.0%  |
| cicatalog                |  100.0%  |
| cilint                   |  100.0%  |
| civariables              |  100.0%  |
| ciyamltemplates          |  100.0%  |
| clusteragents            |   97.0%  |
| commitdiscussions        |   99.2%  |
| commits                  |   97.1%  |
| compliancepolicy         |  100.0%  |
| containerregistry        |  100.0%  |
| customattributes         |   99.0%  |
| customemoji              |  100.0%  |
| dbmigrations             |  100.0%  |
| dependencies             |  100.0%  |
| dependencyproxy          |   93.8%  |
| deploykeys               |  100.0%  |
| deploymentmergerequests  |  100.0%  |
| deployments              |  100.0%  |
| deploytokens             |  100.0%  |
| dockerfiletemplates      |  100.0%  |
| dorametrics              |  100.0%  |
| elicitationtools         |  100.0%  |
| enterpriseusers          |  100.0%  |
| environments             |  100.0%  |
| epicdiscussions          |   93.0%  |
| epicissues               |   96.4%  |
| epicnotes                |   96.0%  |
| epics                    |   99.3%  |
| errortracking            |  100.0%  |
| events                   |  100.0%  |
| externalstatuschecks     |  100.0%  |
| featureflags             |  100.0%  |
| features                 |   97.6%  |
| ffuserlists              |  100.0%  |
| files                    |   92.9%  |
| freezeperiods            |   99.1%  |
| geo                      |  100.0%  |
| gitignoretemplates       |  100.0%  |
| groupanalytics           |  100.0%  |
| groupboards              |  100.0%  |
| groupcredentials         |   98.8%  |
| groupepicboards          |  100.0%  |
| groupimportexport        |   98.4%  |
| groupiterations          |  100.0%  |
| grouplabels              |  100.0%  |
| groupldap                |  100.0%  |
| groupmarkdownuploads     |  100.0%  |
| groupmembers             |  100.0%  |
| groupmilestones          |  100.0%  |
| groupprotectedbranches   |  100.0%  |
| groupprotectedenvs       |   99.4%  |
| grouprelationsexport     |  100.0%  |
| groupreleases            |  100.0%  |
| groups                   |   99.0%  |
| groupsaml                |  100.0%  |
| groupscim                |  100.0%  |
| groupserviceaccounts     |  100.0%  |
| groupsshcerts            |  100.0%  |
| groupstoragemoves        |  100.0%  |
| groupvariables           |  100.0%  |
| groupwikis               |  100.0%  |
| health                   |  100.0%  |
| impersonationtokens      |  100.0%  |
| importservice            |   97.5%  |
| instancevariables        |  100.0%  |
| integrations             |  100.0%  |
| invites                  |  100.0%  |
| issuediscussions         |   99.4%  |
| issuelinks               |   99.1%  |
| issuenotes               |  100.0%  |
| issues                   |   98.6%  |
| issuestatistics          |   95.8%  |
| jobs                     |   97.7%  |
| jobtokenscope            |  100.0%  |
| keys                     |  100.0%  |
| labels                   |   98.9%  |
| license                  |   98.6%  |
| licensetemplates         |  100.0%  |
| markdown                 |  100.0%  |
| memberroles              |  100.0%  |
| members                  |   99.4%  |
| mergerequests            |   97.2%  |
| mergetrains              |  100.0%  |
| metadata                 |  100.0%  |
| milestones               |   96.5%  |
| modelregistry            |   97.1%  |
| mrapprovals              |  100.0%  |
| mrapprovalsettings       |  100.0%  |
| mrchanges                |  100.0%  |
| mrcontextcommits         |  100.0%  |
| mrdiscussions            |   97.5%  |
| mrdraftnotes             |   98.6%  |
| mrnotes                  |   99.3%  |
| namespaces               |   98.3%  |
| notifications            |  100.0%  |
| packages                 |   95.2%  |
| pages                    |   99.1%  |
| pipelines                |   97.6%  |
| pipelineschedules        |  100.0%  |
| pipelinetriggers         |  100.0%  |
| planlimits               |  100.0%  |
| projectaliases           |  100.0%  |
| projectdiscovery         |  100.0%  |
| projectimportexport      |   97.7%  |
| projectiterations        |  100.0%  |
| projectmirrors           |   99.5%  |
| projects                 |   96.7%  |
| projectstatistics        |  100.0%  |
| projectstoragemoves      |  100.0%  |
| projecttemplates         |  100.0%  |
| protectedenvs            |   99.0%  |
| protectedpackages        |  100.0%  |
| releaselinks             |  100.0%  |
| releases                 |   99.0%  |
| repository               |   96.3%  |
| repositorysubmodules     |  100.0%  |
| resourceevents           |  100.0%  |
| resourcegroups           |  100.0%  |
| runnercontrollers        |  100.0%  |
| runnercontrollerscopes   |  100.0%  |
| runnercontrollertokens   |  100.0%  |
| runners                  |   96.8%  |
| samplingtools            |  100.0%  |
| search                   |  100.0%  |
| securefiles              |   99.0%  |
| securityfindings         |   96.4%  |
| securitysettings         |  100.0%  |
| serverupdate             |   90.9%  |
| settings                 |   92.3%  |
| sidekiq                  |  100.0%  |
| snippetdiscussions       |   99.3%  |
| snippetnotes             |  100.0%  |
| snippets                 |   98.7%  |
| snippetstoragemoves      |  100.0%  |
| systemhooks              |   97.0%  |
| tags                     |   99.0%  |
| terraformstates          |   91.8%  |
| todos                    |  100.0%  |
| topics                   |  100.0%  |
| uploads                  |   95.7%  |
| usagedata                |   99.4%  |
| useremails               |  100.0%  |
| usergpgkeys              |  100.0%  |
| users                    |  100.0%  |
| vulnerabilities          |   98.5%  |
| wikis                    |   98.9%  |
| workitems                |  100.0%  |

Coverage target: **>90%** per package. Exceptions:

- **autoupdate** (75.6%) — OS-level operations (`syscall.Exec` process
  replacement, Windows-gated binary rename, signal handling) cannot be
  unit tested without integration infrastructure. The `ExecSelf` function
  replaces the current process, making it untestable in-process.
- **wizard** (83.1%) — Interactive UI code (Bubble Tea TUI, Web UI server,
  OS directory picker, browser launch) requires heavy stubbing. Package-level
  function variables (`allClientsFn`, `openBrowserFn`, `pickDirectoryFn`)
  enable test isolation; see [Wizard Test Helpers](#wizard-test-helpers) below.

## Test Types

### Unit Tests

All unit tests use `httptest` to mock GitLab API responses. No real GitLab API calls are made during unit testing.

**Patterns used:**

- **Table-driven tests** with `t.Run()` subtests — standard across all packages
- **Mock server**: `testutil.NewTestClient()` creates a GitLab client pointing to a local `httptest.Server`
- **JSON responses**: `testutil.RespondJSON()` and `testutil.RespondJSONWithPagination()` helpers
- **Naming convention**: `TestToolName_Scenario_ExpectedResult`

**Example structure:**

```go
func TestGetBranch_Success(t *testing.T) {
    client, mux, cleanup := testutil.NewTestClient()
    defer cleanup()

    mux.HandleFunc("/api/v4/projects/1/repository/branches/main", func(w http.ResponseWriter, r *http.Request) {
        testutil.RespondJSON(w, gitlab.Branch{Name: "main"})
    })

    // ... invoke tool handler, assert result
}
```

### End-to-End Tests

E2E tests run against a **real GitLab instance** using in-memory MCP transport (build tag `e2e`). Two modes are supported:

#### Self-Hosted Mode

Requires a running GitLab instance with credentials in `.env`:

```bash
# .env
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-...
```

```bash
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e
```

#### Docker Mode

Uses an ephemeral GitLab CE container provisioned by Docker Compose. Requires Docker and ~4 GB RAM.

All E2E Docker infrastructure is version-controlled under `test/e2e/`:

- `test/e2e/docker-compose.yml` — GitLab CE + Runner compose definition
- `test/e2e/scripts/setup-gitlab.sh` — Creates test user, PAT, writes `.env.docker`
- `test/e2e/scripts/register-runner.sh` — Registers CI runner in GitLab
- `test/e2e/scripts/wait-for-gitlab.sh` — Polls GitLab readiness endpoint

```bash
# Start GitLab container and provision test environment
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh
./test/e2e/scripts/setup-gitlab.sh    # Creates .env.docker
./test/e2e/scripts/register-runner.sh # Registers CI runner

# Run tests
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/

# Cleanup
docker compose -f test/e2e/docker-compose.yml down -v
```

Or use the Makefile target that automates the full lifecycle:

```bash
make test-e2e-docker
```

Docker mode enables pipeline and job tests that require a CI runner.

#### Test Reports

Both `make test-e2e` and `make test-e2e-docker` use [gotestsum](https://github.com/gotestyourself/gotestsum) to produce structured test reports in `dist/e2e-reports/`:

| File                        | Format    | Purpose                                      |
| --------------------------- | --------- | -------------------------------------------- |
| `e2e-junit.xml`             | JUnit XML | CI/CD integration (GitHub Actions, SonarQube) |
| `e2e-log.json`              | JSON      | Programmatic analysis, filtering              |
| `e2e-output.txt`            | Plain     | Human-readable console output (`testdox`)     |

Docker mode files use the `e2e-docker-` prefix. Reports are written to `dist/e2e-reports/` (gitignored via `dist/`).

Install gotestsum via `make install-tools` or `go install gotest.tools/gotestsum@latest`.

#### Test Architecture

The suite uses 4 MCP server/client pairs via `mcp.NewInMemoryTransports()`:

| Session            | Purpose                                    |
| ------------------ | ------------------------------------------ |
| `session`          | Individual tools (TestFullWorkflow)         |
| `metaSession`      | Meta-tools (TestMetaToolWorkflow)           |
| `samplingSession`  | Sampling tools with mock LLM handler       |
| `elicitSession`    | Elicitation tools with mock user handler   |

**Workflows:**

| Workflow               | Subtests | Functions | Description                                         |
| ---------------------- | -------: | --------: | --------------------------------------------------- |
| TestFullWorkflow       |     ~174 |       191 | Individual tools through complete project lifecycle  |
| TestMetaToolWorkflow   |     ~151 |       156 | Same operations via meta-tools (domain dispatch)     |

**Lifecycle covered:** user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → wikis → CI variables → environments → issue links → deploy keys → snippets → pipeline schedules → badges → access tokens → award emoji → sampling → elicitation → cleanup

**Domains added in Docker mode** (require CI runner):

- Pipeline create/get/cancel/retry/delete
- Job get/log/retry/cancel

**MCP capability tests** (mock handlers):

- Sampling tools (11 tests): summarize issue, analyze MR changes, generate release notes, etc.
- Elicitation tools (1 test): confirm destructive action

#### Fixture Cleanup

Test fixtures (`fixture_test.go`) register `t.Cleanup` handlers that **permanently delete** projects created during tests. GitLab's Delayed Deletion feature requires a two-step process:

1. Mark the project for deletion (`DELETE /projects/:id`)
2. Permanently remove it (`DELETE /projects/:id?permanently_remove=true&full_path=...`)

The `cleanupOrphanedProjects` function in `setup_test.go` runs at suite start to remove leftover projects from interrupted runs, including those already in pending-delete state (`IncludePendingDelete` option).

### Meta-Tool Tests

Meta-tool tests verify the action-dispatch layer that consolidates 1000 individual tools into 28 base / 43 enterprise domain meta-tools. These tests live in `internal/tools/` (the orchestration package).

**What meta-tool tests cover:**

- **Action routing**: Each meta-tool correctly dispatches to the underlying sub-package handler based on the `action` parameter
- **Invalid action**: Requests with unknown actions return an error listing valid actions
- **Metadata audit**: `TestMetadataAudit_*` tests enforce naming conventions, annotations, and tool count invariants across all 1000 tools
- **Destructive metadata consistency**: `TestDestructiveMetadataConsistency` cross-checks `ActionRoute.Destructive` metadata against `toolutil.DeleteAnnotations` on individual tools — ensures meta-tool routes and individual tools agree on which actions are destructive
- **Markdown formatting**: `markdownForResult` delegates to the type-based registry (`toolutil.MarkdownForResult`) which invokes the formatter registered by the sub-package `init()` function
- **next_steps enrichment**: `enrichWithHints()` correctly extracts hints from Markdown and injects them into JSON `structuredContent`

**Running meta-tool tests:**

```bash
# All orchestration tests (register, metatool, markdown, errors)
go test ./internal/tools/ -count=1 -v

# Metadata audit only
go test ./internal/tools/ -run TestMetadataAudit -count=1 -v

# Specific domain meta-tool tests
go test ./internal/tools/ -run TestProject -count=1 -v
go test ./internal/tools/ -run TestBranch -count=1 -v
```

**E2E meta-tool tests:**

The `TestMetaToolWorkflow` E2E test (~151 subtests) exercises the same project lifecycle as `TestFullWorkflow` but through meta-tool action dispatch instead of individual tools. This validates routing, parameter passthrough, and response formatting in a real GitLab environment. It covers 15 additional domains beyond the core workflow: wikis, CI variables, CI lint, environments, issue links, deploy keys, snippets, issue discussions, draft notes, pipeline schedules, badges, access tokens, award emoji, labels, and milestones.

```bash
# Run only the meta-tool E2E workflow
go test -v -tags e2e -timeout 300s -run TestMetaToolWorkflow ./test/e2e/suite/
```

### Validation Tests

Validation tests in `internal/tools/register_validation_test.go` ensure structural integrity across all sub-packages:

| Test                                   | Purpose                                                                                     |
| -------------------------------------- | ------------------------------------------------------------------------------------------- |
| `TestAllSubPackagesRegistered`         | Scans all 162 sub-directories under `internal/tools/` and verifies each has a `RegisterTools` call in `register.go` (with `knownExceptions` for `serverupdate`, which registers in `cmd/server/main.go`) |
| `TestAllMarkdownFormattersRegistered`  | Verifies all ~266 output types across 76 sub-packages have registered markdown formatters via `toolutil.RegisterMarkdown[T]` |
| `TestAllHintReferencesValid`           | Validates all `action 'xxx'` and backtick-quoted `` `gitlab_xxx` `` references in WriteHints across all markdown.go files match registered tools/actions (1071 tools, 898 actions) |

```bash
# Run validation tests
go test ./internal/tools/ -run "TestAllSubPackages|TestAllMarkdown|TestAllHint" -count=1 -v
```

## Running Tests

### Unit Tests

```bash
# All unit tests
go test ./internal/... -count=1

# Specific package (verbose)
go test ./internal/tools/branches/ -count=1 -v

# Specific test by name
go test ./internal/tools/ -run TestBranch -count=1

# With coverage
go test ./internal/tools/branches/ -coverprofile=cover.out -count=1
go tool cover -func=cover.out

# With race detector
go test ./internal/... -race -count=1
```

### E2E Tests

```bash
# Full suite (self-hosted GitLab)
go test -v -tags e2e -timeout 300s ./test/e2e/suite/
make test-e2e

# Docker mode (ephemeral GitLab CE container)
docker compose -f test/e2e/docker-compose.yml up -d
./test/e2e/scripts/wait-for-gitlab.sh && ./test/e2e/scripts/setup-gitlab.sh && ./test/e2e/scripts/register-runner.sh
set -a && source test/e2e/.env.docker && set +a
go test -v -tags e2e -timeout 600s ./test/e2e/suite/
docker compose -f test/e2e/docker-compose.yml down -v

# Individual workflows
go test -v -tags e2e -timeout 300s -run TestFullWorkflow ./test/e2e/suite/
go test -v -tags e2e -timeout 300s -run TestMetaToolWorkflow ./test/e2e/suite/

# Compile-only (verify builds without GitLab)
go test -tags e2e -c -o NUL ./test/e2e/suite/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/suite/  # Linux
```

### Coverage Report

```bash
# Full coverage for all internal packages
go test ./internal/... -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out

# HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Per-package summary
go test ./internal/... -cover -count=1
```

### Makefile Targets

```bash
make test          # Run all unit tests
make test-race     # Run with race detector
make test-e2e      # Run E2E tests (self-hosted GitLab) — generates JUnit + JSON reports
make test-e2e-docker # Run E2E tests with ephemeral GitLab CE — generates JUnit + JSON reports
make coverage      # Generate coverage report
make lint          # Run go vet + staticcheck
make inspector     # Compile + launch MCP Inspector UI via stdio
make inspector-stop # Stop Inspector and clean up temp binary
```

## Test Infrastructure

### Shared Helpers (`internal/testutil/`)

| Helper                          | Purpose                                    |
| ------------------------------- | ------------------------------------------ |
| `NewTestClient()`               | Creates mock GitLab client + httptest server |
| `RespondJSON()`                 | Writes JSON response body                  |
| `RespondJSONWithPagination()`   | Writes JSON + pagination headers           |

### Test File Organization

Each tool sub-package follows this structure:

```text
internal/tools/{domain}/
├── {domain}.go          # Tool handlers
├── {domain}_test.go     # Unit tests
├── register.go          # Tool registration
├── markdown.go          # Markdown formatters (if any)
└── markdown_test.go     # Formatter tests (if any)
```

**Exception — `samplingtools/`** uses per-tool file organization (11 tools, 13 source files + 12 test files):

```text
internal/tools/samplingtools/
├── common.go                          # Shared helpers
├── common_test.go                     # Shared test helpers and constants
├── register.go                        # Tool registration (11 tools)
├── analyze_mr_changes.go              # Per-tool handler
├── analyze_mr_changes_test.go         # Per-tool tests
├── summarize_issue.go
├── summarize_issue_test.go
├── ...                                # 9 more tool/test pairs
└── samplingtools.go                   # Shared types (Input/Output structs)
```

### E2E Test Structure

```text
test/e2e/
├── docker-compose.yml        # Ephemeral GitLab CE + Runner
├── .env.docker               # Docker mode environment variables
├── README.md                 # E2E documentation
├── scripts/                  # Provisioning scripts
│   ├── register-runner.sh
│   ├── setup-gitlab.sh
│   └── wait-for-gitlab.sh
└── suite/                    # Go test package (94 test files)
    ├── setup_test.go         # MCP server setup, helpers, shared state
    ├── fixture_test.go       # Self-contained GitLab resource builders
    └── *_test.go             # Domain-specific test files
```

### Wizard Test Helpers

The `internal/wizard/` package tests interactive UI code (Web UI, Bubble Tea
TUI, CLI) that would normally open browsers, OS dialogs, and write to real
user config files. Test isolation is achieved via **package-level function
variables** overridden in tests with `t.Cleanup` to restore originals.

**Function variables** (defined in source files, overridden in tests):

| Variable          | Source file     | Real function    | Purpose                    |
| ----------------- | --------------- | ---------------- | -------------------------- |
| `allClientsFn`    | `clients.go`    | `AllClients()`   | Returns MCP client configs |
| `openBrowserFn`   | `browser.go`    | `openBrowser()`  | Launches default browser   |
| `pickDirectoryFn` | `dirpicker.go`  | `pickDirectory()`| Opens OS directory picker  |

**Test helpers** (`testhelpers_test.go`):

| Helper                          | Purpose                                                  |
| ------------------------------- | -------------------------------------------------------- |
| `useFakeClients(t)`             | Overrides `allClientsFn` with clients using temp dir paths — prevents writing to real `mcp.json` files |
| `stubPickDirectory(t, path, err)` | Overrides `pickDirectoryFn` — prevents OS directory dialog |
| `stubOpenBrowser(t)`            | Overrides `openBrowserFn` — prevents browser launch       |

**File organization** (12 test files, 159 test functions):

```text
internal/wizard/
├── clients_test.go        # 25 tests — MCP client detection and config paths
├── cli_test.go            # 18 tests — CLI-mode wizard flow
├── envfile_test.go        #  3 tests — .env file operations
├── install_test.go        #  6 tests — Binary installation logic
├── jsonmerge_test.go      #  9 tests — JSON config merge operations
├── paths_test.go          #  4 tests — Platform-specific path resolution
├── prompt_test.go         # 20 tests — User prompt/input handling
├── run_test.go            #  1 test  — Top-level Run() entry point
├── testhelpers_test.go    #  0 tests — Shared test helpers only
├── tui_test.go            # 43 tests — Bubble Tea TUI model and view
├── webui_test.go          # 15 tests — Web UI HTTP handlers
└── wizard_test.go         # 15 tests — Core wizard orchestration
```
