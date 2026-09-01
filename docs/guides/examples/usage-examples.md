# Usage Examples

This document provides practical usage examples for gitlab-mcp-server, demonstrating common workflows with MCP tools, resources, and prompts.

> **Diátaxis type**: How-to
> **Audience**: 👤 End users, AI assistant users
> **Prerequisites**: gitlab-mcp-server configured and running

---

## Setup

### Stdio Mode (Default)

Configure your MCP client (VS Code, Cursor, Copilot CLI, OpenCode) with:

```json
{
  "mcpServers": {
    "gitlab-mcp-server": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxx"
      }
    }
  }
}
```

For self-managed GitLab, add `GITLAB_URL=https://gitlab.example.com`.

### HTTP Mode

Start the server with HTTP transport for multi-client scenarios:

```bash
./gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --http-addr=:8080 \
  --max-http-clients=100
```

Replace `https://gitlab.com` with your self-managed GitLab URL when needed.

Clients connect to `http://localhost:8080/mcp` with their GitLab token in the `Authorization` header.

## Conversational Examples

You do not need to know tool names or parameters. Just ask naturally:

### "What merge requests need my review?"

The AI calls the `my_pending_reviews` prompt and returns a list of MRs assigned to you as reviewer, with clickable links to each one in GitLab.

### "Create an issue about fixing the login timeout"

The AI finds and runs the `issue.create` action with a title derived from your request. It may ask you for the project name and any labels before creating the issue.

### "Generate release notes from v1.1 to v1.2"

The AI calls the `generate_release_notes` prompt which collects commits and MRs between the two tags and produces formatted release notes grouped by type (features, fixes, breaking changes).

---

## Common Workflows

