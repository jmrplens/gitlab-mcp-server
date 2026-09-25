//go:build e2e

// world_extras.go builds the objects the World carries beyond its core, each
// on its own and each allowed to fail.
//
// The core (the group, the project, a branch, a commit, a merge request, an
// issue, a label and a milestone) is what every read test stands on, so a core
// object the World cannot make fails the run. The extras are what a resource
// template or a read action names and nothing else needs: a tag and its
// release, an environment and a deployment into it, a feature flag, a deploy
// key, a project board, a project and a personal snippet, a wiki page, a group
// label and milestone, and a canceled pipeline with its job. An instance can
// legitimately lack any of them (a project whose wiki is disabled answers
// 403), so each is made under its own bounded context with the builders' retry
// policy, and one it cannot make is recorded with its reason and left unbound.
// A sweep then names that reason beside the template it could not bind, or
// beside the action when the name is a plain binding ([plainExtraBindings]) or
// one the action's domain binds ([domainBindings]), and nothing else is
// affected. A template whose first read stands on an extra none of its
// variables names ([templateNeeds]) is left unbound with that extra's reason
// too. An action naming a name only a template binds
// (a wiki's slug, board_id, a feature flag's name, snippet_id) is not bound
// from the World at all, whatever became of the extra, and its line says the
// World has no binding for the name.
//
// None of them goes through a New* builder, because those register their
// deletion on the calling test's ledger, and the first test to end would
// delete a World object every later test stands on. Every extra goes with the
// World's project or group except the personal snippet, which belongs to the
// user and which the World's teardown deletes itself. A run killed hard enough
// to skip that teardown leaks it, since the orphan sweep reaches projects,
// groups and users only (issue 913).

package fixture

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// The extras the World makes, named the way the log and an unbound reason
// name them.
const (
	worldExtraTag            = "tag"
	worldExtraRelease        = "release"
	worldExtraEnvironment    = "environment"
	worldExtraDeployment     = "deployment"
	worldExtraFeatureFlag    = "feature flag"
	worldExtraDeployKey      = "deploy key"
	worldExtraBoard          = "project board"
	worldExtraProjectSnippet = "project snippet"
	worldExtraSnippet        = "personal snippet"
	worldExtraWikiPage       = "wiki page"
	worldExtraGroupLabel     = "group label"
	worldExtraGroupMilestone = "group milestone"
	worldExtraPipeline       = "pipeline"
)

// The prefixes the extras are named under, each scoped to the run through
// worldName like the rest of the World.
const (
	worldTagPrefix            = "world-tag"
	worldEnvironmentPrefix    = "world-environment"
	worldFlagPrefix           = "world-flag"
	worldDeployKeyPrefix      = "world-deploy-key"
	worldBoardPrefix          = "world-board"
	worldProjectSnippetPrefix = "world-project-snippet"
	worldSnippetPrefix        = "world-snippet"
	worldWikiPrefix           = "world-wiki"
	worldGroupLabelPrefix     = "world-group-label"
	worldGroupMilestonePrefix = "world-group-milestone"
)

// worldReadmePath is the file the World's project was created with, and what
// the file resource template and the file_path parameter are bound to. It
// sits on the default branch because a file read with no ref reads the
// default branch, where the World's own file on feature/world is not. The
// file resource alone could reach that one now, since a ref carrying a slash
// arrives percent-encoded and is decoded once (issue 912), but one path has
// to serve both bindings.
const worldReadmePath = "README.md"

// worldExtraBudget bounds the making of one extra, its retries included. It is
// sized for the slowest of them, the pipeline, which waits for a job and then
// for a terminal status inside the same budget.
const worldExtraBudget = 5 * time.Minute

// The World pipeline's two waits: for GitLab to create its job, and for the
// pipeline to reach a terminal status once canceled. Variables so the
// package's own tests can wait less than a real GitLab takes.
var (
	worldJobWait        = 2 * time.Minute
	worldPipelineSettle = 2 * time.Minute
)

// worldPublicKey mints the World deploy key's public half. A variable so a
// test can make it fail, which generating a key never does.
var worldPublicKey = SSHPublicKey

