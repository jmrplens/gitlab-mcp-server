//go:build e2e

// actions_b7_mr_test.go names the catalog actions the merge request batch (B7)
// drives and that no earlier actions file already declares.
//
// They are typed harness.ActionID constants for the same reason the ones in
// actions_test.go are: the push-time static gate reads constants of that type
// out of the type checker's record and follows them through the harness verbs,
// so a literal at a call site would be invisible to it and a renamed action
// would fail to compile in one place rather than silently.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The award emoji on a merge request and on one of its notes. The issue and
// snippet halves of the same family are declared by an earlier batch.
const (
	actionMergeRequestEmojiCreate     harness.ActionID = "merge_request.emoji_mr_create"
	actionMergeRequestEmojiList       harness.ActionID = "merge_request.emoji_mr_list"
	actionMergeRequestEmojiGet        harness.ActionID = "merge_request.emoji_mr_get"
	actionMergeRequestEmojiDelete     harness.ActionID = "merge_request.emoji_mr_delete"
	actionMergeRequestNoteEmojiCreate harness.ActionID = "merge_request.emoji_mr_note_create"
	actionMergeRequestNoteEmojiList   harness.ActionID = "merge_request.emoji_mr_note_list"
	actionMergeRequestNoteEmojiGet    harness.ActionID = "merge_request.emoji_mr_note_get"
	actionMergeRequestNoteEmojiDelete harness.ActionID = "merge_request.emoji_mr_note_delete"
)

// The resource events a merge request records, each as the listing and as the
// singular read of one event by its ID. The state listing is declared by the
// batch that reads it after a close, and is driven here again for the ID the
// singular read takes.
const (
	actionMergeRequestLabelEventList     harness.ActionID = "merge_request.event_mr_label_list"
	actionMergeRequestLabelEventGet      harness.ActionID = "merge_request.event_mr_label_get"
	actionMergeRequestMilestoneEventList harness.ActionID = "merge_request.event_mr_milestone_list"
	actionMergeRequestMilestoneEventGet  harness.ActionID = "merge_request.event_mr_milestone_get"
	actionMergeRequestStateEventGet      harness.ActionID = "merge_request.event_mr_state_get"
)

// The five time-tracking actions, and the two that toggle the caller's own
// subscription to a merge request.
const (
	actionMergeRequestTimeEstimateSet   harness.ActionID = "merge_request.time_estimate_set"
	actionMergeRequestTimeEstimateReset harness.ActionID = "merge_request.time_estimate_reset"
	actionMergeRequestSpentTimeAdd      harness.ActionID = "merge_request.spent_time_add"
	actionMergeRequestSpentTimeReset    harness.ActionID = "merge_request.spent_time_reset"
	actionMergeRequestTimeStats         harness.ActionID = "merge_request.time_stats"
	actionMergeRequestSubscribe         harness.ActionID = "merge_request.subscribe"
	actionMergeRequestUnsubscribe       harness.ActionID = "merge_request.unsubscribe"
)

// The listings at instance and group scope, the reviewers assigned to one
// request, the issues its merge would close, and the cancel that disarms
// auto-merge.
const (
	actionMergeRequestListGlobal      harness.ActionID = "merge_request.list_global"
	actionMergeRequestListGroup       harness.ActionID = "merge_request.list_group"
	actionMergeRequestReviewers       harness.ActionID = "merge_request.reviewers"
	actionMergeRequestIssuesClosed    harness.ActionID = "merge_request.issues_closed"
	actionMergeRequestCancelAutoMerge harness.ActionID = "merge_request.cancel_auto_merge"
)
