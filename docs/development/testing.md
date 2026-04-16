# Testing

> **Diátaxis type**: Reference
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go testing basics, understanding of httptest
>
> Comprehensive test documentation for gitlab-mcp-server. Updated: 2026-03-31.
>
> **Maintenance Rule**: Whenever tests are added, modified, or removed, this document must be updated with the new counts and coverage values.

---

## Overview

| Metric                      | Value   |
| --------------------------- | ------- |
| Total test functions        | 7,950   |
| Unit test functions         | 7,722   |
| E2E test functions          | 186     |
| cmd test functions          | 42      |
| Test files (internal/)      | 335     |
| Tool sub-packages tested    | 162     |
| Core packages tested        | 15      |
| Average coverage            | ~95.5%  |

### Naming Convention Stats

| Pattern                        | Count | %     |
| ------------------------------ | ----: | ----: |
| `TestFunc_Scenario` (2-part)   | 7,184 | 90.4% |
| `TestFunc` (no-underscore)     |   614 |  7.7% |
| `TestFunc_Sc_Exp` (3-part)     |   152 |  1.9% |

## Test Distribution

### By Layer

| Layer                    | Test Functions | Test Files | Description                          |
| ------------------------ | -------------: | ---------: | ------------------------------------ |
| Core packages            |          1,093 |         68 | autoupdate, config, gitlab, wizard…  |
| Tools orchestration      |            205 |         14 | register, metatool, markdown, errors |
| Tool sub-packages (162)  |          6,424 |        253 | Domain-specific tool handlers        |
| E2E integration          |            186 |         82 | Full workflow against real GitLab    |
| cmd/server               |             42 |          1 | Main entry point tests               |
| **Total**                |      **7,950** |    **418** |                                      |

### Core Packages

| Package        | Tests | Coverage | Description                          |
| -------------- | ----: | -------: | ------------------------------------ |
| autoupdate     |   100 |   76.1%  | Self-update via GitLab releases      |
| completions    |    82 |   99.7%  | Argument auto-completion             |
| config         |    26 |   92.5%  | Configuration loading                |
| elicitation    |    73 |   92.1%  | MCP elicitation capability           |
| gitlab         |    29 |   97.6%  | GitLab API client wrapper            |
| logging        |    14 |   97.1%  | MCP logging capability               |
| progress       |    13 |   94.1%  | MCP progress notifications           |
| prompts        |   169 |   92.9%  | MCP prompt implementations           |
| resources      |    56 |   92.9%  | MCP resource implementations         |
| roots          |    15 |   98.2%  | MCP roots capability                 |
| sampling       |    69 |   94.6%  | MCP sampling capability              |
| serverpool     |    31 |   97.8%  | HTTP mode server pool                |
| testutil       |    18 |   94.3%  | Shared test helpers                  |
| toolutil       |   202 |   90.3%  | Shared tool utilities                |
| wizard         |   196 |   83.0%  | Setup wizard (Web UI, TUI, CLI)      |
| **Subtotal**   |**1,093**|        |                                      |

### Tool Sub-Packages (Top Domains by Test Count)

| Sub-package          | Tests | Coverage | Tools |
| -------------------- | ----: | -------: | ----: |
| projects             |   290 |   91.9%  |    54 |
| mergerequests        |   181 |   93.3%  |    30 |
| users                |   180 |   99.4%  |    28 |
| issues               |   171 |   93.7%  |    21 |
| samplingtools        |   122 |   94.6%  |    11 |
| groups               |   117 |   96.5%  |    18 |
| search               |   105 |   99.8%  |    11 |
| awardemoji           |   103 |   94.8%  |    25 |
| packages             |   101 |   93.7%  |     9 |
| runners              |    92 |   92.0%  |    20 |
| jobs                 |    91 |   96.0%  |    16 |
| resourceevents       |    89 |   97.1%  |    16 |
| commits              |    89 |   95.2%  |    13 |
| accesstokens         |    84 |   98.2%  |    19 |
| pipelineschedules    |    77 |   97.5%  |    11 |
| groupmilestones      |    76 |   97.0%  |     9 |
| externalstatuschecks |    74 |   97.7%  |    14 |
| containerregistry    |    74 |   97.5%  |    14 |
| pipelines            |    69 |   96.7%  |    11 |
| branches             |    69 |   92.1%  |    10 |
| tags                 |    66 |   96.6%  |     9 |
| snippets             |    64 |   94.6%  |    17 |

### Complete Tool Sub-Package Test Counts

<details>
<summary>All 162 sub-packages (click to expand)</summary>