// worldJobName and worldJobTag name the World pipeline's one job and the
// runner tag it asks for. No runner the suite registers carries the tag, so
// on an instance with a runner and on one without, the job waits until the
// World cancels it, and both runtimes end with the same canceled pipeline.
const (
	worldJobName = "world-job"
	worldJobTag  = "e2e-world-no-runner"
)

// worldCIYAML is the configuration the World commits to its default branch.
//
// The workflow rule admits a pipeline only when the pipelines API asked for
// one, so neither the commit that adds the file nor any later push runs
// anything: the one pipeline the World has is the one it creates.
const worldCIYAML = `workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "api"

stages:
  - test

` + worldJobName + `:
  stage: test
  script:
    - echo "the World's job, which no runner takes"
  tags:
    - ` + worldJobTag + `
`

// worldCIMessage is the message of the commit that adds worldCIYAML.
const worldCIMessage = "ci: add the World's pipeline configuration"

// worldExtra is one object the World makes beyond its core.
type worldExtra struct {
	// name is what the log and an unbound reason call it.
	name string
	// needs names the extra this one stands on, a release on its tag and a
	// deployment on its environment. It is not attempted when that one was
	// not made.
	needs string
	// create makes it and fills its field of the World in.
	create func(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error
	// made reports whether the World holds it.
	made func(*World) bool
	// read records the fields of it a test could change, for the digest.
	read func(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error
}

// worldExtras are the extras in the order they are made, which puts each after
// the one it stands on.
var worldExtras = []worldExtra{
	{name: worldExtraTag, create: createWorldTag, made: func(w *World) bool { return w.Tag.Name != "" }, read: readTagState},
	{
		name: worldExtraRelease, needs: worldExtraTag, create: createWorldRelease,
		made: func(w *World) bool { return w.Release.TagName != "" }, read: readReleaseState,
	},
	{
		name: worldExtraEnvironment, create: createWorldEnvironment,
		made: func(w *World) bool { return w.Environment.ID != 0 }, read: readEnvironmentState,
	},
	{
		name: worldExtraDeployment, needs: worldExtraEnvironment, create: createWorldDeployment,
		made: func(w *World) bool { return w.Deployment.ID != 0 }, read: readDeploymentState,
	},
	{
		name: worldExtraFeatureFlag, create: createWorldFeatureFlag,
		made: func(w *World) bool { return w.FeatureFlag.Name != "" }, read: readFeatureFlagState,
	},
	{
		name: worldExtraDeployKey, create: createWorldDeployKey,
		made: func(w *World) bool { return w.DeployKey.ID != 0 }, read: readDeployKeyState,
	},
	{name: worldExtraBoard, create: createWorldBoard, made: func(w *World) bool { return w.Board.ID != 0 }, read: readBoardState},
	{
		name: worldExtraProjectSnippet, create: createWorldProjectSnippet,
		made: func(w *World) bool { return w.ProjectSnippet.ID != 0 }, read: readProjectSnippetState,
	},
	{name: worldExtraSnippet, create: createWorldSnippet, made: func(w *World) bool { return w.Snippet.ID != 0 }, read: readSnippetState},
	{
		name: worldExtraWikiPage, create: createWorldWikiPage,
		made: func(w *World) bool { return w.WikiPage.Slug != "" }, read: readWikiPageState,
	},
	{
		name: worldExtraGroupLabel, create: createWorldGroupLabel,
		made: func(w *World) bool { return w.GroupLabel.ID != 0 }, read: readGroupLabelState,
	},
	{
		name: worldExtraGroupMilestone, create: createWorldGroupMilestone,
		made: func(w *World) bool { return w.GroupMilestone.ID != 0 }, read: readGroupMilestoneState,
	},
	{
		name: worldExtraPipeline, create: createWorldPipeline,
		made: func(w *World) bool { return w.Pipeline.ID != 0 }, read: readPipelineState,
	},
}

// buildWorldExtras makes every extra in turn, recording the ones it could not
// make rather than failing anything.
//
// It takes a context and a test rather than an Env, because the extras are
// made on behalf of whichever test asked for the World first: a failure is
// that test's log line and never its verdict.
func buildWorldExtras(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) {
	tb.Helper()

	world.unbound = map[string]string{}
	for _, extra := range worldExtras {
		world.makeExtra(ctx, tb, client, runID, extra)
	}
}

// makeExtra makes one extra under its own bounded context, or records why it
// could not.
func (w *World) makeExtra(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, extra worldExtra) {
	tb.Helper()

	if reason, failed := w.unbound[extra.needs]; failed {
		w.leaveUnbound(tb, extra.name, fmt.Sprintf("it stands on the World's %s, which was not made: %s", extra.needs, reason))
		return
	}
	extraCtx, cancel := context.WithTimeout(ctx, worldExtraBudget)
	defer cancel()
	if err := extra.create(extraCtx, tb, client, runID, w); err != nil {
		w.leaveUnbound(tb, extra.name, err.Error())
	}
}

// leaveUnbound records that the World has no such extra, and why, where the
// sweeps and the log can both read it.
func (w *World) leaveUnbound(tb testing.TB, name, reason string) {
	tb.Helper()
	w.unbound[name] = reason
	tb.Logf("the World has no %s, and what names one is left unbound: %s", name, reason)
}

// keep runs one create under the builders' retry policy and hands what it made
// to set, so a World field is filled in only by a create that succeeded.
func keep[O any](ctx context.Context, tb testing.TB, label string, create func() (O, error), set func(O)) error {
	tb.Helper()

	made, err := retryWhenIn(ctx, tb, label, createRetries, IsRetryable, create)
	if err != nil {
		return err
	}
	set(made)
	return nil
}

// worldDescription is what the World's project and the extras that carry a
// description are described as.
func worldDescription(runID string) string {
	return "The shared read-only World of run " + runID
}

// createWorldTag tags the default branch.
func createWorldTag(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World tag", func() (Tag, error) {
		return createTag(ctx, client, world.Project.ID, worldName(runID, worldTagPrefix), world.Project.DefaultBranch)
	}, func(tag Tag) { world.Tag = tag })
}

