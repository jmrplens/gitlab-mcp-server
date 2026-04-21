// Package prompts provides shared helper functions used by multiple
// team-management prompt handlers across all categories (cross-project,
// team, project reports, analytics, milestone & label).
package prompts

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Reusable argument names for team-management prompts.
const (
	argGroupID      = "group_id"
	argUsername     = "username"
	argDays         = "days"
	argState        = "state"
	argTargetBranch = "target_branch"

	descGroupID      = "GitLab group ID (numeric) or URL-encoded path (e.g. 'my-group' or 'parent/child')"
	descUsername     = "GitLab username to query"
	descDays         = "Number of days to look back (default: %d)"
	descState        = "State filter: opened, closed, merged, all (default: %s)"
	descTargetBranch = "Target branch name to filter MRs (e.g. 'develop_5.4.0')"

	maxListItems = 100
)

// groupIDArg returns a required prompt argument for the GitLab group ID.
func groupIDArg() *mcp.PromptArgument {
	return &mcp.PromptArgument{
		Name:        argGroupID,
		Title:       toolutil.TitleFromName(argGroupID),
		Description: descGroupID,
		Required:    true,
	}
}

// usernameArg returns an optional prompt argument for the GitLab username.
func usernameArg() *mcp.PromptArgument {
	return &mcp.PromptArgument{
		Name:        argUsername,
		Title:       toolutil.TitleFromName(argUsername),
		Description: descUsername,
		Required:    false,
	}
}

// daysArg returns an optional prompt argument for the look-back period.
func daysArg(defaultDays int) *mcp.PromptArgument {
	return &mcp.PromptArgument{
		Name:        argDays,
		Title:       toolutil.TitleFromName(argDays),
		Description: fmt.Sprintf(descDays, defaultDays),
		Required:    false,
	}
}

// stateArg returns an optional prompt argument for state filtering.
func stateArg(defaultState string) *mcp.PromptArgument {
	return &mcp.PromptArgument{
		Name:        argState,
		Title:       toolutil.TitleFromName(argState),
		Description: fmt.Sprintf(descState, defaultState),
		Required:    false,
	}
}

// targetBranchArg returns a prompt argument for the target branch filter.
func targetBranchArg(required bool) *mcp.PromptArgument {
	return &mcp.PromptArgument{
		Name:        argTargetBranch,
		Title:       toolutil.TitleFromName(argTargetBranch),
		Description: descTargetBranch,
		Required:    required,
	}
}

// parseDays converts a string days argument to an int. Returns defaultDays
// if the string is empty or cannot be parsed as a positive integer.
func parseDays(s string, defaultDays int) int {
	if s == "" {
		return defaultDays
	}
	d, err := strconv.Atoi(s)
	if err != nil || d <= 0 {
		return defaultDays
	}
	return d
}

// sinceDate returns a time.Time that is the given number of days in the past,
// truncated to the start of that day in UTC.
func sinceDate(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, -days).Truncate(24 * time.Hour)
}

// extractProjectPath extracts the project path from a BasicMergeRequest.
// It uses References.Full (e.g. "group/project!42") stripped of the MR
// reference, or falls back to WebURL parsing.
func extractProjectPath(mr *gl.BasicMergeRequest) string {
	if mr.References != nil && mr.References.Full != "" {
		ref := mr.References.Full
		if idx := strings.LastIndex(ref, "!"); idx > 0 {
			return ref[:idx]
		}
	}
	if path := projectPathFromWebURL(mr.WebURL); path != "" {
		return path
	}
	return fmt.Sprintf("project-%d", mr.ProjectID)
}

// extractIssueProjectPath extracts the project path from an Issue.
func extractIssueProjectPath(issue *gl.Issue) string {
	if issue.References != nil && issue.References.Full != "" {
		ref := issue.References.Full
		if idx := strings.LastIndex(ref, "#"); idx > 0 {
			return ref[:idx]
		}
	}
	if path := projectPathFromWebURL(issue.WebURL); path != "" {
		return path
	}
	return fmt.Sprintf("project-%d", issue.ProjectID)
}

// projectPathFromWebURL extracts the project path from a GitLab web URL
// (e.g. "https://gitlab.example.com/group/project/-/merge_requests/42" → "group/project").
func projectPathFromWebURL(webURL string) string {
	if webURL == "" || !strings.Contains(webURL, "/-/") {
		return ""
	}
	path := webURL
	if schemeEnd := strings.Index(path, "://"); schemeEnd > 0 {
		path = path[schemeEnd+3:]
	}
	if slashIdx := strings.Index(path, "/"); slashIdx > 0 {
		path = path[slashIdx+1:]
	}
	if dashIdx := strings.Index(path, "/-/"); dashIdx > 0 {
		return path[:dashIdx]
	}
	return ""
}

// groupMRsByProject groups merge requests by their project path.
func groupMRsByProject(mrs []*gl.BasicMergeRequest) map[string][]*gl.BasicMergeRequest {
	result := make(map[string][]*gl.BasicMergeRequest)
	for _, mr := range mrs {
		path := extractProjectPath(mr)
		result[path] = append(result[path], mr)
	}
	return result
}

// groupIssuesByProject groups issues by their project path.
func groupIssuesByProject(issues []*gl.Issue) map[string][]*gl.Issue {
	result := make(map[string][]*gl.Issue)
	for _, issue := range issues {
		path := extractIssueProjectPath(issue)
		result[path] = append(result[path], issue)
	}
	return result
}

