// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// update_guard.go asks before an epic update takes the epic out of its parent
// epic or removes every assignee, the two effects other actions ask about:
// group.epic_issue_remove and /remove_parent_epic for the first, and
// issue.work_item_update with an empty assignee list and /unassign for the
// second.

package epics

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmUpdateRemovals asks before an update sends parent_id 0, which takes
// the epic out of its parent epic, and before it sends assignee_ids [], which
// removes every assignee. It returns nil when the update may proceed. The
// parent is decided by the arguments alone; the assignees are read first, as
// issue.work_item_update reads them, so emptying a list that is already
// empty asks nothing.
func confirmUpdateRemovals(ctx context.Context, client *gitlabclient.Client, input UpdateInput) error {
	path := toolutil.EscapeConsentValue(input.FullPath)
	if err := toolutil.ConfirmParentRemoval(ctx, "epicUpdate", toolutil.RemovesReference(input.ParentID), "parent_id",
		i18n.New(i18n.ConfirmEpicParentRemove, i18n.S("path", path), i18n.N("iid", input.IID))); err != nil {
		return err
	}
	return toolutil.ConfirmAssigneesCleared(ctx, "epicUpdate", toolutil.ClearsList(input.AssigneeIDs),
		func(losses i18n.Message) i18n.Message {
			return i18n.New(i18n.ConfirmEpicAssigneesClear,
				i18n.L("losses", losses), i18n.S("path", path), i18n.N("iid", input.IID))
		},
		func(ctx context.Context) (int, error) { return currentAssignees(ctx, client, input) })
}

// currentAssignees reads how many assignees the epic has, for the question
// [confirmUpdateRemovals] asks before an update removes them all.
func currentAssignees(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (int, error) {
	//gitlab:request optional: the epic is read only when the update empties its assignees and nobody has confirmed it yet
	current, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
	if err != nil {
		return 0, err
	}
	return len(current.Assignees), nil
}
