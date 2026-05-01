package resources

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterWorkflowGuides registers static text resources with workflow
// best-practice content. These guides help LLMs provide consistent
// advice on git workflows, merge requests, commits, code reviews,
// and pipeline troubleshooting.
func RegisterWorkflowGuides(server *mcp.Server) {
	for _, g := range workflowGuides {
		guide := g // capture for closure
		server.AddResource(&mcp.Resource{
			URI:         guide.uri,
			Name:        guide.name,
			MIMEType:    "text/markdown",
			Icons:       toolutil.IconWiki,
			Description: guide.description,
		}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      guide.uri,
					MIMEType: "text/markdown",
					Text:     guide.content,
				}},
			}, nil
		})
	}
}

type workflowGuide struct {
	uri         string
	name        string
	description string
	content     string
}

var workflowGuides = []workflowGuide{
	{
		uri:         "gitlab://guides/git-workflow",
		name:        "git_workflow",
		description: "Best practices for Git branching strategies with GitLab (feature branches, trunk-based, GitLab Flow).",
		content: `# Git Workflow Best Practices

## Branch Naming
- Feature: ` + "`feature/<ticket>-<description>`" + `
- Bugfix: ` + "`fix/<ticket>-<description>`" + `
- Hotfix: ` + "`hotfix/<description>`" + `
- Release: ` + "`release/<version>`" + `

## Recommended Flow (GitLab Flow)
1. Create a feature branch from the default branch.
2. Make small, focused commits (see conventional-commits guide).
3. Push early and open a Draft MR to get CI feedback.
4. When ready, mark the MR as ready and request review.
5. After approval, merge using the project's merge strategy (merge commit, squash, or fast-forward).
6. Delete the source branch after merge.

## Tips
- Pull/rebase frequently to avoid large merge conflicts.
- Never force-push to shared branches.
- Use protected branches for main/release lines.
- Tag releases from the default branch.
`,
	},
	{
		uri:         "gitlab://guides/merge-request-hygiene",
		name:        "merge_request_hygiene",
		description: "Guidelines for creating and reviewing high-quality merge requests.",
		content: `# Merge Request Hygiene

## Creating a Good MR
- Keep MRs small and focused — one logical change per MR.
- Write a clear title following the pattern: ` + "`type: concise description`" + ` (e.g. ` + "`feat: add pagination to user list`" + `).
- Fill in the description template: what changed, why, how to test.
- Link related issues using ` + "`Closes #123`" + ` or ` + "`Relates to #456`" + `.
- Add labels for categorization (bug, feature, documentation).
- Assign reviewers explicitly.

## Reviewing an MR
- Check correctness, edge cases, and error handling.
- Verify tests cover new/changed behavior.
- Look for security issues (input validation, secrets, injections).
- Check for CI pipeline passing before approving.
- Be constructive — suggest improvements, don't just point out problems.

## After Merge
- Delete the source branch.
- Verify the pipeline on the target branch passes.
- Close related issues if not auto-closed.
`,
	},
	{
		uri:         "gitlab://guides/conventional-commits",
		name:        "conventional_commits",
		description: "Conventional commit message format and examples for consistent Git history.",
		content: `# Conventional Commits

## Format
` + "```" + `
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
` + "```" + `

## Types
| Type       | When to use                                      |
|------------|--------------------------------------------------|
| feat       | New feature                                      |
| fix        | Bug fix                                          |
| docs       | Documentation only                               |
| style      | Formatting, missing semi-colons (no code change) |
| refactor   | Code restructuring without behavior change       |
| perf       | Performance improvement                          |
| test       | Adding or correcting tests                       |
| build      | Build system or external dependencies            |
| ci         | CI configuration changes                         |
| chore      | Maintenance tasks                                |
| revert     | Reverts a previous commit                        |

## Examples
- ` + "`feat(auth): add OAuth2 login support`" + `
- ` + "`fix(api): handle nil pointer in user endpoint`" + `
- ` + "`docs: update README with Docker instructions`" + `
- ` + "`test(pipeline): add integration tests for stage validation`" + `

## Breaking Changes
Add ` + "`!`" + ` after type/scope or a ` + "`BREAKING CHANGE:`" + ` footer:
` + "```" + `
feat(api)!: change authentication endpoint path

BREAKING CHANGE: /auth/login is now /api/v2/auth/login
` + "```" + `
`,
	},
	{
		uri:         "gitlab://guides/code-review",
		name:        "code_review",
		description: "Code review checklist and best practices for GitLab merge request reviews.",
		content: `# Code Review Checklist

## Correctness
- [ ] Logic is correct and handles edge cases
- [ ] Error handling is appropriate (no silent failures)
- [ ] No race conditions in concurrent code
- [ ] Input validation at system boundaries

## Security
- [ ] No secrets or tokens in code
- [ ] User input is sanitized
- [ ] SQL queries use parameterized statements
- [ ] Authentication/authorization checks are in place

## Testing
- [ ] New code has unit tests
- [ ] Tests cover happy path and error cases
- [ ] Tests are deterministic and independent
- [ ] No commented-out or skipped tests

## Quality
- [ ] Code is readable and self-documenting
- [ ] No code duplication
- [ ] Functions are small and focused
- [ ] Naming is clear and consistent

## Performance
- [ ] No N+1 query patterns
- [ ] Resources are properly cleaned up (connections, files)
- [ ] Large result sets are paginated

## Documentation
- [ ] Public APIs have doc comments
- [ ] Complex logic has explanatory comments (WHY, not WHAT)
- [ ] Breaking changes are documented
`,
	},
	{
		uri:         "gitlab://guides/pipeline-troubleshooting",
		name:        "pipeline_troubleshooting",
		description: "Common GitLab CI/CD pipeline issues and how to diagnose and fix them.",
		content: `# Pipeline Troubleshooting

## Common Failures

### Job Stuck in Pending
- Check runner availability: are shared or project runners online?
- Verify tags match between job and runner configuration.
- Check runner concurrency limits.

### Build Failures
- Read the full job log — the error is usually near the end.
- Check if dependencies changed (lock file out of sync).
- Verify Docker image is available and correct tag exists.
- Look for environment variable issues (missing secrets).

### Test Failures
- Compare with the same test on the default branch (regression?).
- Check for flaky tests: re-run the job to see if it passes.
- Look for timing-dependent or order-dependent tests.

### Deployment Failures
- Verify deployment credentials and permissions.
- Check target environment health (disk space, connectivity).
- Review recent infrastructure changes.

## Diagnostic Steps
1. Check pipeline status: ` + "`gitlab_pipeline_get`" + ` for pipeline details.
2. List jobs: ` + "`gitlab_job_list`" + ` to find the failing job.
3. Read job log: ` + "`gitlab_job_log`" + ` for detailed output.
4. Check variables: ` + "`gitlab_ci_variable_list`" + ` for missing configuration.
5. Retry the job: ` + "`gitlab_job_retry`" + ` for transient failures.

## Prevention
- Pin dependency versions in lock files.
- Use caching for build dependencies.
- Run linters before tests to catch issues early.
- Set appropriate timeouts on all jobs.
`,
	},
}
