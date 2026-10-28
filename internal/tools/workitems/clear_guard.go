// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// clear_guard.go guards the work item update paths that silently discard data:
// an explicit empty assignee or CRM contact list replaces the current one with
// nothing.

package workitems

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// clearListConfirmID identifies this confirmation inside a multi round-trip
// elicitation flow, keeping it distinct from the destructive-action prompt.
const clearListConfirmID = "work_item_clear_lists"

// confirmListClearing asks the caller to confirm before an update wipes the
// work item's assignees or CRM contacts.
//
// GitLab replaces these lists wholesale, so an explicit empty array deletes
// every entry. That is a legitimate operation — it is how the API removes all
// assignees — but a model can also send [] by accident, and the deletion is not
// recoverable from the response.
//
// Assignees are checked against the work item's current state, so clearing an
// already-empty list costs nothing and proceeds silently; updates that omit the
// field never read the work item at all. CRM contacts are not exposed on the
// read model, so an explicit empty list there always asks: the guard cannot
// tell a no-op from a deletion, and assuming the harmless case would be the one
// mistake it exists to prevent.
//
// Confirmation follows the same precedence as destructive actions: YOLO mode
// and an explicit confirm=true proceed immediately, otherwise the caller is
// elicited. Clients without elicitation get an actionable error telling them to
// resend with confirm=true, so the deletion is never silent.
//
// It returns nil when the update may proceed.
func confirmListClearing(ctx context.Context, client *gitlabclient.Client, input UpdateInput) error {
	if !clearsAnyList(input) {
		return nil
	}
	// Check the bypasses before reading the work item: a confirmed call does
	// not need to know what it is about to remove.
	req := toolutil.RequestFromContext(ctx)
	if toolutil.IsYOLOMode() || toolutil.ExplicitConfirmFromRequest(req) {
		return nil
	}
	losses := pendingClearLosses(ctx, client, input)
	if len(losses) == 0 {
		return nil
	}

	// The question has two readers: the person it is asked of, in the
	// request's language, and the model, which reads it in English as the
	// refusal's opening sentence when the client cannot ask.
	question := i18n.New(i18n.ConfirmWorkItemClear,
		i18n.L("losses", losses...), i18n.S("path", input.FullPath), i18n.N("iid", input.IID))

	flow, flowErr := elicitation.FlowFromRequest(req)
	if flowErr != nil || !flow.IsSupported() {
		return fmt.Errorf(
			"update_work_item: %s Re-send the same call with confirm=true once the user has approved it",
			question.English(),
		)
	}

	confirmed, err := flow.Confirm(ctx, clearListConfirmID, question.Text(ctx))
	// A chain of ifs rather than a switch: a switch case carries no statement
	// counter, so a mutation run reported each condition as not covered
	// whatever the tests did.
	if errors.Is(err, elicitation.ErrInputPending) {
		return flow.PendingError()
	}
	if err != nil {
		return fmt.Errorf("update_work_item: confirmation failed: %w", err)
	}
	if !confirmed {
		return errors.New("update_work_item: the user declined removing the current assignees or CRM contacts; omit assignee_ids/crm_contact_ids to leave them untouched")
	}
	return nil
}

// clearsList reports whether the caller submitted this list as an explicit
// empty array, which is how GitLab is told to remove every entry. A list the
// caller omitted arrives nil and leaves the current entries alone.
//
// It is one function because both readers below ask the same question.
// Spelled out twice, each copy was decided again by the other before anything
// a caller sees was settled, so inverting either one's && changed no answer.
func clearsList(ids []int64) bool {
	return ids != nil && len(ids) == 0
}

// clearsAnyList reports whether the update submits an explicit empty list.
func clearsAnyList(input UpdateInput) bool {
	return clearsList(input.AssigneeIDs) || clearsList(input.CRMContactIDs)
}

// pendingClearLosses describes what an explicit empty list would delete,
// returning nothing when the update clears no list or clears only empty ones.
// Each loss is a catalog message, rendered in whichever language the question
// that names it is.
func pendingClearLosses(ctx context.Context, client *gitlabclient.Client, input UpdateInput) []i18n.Message {
	clearsAssignees := clearsList(input.AssigneeIDs)
	clearsContacts := clearsList(input.CRMContactIDs)
	if !clearsAssignees && !clearsContacts {
		return nil
	}

	var losses []i18n.Message
	if clearsAssignees {
		current, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
		// An if chain rather than a tagless switch, for the reason the
		// confirmation above gives.
		if err != nil {
			// Fail closed: without the current list we cannot tell an
			// empty-to-empty no-op from a deletion, so ask rather than assume.
			losses = append(losses, i18n.New(i18n.ConfirmWorkItemAssigneesUnknown))
		} else if len(current.Assignees) > 0 {
			losses = append(losses, i18n.New(i18n.ConfirmWorkItemAssignees, i18n.N("count", int64(len(current.Assignees)))))
		}
	}
	if clearsContacts {
		losses = append(losses, i18n.New(i18n.ConfirmWorkItemCrmContacts))
	}
	return losses
}