The individual tool names below assume `TOOL_SURFACE=individual`. On the default `dynamic` surface the same operations are reached through `gitlab_execute_action` with the matching canonical action ID — `branch.create`, `merge_request.create`, `issue.update`, `job.trace` — as shown under [Dynamic Discovery and Execution](#dynamic-discovery-and-execution).

### 1. Project Discovery

Discover what a domain covers on the default dynamic surface:

```text
User: "What tools are available for merge requests?"
→ Call gitlab_find_action(query="merge request") for ranked actions with inline schemas, or read gitlab://tools and filter the merge_request.* entries
→ Read gitlab://tools/merge_request.list for one action's call shape and schema
```

With `TOOL_SURFACE=meta` the same information is under `gitlab://tools/gitlab_merge_request.<action>`, and the `gitlab_merge_request` tool description carries the action list.

Individual tool approach:

```text
User: "List my projects"
→ Call gitlab_project_list with owned=true
→ Returns: paginated list of projects with IDs and paths
```

### 2. Merge Request Lifecycle

#### Create a Branch and MR

```text
1. gitlab_branch_create(project_id="42", branch="feature/new-login", ref="main")
2. gitlab_file_create(project_id="42", branch="feature/new-login", ...)
3. gitlab_mr_create(project_id="42", source_branch="feature/new-login", target_branch="main", title="Add new login page")
```

#### Review a Merge Request

```text
1. gitlab_mr_get(project_id="42", merge_request_iid=15)
2. gitlab_mr_changes_get(project_id="42", merge_request_iid=15)
3. Prompt: review_mr(project_id="42", merge_request_iid="15")
   → Returns structured code review with risk categorization
```

#### Approve and Merge

```text
1. gitlab_mr_approve(project_id="42", merge_request_iid=15)
2. gitlab_mr_merge(project_id="42", merge_request_iid=15, squash=true)
```

### 3. Issue Management

```text
1. gitlab_issue_create(project_id="42", title="Fix login bug", labels=["bug", "P1"])
2. gitlab_issue_update(project_id="42", issue_iid=10, assignee_ids=[5])
3. gitlab_issue_note_create(project_id="42", issue_iid=10, body="Investigating...")
4. gitlab_issue_update(project_id="42", issue_iid=10, state_event="close")
```

### 4. CI/CD Pipeline Monitoring

```text
1. Resource: gitlab://project/42/pipelines/latest
   → Returns latest pipeline status
2. gitlab_job_list(project_id="42", pipeline_id=100, scope=["failed"])
3. gitlab_job_trace(project_id="42", job_id=500)
   → Returns job console output for debugging
```

### 5. Release Management

```text
1. gitlab_tag_create(project_id="42", tag_name="v1.2.0", ref="main", message="Release 1.2.0")
2. gitlab_release_create(project_id="42", tag_name="v1.2.0", name="Version 1.2.0", description="...")
3. Prompt: generate_release_notes(project_id="42", from="v1.1.0", to="v1.2.0")
   → Returns structured release notes from commits between tags
```

### 6. Team Dashboards

#### Personal Dashboard

```text
Prompt: my_open_mrs()           → All your open MRs across projects
Prompt: my_pending_reviews()    → MRs waiting for your review
Prompt: my_issues()             → All issues assigned to you
Prompt: daily_standup(project_id="42") → Your standup summary
```

#### Manager Dashboard

```text
Prompt: team_overview(group_id="7")        → Team member workloads
Prompt: reviewer_workload(group_id="7")    → Review distribution analysis
Prompt: group_mr_dashboard(group_id="7")   → All group MRs with filters
Prompt: user_activity_report(group_id="7", username="johndoe") → Individual report
```

### 7. Project Health Monitoring

```text
Prompt: project_health_check(project_id="42")
→ Returns: pipeline status, open MR summary, branch hygiene, recommendations

Prompt: stale_items_report(project_id="42", stale_days="30")
→ Returns: MRs and issues not updated in 30+ days

Prompt: milestone_progress(project_id="42")
→ Returns: completion percentages for all active milestones
```

## Using Resources

Resources provide read-only data via URI patterns:

```text
gitlab://user/current                              → Your profile
gitlab://groups                                    → All accessible groups
gitlab://project/42                                → Project metadata
gitlab://project/42/members                        → Project members
gitlab://project/42/labels                         → Project labels
gitlab://project/42/milestones                     → Project milestones
gitlab://project/42/branches                       → Project branches
gitlab://project/42/issues                         → Open issues
gitlab://project/42/releases                       → Project releases
gitlab://project/42/tags                           → Repository tags
gitlab://project/42/pipelines/latest               → Latest pipeline
gitlab://project/42/pipeline/100                   → Specific pipeline
gitlab://project/42/pipeline/100/jobs              → Pipeline jobs
gitlab://project/42/mr/15                          → Specific MR
gitlab://project/42/issue/10                       → Specific issue
gitlab://group/7                                   → Group details
gitlab://group/7/members                           → Group members
gitlab://group/7/projects                          → Group projects
```

## Dynamic Discovery and Execution

Dynamic mode is the default tool surface. The model first searches the canonical action catalog, then executes one validated action with exact parameters:

```text
Call: gitlab_find_action(query="list open merge requests")
→ Returns: merge_request.list with input schema, examples, safety metadata, and output summary

Call: gitlab_execute_action(action="merge_request.list", params={project_id:"42", state:"opened"})
→ Executes the GitLab API request and returns Markdown plus structured content
```

Use this flow when startup context or visible tool count matters. It reaches the same catalog as meta-tools and individual tools while exposing only `gitlab_find_action` and `gitlab_execute_action` in `tools/list`.

## Meta-Tool Discovery

With `TOOL_SURFACE=meta`, 32 domain-level meta-tools (48 on self-managed Enterprise/Premium, 49 on GitLab.com Enterprise/Premium with Orbit) provide domain dispatcher tools:

```text
Resource: gitlab://tools/gitlab_project
→ Returns: every project action with its guidance (the tool's own description carries the same text)

Call: gitlab_merge_request(action="list", params={project_id:"42"})
→ Dispatches to the merge_request.list route with the given parameters
```

A meta-tool accepts only the top-level keys `action` and `params`; anything else is rejected as an unknown property.

Available meta-tool domains: `access`, `admin`, `branch`, `ci_catalog`, `ci_variable`, `custom_emoji`, `environment`, `feature_flags`, `group`, `issue`, `job`, `merge_request`, `model_registry`, `mr_review`, `package`, `pipeline`, `project`, `release`, `repository`, `runner`, `search`, `server`, `snippet`, `storage_move`, `tag`, `template`, `user`, `wiki` — plus `discover_project` and the four `interactive_*` creation flows, which are standalone tools rather than domain dispatchers. Labels, milestones and members are actions on `gitlab_project` and `gitlab_group` (`label_list`, `milestone_get`, `members`); MR diffs and discussions are actions on `gitlab_mr_review` (`changes_get`, `discussion_list`); CI lint is `gitlab_template` with `action: lint`.

## Error Handling

All tools return actionable error messages that guide toward solutions:

```json
{
  "isError": true,
  "content": [{
    "type": "text",
    "text": "Project not found: '999'. Verify the project ID exists and your token has access. Use gitlab_project_list to find valid project IDs."
  }]
}
```