| Sub-package              | Tests |
| ------------------------ | ----: |
| accessrequests           |    40 |
| accesstokens             |    84 |
| alertmanagement          |    28 |
| appearance               |    11 |
| applications             |    14 |
| appstatistics            |     8 |
| attestations             |    17 |
| auditevents              |    42 |
| avatar                   |     9 |
| awardemoji               |   103 |
| badges                   |    44 |
| boards                   |    61 |
| branches                 |    69 |
| branchrules              |    13 |
| broadcastmessages        |    26 |
| bulkimports              |     7 |
| cicatalog                |    19 |
| cilint                   |    27 |
| civariables              |    37 |
| ciyamltemplates          |    20 |
| clusteragents            |    37 |
| commitdiscussions        |    29 |
| commits                  |    89 |
| compliancepolicy         |     5 |
| containerregistry        |    74 |
| customattributes         |    30 |
| customemoji              |    24 |
| dbmigrations             |     6 |
| dependencies             |    13 |
| dependencyproxy          |     6 |
| deploykeys               |    63 |
| deploymentmergerequests  |    20 |
| deployments              |    44 |
| deploytokens             |    63 |
| dockerfiletemplates      |    14 |
| dorametrics              |     9 |
| elicitationtools         |    48 |
| enterpriseusers          |    31 |
| environments             |    43 |
| epicdiscussions          |    27 |
| epicissues               |    14 |
| epicnotes                |     9 |
| epics                    |    41 |
| errortracking            |    24 |
| events                   |    38 |
| externalstatuschecks     |    74 |
| featureflags             |    34 |
| features                 |    17 |
| ffuserlists              |    24 |
| files                    |    57 |
| freezeperiods            |    31 |
| geo                      |    45 |
| gitignoretemplates       |    13 |
| groupanalytics           |     8 |
| groupboards              |    53 |
| groupcredentials         |    31 |
| groupepicboards          |     8 |
| groupimportexport        |    25 |
| groupiterations          |    18 |
| grouplabels              |    45 |
| groupldap                |     9 |
| groupmarkdownuploads     |    34 |
| groupmembers             |    56 |
| groupmilestones          |    76 |
| groupprotectedbranches   |    15 |
| groupprotectedenvs       |    11 |
| grouprelationsexport     |    26 |
| groupreleases            |    14 |
| groups                   |   117 |
| groupsaml                |    22 |
| groupscim                |    25 |
| groupserviceaccounts     |    18 |
| groupsshcerts            |    22 |
| groupstoragemoves        |    34 |
| groupvariables           |    46 |
| groupwikis               |    31 |
| health                   |    17 |
| impersonationtokens      |    38 |
| importservice            |    27 |
| instancevariables        |    36 |
| integrations             |    27 |
| invites                  |    31 |
| issuediscussions         |    39 |
| issuelinks               |    40 |
| issuenotes               |    36 |
| issues                   |   171 |
| issuestatistics          |    39 |
| jobs                     |    91 |
| jobtokenscope            |    47 |
| keys                     |    21 |
| labels                   |    48 |
| license                  |    15 |
| licensetemplates         |    17 |
| markdown                 |     8 |
| memberroles              |    38 |
| members                  |    54 |
| mergerequests            |   181 |
| mergetrains              |    10 |
| metadata                 |     9 |
| milestones               |    57 |
| modelregistry            |     4 |
| mrapprovals              |    56 |
| mrapprovalsettings       |     8 |
| mrchanges                |    32 |
| mrcontextcommits         |    17 |
| mrdiscussions            |    42 |
| mrdraftnotes             |    55 |
| mrnotes                  |    32 |
| namespaces               |    33 |
| notifications            |    30 |
| packages                 |   101 |
| pages                    |    52 |
| pipelines                |    69 |
| pipelineschedules        |    77 |
| pipelinetriggers         |    46 |
| planlimits               |    12 |
| projectaliases           |    23 |
| projectdiscovery         |    19 |
| projectimportexport      |    27 |
| projectiterations        |    17 |
| projectmirrors           |    43 |
| projects                 |   290 |
| projectstatistics        |     8 |
| projectstoragemoves      |    17 |
| projecttemplates         |    17 |
| protectedenvs            |    30 |
| protectedpackages        |    27 |
| releaselinks             |    50 |
| releases                 |    53 |
| repository               |    64 |
| repositorysubmodules     |    44 |
| resourceevents           |    89 |
| resourcegroups           |    18 |
| runnercontrollers        |    26 |
| runnercontrollerscopes   |    27 |
| runnercontrollertokens   |    30 |
| runners                  |    92 |
| samplingtools            |   122 |
| search                   |   105 |
| securefiles              |    25 |
| securityfindings         |    13 |
| securitysettings         |    31 |
| serverupdate             |    22 |
| settings                 |     9 |
| sidekiq                  |    17 |
| snippetdiscussions       |    28 |
| snippetnotes             |    40 |
| snippets                 |    64 |
| snippetstoragemoves      |    38 |
| systemhooks              |    22 |
| tags                     |    66 |
| terraformstates          |    20 |
| todos                    |    29 |
| topics                   |    24 |
| uploads                  |    27 |
| usagedata                |    25 |
| useremails               |    24 |
| usergpgkeys              |    44 |
| users                    |   180 |
| vulnerabilities          |    52 |
| wikis                    |    51 |
| workitems                |    53 |
| **Total**                | **6,424** |

