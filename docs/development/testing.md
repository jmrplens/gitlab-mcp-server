# Testing

> **Diátaxis type**: Reference
> **Audience**: 🔧 Developers, contributors
> **Prerequisites**: Go testing basics, understanding of httptest
>
> Comprehensive test documentation for gitlab-mcp-server. Updated: 2026-03-23.
>
> **Maintenance Rule**: Whenever tests are added, modified, or removed, this document must be updated with the new counts and coverage values.

---

## Overview

| Metric                      | Value   |
| --------------------------- | ------- |
| Total test functions        | 6,449   |
| Unit test functions         | 6,322   |
| E2E test functions          | 86      |
| cmd test functions          | 36      |
| Test files (internal/)      | 217     |
| Tool sub-packages tested    | 122     |
| Core packages tested        | 14      |
| Average coverage            | ~95.9%  |

### Naming Convention Stats

| Pattern                        | Count | %     |
| ------------------------------ | ----: | ----: |
| `TestFunc_Scenario` (2-part)   | 5,690 | 89.9% |
| `TestFunc` (no-underscore)     |   513 |  8.1% |
| `TestFunc_Sc_Exp` (3-part)     |   125 |  2.0% |
| E2E workflow (exempt)          |     2 |  0.0% |

## Test Distribution

### By Layer

| Layer                    | Test Functions | Test Files | Description                          |
| ------------------------ | -------------: | ---------: | ------------------------------------ |
| Core packages            |            962 |         61 | autoupdate, config, gitlab, wizard…  |
| Tools orchestration      |            205 |         14 | register, metatool, markdown, errors |
| Tool sub-packages (122)  |          5,163 |        142 | Domain-specific tool handlers        |
| E2E integration          |             86 |          3 | Full workflow against real GitLab    |
| cmd/server               |             36 |          1 | Main entry point tests               |
| **Total**                |      **6,449** |    **221** |                                      |

### Core Packages

| Package        | Tests | Coverage | Description                          |
| -------------- | ----: | -------: | ------------------------------------ |
| autoupdate     |    97 |   83.5%  | Self-update via GitLab releases      |
| completions    |    82 |   99.7%  | Argument auto-completion             |
| config         |    25 |   94.7%  | Configuration loading                |
| elicitation    |    65 |   91.5%  | MCP elicitation capability           |
| gitlab         |    12 |  100.0%  | GitLab API client wrapper            |
| logging        |    13 |   97.0%  | MCP logging capability               |
| progress       |    11 |  100.0%  | MCP progress notifications           |
| prompts        |   170 |   92.9%  | MCP prompt implementations           |
| resources      |    56 |   94.3%  | MCP resource implementations         |
| roots          |    15 |   98.2%  | MCP roots capability                 |
| sampling       |    69 |   94.6%  | MCP sampling capability              |
| serverpool     |    22 |   79.3%  | HTTP mode server pool                |
| toolutil       |   166 |   95.1%  | Shared tool utilities                |
| wizard         |   159 |   54.6%  | Setup wizard (Web UI, TUI, CLI)      |
| **Subtotal**   |**962**|          |                                      |

### Tool Sub-Packages (Top Domains by Test Count)

| Sub-package          | Tests | Coverage | Tools |
| -------------------- | ----: | -------: | ----: |
| projects             |   191 |   96.6%  |    33 |
| mergerequests        |   171 |   95.2%  |    30 |
| issues               |   170 |   95.3%  |    21 |
| samplingtools        |   122 |   96.1%  |    11 |
| groups               |   109 |   99.0%  |    16 |
| awardemoji           |   103 |   97.4%  |    25 |
| packages             |   102 |   93.5%  |     9 |
| search               |   105 |  100.0%  |    11 |
| runners              |    92 |   92.0%  |    19 |
| jobs                 |    91 |   96.0%  |    16 |
| commits              |    89 |   96.5%  |    13 |
| accesstokens         |    84 |   98.2%  |    19 |
| resourceevents       |    78 |  100.0%  |    13 |
| pipelineschedules    |    77 |   97.5%  |    11 |
| groupmilestones      |    76 |   96.9%  |     9 |
| containerregistry    |    73 |   97.5%  |    14 |
| pipelines            |    69 |   98.3%  |    11 |
| users                |    68 |  100.0%  |     5 |
| tags                 |    66 |   98.2%  |     8 |
| branches             |    69 |   96.3%  |     8 |
| repository           |    64 |  100.0%  |     4 |
| snippets             |    64 |   95.1%  |     8 |

