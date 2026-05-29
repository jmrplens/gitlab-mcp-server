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
		samplingSpecWithAliases("mr_changes", client, AnalyzeMRChanges, "gitlab_analyze_mr_changes", analyzeMRChangesDescription(),
			"Analyze merge request diffs for code quality risks and improvement opportunities.",
			[]string{"analyze mr changes", "analyze merge request code", "code review analysis", "mr code quality"}),
		samplingSpecWithAliases("issue_summary", client, SummarizeIssue, "gitlab_summarize_issue", summarizeIssueDescription(),
			"Summarize issue discussions into key decisions and actionable next steps.",
			[]string{"summarize issue", "issue summary analysis", "get issue decisions", "issue key points"}),
		samplingSpecWithAliases("release_notes", client, GenerateReleaseNotes, "gitlab_generate_release_notes", generateReleaseNotesDescription(),
			"Generate categorized release notes from commits and merge requests after requested release/compare data is collected.",
			[]string{"generate release notes", "create release notes", "release changelog"}),
		samplingSpecWithAliases("pipeline_failure", client, AnalyzePipelineFailure, "gitlab_analyze_pipeline_failure", analyzePipelineFailureDescription(),
			"Investigate failed pipelines and identify likely root causes from jobs and traces.",
			[]string{"failure", "failed", "analyze pipeline failure", "pipeline failure analysis", "debug pipeline failure", "diagnose pipeline failure", "find pipeline root cause", "why pipeline failed", "pipeline diagnostics", "pipeline incident analysis"}),
		samplingSpecWithAliases("mr_review", client, SummarizeMRReview, "gitlab_summarize_mr_review", summarizeMRReviewDescription(),
			"Summarize merge request reviewer feedback, unresolved threads, and approvals.",
			[]string{"summarize mr review", "review feedback summary", "mr approval summary"}),
		samplingSpecWithAliases("milestone_report", client, GenerateMilestoneReport, "gitlab_generate_milestone_report", generateMilestoneReportDescription(),
			"Produce milestone progress reports with risks, blockers, and recommendations.",
			[]string{"generate milestone report", "milestone progress report", "milestone analysis"}),
		samplingSpecWithAliases("ci_config", client, AnalyzeCIConfig, "gitlab_analyze_ci_configuration", analyzeCIConfigDescription(),
			"Review CI/CD configuration for correctness, security, and maintainability.",
			[]string{"analyze ci config", "ci configuration analysis", "gitlab-ci.yml analysis", "ci pipeline analysis"}),
		samplingSpecWithAliases("issue_scope", client, AnalyzeIssueScope, "gitlab_analyze_issue_scope", analyzeIssueScopeDescription(),
			"Assess issue scope, complexity, and decomposition strategy.",
			[]string{"analyze issue scope", "issue scope analysis", "define issue requirements"}),
		samplingSpecWithAliases("mr_security", client, ReviewMRSecurity, "gitlab_review_mr_security", reviewMRSecurityDescription(),
			"Run a security-focused merge request review for OWASP risks and exposed secrets.",
			[]string{"security", "review", "vulnerability", "owasp", "review mr security", "security review", "security analysis", "vulnerability detection", "merge request security review", "security review merge request"}),
		samplingSpecWithAliases("technical_debt", client, FindTechnicalDebt, "gitlab_find_technical_debt", findTechnicalDebtDescription(),
			"Identify and prioritize technical debt markers in source code.",
			[]string{"find technical debt", "technical debt analysis", "code debt analysis"}),
		samplingSpecWithAliases("deployment_history", client, AnalyzeDeploymentHistory, "gitlab_analyze_deployment_history", analyzeDeploymentHistoryDescription(),
			"Analyze deployment trends, failure patterns, and rollback signals.",
			[]string{"analyze deployment history", "deployment analysis", "release history analysis"}),
	}
}

func samplingSpecWithAliases[T, R any](name string, client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error), individualTool, description, usage string, aliases []string) toolutil.ActionSpec {
	return analyzeSpec(name, samplingRoute[T, R](client, fn, individualTool), individualTool, description, usage, aliases)
}

func analyzeSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases []string) toolutil.ActionSpec {
	if len(aliases) == 0 {
		aliases = []string{individualTool}
	} else {
		aliases = append(aliases, individualTool)
	}
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:        aliases,
		Usage:          usage,
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