</details>

## Coverage Report

### Core Packages

| Package        | Coverage |
| -------------- | -------: |
| autoupdate     |   76.1%  |
| completions    |   99.7%  |
| config         |   92.5%  |
| elicitation    |   92.1%  |
| gitlab         |   97.6%  |
| logging        |   97.1%  |
| progress       |   94.1%  |
| prompts        |   92.9%  |
| resources      |   92.9%  |
| roots          |   98.2%  |
| sampling       |   94.6%  |
| serverpool     |   97.8%  |
| testutil       |   94.3%  |
| toolutil       |   90.3%  |
| wizard         |   83.0%  |

### Tool Sub-Packages

| Package                  | Coverage |
| ------------------------ | -------: |
| tools (orch.)            |   94.4%  |
| accessrequests           |   97.5%  |
| accesstokens             |   98.2%  |
| alertmanagement          |   97.4%  |
| appearance               |  100.0%  |
| applications             |   94.2%  |
| appstatistics            |   94.3%  |
| attestations             |  100.0%  |
| auditevents              |  100.0%  |
| avatar                   |   95.2%  |
| awardemoji               |   94.8%  |
| badges                   |   96.0%  |
| boards                   |   98.5%  |
| branches                 |   92.1%  |
| branchrules              |   94.6%  |
| broadcastmessages        |   98.7%  |
| bulkimports              |   97.0%  |
| cicatalog                |   99.3%  |
| cilint                   |  100.0%  |
| civariables              |   98.8%  |
| ciyamltemplates          |   95.9%  |
| clusteragents            |   94.0%  |
| commitdiscussions        |   97.5%  |
| commits                  |   95.2%  |
| compliancepolicy         |  100.0%  |
| containerregistry        |   97.5%  |
| customattributes         |   95.1%  |
| customemoji              |   98.0%  |
| dbmigrations             |   94.7%  |
| dependencies             |   99.1%  |
| dependencyproxy          |   93.8%  |
| deploykeys               |   99.1%  |
| deploymentmergerequests  |  100.0%  |
| deployments              |   97.1%  |
| deploytokens             |   97.8%  |
| dockerfiletemplates      |  100.0%  |
| dorametrics              |  100.0%  |
| elicitationtools         |  100.0%  |
| enterpriseusers          |   97.0%  |
| environments             |   97.0%  |
| epicdiscussions          |   99.3%  |
| epicissues               |  100.0%  |
| epicnotes                |   98.5%  |
| epics                    |   99.1%  |
| errortracking            |   99.0%  |
| events                   |   98.5%  |
| externalstatuschecks     |   97.7%  |
| featureflags             |   98.9%  |
| features                 |   95.2%  |
| ffuserlists              |   98.1%  |
| files                    |   96.6%  |
| freezeperiods            |   99.1%  |
| geo                      |   98.9%  |
| gitignoretemplates       |   98.0%  |
| groupanalytics           |  100.0%  |
| groupboards              |   98.4%  |
| groupcredentials         |   96.4%  |
| groupepicboards          |  100.0%  |
| groupimportexport        |   98.4%  |
| groupiterations          |   93.4%  |
| grouplabels              |   98.2%  |
| groupldap                |   97.8%  |
| groupmarkdownuploads     |   97.3%  |
| groupmembers             |   97.6%  |
| groupmilestones          |   97.0%  |
| groupprotectedbranches   |   99.3%  |
| groupprotectedenvs       |   91.2%  |
| grouprelationsexport     |  100.0%  |
| groupreleases            |  100.0%  |
| groups                   |   96.5%  |
| groupsaml                |   98.8%  |
| groupscim                |   96.8%  |
| groupserviceaccounts     |   99.1%  |
| groupsshcerts            |   97.6%  |
| groupstoragemoves        |  100.0%  |
| groupvariables           |   98.7%  |
| groupwikis               |   99.2%  |
| health                   |  100.0%  |
| impersonationtokens      |  100.0%  |
| importservice            |   95.8%  |
| instancevariables        |   98.5%  |
| integrations             |   97.7%  |
| invites                  |  100.0%  |
| issuediscussions         |   98.1%  |
| issuelinks               |   96.4%  |
| issuenotes               |   97.1%  |
| issues                   |   93.7%  |
| issuestatistics          |   94.4%  |
| jobs                     |   96.0%  |
| jobtokenscope            |   97.0%  |
| keys                     |  100.0%  |
| labels                   |   95.1%  |
| license                  |   94.4%  |
| licensetemplates         |   98.5%  |
| markdown                 |  100.0%  |
| memberroles              |   98.0%  |
| members                  |   97.0%  |
| mergerequests            |   93.3%  |
| mergetrains              |   93.8%  |
| metadata                 |   96.0%  |
| milestones               |   92.7%  |
| modelregistry            |   97.1%  |
| mrapprovals              |   98.3%  |
| mrapprovalsettings       |   98.7%  |
| mrchanges                |  100.0%  |
| mrcontextcommits         |   94.5%  |
| mrdiscussions            |   91.7%  |
| mrdraftnotes             |   96.3%  |
| mrnotes                  |   96.6%  |
| namespaces               |   91.5%  |
| notifications            |  100.0%  |
| packages                 |   93.7%  |
| pages                    |   96.4%  |
| pipelines                |   96.7%  |
| pipelineschedules        |   97.5%  |
| pipelinetriggers         |   97.4%  |
| planlimits               |   96.5%  |
| projectaliases           |   97.6%  |
| projectdiscovery         |  100.0%  |
| projectimportexport      |   94.7%  |
| projectiterations        |   93.3%  |
| projectmirrors           |   94.8%  |
| projects                 |   91.9%  |
| projectstatistics        |   96.0%  |
| projectstoragemoves      |  100.0%  |
| projecttemplates         |   98.5%  |
| protectedenvs            |   94.3%  |
| protectedpackages        |   95.5%  |
| releaselinks             |   97.0%  |
| releases                 |   95.4%  |
| repository               |  100.0%  |
| repositorysubmodules     |   96.7%  |
| resourceevents           |   97.1%  |
| resourcegroups           |  100.0%  |
| runnercontrollers        |   96.9%  |
| runnercontrollerscopes   |   95.9%  |
| runnercontrollertokens   |   96.7%  |
| runners                  |   92.0%  |
| samplingtools            |   94.6%  |
| search                   |   99.8%  |
| securefiles              |   98.0%  |
| securityfindings         |   91.5%  |
| securitysettings         |  100.0%  |
| serverupdate             |   90.9%  |
| settings                 |   92.3%  |
| sidekiq                  |   96.6%  |
| snippetdiscussions       |   99.3%  |
| snippetnotes             |   98.5%  |
| snippets                 |   94.6%  |
| snippetstoragemoves      |  100.0%  |
| systemhooks              |   95.1%  |
| tags                     |   96.6%  |
| terraformstates          |   91.8%  |
| todos                    |  100.0%  |
| topics                   |   98.0%  |
| uploads                  |   94.7%  |
| usagedata                |   95.7%  |
| useremails               |  100.0%  |
| usergpgkeys              |  100.0%  |
| users                    |   99.4%  |
| vulnerabilities          |   93.4%  |
| wikis                    |   96.1%  |
| workitems                |   99.0%  |

