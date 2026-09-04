//go:build e2e && !enterprise

// pipeline_extras_ce_test.go covers pipeline-domain actions that the main
// pipeline suites do not exercise: resource group get/edit/upcoming-jobs and
// running a pipeline via a trigger token. Neither test needs a CI runner —
// pipeline creation succeeds with jobs left pending, which is enough for the
// resource group to materialize and for the trigger run to return a pipeline.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourcegroups"
)

// pipelineExtrasResourceGroupKey is the resource group key declared in
// pipelineExtrasCIYAML and looked up by the resource group subtests.
const pipelineExtrasResourceGroupKey = "e2e-resource-group"

// pipelineExtrasCIYAML declares a single job bound to a resource group. The
// resource group record is created as soon as the pipeline instantiates the
// job, even while the job is still pending for a runner.
const pipelineExtrasCIYAML = `stages:
  - deploy

deploy-job:
  stage: deploy
  resource_group: ` + pipelineExtrasResourceGroupKey + `
  script:
    - echo "resource group job"
`

// pipelineExtrasSetupProject creates a project seeded with a CI configuration
// that declares a resource group, so pipelines created from the default
// branch materialize the resource group and are runnable via trigger tokens.
func pipelineExtrasSetupProject(ctx context.Context, t *testing.T) ProjectFixture {
	t.Helper()
	proj := createProject(ctx, t, sess.individual)
	commitFileCreateOrUpdate(ctx, t, sess.individual, proj, defaultBranch, ".gitlab-ci.yml", pipelineExtrasCIYAML, "ci: add resource group pipeline config")
	return proj
}

// TestIndividual_PipelineResourceGroups exercises the resource group
// individual tools: gitlab_get_resource_group, gitlab_edit_resource_group,
// and gitlab_list_resource_group_upcoming_jobs.
//
// The test commits a CI configuration whose job declares a resource_group,
// creates a pipeline so GitLab materializes the group, then gets the group by
// key, switches its process mode to oldest_first, and lists its upcoming
// jobs. No runner is required: the pending job is enough for the group to
// exist, and the upcoming jobs listing is asserted only for shape since the
// queue content depends on runner activity.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_PipelineResourceGroups(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := pipelineExtrasSetupProject(ctx, t)

	pipe, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_create", pipelines.CreateInput{
		ProjectID: proj.pidOf(),
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "create pipeline for resource group")
	requireTruef(t, pipe.ID > 0, "expected positive pipeline ID")
	t.Logf("Created pipeline %d (status=%s)", pipe.ID, pipe.Status)

	t.Run("Get", func(t *testing.T) {
		// The group is created together with the pipeline's jobs; retry
		// absorbs the short window before it becomes visible.
		out, getErr := retryOnTransient(ctx, t, "get resource group", 5, func() (resourcegroups.ResourceGroupItem, error) {
			return callToolOn[resourcegroups.ResourceGroupItem](ctx, sess.individual, "gitlab_get_resource_group", resourcegroups.GetInput{
				ProjectID: proj.pidOf(),
				Key:       pipelineExtrasResourceGroupKey,
			})
		})
		requireNoError(t, getErr, "get resource group")
		requireTruef(t, out.Key == pipelineExtrasResourceGroupKey, "expected resource group key %q, got %q", pipelineExtrasResourceGroupKey, out.Key)
		requireTruef(t, out.ProcessMode != "", "expected a process mode, got empty")
		t.Logf("Got resource group %d (key=%s, process_mode=%s)", out.ID, out.Key, out.ProcessMode)
	})

	t.Run("Edit", func(t *testing.T) {
		out, editErr := callToolOn[resourcegroups.ResourceGroupItem](ctx, sess.individual, "gitlab_edit_resource_group", resourcegroups.EditInput{
			ProjectID:   proj.pidOf(),
			Key:         pipelineExtrasResourceGroupKey,
			ProcessMode: "oldest_first",
		})
		requireNoError(t, editErr, "edit resource group")
		requireTruef(t, out.ProcessMode == "oldest_first", "expected process mode oldest_first, got %q", out.ProcessMode)
		t.Logf("Updated resource group %d to process_mode=%s", out.ID, out.ProcessMode)
	})

	t.Run("UpcomingJobs", func(t *testing.T) {
		out, jobsErr := callToolOn[resourcegroups.ListUpcomingJobsOutput](ctx, sess.individual, "gitlab_list_resource_group_upcoming_jobs", resourcegroups.ListUpcomingJobsInput{
			ProjectID: proj.pidOf(),
			Key:       pipelineExtrasResourceGroupKey,
		})
		requireNoError(t, jobsErr, "list resource group upcoming jobs")
		// Without a runner the queue content is timing-dependent, so only
		// the successful listing shape is asserted.
		t.Logf("Resource group has %d upcoming job(s)", len(out.Jobs))
	})
}

// TestIndividual_PipelineTriggerRun exercises gitlab_pipeline_trigger_run via
// the individual tool surface.
//
// The test creates a pipeline trigger token in a project seeded with a valid
// CI configuration, then runs the trigger against the default branch with a
// CI variable and asserts a pipeline is created for the requested ref. The
// triggered pipeline is left pending (no runner needed) and is removed with
// the project fixture.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_PipelineTriggerRun(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := pipelineExtrasSetupProject(ctx, t)

	trigger, err := callToolOn[pipelinetriggers.Output](ctx, sess.individual, "gitlab_pipeline_trigger_create", pipelinetriggers.CreateInput{
		ProjectID:   proj.pidOf(),
		Description: "e2e trigger run token",
	})
	requireNoError(t, err, "create pipeline trigger")
	requireTruef(t, trigger.Token != "", "expected a trigger token, got empty")
	t.Logf("Created trigger %d", trigger.ID)

	out, runErr := callToolOn[pipelinetriggers.RunOutput](ctx, sess.individual, "gitlab_pipeline_trigger_run", pipelinetriggers.RunInput{
		ProjectID: proj.pidOf(),
		Ref:       defaultBranch,
		Token:     trigger.Token,
		Variables: map[string]string{"E2E_TRIGGER_RUN": "1"},
	})
	requireNoError(t, runErr, "run pipeline trigger")
	requireTruef(t, out.ID > 0, "expected positive triggered pipeline ID")
	requireTruef(t, out.Ref == defaultBranch, "expected triggered pipeline ref %q, got %q", defaultBranch, out.Ref)
	t.Logf("Trigger run created pipeline %d (status=%s, source=%s)", out.ID, out.Status, out.Source)
}
