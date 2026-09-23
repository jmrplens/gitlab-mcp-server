//go:build e2e

// world_extras_test.go drives the World's extras against the stub: the build
// that makes them one at a time and survives the ones it cannot make, the
// pipeline sequence that leaves a canceled pipeline and its job, the readers
// that put them in the digest, and the bindings that hand them to the sweeps.
//
// The binding tests read the server's own resource registry through
// internal/resources.NewHandlerIndex, a test-only import of this package: it
// is how a template the server registers and the World cannot bind fails here
// rather than as a line in a Docker run's log.

package fixture

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
	yaml "go.yaml.in/yaml/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// extrasRunID is the run the extras tests build under.
const extrasRunID = "run"

// The objects the stub hands back for the extras, spelled once so a test can
// check that each was filled in from what GitLab answered.
const (
	stubEnvironmentID    int64 = 5
	stubDeploymentID     int64 = 6
	stubDeployKeyID      int64 = 7
	stubBoardID          int64 = 8
	stubProjectSnippetID int64 = 9
	stubSnippetID        int64 = 10
	stubGroupLabelID     int64 = 11
	stubGroupMilestoneID int64 = 12
	stubGroupMilestoneIn int64 = 3
	stubPipelineID       int64 = 77
	stubJobID            int64 = 88
	stubWikiSlug               = "world-wiki-run"
	stubFlagName               = "flag-0123456789"
)

// extrasWorld is a World whose core the stub knows, with none of its extras
// made yet.
func extrasWorld() *World {
	return &World{
		Group:   Group{ID: 1, Path: "e2e-world-group-run"},
		Project: Project{ID: 2, Path: "e2e-world-group-run/e2e-world-project-run", DefaultBranch: "main"},
		Branch:  Branch{Name: "feature/world"},
		Commit:  Commit{SHA: "feature-sha"},
	}
}

