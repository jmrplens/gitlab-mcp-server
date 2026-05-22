package samplingtools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for LLM-assisted GitLab analysis actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		samplingSpec("mr_changes", client, AnalyzeMRChanges, "gitlab_analyze_mr_changes", analyzeMRChangesDescription()),
		samplingSpec("issue_summary", client, SummarizeIssue, "gitlab_summarize_issue", summarizeIssueDescription()),
		samplingSpec("release_notes", client, GenerateReleaseNotes, "gitlab_generate_release_notes", generateReleaseNotesDescription()),
		samplingSpec("pipeline_failure", client, AnalyzePipelineFailure, "gitlab_analyze_pipeline_failure", analyzePipelineFailureDescription()),
		samplingSpec("mr_review", client, SummarizeMRReview, "gitlab_summarize_mr_review", summarizeMRReviewDescription()),
		samplingSpec("milestone_report", client, GenerateMilestoneReport, "gitlab_generate_milestone_report", generateMilestoneReportDescription()),
		samplingSpec("ci_config", client, AnalyzeCIConfig, "gitlab_analyze_ci_configuration", analyzeCIConfigDescription()),
		samplingSpec("issue_scope", client, AnalyzeIssueScope, "gitlab_analyze_issue_scope", analyzeIssueScopeDescription()),
		samplingSpec("mr_security", client, ReviewMRSecurity, "gitlab_review_mr_security", reviewMRSecurityDescription()),
		samplingSpec("technical_debt", client, FindTechnicalDebt, "gitlab_find_technical_debt", findTechnicalDebtDescription()),
		samplingSpec("deployment_history", client, AnalyzeDeploymentHistory, "gitlab_analyze_deployment_history", analyzeDeploymentHistoryDescription()),
	}
}

func samplingSpec[T, R any](name string, client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error), individualTool, description string) toolutil.ActionSpec {
	return analyzeSpec(name, samplingRoute[T, R](client, fn, individualTool), individualTool, description)
}

func analyzeSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"analyze", "sampling"},
		OpenWorld:      true,
		OwnerPackage:   "samplingtools",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	})
}

func analyzeMRChangesDescription() string {
	return "Analyze a GitLab merge request using LLM-assisted code review via MCP sampling. " +
		"Fetches MR details and diffs, then requests LLM analysis for code quality, bugs, and improvements. " +
		samplingRequirement +
		"\n\nReturns: Markdown analysis of merge request changes including code quality, bugs, and improvement recommendations.\n\nSee also: gitlab_review_mr_security, gitlab_summarize_mr_review"
}

func summarizeIssueDescription() string {
	return "Summarize a GitLab issue discussion using LLM-assisted analysis via MCP sampling. " +
		"Fetches issue details and all notes, then requests LLM summary of key decisions and action items. " +
		samplingRequirement +
		"\n\nReturns: Markdown summary of the issue with key decisions and action items.\n\nSee also: gitlab_analyze_issue_scope, gitlab_issue_list"
}

func generateReleaseNotesDescription() string {
	return "Generate polished release notes using LLM-assisted analysis via MCP sampling. " +
		"Compares two Git refs, fetches commits and merged MRs with labels, then requests LLM to produce " +
		"categorized release notes (Features, Bug Fixes, Improvements, Breaking Changes). " +
		samplingRequirement +
		"\n\nReturns: Markdown release notes categorized by Features, Bug Fixes, Improvements, and Breaking Changes.\n\nSee also: gitlab_release_create, gitlab_commit_list"
}

func analyzePipelineFailureDescription() string {
	return "Analyze a GitLab pipeline failure using LLM-assisted root cause analysis via MCP sampling. " +
		"Fetches pipeline details, failed jobs and their traces, then requests LLM analysis for root cause, " +
		"fix suggestions, and impact assessment. " +
		samplingRequirement +
		"\n\nReturns: Markdown analysis of pipeline failure with root cause and suggested fixes.\n\nSee also: gitlab_pipeline_get, gitlab_get_job_trace"
}

func summarizeMRReviewDescription() string {
	return "Summarize a GitLab merge request review using LLM-assisted analysis via MCP sampling. " +
		"Fetches MR details, discussions, and approval state, then requests LLM summary of reviewer feedback, " +
		"unresolved threads, and action items. " +
		samplingRequirement +
		"\n\nReturns: Markdown summary of reviewer feedback, unresolved threads, and action items.\n\nSee also: gitlab_analyze_mr_changes, gitlab_mr_discussion_list"
}

func generateMilestoneReportDescription() string {
	return "Generate a comprehensive milestone progress report using LLM-assisted analysis via MCP sampling. " +
		"Fetches milestone details, linked issues and merge requests, then requests LLM to produce " +
		"a data-driven progress report with metrics, risks, and recommendations. " +
		samplingRequirement +
		"\n\nReturns: Markdown progress report with metrics, risks, and recommendations.\n\nSee also: gitlab_milestone_get, gitlab_list_milestone_issues"
}

func analyzeCIConfigDescription() string {
	return "Analyze a GitLab project's CI/CD configuration using LLM-assisted analysis via MCP sampling. " +
		"Lints the CI config, fetches merged YAML and includes, then requests LLM analysis for " +
		"best practices, performance, security, and maintainability. " +
		samplingRequirement +
		"\n\nReturns: Markdown analysis of CI/CD configuration covering best practices, performance, security, and maintainability.\n\nSee also: gitlab_ci_lint_project, gitlab_pipeline_list"
}

func analyzeIssueScopeDescription() string {
	return "Analyze a GitLab issue's scope and effort using LLM-assisted analysis via MCP sampling. " +
		"Fetches issue details, time stats, participants, related MRs, and discussion notes, then " +
		"requests LLM to assess scope, complexity, risks, and whether the issue should be broken down. " +
		samplingRequirement +
		"\n\nReturns: Markdown analysis of issue scope, complexity, risks, and breakdown recommendations.\n\nSee also: gitlab_summarize_issue, gitlab_issue_get"
}

func reviewMRSecurityDescription() string {
	return "Perform a security-focused review of a GitLab merge request using LLM-assisted analysis via MCP sampling. " +
		"Fetches MR details and code diffs, then requests LLM to identify injection vulnerabilities, " +
		"auth issues, exposed secrets, and OWASP Top 10 findings. " +
		samplingRequirement +
		"\n\nReturns: Markdown security review with vulnerability findings and OWASP Top 10 assessment.\n\nSee also: gitlab_analyze_mr_changes, gitlab_mr_get"
}

func findTechnicalDebtDescription() string {
	return "Find and analyze technical debt in a GitLab project using LLM-assisted analysis via MCP sampling. " +
		"Searches for TODO, FIXME, HACK, XXX, and DEPRECATED markers in source code, then requests LLM " +
		"to categorize, prioritize, and recommend a remediation strategy. " +
		samplingRequirement +
		"\n\nReturns: Markdown report of technical debt categorized by priority with remediation strategy.\n\nSee also: gitlab_search_code, gitlab_project_get"
}

func analyzeDeploymentHistoryDescription() string {
	return "Analyze deployment history and patterns for a GitLab project using LLM-assisted analysis via MCP sampling. " +
		"Fetches recent deployments, then requests LLM to assess deployment frequency, success rate, " +
		"rollback patterns, and suggest improvements. " +
		samplingRequirement +
		"\n\nReturns: Markdown analysis of deployment patterns with frequency, success rate, and improvement suggestions.\n\nSee also: gitlab_deployment_list, gitlab_environment_list"
}
