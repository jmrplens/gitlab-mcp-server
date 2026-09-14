//go:build e2e

// actions_b5_test.go names the catalog actions the issue, merge request,
// snippet, wiki and search families call, once, for the reason
// actions_test.go states: the push-time static gate reads typed constants
// of the harness's ActionID type and would not see a literal.
//
// The constants a family shares with an earlier port (issue.create,
// issue.update, issue.list, merge_request.create, snippet.create,
// snippet.get, snippet.delete) stay in actions_test.go and are reused.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// An issue's own lifecycle beyond its create and update, and the state
// events its close records.
const (
	actionIssueGet            harness.ActionID = "issue.get"
	actionIssueDelete         harness.ActionID = "issue.delete"
	actionIssueStateEventList harness.ActionID = "issue.event_issue_state_list"
)

// An issue's notes.
const (
	actionIssueNoteCreate harness.ActionID = "issue.note_create"
	actionIssueNoteList   harness.ActionID = "issue.note_list"
	actionIssueNoteGet    harness.ActionID = "issue.note_get"
	actionIssueNoteUpdate harness.ActionID = "issue.note_update"
	actionIssueNoteDelete harness.ActionID = "issue.note_delete"
)

// An issue's threaded discussions.
const (
	actionIssueDiscussionCreate     harness.ActionID = "issue.discussion_create"
	actionIssueDiscussionList       harness.ActionID = "issue.discussion_list"
	actionIssueDiscussionGet        harness.ActionID = "issue.discussion_get"
	actionIssueDiscussionAddNote    harness.ActionID = "issue.discussion_add_note"
	actionIssueDiscussionUpdateNote harness.ActionID = "issue.discussion_update_note"
	actionIssueDiscussionDeleteNote harness.ActionID = "issue.discussion_delete_note"
)

// The links between two issues.
const (
	actionIssueLinkCreate harness.ActionID = "issue.link_create"
	actionIssueLinkList   harness.ActionID = "issue.link_list"
	actionIssueLinkGet    harness.ActionID = "issue.link_get"
	actionIssueLinkDelete harness.ActionID = "issue.link_delete"
)

// The award emoji of an issue, of a project snippet and of a snippet's
// note, all owned by the award emoji package and routed through the issue
// and snippet groups.
const (
	actionIssueEmojiCreate       harness.ActionID = "issue.emoji_issue_create"
	actionIssueEmojiList         harness.ActionID = "issue.emoji_issue_list"
	actionIssueEmojiGet          harness.ActionID = "issue.emoji_issue_get"
	actionIssueEmojiDelete       harness.ActionID = "issue.emoji_issue_delete"
	actionSnippetEmojiCreate     harness.ActionID = "snippet.emoji_snippet_create"
	actionSnippetEmojiList       harness.ActionID = "snippet.emoji_snippet_list"
	actionSnippetEmojiGet        harness.ActionID = "snippet.emoji_snippet_get"
	actionSnippetEmojiDelete     harness.ActionID = "snippet.emoji_snippet_delete"
	actionSnippetNoteEmojiCreate harness.ActionID = "snippet.emoji_snippet_note_create"
	actionSnippetNoteEmojiList   harness.ActionID = "snippet.emoji_snippet_note_list"
	actionSnippetNoteEmojiGet    harness.ActionID = "snippet.emoji_snippet_note_get"
	actionSnippetNoteEmojiDelete harness.ActionID = "snippet.emoji_snippet_note_delete"
)

// A project's labels and milestones, routed through the project group.
const (
	actionProjectLabelList       harness.ActionID = "project.label_list"
	actionProjectLabelUpdate     harness.ActionID = "project.label_update"
	actionProjectLabelDelete     harness.ActionID = "project.label_delete"
	actionProjectMilestoneGet    harness.ActionID = "project.milestone_get"
	actionProjectMilestoneUpdate harness.ActionID = "project.milestone_update"
	actionProjectMilestoneDelete harness.ActionID = "project.milestone_delete"
)

