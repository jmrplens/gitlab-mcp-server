//go:build e2e

// mergerequests_extras_test.go covers what else hangs off a merge request
// through the server: the context commits it pins, the to-do it raises for
// the caller, the issues its description closes, and the pipeline it runs
// or that its configuration refuses. Everything here is shared across the
// surfaces, because none of it consumes the request: a pinned commit is
// unpinned again, a to-do is marked done, a pipeline may be created twice.

package common

import (
	"fmt"
	"slices"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The related issues wait: the closing reference in a description is read
// by a background job, and the budget is the one the old suite gave it.
const (
	relatedIssuesInterval = 2 * time.Second
	relatedIssuesWait     = 120 * time.Second
)

// mrPipelineCIYAML runs one job for merge request pipelines only, so that
// creating a pipeline for the request produces one and pushing to the
// branch does not. The job may stay pending without a runner; the pipeline
// is what the scenario reads.
const mrPipelineCIYAML = `mr-check:
  script:
    - echo ok
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
`

// mrRefusedCIYAML admits every pipeline but a merge request's, so that
// creating one for the request is refused by the workflow rules rather than
// by a missing configuration, which an instance with Auto DevOps on would
// fill in.
const mrRefusedCIYAML = `workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      when: never
    - when: always

noop:
  script:
    - echo ok
`

// mrExtrasFixture is a request with an issue for its description to close
// and a commit the request's diff does not carry.
type mrExtrasFixture struct {
	// mr is the request the context commits, the to-do and the related
	// issues are read on.
	mr mergeRequestFixture
	// issue is the one the request's description closes.
	issue fixture.Issue
	// context is a commit on a branch unrelated to mr's, which is what the
	// context commits API pins: a commit the repository has and the diff
	// does not.
	context fixture.Commit
}

// buildMRExtrasFixture creates the project, the issue, the request and the
// unrelated commit.
func buildMRExtrasFixture(e *harness.Env) mrExtrasFixture {
	e.T.Helper()

	mr := newMergeRequestFixture(e, "mrextras")
	issue := fixture.NewIssue(e, mr.project, "closed by the merge request")
	contextBranch := fixture.NewBranch(e, mr.project, e.Name("context"))
	context := fixture.CommitFile(e, mr.project, contextBranch.Name, "context.txt", "context payload\n", "context commit for the merge request")
	return mrExtrasFixture{mr: mr, issue: issue, context: context}
}

// contextCommitIDs lists the SHAs of a context commits listing.
func contextCommitIDs(out mrcontextcommits.ListOutput) []string {
	ids := make([]string, 0, len(out.Commits))
	for _, commit := range out.Commits {
		ids = append(ids, commit.ID)
	}
	return ids
}

// TestMergeRequestExtras_ContextCommitsTodoAndRelatedIssues drives, on
// every surface and one shared request, the three things that hang off it
// without consuming it: the unrelated commit is pinned as context, found
// in the listing and unpinned; a to-do is raised for the caller, a second
// one is refused as already pending and the first is marked done through
// client-go so the next surface starts without one; and the closing
// reference is written into the description, after which the issue it
// names is waited for among the related issues.
//
// Replaces: TestIndividual_MRExtras
func TestMergeRequestExtras_ContextCommitsTodoAndRelatedIssues(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildMRExtrasFixture, func(e *harness.Env, surface harness.Surface, f mrExtrasFixture) {
		s := e.On(surface)
		params := f.mr.params()

		commits := withParams(params, map[string]any{"commits": []string{f.context.SHA}})
		pinned := harness.Do[mrcontextcommits.ListOutput](s, actionMergeRequestContextCommitsCreate, commits)
		if !slices.Contains(contextCommitIDs(pinned), f.context.SHA) {
			e.T.Fatalf("context_commits_create answered %v, want the pinned commit %s", contextCommitIDs(pinned), f.context.ShortID)
		}
		listed := harness.Do[mrcontextcommits.ListOutput](s, actionMergeRequestContextCommitsList, params)
		if !slices.Contains(contextCommitIDs(listed), f.context.SHA) {
			e.T.Errorf("the request's context commits do not hold %s: %v", f.context.ShortID, contextCommitIDs(listed))
		}
		harness.DoVoid(s, actionMergeRequestContextCommitsDelete, commits)
		remaining := harness.Do[mrcontextcommits.ListOutput](s, actionMergeRequestContextCommitsList, params)
		if len(remaining.Commits) != 0 {
			e.T.Errorf("the request still lists %d context commit(s) after the unpin: %v", len(remaining.Commits), contextCommitIDs(remaining))
		}

		todo := harness.Do[mergerequests.CreateTodoOutput](s, actionMergeRequestCreateTodo, params)
		if todo.ID == 0 || todo.State != "pending" {
			e.T.Fatalf("create_todo answered %+v, want a pending to-do with an ID", todo)
		}
		defer markTodoDone(e, todo.ID)
		refusal := harness.ExpectToolError(s, actionMergeRequestCreateTodo, params, "already exists")
		assertMentions(e, "the refusal of a second to-do", refusal, "gitlab_todo_list")

		description := fmt.Sprintf("Closes #%d", f.issue.IID)
		updated := harness.Do[mergerequests.Output](s, actionMergeRequestUpdate, withParams(params, map[string]any{"description": description}))
		if updated.IID != f.mr.mr.IID || updated.Description != description {
			e.T.Fatalf("merge_request update answered %+v, want request !%d describing %q", updated, f.mr.mr.IID, description)
		}
		related := harness.Eventually(s, actionMergeRequestRelatedIssues, params, relatedIssuesInterval, relatedIssuesWait,
			func(out mergerequests.RelatedIssuesOutput) bool { return relatedIssueListed(out, f.issue.IID) })
		e.T.Logf("issue #%d is among the %d related issue(s)", f.issue.IID, len(related.Issues))
	})
}