// createWorldRelease makes a release on the World tag, so tag_name names the
// tag and its release alike.
func createWorldRelease(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World release", func() (Release, error) {
		return createRelease(ctx, client, world.Project.ID, world.Tag.Name, world.Project.DefaultBranch, worldDescription(runID))
	}, func(release Release) { world.Release = release })
}

// createWorldEnvironment makes the environment the World deploys into.
func createWorldEnvironment(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World environment", func() (Environment, error) {
		return createEnvironment(ctx, client, world.Project.ID, worldName(runID, worldEnvironmentPrefix))
	}, func(environment Environment) { world.Environment = environment })
}

// createWorldDeployment deploys the World's own commit, from the branch that
// carries it, into the World environment.
func createWorldDeployment(ctx context.Context, tb testing.TB, client *gitlabclient.Client, _ string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World deployment", func() (Deployment, error) {
		return createDeployment(ctx, client, world.Project.ID, world.Environment, world.Branch.Name, world.Commit.SHA)
	}, func(deployment Deployment) { world.Deployment = deployment })
}

// createWorldFeatureFlag makes an active feature flag on the project.
func createWorldFeatureFlag(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World feature flag", func() (ProjectFeatureFlag, error) {
		return createProjectFeatureFlag(ctx, client, world.Project.ID, featureFlagNameFrom(worldName(runID, worldFlagPrefix)))
	}, func(flag ProjectFeatureFlag) { world.FeatureFlag = flag })
}

// createWorldDeployKey adds a freshly generated, read-only deploy key.
func createWorldDeployKey(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	publicKey, err := worldPublicKey()
	if err != nil {
		return err
	}
	return keep(ctx, tb, "create the World deploy key", func() (DeployKey, error) {
		return createDeployKey(ctx, client, world.Project.ID, worldName(runID, worldDeployKeyPrefix), publicKey)
	}, func(key DeployKey) { world.DeployKey = key })
}

// createWorldBoard makes an issue board in the project, which on every edition
// may have one.
func createWorldBoard(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World project board", func() (ProjectBoard, error) {
		return createProjectBoard(ctx, client, world.Project.Path, worldName(runID, worldBoardPrefix))
	}, func(board ProjectBoard) { world.Board = board })
}

// createWorldProjectSnippet makes a snippet inside the project.
func createWorldProjectSnippet(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World project snippet", func() (ProjectSnippet, error) {
		return createProjectSnippet(ctx, client, world.Project.ID, worldName(runID, worldProjectSnippetPrefix), worldDescription(runID))
	}, func(snippet ProjectSnippet) { world.ProjectSnippet = snippet })
}