### Complete Tool Sub-Package Test Counts

<details>
<summary>All 162 sub-packages (click to expand)</summary>

| Sub-package              | Tests |
| ------------------------ | ----: |
| accessrequests           |    41 |
| accesstokens             |    84 |
| alertmanagement          |    24 |
| appearance               |    11 |
| applications             |    14 |
| appstatistics            |     8 |
| avatar                   |     9 |
| awardemoji               |   103 |
| badges                   |    44 |
| boards                   |    61 |
| branches                 |    69 |
| branchrules              |    12 |
| broadcastmessages        |    26 |
| bulkimports              |     7 |
| cicatalog                |    15 |
| cilint                   |    27 |
| civariables              |    36 |
| ciyamltemplates          |    20 |
| clusteragents            |    37 |
| commitdiscussions        |    29 |
| commits                  |    89 |
| containerregistry        |    73 |
| customattributes         |    30 |
| customemoji              |    21 |
| dbmigrations             |     6 |
| dependencyproxy          |     6 |
| deploykeys               |    63 |
| deploymentmergerequests  |    20 |
| deployments              |    44 |
| deploytokens             |    63 |
| dockerfiletemplates      |    14 |
| elicitationtools         |    48 |
| environments             |    43 |
| epicdiscussions          |    27 |
| errortracking            |    24 |
| events                   |    38 |
| featureflags             |    34 |
| features                 |    17 |
| ffuserlists              |    24 |
| files                    |    57 |
| freezeperiods            |    31 |
| gitignoretemplates       |    13 |
| groupboards              |    53 |
| groupimportexport        |    25 |
| grouplabels              |    45 |
| groupmarkdownuploads     |    31 |
| groupmembers             |    56 |
| groupmilestones          |    76 |
| grouprelationsexport     |    23 |
| groups                   |   109 |
| groupvariables           |    46 |
| health                   |    17 |
| importservice            |    27 |
| instancevariables        |    36 |
| integrations             |    27 |
| invites                  |    31 |
| issuediscussions         |    39 |
| issuelinks               |    40 |
| issuenotes               |    36 |
| issues                   |   170 |
| issuestatistics          |    39 |
| jobs                     |    91 |
| jobtokenscope            |    47 |
| keys                     |    21 |
| labels                   |    48 |
| license                  |    15 |
| licensetemplates         |    17 |
| markdown                 |     8 |
| members                  |    54 |
| mergerequests            |   171 |
| metadata                 |     9 |
| milestones               |    57 |
| mrapprovals              |    56 |
| mrchanges                |    30 |
| mrcontextcommits         |    17 |
| mrdiscussions            |    42 |
| mrdraftnotes             |    49 |
| mrnotes                  |    32 |
| namespaces               |    33 |
| notifications            |    30 |
| packages                 |   102 |
| pages                    |    52 |
| pipelines                |    69 |
| pipelineschedules        |    77 |
| pipelinetriggers         |    46 |
| planlimits               |    12 |
| projectdiscovery         |    18 |
| projectimportexport      |    27 |
| projects                 |   191 |
| projectstatistics        |     8 |
| projecttemplates         |    17 |
| protectedenvs            |    30 |
| releaselinks             |    46 |
| releases                 |    52 |
| repository               |    64 |
| repositorysubmodules     |    44 |
| resourceevents           |    78 |
| resourcegroups           |    18 |
| runnercontrollers        |    31 |
| runnercontrollerscopes   |    32 |
| runnercontrollertokens   |    35 |
| runners                  |    92 |
| samplingtools            |   122 |
| search                   |   105 |
| securefiles              |    20 |
| securityfindings         |    13 |
| serverupdate             |    22 |
| settings                 |     9 |
| sidekiq                  |    17 |
| snippetdiscussions       |    28 |
| snippets                 |    64 |
| systemhooks              |    21 |
| tags                     |    66 |
| terraformstates          |    20 |
| todos                    |    29 |
| topics                   |    24 |
| uploads                  |    27 |
| usagedata                |    25 |
| users                    |    68 |
| vulnerabilities          |    35 |
| wikis                    |    51 |
| workitems                |    53 |
| **Total**                | **5,163** |

