//go:build e2e

// pipelines_latest_test.go covers what pipeline.latest answers when the
// commit at the head of the ref has no pipeline on it. GitLab's route answers
// 403 then, the same status it gives a caller who may not read pipelines, and
// the action falls back to the newest pipeline on the same ref and says so.
// The fixture reaches that state the way a project that runs merge request
// pipelines and no push pipelines does: a head commit whose push workflow:rules
// skips, and a branch with an open merge request, whose head has the merge
// request's pipeline alone.

package common

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// noPushPipelinesCIYAML runs a merge request's pipelines, on the merge
// request's own ref, and creates no pipeline for a push, so every branch
// pipeline is one the fixture creates through the API, in the order it creates
// them (doc/ci/yaml/workflow.md documents both rules). Push pipelines are not
// left to run because GitLab creates them in background jobs, which finish when
// they finish: a run on GitLab 19.4.1 saw the push pipeline of the commit the
// feature branch was created from land after the fixture's own pipeline of the
// branch's next commit, and become the newest pipeline on the branch. Its one
// job is manual under rules, which a merge request pipeline needs (a job with
// neither rules nor only runs on branches and tags alone), so a pipeline it
// creates runs nothing and never holds the runner other tests wait for.
const noPushPipelinesCIYAML = `workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_PIPELINE_SOURCE == "push"
      when: never
    - when: always

held:
  stage: test
  rules:
    - when: manual
  script:
    - echo "e2e held job"
  tags: []
`

// headWithoutPipeline is the fixture the scenario reads: a project whose
// default branch carries an earlier commit with a pipeline and a head commit
// whose push the workflow skipped, and a feature branch whose earlier commit
// has a branch pipeline and whose head, pushed with a merge request open, has
// the merge request's pipeline alone. The feature branch's pipelines, and the
// merge request's, are all newer than the default branch's.
type headWithoutPipeline struct {
	project        fixture.Project
	earlier        fixture.Pipeline
	head           fixture.Commit
	feature        string
	featureEarlier fixture.Pipeline
	featureHead    fixture.Commit
}

// TestPipeline_Latest_HeadCommitWithoutPipeline_ShowsTheEarlierOne builds the
// fixture and asks pipeline.latest on every surface. On the default branch,
// without the ref and with it, the answer is the earlier commit's pipeline
// there, although newer pipelines run on the feature branch and on its merge
// request's ref. On the feature branch it is the branch pipeline of its
// earlier commit, although the merge request's pipeline of the head is the
// first row GitLab's list gives for that branch.
func TestPipeline_Latest_HeadCommitWithoutPipeline_ShowsTheEarlierOne(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, newHeadWithoutPipeline, func(e *harness.Env, surface harness.Surface, f headWithoutPipeline) {
		s := e.On(surface)
		byProject := map[string]any{"project_id": f.project.IDParam()}

		onDefault := latestFallback{ref: f.project.DefaultBranch, earlier: f.earlier.SHA, head: f.head.SHA}
		assertLatestFallsBack(e, s, onDefault, "without a ref", byProject)
		assertLatestFallsBack(e, s, onDefault, "with the default branch named", withParams(byProject, map[string]any{"ref": f.project.DefaultBranch}))
		onFeature := latestFallback{ref: f.feature, earlier: f.featureEarlier.SHA, head: f.featureHead.SHA}
		assertLatestFallsBack(e, s, onFeature, "with the feature branch named", withParams(byProject, map[string]any{"ref": f.feature}))
	})
}

// The bounds of the wait for the merge request's pipeline of the feature
// branch's head.
const (
	mrHeadPipelineInterval = 2 * time.Second
	mrHeadPipelineWait     = 120 * time.Second
)

