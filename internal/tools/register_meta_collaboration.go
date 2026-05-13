package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupiterations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuediscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovalsettings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerMergeRequestMeta registers the gitlab_merge_request meta-tool with actions:
// create, get, list, list_global, list_group, update, merge, approve, unapprove,
// commits, pipelines, delete, rebase, participants, reviewers, create_pipeline,
// issues_closed, cancel_auto_merge, approval_state, approval_rules, approval_config,
// approval_reset, approval_rule_create, approval_rule_update, approval_rule_delete,
// approval_settings_group_get, approval_settings_group_update,
// approval_settings_project_get, approval_settings_project_update,
// subscribe, unsubscribe, time_estimate_set, time_estimate_reset, spent_time_add,
// spent_time_reset, time_stats, context_commits_list, context_commits_create,
// context_commits_delete, create_todo, related_issues,
// dependencies_list, dependency_create, dependency_delete.
func registerMergeRequestMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"create":                           routeAction(client, mergerequests.Create),
		"get":                              routeAction(client, mergerequests.Get),
		"list":                             routeAction(client, mergerequests.List),
		"list_global":                      routeAction(client, mergerequests.ListGlobal),
		"list_group":                       routeAction(client, mergerequests.ListGroup),
		"update":                           routeAction(client, mergerequests.Update),
		"merge":                            destructiveAction(client, mergerequests.Merge),
		"approve":                          routeAction(client, mergerequests.Approve),
		"unapprove":                        destructiveVoidAction(client, mergerequests.Unapprove),
		"commits":                          routeAction(client, mergerequests.Commits),
		"pipelines":                        routeAction(client, mergerequests.Pipelines),
		"delete":                           destructiveVoidAction(client, mergerequests.Delete),
		"rebase":                           routeAction(client, mergerequests.Rebase),
		"participants":                     routeAction(client, mergerequests.Participants),
		"reviewers":                        routeAction(client, mergerequests.Reviewers),
		"create_pipeline":                  routeAction(client, mergerequests.CreatePipeline),
		"issues_closed":                    routeAction(client, mergerequests.IssuesClosed),
		"cancel_auto_merge":                routeAction(client, mergerequests.CancelAutoMerge),
		"approval_state":                   routeAction(client, mrapprovals.State),
		"approval_rules":                   routeAction(client, mrapprovals.Rules),
		"approval_config":                  routeAction(client, mrapprovals.Config),
		"approval_reset":                   destructiveVoidAction(client, mrapprovals.Reset),
		"approval_rule_create":             routeAction(client, mrapprovals.CreateRule),
		"approval_rule_update":             routeAction(client, mrapprovals.UpdateRule),
		"approval_rule_delete":             destructiveVoidAction(client, mrapprovals.DeleteRule),
		"approval_settings_group_get":      routeAction(client, mrapprovalsettings.GetGroupSettings),
		"approval_settings_group_update":   routeAction(client, mrapprovalsettings.UpdateGroupSettings),
		"approval_settings_project_get":    routeAction(client, mrapprovalsettings.GetProjectSettings),
		"approval_settings_project_update": routeAction(client, mrapprovalsettings.UpdateProjectSettings),
		"subscribe":                        routeAction(client, mergerequests.Subscribe),
		"unsubscribe":                      routeAction(client, mergerequests.Unsubscribe),
		"time_estimate_set":                routeAction(client, mergerequests.SetTimeEstimate),
		"time_estimate_reset":              routeAction(client, mergerequests.ResetTimeEstimate),
		"spent_time_add":                   routeAction(client, mergerequests.AddSpentTime),
		"spent_time_reset":                 routeAction(client, mergerequests.ResetSpentTime),
		"time_stats":                       routeAction(client, mergerequests.GetTimeStats),
		"context_commits_list":             routeAction(client, mrcontextcommits.List),
		"context_commits_create":           routeAction(client, mrcontextcommits.Create),
		"context_commits_delete":           destructiveVoidAction(client, mrcontextcommits.Delete),
		"create_todo":                      routeAction(client, mergerequests.CreateTodo),
		"related_issues":                   routeAction(client, mergerequests.RelatedIssues),
		"dependencies_list":                routeAction(client, mergerequests.GetDependencies),
		"dependency_create":                routeAction(client, mergerequests.CreateDependency),
		"dependency_delete":                destructiveVoidAction(client, mergerequests.DeleteDependency),
		"emoji_mr_list":                    routeAction(client, awardemoji.ListMRAwardEmoji),
		"emoji_mr_get":                     routeAction(client, awardemoji.GetMRAwardEmoji),
		"emoji_mr_create":                  routeAction(client, awardemoji.CreateMRAwardEmoji),
		"emoji_mr_delete":                  destructiveVoidAction(client, awardemoji.DeleteMRAwardEmoji),
		"emoji_mr_note_list":               routeAction(client, awardemoji.ListMRNoteAwardEmoji),
		"emoji_mr_note_get":                routeAction(client, awardemoji.GetMRNoteAwardEmoji),
		"emoji_mr_note_create":             routeAction(client, awardemoji.CreateMRNoteAwardEmoji),
		"emoji_mr_note_delete":             destructiveVoidAction(client, awardemoji.DeleteMRNoteAwardEmoji),
		"event_mr_label_list":              routeAction(client, resourceevents.ListMRLabelEvents),
		"event_mr_label_get":               routeAction(client, resourceevents.GetMRLabelEvent),
		"event_mr_milestone_list":          routeAction(client, resourceevents.ListMRMilestoneEvents),
		"event_mr_milestone_get":           routeAction(client, resourceevents.GetMRMilestoneEvent),
		"event_mr_state_list":              routeAction(client, resourceevents.ListMRStateEvents),
		"event_mr_state_get":               routeAction(client, resourceevents.GetMRStateEvent),
	}

	addMetaTool(server, "gitlab_merge_request", `Manage GitLab merge request lifecycle plus approval rules and settings, time tracking, subscriptions, context commits, MR dependencies (blocking MRs), todos, related issues, award emoji, and resource events. Delete permanently removes an MR.
When to use: MR lifecycle (open/list/update/merge/close/delete/rebase), approvals at MR/group/project level, time tracking, subscriptions, context commits, MR dependencies, todos, related issues, award emoji, MR resource events.
NOT for: comments, discussions, diffs, draft notes, raw diffs (use gitlab_mr_review), CI pipelines (use gitlab_pipeline; use action 'pipelines' here only to LIST MR pipelines), branches/tags (use gitlab_branch / gitlab_tag), commits in the repo (use gitlab_repository).

Returns:
- list / list_global / list_group / commits / pipelines / participants / reviewers / issues_closed / related_issues / dependencies_list / approval_rules / context_commits_list / event_*_list / emoji_*_list: arrays with pagination {page, per_page, total, next_page}.
- get / create / update / approve / merge / rebase / approval_state / approval_config / approval_rule_create / approval_rule_update / approval_settings_* / dependency_create / create_todo: MR, dependency, todo or settings object.
- time_estimate_set / spent_time_add / time_stats / time_estimate_reset / spent_time_reset: {time_estimate, total_time_spent, human_time_estimate, human_total_time_spent}.
- subscribe / unsubscribe / cancel_auto_merge / create_pipeline: updated MR or pipeline object.
- delete / unapprove / approval_reset / approval_rule_delete / context_commits_delete / dependency_delete / emoji_*_delete: {success, message}.
Errors: 404 (hint: confirm project_id and merge_request_iid — merge_request_iid is project-scoped, not the global ID), 403 (hint: requires Reporter+ to comment, Developer+ to merge, configured approvers to approve), 405/409 on merge (hint: WIP/draft, unresolved threads, failing pipelines or pending approvals — see approval_state).

Param conventions: * = required. Most actions need project_id*, merge_request_iid*. List actions accept page, per_page.

IMPORTANT for create: target_branch* — if user doesn't specify, retrieve project's default_branch via gitlab_project get; do NOT assume 'main'.
IMPORTANT for merge: auto-detects project squash/delete-branch settings — do NOT set squash or should_remove_source_branch unless user explicitly asks.

MR lifecycle:
- create: project_id*, source_branch*, target_branch*, title*, description, assignee_id, assignee_ids, reviewer_ids, labels (comma-separated), milestone_id, remove_source_branch, squash, allow_collaboration, target_project_id (forks)
- get: project_id*, merge_request_iid*
- list: project_id*, state (opened/closed/merged/all), labels, not_labels, milestone, scope, search, source_branch, target_branch, author_username, draft, iids, created_after/before, updated_after/before, order_by, sort
- list_global / list_group: same filters as list. list_group needs group_id* instead of project_id.
- update: project_id*, merge_request_iid*, title, description, target_branch, assignee_id, assignee_ids, reviewer_ids, labels, add_labels, remove_labels, milestone_id, remove_source_branch, squash, discussion_locked, allow_collaboration, state_event (close/reopen)
- merge: project_id*, merge_request_iid*, merge_commit_message, squash, should_remove_source_branch, auto_merge, sha, squash_commit_message
- approve / unapprove / rebase / delete / participants / reviewers / create_pipeline / cancel_auto_merge: project_id*, merge_request_iid*
- rebase also accepts: skip_ci
- commits / pipelines / issues_closed: project_id*, merge_request_iid*
- subscribe / unsubscribe: project_id*, merge_request_iid*

Approvals:
- approval_state / approval_rules / approval_config / approval_reset: project_id*, merge_request_iid*
- approval_rule_create: project_id*, merge_request_iid*, name*, approvals_required*, approval_project_rule_id, user_ids, group_ids
- approval_rule_update: project_id*, merge_request_iid*, approval_rule_id*, name, approvals_required, user_ids, group_ids
- approval_rule_delete: project_id*, merge_request_iid*, approval_rule_id*
- approval_settings_group_get / approval_settings_group_update: group_id*. Update params: allow_author_approval, allow_committer_approval, allow_overrides_approver_list_per_mr, retain_approvals_on_push, require_reauthentication_to_approve
- approval_settings_project_get / approval_settings_project_update: project_id*. Same params + selective_code_owner_removals.

Time tracking:
- time_estimate_set / spent_time_add: project_id*, merge_request_iid*, duration* (e.g. '3h30m', '1w2d'). spent_time_add also accepts summary.
- time_estimate_reset / spent_time_reset / time_stats: project_id*, merge_request_iid*

Context commits:
- context_commits_list / context_commits_create / context_commits_delete: project_id*, merge_request_iid*. create/delete need commits ([]string)*.

MR dependencies (blocking MRs):
- dependencies_list: project_id*, merge_request_iid* — list MRs that block this MR from merging.
- dependency_create: project_id*, merge_request_iid*, blocking_merge_request_id* (global ID of the blocking MR).
- dependency_delete: project_id*, merge_request_iid*, blocking_merge_request_id*.

Todos and related issues:
- create_todo: project_id*, merge_request_iid* — add this MR to the authenticated user's to-do list.
- related_issues: project_id*, merge_request_iid* — list issues mentioned or linked from the MR (paginated).

Award emoji:
- emoji_mr_list / emoji_mr_create / emoji_mr_delete: project_id*, merge_request_iid*, name* (create), award_id* (get/delete)
- emoji_mr_get: project_id*, merge_request_iid*, award_id*
- emoji_mr_note_list / emoji_mr_note_create / emoji_mr_note_delete: project_id*, merge_request_iid*, note_id*, name* (create), award_id* (get/delete)
- emoji_mr_note_get: project_id*, merge_request_iid*, note_id*, award_id*

Resource events:
- event_mr_label_list / event_mr_label_get: project_id*, merge_request_iid*, label_event_id* (get)
- event_mr_milestone_list / event_mr_milestone_get: project_id*, merge_request_iid*, milestone_event_id* (get)
- event_mr_state_list / event_mr_state_get: project_id*, merge_request_iid*, state_event_id* (get)

See also: gitlab_mr_review (comments, discussions, diffs, raw diffs, draft notes), gitlab_pipeline, gitlab_branch, gitlab_issue (linked/related issue lifecycle)`, routes, toolutil.IconMR)
}

