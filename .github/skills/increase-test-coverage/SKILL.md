---
name: increase-test-coverage
description: 'Increase Go test coverage to 90%+ using a Research → Plan → Implement pipeline. Analyzes coverage gaps, generates table-driven tests with httptest mocks, and validates results with go test -coverprofile. Designed for Go MCP server projects using the official go-sdk and gitlab.com/gitlab-org/api/client-go/v2.'
---

# Increase Test Coverage

## Primary Directive

Systematically increase Go test coverage to **90%+ per package** using a structured Research → Plan → Implement pipeline. Generate comprehensive, buildable, passing tests that follow project conventions and use proper mocks for external dependencies.

All tests must:

- Compile and pass on the first run
- Use existing test helpers and patterns found in the codebase
- Mock external dependencies (GitLab API) via `httptest`
- Follow table-driven test patterns with `t.Run()` subtests
- Cover happy paths, edge cases, and error scenarios
- **Include documentation (doc comment) explaining what each test validates and why**
- **Be verified against false passes** (assertions that never fail)
- Be written in English per project language policy

## Execution Context

This skill is designed for the `Test Expert` agent or any agent tasked with increasing test coverage. It operates on Go codebases that use the standard `testing` package, `net/http/httptest`, and optionally `testify/assert`.

## Pipeline Overview

```text
┌────────────────────────────────────────────────────────┐
│              INCREASE TEST COVERAGE                    │
│  Orchestrates the full Research → Plan → Implement     │
│  pipeline and tracks coverage progress                 │
└─────────────────────┬──────────────────────────────────┘
                      │
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
┌───────────┐  ┌───────────┐  ┌───────────────┐
│ RESEARCH  │  │   PLAN    │  │  IMPLEMENT    │
│           │  │           │  │               │
│ Measure   │  │ Prioritize│  │ Write tests   │
│ coverage  │→ │ gaps by   │→ │ per phase     │
│ & analyze │  │ package   │  │ & validate    │
│ gaps      │  │           │  │               │
└───────────┘  └───────────┘  └───────┬───────┘
                                      │
                              ┌───────┼───────┐
                              ▼       ▼       ▼
                          ┌───────┐┌──────┐┌───────┐
                          │ BUILD ││ TEST ││ FIX   │
                          │       ││      ││       │
                          │go vet ││go    ││repair │
                          │build  ││test  ││errors │
                          └───────┘└──────┘└───────┘
```

---

## Phase 1: Research

Thoroughly analyze the codebase and measure current coverage before writing any tests.

### Step 1: Measure Baseline Coverage

Run coverage analysis for the entire project:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Record the **per-package** and **per-function** coverage percentages. This is the baseline.

### Step 2: Generate HTML Coverage Report (Optional)

```bash
go tool cover -html=coverage.out -o coverage.html
```

Review the HTML report to visually identify uncovered code blocks.

### Step 3: Identify Coverage Gaps

For each package, analyze:

1. **Uncovered functions** — Functions with 0% coverage (highest priority)
2. **Partially covered functions** — Functions below 90% (error paths, edge cases)
3. **Untested branches** — Conditional logic where only one branch is covered
4. **Error handling paths** — `if err != nil` blocks that are never tested
5. **Edge cases** — Boundary conditions, nil inputs, empty slices, zero values

### Step 4: Discover Existing Test Patterns

Before writing any test, analyze the codebase for conventions:

| Convention | Where to Look |
|------------|---------------|
| Test helpers | `**/helpers_test.go` — `newTestClient()`, `respondJSON()`, `respondJSONWithPagination()` |
| Mock patterns | How `httptest.NewServer` + `http.HandlerFunc` are used to mock GitLab API |
| Assertion style | Whether `testify/assert` or stdlib `t.Errorf`/`t.Fatalf` is used |
| Test naming | Pattern: `TestFunctionName_Scenario` or `TestFunctionName_Scenario_ExpectedResult` |
| Table-driven tests | Look for `tests := []struct{}` or `cases := map[string]struct{}` patterns |
| File placement | Tests in same package (white-box) or `_test` package (black-box) |
| Pagination helpers | `paginationHeaders` struct and `respondJSONWithPagination()` |

### Step 5: Document Research Findings

Create a coverage analysis summary containing:

