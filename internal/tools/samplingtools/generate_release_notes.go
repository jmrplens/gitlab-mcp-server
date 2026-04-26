// generate_release_notes.go implements the sampling-based release notes generation tool.

package samplingtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GenerateReleaseNotesInput defines parameters for LLM-assisted release notes generation.
type GenerateReleaseNotesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	From      string               `json:"from"       jsonschema:"Starting ref: tag, branch or SHA,required"`
	To        string               `json:"to"         jsonschema:"Ending ref: tag, branch or SHA (defaults to HEAD)"`
}

// GenerateReleaseNotesOutput holds the LLM-generated release notes.
type GenerateReleaseNotesOutput struct {
	toolutil.HintableOutput
	From         string `json:"from"`
	To           string `json:"to"`
	ReleaseNotes string `json:"release_notes"`
	Model        string `json:"model"`
	Truncated    bool   `json:"truncated"`
}

// generateReleaseNotesPrompt instructs the LLM to produce categorized release notes.
const generateReleaseNotesPrompt = `Generate polished, user-facing release notes from the data provided.

Requirements:
1. Categorize changes into: **Features**, **Bug Fixes**, **Improvements**, **Breaking Changes**, **Documentation**, **Other**
2. Use merge request titles and labels as the primary source for categorization
3. Use labels (bug, feature, enhancement, breaking, docs) when available to assign categories
4. For each entry include the MR reference (!IID) or commit SHA, a concise one-line description, and the author
5. Omit merge commits and internal CI/infrastructure changes unless they affect end users
6. If a commit is associated with a merge request, prefer the MR entry over the commit
7. Write in past tense ("Added", "Fixed", "Improved")
8. Order sections by importance: Breaking Changes first, then Features, Bug Fixes, etc.
9. Include a brief summary paragraph at the top

Output Markdown only — no preamble, no explanations.`

// GenerateReleaseNotes fetches commits, diffs, and merged MRs between two refs,
// then delegates to the MCP sampling capability for LLM-generated release notes.
// Returns [sampling.ErrSamplingNotSupported] if the client lacks sampling support.
func GenerateReleaseNotes(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input GenerateReleaseNotesInput) (GenerateReleaseNotesOutput, error) {
	if input.ProjectID == "" {
		return GenerateReleaseNotesOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.From == "" {
		return GenerateReleaseNotesOutput{}, toolutil.ErrFieldRequired("from")
	}
	if input.To == "" {
		input.To = "HEAD"
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 5, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return GenerateReleaseNotesOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 5, "Comparing refs...")

	cmp, err := repository.Compare(ctx, client, repository.CompareInput{
		ProjectID: input.ProjectID,
		From:      input.From,
		To:        input.To,
	})
	if err != nil {
		return GenerateReleaseNotesOutput{}, fmt.Errorf("comparing refs: %w", err)
	}

	tracker.Step(ctx, 3, 5, "Fetching merged merge requests...")

	mrs, _ := mergerequests.List(ctx, client, mergerequests.ListInput{
		ProjectID: input.ProjectID,
		State:     "merged",
		PaginationInput: toolutil.PaginationInput{
			PerPage: 100,
		},
	})

	data := FormatReleaseDataForAnalysis(input.From, input.To, cmp, mrs)
	tracker.Step(ctx, 4, 5, "Requesting LLM release notes generation...")

	result, err := samplingClient.Analyze(ctx, generateReleaseNotesPrompt, data,
		sampling.WithMaxTokens(4096),
		sampling.WithTemperature(0.4),
		sampling.WithModelPriorities(0.5, 0.6, 0.4),
	)
	if err != nil {
		return GenerateReleaseNotesOutput{}, fmt.Errorf("LLM release notes generation: %w", err)
	}

	tracker.Step(ctx, 5, 5, "Release notes generated")

	return GenerateReleaseNotesOutput{
		From:         input.From,
		To:           input.To,
		ReleaseNotes: result.Content,
		Model:        result.Model,
		Truncated:    result.Truncated,
	}, nil
}

// FormatReleaseDataForAnalysis builds a Markdown document combining commits,
// merged MRs, and file diffs, suitable for LLM release notes generation.
func FormatReleaseDataForAnalysis(from, to string, cmp repository.CompareOutput, mrs mergerequests.ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Release: %s → %s\n\n", from, to)

	if len(mrs.MergeRequests) > 0 {
		fmt.Fprintf(&b, "## Merged MRs (%d)\n\n", len(mrs.MergeRequests))
		for _, mr := range mrs.MergeRequests {
			labels := ""
			if len(mr.Labels) > 0 {
				labels = " [" + strings.Join(mr.Labels, ", ") + "]"
			}
			fmt.Fprintf(&b, "- !%d — %s (@%s)%s\n", mr.IID, mr.Title, mr.Author, labels)
			if mr.Description != "" {
				desc := strings.SplitN(mr.Description, "\n", 2)[0]
				if len(desc) > 200 {
					desc = desc[:200] + "..."
				}
				fmt.Fprintf(&b, "  > %s\n", desc)
			}
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## Commits (%d)\n\n", len(cmp.Commits))
	for _, c := range cmp.Commits {
		sha := c.ShortID
		if sha == "" && len(c.ID) > 8 {
			sha = c.ID[:8]
		}
		fmt.Fprintf(&b, "- %s — %s (%s)\n", sha, c.Title, c.AuthorName)
	}

	fmt.Fprintf(&b, "\n## Files Changed (%d)\n\n", len(cmp.Diffs))
	for _, d := range cmp.Diffs {
		fmt.Fprintf(&b, "- %s\n", d.NewPath)
	}

	return b.String()
}

// FormatGenerateReleaseNotesMarkdown renders LLM-generated release notes.
func FormatGenerateReleaseNotesMarkdown(r GenerateReleaseNotesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Release Notes: %s → %s\n\n", r.From, r.To)
	if r.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Release notes were truncated due to size limits.*\n\n")
	}
	b.WriteString(r.ReleaseNotes)
	b.WriteString("\n")
	if r.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", r.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_release_create` to publish the release with these notes",
		"Use `gitlab_release_link_create` to attach assets to the release",
	)
	return b.String()
}