// registerMRReviewMeta registers the gitlab_mr_review meta-tool with actions:
// note_create, note_list, note_update, note_delete, discussion_create,
// discussion_list, discussion_get, discussion_reply, discussion_resolve,
// discussion_note_update, discussion_note_delete, changes_get, raw_diffs,
// draft_note_list, draft_note_get, draft_note_create, draft_note_update,
// draft_note_delete, draft_note_publish, draft_note_publish_all,
// diff_versions_list, diff_version_get.
func registerMRReviewMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"note_create":            routeAction(client, mrnotes.Create),
		"note_list":              routeAction(client, mrnotes.List),
		"note_get":               routeAction(client, mrnotes.GetNote),
		"note_update":            routeAction(client, mrnotes.Update),
		"note_delete":            destructiveVoidAction(client, mrnotes.Delete),
		"discussion_create":      routeAction(client, mrdiscussions.Create),
		"discussion_list":        routeAction(client, mrdiscussions.List),
		"discussion_get":         routeAction(client, mrdiscussions.Get),
		"discussion_reply":       routeAction(client, mrdiscussions.Reply),
		"discussion_resolve":     routeAction(client, mrdiscussions.Resolve),
		"discussion_note_update": routeAction(client, mrdiscussions.UpdateNote),
		"discussion_note_delete": destructiveVoidAction(client, mrdiscussions.DeleteNote),
		"changes_get":            routeAction(client, mrchanges.Get),
		"raw_diffs":              routeAction(client, mrchanges.RawDiffs),
		"draft_note_list":        routeAction(client, mrdraftnotes.List),
		"draft_note_get":         routeAction(client, mrdraftnotes.Get),
		"draft_note_create":      routeAction(client, mrdraftnotes.Create),
		"draft_note_update":      routeAction(client, mrdraftnotes.Update),
		"draft_note_delete":      destructiveVoidAction(client, mrdraftnotes.Delete),
		"draft_note_publish":     routeVoidAction(client, mrdraftnotes.Publish),
		"draft_note_publish_all": routeVoidAction(client, mrdraftnotes.PublishAll),
		"diff_versions_list":     routeAction(client, mrchanges.ListDiffVersions),
		"diff_version_get":       routeAction(client, mrchanges.GetDiffVersion),
	}

	addMetaTool(server, "gitlab_mr_review", `Review and comment on GitLab merge requests: notes, threaded discussions (inline + general), code diffs, draft notes (batch review), diff versions, and the per-version diff payload.
When to use: post review comments, open or resolve discussion threads, fetch the diff to comment inline, queue draft notes during a session and publish them as a single review. For prompts like "inspect/view MR changes/diffs" or "without running an LLM analyzer", choose changes_get first.
NOT for: MR lifecycle — create/update/merge/approve/rebase/delete (use gitlab_merge_request), reactions on MR notes (use gitlab_merge_request emoji_mr_note_*), CI pipelines on the MR (use gitlab_pipeline or gitlab_merge_request pipelines).

IMPORTANT — action choice: use note_create only for a general/top-level MR comment with no file or line position. Use discussion_create without position for a general threaded discussion. Add a position object only for inline review comments, including prompts like "comment on this line/file/hunk". If it says draft review note or batch review, use draft_note_create first and draft_note_publish_all once at the end. For raw patch text, use raw_diffs; for structured MR changes by file, use changes_get.

Returns:
- *_list: array with pagination (page, per_page, total, next_page).
- note_*, discussion_*, draft_note_*, diff_*: resource object(s) with id, body/note, author, position (when inline).
- changes_get: {changes: [{old_path, new_path, diff, ...}], truncated_files} — if truncated, use diff_versions_list + diff_version_get, or raw_diffs for the full unified diff payload.
- raw_diffs: {raw_diff: string} — full unified diff for the MR head; ideal when changes_get returns truncated_files.
- *_delete / *_publish: {success: bool, message: string}.
Errors: 404 not found (hint: check note_id/discussion_id and merge_request_iid), 403 forbidden (hint: requires Reporter+ to comment), 400 invalid params (hint: position requires base_sha + start_sha + head_sha + new_path/old_path + new_line/old_line).

Param conventions: * = required. All actions need project_id*, merge_request_iid*. List actions accept page, per_page. position object: {base_sha, start_sha, head_sha, new_path, old_path, new_line (added/modified), old_line (removed), both lines for unchanged context}.

Notes (general comments):
- note_list: order_by (created_at/updated_at), sort
- note_get / note_delete: note_id*
- note_create: body*
- note_update: note_id*, body*

Discussions (threaded, can be inline via position):
- discussion_list
- discussion_get: discussion_id*
- discussion_create: body*, position (inline)
- discussion_reply: discussion_id*, body*
- discussion_resolve: discussion_id*, resolved* (bool)
- discussion_note_update: discussion_id*, note_id*, body, resolved
- discussion_note_delete: discussion_id*, note_id*

Changes and diff versions:
- changes_get: returns structured MR file diffs by merge_request_iid; use this for "inspect/view MR changes" and for comments that need new_path/old_path/new_line/old_line.
- raw_diffs: project_id*, merge_request_iid* — returns the full raw unified diff for the MR head (use only when a raw patch/unified diff is requested or changes_get reports truncated_files)
- diff_versions_list: list MR diff revisions
- diff_version_get: version_id*, unidiff (bool)

Draft notes (batch review, not immediately published):
- draft_note_list: order_by, sort
- draft_note_get: note_id*
- draft_note_create: note*, commit_id, in_reply_to_discussion_id, resolve_discussion (bool), position
- draft_note_update: note_id*, note, position
- draft_note_delete / draft_note_publish: note_id*
- draft_note_publish_all: publishes ALL pending drafts as a single review notification

See also: gitlab_merge_request (MR lifecycle, approvals, merge, time tracking, reactions), gitlab_pipeline (MR pipelines), gitlab_repository (file blame for context).`, routes, toolutil.IconDiscussion)
}

