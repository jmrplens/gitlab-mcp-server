// clear_guard.go guards the work item update paths that silently discard data:
// an explicit empty assignee or CRM contact list replaces the current one with
// nothing.

package workitems

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

	message := fmt.Sprintf(
		"Remove %s from work item %s#%d? This replaces the list and cannot be undone from the response.",
		strings.Join(losses, " and "), input.FullPath, input.IID,
	)

	flow, flowErr := elicitation.FlowFromRequest(req)
	if flowErr != nil || !flow.IsSupported() {
		return fmt.Errorf(
			"update_work_item: %s Re-send the same call with confirm=true once the user has approved it",
			message,
		)
	}

	confirmed, err := flow.Confirm(ctx, clearListConfirmID, message)
	switch {
	case errors.Is(err, elicitation.ErrInputPending):
		return flow.PendingError()
	case err != nil:
		return fmt.Errorf("update_work_item: confirmation failed: %w", err)
	case !confirmed:
		return errors.New("update_work_item: the user declined removing the current assignees or CRM contacts; omit assignee_ids/crm_contact_ids to leave them untouched")
	}
	return nil
}

// clearsAnyList reports whether the update submits an explicit empty list.
func clearsAnyList(input UpdateInput) bool {
	return (input.AssigneeIDs != nil && len(input.AssigneeIDs) == 0) ||
		(input.CRMContactIDs != nil && len(input.CRMContactIDs) == 0)
}

// pendingClearLosses describes what an explicit empty list would delete,
// returning nothing when the update clears no list or clears only empty ones.
func pendingClearLosses(ctx context.Context, client *gitlabclient.Client, input UpdateInput) []string {
	clearsAssignees := input.AssigneeIDs != nil && len(input.AssigneeIDs) == 0
	clearsContacts := input.CRMContactIDs != nil && len(input.CRMContactIDs) == 0
	if !clearsAssignees && !clearsContacts {
		return nil
	}

	var losses []string
	if clearsAssignees {
		current, _, err := client.GL().WorkItems.GetWorkItem(input.FullPath, input.IID, gl.WithContext(ctx))
		switch {
		case err != nil:
			// Fail closed: without the current list we cannot tell an
			// empty-to-empty no-op from a deletion, so ask rather than assume.
			losses = append(losses, "every assignee (the current list could not be read)")
		case len(current.Assignees) > 0:
			losses = append(losses, fmt.Sprintf("all %d assignees", len(current.Assignees)))
		}
	}
	if clearsContacts {
		losses = append(losses, "every CRM contact")
	}
	return losses
}