// markTodoDone marks a to-do done through client-go, failing the test when
// GitLab refuses: a to-do left pending would make the next surface's create
// answer the refusal where it expects the to-do.
func markTodoDone(e *harness.Env, todoID int64) {
	e.T.Helper()
	if _, err := e.Client().GL().Todos.MarkTodoAsDone(todoID, gl.WithContext(e.Ctx)); err != nil {
		e.T.Errorf("marking to-do %d done: %v", todoID, err)
	}
}

// pipelineListed reports whether a request's pipelines hold the one with
// the given ID.
func pipelineListed(out mergerequests.PipelinesOutput, id int64) bool {
	for _, pipeline := range out.Pipelines {
		if pipeline.ID == id {
			return true
		}
	}
	return false
}

// relatedIssueListed reports whether a related issues listing holds the
// issue.
func relatedIssueListed(out mergerequests.RelatedIssuesOutput, iid int64) bool {
	for _, issue := range out.Issues {
		if issue.IID == iid {
			return true
		}
	}
	return false
}

// mrPipelineFixture is two requests in one project: one whose branch runs a
// job on merge request pipelines, one whose branch's workflow rules admit
// no merge request pipeline at all.
type mrPipelineFixture struct {
	admitted mergeRequestFixture
	refused  mergeRequestFixture
}

// buildMRPipelineFixture creates the project and the two requests, each
// from a branch carrying its own CI configuration. The file lands on the
// branch before the request exists, so the request is born with a visible
// configuration rather than racing GitLab's refresh of its merge ref.
func buildMRPipelineFixture(e *harness.Env) mrPipelineFixture {
	e.T.Helper()

	project := fixture.NewProject(e, fixture.WithNamePrefix("mrpipeline"))
	return mrPipelineFixture{
		admitted: newMergeRequestWithCI(e, project, "admitted", mrPipelineCIYAML),
		refused:  newMergeRequestWithCI(e, project, "refused", mrRefusedCIYAML),
	}
}

// newMergeRequestWithCI opens a request from a branch carrying the given CI
// configuration.
func newMergeRequestWithCI(e *harness.Env, project fixture.Project, prefix, ciYAML string) mergeRequestFixture {
	e.T.Helper()

	branch := fixture.NewBranch(e, project, e.Name(prefix))
	commit := fixture.CommitFile(e, project, branch.Name, fixture.CIFilePath, ciYAML, "ci: the "+prefix+" configuration")
	mr := fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, e.Name(prefix))
	return mergeRequestFixture{project: project, branch: branch, commit: commit, mr: mr}
}

// TestMergeRequestPipeline_Create_RunsOrIsRefusedByTheRules creates a
// pipeline on every surface for the request whose branch runs a job on
// merge request pipelines, reads the pipeline off the answer and finds it
// again in the request's own pipelines listing, then asks for one on the
// request whose workflow rules admit none and reads the refusal, which
// names the configuration file a caller should look at and carries
// GitLab's own reason.
//
// Replaces: TestIndividual_MRExtras
func TestMergeRequestPipeline_Create_RunsOrIsRefusedByTheRules(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildMRPipelineFixture, func(e *harness.Env, surface harness.Surface, f mrPipelineFixture) {
		s := e.On(surface)

		// GitLab answers 405 while the request's diff is still being written,
		// which the fixture's readiness wait covers on a quiet instance and
		// a loaded one may outlast; the old suite met that same answer and
		// retried for two minutes.
		created := harness.Eventually(s, actionMergeRequestCreatePipeline, f.admitted.params(), mergeRequestReadInterval, mergeRequestReadWait,
			func(out pipelines.Output) bool { return out.ID != 0 })
		if created.Source != "merge_request_event" {
			e.T.Errorf("create_pipeline answered %+v, want a merge request pipeline", created)
		}
		// The listing is asked here rather than beside the approval
		// scenario, which runs on a branch whose pipeline count depends on
		// whether the instance has Auto DevOps on: this request has a
		// pipeline of its own that was just created, so the listing can be
		// held to it.
		listed := harness.Eventually(s, actionMergeRequestPipelines, f.admitted.params(), mergeRequestReadInterval, mergeRequestReadWait,
			func(out mergerequests.PipelinesOutput) bool { return pipelineListed(out, created.ID) })
		e.T.Logf("the request lists %d pipeline(s), the created one among them", len(listed.Pipelines))

		// Two halves of one message: GitLab's own reason, which names the
		// keyword whose rules filtered the pipeline out, and this server's
		// hint for a 400, which names the file a caller should look at.
		refusal := harness.ExpectToolError(s, actionMergeRequestCreatePipeline, f.refused.params(), ".gitlab-ci.yml")
		assertMentions(e, "the refusal of a pipeline no rule admits", refusal, "workflow:rules", "did not run")
	})
}
