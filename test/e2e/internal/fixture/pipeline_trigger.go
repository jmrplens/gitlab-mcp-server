//go:build e2e

// pipeline_trigger.go builds a pipeline trigger token, which is the object a
// trigger case reads, edits, deletes or fires a pipeline with.
//
// It needs no runner: a trigger exists whether or not anything can run the
// pipeline it starts, and the fixture is about the token rather than about
// what a triggered pipeline does.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// PipelineTrigger is a trigger token a builder created.
type PipelineTrigger struct {
	// ID is what the trigger actions take.
	ID int64
	// Description is what it was created as.
	Description string
	// Token is the secret itself, which a trigger case fires a pipeline
	// with.
	Token string
}

// NewPipelineTrigger adds a trigger token to the project and registers its
// deletion.
func NewPipelineTrigger(e *harness.Env, project Project) PipelineTrigger {
	e.T.Helper()

	description := e.Name("trigger")
	trigger, err := retryTransient(e, "create pipeline trigger "+description, createRetries, func() (PipelineTrigger, error) {
		return createPipelineTrigger(e.Ctx, e.Client(), project.ID, description)
	})
	if err != nil {
		e.T.Fatalf("creating pipeline trigger %q in project %d: %v", description, project.ID, err)
	}

	e.Defer("pipeline trigger "+description, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deletePipelineTrigger(ctx, e.Client(), project.ID, trigger.ID)
	})
	return trigger
}

// createPipelineTrigger asks GitLab for the token.
func createPipelineTrigger(ctx context.Context, client *gitlabclient.Client, projectID int64, description string) (PipelineTrigger, error) {
	created, _, err := client.GL().PipelineTriggers.AddPipelineTrigger(projectID, &gl.AddPipelineTriggerOptions{
		Description: new(description),
	}, gl.WithContext(ctx))
	if err != nil {
		return PipelineTrigger{}, err
	}
	return PipelineTrigger{ID: created.ID, Description: created.Description, Token: created.Token}, nil
}

// deletePipelineTrigger removes the token and tolerates one a case deleted.
func deletePipelineTrigger(ctx context.Context, client *gitlabclient.Client, projectID, triggerID int64) error {
	_, err := client.GL().PipelineTriggers.DeletePipelineTrigger(projectID, triggerID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting pipeline trigger %d of project %d: %w", triggerID, projectID, err)
	}
	return nil
}