// registerIssueMeta registers the gitlab_issue meta-tool with actions:
// create, get, list, update, delete, note_create, note_list, note_get,
// note_update, note_delete, list_group, link_list, link_get, link_create, link_delete,
// work_item_get, work_item_list, work_item_create, work_item_update, work_item_delete.
func registerIssueMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	routes := actionMap{
		"create":                     routeAction(client, issues.Create),
		"get":                        routeAction(client, issues.Get),
		"get_by_id":                  routeAction(client, issues.GetByID),
		"list":                       routeAction(client, issues.List),
		"list_all":                   routeAction(client, issues.ListAll),
		"update":                     routeAction(client, issues.Update),
		"delete":                     destructiveVoidAction(client, issues.Delete),
		"list_group":                 routeAction(client, issues.ListGroup),
		"reorder":                    routeAction(client, issues.Reorder),
		"move":                       routeAction(client, issues.Move),
		"subscribe":                  routeAction(client, issues.Subscribe),
		"unsubscribe":                routeAction(client, issues.Unsubscribe),
		"create_todo":                routeAction(client, issues.CreateTodo),
		"note_create":                routeAction(client, issuenotes.Create),
		"note_list":                  routeAction(client, issuenotes.List),
		"note_get":                   routeAction(client, issuenotes.GetNote),
		"note_update":                routeAction(client, issuenotes.Update),
		"note_delete":                destructiveVoidAction(client, issuenotes.Delete),
		"link_list":                  routeAction(client, issuelinks.List),
		"link_get":                   routeAction(client, issuelinks.Get),
		"link_create":                routeAction(client, issuelinks.Create),
		"link_delete":                destructiveVoidAction(client, issuelinks.Delete),
		"time_estimate_set":          routeAction(client, issues.SetTimeEstimate),
		"time_estimate_reset":        routeAction(client, issues.ResetTimeEstimate),
		"spent_time_add":             routeAction(client, issues.AddSpentTime),
		"spent_time_reset":           routeAction(client, issues.ResetSpentTime),
		"time_stats_get":             routeAction(client, issues.GetTimeStats),
		"participants":               routeAction(client, issues.GetParticipants),
		"mrs_closing":                routeAction(client, issues.ListMRsClosing),
		"mrs_related":                routeAction(client, issues.ListMRsRelated),
		"work_item_get":              routeAction(client, workitems.Get),
		"work_item_list":             routeAction(client, workitems.List),
		"work_item_create":           routeAction(client, workitems.Create),
		"work_item_update":           routeAction(client, workitems.Update),
		"work_item_delete":           destructiveVoidAction(client, workitems.Delete),
		"discussion_list":            routeAction(client, issuediscussions.List),
		"discussion_get":             routeAction(client, issuediscussions.Get),
		"discussion_create":          routeAction(client, issuediscussions.Create),
		"discussion_add_note":        routeAction(client, issuediscussions.AddNote),
		"discussion_update_note":     routeAction(client, issuediscussions.UpdateNote),
		"discussion_delete_note":     destructiveVoidAction(client, issuediscussions.DeleteNote),
		"statistics_get":             routeAction(client, issuestatistics.Get),
		"statistics_get_group":       routeAction(client, issuestatistics.GetGroup),
		"statistics_get_project":     routeAction(client, issuestatistics.GetProject),
		"emoji_issue_list":           routeAction(client, awardemoji.ListIssueAwardEmoji),
		"emoji_issue_get":            routeAction(client, awardemoji.GetIssueAwardEmoji),
		"emoji_issue_create":         routeAction(client, awardemoji.CreateIssueAwardEmoji),
		"emoji_issue_delete":         destructiveVoidAction(client, awardemoji.DeleteIssueAwardEmoji),
		"emoji_issue_note_list":      routeAction(client, awardemoji.ListIssueNoteAwardEmoji),
		"emoji_issue_note_get":       routeAction(client, awardemoji.GetIssueNoteAwardEmoji),
		"emoji_issue_note_create":    routeAction(client, awardemoji.CreateIssueNoteAwardEmoji),
		"emoji_issue_note_delete":    destructiveVoidAction(client, awardemoji.DeleteIssueNoteAwardEmoji),
		"event_issue_label_list":     routeAction(client, resourceevents.ListIssueLabelEvents),
		"event_issue_label_get":      routeAction(client, resourceevents.GetIssueLabelEvent),
		"event_issue_milestone_list": routeAction(client, resourceevents.ListIssueMilestoneEvents),
		"event_issue_milestone_get":  routeAction(client, resourceevents.GetIssueMilestoneEvent),
		"event_issue_state_list":     routeAction(client, resourceevents.ListIssueStateEvents),
		"event_issue_state_get":      routeAction(client, resourceevents.GetIssueStateEvent),
		"event_issue_iteration_list": routeAction(client, resourceevents.ListIssueIterationEvents),
		"event_issue_iteration_get":  routeAction(client, resourceevents.GetIssueIterationEvent),
		"event_issue_weight_list":    routeAction(client, resourceevents.ListIssueWeightEvents),
	}

	if enterprise {
		routes["iteration_list_project"] = routeAction(client, projectiterations.List)
		routes["iteration_list_group"] = routeAction(client, groupiterations.List)
	}

	desc := `Manage GitLab issues: CRUD, notes, discussions, links, time tracking, work items, award emoji, statistics, and resource events.
When to use: issue lifecycle — create, update, close, move, comment, link, time-track, and manage Work Items (including Epics). NOT for: merge requests (use gitlab_merge_request), project settings (use gitlab_project), CI/CD (use gitlab_pipeline).

Side effects: delete/move are irreversible; move changes URL and IID. Time tracking uses dedicated actions — do NOT pass time params to update.

Returns: resource object for single-item actions; paginated list ({page, per_page, total, next_page}) for *_list / list / list_all / list_group / participants / mrs_* / iteration_list_*; GraphQL cursor pagination ({nodes, page_info}) for work_item_list; {success, message} for delete actions.
Errors: 404 (hint: issue_iid is project-scoped — supply project_id; for list_all use scope/iids), 403 (hint: Reporter+ to comment, Developer+ to edit/move), 400 (hint: state_event ∈ close/reopen; dates ISO 8601; weight integer 0–9 — Premium+).

Param conventions: * = required. Most actions need project_id* + issue_iid*. List actions accept page, per_page. Work item actions use full_path* + work_item_iid* (GraphQL).

Issue CRUD:
- create: project_id*, title*, description, assignee_id, assignee_ids ([]int), labels (comma-separated), milestone_id, due_date (YYYY-MM-DD), confidential, issue_type (issue/incident/test_case/task), weight, epic_id
- get: project_id*, issue_iid*
- get_by_id: issue_id* (global ID, no project_id needed)
- list: project_id*, state (opened/closed/all), labels, not_labels, milestone, scope (created_by_me/assigned_to_me/all), search, assignee_username, author_username, iids ([]int), issue_type, confidential, created_after/before, updated_after/before (ISO 8601), order_by (created_at/updated_at/priority/due_date), sort (asc/desc)
- list_all: global issue list (no project_id). Same filters as list.
- list_group: group_id*, state, labels, milestone, scope, search, order_by, sort
- update: project_id*, issue_iid*, title, description, state_event (close/reopen), assignee_id, assignee_ids, labels, add_labels, remove_labels, milestone_id, due_date, confidential, issue_type, weight, epic_id, discussion_locked
- delete: project_id*, issue_iid* (permanent, irreversible)
- reorder: project_id*, issue_iid*, move_after_id, move_before_id
- move: project_id*, issue_iid*, to_project_id* (moves to another project)
- subscribe / unsubscribe: project_id*, issue_iid*
- create_todo: project_id*, issue_iid*

Time tracking:
- time_estimate_set: project_id*, issue_iid*, duration* (e.g. 3h30m)
- time_estimate_reset / spent_time_reset: project_id*, issue_iid*
- spent_time_add: project_id*, issue_iid*, duration*, summary
- time_stats_get: project_id*, issue_iid*

Participants & related MRs:
- participants: project_id*, issue_iid*
- mrs_closing / mrs_related: project_id*, issue_iid*

Notes (note_*): project_id*, issue_iid* for all.
- note_list: order_by, sort
- note_get / note_delete: note_id*
- note_create: body*, internal
- note_update: note_id*, body*

Issue links (link_*): project_id*, issue_iid* for all.
- link_list
- link_get / link_delete: issue_link_id*
- link_create: target_project_id*, target_issue_iid*, link_type

Discussions (discussion_*): project_id*, issue_iid* for all.
- discussion_list
- discussion_get: discussion_id*
- discussion_create: body*
- discussion_add_note: discussion_id*, body*
- discussion_update_note: discussion_id*, note_id*, body*
- discussion_delete_note: discussion_id*, note_id*

Work Items (work_item_*): full_path* for all. Use types=["Epic"] to list epics (replaces deprecated epic_list).
- work_item_list: state, search, types, author_username, label_name, confidential, sort, first, after
- work_item_get: work_item_iid*
- work_item_create: work_item_type_id*, title*, description, confidential, assignee_ids, milestone_id, label_ids, weight, health_status, color, due_date, start_date, linked_items {work_item_ids, link_type}
- work_item_update: work_item_iid*, title, state_event (CLOSE/REOPEN), description, assignee_ids, milestone_id, crm_contact_ids, parent_id, add_label_ids, remove_label_ids, start_date, due_date, weight, health_status, iteration_id, color, status (TODO/IN_PROGRESS/DONE/WONT_DO/DUPLICATE)
- work_item_delete: work_item_iid* (permanent)

Statistics:
- statistics_get: global issue stats (optional filters same as list)
- statistics_get_group: group_id*
- statistics_get_project: project_id*

Award emoji (emoji_issue_*): project_id*, issue_iid* for all.
- emoji_issue_list / emoji_issue_get (+ award_id*) / emoji_issue_delete (+ award_id*)
- emoji_issue_create: name*
- emoji_issue_note_list / emoji_issue_note_get: note_id*, (+ award_id* for get)
- emoji_issue_note_create: note_id*, name*
- emoji_issue_note_delete: note_id*, award_id*

Resource events (event_issue_*): project_id*, issue_iid* for all.
- event_issue_label_list / event_issue_label_get (+ label_event_id*)
- event_issue_milestone_list / event_issue_milestone_get (+ milestone_event_id*)
- event_issue_state_list / event_issue_state_get (+ state_event_id*)
- event_issue_iteration_list / event_issue_iteration_get (+ iteration_event_id*)
- event_issue_weight_list`

	if enterprise {
		desc += `

Iterations (Premium+ — requires GITLAB_ENTERPRISE=true):
- iteration_list_project: project_id*, state (1=opened, 2=upcoming, 3=current, 4=closed), search, include_ancestors
- iteration_list_group: group_id*, state, search, include_ancestors`
	}

	desc += `

See also: gitlab_merge_request (MR lifecycle), gitlab_project (project settings, labels, milestones), gitlab_pipeline (CI/CD).`

	addMetaTool(server, "gitlab_issue", desc, routes, toolutil.IconIssue)
}

