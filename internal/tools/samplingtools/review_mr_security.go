// review_mr_security.go implements the sampling-based merge request security review tool.

package samplingtools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ReviewMRSecurityInput defines parameters for LLM-assisted MR security review.
type ReviewMRSecurityInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// ReviewMRSecurityOutput holds the LLM security review of an MR.
type ReviewMRSecurityOutput struct {
	toolutil.HintableOutput
	MRIID     int64  `json:"merge_request_iid"`
	Title     string `json:"title"`
	Review    string `json:"review"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

const reviewMRSecurityPrompt = `Perform a security-focused review of this GitLab merge request. Analyze the code changes for:
1. **Injection vulnerabilities** — SQL injection, command injection, XSS, LDAP injection
2. **Authentication & authorization** — missing auth checks, privilege escalation, IDOR
3. **Sensitive data exposure** — hardcoded secrets, tokens, API keys, PII in logs
4. **Cryptographic issues** — weak algorithms, insecure random, missing encryption
5. **Input validation** — missing or insufficient validation, path traversal
6. **Dependency risks** — new dependencies with known vulnerabilities
7. **Security misconfigurations** — debug mode, permissive CORS, missing security headers
8. **OWASP Top 10 mapping** — map each finding to the relevant OWASP category

Rate each finding as: ` + toolutil.EmojiRed + ` CRITICAL, ` + toolutil.EmojiYellow + ` MEDIUM, or ` + toolutil.EmojiGreen + ` LOW.
Include specific file names and line context. If no security issues found, state so clearly.`

// ReviewMRSecurity fetches an MR and its diffs, then delegates to the MCP
// sampling capability for a security-focused code review.
func ReviewMRSecurity(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input ReviewMRSecurityInput) (ReviewMRSecurityOutput, error) {
	if input.ProjectID == "" {
		return ReviewMRSecurityOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return ReviewMRSecurityOutput{}, errors.New("merge_request_iid must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 4, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return ReviewMRSecurityOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 4, "Fetching merge request details and diffs...")

	mr, err := mergerequests.Get(ctx, client, mergerequests.GetInput{
		ProjectID: input.ProjectID,
		MRIID:     input.MRIID,
	})
	if err != nil {
		return ReviewMRSecurityOutput{}, fmt.Errorf("fetching MR: %w", err)
	}

	changes, err := mrchanges.Get(ctx, client, mrchanges.GetInput{
		ProjectID: input.ProjectID,
		MRIID:     input.MRIID,
	})
	if err != nil {
		return ReviewMRSecurityOutput{}, fmt.Errorf("fetching MR changes: %w", err)
	}

	// Reuse the same data format as code review — the security prompt
	// reinterprets the same diff data through a security lens.
	data := FormatMRForAnalysis(mr, changes)
	tracker.Step(ctx, 3, 4, "Requesting LLM security review...")

	result, err := samplingClient.Analyze(ctx, reviewMRSecurityPrompt, data,
		sampling.WithTemperature(0),
		sampling.WithModelPriorities(0, 0, 1),
	)
	if err != nil {
		return ReviewMRSecurityOutput{}, fmt.Errorf("LLM security review: %w", err)
	}

	tracker.Step(ctx, 4, 4, "Security review complete")

	return ReviewMRSecurityOutput{
		MRIID:     input.MRIID,
		Title:     mr.Title,
		Review:    result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// FormatReviewMRSecurityMarkdown renders an LLM-generated MR security review.
func FormatReviewMRSecurityMarkdown(r ReviewMRSecurityOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Security Review: !%d — %s\n\n", r.MRIID, toolutil.EscapeMdHeading(r.Title))
	if r.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Review was truncated due to size limits.*\n\n")
	}
	b.WriteString(r.Review)
	b.WriteString("\n")
	if r.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", r.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_add_mr_note` to flag security concerns on the MR",
		"Use `gitlab_issue_create` to track security findings as issues",
	)
	return b.String()
}
