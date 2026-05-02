# AI Agents & Skills

This project includes a comprehensive AI assistance infrastructure for development workflows. All agents, skills, and instruction files are located in `.github/` and are designed for use with GitHub Copilot and compatible AI assistants.

## Agents (7)

| Agent | Description |
|-------|-------------|
| **Go MCP Server Expert** | Primary coding agent for Go MCP development. Implements tools, fixes handlers, answers SDK questions. Has Context7 integration for up-to-date library docs. |
| **Test Expert** | Testing specialist: writes tests, analyzes coverage to 90%+, detects false passes, identifies edge cases, and refreshes `docs/development/testing.md` with `cmd/gen_testing_docs`. Uses Context7 for Go testing docs. |
| **Plan Expert** | Strategic planning for features, refactoring, architecture, tests, bugs, docs, and upgrades. Generates structured plans — does NOT write code. |
| **Debug Mode** | Systematic bug investigation with 4-phase workflow: reproduce → hypothesize → fix → verify. |
| **SE: Reviewer** | Security review (OWASP Top 10, Zero Trust, LLM security) and architecture review (Well-Architected frameworks, ADRs). |
| **Documentation Writer** | Generates project documentation using Diátaxis framework + Mermaid diagrams. Uses Context7 and web research for external references. |
| **Go Source Documenter** | Adds godoc-compliant doc comments to Go source and test files. Covers all symbol types. |

## Skills (18)

| Skill | Purpose |
|-------|---------|
| **create-implementation-plan** | Structured plan with phased tasks, saved to `plan/` |
| **create-specification** | Formal spec with requirements and acceptance criteria |
| **create-architectural-decision-record** | ADR with standardized format, saved to `docs/adr/` |
| **create-mcp-tool** | End-to-end workflow for creating a new MCP tool |
| **create-mcp-evaluation** | Generate Q&A pairs to benchmark MCP server quality |
| **increase-test-coverage** | Research → Plan → Implement pipeline to reach 90%+ coverage |
| **review-and-refactor** | Code quality review + MCP patterns + OWASP, then refactor |
| **generate-project-documentation** | Full documentation suite (architecture, API, onboarding) |
| **update-project-documentation** | Delta-update docs after code changes |
| **update-starlight-docs** | Update Astro Starlight user docs (EN/ES) when dev docs change |
| **generate-release-notes** | Categorized release notes between two Git refs |
| **go-source-documentation** | Add godoc-compliant comments to Go files |
| **go-safe-move-refactor** | Move Go files between packages with zero compilation downtime |
| **modularize-go-package** | Split monolithic package into domain sub-packages |
| **golang-testing** | Reference: table-driven tests, subtests, benchmarks, fuzzing |
| **golang-patterns** | Reference: error handling, concurrency, interfaces, memory |
| **git-commit** | Conventional commit with auto-detected type/scope from diff |
| **upstream-contribution** | Contribute fixes to upstream `gitlab.com/gitlab-org/api/client-go` |

## Instruction Files (7)

Instruction files in `.github/instructions/` are automatically applied when editing matching files:

| File | Applies To | Description |
|------|-----------|-------------|
| `go.instructions.md` | `**/*.go` | Idiomatic Go practices, naming, error handling |
| `go-mcp-server.instructions.md` | `**/*.go` | MCP server patterns: tool registration, typed I/O, annotations |
| `mcp-best-practices.instructions.md` | `**/*.go` | Protocol-level tool design, response formats, pagination |
| `security-and-owasp.instructions.md` | `*` | OWASP Top 10, input validation, secrets management |
| `code-review-generic.instructions.md` | `**` | Code review priorities and checklist |
| `context-engineering.instructions.md` | `**` | Project structure principles for AI-readable code |
| `self-explanatory-code-commenting.instructions.md` | `**` | Comment only WHY, not WHAT |

## Usage

Agents are invoked via GitHub Copilot Chat using `@agent-name`. Skills are task templates that can be triggered by any agent or directly in chat.

For the full catalog with detailed descriptions and workflows, see [CLAUDE.md](CLAUDE.md).
