//go:build e2e

// pipelines_latest_test.go covers what pipeline.latest answers when the
// commit at the head of the ref has no pipeline on it. GitLab's route answers
// 403 then, the same status it gives a caller who may not read pipelines, and
// the action falls back to the newest pipeline on the same ref and says so,
// as do the latest_pipeline resource and the two prompts that read it.
// The fixture reaches that state the way a project that runs merge request
// pipelines and no push pipelines does: a head commit whose push workflow:rules
// skips, and a branch with an open merge request, whose head has the merge
// request's pipeline alone.

package common

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
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
//
// The other readers of the same route are held to the same answer (issue
// 1242): the latest_pipeline resource and the summarize_pipeline_status and
// project_health_check prompts, which read the default branch's latest
// pipeline: the resource used to answer GitLab's 403 with the generic
// sentence naming a Maintainer or Owner role, the summary passed GitLab's bare
// "403 Forbidden" on, and the health check wrote N/A. They
// address the server by a URI or a name rather than through a tool, and are
// the same on every surface, so they are read once, on the default one.
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

		if surface == harness.SurfaceDynamic {
			assertLatestResourceFallsBack(e, s, f.project, onDefault)
			assertLatestPromptsFallBack(e, s, f.project, onDefault)
		}
	})
}

// assertLatestResourceFallsBack reads the latest_pipeline resource of the
// project and holds it to want, as pipeline.latest answers without a ref: the
// earlier commit's pipeline on the default branch, the head commit beside it
// and the note.
func assertLatestResourceFallsBack(e *harness.Env, s *harness.Session, project fixture.Project, want latestFallback) {
	e.T.Helper()

	uri := "gitlab://project/" + project.IDParam() + "/pipelines/latest"
	result := s.ReadResource(uri)
	if len(result.Contents) == 0 {
		e.T.Fatalf("%s answered with no contents", uri)
	}
	var latest resources.LatestPipelineResourceOutput
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &latest); err != nil {
		e.T.Fatalf("decoding %s: %v", uri, err)
	}
	if latest.Ref != want.ref || latest.SHA != want.earlier {
		e.T.Errorf("%s answered pipeline %d on %q at %s, want a pipeline on %s at the earlier commit %s",
			uri, latest.ID, latest.Ref, fixture.ShortSHA(latest.SHA), want.ref, fixture.ShortSHA(want.earlier))
	}
	if latest.HeadSHA != want.head {
		e.T.Errorf("%s head_sha = %q, want the head commit %s", uri, latest.HeadSHA, want.head)
	}
	if !strings.Contains(latest.FallbackNote, "earlier commit") {
		e.T.Errorf("%s fallback_note = %q, want it to say the pipeline ran for an earlier commit", uri, latest.FallbackNote)
	}
}

// assertLatestPromptsFallBack renders the two prompts that read the latest
// pipeline of the project's default branch and holds each to want: the
// earlier commit's pipeline, the head commit beside it and the note, where
// the summary used to be refused with GitLab's bare 403 and the health check
// to read N/A.
func assertLatestPromptsFallBack(e *harness.Env, s *harness.Session, project fixture.Project, want latestFallback) {
	e.T.Helper()

	for _, name := range []string{"summarize_pipeline_status", "project_health_check"} {
		result := s.GetPrompt(name, map[string]string{"project_id": project.IDParam()})
		if len(result.Messages) == 0 {
			e.T.Fatalf("prompt %s answered with no message", name)
		}
		content, ok := result.Messages[0].Content.(*mcp.TextContent)
		if !ok {
			e.T.Fatalf("prompt %s answered %T, want text", name, result.Messages[0].Content)
		}
		for _, part := range []string{
			"**SHA**: " + fixture.ShortSHA(want.earlier),
			"**Head SHA**: " + fixture.ShortSHA(want.head),
			"earlier commit",
		} {
			if !strings.Contains(content.Text, part) {
				e.T.Errorf("prompt %s does not hold %q:\n%s", name, part, content.Text)
			}
		}
	}
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
	// The configuration's commit carries no skip marker, unlike the other
	// configurations a scenario commits to a default branch, and must not.
	// Its workflow rules already keep the push from creating any pipeline, so
	// every branch pipeline is one this fixture creates, which is the premise
	// above. GitLab judges the marker before the workflow rules, so a marked
	// commit would make it record a skipped pipeline for every push it heads,
	// the feature branch created from it below among them. GitLab records
	// that one in a background job, so it can land after the branch's own
	// earlier pipeline and become the newest on that branch, the race
	// noPushPipelinesCIYAML records, and the feature branch's case would read
	// it instead.
	//gitlab:allow-push-pipeline the workflow rules already refuse a push pipeline
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
