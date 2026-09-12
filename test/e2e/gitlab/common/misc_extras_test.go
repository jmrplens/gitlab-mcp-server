//go:build e2e

// misc_extras_test.go ports the single-action coverage gaps the old suite kept
// in its misc extras file: the latest-release read, the merge request raw
// diffs, the CI/CD catalog resource read, the deployment merge requests list,
// the work item type list and the job-token scope read.
//
// Each names one action the reads sweep cannot reach, and each is placed here
// by the family of that action. The stateful and runner-bound ones run on the
// individual surface the old suite used, since building their fixtures on three
// surfaces would treble the suite for a read the other surfaces already reach
// through their families; the job-token read runs through the meta tool.

package common

import (
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestReleases_GetLatest creates a tag and a release on a fresh project and
// reads the latest release back, waiting because a just-created release can
// lag the latest-release endpoint.
//
// Replaces: TestIndividual_ReleaseGetLatest
func TestReleases_GetLatest(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceIndividual)
	project := fixture.NewProject(e, fixture.WithNamePrefix("rel-latest"))

	const tagName = "v1.0.0-latest-e2e"
	created := harness.Do[tags.Output](s, actionTagCreate, map[string]any{
		"project_id": project.IDParam(), "tag_name": tagName, "ref": project.DefaultBranch, "message": "Latest release E2E tag",
	})
	if created.Name != tagName {
		e.T.Fatalf("tag create answered %q, want %q", created.Name, tagName)
	}
	release := harness.Do[releases.Output](s, actionReleaseCreate, map[string]any{
		"project_id": project.IDParam(), "tag_name": tagName, "name": "E2E Latest Release", "description": "Automated E2E latest-release fixture.",
	})
	if release.TagName != tagName {
		e.T.Fatalf("release create answered %q, want %q", release.TagName, tagName)
	}

	latest := harness.Eventually[releases.Output](s, actionReleaseGetLatest, map[string]any{"project_id": project.IDParam()},
		2*time.Second, 60*time.Second, func(out releases.Output) bool { return out.TagName == tagName })
	if latest.TagName != tagName {
		e.T.Errorf("release get_latest answered %q, want %q", latest.TagName, tagName)
	}
}

// TestMergeRequests_RawDiffs creates a merge request from a diverged feature
// branch and reads its raw unified diff, which GitLab computes asynchronously,
// asserting the diff mentions the feature-branch file.
//
// Replaces: TestIndividual_MRRawDiffs
func TestMergeRequests_RawDiffs(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceIndividual)
	project := fixture.NewProject(e, fixture.WithNamePrefix("mr-rawdiff"))

	branch := fixture.NewBranch(e, project, e.Name("feature"))
	fixture.CommitFile(e, project, branch.Name, "mr-test.txt", "raw diff content", "add the raw diff fixture file")
	mr := fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, "MR for raw diffs test")

	// The raw diff is computed asynchronously, so it is polled until it is
	// non-empty, and must mention the file only the feature branch carries.
	out := harness.Eventually[mrchanges.RawDiffsOutput](s, actionMRRawDiffs, map[string]any{"project_id": project.IDParam(), "merge_request_iid": mr.IID},
		2*time.Second, 90*time.Second, func(out mrchanges.RawDiffsOutput) bool { return out.RawDiff != "" })
	if out.MRIID != mr.IID {
		e.T.Errorf("mr raw_diffs answered MR %d, want %d", out.MRIID, mr.IID)
	}
	if !strings.Contains(out.RawDiff, "mr-test.txt") {
		e.T.Errorf("the raw diff does not mention the feature-branch file: %q", out.RawDiff)
	}
}

// TestDeploymentMergeRequests_List creates an environment and an API
// deployment through the server, then lists the merge requests the deployment
// shipped, which is an empty but well-formed list for an API deployment.
//
// Replaces: TestIndividual_DeploymentMergeRequests
func TestDeploymentMergeRequests_List(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceIndividual)
	project := fixture.NewProject(e, fixture.WithNamePrefix("dmr"))
	commit := fixture.CommitFile(e, project, project.DefaultBranch, "deploy-mr.txt", "deployment content", "deployment commit")

	envName := e.Name("dmr-env")
	env := harness.Do[environments.Output](s, actionEnvironmentCreate, map[string]any{"project_id": project.IDParam(), "name": envName})
	if env.ID == 0 {
		e.T.Fatalf("environment create answered %+v, want an environment with an ID", env)
	}

	// GitLab 19 rejects a deployment creation that omits the tag flag, so the
	// request states the ref is a branch and the status GitLab accepts.
	deployment := harness.Do[deployments.Output](s, actionEnvironmentDeploymentCreate, map[string]any{
		"project_id": project.IDParam(), "environment": envName, "ref": project.DefaultBranch,
		"sha": commit.SHA, "tag": false, "status": "running",
	})
	if deployment.ID == 0 {
		e.T.Fatalf("deployment create answered %+v, want a deployment with an ID", deployment)
	}

	out := harness.Do[deploymentmergerequests.ListOutput](s, actionEnvironmentDeploymentMergeRequests, map[string]any{
		"project_id": project.IDParam(), "deployment_id": deployment.ID,
	})
	e.T.Logf("deployment %d shipped %d merge request(s)", deployment.ID, len(out.MergeRequests))
}