// createWorldSnippet makes a personal snippet of the run's user.
func createWorldSnippet(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World personal snippet", func() (Snippet, error) {
		return createPersonalSnippet(ctx, client, worldName(runID, worldSnippetPrefix), worldDescription(runID))
	}, func(snippet Snippet) { world.Snippet = snippet })
}

// createWorldWikiPage makes a page in the project's wiki.
func createWorldWikiPage(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World wiki page", func() (WikiPage, error) {
		return createWikiPage(ctx, client, world.Project.ID, worldName(runID, worldWikiPrefix))
	}, func(page WikiPage) { world.WikiPage = page })
}

// createWorldGroupLabel makes a label of the World's group, which is what the
// group label template names rather than the project's.
func createWorldGroupLabel(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	return keep(ctx, tb, "create the World group label", func() (GroupLabel, error) {
		return createGroupLabel(ctx, client, world.Group.ID, worldName(runID, worldGroupLabelPrefix), worldDescription(runID))
	}, func(label GroupLabel) { world.GroupLabel = label })
}

// createWorldGroupMilestone makes a milestone of the World's group, running
// from today.
func createWorldGroupMilestone(ctx context.Context, tb testing.TB, client *gitlabclient.Client, runID string, world *World) error {
	tb.Helper()
	start := time.Now().UTC()
	return keep(ctx, tb, "create the World group milestone", func() (GroupMilestone, error) {
		return createGroupMilestone(ctx, client, world.Group.ID, worldName(runID, worldGroupMilestonePrefix), worldDescription(runID), start)
	}, func(milestone GroupMilestone) { world.GroupMilestone = milestone })
}

// createWorldPipeline gives the World a pipeline and a job that nothing runs:
// it commits worldCIYAML to the default branch, asks the pipelines API for one
// pipeline, waits for the job, cancels the pipeline and waits for it to
// settle, and fills the two in only once it has.
//
// Settled is the condition because binding them puts pipeline.wait and
// job.wait into the reads sweep, and on a pipeline that has not finished each
// of those blocks for its own default wait, on each of three surfaces.
// Canceling needs no runner, which is what makes the outcome the same on an
// instance with one and on one without. A job that never appears, or a cancel
// GitLab refuses, leaves the pipeline pending; either way it goes with the
// project. The project holds it from the moment GitLab created it, settled or
// not, which is recorded apart from the two bindings because the latest
// pipeline template reads that pipeline and binds nothing of it.
func createWorldPipeline(ctx context.Context, tb testing.TB, client *gitlabclient.Client, _ string, world *World) error {
	tb.Helper()
	project := world.Project
	_, err := retryWhenIn(ctx, tb, "commit the World's pipeline configuration", createRetries, IsRetryable, func() (Commit, error) {
		return commitWorldCIFile(ctx, client, project)
	})
	if err != nil {
		return err
	}
	pipeline, err := retryWhenIn(ctx, tb, "create the World pipeline", createRetries, IsRetryable, func() (Pipeline, error) {
		return createPipeline(ctx, client, project.ID, project.DefaultBranch)
	})
	if err != nil {
		return err
	}
	world.holdsPipeline = true
	jobID, err := awaitPipelineJob(ctx, client, project.ID, pipeline.ID, worldJobWait, isWorldJob)
	if err != nil {
		return fmt.Errorf("pipeline %d grew no %s job within %s: %w", pipeline.ID, worldJobName, worldJobWait, err)
	}
	if cancelErr := cancelPipeline(ctx, client, project.ID, pipeline.ID); cancelErr != nil {
		return cancelErr
	}
	status, err := waitForPipelineStatus(ctx, client, project.ID, pipeline.ID, worldPipelineSettle)
	if err != nil {
		return fmt.Errorf("pipeline %d did not settle within %s of its cancel (last status %s): %w",
			pipeline.ID, worldPipelineSettle, status, err)
	}
	pipeline.Status = status
	world.Pipeline = pipeline
	world.JobID = jobID
	return nil
}