</details>

## Coverage Report

### Core Packages

| Package        | Coverage |
| -------------- | -------: |
| autoupdate     |   83.5%  |
| completions    |   99.7%  |
| config         |   94.7%  |
| elicitation    |   91.5%  |
| gitlab         |  100.0%  |
| logging        |   97.0%  |
| progress       |  100.0%  |
| prompts        |   92.9%  |
| resources      |   94.3%  |
| roots          |   98.2%  |
| sampling       |   94.6%  |
| serverpool     |   79.3%  |
| toolutil       |   95.1%  |
| wizard         |   54.6%  |

### Tool Sub-Packages

| Package                  | Coverage |
| ------------------------ | -------: |
| tools (orch.)            |   94.1%  |
| accessrequests           |   97.4%  |
| accesstokens             |   98.2%  |
| alertmanagement          |   98.9%  |
| appearance               |  100.0%  |
| applications             |   94.0%  |
| appstatistics            |   96.3%  |
| avatar                   |   95.0%  |
| awardemoji               |   97.4%  |
| badges                   |   97.9%  |
| boards                   |   98.5%  |
| branches                 |   96.3%  |
| branchrules              |   94.3%  |
| broadcastmessages        |   98.7%  |
| bulkimports              |   96.9%  |
| cicatalog                |   89.7%  |
| cilint                   |  100.0%  |
| civariables              |   98.7%  |
| ciyamltemplates          |   95.7%  |
| clusteragents            |   93.8%  |
| commitdiscussions        |   97.4%  |
| commits                  |   96.5%  |
| containerregistry        |   97.5%  |
| customattributes         |   95.0%  |
| customemoji              |   79.4%  |
| dbmigrations             |   94.4%  |
| dependencyproxy          |   93.8%  |
| deploykeys               |   99.1%  |
| deploymentmergerequests  |  100.0%  |
| deployments              |   98.2%  |
| deploytokens             |   97.8%  |
| dockerfiletemplates      |  100.0%  |
| elicitationtools         |  100.0%  |
| environments             |   98.2%  |
| epicdiscussions          |   99.3%  |
| errortracking            |   98.9%  |
| events                   |  100.0%  |
| featureflags             |   98.9%  |
| features                 |   95.1%  |
| ffuserlists              |   98.1%  |
| files                    |   99.3%  |
| freezeperiods            |   99.1%  |
| gitignoretemplates       |   97.9%  |
| groupboards              |   98.4%  |
| groupimportexport        |   98.0%  |
| grouplabels              |   98.2%  |
| groupmarkdownuploads     |   96.7%  |
| groupmembers             |   97.7%  |
| groupmilestones          |   96.9%  |
| grouprelationsexport     |  100.0%  |
| groups                   |   99.0%  |
| groupvariables           |   98.7%  |
| health                   |  100.0%  |
| importservice            |   95.8%  |
| instancevariables        |   98.5%  |
| integrations             |   97.7%  |
| invites                  |  100.0%  |
| issuediscussions         |   98.1%  |
| issuelinks               |   98.1%  |
| issuenotes               |   98.5%  |
| issues                   |   95.3%  |
| issuestatistics          |   95.7%  |
| jobs                     |   96.0%  |
| jobtokenscope            |   96.9%  |
| keys                     |  100.0%  |
| labels                   |   97.7%  |
| license                  |   94.1%  |
| licensetemplates         |   98.4%  |
| markdown                 |  100.0%  |
| members                  |   98.8%  |
| mergerequests            |   95.2%  |
| metadata                 |   95.8%  |
| milestones               |   93.6%  |
| mrapprovals              |   98.7%  |
| mrchanges                |  100.0%  |
| mrcontextcommits         |   94.4%  |
| mrdiscussions            |   92.4%  |
| mrdraftnotes             |   90.6%  |
| mrnotes                  |   97.9%  |
| namespaces               |  100.0%  |
| notifications            |  100.0%  |
| packages                 |   93.5%  |
| pages                    |   96.4%  |
| pipelines                |   98.3%  |
| pipelineschedules        |   97.5%  |
| pipelinetriggers         |   97.3%  |
| planlimits               |   96.4%  |
| projectdiscovery         |  100.0%  |
| projectimportexport      |   94.5%  |
| projects                 |   96.6%  |
| projectstatistics        |   95.8%  |
| projecttemplates         |   98.5%  |
| protectedenvs            |   94.2%  |
| releaselinks             |   98.1%  |
| releases                 |   96.8%  |
| repository               |  100.0%  |
| repositorysubmodules     |   96.6%  |
| resourceevents           |  100.0%  |
| resourcegroups           |  100.0%  |
| runnercontrollers        |   96.9%  |
| runnercontrollerscopes   |   95.8%  |
| runnercontrollertokens   |   96.7%  |
| runners                  |   92.0%  |
| samplingtools            |   96.1%  |
| search                   |  100.0%  |
| securefiles              |   98.7%  |
| securityfindings         |   91.4%  |
| serverupdate             |   94.2%  |
| settings                 |   92.1%  |
| sidekiq                  |   96.5%  |
| snippetdiscussions       |   99.3%  |
| snippets                 |   95.1%  |
| systemhooks              |   95.0%  |
| tags                     |   98.2%  |
| terraformstates          |   91.6%  |
| todos                    |  100.0%  |
| topics                   |   98.0%  |
| uploads                  |   94.6%  |
| usagedata                |   95.5%  |
| users                    |  100.0%  |
| vulnerabilities          |   80.7%  |
| wikis                    |   97.7%  |
| workitems                |   99.0%  |

