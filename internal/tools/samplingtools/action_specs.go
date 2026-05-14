package samplingtools

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for LLM-assisted GitLab analysis actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		analyzeSpec("mr_changes", samplingRoute[AnalyzeMRChangesInput, AnalyzeMRChangesOutput](client, AnalyzeMRChanges), "gitlab_analyze_mr_changes"),
		analyzeSpec("issue_summary", samplingRoute[SummarizeIssueInput, SummarizeIssueOutput](client, SummarizeIssue), "gitlab_summarize_issue"),
		analyzeSpec("release_notes", samplingRoute[GenerateReleaseNotesInput, GenerateReleaseNotesOutput](client, GenerateReleaseNotes), "gitlab_generate_release_notes"),
		analyzeSpec("pipeline_failure", samplingRoute[AnalyzePipelineFailureInput, AnalyzePipelineFailureOutput](client, AnalyzePipelineFailure), "gitlab_analyze_pipeline_failure"),
		analyzeSpec("mr_review", samplingRoute[SummarizeMRReviewInput, SummarizeMRReviewOutput](client, SummarizeMRReview), "gitlab_summarize_mr_review"),
		analyzeSpec("milestone_report", samplingRoute[GenerateMilestoneReportInput, GenerateMilestoneReportOutput](client, GenerateMilestoneReport), "gitlab_generate_milestone_report"),
		analyzeSpec("ci_config", samplingRoute[AnalyzeCIConfigInput, AnalyzeCIConfigOutput](client, AnalyzeCIConfig), "gitlab_analyze_ci_configuration"),
		analyzeSpec("issue_scope", samplingRoute[AnalyzeIssueScopeInput, AnalyzeIssueScopeOutput](client, AnalyzeIssueScope), "gitlab_analyze_issue_scope"),
		analyzeSpec("mr_security", samplingRoute[ReviewMRSecurityInput, ReviewMRSecurityOutput](client, ReviewMRSecurity), "gitlab_review_mr_security"),
		analyzeSpec("technical_debt", samplingRoute[FindTechnicalDebtInput, FindTechnicalDebtOutput](client, FindTechnicalDebt), "gitlab_find_technical_debt"),
		analyzeSpec("deployment_history", samplingRoute[AnalyzeDeploymentHistoryInput, AnalyzeDeploymentHistoryOutput](client, AnalyzeDeploymentHistory), "gitlab_analyze_deployment_history"),
	}
}

func analyzeSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"analyze", "sampling"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "samplingtools",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
