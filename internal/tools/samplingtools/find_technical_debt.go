// find_technical_debt.go implements the sampling-based technical debt detection tool.
package samplingtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FindTechnicalDebtInput defines parameters for LLM-assisted technical debt discovery.
type FindTechnicalDebtInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ref       string               `json:"ref"        jsonschema:"Branch or tag to search in (defaults to default branch)"`
}

// FindTechnicalDebtOutput holds the LLM analysis of technical debt indicators.
type FindTechnicalDebtOutput struct {
	toolutil.HintableOutput
	ProjectID string `json:"project_id"`
	Analysis  string `json:"analysis"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

const findTechnicalDebtPrompt = `Analyze the technical debt indicators found in this codebase and provide:
1. **Summary** — overall technical debt level (low/medium/high) with total counts
2. **Categorized findings** — group by type (TODO, FIXME, HACK, XXX, DEPRECATED)
3. **Hotspots** — files/directories with the highest concentration of debt markers
4. **Priority items** — which items seem most urgent based on context and wording
5. **Recommendations** — suggest a prioritized approach to address the debt
6. **Patterns** — identify recurring themes or systemic issues

Be specific, quote the actual marker text, and reference file paths.`

// FindTechnicalDebt searches for debt markers (TODO, FIX-ME, HACK, XXX, DEPRECATED)
// in the project codebase, then delegates to the MCP sampling capability for analysis.
func FindTechnicalDebt(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input FindTechnicalDebtInput) (FindTechnicalDebtOutput, error) {
	if input.ProjectID == "" {
		return FindTechnicalDebtOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 4, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return FindTechnicalDebtOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 4, "Searching for technical debt markers...")

	markers := []string{"TODO", "FIXME", "HACK", "XXX", "DEPRECATED"}
	var allBlobs []search.BlobOutput
	for _, marker := range markers {
		result, err := search.Code(ctx, client, search.CodeInput{
			ProjectID: input.ProjectID,
			Query:     marker,
			Ref:       input.Ref,
			PaginationInput: toolutil.PaginationInput{
				PerPage: 50,
			},
		})
		if err != nil {
			continue
		}
		allBlobs = append(allBlobs, result.Blobs...)
	}

	if len(allBlobs) == 0 {
		return FindTechnicalDebtOutput{
			ProjectID: string(input.ProjectID),
			Analysis:  "No technical debt markers (TODO, FIXME, HACK, XXX, DEPRECATED) found in the codebase.",
		}, nil
	}

	data := FormatTechnicalDebtForAnalysis(allBlobs)
	tracker.Step(ctx, 3, 4, "Requesting LLM analysis...")

	result, err := samplingClient.Analyze(ctx, findTechnicalDebtPrompt, data,
		sampling.WithTemperature(0.1),
		sampling.WithModelPriorities(0.5, 0.5, 0.5),
	)
	if err != nil {
		return FindTechnicalDebtOutput{}, fmt.Errorf("LLM analysis: %w", err)
	}

	tracker.Step(ctx, 4, 4, "Analysis complete")

	return FindTechnicalDebtOutput{
		ProjectID: string(input.ProjectID),
		Analysis:  result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// FormatTechnicalDebtForAnalysis builds a Markdown document from code search
// results containing debt markers for LLM analysis.
func FormatTechnicalDebtForAnalysis(blobs []search.BlobOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Technical Debt Markers (%d results)\n\n", len(blobs))
	for _, blob := range blobs {
		fmt.Fprintf(&b, "### %s (line %d)\n\n", blob.Path, blob.Startline)
		fmt.Fprintf(&b, "```\n%s\n```\n\n", blob.Data)
	}
	return b.String()
}

// FormatFindTechnicalDebtMarkdown renders an LLM-generated technical debt analysis.
func FormatFindTechnicalDebtMarkdown(f FindTechnicalDebtOutput) string {
	var b strings.Builder
	b.WriteString("## Technical Debt Analysis\n\n")
	if f.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Analysis was truncated due to size limits.*\n\n")
	}
	b.WriteString(f.Analysis)
	b.WriteString("\n")
	if f.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", f.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_issue_create` to track debt items as issues",
		"Use `gitlab_label_create` to add a 'technical-debt' label for tracking",
	)
	return b.String()
}
