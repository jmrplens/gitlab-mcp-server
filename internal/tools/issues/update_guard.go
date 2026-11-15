// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// update_guard.go asks before an issue update takes the issue out of its
// epic or removes every assignee, the two effects other actions ask about:
// group.epic_issue_remove and /remove_epic for the first, and
// issue.work_item_update with an empty assignee list and /unassign for the
// second.

package issues

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmUpdateRemovals asks before an update sends epic_id 0, which takes
// the issue out of its epic, and before it sends assignee_ids [] or a list
// that begins with 0, or assignee_id 0, which removes every assignee. It returns nil when the update
// may proceed. The epic is decided by the arguments alone; the assignees are
// read first, so emptying a list that is already empty asks nothing.
func confirmUpdateRemovals(ctx context.Context, client *gitlabclient.Client, input UpdateInput) error {
	target := toolutil.ConfirmTargetProject(string(input.ProjectID))
	if err := toolutil.ConfirmParentRemoval(ctx, "issueUpdate", toolutil.RemovesReference(input.EpicID), "epic_id",
		i18n.New(i18n.ConfirmIssueEpicRemove, i18n.N("iid", input.IssueIID), i18n.M("target", target))); err != nil {
		return err
	}
	clears := toolutil.ClearsAssigneeIDs(input.AssigneeIDs) || toolutil.RemovesReference(input.AssigneeID)
	return toolutil.ConfirmAssigneesCleared(ctx, "issueUpdate", clears,
		func(losses i18n.Message) i18n.Message {
			return i18n.New(i18n.ConfirmIssueAssigneesClear,
				i18n.L("losses", losses), i18n.N("iid", input.IssueIID), i18n.M("target", target))
		},
		func(ctx context.Context) (int, error) { return currentAssignees(ctx, client, input) })
}

// currentAssignees reads how many assignees the issue has, for the question
// [confirmUpdateRemovals] asks before an update removes them all.
func currentAssignees(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (int, error) {
	//gitlab:request optional: the issue is read only when the update empties its assignees and nobody has confirmed it yet
	issue, _, err := client.GL().Issues.GetIssue(string(input.ProjectID), input.IssueIID, gl.WithContext(ctx))
	if err != nil {
		return 0, err
	}
	return len(issue.Assignees), nil
}
