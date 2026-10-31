// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
//
// The creation itself can be refused for a branch that exists. GitLab checks
// both branches against a Redis cache of the project's branch names, and a
// read that listed the branches from Gitaly before the source branch was
// created can write its list back after the creation emptied the cache
// (upstream-bugs row 152). The cache then answers that the branch does not
// exist until a push job refreshes it, a few seconds later.

package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
// A creation refused because a branch "does not exist" is tried again once
// GitLab's branch name cache lists that branch ([awaitBranchesListed]), and
// failed with GitLab's refusal when the cache does not list it within the
// wait, which is what a branch that really is missing looks like. The refusal
// is a validation error, so the attempt it answers saved nothing.
//
// The merge request goes with the project, so nothing is registered.
func NewMergeRequest(e *harness.Env, project Project, source, target, title string) MergeRequest {
	e.T.Helper()

	mr, err := harness.Retry(e.Ctx, e.T, "create merge request", createRetries, retryBaseDelay, func(int) (MergeRequest, bool, string, error) {
		created, _, err := e.Client().GL().MergeRequests.CreateMergeRequest(project.ID, &gl.CreateMergeRequestOptions{
			Title:        new(title),
			SourceBranch: new(source),
			TargetBranch: new(target),
		}, gl.WithContext(e.Ctx))
		if err == nil {
			return mergeRequestOf(created), false, "", nil
		}
		if unlisted := unlistedBranches(err, source, target); len(unlisted) > 0 {
			return MergeRequest{}, awaitBranchesListed(e, project, unlisted), branchUnlistedReason, err
		}
		return MergeRequest{}, IsRetryable(err), describeRetry(err), err
	})
	if err != nil {
		e.T.Fatalf("creating a merge request %q -> %q in project %d: %v", source, target, project.ID, err)
	}

	mr.Status = WaitForMergeRequest(e, project, mr.IID)
	return mr
}

// branchUnlistedReason is how a retried refusal of an existing branch reads in
// the log.
const branchUnlistedReason = "branch missing from GitLab's branch name cache"

// branchMissingMessage is the validation error GitLab gives a merge request
// branch its branch name cache does not list.
const branchMissingMessage = "does not exist"

// unlistedBranches returns the branches a merge request creation was refused
// for as not existing, the source before the target, and none for any other
// failure.
//
// GitLab answers that refusal 400 with the validation errors keyed by
// attribute, {"message":{"source_branch":["does not exist"]}}, so the body is
// read rather than the text client-go renders from it.
func unlistedBranches(err error, source, target string) []string {
	refusal, isRefusal := errors.AsType[*gl.ErrorResponse](err)
	if !isRefusal || statusOf(err) != http.StatusBadRequest {
		return nil
	}
	var body struct {
		Message map[string][]string `json:"message"`
	}
	if json.Unmarshal(refusal.Body, &body) != nil {
		return nil
	}
	var unlisted []string
	if slices.Contains(body.Message["source_branch"], branchMissingMessage) {
		unlisted = append(unlisted, source)
	}
	if slices.Contains(body.Message["target_branch"], branchMissingMessage) {
		unlisted = append(unlisted, target)
	}
	return unlisted
}

// The wait for GitLab's branch name cache to list a branch again, larger on a
// licensed instance like every wait here, and how often it is asked.
// Variables so the package's own tests can bound a wait that never ends.
var (
	branchListedWait           = 30 * time.Second
	enterpriseBranchListedWait = 60 * time.Second
	branchListedPollInterval   = 250 * time.Millisecond
)

// branchListedRequestGrace is how long a HEAD still in flight when the wait
// runs out may take to be answered, beyond the wait itself.
const branchListedRequestGrace = 5 * time.Second

// awaitBranchesListed holds until GitLab's branch name cache lists every
// branch named, and reports whether it did within the wait. It never fails the
// test itself: a branch the cache does not list in time is logged with what
// was last seen, and the caller fails with GitLab's own refusal.
//
// Sidekiq is drained first, since what puts a branch back into the cache is
// the PostReceive job of a push to the project.
func awaitBranchesListed(e *harness.Env, project Project, branches []string) bool {
	e.T.Helper()

	DrainSidekiq(e.Ctx, e.Client())
	wait := budget(e, branchListedWait, enterpriseBranchListedWait)
	for _, branch := range branches {
		if err := waitForBranchListed(e.Ctx, e.Client(), project.ID, branch, wait); err != nil {
			e.T.Logf("branch %q of project %d: %v", branch, project.ID, err)
			return false
		}
	}
	return true
}

// waitForBranchListed polls until GitLab answers HEAD on the branch with a
// success, or the wait is spent. A failure to get any answer, a 404 and the
// answers a converging GitLab gives keep it polling; any other answer ends it.
//
// It asks with HEAD because GitLab answers HEAD from the branch name cache
// (Repository#branch_exists?), the one the merge request validation reads,
// while GET reads the branch from Gitaly, which held it all along and would
// end the wait at once.
//
// The wait ends on Poll's own timer, and its requests are bounded
// [branchListedRequestGrace] later. Were they bounded by the wait itself, the
// HEAD in flight when it ran out would be cut short, and the timeout would
// name a request GitLab seemed never to answer rather than the 404 it kept
// answering.
func waitForBranchListed(ctx context.Context, client *gitlabclient.Client, projectID int64, branch string, wait time.Duration) error {
	requestCtx, cancel := context.WithTimeout(ctx, wait+branchListedRequestGrace)
	defer cancel()

	path := fmt.Sprintf("projects/%d/repository/branches/%s", projectID, gl.PathEscape(branch))
	return harness.Poll(requestCtx, branchListedPollInterval, wait, func() (bool, string, error) {
		answer, err := probeREST(requestCtx, client, http.MethodHead, path, nil)
		if err != nil {
			return harness.WaitThrough(fmt.Sprintf("HEAD of branch %q in project %d", branch, projectID), err)
		}
		state := fmt.Sprintf("HEAD of branch %q in project %d: HTTP %d", branch, projectID, answer.Status)
		if answer.Status >= http.StatusOK && answer.Status < http.StatusMultipleChoices {
			return true, state, nil
		}
		if !convergingStatus(answer.Status) {
			return false, state, fmt.Errorf("asking whether project %d lists branch %q: HTTP %d", projectID, branch, answer.Status)
		}
		return false, state, nil
	})
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

// AllowAuthorApproval lets the authors of this project's merge requests
// approve their own, and reports whether an author may now approve.
//
// It is a precondition rather than a subject, and it exists because the run
// holds one credential: every request a test opens is authored by the same
// user that then approves it. A licensed instance ships "Prevent approval by
// author" on, so GitLab answers that approval 401 and the scenario cannot
// reach the action it is about. An unlicensed instance serves no approval
// configuration at all, which is the 404 read below: there the precondition
// is already met, since nothing there prevents an author from approving.
//
// The project is disposable, so nothing is restored.
func AllowAuthorApproval(e *harness.Env, project Project) bool {
	e.T.Helper()

	approvals, _, err := e.Client().GL().Projects.ChangeApprovalConfiguration(project.ID,
		&gl.ChangeApprovalConfigurationOptions{MergeRequestsAuthorApproval: new(true)}, gl.WithContext(e.Ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			e.T.Logf("project %d serves no approval configuration, so this instance is unlicensed and "+
				"an author may approve already", project.ID)
			return true
		}
		e.T.Fatalf("allowing author approval on project %d: %v", project.ID, err)
	}
	return approvals.MergeRequestsAuthorApproval
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
