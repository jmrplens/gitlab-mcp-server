// summarize_mr_review.go implements the sampling-based merge request review summarization tool.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// SummarizeMRReviewInput defines parameters for LLM-assisted MR review summarization.
type SummarizeMRReviewInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
}

// SummarizeMRReviewOutput holds the LLM summary of an MR review.
type SummarizeMRReviewOutput struct {
	toolutil.HintableOutput
	MRIID     int64  `json:"merge_request_iid"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

const summarizeMRReviewPrompt = `Summarize this GitLab merge request review and provide:
1. **Review status** — overall approval state, how many approvals received vs required
2. **Discussion summary** — key points raised by reviewers, grouped by theme
3. **Unresolved threads** — list any open/unresolved discussions that need attention
4. **Consensus** — areas of agreement and disagreement among reviewers
5. **Action items** — what the author needs to address before merge
6. **Positive feedback** — note any praised aspects of the implementation

Focus on reviewer feedback, not code changes.`

// SummarizeMRReview fetches an MR, its discussions, and approval state,
// then delegates to the MCP sampling capability for review summarization.
func SummarizeMRReview(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input SummarizeMRReviewInput) (SummarizeMRReviewOutput, error) {
	if input.ProjectID == "" {
		return SummarizeMRReviewOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return SummarizeMRReviewOutput{}, errors.New("merge_request_iid must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 5, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return SummarizeMRReviewOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 5, "Fetching merge request context...")

	var data, title string

	// Try GraphQL aggregation (single request replaces 3 REST calls) with fallback.
	gqlResult, gqlErr := BuildMRContext(ctx, client, string(input.ProjectID), input.MRIID)
	if gqlErr == nil {
		data = gqlResult.Content
		title = gqlResult.Title
	} else {
		mr, err := mergerequests.Get(ctx, client, mergerequests.GetInput{
			ProjectID: input.ProjectID,
			MRIID:     input.MRIID,
		})
		if err != nil {
			return SummarizeMRReviewOutput{}, fmt.Errorf("fetching MR: %w", err)
		}
		title = mr.Title

		tracker.Step(ctx, 3, 5, "Fetching discussions and approval state...")

		discussions, err := mrdiscussions.List(ctx, client, mrdiscussions.ListInput{
			ProjectID: input.ProjectID,
			MRIID:     input.MRIID,
			PaginationInput: toolutil.PaginationInput{
				PerPage: 100,
			},
		})
		if err != nil {
			return SummarizeMRReviewOutput{}, fmt.Errorf("fetching discussions: %w", err)
		}

		approvalState, _ := mrapprovals.State(ctx, client, mrapprovals.StateInput{
			ProjectID: input.ProjectID,
			MRIID:     input.MRIID,
		})
		data = FormatMRReviewForAnalysis(mr, discussions, approvalState)
	}

	tracker.Step(ctx, 4, 5, "Requesting LLM summary...")

	result, err := samplingClient.Analyze(ctx, summarizeMRReviewPrompt, data,
		sampling.WithMaxTokens(2048),
		sampling.WithTemperature(0.3),
		sampling.WithModelPriorities(0.4, 0.5, 0.5),
	)
	if err != nil {
		return SummarizeMRReviewOutput{}, fmt.Errorf("LLM summary: %w", err)
	}

	tracker.Step(ctx, 5, 5, "Summary complete")

	return SummarizeMRReviewOutput{
		MRIID:     input.MRIID,
		Title:     title,
		Summary:   result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// FormatMRReviewForAnalysis builds a Markdown document from an MR, its discussions,
// and approval state for LLM review summarization.
func FormatMRReviewForAnalysis(mr mergerequests.Output, discussions mrdiscussions.ListOutput, approvals mrapprovals.StateOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# MR Review: !%d — %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&b, toolutil.FmtMdState, mr.State)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, mr.Author)
	fmt.Fprintf(&b, "- **Source**: %s → %s\n", mr.SourceBranch, mr.TargetBranch)

	if len(approvals.Rules) > 0 {
		b.WriteString("\n## Approval Rules\n\n")
		for _, rule := range approvals.Rules {
			status := toolutil.BoolEmoji(rule.Approved)
			if rule.Approved {
				status += " Approved"
			} else {
				status += " Not approved"
			}
			fmt.Fprintf(&b, "- **%s**: %s (required: %d, approved by: %s)\n",
				rule.Name, status, rule.ApprovalsRequired,
				strings.Join(rule.ApprovedByNames, ", "))
		}
	}

	if len(discussions.Discussions) > 0 {
		b.WriteString("\n## Discussions\n\n")
		for _, d := range discussions.Discussions {
			for _, n := range d.Notes {
				if n.System {
					continue
				}
				resolved := ""
				if n.Resolvable {
					if n.Resolved {
						resolved = " [RESOLVED]"
					} else {
						resolved = " [UNRESOLVED]"
					}
				}
				fmt.Fprintf(&b, "**%s** (%s)%s:\n%s\n\n---\n\n", n.Author, toolutil.FormatTime(n.CreatedAt), resolved, n.Body)
			}
		}
	}
	return b.String()
}

// FormatSummarizeMRReviewMarkdown renders an LLM-generated MR review summary.
func FormatSummarizeMRReviewMarkdown(s SummarizeMRReviewOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Review Summary: !%d — %s\n\n", s.MRIID, toolutil.EscapeMdHeading(s.Title))
	if s.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Summary was truncated due to size limits.*\n\n")
	}
	b.WriteString(s.Summary)
	b.WriteString("\n")
	if s.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", s.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_add_mr_note` to post the summary as a review comment",
		"Use `gitlab_mr_approve` or `gitlab_mr_update` to act on the review",
	)
	return b.String()
}