// commitWorldCIFile adds worldCIYAML to the project's default branch.
func commitWorldCIFile(ctx context.Context, client *gitlabclient.Client, project Project) (Commit, error) {
	created, _, err := client.GL().Commits.CreateCommit(project.ID, &gl.CreateCommitOptions{
		Branch:        new(project.DefaultBranch),
		CommitMessage: new(worldCIMessage),
		Actions: []*gl.CommitActionOptions{
			{Action: new(gl.FileCreate), FilePath: new(CIFilePath), Content: new(worldCIYAML)},
		},
	}, gl.WithContext(ctx))
	if err != nil {
		return Commit{}, err
	}
	return Commit{SHA: created.ID, ShortID: created.ShortID, Branch: project.DefaultBranch, FilePath: CIFilePath}, nil
}

// isWorldJob picks the World pipeline's one job out of its listing.
func isWorldJob(job *gl.Job) bool {
	return job.Name == worldJobName
}

// The readers below record, for the digest, the fields of each extra a test
// could change. None of them reads a status GitLab moves by itself (a
// pipeline's, a job's, a deployment's, an environment's), because the digest
// compares a reading taken when the World was built with one taken at the end
// of the run, and a status that settled in between would read as a test's
// write. A read that fails fails the digest, which is what a preview that ran
// anyway and deleted the object produces.

// readTagState records what the tag points at and says.
func readTagState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Tags.GetTag(world.Project.ID, world.Tag.Name, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading tag %q of project %d: %w", world.Tag.Name, world.Project.ID, err)
	}
	state["tag.target"] = got.Target
	state["tag.message"] = got.Message
	return nil
}

// readReleaseState records the release's title and notes.
func readReleaseState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Releases.GetRelease(world.Project.ID, world.Release.TagName, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading the release on %q of project %d: %w", world.Release.TagName, world.Project.ID, err)
	}
	state["release.name"] = got.Name
	state["release.description"] = got.Description
	return nil
}

// readEnvironmentState records the environment's name and address.
func readEnvironmentState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Environments.GetEnvironment(world.Project.ID, world.Environment.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading environment %d of project %d: %w", world.Environment.ID, world.Project.ID, err)
	}
	state["environment.name"] = got.Name
	state["environment.external_url"] = got.ExternalURL
	return nil
}

// readDeploymentState records what the deployment deployed.
func readDeploymentState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Deployments.GetProjectDeployment(world.Project.ID, world.Deployment.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading deployment %d of project %d: %w", world.Deployment.ID, world.Project.ID, err)
	}
	state["deployment.ref"] = got.Ref
	state["deployment.sha"] = got.SHA
	return nil
}

// readFeatureFlagState records whether the flag is on and what it says.
func readFeatureFlagState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().ProjectFeatureFlags.GetProjectFeatureFlag(world.Project.ID, world.FeatureFlag.Name, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading feature flag %q of project %d: %w", world.FeatureFlag.Name, world.Project.ID, err)
	}
	state["feature_flag.active"] = strconv.FormatBool(got.Active)
	state["feature_flag.description"] = got.Description
	return nil
}

// readDeployKeyState records the key's title and whether it may push.
func readDeployKeyState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().DeployKeys.GetDeployKey(world.Project.ID, world.DeployKey.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading deploy key %d of project %d: %w", world.DeployKey.ID, world.Project.ID, err)
	}
	state["deploy_key.title"] = got.Title
	state["deploy_key.can_push"] = strconv.FormatBool(got.CanPush)
	return nil
}

// readBoardState records the board's name.
func readBoardState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Boards.GetIssueBoard(world.Project.ID, world.Board.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading board %d of project %d: %w", world.Board.ID, world.Project.ID, err)
	}
	state["board.name"] = got.Name
	return nil
}

// readProjectSnippetState records the project snippet's title, notes and
// visibility.
func readProjectSnippetState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().ProjectSnippets.GetSnippet(world.Project.ID, world.ProjectSnippet.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading snippet %d of project %d: %w", world.ProjectSnippet.ID, world.Project.ID, err)
	}
	state["project_snippet.title"] = got.Title
	state["project_snippet.description"] = got.Description
	state["project_snippet.visibility"] = got.Visibility
	return nil
}

// readSnippetState records the personal snippet's title and visibility.
func readSnippetState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Snippets.GetSnippet(world.Snippet.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading snippet %d: %w", world.Snippet.ID, err)
	}
	state["snippet.title"] = got.Title
	state["snippet.visibility"] = got.Visibility
	return nil
}

