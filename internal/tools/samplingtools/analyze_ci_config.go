// analyze_ci_config.go implements the sampling-based CI configuration analysis tool.
package samplingtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// AnalyzeCIConfigInput defines parameters for LLM-assisted CI configuration analysis.
type AnalyzeCIConfigInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	ContentRef string               `json:"content_ref" jsonschema:"Branch or tag for CI content (defaults to default branch)"`
}

// AnalyzeCIConfigOutput holds the LLM analysis of a CI configuration.
type AnalyzeCIConfigOutput struct {
	toolutil.HintableOutput
	ProjectID string `json:"project_id"`
	Valid     bool   `json:"valid"`
	Analysis  string `json:"analysis"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

const analyzeCIConfigPrompt = `Analyze this GitLab CI/CD configuration and provide:
1. **Validation status** — report any errors or warnings found by the linter
2. **Structure overview** — summarize stages, jobs, and their dependencies
3. **Best practices** — check for CI/CD best practices (caching, artifacts, rules vs only/except)
4. **Performance suggestions** — identify opportunities for parallelism, caching, or faster execution
5. **Security review** — flag any exposed secrets, insecure patterns, or missing security jobs
6. **Maintainability** — assess use of includes, templates, anchors, and overall organization

Be specific and reference job names. Output Markdown only.`

// AnalyzeCIConfig lints a project's CI configuration and delegates to the MCP
// sampling capability for LLM-assisted analysis of the configuration quality.
func AnalyzeCIConfig(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input AnalyzeCIConfigInput) (AnalyzeCIConfigOutput, error) {
	if input.ProjectID == "" {
		return AnalyzeCIConfigOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 4, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return AnalyzeCIConfigOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 4, "Linting CI configuration...")

	lintResult, err := cilint.LintProject(ctx, client, cilint.ProjectInput{
		ProjectID:   input.ProjectID,
		ContentRef:  input.ContentRef,
		IncludeJobs: new(true),
	})
	if err != nil {
		return AnalyzeCIConfigOutput{}, fmt.Errorf("linting CI config: %w", err)
	}

	// Detect missing .gitlab-ci.yml before wasting a sampling call.
	if !lintResult.Valid && isMissingCIConfig(lintResult.Errors) {
		return AnalyzeCIConfigOutput{}, fmt.Errorf(
			"project %s has no .gitlab-ci.yml — add a CI/CD configuration first, then retry",
			input.ProjectID,
		)
	}

	data := FormatCIConfigForAnalysis(lintResult)
	tracker.Step(ctx, 3, 4, "Requesting LLM analysis...")

	result, err := samplingClient.Analyze(ctx, analyzeCIConfigPrompt, data,
		sampling.WithTemperature(0.2),
		sampling.WithModelPriorities(0.3, 0.3, 0.7),
	)
	if err != nil {
		return AnalyzeCIConfigOutput{}, fmt.Errorf("LLM analysis: %w", err)
	}

	tracker.Step(ctx, 4, 4, "Analysis complete")

	return AnalyzeCIConfigOutput{
		ProjectID: string(input.ProjectID),
		Valid:     lintResult.Valid,
		Analysis:  result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// FormatCIConfigForAnalysis builds a Markdown document from CI lint results
// for LLM configuration analysis.
func FormatCIConfigForAnalysis(lint cilint.Output) string {
	var b strings.Builder
	b.WriteString("# CI/CD Configuration Analysis\n\n")
	fmt.Fprintf(&b, "- **Valid**: %v\n", lint.Valid)

	if len(lint.Errors) > 0 {
		fmt.Fprintf(&b, "\n## Errors (%d)\n\n", len(lint.Errors))
		for _, e := range lint.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}

	if len(lint.Warnings) > 0 {
		fmt.Fprintf(&b, "\n## Warnings (%d)\n\n", len(lint.Warnings))
		for _, w := range lint.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}

	if len(lint.Includes) > 0 {
		fmt.Fprintf(&b, "\n## Includes (%d)\n\n", len(lint.Includes))
		for _, inc := range lint.Includes {
			if inc.ContextProject != "" {
				fmt.Fprintf(&b, "- [%s] %s (from %s)\n", inc.Type, inc.Location, inc.ContextProject)
			} else {
				fmt.Fprintf(&b, "- [%s] %s\n", inc.Type, inc.Location)
			}
		}
	}

	if lint.MergedYaml != "" {
		yaml := lint.MergedYaml
		// Truncate large YAML to keep within reasonable LLM context.
		const maxYamlLen = 50000
		if len(yaml) > maxYamlLen {
			yaml = yaml[:maxYamlLen] + "\n... (truncated)"
		}
		fmt.Fprintf(&b, "\n## Merged YAML\n\n```yaml\n%s\n```\n", yaml)
	}
	return b.String()
}

// FormatAnalyzeCIConfigMarkdown renders an LLM-generated CI config analysis.
func FormatAnalyzeCIConfigMarkdown(a AnalyzeCIConfigOutput) string {
	var b strings.Builder
	validStr := "Valid " + toolutil.EmojiSuccess
	if !a.Valid {
		validStr = "Invalid " + toolutil.EmojiCross
	}
	fmt.Fprintf(&b, "## CI Configuration Analysis (%s)\n\n", validStr)
	if a.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Analysis was truncated due to size limits.*\n\n")
	}
	b.WriteString(a.Analysis)
	b.WriteString("\n")
	if a.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", a.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_ci_lint` to validate updated CI configuration",
		"Use `gitlab_ci_variable_get` to review referenced variables",
	)
	return b.String()
}

// isMissingCIConfig returns true when the lint errors indicate the project
// has no .gitlab-ci.yml file at all.
func isMissingCIConfig(errs []string) bool {
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e), "provide content of .gitlab-ci.yml") {
			return true
		}
	}
	return false
}