// sortedKeys returns the map keys sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// writeMRTable writes a Markdown table of merge requests with standard columns.
func writeMRTable(b *strings.Builder, mrs []*gl.BasicMergeRequest) {
	if len(mrs) == 0 {
		b.WriteString("No merge requests found.\n")
		return
	}
	b.WriteString("| MR | Title | Author | Branch | Age | Status |\n")
	b.WriteString("|----|-------|--------|--------|-----|--------|\n")
	for _, mr := range mrs {
		author := "unknown"
		if mr.Author != nil {
			author = "@" + mr.Author.Username
		}
		branch := fmt.Sprintf("%s → %s", mr.SourceBranch, mr.TargetBranch)
		status := mrStatus(mr)
		fmt.Fprintf(b, "| !%d | %s | %s | %s | %s | %s |\n",
			mr.IID, mr.Title, author, branch, mrAge(mr), status)
	}
}

// writeIssueTable writes a Markdown table of issues with standard columns.
func writeIssueTable(b *strings.Builder, issues []*gl.Issue) {
	if len(issues) == 0 {
		b.WriteString("No issues found.\n")
		return
	}
	b.WriteString("| Issue | Title | Labels | Milestone | Age | Due |\n")
	b.WriteString("|-------|-------|--------|-----------|-----|-----|\n")
	for _, issue := range issues {
		labels := "—"
		if len(issue.Labels) > 0 {
			labels = strings.Join(issue.Labels, ", ")
		}
		milestone := "—"
		if issue.Milestone != nil {
			milestone = issue.Milestone.Title
		}
		due := "—"
		if issue.DueDate != nil {
			due = time.Time(*issue.DueDate).Format("2006-01-02")
		}
		fmt.Fprintf(b, "| #%d | %s | %s | %s | %s | %s |\n",
			issue.IID, issue.Title, labels, milestone, issueAge(issue), due)
	}
}

// mrAge formats the age of a merge request as a human-readable duration.
func mrAge(mr *gl.BasicMergeRequest) string {
	if mr.CreatedAt == nil {
		return "?"
	}
	return formatAge(time.Since(*mr.CreatedAt))
}

// issueAge formats the age of an issue as a human-readable duration.
func issueAge(issue *gl.Issue) string {
	if issue.CreatedAt == nil {
		return "?"
	}
	return formatAge(time.Since(*issue.CreatedAt))
}

// formatAge converts a duration to a compact human-readable string.
func formatAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days < 1:
		return "<1d"
	case days < 7:
		return fmt.Sprintf("%dd", days)
	case days < 30:
		return fmt.Sprintf("%dw", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}

// mrStatus returns a compact status string for a merge request.
func mrStatus(mr *gl.BasicMergeRequest) string {
	var parts []string
	if mr.Draft {
		parts = append(parts, "draft")
	}
	if mr.HasConflicts {
		parts = append(parts, "conflicts")
	}
	if mr.DetailedMergeStatus != "" {
		parts = append(parts, mr.DetailedMergeStatus)
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

// promptPipelineEmojis maps pipeline status strings to their emoji.
var promptPipelineEmojis = map[string]string{
	"success":   toolutil.EmojiSuccess,
	"failed":    toolutil.EmojiCross,
	"running":   "⏳",
	"pending":   "⏳",
	"canceled":  toolutil.EmojiProhibited,
	"cancelled": toolutil.EmojiProhibited,
	"skipped":   "⏭️",
}

// pipelineEmoji converts a pipeline status string to a corresponding emoji.
func pipelineEmoji(status string) string {
	if e, ok := promptPipelineEmojis[strings.ToLower(status)]; ok {
		return e
	}
	return toolutil.EmojiWhiteCircle
}

// mergeDuration computes the time between creation and merge of a MR.
// Returns 0 if either timestamp is nil.
func mergeDuration(mr *gl.BasicMergeRequest) time.Duration {
	if mr.CreatedAt == nil || mr.MergedAt == nil {
		return 0
	}
	return mr.MergedAt.Sub(*mr.CreatedAt)
}

// formatDuration converts a duration to a human-readable string (e.g. "2d 5h", "3h 20m").
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	hours := int(d.Hours())
	days := hours / 24
	remainingHours := hours % 24

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, remainingHours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

// avgDuration computes the average of a slice of durations.
func avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// medianDuration computes the median of a slice of durations.
func medianDuration(durations []time.Duration) time.Duration {
	n := len(durations)
	if n == 0 {
		return 0
	}
	// Copy to avoid mutating caller's slice
	sorted := make([]time.Duration, n)
	copy(sorted, durations)
	// Simple sort
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// progressBar returns an ASCII progress bar like [████████░░] 80%.
func progressBar(done, total int) string {
	if total == 0 {
		return "[░░░░░░░░░░] 0%"
	}
	pct := float64(done) / float64(total) * 100
	filled := min(int(math.Round(pct/10)), 10)
	return fmt.Sprintf("[%s%s] %.0f%%",
		strings.Repeat("█", filled),
		strings.Repeat("░", 10-filled),
		pct)
}

// deduplicateMRs merges two MR slices and removes duplicates by IID+ProjectID.
func deduplicateMRs(a, b []*gl.BasicMergeRequest) []*gl.BasicMergeRequest {
	type mrKey struct {
		projectID int64
		iid       int64
	}
	seen := make(map[mrKey]bool)
	var result []*gl.BasicMergeRequest
	for _, mrs := range [][]*gl.BasicMergeRequest{a, b} {
		for _, mr := range mrs {
			k := mrKey{projectID: mr.ProjectID, iid: mr.IID}
			if !seen[k] {
				seen[k] = true
				result = append(result, mr)
			}
		}
	}
	return result
}

// getArgOr returns the argument value or a default if empty.
func getArgOr(args map[string]string, key, defaultVal string) string {
	if v, ok := args[key]; ok && v != "" {
		return v
	}
	return defaultVal
}
