package prompts

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var (
	conventionalCommitPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^)]+\))?!?: .+`)
	breakingChangePattern     = regexp.MustCompile(`(?im)^BREAKING[ -]CHANGE:`)
	linkedWorkPattern         = regexp.MustCompile(`(?i)(close[sd]?|fix(e[sd])?|resolve[sd]?)\s+#\d+|[#!]\d+|[A-Z][A-Z0-9]+-\d+`)
	testEvidencePattern       = regexp.MustCompile(`(?i)\b(test|tests|tested|qa|verification|validated|manual check|automated)\b`)
	riskEvidencePattern       = regexp.MustCompile(`(?i)\b(risk|rollback|rollout|migration|deploy|release|feature flag|flag)\b`)
)

// registerGitWorkflowPrompts registers prompts for Git history and MR authoring quality.
func registerGitWorkflowPrompts(server *mcp.Server, client *gitlabclient.Client) {
	registerAuditCommitHygienePrompt(server, client)
	registerMRDescriptionQualityPrompt(server, client)
}

// registerAuditCommitHygienePrompt registers the audit_commit_hygiene prompt.
func registerAuditCommitHygienePrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "audit_commit_hygiene",
		Title:       toolutil.TitleFromName("audit_commit_hygiene"),
		Description: "Audit commit message quality between two refs. Scores Conventional Commit usage, merge commits, breaking-change markers, body/detail quality, and linked work references for release and contribution readiness.",
		Icons:       toolutil.IconCommit,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			{Name: "from", Title: toolutil.TitleFromName("from"), Description: "Starting ref: tag name, branch name, or commit SHA", Required: true},
			{Name: "to", Title: toolutil.TitleFromName("to"), Description: "Ending ref: tag name, branch name, or commit SHA (defaults to HEAD if omitted)", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleAuditCommitHygiene(ctx, client, req)
	})
}

// handleAuditCommitHygiene compares two refs and prepares a commit-quality audit.
func handleAuditCommitHygiene(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	from := req.Params.Arguments["from"]
	if projectID == "" || from == "" {
		return nil, fmt.Errorf("%s and from are required", argProjectID)
	}
	to := getArgOr(req.Params.Arguments, "to", "HEAD")

	comparison, _, err := client.GL().Repositories.Compare(projectID, &gl.CompareOptions{
		From: new(from),
		To:   new(to),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("audit_commit_hygiene: failed to compare refs: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Commit Hygiene Audit: %s → %s\n\n", from, to)

	if comparison.CompareSameRef || len(comparison.Commits) == 0 {
		b.WriteString("No commits found between these refs.\n")
		return promptResult(b.String()), nil
	}

	stats := analyzeCommitHygiene(comparison.Commits)
	b.WriteString(mdSummaryHeader)
	b.WriteString("| Metric | Count |\n|--------|-------|\n")
	fmt.Fprintf(&b, "| Total commits | %d |\n", len(comparison.Commits))
	fmt.Fprintf(&b, "| Conventional titles | %d |\n", stats.conventional)
	fmt.Fprintf(&b, "| Merge commits | %d |\n", stats.mergeCommits)
	fmt.Fprintf(&b, "| Breaking-change markers | %d |\n", stats.breakingChanges)
	fmt.Fprintf(&b, "| Commit bodies/details present | %d |\n", stats.withBody)
	fmt.Fprintf(&b, "| Linked work references | %d |\n", stats.linkedWork)
	b.WriteString("\n")

	b.WriteString("## Commit Details\n\n")
	b.WriteString("| Commit | Title | Author | Hygiene |\n")
	b.WriteString("|--------|-------|--------|---------|\n")
	for _, commit := range comparison.Commits {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", shortSHA(commit.ID), firstLine(commit.Title), commit.AuthorName, commitHygieneLabel(commit))
	}

	b.WriteString("\n---\nPlease assess the history quality for release/readiness review. Highlight commits that should be squashed, reworded, linked to issues, or marked as breaking changes. Use Conventional Commits categories such as feat, fix, docs, refactor, test, build, ci, chore, perf, and revert.\n")

	return promptResult(b.String()), nil
}

// registerMRDescriptionQualityPrompt registers the mr_description_quality prompt.
func registerMRDescriptionQualityPrompt(server *mcp.Server, client *gitlabclient.Client) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "mr_description_quality",
		Title:       toolutil.TitleFromName("mr_description_quality"),
		Description: "Score a merge request description for reviewer readiness. Checks context, linked work, test evidence, rollout/risk notes, checklists, and whether changed files suggest missing screenshots or migration notes.",
		Icons:       toolutil.IconMR,
		Arguments: []*mcp.PromptArgument{
			projectIDArg(),
			mrIIDArg(),
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return handleMRDescriptionQuality(ctx, client, req)
	})
}

