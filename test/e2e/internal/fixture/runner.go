//go:build e2e

// runner.go creates a runner nothing will ever run a job on, for the cases
// whose subject is the runner record itself: reading one, pausing one,
// deleting one.
//
// It is created through the user runners endpoint rather than registered with
// a registration token, because a registration token is what GitLab 16
// deprecated and 17 stopped accepting by default; the endpoint takes the
// caller's own credential and needs nothing provisioned.
//
// The runner is created paused and tagged with a tag no pipeline in this
// suite names, which is the whole point of the word disposable: an unpaused
// untagged runner on the instance would start picking up the jobs the Docker
// runner is meant to run, and a case that deleted it mid-job would leave that
// job stuck. The reader of the Docker stack's own runner is in pipeline.go,
// beside the pipeline wait that needs it.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// DisposableRunnerTag is the tag every runner this builder creates carries.
// No fixture pipeline configuration names it, so nothing is ever scheduled
// onto one of these.
const DisposableRunnerTag = "e2e-disposable"

// Runner is a runner a builder created.
type Runner struct {
	// ID is what the runner actions take.
	ID int64
	// Description is what it was created as.
	Description string
	// Token is the authentication token GitLab minted for it, which a
	// verify or unregister case sends instead of the ID.
	Token string
}

// NewProjectRunner creates a paused, tagged project runner and registers its
// deletion.
func NewProjectRunner(e *harness.Env, project Project) Runner {
	e.T.Helper()

	description := e.Name("runner")
	runner, err := retryTransient(e, "create runner "+description, createRetries, func() (Runner, error) {
		return createProjectRunner(e.Ctx, e.Client(), project.ID, description)
	})
	if err != nil {
		e.T.Fatalf("creating a runner for project %d: %v", project.ID, err)
	}

	e.Defer("runner "+description, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteRunner(ctx, e.Client(), runner.ID)
	})
	return runner
}

// createProjectRunner asks GitLab for the runner.
func createProjectRunner(ctx context.Context, client *gitlabclient.Client, projectID int64, description string) (Runner, error) {
	tags := []string{DisposableRunnerTag}
	created, _, err := client.GL().Users.CreateUserRunner(&gl.CreateUserRunnerOptions{
		RunnerType:  new("project_type"),
		ProjectID:   new(projectID),
		Description: new(description),
		Paused:      new(true),
		RunUntagged: new(false),
		TagList:     &tags,
	}, gl.WithContext(ctx))
	if err != nil {
		return Runner{}, err
	}
	return Runner{ID: created.ID, Description: description, Token: created.Token}, nil
}

// deleteRunner removes the runner and tolerates one a case deleted.
func deleteRunner(ctx context.Context, client *gitlabclient.Client, runnerID int64) error {
	_, err := client.GL().Runners.DeleteRegisteredRunnerByID(runnerID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting runner %d: %w", runnerID, err)
	}
	return nil
}
