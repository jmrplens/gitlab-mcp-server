// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// detach_guard.go asks before an update takes an item out of its epic or its
// parent, or removes every assignee of an issue, a merge request or an epic.
// group.epic_issue_remove, /remove_epic and /remove_parent ask before the
// first, and issue.work_item_update asks before the second, so an update
// field that does the same asks the same.

package toolutil

import (
	"context"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// Confirmation identifiers of the two questions, distinct from the
// destructive-action prompt and from each other.
const (
	// ParentRemovalConfirmID is the question issue.update asks before
	// epic_id 0, and issue.work_item_update and group.epic_update before
	// parent_id 0.
	ParentRemovalConfirmID = "parent_removal"
	// AssigneesClearConfirmID is the question issue.update,
	// merge_request.update and group.epic_update ask before they remove
	// every assignee.
	AssigneesClearConfirmID = "assignees_clear"
)

// RemovesReference reports whether an update field that names an epic, a
// parent or an assignee by id is sent as 0, which GitLab reads as removing
// it. A field the call omits is nil and leaves the reference alone.
func RemovesReference(id *int64) bool {
	return id != nil && *id == 0
}

// ClearsList reports whether the call sent a list as an explicit empty array,
// which GitLab reads as removing every entry. A list the call omitted arrives
// nil and leaves the current entries alone.
func ClearsList(ids []int64) bool {
	return ids != nil && len(ids) == 0
}

// ClearsAssigneeIDs reports whether the assignee_ids of a REST update of an
// issue or a merge request remove every assignee: an explicit empty array, or
// a list whose first id is 0. GitLab reads the lone id 0 as "nobody"
// (doc/api/issues.md and merge_requests.md), and on an item that takes a
// single assignee, as every issue and merge request on a Free instance does,
// it keeps only the first id before it looks, so [0, 9] removes them all
// there too. A list whose first id is a user is not asked about: GitLab sets
// the users it resolves, and leaves the assignees alone when it resolves
// none, so such a list never empties them.
func ClearsAssigneeIDs(ids []int64) bool {
	return ClearsList(ids) || (len(ids) > 0 && ids[0] == 0)
}

// ConfirmParentRemoval asks before an update takes an item out of its epic or
// its parent, and returns nil when the update may proceed. removes is whether
// the call sends the field as 0, field is its name, and question names the
// item in the words of its kind.
//
// Taking an item out of its epic is what group.epic_issue_remove does, which
// is destructive, and what /remove_epic and /remove_parent in a text do,
// which ask. Naming another epic or parent is not asked about: the item moves
// under it and the same field moves it back. The arguments alone decide, so
// nothing is read, and an item that has no parent costs a question nobody
// needed rather than a read before every removal.
func ConfirmParentRemoval(ctx context.Context, op string, removes bool, field string, question i18n.Message) error {
	if !removes {
		return nil
	}
	return ConfirmGuardedCall(ctx, op, ParentRemovalConfirmID, question,
		"the user declined taking the item out of its epic or parent; omit "+field+" to leave it where it is")
}

// ConfirmAssigneesCleared asks before an update removes every assignee of an
// item, and returns nil when the update may proceed. clears is whether the
// call empties the list, question renders the question around what it would
// remove, and count reads how many assignees the item has.
//
// It asks as issue.work_item_update does when assignee_ids is []: GitLab
// replaces the list wholesale, the response does not say who was assigned,
// and a model sends an empty list by accident as readily as on purpose. A
// call nobody is going to be asked about reads nothing. Otherwise the item is
// read, so clearing a list that is already empty proceeds unasked, and a read
// that fails asks rather than assumes the harmless case. A read the caller
// abandoned is answered with the context's error rather than with a question
// nobody is there to answer.
func ConfirmAssigneesCleared(ctx context.Context, op string, clears bool,
	question func(losses i18n.Message) i18n.Message, count func(context.Context) (int, error),
) error {
	if !clears || GuardedCallAnswered(ctx) {
		return nil
	}
	assigned, err := count(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err == nil && assigned == 0 {
		return nil
	}
	losses := i18n.New(i18n.ConfirmWorkItemAssigneesUnknown)
	if err == nil {
		losses = i18n.New(i18n.ConfirmWorkItemAssignees, i18n.N("count", int64(assigned)))
	}
	return ConfirmGuardedCall(ctx, op, AssigneesClearConfirmID, question(losses),
		"the user declined removing every assignee; omit assignee_ids, and assignee_id where the action has it, to leave the assignees as they are")
}