// newHeadWithoutPipeline builds the fixture. The branch pipelines are created
// through the API, so each exists when the call returns and they are numbered
// in the order they are created. The push of each commit, and the opening of
// the merge request, are processed in background jobs, so the queue is drained
// before anything is asked: a push pipeline the workflow had let through would
// exist by then. It then holds the fixture to its own premise, that the merge
// request ran a pipeline for the feature branch's head, since without one the
// feature branch would ask nothing a list on the branch alone could get wrong.
//
// That premise is waited for rather than read once. GitLab creates the merge
// request's pipeline for a push at the end of a chain of four background jobs
// (the push, the merge request refresh, the refresh's pipeline job, the
// pipeline creation), each enqueued by the one before it while it runs, so a
// queue seen empty once can still have the chain under way: two runs on
// GitLab 19.4.1 read the list about two seconds before the pipeline was
// created.
func newHeadWithoutPipeline(e *harness.Env) headWithoutPipeline {
	e.T.Helper()

	project := fixture.NewProject(e, fixture.WithNamePrefix("pipelatest"))
	fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, noPushPipelinesCIYAML,
		"ci: run merge request pipelines and no push pipelines")
	earlier := fixture.NewPipelineNoWait(e, project, project.DefaultBranch)

	feature := fixture.NewBranch(e, project, "feature-latest").Name
	fixture.CommitFile(e, project, feature, "FEATURE.md", "a feature\n", "feat: add a feature")
	featureEarlier := fixture.NewPipelineNoWait(e, project, feature)
	mr := fixture.NewMergeRequest(e, project, feature, project.DefaultBranch, "Feature with merge request pipelines")
	featureHead := fixture.CommitFile(e, project, feature, "FEATURE.md", "a feature, revised\n", "feat: revise the feature")

	head := fixture.CommitFile(e, project, project.DefaultBranch, "NOTES.md", "a commit no pipeline runs for\n",
		"docs: add notes")
	fixture.DrainSidekiq(e.Ctx, e.Client())

	err := harness.Poll(e.Ctx, mrHeadPipelineInterval, mrHeadPipelineWait, func() (bool, string, error) {
		mrPipelines, _, listErr := e.Client().GL().MergeRequests.ListMergeRequestPipelines(project.ID, mr.IID, gl.WithContext(e.Ctx))
		if listErr != nil {
			return harness.WaitThrough(fmt.Sprintf("listing the pipelines of merge request !%d", mr.IID), listErr)
		}
		if slices.ContainsFunc(mrPipelines, func(p *gl.PipelineInfo) bool { return p.SHA == featureHead.SHA }) {
			return true, "", nil
		}
		return false, fmt.Sprintf("merge request !%d lists %d pipelines, none for %s", mr.IID, len(mrPipelines), fixture.ShortSHA(featureHead.SHA)), nil
	})
	if err != nil {
		e.T.Fatalf("merge request !%d ran no pipeline for the head of %s (%s), which the workflow should have created: %v",
			mr.IID, feature, fixture.ShortSHA(featureHead.SHA), err)
	}

	return headWithoutPipeline{
		project: project, earlier: earlier, head: head,
		feature: feature, featureEarlier: featureEarlier, featureHead: featureHead,
	}
}

// latestFallback is what pipeline.latest has to answer for one ref: a
// pipeline on that ref, of its earlier commit, with its head beside it.
type latestFallback struct {
	ref     string
	earlier string
	head    string
}

// assertLatestFallsBack asks pipeline.latest with params and holds the answer
// to want: the earlier commit's pipeline on the ref, the head commit beside
// it, and the note.
func assertLatestFallsBack(e *harness.Env, s *harness.Session, want latestFallback, asked string, params map[string]any) {
	e.T.Helper()

	latest := harness.Do[pipelines.LatestOutput](s, actionPipelineLatest, params)
	if latest.Ref != want.ref || latest.SHA != want.earlier {
		e.T.Errorf("pipeline.latest %s answered pipeline %d on %q at %s, want a pipeline on %s at the earlier commit %s",
			asked, latest.ID, latest.Ref, fixture.ShortSHA(latest.SHA), want.ref, fixture.ShortSHA(want.earlier))
	}
	if latest.HeadSHA != want.head {
		e.T.Errorf("pipeline.latest %s head_sha = %q, want the head commit %s", asked, latest.HeadSHA, want.head)
	}
	if !strings.Contains(latest.FallbackNote, "earlier commit") {
		e.T.Errorf("pipeline.latest %s fallback_note = %q, want it to say the pipeline ran for an earlier commit", asked, latest.FallbackNote)
	}
}
