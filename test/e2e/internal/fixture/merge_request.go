//go:build e2e

// merge_request.go builds a merge request and waits until GitLab has
// finished thinking about it.
//
// A merge request exists the moment it is created and is useless for a while
// after: GitLab computes its diff and its merge ref in the background, and
// while detailed_merge_status reads "preparing", "checking" or "unchecked"
// the commits, diff versions and mergeability endpoints answer with nothing.
// Every test that reads any of those would be flaky on a slow Docker GitLab
// without the wait below, and none of them should have to know why.

package fixture

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// MergeRequest is a merge request a builder created.
type MergeRequest struct {
	// IID is the project-scoped identifier every merge request action takes.
	IID int64
	// ID is the instance-wide identifier.
	ID int64
	// SourceBranch and TargetBranch are the two sides of it.
	SourceBranch string
	TargetBranch string
	// Title is what it was created with.
	Title string
	// Status is the detailed_merge_status it settled on: "mergeable" for a
	// fixture whose branches differ and do not conflict.
	Status string
}

// NewMergeRequest creates a merge request from source into target and waits
// until GitLab has finished preparing it. The source branch needs at least
// one commit the target does not have, or GitLab creates nothing to merge.
//
// The merge request goes with the project, so nothing is registered.
func NewMergeRequest(e *harness.Env, project Project, source, target, title string) MergeRequest {
	e.T.Helper()

	mr, err := retryTransient(e, "create merge request", createRetries, func() (MergeRequest, error) {
		created, _, err := e.Client().GL().MergeRequests.CreateMergeRequest(project.ID, &gl.CreateMergeRequestOptions{
			Title:        new(title),
			SourceBranch: new(source),
			TargetBranch: new(target),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return MergeRequest{}, err
		}
		return mergeRequestOf(created), nil
	})
	if err != nil {
		e.T.Fatalf("creating a merge request %q -> %q in project %d: %v", source, target, project.ID, err)
	}

	mr.Status = WaitForMergeRequest(e, project, mr.IID)
	return mr
}

// EnableMergeTrains turns merged results pipelines and merge trains on for
// a project and reports whether GitLab kept both.
//
// Both switches are needed, because setting merge_trains_enabled alone is
// silently dropped. The report matters on a self-managed instance: GitLab
// does not persist the flags on a project in a personal namespace, since the
// namespace lacks the licensed feature, and answers the edit with 200 all
// the same. A test that needs a train reads the answer and says what it can
// assert when the flags did not stick.
func EnableMergeTrains(e *harness.Env, project Project) bool {
	e.T.Helper()

	_, _, err := e.Client().GL().Projects.EditProject(project.ID, &gl.EditProjectOptions{
		MergePipelinesEnabled: new(true),
		MergeTrainsEnabled:    new(true),
	}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("enabling merge trains on project %d: %v", project.ID, err)
	}
	current, _, err := e.Client().GL().Projects.GetProject(project.ID, nil, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading project %d after enabling merge trains: %v", project.ID, err)
	}
	return current.MergePipelinesEnabled && current.MergeTrainsEnabled
}

// mergeRequestOf reads what a test needs out of what GitLab returned.
func mergeRequestOf(mr *gl.MergeRequest) MergeRequest {
	return MergeRequest{
		IID:          mr.IID,
		ID:           mr.ID,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		Title:        mr.Title,
		Status:       mr.DetailedMergeStatus,
	}
}

// The waits a merge request may take to leave its transitional states.
const (
	mergeRequestWait           = 120 * time.Second
	enterpriseMergeRequestWait = 300 * time.Second
	mergeRequestPollInterval   = 500 * time.Millisecond
)

// WaitForMergeRequest drains Sidekiq and polls the merge request until its
// detailed_merge_status leaves the transitional values, returning the status
// it settled on.
//
// It is best effort and never fails the test on its own: a merge request that
// is still "checking" when the budget runs out is logged and handed back, so
// the test's own assertion produces the message that says what it needed.
func WaitForMergeRequest(e *harness.Env, project Project, iid int64) string {
	e.T.Helper()

	DrainSidekiq(e.Ctx, e.Client())
	status, err := waitForMergeRequestReady(e.Ctx, e.Client(), project.ID, iid, budget(e, mergeRequestWait, enterpriseMergeRequestWait))
	if err != nil {
		e.T.Logf("merge request !%d in project %d: readiness wait ended: %v", iid, project.ID, err)
	}
	return status
}

// waitForMergeRequestReady polls until the merge request leaves the
// transitional states or the budget is spent, returning the last status seen.
func waitForMergeRequestReady(ctx context.Context, client *gitlabclient.Client, projectID, iid int64, wait time.Duration) (string, error) {
	pollCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	status := ""
	err := harness.Poll(pollCtx, mergeRequestPollInterval, wait, func() (bool, string, error) {
		mr, resp, err := client.GL().MergeRequests.GetMergeRequest(projectID, iid, nil, gl.WithContext(pollCtx))
		if err == nil {
			status = mr.DetailedMergeStatus
			return !transitionalMergeStatus(status), "detailed_merge_status=" + status, nil
		}
		if pollCtx.Err() != nil {
			return false, "", pollCtx.Err()
		}
		state := fmt.Sprintf("merge request !%d in project %d: %v", iid, projectID, err)
		if resp != nil {
			state = fmt.Sprintf("merge request !%d in project %d: HTTP %d", iid, projectID, resp.StatusCode)
		}
		if resp == nil && !IsTransientNetwork(err) {
			return false, state, fmt.Errorf("reading merge request !%d of project %d: %w", iid, projectID, err)
		}
		if resp != nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < http.StatusInternalServerError {
			return false, state, fmt.Errorf("reading merge request !%d of project %d: %w", iid, projectID, err)
		}
		return false, state, nil
	})
	return status, err
}

// transitionalMergeStatus reports whether a detailed_merge_status is one
// GitLab reports while it is still computing the diff and the merge ref. The
// empty string is one of them: older releases answered nothing at all until
// the check had run.
func transitionalMergeStatus(status string) bool {
	switch status {
	case "preparing", "checking", "unchecked", "":
		return true
	default:
		return false
	}
}