- Current overall coverage percentage
- Per-package coverage breakdown
- List of uncovered/partially covered functions ranked by priority
- Existing test patterns and helpers available
- Dependencies that need mocking (GitLab API endpoints)

---

## Phase 2: Plan

Create a phased implementation plan that prioritizes maximum coverage impact.

### Step 1: Prioritize Packages

Rank packages by coverage gap impact:

| Priority | Criteria |
|----------|----------|
| P0 — Critical | Core business logic with 0% coverage |
| P1 — High | Tool handlers, resource handlers below 80% |
| P2 — Medium | Helper functions, config loading below 90% |
| P3 — Low | Already above 90%, minor edge cases |

### Step 2: Group into Phases

Divide work into **2-5 phases**, each targeting a specific package or functional area:

```text
Phase 1: [package] — Current: X% → Target: 90%+
  - TestFunctionA_HappyPath
  - TestFunctionA_ErrorCase
  - TestFunctionA_EdgeCase_EmptyInput
  ...

Phase 2: [package] — Current: X% → Target: 90%+
  ...
```

### Step 3: Define Test Cases per Function

For each uncovered function, specify:

1. **Happy path** — Valid inputs producing expected outputs
2. **Error cases** — Invalid inputs, API failures (4xx, 5xx), network errors
3. **Edge cases** — Empty strings, zero values, nil pointers, large inputs
4. **Context cancellation** — Cancelled or timed-out contexts
5. **Pagination** — Multiple pages, empty results, single page

### Step 4: Identify Required Mocks

For each test, document:

- Which GitLab API endpoint needs mocking (method + path)
- Expected request body validation (if any)
- Response status code and JSON payload
- Pagination headers (if applicable)

### Step 5: Plan Structure

Present the plan to the user for confirmation before implementation. The plan should include:

- Phase breakdown with package targets
- Estimated number of new test functions per phase
- Expected coverage increase per phase
- New test helpers needed (if any)
- Total estimated coverage after all phases

---

## Phase 3: Implement

Execute the plan phase by phase, validating after each phase.

### Step 1: Implement One Phase at a Time

For each phase:

1. **Read source code** — Understand the function signatures, logic branches, and dependencies
2. **Write test file** — Create or extend `*_test.go` files following existing patterns
3. **Use existing helpers** — Reuse `newTestClient()`, `respondJSON()`, `respondJSONWithPagination()`
4. **Create new helpers if needed** — Add to `helpers_test.go` for reusable mock patterns

### Step 2: Test Writing Patterns

#### Table-Driven Tests (Preferred)

```go
func TestFunctionName_Scenarios(t *testing.T) {
    tests := []struct {
        name       string
        input      InputType
        mockStatus int
        mockBody   string
        wantErr    bool
        want       OutputType
    }{
        {
            name:       "happy path",
            input:      InputType{Field: "value"},
            mockStatus: http.StatusOK,
            mockBody:   `{"id": 1, "name": "test"}`,
            want:       OutputType{ID: 1, Name: "test"},
        },
        {
            name:       "not found",
            input:      InputType{Field: "missing"},
            mockStatus: http.StatusNotFound,
            mockBody:   `{"message": "404 Not found"}`,
            wantErr:    true,
        },
        {
            name:       "empty input",
            input:      InputType{},
            mockStatus: http.StatusBadRequest,
            mockBody:   `{"error": "field required"}`,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                respondJSON(w, tt.mockStatus, tt.mockBody)
            }))

            got, err := functionUnderTest(context.Background(), client, tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && got != tt.want {
                t.Errorf("got %+v, want %+v", got, tt.want)
            }
        })
    }
}
```

#### Request Validation Mocks

When testing POST/PUT operations, validate the request:

```go
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        t.Errorf("method = %s, want POST", r.Method)
    }
    if r.URL.Path != "/api/v4/projects/42/issues" {
        t.Errorf("path = %s, want /api/v4/projects/42/issues", r.URL.Path)
    }
    respondJSON(w, http.StatusCreated, `{...}`)
}))
```

#### Pagination Tests

```go
client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    respondJSONWithPagination(w, http.StatusOK, `[...]`, paginationHeaders{
        Page:       "1",
        PerPage:    "20",
        Total:      "50",
        TotalPages: "3",
        NextPage:   "2",
    })
}))
```

#### Context Cancellation Tests