Coverage target: **>90%** per package. Exceptions:

- **autoupdate** (76.1%) — OS-level operations (`syscall.Exec` process
  replacement, Windows-gated binary rename, signal handling) cannot be
  unit tested without integration infrastructure. The `ExecSelf` function
  replaces the current process, making it untestable in-process.
- **wizard** (83.0%) — Interactive UI code (Bubble Tea TUI, Web UI server,
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
| TestFullWorkflow       |     ~174 |       186 | Individual tools through complete project lifecycle  |
| TestMetaToolWorkflow   |     ~151 |       156 | Same operations via meta-tools (domain dispatch)     |

**Lifecycle covered:** user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → wikis → CI variables → environments → issue links → deploy keys → snippets → pipeline schedules → badges → access tokens → award emoji → sampling → elicitation → cleanup

**Domains added in Docker mode** (require CI runner):

- Pipeline create/get/cancel/retry/delete
- Job get/log/retry/cancel

**MCP capability tests** (mock handlers):

- Sampling tools (11 tests): summarize issue, analyze MR changes, generate release notes, etc.
- Elicitation tools (1 test): confirm destructive action

### Meta-Tool Tests

Meta-tool tests verify the action-dispatch layer that consolidates 1004 individual tools into 40 base / 59 enterprise domain meta-tools. These tests live in `internal/tools/` (the orchestration package).

**What meta-tool tests cover:**

- **Action routing**: Each meta-tool correctly dispatches to the underlying sub-package handler based on the `action` parameter
- **Invalid action**: Requests with unknown actions return an error listing valid actions
- **Metadata audit**: `TestMetadataAudit_*` tests enforce naming conventions, annotations, and tool count invariants across all 1004 tools
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
make test-e2e      # Run E2E tests
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
└── suite/                    # Go test package (82 test files)
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
