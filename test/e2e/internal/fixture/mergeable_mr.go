//go:build e2e

// mergeable_mr.go builds a merge request GitLab will actually merge.
//
// An ordinary merge request fixture is not enough for a merge case: a project
// may require approvals, and GitLab refuses the merge with a message about
// approvals rather than about anything the case did. The approval count lives
// in two places since GitLab 16.0, the merge request's own configuration and
// its rules, and both are cleared here.
//
// Every approval call is tolerated when it is refused with a 400, a 403 or a
// 404, because the approvals API is a licensed surface: on Free it answers
// one of those three and there is nothing to clear, which is the outcome the
// caller wanted anyway.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// NewMergeableMergeRequest creates a merge request from source into target and
// clears every approval requirement standing between it and a merge.
//
// The merge request goes with the project, so nothing is registered.
func NewMergeableMergeRequest(e *harness.Env, project Project, source, target, title string) MergeRequest {
	e.T.Helper()

	mr := NewMergeRequest(e, project, source, target, title)
	if err := clearMergeRequestApprovals(e.Ctx, e.Client(), project.ID, mr.IID); err != nil {
		e.T.Fatalf("clearing the approvals of merge request !%d in project %d: %v", mr.IID, project.ID, err)
	}
	// The approval edits are themselves a change GitLab recomputes the merge
	// status after, so the readiness wait is repeated rather than trusted
	// from before them.
	mr.Status = WaitForMergeRequest(e, project, mr.IID)
	return mr
}

// clearMergeRequestApprovals sets the merge request's own approval count to
// zero and then every rule's, which are two different records of the same
// requirement.
func clearMergeRequestApprovals(ctx context.Context, client *gitlabclient.Client, projectID, iid int64) error {
	none := int64(0)
	//nolint:staticcheck // client-go marks it deprecated as of GitLab 16.0, and the pinned release still serves POST .../merge_requests/:iid/approvals; it is the one request that clears the merge request's own count, and the rules below are the other half.
	_, _, err := client.GL().MergeRequestApprovals.ChangeApprovalConfiguration(projectID, iid,
		&gl.ChangeMergeRequestApprovalConfigurationOptions{ApprovalsRequired: &none}, gl.WithContext(ctx))
	if err != nil && !approvalsUnavailable(err) {
		return fmt.Errorf("setting the approval count of !%d to zero: %w", iid, err)
	}

	rules, _, err := client.GL().MergeRequestApprovals.GetApprovalRules(projectID, iid, gl.WithContext(ctx))
	if err != nil {
		if approvalsUnavailable(err) {
			return nil
		}
		return fmt.Errorf("reading the approval rules of !%d: %w", iid, err)
	}
	for _, rule := range rules {
		if rule == nil || rule.ID == 0 || rule.ApprovalsRequired == 0 {
			continue
		}
		_, _, updateErr := client.GL().MergeRequestApprovals.UpdateApprovalRule(projectID, iid, rule.ID,
			&gl.UpdateMergeRequestApprovalRuleOptions{ApprovalsRequired: &none}, gl.WithContext(ctx))
		if updateErr != nil && !approvalsUnavailable(updateErr) {
			return fmt.Errorf("clearing approval rule %d of !%d: %w", rule.ID, iid, updateErr)
		}
	}
	return nil
}

// approvalsUnavailable reports whether GitLab refused an approvals call in one
// of the three ways that mean the surface is not there to be cleared.
func approvalsUnavailable(err error) bool {
	return IsStatus(err, http.StatusBadRequest) ||
		IsStatus(err, http.StatusForbidden) ||
		IsStatus(err, http.StatusNotFound)
}