// A merge request's own lifecycle beyond its create, the reads that hang
// off it, and the state events its close records.
const (
	actionMergeRequestGet            harness.ActionID = "merge_request.get"
	actionMergeRequestList           harness.ActionID = "merge_request.list"
	actionMergeRequestUpdate         harness.ActionID = "merge_request.update"
	actionMergeRequestDelete         harness.ActionID = "merge_request.delete"
	actionMergeRequestCommits        harness.ActionID = "merge_request.commits"
	actionMergeRequestParticipants   harness.ActionID = "merge_request.participants"
	actionMergeRequestStateEventList harness.ActionID = "merge_request.event_mr_state_list"
)

// The Free half of a merge request's approval and merge lifecycle: the
// approve and unapprove GitLab serves every edition, the configuration read
// that shows them, the rebase, the pipelines listing and the merge itself.
const (
	actionMergeRequestPipelines      harness.ActionID = "merge_request.pipelines"
	actionMergeRequestRebase         harness.ActionID = "merge_request.rebase"
	actionMergeRequestApprove        harness.ActionID = "merge_request.approve"
	actionMergeRequestUnapprove      harness.ActionID = "merge_request.unapprove"
	actionMergeRequestApprovalConfig harness.ActionID = "merge_request.approval_config"
	actionMergeRequestMerge          harness.ActionID = "merge_request.merge"
)

// What else hangs off a merge request: the context commits it pins, the
// to-do it raises, the issues it closes and the pipeline it runs.
const (
	actionMergeRequestContextCommitsCreate harness.ActionID = "merge_request.context_commits_create"
	actionMergeRequestContextCommitsList   harness.ActionID = "merge_request.context_commits_list"
	actionMergeRequestContextCommitsDelete harness.ActionID = "merge_request.context_commits_delete"
	actionMergeRequestCreateTodo           harness.ActionID = "merge_request.create_todo"
	actionMergeRequestRelatedIssues        harness.ActionID = "merge_request.related_issues"
	actionMergeRequestCreatePipeline       harness.ActionID = "merge_request.create_pipeline"
)

// The review group: a request's changes and diff versions.
const (
	actionMRReviewChangesGet       harness.ActionID = "mr_review.changes_get"
	actionMRReviewDiffVersionsList harness.ActionID = "mr_review.diff_versions_list"
	actionMRReviewDiffVersionGet   harness.ActionID = "mr_review.diff_version_get"
)

// The review group: a request's threaded discussions.
const (
	actionMRReviewDiscussionCreate     harness.ActionID = "mr_review.discussion_create"
	actionMRReviewDiscussionList       harness.ActionID = "mr_review.discussion_list"
	actionMRReviewDiscussionGet        harness.ActionID = "mr_review.discussion_get"
	actionMRReviewDiscussionReply      harness.ActionID = "mr_review.discussion_reply"
	actionMRReviewDiscussionResolve    harness.ActionID = "mr_review.discussion_resolve"
	actionMRReviewDiscussionNoteUpdate harness.ActionID = "mr_review.discussion_note_update"
	actionMRReviewDiscussionNoteDelete harness.ActionID = "mr_review.discussion_note_delete"
)

// The review group: a request's plain notes.
const (
	actionMRReviewNoteCreate harness.ActionID = "mr_review.note_create"
	actionMRReviewNoteList   harness.ActionID = "mr_review.note_list"
	actionMRReviewNoteGet    harness.ActionID = "mr_review.note_get"
	actionMRReviewNoteUpdate harness.ActionID = "mr_review.note_update"
	actionMRReviewNoteDelete harness.ActionID = "mr_review.note_delete"
)