```go
func TestFunction_ContextCancelled(t *testing.T) {
    client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        respondJSON(w, http.StatusOK, `{}`)
    }))

    ctx, cancel := context.WithCancel(context.Background())
    cancel()

    _, err := functionUnderTest(ctx, client, input)
    if err == nil {
        t.Fatal("expected error for cancelled context, got nil")
    }
}
```

### Step 3: Validate After Each Phase

After writing tests for a phase:

1. **Compile**: `go build ./...`
2. **Vet**: `go vet ./...`
3. **Run tests**: `go test -v ./internal/[package]/...`
4. **Measure coverage**: `go test -coverprofile=coverage.out ./internal/[package]/...`
5. **Check coverage**: `go tool cover -func=coverage.out | grep [package]`

If tests fail:

- Fix compilation errors immediately
- Adjust mock responses to match actual API behavior
- Verify test expectations match function implementation

### Step 4: Track Progress

After each phase, update progress:

```text
Package             Before    After     Target    Status
internal/tools      72%       91%       90%       ✅ Done
internal/gitlab     65%       —         90%       🔄 In Progress
internal/config     80%       —         90%       ⏳ Pending
internal/resources  70%       —         90%       ⏳ Pending
internal/prompts    68%       —         90%       ⏳ Pending
```

### Step 5: Final Validation

After all phases complete:

1. Run full test suite: `go test -race -coverprofile=coverage.out ./...`
2. Generate final coverage report: `go tool cover -func=coverage.out`
3. Verify every package meets 90%+ target
4. Run quality checks: `go vet ./...` and `staticcheck ./...` (if available)
5. Clean up any temporary files

---

## Test Quality Standards

### What to Test

| Category | Examples |
|----------|----------|
| Happy path | Valid inputs → expected output |
| Error responses | 400, 401, 403, 404, 422, 500 from GitLab API |
| Empty results | Empty arrays, null fields, missing optional fields |
| Input validation | Required fields missing, invalid IDs, empty strings |
| Pagination | First page, last page, single page, many pages |
| Context | Cancelled context, timed-out context |
| Edge cases | Very long strings, special characters, unicode, zero values |

### What NOT to Test

- Third-party library internals (GitLab client SDK, MCP SDK)
- Trivial getters/setters with no logic
- Generated code (JSON schema tags, struct definitions)
- The Go standard library itself

### Test Quality Checklist

- [ ] Tests are independent — no shared mutable state between tests
- [ ] Tests are deterministic — same result every run
- [ ] Test names describe behavior: `TestFunction_Scenario_ExpectedResult`
- [ ] Each test has clear Arrange-Act-Assert sections
- [ ] Mock responses match real GitLab API response formats
- [ ] Error messages include context: `"functionName() expected error, got nil"`
- [ ] `t.Helper()` is called in all test helper functions
- [ ] `t.Cleanup()` is used for resource cleanup
- [ ] No hardcoded sleep or timing dependencies
- [ ] Tests run fast (< 1 second per test function)

---

## Coverage Targets

| Package | Minimum Target | Stretch Goal |
|---------|---------------|-------------|
| `internal/tools` | 90% | 95% |
| `internal/resources` | 90% | 95% |
| `internal/prompts` | 90% | 95% |
| `internal/gitlab` | 90% | 95% |
| `internal/config` | 90% | 95% |
| `cmd/server` | 80% | 90% |
| **Overall** | **90%** | **95%** |

---

## Troubleshooting

### Tests don't compile

- Check import paths match the module path in `go.mod`
- Verify test file is in the correct package (same as source file)
- Ensure mock response JSON matches expected struct field names
- Run `go vet ./...` for quick diagnostics

### Tests fail unexpectedly

- Print actual vs expected values with `%+v` formatting
- Check mock handler serves the correct URL path and method
- Verify GitLab API response format (field names are snake_case in JSON)
- Check if function under test modifies input struct

### Coverage doesn't increase

- Ensure test exercises the uncovered code path (check HTML report)
- Look for unreachable code or dead branches
- Some error paths require specific mock setups (e.g., network errors)
- Test both `if` and `else` branches of conditionals

### Race conditions detected

- Use `go test -race ./...` to detect data races
- Ensure tests don't share mutable state
- Use `t.Parallel()` carefully — only when tests are truly independent
- Check that test helpers don't store state across calls
