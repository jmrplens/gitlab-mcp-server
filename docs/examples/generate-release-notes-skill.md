# Generate Release Notes — Copilot Skill

> **Copy this file** to your project's `.github/skills/generate-release-notes/SKILL.md` to enable the
> `generate-release-notes` skill in GitHub Copilot.
>
> **Original location**: [.github/skills/generate-release-notes/SKILL.md](../../.github/skills/generate-release-notes/SKILL.md)

This is a ready-to-use [GitHub Copilot Skill](https://code.visualstudio.com/docs/copilot/customization/custom-instructions)
that generates categorized release notes for any GitLab project by comparing two
Git refs. It orchestrates gitlab-mcp-server MCP tools to gather commits, merged merge
requests, and diffs, then produces polished release notes.

## Quick Setup

1. Copy the skill folder into your project:

   ```text
   your-project/
   └── .github/
       └── skills/
           └── generate-release-notes/
               └── SKILL.md    ← copy from this project
   ```

2. The skill becomes available in Copilot Chat automatically.

3. Ask Copilot: *"Generate release notes from v1.0.0 to v1.1.0 for project 42"*

## Three Approaches

The skill documents three complementary approaches:

| Approach | Tool | Requires | Best For |
| -------- | ---- | -------- | -------- |
| **A. LLM-Assisted** | `gitlab_generate_release_notes` | MCP Sampling | Fully automated, categorized notes |
| **B. Manual** | `gitlab_repository` + `gitlab_merge_request` | Nothing extra | Full control, no LLM needed |
| **C. Prompt-Based** | `generate_release_notes` prompt | LLM client | Editable LLM-enriched context |

## Categories

Changes are automatically sorted into:

- **Breaking Changes** — label `breaking` or commit prefix `feat!:`
- **Features** — label `feature` or commit prefix `feat:`
- **Bug Fixes** — label `bug` or commit prefix `fix:`
- **Improvements** — label `enhancement` or prefixes `perf:`, `refactor:`
- **Documentation** — label `documentation` or commit prefix `docs:`
- **Other** — everything else

## Prerequisites

- gitlab-mcp-server MCP server running and connected
- A GitLab project with two valid Git refs to compare
- For Approach A: MCP client must support the [Sampling capability](../capabilities/sampling.md)

## Related

- [Full skill source](../../.github/skills/generate-release-notes/SKILL.md) — complete workflow with examples
- [Capabilities — Sampling](../capabilities/sampling.md) — how LLM-assisted tools work
- [Usage Examples](usage-examples.md) — more MCP tool usage scenarios
