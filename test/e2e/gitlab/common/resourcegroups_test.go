//go:build e2e

// resourcegroups_test.go covers a project's resource groups, which GitLab
// materializes when a pipeline instantiates a job bound to one: the listing
// before and after, the read by key, the change of the process mode, and
// the upcoming jobs of the group.

package common

import (
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourcegroups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// resourceGroupKey is the resource group resourceGroupCIYAML binds its job
// to.
const resourceGroupKey = "e2e-resource-group"

// resourceGroupCIYAML declares one job bound to a resource group. The group
// record exists as soon as a pipeline instantiates the job, whether or not
// a runner ever picks it up.
const resourceGroupCIYAML = `stages:
  - deploy

deploy-job:
  stage: deploy
  resource_group: ` + resourceGroupKey + `
  script:
    - echo "resource group job"
  tags: []
`

// The group appears shortly after the pipeline is created, so the read is
// polled at this cadence.
const (
	resourceGroupPollInterval = 2 * time.Second
	resourceGroupPollTimeout  = 60 * time.Second
)

// resourceGroupKeys returns the keys of a resource group listing.
func resourceGroupKeys(listed []resourcegroups.ResourceGroupItem) []string {
	keys := make([]string, 0, len(listed))
	for _, group := range listed {
		keys = append(keys, group.Key)
	}
	return keys
}

// TestResourceGroup_Lifecycle_ListGetEditUpcomingJobs lists the resource
// groups of a fresh shared project, which are none, creates a pipeline whose
// job is bound to one, waits for the group to appear, finds it in the
// listing, switches its process mode to oldest_first and lists the jobs
// queued on it. The pipeline is not waited for: the group exists whether or
// not the job ever runs.
//
// Replaces: TestMeta_ResourceGroups, TestIndividual_PipelineResourceGroups
func TestResourceGroup_Lifecycle_ListGetEditUpcomingJobs(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("resgrp"))
		fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, resourceGroupCIYAML, "ci: bind a job to a resource group")
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		byKey := withParams(params, map[string]any{"key": resourceGroupKey})

		created := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate, withParams(params, map[string]any{"ref": project.DefaultBranch}))
		if created.ID == 0 {
			e.T.Fatalf("pipeline create answered %+v, want a pipeline with an ID", created)
		}

		got := harness.Eventually(s, actionPipelineResourceGroupGet, byKey, resourceGroupPollInterval, resourceGroupPollTimeout,
			func(out resourcegroups.ResourceGroupItem) bool { return out.Key == resourceGroupKey })
		if got.ID == 0 || got.ProcessMode == "" {
			e.T.Errorf("resource group get answered %+v, want %s with an ID and a process mode", got, resourceGroupKey)
		}
		listed := harness.Do[resourcegroups.ListOutput](s, actionPipelineResourceGroupList, params)
		if !slices.Contains(resourceGroupKeys(listed.Groups), resourceGroupKey) {
			e.T.Errorf("the resource group listing does not hold %s: %v", resourceGroupKey, resourceGroupKeys(listed.Groups))
		}

		edited := harness.Do[resourcegroups.ResourceGroupItem](s, actionPipelineResourceGroupEdit, withParams(byKey, map[string]any{"process_mode": "oldest_first"}))
		if edited.ID != got.ID || edited.ProcessMode != "oldest_first" {
			e.T.Errorf("resource group edit answered %+v, want group %d in oldest_first mode", edited, got.ID)
		}

		// What is queued on the group depends on whether a runner has taken
		// the job yet, so the listing is asserted on its shape: every job it
		// names is the one the configuration bound to the group.
		upcoming := harness.Do[resourcegroups.ListUpcomingJobsOutput](s, actionPipelineResourceGroupUpcomingJobs, byKey)
		for _, job := range upcoming.Jobs {
			if job.Name != "deploy-job" {
				e.T.Errorf("the upcoming jobs of %s name %q, and only deploy-job is bound to it", resourceGroupKey, job.Name)
			}
		}
		e.T.Logf("resource group %s has %d upcoming job(s)", resourceGroupKey, len(upcoming.Jobs))
	})
}