// handleMRDescriptionQuality gathers MR metadata and diffs for description scoring.
func handleMRDescriptionQuality(ctx context.Context, client *gitlabclient.Client, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	projectID := req.Params.Arguments[argProjectID]
	mrIID := req.Params.Arguments[argMRIID]
	if projectID == "" || mrIID == "" {
		return nil, fmt.Errorf(fmtTwoArgsRequired, argProjectID, argMRIID)
	}

	iid := parseIID(mrIID)
	if iid == 0 {
		return nil, errors.New("merge_request_iid must be a positive integer")
	}

	mr, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, iid, &gl.GetMergeRequestsOptions{}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRFailed, err)
	}
	diffs, _, err := client.GL().MergeRequests.ListMergeRequestDiffs(projectID, iid, nil, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf(fmtGetMRDiffsFailed, err)
	}

	signals := analyzeMRDescription(mr.Description)
	metrics := computeDiffMetrics(diffs)

	var b strings.Builder
	fmt.Fprintf(&b, "# MR Description Quality: !%d — %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&b, "**Branch**: %s → %s\n", mr.SourceBranch, mr.TargetBranch)
	fmt.Fprintf(&b, "**Description length**: %d characters\n", len(strings.TrimSpace(mr.Description)))
	fmt.Fprintf(&b, "**Files changed**: %d | **Tests changed**: %d | **Docs changed**: %d | **Sensitive files touched**: %d\n\n", len(diffs), countMatchingDiffs(diffs, isTestPath), countMatchingDiffs(diffs, isDocPath), metrics.sensitiveFiles)

	b.WriteString("## Description Signals\n\n")
	b.WriteString("| Signal | Present |\n|--------|---------|\n")
	fmt.Fprintf(&b, "| Clear context (>120 chars) | %s |\n", toolutil.BoolEmoji(signals.hasContext))
	fmt.Fprintf(&b, "| Linked issue/MR/work item | %s |\n", toolutil.BoolEmoji(signals.hasLinkedWork))
	fmt.Fprintf(&b, "| Test or verification evidence | %s |\n", toolutil.BoolEmoji(signals.hasTestEvidence))
	fmt.Fprintf(&b, "| Rollout, risk, or rollback notes | %s |\n", toolutil.BoolEmoji(signals.hasRiskNotes))
	fmt.Fprintf(&b, "| Checklist present | %s |\n", toolutil.BoolEmoji(signals.hasChecklist))
	b.WriteString("\n")

	if strings.TrimSpace(mr.Description) != "" {
		b.WriteString("## Current Description\n\n")
		b.WriteString(mr.Description)
		b.WriteString("\n\n")
	}

	b.WriteString("## Changed Files\n\n")
	for _, diff := range diffs {
		fmt.Fprintf(&b, fmtListItem, diff.NewPath, changeType(diff))
	}

	b.WriteString("\n---\nPlease score this MR description from 0-100 for reviewer readiness. Identify missing context, test evidence, rollout/risk information, screenshots/UI evidence when relevant, and linked work. Then propose a concise improved MR description template for this change.\n")

	return promptResult(b.String()), nil
}

type commitHygieneStats struct {
	conventional    int
	mergeCommits    int
	breakingChanges int
	withBody        int
	linkedWork      int
}

func analyzeCommitHygiene(commits []*gl.Commit) commitHygieneStats {
	var stats commitHygieneStats
	for _, commit := range commits {
		if isConventionalCommit(commit.Title) {
			stats.conventional++
		}
		if isMergeCommit(commit) {
			stats.mergeCommits++
		}
		if hasBreakingChangeMarker(commit) {
			stats.breakingChanges++
		}
		if commitBody(commit) != "" {
			stats.withBody++
		}
		if hasLinkedWork(commit) {
			stats.linkedWork++
		}
	}
	return stats
}

func commitHygieneLabel(commit *gl.Commit) string {
	var parts []string
	if isMergeCommit(commit) {
		parts = append(parts, "merge")
	}
	if isConventionalCommit(commit.Title) {
		parts = append(parts, "conventional")
	} else {
		parts = append(parts, "needs title")
	}
	if commitBody(commit) != "" {
		parts = append(parts, "body")
	}
	if hasLinkedWork(commit) {
		parts = append(parts, "linked")
	}
	if hasBreakingChangeMarker(commit) {
		parts = append(parts, "breaking")
	}
	return strings.Join(parts, ", ")
}

func isConventionalCommit(title string) bool {
	return conventionalCommitPattern.MatchString(strings.TrimSpace(title))
}

func isMergeCommit(commit *gl.Commit) bool {
	return len(commit.ParentIDs) > 1 || strings.HasPrefix(strings.ToLower(commit.Title), "merge ")
}

func hasBreakingChangeMarker(commit *gl.Commit) bool {
	return strings.Contains(commit.Title, "!:") || breakingChangePattern.MatchString(commit.Message)
}

func hasLinkedWork(commit *gl.Commit) bool {
	return linkedWorkPattern.MatchString(commit.Title) || linkedWorkPattern.MatchString(commit.Message)
}

func commitBody(commit *gl.Commit) string {
	message := strings.TrimSpace(commit.Message)
	if message == "" {
		return ""
	}
	lines := strings.Split(message, "\n")
	if len(lines) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[1:], "\n"))
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

type mrDescriptionSignals struct {
	hasContext      bool
	hasLinkedWork   bool
	hasTestEvidence bool
	hasRiskNotes    bool
	hasChecklist    bool
}

func analyzeMRDescription(description string) mrDescriptionSignals {
	trimmed := strings.TrimSpace(description)
	lower := strings.ToLower(trimmed)
	return mrDescriptionSignals{
		hasContext:      len(trimmed) >= 120,
		hasLinkedWork:   linkedWorkPattern.MatchString(trimmed),
		hasTestEvidence: testEvidencePattern.MatchString(trimmed),
		hasRiskNotes:    riskEvidencePattern.MatchString(trimmed),
		hasChecklist:    strings.Contains(lower, "- [ ]") || strings.Contains(lower, "- [x]") || strings.Contains(lower, "checklist"),
	}
}

func countMatchingDiffs(diffs []*gl.MergeRequestDiff, match func(string) bool) int {
	count := 0
	for _, diff := range diffs {
		if match(diff.NewPath) || match(diff.OldPath) {
			count++
		}
	}
	return count
}