// scriptWorldExtras tells the stub to accept every extra the World makes, the
// pipeline settling as canceled, so a test changes only the answer it is
// about.
func scriptWorldExtras(stub *stubGitLab) {
	tagName := worldName(extrasRunID, worldTagPrefix)
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/tags", stubCreated(map[string]any{"name": tagName, "message": tagMessage}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/releases", stubCreated(map[string]any{"tag_name": tagName, "name": "Release " + tagName}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/environments", stubCreated(map[string]any{"id": stubEnvironmentID, "name": "world-environment-run"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/deployments", stubCreated(map[string]any{"id": stubDeploymentID, "sha": "feature-sha", "status": "running"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/feature_flags", stubCreated(map[string]any{"name": stubFlagName, "active": true}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/deploy_keys", stubCreated(map[string]any{"id": stubDeployKeyID, "title": "world-deploy-key-run"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/snippets", stubCreated(map[string]any{"id": stubProjectSnippetID, "title": "world-project-snippet-run"}))
	stub.answers(http.MethodPost, "/api/v4/snippets", stubCreated(map[string]any{"id": stubSnippetID, "title": "world-snippet-run"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/wikis", stubCreated(map[string]any{"slug": stubWikiSlug, "title": "world-wiki-run"}))
	stub.answers(http.MethodPost, "/api/v4/groups/1/labels", stubCreated(map[string]any{"id": stubGroupLabelID, "name": "world-group-label-run"}))
	stub.answers(http.MethodPost, "/api/v4/groups/1/milestones", stubCreated(map[string]any{
		"id": stubGroupMilestoneID, "iid": stubGroupMilestoneIn, "title": "world-group-milestone-run",
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/commits", stubCreated(map[string]any{"id": "ci-sha", "short_id": "ci-sha"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/pipeline", stubCreated(map[string]any{
		"id": stubPipelineID, "ref": "main", "sha": "ci-sha", "status": "created",
	}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/pipelines/77/jobs", stubOK([]map[string]any{
		{"id": 87, "name": "some-other-job", "status": "pending"},
		{"id": stubJobID, "name": worldJobName, "status": "pending"},
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/pipelines/77/cancel", stubOK(map[string]any{"id": stubPipelineID, "status": "canceling"}))
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"createBoard":{"board":{"id":"gid://gitlab/Board/8"},"errors":[]}}}`}
		stub.pipelineStatuses = []string{"canceling", "canceled"}
	})
}

// shortWorldPipelineWaits makes the World pipeline's waits short enough for a
// test that watches one run out, and restores them when the test ends.
func shortWorldPipelineWaits(t *testing.T) {
	t.Helper()
	shortPipelinePolls(t)
	savedJob, savedSettle := worldJobWait, worldPipelineSettle
	worldJobWait, worldPipelineSettle = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { worldJobWait, worldPipelineSettle = savedJob, savedSettle })
}

// sentPaths lists the method and path of every request the scripted route
// answered, for a test asserting what was and was not asked for.
func sentPaths(stub *stubGitLab) []string {
	var paths []string
	for _, request := range stub.recordedRequests() {
		paths = append(paths, request.Method+" "+request.Path)
	}
	return paths
}

// requestTo returns the first recorded request to one method and path.
func requestTo(t *testing.T, stub *stubGitLab, method, path string) stubRequest {
	t.Helper()
	for _, request := range stub.recordedRequests() {
		if request.Method == method && request.Path == path {
			return request
		}
	}
	t.Fatalf("no %s %s was sent; sent: %v", method, path, sentPaths(stub))
	return stubRequest{}
}

// TestBuildWorldExtras_OneRefused_OthersStillBind checks the promise the
// extras rest on: an instance that refuses one of them (a project whose wiki
// is disabled answers 403) leaves that one unbound with GitLab's own reason,
// and every other is still made, without the build failing the test that
// asked for the World.
func TestBuildWorldExtras_OneRefused_OthersStillBind(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, client := newStubGitLab(t)
	scriptWorldExtras(stub)
	stub.answers(http.MethodPost, "/api/v4/projects/2/wikis", stubRefusal(http.StatusForbidden, "403 Forbidden"))
	world := extrasWorld()

	buildWorldExtras(t.Context(), t, client, extrasRunID, world)

	if got := slices.Collect(maps.Keys(world.unbound)); !slices.Equal(got, []string{worldExtraWikiPage}) {
		t.Fatalf("unbound = %v, want only the wiki page", world.unbound)
	}
	if reason := world.unbound[worldExtraWikiPage]; !strings.Contains(reason, "403") {
		t.Errorf("the wiki page is unbound for %q, want GitLab's 403", reason)
	}
	made := map[string]bool{}
	for _, extra := range worldExtras {
		made[extra.name] = extra.made(world)
	}
	for _, extra := range worldExtras {
		t.Run(extra.name, func(t *testing.T) {
			if want := extra.name != worldExtraWikiPage; made[extra.name] != want {
				t.Errorf("made = %t, want %t", made[extra.name], want)
			}
		})
	}

	want := World{
		Tag:            Tag{Name: worldName(extrasRunID, worldTagPrefix), Ref: "main"},
		Release:        Release{TagName: worldName(extrasRunID, worldTagPrefix), Name: "Release " + worldName(extrasRunID, worldTagPrefix)},
		Environment:    Environment{ID: stubEnvironmentID, Name: "world-environment-run"},
		FeatureFlag:    ProjectFeatureFlag{Name: stubFlagName, Active: true},
		Board:          ProjectBoard{ID: stubBoardID, Name: worldName(extrasRunID, worldBoardPrefix)},
		Snippet:        Snippet{ID: stubSnippetID, Title: "world-snippet-run"},
		GroupLabel:     GroupLabel{ID: stubGroupLabelID, Name: "world-group-label-run"},
		GroupMilestone: GroupMilestone{ID: stubGroupMilestoneID, IID: stubGroupMilestoneIn, Title: "world-group-milestone-run"},
		Pipeline:       Pipeline{ID: stubPipelineID, Ref: "main", SHA: "ci-sha", Status: "canceled"},
		JobID:          stubJobID,
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{name: "tag", got: world.Tag, want: want.Tag},
		{name: "release", got: world.Release, want: want.Release},
		{name: "environment", got: world.Environment, want: want.Environment},
		{name: "deployment", got: world.Deployment.ID, want: stubDeploymentID},
		{name: "feature flag", got: world.FeatureFlag, want: want.FeatureFlag},
		{name: "deploy key", got: world.DeployKey.ID, want: stubDeployKeyID},
		{name: "board", got: world.Board, want: want.Board},
		{name: "project snippet", got: world.ProjectSnippet.ID, want: stubProjectSnippetID},
		{name: "personal snippet", got: world.Snippet, want: want.Snippet},
		{name: "group label", got: world.GroupLabel, want: want.GroupLabel},
		{name: "group milestone", got: world.GroupMilestone, want: want.GroupMilestone},
		{name: "pipeline", got: world.Pipeline, want: want.Pipeline},
		{name: "job", got: world.JobID, want: want.JobID},
		{name: "wiki page", got: world.WikiPage, want: WikiPage{}},
	}
	for _, check := range checks {
		t.Run("filled in "+check.name, func(t *testing.T) {
			if check.got != check.want {
				t.Errorf("%s = %+v, want %+v", check.name, check.got, check.want)
			}
		})
	}
}

// TestBuildWorldExtras_Requests_NameEachExtraForTheRun checks what the build
// asks GitLab for: each extra named under the run so the World's objects are
// recognizably the run's, the deployment of the World's own commit from the
// branch that carries it, the board in the World's project rather than its
// group, and the two group objects in the World's group.
func TestBuildWorldExtras_Requests_NameEachExtraForTheRun(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, client := newStubGitLab(t)
	scriptWorldExtras(stub)

	buildWorldExtras(t.Context(), t, client, extrasRunID, extrasWorld())

	cases := []struct {
		name, method, path, field string
		want                      any
	}{
		{name: "tag", method: http.MethodPost, path: "/api/v4/projects/2/repository/tags", field: "tag_name", want: worldName(extrasRunID, worldTagPrefix)},
		{name: "tag ref", method: http.MethodPost, path: "/api/v4/projects/2/repository/tags", field: "ref", want: "main"},
		{name: "release", method: http.MethodPost, path: "/api/v4/projects/2/releases", field: "tag_name", want: worldName(extrasRunID, worldTagPrefix)},
		{name: "release notes", method: http.MethodPost, path: "/api/v4/projects/2/releases", field: "description", want: worldDescription(extrasRunID)},
		{name: "environment", method: http.MethodPost, path: "/api/v4/projects/2/environments", field: "name", want: worldName(extrasRunID, worldEnvironmentPrefix)},
		{name: "deployment ref", method: http.MethodPost, path: "/api/v4/projects/2/deployments", field: "ref", want: "feature/world"},
		{name: "deployment sha", method: http.MethodPost, path: "/api/v4/projects/2/deployments", field: "sha", want: "feature-sha"},
		{name: "deployment environment", method: http.MethodPost, path: "/api/v4/projects/2/deployments", field: "environment", want: "world-environment-run"},
		{
			name: "feature flag", method: http.MethodPost, path: "/api/v4/projects/2/feature_flags", field: "name",
			want: featureFlagNameFrom(worldName(extrasRunID, worldFlagPrefix)),
		},
		{name: "deploy key", method: http.MethodPost, path: "/api/v4/projects/2/deploy_keys", field: "title", want: worldName(extrasRunID, worldDeployKeyPrefix)},
		{
			name: "project snippet", method: http.MethodPost, path: "/api/v4/projects/2/snippets", field: "title",
			want: worldName(extrasRunID, worldProjectSnippetPrefix),
		},
		{name: "personal snippet", method: http.MethodPost, path: "/api/v4/snippets", field: "title", want: worldName(extrasRunID, worldSnippetPrefix)},
		{name: "wiki page", method: http.MethodPost, path: "/api/v4/projects/2/wikis", field: "title", want: worldName(extrasRunID, worldWikiPrefix)},
		{name: "group label", method: http.MethodPost, path: "/api/v4/groups/1/labels", field: "name", want: worldName(extrasRunID, worldGroupLabelPrefix)},
		{
			name: "group milestone", method: http.MethodPost, path: "/api/v4/groups/1/milestones", field: "title",
			want: worldName(extrasRunID, worldGroupMilestonePrefix),
		},
		{name: "pipeline ref", method: http.MethodPost, path: "/api/v4/projects/2/pipeline", field: "ref", want: "main"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := requestTo(t, stub, testCase.method, testCase.path).Body[testCase.field]; got != testCase.want {
				t.Errorf("%s %s sent %s = %v, want %v", testCase.method, testCase.path, testCase.field, got, testCase.want)
			}
		})
	}

	documents := stub.graphqlDocuments
	if len(documents) != 1 || !strings.Contains(documents[0], "projectPath") || strings.Contains(documents[0], "groupPath") {
		t.Errorf("the board was asked for with %q, want one document naming the project and not the group", documents)
	}
}

// TestBuildWorldExtras_StandsOnOneNotMade_IsNotAttempted checks the two
// dependencies: a release is not asked for when its tag was refused, nor a
// deployment when its environment was, and each says which object it lacked.
func TestBuildWorldExtras_StandsOnOneNotMade_IsNotAttempted(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, client := newStubGitLab(t)
	scriptWorldExtras(stub)
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/tags", stubRefusal(http.StatusForbidden, "403 Forbidden"))
	stub.answers(http.MethodPost, "/api/v4/projects/2/environments", stubRefusal(http.StatusForbidden, "403 Forbidden"))
	world := extrasWorld()

	buildWorldExtras(t.Context(), t, client, extrasRunID, world)

	cases := []struct {
		extra, stood, path string
	}{
		{extra: worldExtraRelease, stood: worldExtraTag, path: "/api/v4/projects/2/releases"},
		{extra: worldExtraDeployment, stood: worldExtraEnvironment, path: "/api/v4/projects/2/deployments"},
	}
	for _, testCase := range cases {
		t.Run(testCase.extra, func(t *testing.T) {
			reason, unbound := world.unbound[testCase.extra]
			if !unbound {
				t.Fatalf("the %s was made on a %s the World does not have", testCase.extra, testCase.stood)
			}
			if !strings.Contains(reason, "stands on the World's "+testCase.stood) || !strings.Contains(reason, "403") {
				t.Errorf("the %s is unbound for %q, want it to name the %s and why that was not made", testCase.extra, reason, testCase.stood)
			}
			if slices.Contains(sentPaths(stub), http.MethodPost+" "+testCase.path) {
				t.Errorf("the %s was asked for although the %s it stands on was not made", testCase.extra, testCase.stood)
			}
		})
	}
}

// TestCreateWorldPipeline_Cancelled_BindsPipelineAndJob checks the sequence on
// an instance that does what it is asked: the configuration committed to the
// default branch admits only pipelines the API asks for and runs its one job
// on a runner nobody has, the job picked out by name rather than position,
// the pipeline canceled, and the two filled in once it settled.
func TestCreateWorldPipeline_Cancelled_BindsPipelineAndJob(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, client := newStubGitLab(t)
	scriptWorldExtras(stub)
	world := extrasWorld()

	if err := createWorldPipeline(t.Context(), t, client, extrasRunID, world); err != nil {
		t.Fatalf("createWorldPipeline() error = %v, want nil", err)
	}
	if world.Pipeline != (Pipeline{ID: stubPipelineID, Ref: "main", SHA: "ci-sha", Status: "canceled"}) || world.JobID != stubJobID {
		t.Errorf("pipeline = %+v, job = %d, want pipeline 77 canceled and its world-job 88", world.Pipeline, world.JobID)
	}
	requestTo(t, stub, http.MethodPost, "/api/v4/projects/2/pipelines/77/cancel")

	commit := requestTo(t, stub, http.MethodPost, "/api/v4/projects/2/repository/commits")
	if commit.Body["branch"] != "main" {
		t.Errorf("the configuration was committed to %v, want the default branch", commit.Body["branch"])
	}
	actions, _ := commit.Body["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("the commit carried %d actions, want the one file", len(actions))
	}
	action, _ := actions[0].(map[string]any)
	if action["action"] != "create" || action["file_path"] != CIFilePath {
		t.Errorf("the commit action is %v %v, want a create of %s", action["action"], action["file_path"], CIFilePath)
	}
	content, _ := action["content"].(string)
	var config struct {
		Workflow struct {
			Rules []map[string]any `yaml:"rules"`
		} `yaml:"workflow"`
		Job struct {
			Tags []string `yaml:"tags"`
		} `yaml:"world-job"`
	}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		t.Fatalf("the committed configuration is not YAML: %v\n%s", err, content)
	}
	if len(config.Workflow.Rules) != 1 || config.Workflow.Rules[0]["if"] != `$CI_PIPELINE_SOURCE == "api"` || len(config.Workflow.Rules[0]) != 1 {
		t.Errorf("workflow rules = %v, want the one rule admitting only pipelines the API asks for", config.Workflow.Rules)
	}
	if !slices.Equal(config.Job.Tags, []string{worldJobTag}) {
		t.Errorf("the job asks for runner tags %v, want only %q, which no runner carries", config.Job.Tags, worldJobTag)
	}
}

// TestCreateWorldPipeline_NeverSettles_LeavesBothUnbound checks that a
// pipeline still canceling when the settle budget runs out binds neither
// itself nor its job, since binding one puts a wait on an unfinished pipeline
// into the reads sweep.
func TestCreateWorldPipeline_NeverSettles_LeavesBothUnbound(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, client := newStubGitLab(t)
	scriptWorldExtras(stub)
	stub.configure(func() { stub.pipelineStatuses = []string{"canceling"} })
	world := extrasWorld()

	err := createWorldPipeline(t.Context(), t, client, extrasRunID, world)

	if !errors.Is(err, harness.ErrPollTimeout) || !strings.Contains(err.Error(), "did not settle") || !strings.Contains(err.Error(), "canceling") {
		t.Errorf("createWorldPipeline() error = %v, want a settle timeout naming the last status", err)
	}
	if world.Pipeline != (Pipeline{}) || world.JobID != 0 {
		t.Errorf("pipeline = %+v, job = %d, want neither filled in", world.Pipeline, world.JobID)
	}
}

// TestCreateWorldPipeline_NoJob_StopsBeforeTheCancel checks that a pipeline
// that never grows its job is reported with the job it waited for, and is
// neither canceled nor bound.
func TestCreateWorldPipeline_NoJob_StopsBeforeTheCancel(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, client := newStubGitLab(t)
	scriptWorldExtras(stub)
	stub.answers(http.MethodGet, "/api/v4/projects/2/pipelines/77/jobs", stubOK([]map[string]any{{"id": 87, "name": "some-other-job"}}))
	world := extrasWorld()

	err := createWorldPipeline(t.Context(), t, client, extrasRunID, world)

	if !errors.Is(err, harness.ErrPollTimeout) || !strings.Contains(err.Error(), "grew no "+worldJobName) {
		t.Errorf("createWorldPipeline() error = %v, want a wait that names the job it never saw", err)
	}
	if slices.Contains(sentPaths(stub), http.MethodPost+" /api/v4/projects/2/pipelines/77/cancel") {
		t.Error("a pipeline with no job was canceled")
	}
	if world.Pipeline != (Pipeline{}) || world.JobID != 0 {
		t.Errorf("pipeline = %+v, job = %d, want neither filled in", world.Pipeline, world.JobID)
	}
}

// TestCreateWorldPipeline_StepRefused_StopsThereWithGitLabsReason checks each
// request of the sequence being refused: the sequence stops at it, reports
// GitLab's answer and binds nothing.
func TestCreateWorldPipeline_StepRefused_StopsThereWithGitLabsReason(t *testing.T) {
	cases := []struct {
		name, method, path, later string
	}{
		{name: "commit", method: http.MethodPost, path: "/api/v4/projects/2/repository/commits", later: "/api/v4/projects/2/pipeline"},
		{name: "create", method: http.MethodPost, path: "/api/v4/projects/2/pipeline", later: "/api/v4/projects/2/pipelines/77/jobs"},
		{name: "cancel", method: http.MethodPost, path: "/api/v4/projects/2/pipelines/77/cancel"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			shortWorldPipelineWaits(t)
			stub, client := newStubGitLab(t)
			scriptWorldExtras(stub)
			stub.answers(testCase.method, testCase.path, stubRefusal(http.StatusForbidden, "403 Forbidden"))
			world := extrasWorld()

			err := createWorldPipeline(t.Context(), t, client, extrasRunID, world)

			if !IsStatus(err, http.StatusForbidden) {
				t.Errorf("createWorldPipeline() error = %v, want GitLab's 403", err)
			}
			for _, sent := range sentPaths(stub) {
				if testCase.later != "" && strings.HasSuffix(sent, " "+testCase.later) {
					t.Errorf("%s was sent after the %s was refused", sent, testCase.name)
				}
			}
			if world.Pipeline != (Pipeline{}) || world.JobID != 0 {
				t.Errorf("pipeline = %+v, job = %d, want neither filled in", world.Pipeline, world.JobID)
			}
		})
	}
}

// TestCreateWorldDeployKey_KeyNotMade_IsTheReasonAndNothingIsSent covers the
// one failure an extra can have before it reaches GitLab.
func TestCreateWorldDeployKey_KeyNotMade_IsTheReasonAndNothingIsSent(t *testing.T) {
	stub, client := newStubGitLab(t)
	saved := worldPublicKey
	t.Cleanup(func() { worldPublicKey = saved })
	errNoKey := errors.New("no entropy")
	worldPublicKey = func() (string, error) { return "", errNoKey }
	world := extrasWorld()

	err := createWorldDeployKey(t.Context(), t, client, extrasRunID, world)

	if !errors.Is(err, errNoKey) {
		t.Errorf("createWorldDeployKey() error = %v, want the key generation's", err)
	}
	if sent := sentPaths(stub); len(sent) != 0 {
		t.Errorf("sent %v, want nothing without a key to add", sent)
	}
	if world.DeployKey != (DeployKey{}) {
		t.Errorf("deploy key = %+v, want none", world.DeployKey)
	}
}

// madeWorld is a World holding every extra, as a build against a GitLab that
// refused nothing leaves it.
func madeWorld() *World {
	world := extrasWorld()
	world.Label = Label{ID: 55}
	world.Milestone = Milestone{ID: 66, IID: 4}
	world.Tag = Tag{Name: "world-tag-run", Ref: "main"}
	world.Release = Release{TagName: "world-tag-run", Name: "Release world-tag-run"}
	world.Environment = Environment{ID: stubEnvironmentID, Name: "world-environment-run"}
	world.Deployment = Deployment{ID: stubDeploymentID}
	world.FeatureFlag = ProjectFeatureFlag{Name: stubFlagName, Active: true}
	world.DeployKey = DeployKey{ID: stubDeployKeyID}
	world.Board = ProjectBoard{ID: stubBoardID}
	world.ProjectSnippet = ProjectSnippet{ID: stubProjectSnippetID}
	world.Snippet = Snippet{ID: stubSnippetID}
	world.WikiPage = WikiPage{Slug: stubWikiSlug}
	world.GroupLabel = GroupLabel{ID: stubGroupLabelID}
	world.GroupMilestone = GroupMilestone{ID: stubGroupMilestoneID, IID: stubGroupMilestoneIn}
	world.Pipeline = Pipeline{ID: stubPipelineID}
	world.JobID = stubJobID
	return world
}

// registeredTemplates returns every resource the server registers, keyed by
// URI or template, from the server's own resource registry.
func registeredTemplates(t *testing.T) []string {
	t.Helper()
	_, client := newStubGitLab(t)
	templates := slices.Sorted(maps.Keys(resources.NewHandlerIndex(client)))
	if len(templates) < 30 {
		t.Fatalf("the server registers %d resources, which is not the registry this test was written against", len(templates))
	}
	return templates
}

// TestWorldTemplates_EveryRegisteredTemplate_Binds holds the World to every
// resource template the server registers: each variable of each binds from a
// World that made all its extras. The tool manifest's detail template is not
// in the registry, since its id is a surface's and not the World's.
func TestWorldTemplates_EveryRegisteredTemplate_Binds(t *testing.T) {
	world := madeWorld()

	variables := 0
	for _, template := range registeredTemplates(t) {
		for _, variable := range harness.TemplateVariables(template) {
			variables++
			t.Run(template+" "+variable, func(t *testing.T) {
				value, bound, reason := world.BindTemplate(template, variable)
				if !bound || value == nil || reason != "" {
					t.Errorf("BindTemplate() = (%v, %t, %q), want a value", value, bound, reason)
				}
			})
		}
	}
	if variables == 0 {
		t.Fatal("no registered template has a variable, so nothing was held to a binding")
	}
}

// TestWorldTemplates_WithoutExtras_NamesWhatIsMissing checks the other side:
// a World that made none of its extras leaves exactly the templates that name
// one unbound, each with a reason naming the object.
func TestWorldTemplates_WithoutExtras_NamesWhatIsMissing(t *testing.T) {
	world := extrasWorld()
	world.Label, world.Milestone = Label{ID: 55}, Milestone{ID: 66, IID: 4}

	var unbound []string
	for _, template := range registeredTemplates(t) {
		for _, variable := range harness.TemplateVariables(template) {
			if _, bound, reason := world.BindTemplate(template, variable); !bound {
				unbound = append(unbound, template)
				if !strings.Contains(reason, "the World holds no ") {
					t.Errorf("%s %s is unbound for %q, want the object it lacks named", template, variable, reason)
				}
			}
		}
	}
	want := []string{
		"gitlab://group/{group_id}/label/{label_id}",
		"gitlab://group/{group_id}/milestone/{milestone_iid}",
		"gitlab://project/{project_id}/board/{board_id}",
		"gitlab://project/{project_id}/deploy_key/{deploy_key_id}",
		"gitlab://project/{project_id}/deployment/{deployment_id}",
		"gitlab://project/{project_id}/environment/{environment_id}",
		"gitlab://project/{project_id}/feature_flag/{name}",
		"gitlab://project/{project_id}/job/{job_id}",
		"gitlab://project/{project_id}/pipeline/{pipeline_id}",
		"gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs",
		"gitlab://project/{project_id}/release/{tag_name}",
		"gitlab://project/{project_id}/snippet/{snippet_id}",
		"gitlab://project/{project_id}/tag/{tag_name}",
		"gitlab://project/{project_id}/wiki/{slug}",
		"gitlab://snippet/{snippet_id}",
	}
	if !slices.Equal(unbound, want) {
		t.Errorf("unbound without extras:\n  %s\nwant:\n  %s", strings.Join(unbound, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestWorldTemplates_TableNamesOnlyRegisteredTemplates is the drift guard on
// the per-template bindings: a template renamed in the server, or a variable
// renamed in one, would otherwise leave an entry here that binds nothing and
// the template it meant falling back to a binding for another object.
func TestWorldTemplates_TableNamesOnlyRegisteredTemplates(t *testing.T) {
	registered := registeredTemplates(t)
	for _, template := range slices.Sorted(maps.Keys(templateBindings)) {
		t.Run(template, func(t *testing.T) {
			if !slices.Contains(registered, template) {
				t.Fatalf("the server registers no template %s", template)
			}
			for variable := range templateBindings[template] {
				if !slices.Contains(harness.TemplateVariables(template), variable) {
					t.Errorf("the template has no variable %s", variable)
				}
			}
		})
	}
}

// TestWorldTemplates_ScopedNames_BindTheirTemplatesObject checks the names
// that mean more than one object: each template gets its own object, and a
// name bound for one template stays out of the plain bindings the read
// sweeps take.
func TestWorldTemplates_ScopedNames_BindTheirTemplatesObject(t *testing.T) {
	world := madeWorld()
	cases := []struct {
		template, variable string
		want               any
	}{
		{template: "gitlab://group/{group_id}/label/{label_id}", variable: "label_id", want: stubGroupLabelID},
		{template: "gitlab://project/{project_id}/label/{label_id}", variable: "label_id", want: int64(55)},
		{template: "gitlab://group/{group_id}/milestone/{milestone_iid}", variable: "milestone_iid", want: stubGroupMilestoneIn},
		{template: "gitlab://project/{project_id}/milestone/{milestone_iid}", variable: "milestone_iid", want: int64(4)},
		{template: "gitlab://snippet/{snippet_id}", variable: "snippet_id", want: stubSnippetID},
		{template: "gitlab://project/{project_id}/snippet/{snippet_id}", variable: "snippet_id", want: stubProjectSnippetID},
		{template: "gitlab://project/{project_id}/board/{board_id}", variable: "board_id", want: stubBoardID},
		{template: "gitlab://project/{project_id}/feature_flag/{name}", variable: "name", want: stubFlagName},
		{template: "gitlab://project/{project_id}/wiki/{slug}", variable: "slug", want: stubWikiSlug},
		{template: "gitlab://project/{project_id}/file/{ref}/{+path}", variable: "path", want: worldReadmePath},
		{template: "gitlab://project/{project_id}/file/{ref}/{+path}", variable: "ref", want: "main"},
		{template: "gitlab://project/{project_id}/release/{tag_name}", variable: "tag_name", want: "world-tag-run"},
		{template: "gitlab://project/{project_id}/branch/{branch}", variable: "branch", want: "feature/world"},
	}
	for _, testCase := range cases {
		t.Run(testCase.template+" "+testCase.variable, func(t *testing.T) {
			value, bound, _ := world.BindTemplate(testCase.template, testCase.variable)
			if !bound || value != testCase.want {
				t.Errorf("BindTemplate() = (%v, %t), want %v", value, bound, testCase.want)
			}
		})
	}
	for _, scoped := range []string{"board_id", "name", "slug", "path", "snippet_id"} {
		t.Run("plain "+scoped, func(t *testing.T) {
			if value, bound := world.Bind(scoped); bound {
				t.Errorf("Bind(%s) = %v, want no plain binding for a name that means several objects", scoped, value)
			}
		})
	}
}

// TestWorldTemplates_UnmadeObject_NamesWhy checks the reasons a sweep logs: an
// extra the build could not make is named with GitLab's own reason, whether
// the template binds it alone or by its plain name, and a name the World never
// carries says so.
func TestWorldTemplates_UnmadeObject_NamesWhy(t *testing.T) {
	world := extrasWorld()
	world.unbound = map[string]string{
		worldExtraWikiPage: "403 Forbidden",
		worldExtraPipeline: "pipeline 77 did not settle",
	}
	cases := []struct {
		name, template, variable string
		want                     []string
	}{
		{
			name: "scoped", template: "gitlab://project/{project_id}/wiki/{slug}", variable: "slug",
			want: []string{"the World's wiki page was not made (403 Forbidden)", "no binding for slug"},
		},
		{
			name: "plain", template: "gitlab://project/{project_id}/job/{job_id}", variable: "job_id",
			want: []string{"the World's pipeline was not made (pipeline 77 did not settle)", "no binding for job_id"},
		},
		{
			name: "never attempted", template: "gitlab://project/{project_id}/tag/{tag_name}", variable: "tag_name",
			want: []string{"the World holds no tag", "no binding for tag_name"},
		},
		{
			name: "never carried", template: "gitlab://project/{project_id}/thing/{thing_id}", variable: "thing_id",
			want: []string{"the World has no binding for thing_id"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, bound, reason := world.BindTemplate(testCase.template, testCase.variable)
			if bound || value != nil {
				t.Fatalf("BindTemplate() bound %v, want nothing", value)
			}
			for _, part := range testCase.want {
				if !strings.Contains(reason, part) {
					t.Errorf("reason = %q, want it to say %q", reason, part)
				}
			}
		})
	}
}

// TestWorldBindParam_UnboundName_NamesWhy checks the reason the read and
// preview sweeps log beside an action they could not bind: an extra the build
// could not make is named with GitLab's own reason, one a World never
// attempted says it holds none, and a name no binding carries says so, while
// a bound name comes back with its value and no reason.
func TestWorldBindParam_UnboundName_NamesWhy(t *testing.T) {
	refused := extrasWorld()
	refused.unbound = map[string]string{worldExtraPipeline: "pipeline 77 did not settle"}
	cases := []struct {
		name, param string
		world       *World
		want        string
	}{
		{
			name: "extra not made", param: "pipeline_id", world: refused,
			want: "the World's pipeline was not made (pipeline 77 did not settle), so it has no binding for pipeline_id",
		},
		{
			name: "extra never attempted", param: "pipeline_id", world: extrasWorld(),
			want: "the World holds no pipeline, so it has no binding for pipeline_id",
		},
		{
			name: "never carried", param: "thing_id", world: extrasWorld(),
			want: "the World has no binding for thing_id",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, bound, reason := testCase.world.BindParam(testCase.param)
			if bound || value != nil {
				t.Fatalf("BindParam(%s) bound %v, want nothing", testCase.param, value)
			}
			if reason != testCase.want {
				t.Errorf("BindParam(%s) reason = %q, want %q", testCase.param, reason, testCase.want)
			}
		})
	}

	boundCases := []struct {
		name, param string
		want        any
	}{
		{name: "core", param: "project_id", want: int64(2)},
		{name: "made extra", param: "pipeline_id", want: stubPipelineID},
	}
	made := madeWorld()
	for _, testCase := range boundCases {
		t.Run(testCase.name, func(t *testing.T) {
			value, bound, reason := made.BindParam(testCase.param)
			if !bound || value != testCase.want || reason != "" {
				t.Errorf("BindParam(%s) = (%v, %t, %q), want (%v, true, \"\")", testCase.param, value, bound, reason, testCase.want)
			}
		})
	}
}

// stubExtraState puts every extra's read-back into the stub, as GitLab would
// answer it, and returns the digest fields those answers produce.
func stubExtraState(stub *stubGitLab) map[string]string {
	stub.answers(http.MethodGet, "/api/v4/projects/2/repository/tags/world-tag-run", stubOK(map[string]any{"target": "tag-sha", "message": tagMessage}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/releases/world-tag-run", stubOK(map[string]any{"name": "Release world-tag-run", "description": "notes"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/environments/5", stubOK(map[string]any{"name": "world-environment-run", "external_url": "", "state": "available"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/deployments/6", stubOK(map[string]any{"ref": "feature/world", "sha": "feature-sha", "status": "running"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/feature_flags/"+stubFlagName, stubOK(map[string]any{"active": true, "description": "flag"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/deploy_keys/7", stubOK(map[string]any{"title": "key", "can_push": false}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/boards/8", stubOK(map[string]any{"name": "board"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/snippets/9", stubOK(map[string]any{"title": "ps", "description": "d", "visibility": "private"}))
	stub.answers(http.MethodGet, "/api/v4/snippets/10", stubOK(map[string]any{"title": "s", "visibility": "private"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/wikis/"+stubWikiSlug, stubOK(map[string]any{"title": "wiki", "content": "# page"}))
	stub.answers(http.MethodGet, "/api/v4/groups/1/labels/11", stubOK(map[string]any{"name": "gl", "color": "#428BCA", "description": "gd"}))
	stub.answers(http.MethodGet, "/api/v4/groups/1/milestones/12", stubOK(map[string]any{"title": "gm", "state": "active", "description": "md"}))
	stub.answers(http.MethodGet, "/api/v4/projects/2/jobs/88", stubOK(map[string]any{"name": worldJobName, "ref": "main", "status": "canceled"}))
	stub.configure(func() {
		stub.state["/api/v4/projects/2/pipelines/77"] = map[string]any{"id": stubPipelineID, "ref": "main", "sha": "ci-sha", "status": "canceled"}
	})
	return map[string]string{
		"tag.target": "tag-sha", "tag.message": tagMessage,
		"release.name": "Release world-tag-run", "release.description": "notes",
		"environment.name": "world-environment-run", "environment.external_url": "",
		"deployment.ref": "feature/world", "deployment.sha": "feature-sha",
		"feature_flag.active": "true", "feature_flag.description": "flag",
		"deploy_key.title": "key", "deploy_key.can_push": "false",
		"board.name":            "board",
		"project_snippet.title": "ps", "project_snippet.description": "d", "project_snippet.visibility": "private",
		"snippet.title": "s", "snippet.visibility": "private",
		"wiki.title": "wiki", "wiki.content": "# page",
		"group_label.name": "gl", "group_label.color": "#428BCA", "group_label.description": "gd",
		"group_milestone.title": "gm", "group_milestone.state": "active", "group_milestone.description": "md",
		"pipeline.ref": "main", "pipeline.sha": "ci-sha", "job.name": worldJobName, "job.ref": "main",
	}
}

// madeStubWorld is madeWorld with its core put into the stub too, so
// worldState can read the whole of it.
func madeStubWorld(stub *stubGitLab) *World {
	core := stubWorld(stub)
	world := madeWorld()
	world.Group, world.Project, world.Branch = core.Group, core.Project, core.Branch
	world.MergeRequest, world.Issue, world.Label, world.Milestone = core.MergeRequest, core.Issue, core.Label, core.Milestone
	return world
}

// TestWorldState_Extras_ReadEveryMadeOneAndNoStatus checks that the digest
// takes in every extra the World made, field by field, and none of the
// statuses GitLab moves by itself.
func TestWorldState_Extras_ReadEveryMadeOneAndNoStatus(t *testing.T) {
	stub, client := newStubGitLab(t)
	world := madeStubWorld(stub)
	want := stubExtraState(stub)

	state, err := worldState(t.Context(), client, world)
	if err != nil {
		t.Fatalf("worldState() error = %v", err)
	}
	for field, value := range want {
		t.Run(field, func(t *testing.T) {
			if got, read := state[field]; !read || got != value {
				t.Errorf("state[%s] = %q (read %t), want %q", field, got, read, value)
			}
		})
	}
	// The stub answers every one of these, so a reader that took one in would
	// be caught here.
	for _, status := range []string{"pipeline.status", "job.status", "deployment.status", "environment.state"} {
		t.Run("no "+status, func(t *testing.T) {
			if value, read := state[status]; read {
				t.Errorf("the digest reads %s = %q, a status GitLab moves by itself", status, value)
			}
		})
	}
}

// TestWorldState_ExtraNotMade_IsNotRead checks that an extra the World could
// not make contributes nothing and asks GitLab for nothing: there is no object
// to read, and a read of one would fail the digest of a World that is intact.
func TestWorldState_ExtraNotMade_IsNotRead(t *testing.T) {
	stub, client := newStubGitLab(t)
	world := stubWorld(stub)

	state, err := worldState(t.Context(), client, world)
	if err != nil {
		t.Fatalf("worldState() error = %v", err)
	}
	for field := range state {
		prefix, _, _ := strings.Cut(field, ".")
		if !slices.Contains([]string{"group", "project", "branch", "merge_request", "issue", "label", "milestone"}, prefix) {
			t.Errorf("the digest of a World with no extras reads %s", field)
		}
	}
	if sent := sentPaths(stub); len(sent) != 0 {
		t.Errorf("reading a World with no extras asked for %v", sent)
	}
}

// TestWorldState_ExtraGone_FailsNamingIt checks the property the extras are
// in the digest for: a preview that ran anyway and deleted one makes the
// teardown's reading fail and say which object it could not read.
func TestWorldState_ExtraGone_FailsNamingIt(t *testing.T) {
	cases := []struct {
		extra, method, path, want string
	}{
		{extra: "tag", method: http.MethodGet, path: "/api/v4/projects/2/repository/tags/world-tag-run", want: `reading tag "world-tag-run"`},
		{extra: "release", method: http.MethodGet, path: "/api/v4/projects/2/releases/world-tag-run", want: `reading the release on "world-tag-run"`},
		{extra: "environment", method: http.MethodGet, path: "/api/v4/projects/2/environments/5", want: "reading environment 5"},
		{extra: "deployment", method: http.MethodGet, path: "/api/v4/projects/2/deployments/6", want: "reading deployment 6"},
		{extra: "feature flag", method: http.MethodGet, path: "/api/v4/projects/2/feature_flags/" + stubFlagName, want: `reading feature flag "` + stubFlagName},
		{extra: "deploy key", method: http.MethodGet, path: "/api/v4/projects/2/deploy_keys/7", want: "reading deploy key 7"},
		{extra: "board", method: http.MethodGet, path: "/api/v4/projects/2/boards/8", want: "reading board 8"},
		{extra: "project snippet", method: http.MethodGet, path: "/api/v4/projects/2/snippets/9", want: "reading snippet 9 of project 2"},
		{extra: "personal snippet", method: http.MethodGet, path: "/api/v4/snippets/10", want: "reading snippet 10:"},
		{extra: "wiki page", method: http.MethodGet, path: "/api/v4/projects/2/wikis/" + stubWikiSlug, want: `reading wiki page "` + stubWikiSlug},
		{extra: "group label", method: http.MethodGet, path: "/api/v4/groups/1/labels/11", want: "reading label 11 of group 1"},
		{extra: "group milestone", method: http.MethodGet, path: "/api/v4/groups/1/milestones/12", want: "reading milestone 12 of group 1"},
		{extra: "job", method: http.MethodGet, path: "/api/v4/projects/2/jobs/88", want: "reading job 88"},
	}
	for _, testCase := range cases {
		t.Run(testCase.extra, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			world := madeStubWorld(stub)
			stubExtraState(stub)
			stub.answers(testCase.method, testCase.path, stubRefusal(http.StatusNotFound, "404 Not Found"))

			_, err := worldState(t.Context(), client, world)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("worldState() error = %v, want it to name %q", err, testCase.want)
			}
		})
	}
	t.Run("pipeline", func(t *testing.T) {
		stub, client := newStubGitLab(t)
		world := madeStubWorld(stub)
		stubExtraState(stub)
		stub.configure(func() {
			delete(stub.state, "/api/v4/projects/2/pipelines/77")
			stub.pipelineFailures = 1
		})

		_, err := worldState(t.Context(), client, world)
		if err == nil || !strings.Contains(err.Error(), "reading pipeline 77") {
			t.Errorf("worldState() error = %v, want it to name the pipeline", err)
		}
	})
}

// TestWorldExtras_Table_MakesEachAfterWhatItStandsOn pins the order and the
// dependencies of the extras, since an extra made before the one it stands on
// would always be skipped for want of it.
func TestWorldExtras_Table_MakesEachAfterWhatItStandsOn(t *testing.T) {
	var names []string
	for _, extra := range worldExtras {
		if extra.needs != "" && !slices.Contains(names, extra.needs) {
			t.Errorf("the %s is made before the %s it stands on", extra.name, extra.needs)
		}
		names = append(names, extra.name)
	}
	want := []string{
		worldExtraTag, worldExtraRelease, worldExtraEnvironment, worldExtraDeployment, worldExtraFeatureFlag,
		worldExtraDeployKey, worldExtraBoard, worldExtraProjectSnippet, worldExtraSnippet, worldExtraWikiPage,
		worldExtraGroupLabel, worldExtraGroupMilestone, worldExtraPipeline,
	}
	if !slices.Equal(names, want) {
		t.Errorf("extras = %v, want %v", names, want)
	}
	needs := map[string]string{}
	for _, extra := range worldExtras {
		if extra.needs != "" {
			needs[extra.name] = extra.needs
		}
	}
	if want := map[string]string{worldExtraRelease: worldExtraTag, worldExtraDeployment: worldExtraEnvironment}; !maps.Equal(needs, want) {
		t.Errorf("dependencies = %v, want %v", needs, want)
	}
}

// TestMakeExtra_Budget_BoundsTheCreate checks that each extra runs under a
// context of its own with the extra budget as its deadline, so one that hangs
// cannot hold up the rest past it.
func TestMakeExtra_Budget_BoundsTheCreate(t *testing.T) {
	world := extrasWorld()
	world.unbound = map[string]string{}
	var remaining time.Duration
	world.makeExtra(context.Background(), t, nil, extrasRunID, worldExtra{
		name: "probe",
		create: func(ctx context.Context, _ testing.TB, _ *gitlabclient.Client, _ string, _ *World) error {
			deadline, _ := ctx.Deadline()
			remaining = time.Until(deadline)
			return nil
		},
	})
	if remaining <= worldExtraBudget-time.Minute || remaining > worldExtraBudget {
		t.Errorf("the create ran with %s left, want the extra budget of %s", remaining, worldExtraBudget)
	}
	if len(world.unbound) != 0 {
		t.Errorf("an extra that was made is recorded unbound: %v", world.unbound)
	}
}

// TestWorldDescription_NamesTheRun pins the description the World's project
// and extras carry, which is how a reader of an instance tells the World's
// objects from a scenario's.
func TestWorldDescription_NamesTheRun(t *testing.T) {
	if got, want := worldDescription("20260923t101500z"), "The shared read-only World of run 20260923t101500z"; got != want {
		t.Errorf("worldDescription() = %q, want %q", got, want)
	}
}

// TestIsWorldJob_PicksTheJobByName checks the predicate the job wait uses.
func TestIsWorldJob_PicksTheJobByName(t *testing.T) {
	cases := map[string]bool{worldJobName: true, "fast-pass": false, "": false}
	for name, want := range cases {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			if got := isWorldJob(&gl.Job{Name: name}); got != want {
				t.Errorf("isWorldJob(%q) = %t, want %t", name, got, want)
			}
		})
	}
}
