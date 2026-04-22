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
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxx"
      }
    }
  }
}
```

### HTTP Mode

Start the server with HTTP transport for multi-client scenarios:

```bash
./gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --http-addr=:8080 \
  --max-http-clients=100
```

Clients connect to `http://localhost:8080/mcp` with their GitLab token in the `Authorization` header.

## Conversational Examples

You do not need to know tool names or parameters. Just ask naturally:

### "What merge requests need my review?"

The AI calls the `my_pending_reviews` prompt and returns a list of MRs assigned to you as reviewer, with clickable links to each one in GitLab.

### "Summarize what changed in MR !15 of my-app"

The AI calls `gitlab_analyze_mr_changes` which fetches the diff, sends it to the LLM for analysis, and returns a structured review covering: what changed, risk assessment, and suggestions.

### "Create an issue about fixing the login timeout"

The AI calls `gitlab_create_issue` with a title derived from your request. It may ask you for the project name and any labels before creating the issue.

### "Why did the latest pipeline fail?"

The AI calls `gitlab_analyze_pipeline_failure` which fetches the pipeline logs, identifies the failing job, and returns a plain-language explanation of what went wrong and how to fix it.

### "Generate release notes from v1.1 to v1.2"

The AI calls the `generate_release_notes` prompt which collects commits and MRs between the two tags and produces formatted release notes grouped by type (features, fixes, breaking changes).

---

## Common Workflows

### 1. Project Discovery

Use meta-tools for guided tool discovery:

```text
User: "What tools are available for merge requests?"
→ Call gitlab_merge_request with action "help"
→ Returns: list of all MR-related tools with descriptions
```

Individual tool approach:

```text
User: "List my projects"
→ Call gitlab_list_projects with owned=true
→ Returns: paginated list of projects with IDs and paths
```

### 2. Merge Request Lifecycle

#### Create a Branch and MR

```text
1. gitlab_create_branch(project_id="42", branch="feature/new-login", ref="main")
2. gitlab_create_or_update_file(project_id="42", branch="feature/new-login", ...)
3. gitlab_create_merge_request(project_id="42", source_branch="feature/new-login", target_branch="main", title="Add new login page")
```

#### Review a Merge Request

```text
1. gitlab_get_merge_request(project_id="42", mr_iid=15)
2. gitlab_list_mr_changes(project_id="42", mr_iid=15)
3. Prompt: review_mr(project_id="42", mr_iid="15")
   → Returns structured code review with risk categorization
```

#### Approve and Merge

```text
1. gitlab_approve_merge_request(project_id="42", mr_iid=15)
2. gitlab_accept_merge_request(project_id="42", mr_iid=15, squash=true)
```

### 3. Issue Management

```text
1. gitlab_create_issue(project_id="42", title="Fix login bug", labels=["bug", "P1"])
2. gitlab_update_issue(project_id="42", issue_iid=10, assignee_ids=[5])
3. gitlab_create_issue_note(project_id="42", issue_iid=10, body="Investigating...")
4. gitlab_close_issue(project_id="42", issue_iid=10)
```

### 4. CI/CD Pipeline Monitoring

```text
1. Resource: gitlab://project/42/pipelines/latest
   → Returns latest pipeline status
2. gitlab_list_pipeline_jobs(project_id="42", pipeline_id=100)
3. gitlab_get_job_log(project_id="42", job_id=500)
   → Returns job console output for debugging
```

### 5. Release Management

```text
1. gitlab_create_tag(project_id="42", tag_name="v1.2.0", ref="main", message="Release 1.2.0")
2. gitlab_create_release(project_id="42", tag_name="v1.2.0", name="Version 1.2.0", description="...")
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
Prompt: team_mr_dashboard(group_id="7")    → All group MRs with filters
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

## Meta-Tool Discovery

When `META_TOOLS=true` (default), 28 domain-level meta-tools (43 with GITLAB_ENTERPRISE=true) provide guided discovery:

```text
Call: gitlab_project(action="help")
→ Returns: all project-related tools with descriptions and parameters

Call: gitlab_merge_request(action="list", project_id="42")
→ Dispatches to gitlab_list_merge_requests with the given parameters
```

Available meta-tool domains: `project`, `branch`, `tag`, `release`, `merge_request`, `mr_review`, `repository`, `group`, `issue`, `pipeline`, `job`, `user`, `wiki`, `environment`, `deployment`, `pipeline_schedule`, `ci_variable`, `template`, `admin`, `access`, `package`, `snippet`, `feature_flags`, `search`, `runner`, `analyze_mr_changes`, `summarize_issue`.

## Sampling Tools

When the MCP client supports sampling, these tools delegate analysis to the LLM:

```text
gitlab_analyze_mr_changes(project_id="42", mr_iid=15)
→ Sends MR diff data to LLM for code review analysis

gitlab_summarize_issue(project_id="42", issue_iid=10)
→ Sends issue details to LLM for summarization
```

## Error Handling

All tools return actionable error messages that guide toward solutions:

```json
{
  "isError": true,
  "content": [{
    "type": "text",
    "text": "Project not found: '999'. Verify the project ID exists and your token has access. Use gitlab_list_projects to find valid project IDs."
  }]
}
```