// registerWikiMeta registers the gitlab_wiki meta-tool with actions:
// list, get, create, update, delete, upload_attachment.
func registerWikiMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":              routeAction(client, wikis.List),
		"get":               routeAction(client, wikis.Get),
		"create":            routeAction(client, wikis.Create),
		"update":            routeAction(client, wikis.Update),
		"delete":            destructiveVoidAction(client, wikis.Delete),
		"upload_attachment": routeAction(client, wikis.UploadAttachment),
	}

	addMetaTool(server, "gitlab_wiki", `CRUD project wiki pages and upload attachments to wikis. Delete is destructive and irreversible.
When to use: read, write, or delete wiki pages of a project; attach images or files referenced from wiki content.
NOT for: repository files or commits (use gitlab_repository), code snippets (use gitlab_snippet), group-level wikis (Enterprise/Premium — use gitlab_group when GITLAB_ENTERPRISE=true), issues or MR descriptions (use gitlab_issue / gitlab_merge_request).

Returns:
- get / create / update: {slug, title, format, content, encoding}.
- list: array of {slug, title, format} (content omitted unless with_content=true).
- delete: {success: bool, message: string}.
- upload_attachment: {file_name, url, alt, markdown} — embed `+"`markdown`"+` directly in a page.
Errors: 404 not found (hint: check slug or project_id), 403 forbidden (hint: wiki disabled or insufficient role), 400 invalid params (hint: title/content required, slug must be URL-encoded).

Param conventions: * = required. All actions need project_id* (numeric ID or url-encoded path). slug is the URL-encoded page path (e.g. `+"`docs%2Fsetup`"+`). format default = markdown. content max ~1 MB.

- list: project_id*, with_content (bool)
- get: project_id*, slug*, render_html (bool), version (commit SHA)
- create: project_id*, title*, content*, format (markdown/rdoc/asciidoc/org)
- update: project_id*, slug*, title, content, format
- delete: project_id*, slug*
- upload_attachment: project_id*, filename*, content_base64 OR file_path (exactly one), branch

See also: gitlab_project (settings/membership), gitlab_repository (file commits), gitlab_snippet (standalone code snippets).`, routes, toolutil.IconWiki)
}