Coverage target: **>90%** per package. Exceptions:

- **autoupdate** (83.5%) — OS-level operations (rename, exec, signal handling)
  are difficult to unit test without integration infrastructure.
- **wizard** (53.5%) — Interactive UI code (Bubble Tea TUI, Web UI server,
  OS directory picker, browser launch) requires heavy stubbing. Package-level
  function variables (`allClientsFn`, `openBrowserFn`, `pickDirectoryFn`)
  enable test isolation; see [Wizard Test Helpers](#wizard-test-helpers) below.
- **vulnerabilities** (80.7%) — GraphQL-based tool with complex response
  parsing and multiple mutation variants. Coverage limited by deep nesting
  in the `Data` envelope unmarshalling paths.
- **customemoji** (79.4%) — GraphQL-based tool; lower coverage due to
  mutation error paths and delete confirmation flow branches.
- **cicatalog** (89.7%) — GraphQL-based tool; marginally below target due
  to nested component structure parsing paths.

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

E2E tests run against a **real GitLab instance** using in-memory MCP transport (build tag `e2e`):

```bash
go test -v -tags e2e -timeout 300s ./test/e2e/
```

**Requirements:**

- `.env` with `GITLAB_URL` and `GITLAB_TOKEN`
- User must have permissions to create/delete projects

**Workflows:**

| Workflow               | Subtests | Description                                     |
| ---------------------- | -------: | ----------------------------------------------- |
| TestFullWorkflow       |      ~77 | Individual tools through complete project lifecycle |
| TestMetaToolWorkflow   |      ~78 | Same operations via meta-tools (domain dispatch)   |

**Lifecycle covered:** user → project CRUD → commits → branches → tags → releases → issues → labels → milestones → members → upload → MR lifecycle → notes → discussions → search → groups → pipelines → packages → cleanup

**Not covered** (requires infrastructure unavailable in test environment):

- Pipeline create/get/cancel/retry/delete — requires CI runner
- Job tools — requires running pipeline
- Sampling tools — requires MCP sampling capability
- Elicitation tools — requires MCP elicitation capability

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

The `TestMetaToolWorkflow` E2E test (~78 subtests) exercises the same project lifecycle as `TestFullWorkflow` but through meta-tool action dispatch instead of individual tools. This validates routing, parameter passthrough, and response formatting in a real GitLab environment.

```bash
# Run only the meta-tool E2E workflow
go test -v -tags e2e -timeout 300s -run TestMetaToolWorkflow ./test/e2e/
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
# Full suite
go test -v -tags e2e -timeout 300s ./test/e2e/
make test-e2e

# Compile-only (verify builds without GitLab)
go test -tags e2e -c -o NUL ./test/e2e/       # Windows
go test -tags e2e -c -o /dev/null ./test/e2e/  # Linux
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
├── setup_test.go             # Dual MCP server setup, helpers, shared state
├── workflow_test.go          # TestFullWorkflow (individual tools)
└── metatool_workflow_test.go # TestMetaToolWorkflow (meta-tools)
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