// The review group: a request's draft notes and the two ways to publish
// them.
const (
	actionMRReviewDraftNoteCreate     harness.ActionID = "mr_review.draft_note_create"
	actionMRReviewDraftNoteList       harness.ActionID = "mr_review.draft_note_list"
	actionMRReviewDraftNoteGet        harness.ActionID = "mr_review.draft_note_get"
	actionMRReviewDraftNoteUpdate     harness.ActionID = "mr_review.draft_note_update"
	actionMRReviewDraftNoteDelete     harness.ActionID = "mr_review.draft_note_delete"
	actionMRReviewDraftNotePublish    harness.ActionID = "mr_review.draft_note_publish"
	actionMRReviewDraftNotePublishAll harness.ActionID = "mr_review.draft_note_publish_all"
)

// A personal snippet beyond its create, get and delete: its content, the
// listings, and the read of one file at a ref.
const (
	actionSnippetUpdate      harness.ActionID = "snippet.update"
	actionSnippetContent     harness.ActionID = "snippet.content"
	actionSnippetList        harness.ActionID = "snippet.list"
	actionSnippetListAll     harness.ActionID = "snippet.list_all"
	actionSnippetExplore     harness.ActionID = "snippet.explore"
	actionSnippetFileContent harness.ActionID = "snippet.file_content"
)

// A project snippet's lifecycle.
const (
	actionSnippetProjectCreate  harness.ActionID = "snippet.project_create"
	actionSnippetProjectList    harness.ActionID = "snippet.project_list"
	actionSnippetProjectGet     harness.ActionID = "snippet.project_get"
	actionSnippetProjectContent harness.ActionID = "snippet.project_content"
	actionSnippetProjectUpdate  harness.ActionID = "snippet.project_update"
	actionSnippetProjectDelete  harness.ActionID = "snippet.project_delete"
)

// A project snippet's notes.
const (
	actionSnippetNoteCreate harness.ActionID = "snippet.note_create"
	actionSnippetNoteList   harness.ActionID = "snippet.note_list"
	actionSnippetNoteGet    harness.ActionID = "snippet.note_get"
	actionSnippetNoteUpdate harness.ActionID = "snippet.note_update"
	actionSnippetNoteDelete harness.ActionID = "snippet.note_delete"
)

// A project snippet's threaded discussions.
const (
	actionSnippetDiscussionCreate     harness.ActionID = "snippet.discussion_create"
	actionSnippetDiscussionList       harness.ActionID = "snippet.discussion_list"
	actionSnippetDiscussionGet        harness.ActionID = "snippet.discussion_get"
	actionSnippetDiscussionAddNote    harness.ActionID = "snippet.discussion_add_note"
	actionSnippetDiscussionUpdateNote harness.ActionID = "snippet.discussion_update_note"
	actionSnippetDiscussionDeleteNote harness.ActionID = "snippet.discussion_delete_note"
)

// A project wiki's pages and attachments.
const (
	actionWikiCreate           harness.ActionID = "wiki.create"
	actionWikiGet              harness.ActionID = "wiki.get"
	actionWikiList             harness.ActionID = "wiki.list"
	actionWikiUpdate           harness.ActionID = "wiki.update"
	actionWikiDelete           harness.ActionID = "wiki.delete"
	actionWikiUploadAttachment harness.ActionID = "wiki.upload_attachment"
)

// The search scopes.
const (
	actionSearchCode          harness.ActionID = "search.code"
	actionSearchProjects      harness.ActionID = "search.projects"
	actionSearchIssues        harness.ActionID = "search.issues"
	actionSearchMergeRequests harness.ActionID = "search.merge_requests"
	actionSearchCommits       harness.ActionID = "search.commits"
	actionSearchMilestones    harness.ActionID = "search.milestones"
	actionSearchNotes         harness.ActionID = "search.notes"
	actionSearchSnippets      harness.ActionID = "search.snippets"
	actionSearchUsers         harness.ActionID = "search.users"
	actionSearchWiki          harness.ActionID = "search.wiki"
)
