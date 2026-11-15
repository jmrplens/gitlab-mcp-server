// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// update_guard.go asks before a merge request update removes every assignee,
// as issue.work_item_update asks before an empty assignee list and a text
// asks before /unassign with nobody after it.

package mergerequests

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmAssigneesCleared asks before an update sends assignee_ids [] or a
// list that begins with 0, or assignee_id 0, which removes every assignee, and returns nil when the
// update may proceed. The merge request is read first, so emptying a list
// that is already empty asks nothing. An empty reviewer_ids is not asked
// about, and neither is /unassign_reviewer in a text: no action asks before
// it removes the reviewers of a merge request.
func confirmAssigneesCleared(ctx context.Context, client *gitlabclient.Client, input UpdateInput) error {
	clears := toolutil.ClearsAssigneeIDs(input.AssigneeIDs) || toolutil.RemovesReference(input.AssigneeID)
	return toolutil.ConfirmAssigneesCleared(ctx, "mrUpdate", clears,
		func(losses i18n.Message) i18n.Message {
			return i18n.New(i18n.ConfirmMergeRequestAssigneesClear,
				i18n.L("losses", losses), i18n.N("iid", input.MRIID),
				i18n.M("target", toolutil.ConfirmTargetProject(string(input.ProjectID))))
		},
		func(ctx context.Context) (int, error) { return currentAssignees(ctx, client, input) })
}

// currentAssignees reads how many assignees the merge request has, for the
// question [confirmAssigneesCleared] asks before an update removes them all.
func currentAssignees(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (int, error) {
	//gitlab:request optional: the merge request is read only when the update empties its assignees and nobody has confirmed it yet
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(string(input.ProjectID), input.MRIID, nil, gl.WithContext(ctx))
	if err != nil {
		return 0, err
	}
	return len(mr.Assignees), nil
}
