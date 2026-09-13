//go:build e2e

// environments_lifecycle_test.go covers an environment through the server
// after its creation, which environments_test.go already drives on its
// own: the reads, the update, the stop GitLab completes in the request for
// an environment without a stop job, and the delete; and the freeze periods
// of a project, which the environment group carries.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// environmentIDs returns the identifiers of an environment listing.
func environmentIDs(listed []environments.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, environment := range listed {
		ids = append(ids, environment.ID)
	}
	return ids
}

// TestEnvironment_Lifecycle_GetListUpdateStopDelete creates an environment
// per surface on a shared project, reads and lists it, gives it an external
// URL, stops it, deletes it and asserts the read afterwards is refused as
// not found.
//
// Replaces: TestIndividual_Environments, TestMeta_Environments
func TestEnvironment_Lifecycle_GetListUpdateStopDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("envlife"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		name := e.Name("staging")

		created := harness.Do[environments.Output](s, actionEnvironmentCreate, withParams(params, map[string]any{"name": name}))
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("environment create answered %+v, want %q with an ID", created, name)
		}
		byID := withParams(params, map[string]any{"environment_id": created.ID})

		got := harness.Do[environments.Output](s, actionEnvironmentGet, byID)
		if got.ID != created.ID || got.Name != name || got.State != "available" {
			e.T.Errorf("environment get answered %+v, want the available environment %d %q", got, created.ID, name)
		}
		listed := harness.Do[environments.ListOutput](s, actionEnvironmentList, params)
		if !containsID(environmentIDs(listed.Environments), created.ID) {
			e.T.Errorf("the environment listing does not hold %d: %v", created.ID, environmentIDs(listed.Environments))
		}

		externalURL := "https://" + string(surface) + "-staging.example.com"
		updated := harness.Do[environments.Output](s, actionEnvironmentUpdate, withParams(byID, map[string]any{"external_url": externalURL}))
		if updated.ID != created.ID || updated.ExternalURL != externalURL {
			e.T.Errorf("environment update answered %+v, want environment %d at %s", updated, created.ID, externalURL)
		}

		// The environment has no stop job, so GitLab completes the stop in
		// the request rather than leaving it stopping.
		stopped := harness.Do[environments.Output](s, actionEnvironmentStop, byID)
		if stopped.ID != created.ID || stopped.State != "stopped" {
			e.T.Errorf("environment stop answered %+v, want environment %d stopped", stopped, created.ID)
		}

		harness.DoVoid(s, actionEnvironmentDelete, byID)
		refused := harness.Refused(s, actionEnvironmentGet, byID, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}

// The cron expression a freeze period is updated to; the window it is
// created with, and the reader of a listing's identifiers, are declared once
// in freezeperiods_test.go, which covers the same family.
const freezeUpdatedStart = "0 22 * * 5"

// TestEnvironment_FreezePeriods_ListCreateGetUpdateDelete creates a freeze
// period per surface on a shared project, finds it in the listing, reads
// it, moves its start, deletes it and asserts the read afterwards is
// refused as not found.
//
// Replaces: TestMeta_EnvironmentsFreeze
func TestEnvironment_FreezePeriods_ListCreateGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("freeze"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[freezeperiods.Output](s, actionEnvironmentFreezeCreate, withParams(params, map[string]any{
			"freeze_start": freezeStart, "freeze_end": freezeEnd, "cron_timezone": "UTC",
		}))
		if created.ID == 0 || created.FreezeStart != freezeStart || created.FreezeEnd != freezeEnd || created.CronTimezone != "UTC" {
			e.T.Fatalf("freeze period create answered %+v, want %q to %q in UTC with an ID", created, freezeStart, freezeEnd)
		}
		byID := withParams(params, map[string]any{"freeze_period_id": created.ID})

		listed := harness.Do[freezeperiods.ListOutput](s, actionEnvironmentFreezeList, params)
		if !slices.Contains(freezePeriodIDs(listed.FreezePeriods), created.ID) {
			e.T.Errorf("the freeze period listing does not hold %d: %v", created.ID, freezePeriodIDs(listed.FreezePeriods))
		}
		got := harness.Do[freezeperiods.Output](s, actionEnvironmentFreezeGet, byID)
		if got.ID != created.ID || got.FreezeStart != freezeStart {
			e.T.Errorf("freeze period get answered %+v, want period %d starting %q", got, created.ID, freezeStart)
		}
		updated := harness.Do[freezeperiods.Output](s, actionEnvironmentFreezeUpdate, withParams(byID, map[string]any{"freeze_start": freezeUpdatedStart}))
		if updated.ID != created.ID || updated.FreezeStart != freezeUpdatedStart {
			e.T.Errorf("freeze period update answered %+v, want period %d starting %q", updated, created.ID, freezeUpdatedStart)
		}

		harness.DoVoid(s, actionEnvironmentFreezeDelete, byID)
		refused := harness.Refused(s, actionEnvironmentFreezeGet, byID, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