// registerSnippetMeta registers the gitlab_snippet meta-tool with actions:
// list, list_all, get, content, file_content, create, update, delete, explore,
// project_list, project_get, project_content, project_create, project_update, project_delete,
// discussion_list, discussion_get, discussion_create, discussion_add_note,
// discussion_update_note, discussion_delete_note, note_list, note_get, note_create,
// note_update, and note_delete.
func registerSnippetMeta(server *mcp.Server, client *gitlabclient.Client) {
	createRoute := routeAction(client, snippets.Create)
	createRoute.InputSchema = snippets.CreateInputSchemaMap()
	projectCreateRoute := routeAction(client, snippets.ProjectCreate)
	projectCreateRoute.InputSchema = snippets.ProjectCreateInputSchemaMap()

	routes := actionMap{
		"list":                      routeAction(client, snippets.List),
		"list_all":                  routeAction(client, snippets.ListAll),
		"get":                       routeAction(client, snippets.Get),
		"content":                   routeAction(client, snippets.Content),
		"file_content":              routeAction(client, snippets.FileContent),
		"create":                    createRoute,
		"update":                    routeAction(client, snippets.Update),
		"delete":                    destructiveVoidAction(client, snippets.Delete),
		"explore":                   routeAction(client, snippets.Explore),
		"project_list":              routeAction(client, snippets.ProjectList),
		"project_get":               routeAction(client, snippets.ProjectGet),
		"project_content":           routeAction(client, snippets.ProjectContent),
		"project_create":            projectCreateRoute,
		"project_update":            routeAction(client, snippets.ProjectUpdate),
		"project_delete":            destructiveVoidAction(client, snippets.ProjectDelete),
		"discussion_list":           routeAction(client, snippetdiscussions.List),
		"discussion_get":            routeAction(client, snippetdiscussions.Get),
		"discussion_create":         routeAction(client, snippetdiscussions.Create),
		"discussion_add_note":       routeAction(client, snippetdiscussions.AddNote),
		"discussion_update_note":    routeAction(client, snippetdiscussions.UpdateNote),
		"discussion_delete_note":    destructiveVoidAction(client, snippetdiscussions.DeleteNote),
		"note_list":                 routeAction(client, snippetnotes.List),
		"note_get":                  routeAction(client, snippetnotes.Get),
		"note_create":               routeAction(client, snippetnotes.Create),
		"note_update":               routeAction(client, snippetnotes.Update),
		"note_delete":               destructiveVoidAction(client, snippetnotes.Delete),
		"emoji_snippet_list":        routeAction(client, awardemoji.ListSnippetAwardEmoji),
		"emoji_snippet_get":         routeAction(client, awardemoji.GetSnippetAwardEmoji),
		"emoji_snippet_create":      routeAction(client, awardemoji.CreateSnippetAwardEmoji),
		"emoji_snippet_delete":      destructiveVoidAction(client, awardemoji.DeleteSnippetAwardEmoji),
		"emoji_snippet_note_list":   routeAction(client, awardemoji.ListSnippetNoteAwardEmoji),
		"emoji_snippet_note_get":    routeAction(client, awardemoji.GetSnippetNoteAwardEmoji),
		"emoji_snippet_note_create": routeAction(client, awardemoji.CreateSnippetNoteAwardEmoji),
		"emoji_snippet_note_delete": destructiveVoidAction(client, awardemoji.DeleteSnippetNoteAwardEmoji),
	}
	addMetaTool(server, "gitlab_snippet", `Manage GitLab snippets (personal, project-scoped, and explore feed): CRUD snippet metadata and content, threaded discussions, notes (project snippets only), and award emoji on snippets and snippet notes. Delete is destructive and irreversible.
When to use: store/share standalone code or text outside a repo, comment on existing snippets, react with emoji on a snippet or snippet note, browse public snippets via explore.
NOT for: files in a repository (use gitlab_repository), wiki pages (use gitlab_wiki), MR/issue notes (use gitlab_mr_review or gitlab_issue), custom group emoji (use gitlab_custom_emoji — enterprise).

Returns:
- *_list / list_all / explore: array with pagination.
- *_get / *_create / *_update: snippet object {id, title, file_name, files, visibility, author, web_url, raw_url}.
- content / project_content: raw snippet body as text.
- file_content: raw content of a single file in a multi-file snippet.
- discussion_* / note_*: discussion or note object.
- emoji_*: award emoji object.
- *_delete: {success: bool, message: string}.
Errors: 404 not found, 403 forbidden (hint: requires Reporter+ for project snippets; private snippets require ownership), 400 invalid params (hint: visibility ∈ private/internal/public).

Param conventions: * = required. List actions accept page, per_page. visibility ∈ private/internal/public.

Personal snippets:
- list / list_all / explore: (no required params)
- get / content: snippet_id*
- file_content: snippet_id*, file_path*
- create: title*, file_name*, content*, visibility, description
- update: snippet_id*, title, file_name, content, visibility, description
- delete: snippet_id*

Project snippets:
- project_list: project_id*
- project_get / project_content: project_id*, snippet_id*
- project_create: project_id*, title*, file_name*, content*, visibility
- project_update: project_id*, snippet_id*, title, visibility, files* to change snippet file content. For updating existing content use files: [{"action":"update","file_path":"<returned file_path>","content":"..."}]; do not put file_path/content directly under params.
- project_delete: project_id*, snippet_id*

Discussions (threaded):
- discussion_list: snippet_id*
- discussion_get: snippet_id*, discussion_id*
- discussion_create: snippet_id*, body*
- discussion_add_note: snippet_id*, discussion_id*, body*
- discussion_update_note: snippet_id*, discussion_id*, note_id*, body*
- discussion_delete_note: snippet_id*, discussion_id*, note_id*

Notes (project snippets only) — all need project_id*, snippet_id*:
- note_list: order_by, sort
- note_get / note_delete: note_id*
- note_create: body*
- note_update: note_id*, body*

Award emoji — all need project_id*, snippet_id* (snippet emoji) or project_id*, snippet_id*, note_id* (note emoji):
- emoji_snippet_list / emoji_snippet_note_list: (no extra params)
- emoji_snippet_get / emoji_snippet_note_get: award_id*
- emoji_snippet_create / emoji_snippet_note_create: name*
- emoji_snippet_delete / emoji_snippet_note_delete: award_id*

See also: gitlab_repository (project files and commits), gitlab_wiki (long-form project docs), gitlab_project (project membership and visibility), gitlab_custom_emoji (define group-level custom emoji).`, routes, toolutil.IconSnippet)
}
