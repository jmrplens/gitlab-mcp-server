// analyze_deployment_history.go implements the sampling-based deployment history analysis tool.

package samplingtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// AnalyzeDeploymentHistoryInput defines parameters for LLM-assisted deployment analysis.
type AnalyzeDeploymentHistoryInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	Environment string               `json:"environment,omitempty" jsonschema:"Filter by environment name (e.g. production, staging)"`
}

// AnalyzeDeploymentHistoryOutput holds the LLM analysis of deployment history.
type AnalyzeDeploymentHistoryOutput struct {
	toolutil.HintableOutput
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment,omitempty"`
	Analysis    string `json:"analysis"`
	Model       string `json:"model"`
	Truncated   bool   `json:"truncated"`
}

const analyzeDeploymentHistoryPrompt = `Analyze this deployment history and provide:
1. **Deployment frequency** — how often deployments happen, any patterns in timing
2. **Success rate** — ratio of successful vs failed deployments
3. **Rollback indicators** — identify any rollback patterns (same ref deployed multiple times)
4. **Environment health** — assess deployment stability per environment
5. **Deployment velocity** — is deployment frequency increasing or decreasing over time
6. **Risk assessment** — flag any concerning patterns (frequent failures, weekend deployments)
7. **Recommendations** — suggest improvements to the deployment process

Use specific dates, refs, and status data. Output Markdown only.`

// AnalyzeDeploymentHistory fetches recent deployments, then delegates to the MCP
// sampling capability for deployment pattern analysis.
func AnalyzeDeploymentHistory(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input AnalyzeDeploymentHistoryInput) (AnalyzeDeploymentHistoryOutput, error) {
	if input.ProjectID == "" {
		return AnalyzeDeploymentHistoryOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 4, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return AnalyzeDeploymentHistoryOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 4, "Fetching deployment history...")

	depList, err := deployments.List(ctx, client, deployments.ListInput{
		ProjectID:   input.ProjectID,
		Environment: input.Environment,
		OrderBy:     "created_at",
		Sort:        "desc",
		PaginationInput: toolutil.PaginationInput{
			PerPage: 100,
		},
	})
	if err != nil {
		return AnalyzeDeploymentHistoryOutput{}, fmt.Errorf("fetching deployments: %w", err)
	}

	if len(depList.Deployments) == 0 {
		envNote := ""
		if input.Environment != "" {
			envNote = " for environment " + input.Environment
		}
		return AnalyzeDeploymentHistoryOutput{
			ProjectID:   string(input.ProjectID),
			Environment: input.Environment,
			Analysis:    "No deployments found" + envNote + ".",
		}, nil
	}

	data := FormatDeploymentHistoryForAnalysis(depList, input.Environment)
	tracker.Step(ctx, 3, 4, "Requesting LLM analysis...")

	result, err := samplingClient.Analyze(ctx, analyzeDeploymentHistoryPrompt, data,
		sampling.WithMaxTokens(2048),
		sampling.WithTemperature(0.2),
		sampling.WithModelPriorities(0.4, 0.5, 0.5),
	)
	if err != nil {
		return AnalyzeDeploymentHistoryOutput{}, fmt.Errorf("LLM analysis: %w", err)
	}

	tracker.Step(ctx, 4, 4, "Analysis complete")

	return AnalyzeDeploymentHistoryOutput{
		ProjectID:   string(input.ProjectID),
		Environment: input.Environment,
		Analysis:    result.Content,
		Model:       result.Model,
		Truncated:   result.Truncated,
	}, nil
}

// FormatDeploymentHistoryForAnalysis builds a Markdown document from deployment
// records for LLM deployment pattern analysis.
func FormatDeploymentHistoryForAnalysis(depList deployments.ListOutput, environment string) string {
	var b strings.Builder
	title := "Deployment History"
	if environment != "" {
		title += " — " + environment
	}
	fmt.Fprintf(&b, "# %s (%d deployments)\n\n", title, len(depList.Deployments))

	// Summary stats.
	success, failed, other := 0, 0, 0
	for _, d := range depList.Deployments {
		switch d.Status {
		case "success":
			success++
		case "failed":
			failed++
		default:
			other++
		}
	}
	fmt.Fprintf(&b, "- **Success**: %d\n", success)
	fmt.Fprintf(&b, "- **Failed**: %d\n", failed)
	if other > 0 {
		fmt.Fprintf(&b, "- **Other**: %d\n", other)
	}

	b.WriteString("\n## Deployments\n\n")
	for _, d := range depList.Deployments {
		env := d.EnvironmentName
		if env == "" {
			env = "unknown"
		}
		fmt.Fprintf(&b, "- **#%d** [%s] ref=%s sha=%s env=%s user=%s created=%s\n",
			d.ID, d.Status, d.Ref, d.SHA, env, d.UserName, d.CreatedAt)
	}
	return b.String()
}

// FormatAnalyzeDeploymentHistoryMarkdown renders an LLM-generated deployment analysis.
func FormatAnalyzeDeploymentHistoryMarkdown(a AnalyzeDeploymentHistoryOutput) string {
	var b strings.Builder
	title := "Deployment History Analysis"
	if a.Environment != "" {
		title += " — " + a.Environment
	}
	fmt.Fprintf(&b, "## %s\n\n", title)
	if a.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Analysis was truncated due to size limits.*\n\n")
	}
	b.WriteString(a.Analysis)
	b.WriteString("\n")
	if a.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", a.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_list_deployments` to drill into specific deployments",
		"Use `gitlab_list_environments` to review environment configuration",
	)
	return b.String()
}
