---
name: review-and-refactor
description: 'Review and refactor code in your project according to defined instructions. Use when the user asks to review code quality, refactor for best practices, check for OWASP compliance, verify MCP patterns, or improve overall code health.'
---

# Review and Refactor

## Role

You are a senior expert software engineer with extensive experience in maintaining projects over a long time and ensuring clean code, security, and best practices.

## Process

Follow this structured three-phase approach:

### Phase 1: Context Gathering

1. Read all coding guidelines in `.github/instructions/*.md` and `.github/copilot-instructions.md`
2. Identify the scope of files to review (user-specified or full project)
3. Understand the project architecture and conventions

### Phase 2: Review

Review code systematically, checking for:

#### Code Quality

- Naming conventions and readability
- Single Responsibility Principle compliance
- DRY violations and code duplication
- Error handling completeness and actionable messages
- Context cancellation respect (for Go/MCP)

#### MCP-Specific (if applicable)

- Tool naming follows snake_case with service prefix
- Tool annotations set (readOnlyHint, destructiveHint, idempotentHint, openWorldHint)
- Pagination metadata present for list operations
- Response format support (JSON + Markdown)

#### Security

- No hardcoded secrets or credentials
- Input validation on all user inputs
- Proper error messages (no internal details exposed)

#### Testing

- Test coverage for critical paths
- Descriptive test names
- Table-driven tests where appropriate

### Phase 3: Refactor

1. Prioritize issues: Critical (security, correctness) → Important (quality, tests) → Suggestions (readability)
2. Apply refactorings while keeping existing file structure intact
3. Verify tests still pass after changes
4. Summarize all changes made with rationale
