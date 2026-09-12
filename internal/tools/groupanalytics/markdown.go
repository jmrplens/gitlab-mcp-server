package groupanalytics

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name. These three actions are routes
// on the group catalog group, so their domain is "group".
const (
	actionIssuesCount  = "group.analytics_issues_count"
	actionMRCount      = "group.analytics_mr_count"
	actionMembersCount = "group.analytics_members_count"
	actionIssueList    = "issue.list_group"
	actionMRList       = "merge_request.list_group"
	actionMemberList   = "group.members"
)

// FormatIssuesCountMarkdown renders the recently created issue count as a card.
func FormatIssuesCountMarkdown(out IssuesCountOutput) string {
	return countCard("Recently Created Issues Count", out.GroupPath,
		"Issues Count (last 90 days)", out.IssuesCount,
		toolutil.HintAction(actionMRCount, "compare with merge request activity"),
		toolutil.HintAction(actionIssueList, "read the issues themselves"),
	)
}

// FormatMRCountMarkdown renders the recently created merge request count as a
// card.
func FormatMRCountMarkdown(out MRCountOutput) string {
	return countCard("Recently Created Merge Requests Count", out.GroupPath,
		"Merge Requests Count (last 90 days)", out.MergeRequestsCount,
		toolutil.HintAction(actionIssuesCount, "compare with issue activity"),
		toolutil.HintAction(actionMRList, "read the merge requests themselves"),
	)
}

// FormatMembersCountMarkdown renders the recently added member count as a card.
func FormatMembersCountMarkdown(out MembersCountOutput) string {
	return countCard("Recently Added Members Count", out.GroupPath,
		"New Members Count (last 90 days)", out.NewMembersCount,
		toolutil.HintAction(actionMemberList, "read the members themselves"),
		toolutil.HintAction(actionIssuesCount, "see the group's development activity"),
	)
}

// countCard renders the one shape all three analytics answers share: the group
// the count was taken over, and the count. The path goes in a code span, being
// the value a caller copies into the next request.
func countCard(heading, groupPath, label string, count int64, hints ...string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, heading)
	c.Code("Group", groupPath)
	c.Int(label, count)
	c.End(hints...)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatIssuesCountMarkdown)  // IssuesCountOutput
	toolutil.RegisterMarkdown(FormatMRCountMarkdown)      // MRCountOutput
	toolutil.RegisterMarkdown(FormatMembersCountMarkdown) // MembersCountOutput
}