// readWikiPageState records the page's title and content.
func readWikiPageState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().Wikis.GetWikiPage(world.Project.ID, world.WikiPage.Slug, &gl.GetWikiPageOptions{}, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading wiki page %q of project %d: %w", world.WikiPage.Slug, world.Project.ID, err)
	}
	state["wiki.title"] = got.Title
	state["wiki.content"] = got.Content
	return nil
}

// readGroupLabelState records the group label's name, color and description.
func readGroupLabelState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().GroupLabels.GetGroupLabel(world.Group.ID, world.GroupLabel.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading label %d of group %d: %w", world.GroupLabel.ID, world.Group.ID, err)
	}
	state["group_label.name"] = got.Name
	state["group_label.color"] = got.Color
	state["group_label.description"] = got.Description
	return nil
}

// readGroupMilestoneState records the group milestone's title, state and
// description.
func readGroupMilestoneState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	got, _, err := client.GL().GroupMilestones.GetGroupMilestone(world.Group.ID, world.GroupMilestone.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading milestone %d of group %d: %w", world.GroupMilestone.ID, world.Group.ID, err)
	}
	state["group_milestone.title"] = got.Title
	state["group_milestone.state"] = got.State
	state["group_milestone.description"] = got.Description
	return nil
}

// readPipelineState records what the pipeline ran against and which job it
// holds, and never either one's status.
func readPipelineState(ctx context.Context, client *gitlabclient.Client, world *World, state map[string]string) error {
	pipeline, _, err := client.GL().Pipelines.GetPipeline(world.Project.ID, world.Pipeline.ID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading pipeline %d of project %d: %w", world.Pipeline.ID, world.Project.ID, err)
	}
	job, _, err := client.GL().Jobs.GetJob(world.Project.ID, world.JobID, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reading job %d of project %d: %w", world.JobID, world.Project.ID, err)
	}
	state["pipeline.ref"] = pipeline.Ref
	state["pipeline.sha"] = pipeline.SHA
	state["job.name"] = job.Name
	state["job.ref"] = job.Ref
	return nil
}

// worldBinding is how one name binds from the World outside its core
// bindings: the extra the value belongs to, for the reason a missing one
// gives, and the value with whether the World holds it. The extra is empty for
// a value of the World's core, which it always holds.
type worldBinding struct {
	extra string
	value func(*World) (any, bool)
}

// plainExtraBindings are the extras whose parameter name means one object
// across the whole catalog, so they join [World.Bindings] once made and the
// read and preview sweeps bind them by name like the core ones. tag_name is
// among them because the World's release stands on its tag, and the release
// domain's actions, where it names the release rather than the tag, bind it
// through [domainBindings] instead.
var plainExtraBindings = map[string]worldBinding{
	"pipeline_id":    {worldExtraPipeline, func(w *World) (any, bool) { return w.Pipeline.ID, w.Pipeline.ID != 0 }},
	"job_id":         {worldExtraPipeline, func(w *World) (any, bool) { return w.JobID, w.JobID != 0 }},
	"tag_name":       {worldExtraTag, func(w *World) (any, bool) { return w.Tag.Name, w.Tag.Name != "" }},
	"environment_id": {worldExtraEnvironment, func(w *World) (any, bool) { return w.Environment.ID, w.Environment.ID != 0 }},
	"deployment_id":  {worldExtraDeployment, func(w *World) (any, bool) { return w.Deployment.ID, w.Deployment.ID != 0 }},
	"deploy_key_id":  {worldExtraDeployKey, func(w *World) (any, bool) { return w.DeployKey.ID, w.DeployKey.ID != 0 }},
}

// releaseTagBinding is tag_name where it names the release rather than the
// tag: bound only once the release standing on the tag was made, so a tag
// made without its release does not bind a release read that can only fail.
var releaseTagBinding = worldBinding{worldExtraRelease, func(w *World) (any, bool) { return w.Release.TagName, w.Release.TagName != "" }}