// TestWorkItems_Types_Individual lists a project's work item types through the
// individual tool and finds the system-defined Issue type among them.
//
// Replaces: TestIndividual_WorkItemTypes
func TestWorkItems_Types_Individual(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceIndividual)
	project := fixture.NewProject(e, fixture.WithNamePrefix("witypes-ind"))

	out := harness.Do[workitems.WorkItemTypeListOutput](s, actionWorkItemTypeList, map[string]any{"full_path": project.Path})
	if len(out.Types) == 0 {
		e.T.Fatalf("work_item_type_list answered no types")
	}
	found := false
	for _, workItemType := range out.Types {
		if workItemType.ID == "" {
			e.T.Errorf("work item type %q has an empty GID", workItemType.Name)
		}
		if strings.EqualFold(workItemType.Name, "issue") {
			found = true
		}
	}
	if !found {
		e.T.Errorf("the work item types do not hold the system-defined Issue type: %+v", out.Types)
	}
}

// TestJobTokens_ScopeAndList lists a project's jobs and reads its job-token
// access scope through the gitlab_job meta tool.
//
// Replaces: TestMeta_JobTokens
func TestJobTokens_ScopeAndList(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)
	project := fixture.NewProject(e, fixture.WithNamePrefix("jobtokens"))

	list := harness.Do[jobs.ListOutput](s, actionJobListProject, map[string]any{"project_id": project.IDParam()})
	e.T.Logf("project %s has %d job(s)", project.Path, len(list.Jobs))

	scope := harness.Do[jobtokenscope.AccessSettingsOutput](s, actionJobTokenScopeGet, map[string]any{"project_id": project.IDParam()})
	e.T.Logf("job token inbound scope enabled: %v", scope.InboundEnabled)
}

// TestCICatalog_Get_AfterPublish marks a project as a CI/CD catalog resource,
// publishes its first version through a real runner pipeline, and reads the
// resource back by full path. Publishing needs a runner and pulls the
// release-cli image, so the test needs the Docker stack; a version that does
// not publish leaves the resource an unqueryable draft and the read is
// skipped.
//
// Replaces: TestIndividual_CICatalogGet
func TestCICatalog_Get_AfterPublish(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))
	s := e.On(harness.SurfaceIndividual)
	if !e.DockerMode() {
		e.Skipf("publishing a catalog version needs the compose-internal GitLab URL and a runner, which only the Docker stack provides")
	}
	project := fixture.NewProject(e, fixture.WithNamePrefix("cicat-get"))

	if !markCatalogResource(e, project) {
		return
	}
	publishCatalogVersion(e, project)

	got, err := harness.Try[cicatalog.GetOutput](s, actionCICatalogGet, map[string]any{"full_path": project.Path})
	if err != nil {
		e.Skipf("the catalog resource is not queryable after the publish attempt (async publication or a runner-less version): %v", err)
	}
	if got.Resource.FullPath == "" {
		e.T.Errorf("ci_catalog get answered a resource with no full path: %+v", got.Resource)
	}
	e.T.Logf("read catalog resource %s (id=%s)", got.Resource.FullPath, got.Resource.ID)
}

// markCatalogResource marks the project as a catalog resource through GraphQL,
// which the fixture (a project with a description and a README) already
// satisfies. It reports whether the mark succeeded; a version- or
// policy-dependent refusal skips the test.
func markCatalogResource(e *harness.Env, project fixture.Project) bool {
	e.T.Helper()

	const mutation = `mutation($projectPath: ID!) {
		catalogResourcesCreate(input: {projectPath: $projectPath}) {
			errors
		}
	}`
	var resp struct {
		CatalogResourcesCreate struct {
			Errors []string `json:"errors"`
		} `json:"catalogResourcesCreate"`
	}
	if _, err := e.Client().GL().GraphQL.Do(gl.GraphQLQuery{Query: mutation, Variables: map[string]any{"projectPath": project.Path}},
		&resp, gl.WithContext(e.Ctx)); err != nil {
		e.Skipf("catalogResourcesCreate is unavailable on this GitLab: %v", err)
		return false
	}
	if len(resp.CatalogResourcesCreate.Errors) > 0 {
		e.Skipf("could not mark the project as a catalog resource: %v", resp.CatalogResourcesCreate.Errors)
		return false
	}
	return true
}

