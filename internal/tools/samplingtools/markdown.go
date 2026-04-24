// markdown.go registers Markdown formatting functions for sampling MCP tool output.

package samplingtools

import "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

func init() {
	toolutil.RegisterMarkdown(FormatAnalyzeMRChangesMarkdown)
	toolutil.RegisterMarkdown(FormatSummarizeIssueMarkdown)
	toolutil.RegisterMarkdown(FormatGenerateReleaseNotesMarkdown)
	toolutil.RegisterMarkdown(FormatAnalyzePipelineFailureMarkdown)
	toolutil.RegisterMarkdown(FormatSummarizeMRReviewMarkdown)
	toolutil.RegisterMarkdown(FormatGenerateMilestoneReportMarkdown)
	toolutil.RegisterMarkdown(FormatAnalyzeCIConfigMarkdown)
	toolutil.RegisterMarkdown(FormatAnalyzeIssueScopeMarkdown)
	toolutil.RegisterMarkdown(FormatReviewMRSecurityMarkdown)
	toolutil.RegisterMarkdown(FormatFindTechnicalDebtMarkdown)
	toolutil.RegisterMarkdown(FormatAnalyzeDeploymentHistoryMarkdown)
}