// domainBindings are values bound for the actions of one catalog domain only,
// because there the name means another object than the plain binding gives
// it. The release domain's tag_name is the release wherever an action
// addresses one that exists, so it is bound only once the World's release was
// made, and when it was not, release.get, release.link_list, release.update
// and release.delete are logged with the release's reason, as the release
// template is.
//
// What the plain binding would cost differs by sweep. The reads release.get
// and release.link_list would be called on a tag with no release whenever the
// tag was made and the release was not, and the read sweep would count each
// answer among its errors without naming the action or the reason.
// release.update and release.delete are never read: the preview sweep calls
// them in safe mode, which never reaches GitLab, and they bind the release so
// that a preview names a release the World holds, which leaves them unbound
// rather than previewed when the release was not made. release.link_get and
// the release link mutations also need a link id, or links, or a name and a
// URL, that the World never binds, so they stay unbound whatever tag_name
// binds to, and are logged as naming a parameter the World has no binding
// for.
//
// The tag domain's actions keep the plain binding, since the tag is what they
// address, and so do the actions [domainBindingExemptions] names.
var domainBindings = map[string]map[string]worldBinding{
	"release": {"tag_name": releaseTagBinding},
}

// domainBindingExemptions are the actions of a domain in [domainBindings]
// whose parameter does not name the domain's object, and which keep the plain
// binding. release.create's tag_name names no release that exists: it is the
// tag the new release will stand on, one GitLab creates from ref when it does
// not exist, so it binds the World's tag, and the preview sweep previews it,
// whether or not the World's release was made.
var domainBindingExemptions = map[string]struct{}{
	"release.create": {},
}

// worldNeed is an extra a resource template's first read stands on although
// none of its variables names it: the extra, for the reason a missing one
// gives, whether the World holds what the read needs, and what the template
// lacks without it.
type worldNeed struct {
	extra string
	held  func(*World) bool
	lacks string
}

// templateNeeds are templates every variable of which binds from the core,
// and whose first read fails all the same while an extra is missing. The
// latest pipeline template names only the project, and GitLab answers its
// read with an error while the project holds no pipeline: the resource sweep
// would count that among its errors, and a subscribe is declined after the
// harness's thirty seconds, which the subscription sweep fails on. Gated here,
// the template is left unbound with the pipeline's reason instead.
//
// What it needs is a pipeline the project holds, not the settled one the
// pipeline_id binding is: a job that never appeared, a cancel GitLab refused
// or the settle budget leave pipeline_id unbound, and the project holds the
// pipeline all the same, so the template stays bound. Only a pipeline GitLab
// never created (its configuration commit or the create refused) leaves it
// unbound.
var templateNeeds = map[string]worldNeed{
	"gitlab://project/{project_id}/pipelines/latest": {
		extra: worldExtraPipeline,
		held:  func(w *World) bool { return w.holdsPipeline },
		lacks: "so the project holds no pipeline for the latest pipeline template to read",
	},
}

// templateBindings are values bound for one resource template only, because
// the variable's name means more than one object across the catalog: board_id
// also names group and epic boards, name is a required parameter of dozens of
// actions, slug is also an integration's, snippet_id serves both snippet kinds,
// and label_id and milestone_iid bind the project's objects everywhere else.
// Putting any of them in [World.Bindings] would have the read sweep call an
// action with the wrong object and credit the refusal as a read.
//
// The release template is here too, so a tag made without its release does
// not bind a release read that can only fail, and so is the file template,
// whose path is the README the project was created with.
var templateBindings = map[string]map[string]worldBinding{
	"gitlab://project/{project_id}/board/{board_id}": {
		"board_id": {worldExtraBoard, func(w *World) (any, bool) { return w.Board.ID, w.Board.ID != 0 }},
	},
	"gitlab://project/{project_id}/feature_flag/{name}": {
		"name": {worldExtraFeatureFlag, func(w *World) (any, bool) { return w.FeatureFlag.Name, w.FeatureFlag.Name != "" }},
	},
	"gitlab://project/{project_id}/wiki/{slug}": {
		"slug": {worldExtraWikiPage, func(w *World) (any, bool) { return w.WikiPage.Slug, w.WikiPage.Slug != "" }},
	},
	"gitlab://project/{project_id}/file/{ref}/{+path}": {
		"path": {"", func(*World) (any, bool) { return worldReadmePath, true }},
	},
	"gitlab://project/{project_id}/release/{tag_name}": {
		"tag_name": releaseTagBinding,
	},
	"gitlab://project/{project_id}/snippet/{snippet_id}": {
		"snippet_id": {worldExtraProjectSnippet, func(w *World) (any, bool) { return w.ProjectSnippet.ID, w.ProjectSnippet.ID != 0 }},
	},
	"gitlab://snippet/{snippet_id}": {
		"snippet_id": {worldExtraSnippet, func(w *World) (any, bool) { return w.Snippet.ID, w.Snippet.ID != 0 }},
	},
	"gitlab://group/{group_id}/label/{label_id}": {
		"label_id": {worldExtraGroupLabel, func(w *World) (any, bool) { return w.GroupLabel.ID, w.GroupLabel.ID != 0 }},
	},
	"gitlab://group/{group_id}/milestone/{milestone_iid}": {
		"milestone_iid": {worldExtraGroupMilestone, func(w *World) (any, bool) { return w.GroupMilestone.IID, w.GroupMilestone.IID != 0 }},
	},
}

