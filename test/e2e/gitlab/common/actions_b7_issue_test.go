//go:build e2e

// actions_b7_issue_test.go names the issue-family catalog actions the b7
// front drives and no earlier file declared: the time-tracking routes, the
// subscription and to-do writes, the move and the reorder, the resource
// events read one at a time, the emoji on an issue note, the reads that
// answer about an issue from outside the project it lives in, and the three
// scopes of the issue statistics.
//
// The ids an earlier port already declared are reused from the files that
// declared them (issue.create, issue.get, issue.list, issue.update,
// issue.note_create and issue.event_issue_state_list among them), since Go
// admits one declaration per name and two constants for one action would be
// two names for the same string.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The issue actions this front drives.
const (
	// Time tracking: the estimate, the time logged against it and the totals.
	actionIssueTimeEstimateSet   harness.ActionID = "issue.time_estimate_set"
	actionIssueTimeEstimateReset harness.ActionID = "issue.time_estimate_reset"
	actionIssueSpentTimeAdd      harness.ActionID = "issue.spent_time_add"
	actionIssueSpentTimeReset    harness.ActionID = "issue.spent_time_reset"
	actionIssueTimeStatsGet      harness.ActionID = "issue.time_stats_get"

	// What the caller can attach to themselves: a subscription and a to-do.
	actionIssueSubscribe   harness.ActionID = "issue.subscribe"
	actionIssueUnsubscribe harness.ActionID = "issue.unsubscribe"
	actionIssueCreateTodo  harness.ActionID = "issue.create_todo"

	// The two writes that change where an issue sits.
	actionIssueMove    harness.ActionID = "issue.move"
	actionIssueReorder harness.ActionID = "issue.reorder"

	// The resource events an issue records, listed and read one at a time.
	actionIssueLabelEventList     harness.ActionID = "issue.event_issue_label_list"
	actionIssueLabelEventGet      harness.ActionID = "issue.event_issue_label_get"
	actionIssueMilestoneEventList harness.ActionID = "issue.event_issue_milestone_list"
	actionIssueMilestoneEventGet  harness.ActionID = "issue.event_issue_milestone_get"
	actionIssueStateEventGet      harness.ActionID = "issue.event_issue_state_get"

	// The award emoji of a note on an issue, which are their own four routes
	// beside the ones that react to the issue itself.
	actionIssueNoteEmojiCreate harness.ActionID = "issue.emoji_issue_note_create"
	actionIssueNoteEmojiList   harness.ActionID = "issue.emoji_issue_note_list"
	actionIssueNoteEmojiGet    harness.ActionID = "issue.emoji_issue_note_get"
	actionIssueNoteEmojiDelete harness.ActionID = "issue.emoji_issue_note_delete"

	// What an issue answers about the work around it.
	actionIssueParticipants harness.ActionID = "issue.participants"
	actionIssueMRsClosing   harness.ActionID = "issue.mrs_closing"
	actionIssueMRsRelated   harness.ActionID = "issue.mrs_related"

	// The reads that reach an issue from outside its own project.
	actionIssueGetByID   harness.ActionID = "issue.get_by_id"
	actionIssueListAll   harness.ActionID = "issue.list_all"
	actionIssueListGroup harness.ActionID = "issue.list_group"

	// The issue counts, at the three scopes GitLab offers them.
	actionIssueStatisticsGet        harness.ActionID = "issue.statistics_get"
	actionIssueStatisticsGetGroup   harness.ActionID = "issue.statistics_get_group"
	actionIssueStatisticsGetProject harness.ActionID = "issue.statistics_get_project"
)
