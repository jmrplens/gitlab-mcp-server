---
name: generate-release-notes
description: 'Generate comprehensive, categorized release notes between two Git refs (tags, branches, commits). Compares refs to gather commits and merged MRs, then produces polished release notes organized by category (Features, Bug Fixes, Improvements, Breaking Changes, Documentation). Works with any GitLab project accessible via the MCP server.'
---

# Generate Release Notes

## Overview

Generate polished, categorized release notes for a GitLab project by comparing two Git refs. This skill orchestrates multiple MCP tools to gather commits, merged merge requests, and diffs, then produces human-readable release notes.

## When to Use

- Creating release notes for a new tag or version
- Summarizing changes between any two Git references (tags, branches, SHAs)
- Preparing changelog entries for a release
- Reviewing what changed between deployments

## Prerequisites

- A GitLab project accessible via the MCP server
- Two valid Git refs (tags, branches, or commit SHAs) to compare
- For LLM-assisted mode: MCP client must support the sampling capability

## Workflow

### Step 1: Identify the Project and Refs

Determine the project ID (or `namespace/project` path) and the two refs to compare.

Common patterns:

- **Tag to tag**: `v1.0.0` → `v1.1.0`
- **Tag to branch**: `v1.0.0` → `main`
- **Branch to branch**: `release/1.0` → `release/1.1`
- **Commit to tag**: `abc1234` → `v1.1.0`

### Step 2: Gather Data (Choose One Approach)

#### Approach A: LLM-Assisted (Recommended)

Use the `gitlab_generate_release_notes` sampling tool for fully automated, LLM-categorized release notes:

```text
Use gitlab_generate_release_notes with:
  - project_id: <project_id or "namespace/project">
  - from: <base_ref>  (e.g., "v1.0.0")
  - to: <target_ref>  (e.g., "v1.1.0")
```

This single tool call:

1. Compares the two refs to get commits and diffs
2. Fetches all merge requests merged in the date range
3. Sends the data to the LLM for intelligent categorization
4. Returns polished, formatted release notes

#### Approach B: Manual Orchestration

If sampling is not available, orchestrate the tools manually:

1. **Compare refs** to get commits and diffs:

   ```text
   Use gitlab_repository with action "compare":
     - project_id: <project_id>
     - from: <base_ref>
     - to: <target_ref>
   ```

2. **Search for merged MRs** in the change range:

   ```text
   Use gitlab_merge_request with action "list":
     - project_id: <project_id>
     - state: "merged"
     - per_page: 100
   ```

3. **Correlate commits with MRs** to enrich the data.

4. **Categorize changes** based on MR labels, commit prefixes, and descriptions.

#### Approach C: Use the MCP Prompt

For a prompt-based approach that provides enriched context to the LLM:

```text
Use the generate_release_notes prompt with:
  - project_id: <project_id>
  - from: <base_ref>
  - to: <target_ref>
```

### Step 3: Format the Release Notes

Organize changes into these categories (skip empty categories):

```markdown
## Release Notes: <from> → <to>

### Breaking Changes
- **!IID** — Description of breaking change (@author)

### Features
- **!IID** — New feature description (@author)

### Bug Fixes
- **!IID** — Bug fix description (@author)

### Improvements
- **!IID** — Improvement description (@author)

### Documentation
- **!IID** — Documentation change (@author)

### Other
- **!IID** — Other change description (@author)

---
**Full diff**: <compare_url>
**Contributors**: @user1, @user2, @user3
**Commits**: N | **Merge Requests**: M | **Files Changed**: F
```

## Categorization Rules

Assign each change to a category using these heuristics:

| Signal                  | Category          |
| ----------------------- | ----------------- |
| Label `breaking`        | Breaking Changes  |
| Commit prefix `feat:`   | Features          |
| Label `feature`         | Features          |
| Commit prefix `fix:`    | Bug Fixes         |
| Label `bug`             | Bug Fixes         |
| Commit prefix `docs:`   | Documentation     |
| Label `documentation`   | Documentation     |
| Commit prefix `perf:`   | Improvements      |
| Commit prefix `refactor:` | Improvements   |
| Label `enhancement`     | Improvements      |
| Everything else         | Other             |

**Priority**: Labels take precedence over commit prefixes. Breaking changes always come first.

## Writing Style

- Use **past tense** for descriptions ("Added", "Fixed", "Improved")
- Keep each entry to **one line** with the MR reference
- Include the **author** for attribution
- List **Breaking Changes first** — they need immediate attention
- Skip empty categories entirely
- Reference MR IIDs (e.g., `!42`) rather than commit SHAs for readability

## Example Output

```markdown
## Release Notes: v1.0.0 → v1.1.0

### Breaking Changes
- **!98** — Removed deprecated `/api/v1/users` endpoint (@alice)

### Features
- **!95** — Added webhook support for pipeline events (@bob)
- **!92** — Implemented project template cloning (@alice)

### Bug Fixes
- **!97** — Fixed race condition in concurrent MR updates (@carol)
- **!93** — Resolved TLS verification failure on self-signed certs (@bob)

### Improvements
- **!96** — Reduced API calls by caching project metadata (@carol)
- **!94** — Refactored pagination to use keyset pagination (@alice)

### Documentation
- **!91** — Updated installation guide for v1.1.0 (@bob)

---
**Full diff**: https://gitlab.example.com/group/project/-/compare/v1.0.0...v1.1.0
**Contributors**: @alice, @bob, @carol
**Commits**: 24 | **Merge Requests**: 8 | **Files Changed**: 37
```

## Tips

- For large ranges with many commits, the LLM-assisted approach (Approach A) produces the best results
- If the range spans hundreds of commits, the data may be truncated — the tool will indicate this
- Use specific tags rather than branch names for reproducible release notes
- The prompt-based approach (Approach C) gives you the most control over the final output since you can edit the LLM response
