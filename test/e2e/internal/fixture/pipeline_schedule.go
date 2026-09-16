//go:build e2e

// pipeline_schedule.go builds a pipeline schedule.
//
// The cron is deliberately one that will not fire during a run: a schedule
// that ran would create pipelines nothing cleans up, on an instance whose
// runner the rest of the suite is waiting for. A case that wants the schedule
// to run asks GitLab to play it, which is an action rather than a wait.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The schedule's cadence: once a year, in a timezone GitLab always knows.
const (
	pipelineScheduleCron     = "0 4 1 1 *"
	pipelineScheduleTimezone = "UTC"
)

// PipelineSchedule is a pipeline schedule a builder created.
type PipelineSchedule struct {
	// ID is what the schedule actions take.
	ID int64
	// Description is what it was created as.
	Description string
	// Ref is the branch it would run on.
	Ref string
	// Cron is the cadence it was created with.
	Cron string
}

// NewPipelineSchedule adds an inactive schedule to the project on its default
// branch and registers its deletion.
func NewPipelineSchedule(e *harness.Env, project Project) PipelineSchedule {
	e.T.Helper()

	description := e.Name("schedule")
	schedule, err := retryTransient(e, "create pipeline schedule "+description, createRetries, func() (PipelineSchedule, error) {
		return createPipelineSchedule(e.Ctx, e.Client(), project.ID, description, project.DefaultBranch)
	})
	if err != nil {
		e.T.Fatalf("creating pipeline schedule %q in project %d: %v", description, project.ID, err)
	}

	e.Defer("pipeline schedule "+description, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deletePipelineSchedule(ctx, e.Client(), project.ID, schedule.ID)
	})
	return schedule
}

// createPipelineSchedule asks GitLab for the schedule, inactive so that
// nothing it describes ever runs by itself.
func createPipelineSchedule(ctx context.Context, client *gitlabclient.Client, projectID int64, description, ref string) (PipelineSchedule, error) {
	created, _, err := client.GL().PipelineSchedules.CreatePipelineSchedule(projectID, &gl.CreatePipelineScheduleOptions{
		Description:  new(description),
		Ref:          new(ref),
		Cron:         new(pipelineScheduleCron),
		CronTimezone: new(pipelineScheduleTimezone),
		Active:       new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return PipelineSchedule{}, err
	}
	return PipelineSchedule{ID: created.ID, Description: created.Description, Ref: created.Ref, Cron: created.Cron}, nil
}

// deletePipelineSchedule removes the schedule and tolerates one a case
// deleted.
func deletePipelineSchedule(ctx context.Context, client *gitlabclient.Client, projectID, scheduleID int64) error {
	_, err := client.GL().PipelineSchedules.DeletePipelineSchedule(projectID, scheduleID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting pipeline schedule %d of project %d: %w", scheduleID, projectID, err)
	}
	return nil
}
