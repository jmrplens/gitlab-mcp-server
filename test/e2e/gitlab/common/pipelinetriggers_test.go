//go:build e2e

// pipelinetriggers_test.go covers a pipeline trigger token through the
// server: its lifecycle, and the pipeline running it starts on a project
// that carries a CI configuration.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// triggerIDs returns the identifiers of a trigger listing.
func triggerIDs(listed []pipelinetriggers.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, trigger := range listed {
		ids = append(ids, trigger.ID)
	}
	return ids
}

// TestPipelineTrigger_Lifecycle_CreateListGetUpdateRunDelete creates a
// trigger token per surface on a shared project carrying the fixture's CI
// configuration, finds it in the listing, reads it, renames it, runs a
// pipeline with it on the default branch, deletes it and asserts the
// listing no longer holds it and the read is refused as not found.
//
// The triggered pipeline is left to the runner, or to stay pending without
// one: what the run proves is the pipeline the answer names, not its end.
//
// Replaces: TestMeta_PipelineTriggers, TestIndividual_PipelineTriggerRun
func TestPipelineTrigger_Lifecycle_CreateListGetUpdateRunDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("trigger"))
		fixture.CIFile(e, project)
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		description := e.Name("trigger")

		created := harness.Do[pipelinetriggers.Output](s, actionPipelineTriggerCreate, withParams(params, map[string]any{"description": description}))
		if created.ID == 0 || created.Token == "" || created.Description != description {
			e.T.Fatalf("trigger create answered %+v, want %q with an ID and a token", created, description)
		}
		byID := withParams(params, map[string]any{"trigger_id": created.ID})

		listed := harness.Do[pipelinetriggers.ListOutput](s, actionPipelineTriggerList, params)
		if !containsID(triggerIDs(listed.Triggers), created.ID) {
			e.T.Errorf("the trigger listing does not hold %d: %v", created.ID, triggerIDs(listed.Triggers))
		}
		got := harness.Do[pipelinetriggers.Output](s, actionPipelineTriggerGet, byID)
		if got.ID != created.ID || got.Description != description {
			e.T.Errorf("trigger get answered %+v, want trigger %d %q", got, created.ID, description)
		}
		updated := harness.Do[pipelinetriggers.Output](s, actionPipelineTriggerUpdate, withParams(byID, map[string]any{"description": description + "-updated"}))
		if updated.ID != created.ID || updated.Description != description+"-updated" {
			e.T.Errorf("trigger update answered %+v, want trigger %d renamed", updated, created.ID)
		}

		ran := harness.Do[pipelinetriggers.RunOutput](s, actionPipelineTriggerRun, withParams(params, map[string]any{
			"ref": project.DefaultBranch, "token": created.Token, "variables": map[string]string{"E2E_TRIGGER_RUN": "1"},
		}))
		if ran.ID == 0 || ran.Ref != project.DefaultBranch || ran.SHA == "" {
			e.T.Errorf("trigger run answered %+v, want a pipeline on %s with an ID and a commit", ran, project.DefaultBranch)
		}

		harness.DoVoid(s, actionPipelineTriggerDelete, byID)
		remaining := harness.Do[pipelinetriggers.ListOutput](s, actionPipelineTriggerList, params)
		if containsID(triggerIDs(remaining.Triggers), created.ID) {
			e.T.Errorf("the trigger listing still holds %d after its delete: %v", created.ID, triggerIDs(remaining.Triggers))
		}
		refused := harness.Refused(s, actionPipelineTriggerGet, byID, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
