//go:build e2e && !enterprise

// mrextras_ce_test.go tests merge request auxiliary MCP tools against a live
// GitLab instance: context commits (create/delete), to-do creation, related
// issues, blocking dependencies listing, and MR pipeline creation.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
)

// mrExtrasCIYAML is a minimal CI configuration whose single job runs only for
// merge-request pipelines. This makes gitlab_mr_create_pipeline produce a
// detached MR pipeline; the job may stay pending without a runner, which is
// fine because only the pipeline entity itself is asserted.
const mrExtrasCIYAML = `mr-check:
  script:
    - echo ok
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
`

// TestIndividual_MRExtras exercises the merge request auxiliary tools using
// individual MCP tools: context commits create/delete, to-do creation,
// related issues, dependency listing, and MR pipeline creation.
//
// The test builds an MR fixture whose description closes a project issue,
// pins a commit from an unrelated branch as an MR context commit and removes
// it again, creates a to-do for the caller, polls the related-issues endpoint
// until the closing reference is surfaced, asserts a fresh MR has no blocking
// dependencies, and finally commits a merge-request-only CI configuration to
// the source branch and triggers a detached MR pipeline.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_MRExtras(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	// 480s base: the shared MR readiness wait plus the pipeline retry loop
	// (MR "checking" state can last minutes under full-suite load) both fit.
	ctx, cancel := e2eTimeoutContext(480*time.Second, 600*time.Second)
	defer cancel()

	proj, branch := setupMRProject(ctx, t, sess.individual)
	issue := createIssue(ctx, t, sess.individual, proj, "MR extras closing target")

	mr, err := callToolOn[mergerequests.Output](ctx, sess.individual, "gitlab_mr_create", mergerequests.CreateInput{
		ProjectID:    proj.pidOf(),
		SourceBranch: branch,
		TargetBranch: defaultBranch,
		Title:        "E2E MR extras",
		Description:  fmt.Sprintf("Closes #%d", issue.IID),
	})
	requireNoError(t, err, "create MR with closing reference")
	waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)

	// The context-commits API pins commits that exist in the repository but
	// are not part of the MR diff, so the fixture commit lives on a branch
	// unrelated to the MR's source branch.
	const ctxBranch = "feature/mr-extras-context"
	createBranch(ctx, t, sess.individual, proj, ctxBranch)
	ctxCommit := commitFile(ctx, t, sess.individual, proj, ctxBranch, "context.txt", "context payload", "context commit for MR extras")

	t.Run("ContextCommitsCreate", func(t *testing.T) {
		out, cErr := callToolOn[mrcontextcommits.ListOutput](ctx, sess.individual, "gitlab_create_mr_context_commits", mrcontextcommits.CreateInput{
			ProjectID:    proj.pidOf(),
			MergeRequest: mr.IID,
			Commits:      []string{ctxCommit.SHA},
		})
		requireNoError(t, cErr, "create MR context commits")
		found := false
		for _, c := range out.Commits {
			if c.ID == ctxCommit.SHA {
				found = true
				break
			}
		}
		requireTruef(t, found, "expected context commit %s in response, got %d commits", ctxCommit.SHA, len(out.Commits))
		t.Logf("Pinned context commit %s to MR !%d", ctxCommit.ShortID, mr.IID)
	})

	t.Run("ContextCommitsDelete", func(t *testing.T) {
		dErr := callToolVoidOn(ctx, sess.individual, "gitlab_delete_mr_context_commits", mrcontextcommits.DeleteInput{
			ProjectID:    proj.pidOf(),
			MergeRequest: mr.IID,
			Commits:      []string{ctxCommit.SHA},
		})
		requireNoError(t, dErr, "delete MR context commits")

		out, lErr := callToolOn[mrcontextcommits.ListOutput](ctx, sess.individual, "gitlab_list_mr_context_commits", mrcontextcommits.ListInput{
			ProjectID:    proj.pidOf(),
			MergeRequest: mr.IID,
		})
		requireNoError(t, lErr, "list MR context commits after delete")
		requireTruef(t, len(out.Commits) == 0, "expected no context commits after delete, got %d", len(out.Commits))
		t.Log("Removed context commit from MR")
	})

	t.Run("CreateTodo", func(t *testing.T) {
		out, tErr := callToolOn[mergerequests.CreateTodoOutput](ctx, sess.individual, "gitlab_mr_create_todo", mergerequests.CreateTodoInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		requireNoError(t, tErr, "create MR todo")
		requireTruef(t, out.ID > 0, "expected non-zero todo ID")
		t.Logf("Created todo %d (state=%s)", out.ID, out.State)
	})

	t.Run("RelatedIssues", func(t *testing.T) {
		// The closing reference in the MR description is extracted by an
		// asynchronous sidekiq job, so poll until the issue surfaces.
		maxWait := e2eTimeout(45*time.Second, 120*time.Second)
		pollCtx, cancelPoll := context.WithTimeout(ctx, maxWait)
		defer cancelPoll()

		found := false
		pErr := Poll(pollCtx, time.Second, maxWait, func() (bool, string, error) {
			out, callErr := callToolOn[mergerequests.RelatedIssuesOutput](pollCtx, sess.individual, "gitlab_mr_related_issues", mergerequests.RelatedIssuesInput{
				ProjectID: proj.pidOf(),
				MRIID:     mr.IID,
			})
			if callErr != nil {
				return false, fmt.Sprintf("related issues call: %v", callErr), nil
			}
			for _, related := range out.Issues {
				if related.IID == issue.IID {
					found = true
					return true, "closing issue listed as related", nil
				}
			}
			return false, fmt.Sprintf("%d related issues, target #%d not yet listed", len(out.Issues), issue.IID), nil
		})
		requireNoError(t, pErr, "poll MR related issues")
		requireTruef(t, found, "expected issue #%d among MR related issues", issue.IID)
		t.Logf("Issue #%d listed as related to MR !%d", issue.IID, mr.IID)
	})

	t.Run("DependenciesList", func(t *testing.T) {
		out, dErr := callToolOn[mergerequests.DependenciesOutput](ctx, sess.individual, "gitlab_mr_dependencies_list", mergerequests.GetDependenciesInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		// MR dependencies are a Premium feature: on a Free-tier catalog the
		// gitlab_mr_dependencies_list tool is not registered at all (unknown
		// tool), and an ungated server against live CE gets 404/403 from the
		// API. An empty list on a fresh MR is the expected result when
		// available.
		if dErr != nil && (isHTTPStatus(dErr, 403) || isHTTPStatus(dErr, 404) ||
			strings.Contains(dErr.Error(), "Premium") || strings.Contains(dErr.Error(), "unknown tool")) {
			t.Skipf("MR dependencies not available on this tier: %v", dErr)
		}
		requireNoError(t, dErr, "list MR dependencies")
		requireTruef(t, len(out.Dependencies) == 0, "expected no dependencies on a fresh MR, got %d", len(out.Dependencies))
		t.Log("Fresh MR has no blocking dependencies")
	})

	t.Run("CreatePipeline", func(t *testing.T) {
		// Committing the CI config to an existing MR's branch races GitLab's
		// asynchronous merge-request ref refresh, which under full-suite load
		// stays stale for minutes. Give the pipeline its own MR instead: the
		// CI file lands on the branch before the MR exists, so the MR is
		// born with a visible configuration.
		pipeBranch := createBranch(ctx, t, sess.individual, proj, "mrx-pipeline")
		commitFileCreateOrUpdate(ctx, t, sess.individual, proj, pipeBranch.Name, ".gitlab-ci.yml", mrExtrasCIYAML, "ci: add MR pipeline configuration")
		pipeMR := createMR(ctx, t, sess.individual, proj, pipeBranch.Name, defaultBranch, "MR extras pipeline fixture")

		// Right after creation the MR sits in detailed_merge_status
		// "checking" for minutes under load; the pipelines endpoint answers
		// 405 until the MR settles and 400 while the CI config propagates —
		// both are transient states, so poll patiently over both.
		out, pErr := retryWithBackoffInterval(ctx, t, "create MR pipeline", 20, 8*time.Second, func(int) (pipelines.Output, bool, string, error) {
			out, callErr := callToolOn[pipelines.Output](ctx, sess.individual, "gitlab_mr_create_pipeline", mergerequests.CreatePipelineInput{
				ProjectID: proj.pidOf(),
				MRIID:     pipeMR.IID,
			})
			if callErr == nil {
				return out, false, "", nil
			}
			retryable := isHTTPStatus(callErr, 400) || isHTTPStatus(callErr, 405) || isRetryableError(callErr)
			return out, retryable, "merge request not yet ready for pipeline creation", callErr
		})
		requireNoError(t, pErr, "create MR pipeline")
		requireTruef(t, out.ID > 0, "expected non-zero pipeline ID")
		t.Logf("Created MR pipeline %d (status=%s, source=%s)", out.ID, out.Status, out.Source)
	})
}