// catalogComponent is the CI/CD component the catalog version publishes.
const catalogComponent = `spec:
  inputs:
    stage:
      default: test
---
component-job:
  stage: $[[ inputs.stage ]]
  script:
    - echo "hello from the e2e component"
`

// publishCatalogVersion publishes the project's first catalog version through
// a real pipeline: it commits a component, a README and a release job, tags a
// semver, and waits for the tag pipeline's release job to publish the version.
// A catalog version publishes only through the release: keyword inside a
// pipeline, so a REST-created release does not count.
func publishCatalogVersion(e *harness.Env, project fixture.Project) {
	e.T.Helper()

	releaseJob := "create-release:\n" +
		"  image: registry.gitlab.com/gitlab-org/release-cli:latest\n" +
		"  rules:\n" +
		"    - if: $CI_COMMIT_TAG\n" +
		"  script:\n" +
		"    - release-cli --server-url " + fixture.InternalGitLabURL(e) +
		" --job-token \"$CI_JOB_TOKEN\" create --tag-name \"$CI_COMMIT_TAG\" --description \"Release $CI_COMMIT_TAG\"\n"
	fixture.CommitFile(e, project, project.DefaultBranch, "README.md", "# e2e catalog component\n", "Add README")
	fixture.CommitFile(e, project, project.DefaultBranch, "templates/hello.yml", catalogComponent, "Add component")
	fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, releaseJob, "Add release job")

	// Tag pipelines are created asynchronously by Sidekiq, so the queue is
	// drained first and the tag pipeline is polled for.
	fixture.DrainSidekiq(e.Ctx, e.Client())
	const tagName = "1.0.0"
	if _, _, err := e.Client().GL().Tags.CreateTag(project.ID, &gl.CreateTagOptions{
		TagName: new(tagName), Ref: new(project.DefaultBranch),
	}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("creating the semver tag for the catalog release: %v", err)
	}

	var pipelineID int64
	if err := harness.Poll(e.Ctx, 5*time.Second, 300*time.Second, func() (bool, string, error) {
		pipelines, _, listErr := e.Client().GL().Pipelines.ListProjectPipelines(project.ID, &gl.ListProjectPipelinesOptions{Ref: new(tagName)}, gl.WithContext(e.Ctx))
		if listErr != nil {
			//nolint:nilerr // A transient list error is retried until the poll deadline.
			return false, "listing tag pipelines: " + listErr.Error(), nil
		}
		if len(pipelines) == 0 {
			return false, "the tag pipeline is not created yet", nil
		}
		pipelineID = pipelines[0].ID
		return true, "the tag pipeline exists", nil
	}); err != nil {
		e.Skipf("the catalog release tag pipeline never appeared: %v", err)
	}

	// The wait is bounded and non-fatal here, unlike the fixture's own: the
	// release-cli image pull is an external dependency that can be slow or
	// fail on an isolated network, and a catalog version that does not
	// publish is a skip rather than a failure of the whole run.
	fixture.DrainSidekiq(e.Ctx, e.Client())
	status := "unknown"
	if err := harness.Poll(e.Ctx, 10*time.Second, 8*time.Minute, func() (bool, string, error) {
		pipeline, _, getErr := e.Client().GL().Pipelines.GetPipeline(project.ID, pipelineID, gl.WithContext(e.Ctx))
		if getErr != nil {
			//nolint:nilerr // A transient read is retried until the poll deadline.
			return false, "reading the release pipeline: " + getErr.Error(), nil
		}
		status = pipeline.Status
		return fixture.IsTerminalPipelineStatus(status), "status=" + status, nil
	}); err != nil {
		e.Skipf("the catalog release pipeline did not finish in time (last status %q); the release-cli image pull can be slow: %v", status, err)
	}
	if status != "success" {
		e.Skipf("the catalog release pipeline finished %q rather than success (the release-cli image pull can fail on an isolated network)", status)
	}
	e.T.Logf("published catalog version %s through pipeline %d", tagName, pipelineID)
}
