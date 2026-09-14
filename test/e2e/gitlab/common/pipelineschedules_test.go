//go:build e2e

// pipelineschedules_test.go covers a pipeline schedule through the server:
// its lifecycle, the variables it carries, the ownership of it, the run it
// can be asked for, the pipelines that run created, and its delete. The old
// suite ran a schedule and never read the answer; it is asserted here.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The cron expressions a schedule is created with and updated to.
const (
	scheduleCron        = "0 1 * * *"
	scheduleUpdatedCron = "0 2 * * *"
)

// scheduleRefIsBranch reports whether the ref a schedule answers names the
// branch it was created for.
//
// GitLab 19 stores a schedule's ref fully qualified and answers
// refs/heads/main for a schedule created on main, so both spellings are
// accepted and nothing else is.
func scheduleRefIsBranch(ref, branch string) bool {
	return ref == branch || ref == "refs/heads/"+branch
}

// scheduleIDs returns the identifiers of a schedule listing.
func scheduleIDs(listed []pipelineschedules.Output) []int {
	ids := make([]int, 0, len(listed))
	for _, schedule := range listed {
		ids = append(ids, schedule.ID)
	}
	return ids
}

// scheduleVariableKeys returns the keys of a schedule's variables.
func scheduleVariableKeys(variables []pipelineschedules.VariableObject) []string {
	keys := make([]string, 0, len(variables))
	for _, variable := range variables {
		keys = append(keys, variable.Key)
	}
	return keys
}

// scheduleVariableLifecycle adds a variable to a schedule, edits it, reads
// the schedule to find it, removes it and reads the schedule again to find
// it gone.
func scheduleVariableLifecycle(e *harness.Env, s *harness.Session, byID map[string]any, scheduleID int, key string) {
	e.T.Helper()

	variable := harness.Do[pipelineschedules.VariableOutput](s, actionPipelineScheduleCreateVariable, withParams(byID, map[string]any{"key": key, "value": "original"}))
	if variable.Key != key || variable.Value != "original" {
		e.T.Errorf("schedule variable create answered %+v, want %s=original", variable, key)
	}
	edited := harness.Do[pipelineschedules.VariableOutput](s, actionPipelineScheduleEditVariable, withParams(byID, map[string]any{"key": key, "value": "edited"}))
	if edited.Key != key || edited.Value != "edited" {
		e.T.Errorf("schedule variable edit answered %+v, want %s=edited", edited, key)
	}
	withVariable := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleGet, byID)
	if !slices.Contains(scheduleVariableKeys(withVariable.Variables), key) {
		e.T.Errorf("schedule %d does not carry the variable %s: %v", scheduleID, key, scheduleVariableKeys(withVariable.Variables))
	}
	harness.DoVoid(s, actionPipelineScheduleDeleteVariable, withParams(byID, map[string]any{"key": key}))
	withoutVariable := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleGet, byID)
	if slices.Contains(scheduleVariableKeys(withoutVariable.Variables), key) {
		e.T.Errorf("schedule %d still carries the variable %s after its delete: %v", scheduleID, key, scheduleVariableKeys(withoutVariable.Variables))
	}
}

// TestPipelineSchedule_Lifecycle_VariablesOwnershipRunDelete creates a
// schedule per surface on a shared project, reads and lists it, changes its
// description and cron, adds a variable, edits it and removes it, takes
// ownership of the schedule, runs it, lists the pipelines it triggered,
// deletes it and asserts the read afterwards is refused as not found.
//
// Replaces: TestIndividual_PipelineSchedules, TestMeta_PipelineSchedules, TestMeta_PipelineSchedulesExtended
func TestPipelineSchedule_Lifecycle_VariablesOwnershipRunDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("sched"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		description := e.Name("schedule")

		created := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleCreate, withParams(params, map[string]any{
			"description": description, "ref": project.DefaultBranch, "cron": scheduleCron,
		}))
		if created.ID == 0 || created.Description != description || created.Cron != scheduleCron || !scheduleRefIsBranch(created.Ref, project.DefaultBranch) {
			e.T.Fatalf("schedule create answered %+v, want %q on %s at %q with an ID", created, description, project.DefaultBranch, scheduleCron)
		}
		byID := withParams(params, map[string]any{"schedule_id": created.ID})

		got := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleGet, byID)
		if got.ID != created.ID || got.Description != description {
			e.T.Errorf("schedule get answered %+v, want schedule %d %q", got, created.ID, description)
		}
		listed := harness.Do[pipelineschedules.ListOutput](s, actionPipelineScheduleList, params)
		if !slices.Contains(scheduleIDs(listed.Schedules), created.ID) {
			e.T.Errorf("the schedule listing does not hold %d: %v", created.ID, scheduleIDs(listed.Schedules))
		}
		updated := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleUpdate, withParams(byID, map[string]any{
			"description": description + "-updated", "cron": scheduleUpdatedCron,
		}))
		if updated.ID != created.ID || updated.Description != description+"-updated" || updated.Cron != scheduleUpdatedCron {
			e.T.Errorf("schedule update answered %+v, want schedule %d renamed and at %q", updated, created.ID, scheduleUpdatedCron)
		}

		scheduleVariableLifecycle(e, s, byID, created.ID, variableKeyFor("E2E_SCHED", surface))

		owned := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleTakeOwnership, byID)
		if owned.ID != created.ID || owned.Owner == nil || owned.Owner.Username != e.Runtime().Username {
			e.T.Errorf("schedule take_ownership answered %+v, want schedule %d owned by %s", owned, created.ID, e.Runtime().Username)
		}

		// The run is scheduled asynchronously: the action answers the
		// schedule itself, and the pipeline it starts may or may not be
		// listed yet when the triggered pipelines are read.
		ran := harness.Do[pipelineschedules.Output](s, actionPipelineScheduleRun, byID)
		if ran.ID != created.ID {
			e.T.Errorf("schedule run answered %+v, want schedule %d", ran, created.ID)
		}
		triggered := harness.Do[pipelineschedules.TriggeredPipelinesListOutput](s, actionPipelineScheduleListTriggeredPipelines, byID)
		e.T.Logf("schedule %d has triggered %d pipeline(s) so far", created.ID, len(triggered.Pipelines))

		harness.DoVoid(s, actionPipelineScheduleDelete, byID)
		refused := harness.Refused(s, actionPipelineScheduleGet, byID, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