// BindTemplate returns the World's value for one variable of one resource
// template, whether it has one, and, when it has none, why.
//
// A template whose first read stands on an extra the World does not hold
// ([templateNeeds]) binds none of its variables, and says which extra. Then a
// value bound for that template alone wins, then the plain bindings through
// [World.BindParam], which also gives the reason for a variable neither
// carries.
func (w *World) BindTemplate(template, variable string) (value any, bound bool, reason string) {
	if need, gated := templateNeeds[template]; gated && !need.held(w) {
		return nil, false, w.extraReason(need.extra) + ", " + need.lacks
	}
	if binding, scoped := templateBindings[template][variable]; scoped {
		return w.bindExtra(binding, variable)
	}
	return w.BindParam(variable)
}

// BindActionParam returns the World's value for one required parameter of one
// catalog action, whether it has one, and, when it has none, why. It is what
// the read and preview sweeps bind an action's requirements through.
//
// A value bound for the action's domain ([domainBindings]) wins, unless the
// action is one [domainBindingExemptions] names, then [World.BindParam]. The
// domain is the canonical ID's part before its dot.
func (w *World) BindActionParam(action, name string) (value any, bound bool, reason string) {
	if _, exempt := domainBindingExemptions[action]; exempt {
		return w.BindParam(name)
	}
	domain, _, _ := strings.Cut(action, ".")
	if binding, scoped := domainBindings[domain][name]; scoped {
		return w.bindExtra(binding, name)
	}
	return w.BindParam(name)
}

// bindExtra returns one extra binding's value, or why the World holds none.
func (w *World) bindExtra(binding worldBinding, name string) (value any, bound bool, reason string) {
	if made, held := binding.value(w); held {
		return made, true, ""
	}
	return nil, false, w.unboundReason(binding.extra, name)
}

// BindParam returns the World's value for one parameter name, whether it has
// one, and, when it has none, why. It is [World.Bind] with the reason, for a
// sweep that logs what it could not bind.
//
// A name a plain extra binding would carry ([plainExtraBindings]) is reported
// with the reason that extra was not made, so the read and preview sweeps name
// the real cause (a pipeline that never settled) beside the action rather than
// a missing name, as the resource and subscription sweeps do beside a
// template. A name only a template binds ([templateBindings]: a wiki's slug,
// board_id, a feature flag's name, snippet_id, the file template's path) reads
// here as a name the World has no binding for, whatever became of the extra,
// and only [World.BindTemplate] gives that extra's reason.
func (w *World) BindParam(name string) (value any, bound bool, reason string) {
	if plainValue, plainBound := w.Bind(name); plainBound {
		return plainValue, true, ""
	}
	if binding, plain := plainExtraBindings[name]; plain {
		return nil, false, w.unboundReason(binding.extra, name)
	}
	return nil, false, "the World has no binding for " + name
}

// unboundReason says why the World holds no value for a name an extra would
// carry: the reason that extra was not made, when the build recorded one, and
// otherwise that the World holds none, which is what a World built without
// its extras says.
func (w *World) unboundReason(extra, name string) string {
	return w.extraReason(extra) + ", so it has no binding for " + name
}

// extraReason says why the World lacks one extra: the reason it was not made,
// when the build recorded one, and otherwise that the World holds none.
func (w *World) extraReason(extra string) string {
	if reason, recorded := w.unbound[extra]; recorded {
		return fmt.Sprintf("the World's %s was not made (%s)", extra, reason)
	}
	return "the World holds no " + extra
}
