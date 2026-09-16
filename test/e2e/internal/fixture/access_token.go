//go:build e2e

// access_token.go builds a project access token, which is the object a
// revocation case revokes and a rotation case rotates.
//
// It registers its own revocation even though the token dies with the project
// that owns it, for one reason worth keeping: a case whose whole subject is
// revoking the token leaves nothing behind, and the cleanup has to tolerate
// that rather than report the run as leaking. Every deletion here therefore
// treats "already gone" as success.

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

// ProjectAccessToken is a project access token a builder created.
type ProjectAccessToken struct {
	// ID is what the revoke and rotate actions take.
	ID int64
	// Name is what it was created as.
	Name string
	// Value is the secret itself, which GitLab shows once at creation.
	Value string
	// ProjectID is the project that owns it.
	ProjectID int64
}

// NewProjectAccessToken creates a maintainer-level token on the project with
// the api scope and registers its revocation on the Env.
func NewProjectAccessToken(e *harness.Env, project Project) ProjectAccessToken {
	e.T.Helper()

	name := e.Name("pat")
	token, err := retryTransient(e, "create project access token "+name, createRetries, func() (ProjectAccessToken, error) {
		return createProjectAccessToken(e.Ctx, e.Client(), project.ID, name, time.Now().Add(tokenLifetime))
	})
	if err != nil {
		e.T.Fatalf("creating project access token %q in project %d: %v", name, project.ID, err)
	}

	e.Defer("project access token "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return revokeProjectAccessToken(ctx, e.Client(), project.ID, token.ID)
	})
	return token
}

// createProjectAccessToken asks GitLab for the token and reads back what a
// test addresses it by.
func createProjectAccessToken(ctx context.Context, client *gitlabclient.Client, projectID int64, name string, expiry time.Time) (ProjectAccessToken, error) {
	scopes := []string{"api"}
	isoExpiry := gl.ISOTime(expiry)
	created, _, err := client.GL().ProjectAccessTokens.CreateProjectAccessToken(projectID, &gl.CreateProjectAccessTokenOptions{
		Name:        new(name),
		Scopes:      &scopes,
		AccessLevel: new(gl.MaintainerPermissions),
		ExpiresAt:   &isoExpiry,
	}, gl.WithContext(ctx))
	if err != nil {
		return ProjectAccessToken{}, err
	}
	return ProjectAccessToken{ID: created.ID, Name: created.Name, Value: created.Token, ProjectID: projectID}, nil
}

// revokeProjectAccessToken revokes the token and tolerates one a case has
// already revoked.
//
// The 404 is the ordinary ending rather than an exception: the destructive
// case that revokes the token is why the fixture exists.
func revokeProjectAccessToken(ctx context.Context, client *gitlabclient.Client, projectID, tokenID int64) error {
	_, err := client.GL().ProjectAccessTokens.RevokeProjectAccessToken(projectID, tokenID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("revoking project access token %d of project %d: %w", tokenID, projectID, err)
	}
	return nil
}
