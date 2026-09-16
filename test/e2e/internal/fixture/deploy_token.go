//go:build e2e

// deploy_token.go builds a project deploy token.
//
// The username is left to GitLab rather than named: a deploy token username
// is unique per project and GitLab generates one that cannot collide, while a
// name chosen here would have to carry the run scoping twice.

package fixture

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// DeployToken is a deploy token a builder created on a project.
type DeployToken struct {
	// ID is what the deploy token actions take.
	ID int64
	// Name is what it was created as.
	Name string
	// Username is what GitLab generated for it.
	Username string
	// Value is the secret itself, shown once at creation.
	Value string
}

// NewProjectDeployToken creates a read-only deploy token on the project and
// registers its deletion.
func NewProjectDeployToken(e *harness.Env, project Project) DeployToken {
	e.T.Helper()

	name := e.Name("deploytoken")
	token, err := retryTransient(e, "create deploy token "+name, createRetries, func() (DeployToken, error) {
		return createProjectDeployToken(e.Ctx, e.Client(), project.ID, name, time.Now().Add(tokenLifetime))
	})
	if err != nil {
		e.T.Fatalf("creating deploy token %q in project %d: %v", name, project.ID, err)
	}

	e.Defer("deploy token "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectDeployToken(ctx, e.Client(), project.ID, token.ID)
	})
	return token
}

// createProjectDeployToken asks GitLab for the token.
func createProjectDeployToken(ctx context.Context, client *gitlabclient.Client, projectID int64, name string, expiry time.Time) (DeployToken, error) {
	scopes := []string{"read_repository"}
	created, _, err := client.GL().DeployTokens.CreateProjectDeployToken(projectID, &gl.CreateProjectDeployTokenOptions{
		Name:      new(name),
		Scopes:    &scopes,
		ExpiresAt: &expiry,
	}, gl.WithContext(ctx))
	if err != nil {
		return DeployToken{}, err
	}
	return DeployToken{ID: created.ID, Name: created.Name, Username: created.Username, Value: created.Token}, nil
}

// deleteProjectDeployToken removes the token and tolerates one a case deleted.
func deleteProjectDeployToken(ctx context.Context, client *gitlabclient.Client, projectID, tokenID int64) error {
	_, err := client.GL().DeployTokens.DeleteProjectDeployToken(projectID, tokenID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting deploy token %d of project %d: %w", tokenID, projectID, err)
	}
	return nil
}
