//go:build e2e

// deploy_key.go builds a project deploy key.
//
// The key is generated per fixture rather than taken from a constant, because
// GitLab holds a deploy key's fingerprint unique across the instance: two
// attempts offering one committed public key would have the second refused,
// and the refusal ("has already been taken") reads as a bug in the case
// rather than as a collision between two runs.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// DeployKey is a deploy key a builder added to a project.
type DeployKey struct {
	// ID is what the deploy key actions take.
	ID int64
	// Title is what it was created as.
	Title string
	// Key is the public key GitLab stored.
	Key string
}

// NewDeployKey generates a key pair, adds the public half to the project as a
// read-only deploy key and registers its removal.
func NewDeployKey(e *harness.Env, project Project) DeployKey {
	e.T.Helper()

	publicKey, keyErr := SSHPublicKey()
	if keyErr != nil {
		e.T.Fatal(keyErr)
	}
	title := e.Name("deploykey")

	key, err := retryTransient(e, "create deploy key "+title, createRetries, func() (DeployKey, error) {
		return createDeployKey(e.Ctx, e.Client(), project.ID, title, publicKey)
	})
	if err != nil {
		e.T.Fatalf("adding deploy key %q to project %d: %v", title, project.ID, err)
	}

	e.Defer("deploy key "+title, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteDeployKey(ctx, e.Client(), project.ID, key.ID)
	})
	return key
}

// createDeployKey asks GitLab for the key.
func createDeployKey(ctx context.Context, client *gitlabclient.Client, projectID int64, title, publicKey string) (DeployKey, error) {
	created, _, err := client.GL().DeployKeys.AddDeployKey(projectID, &gl.AddDeployKeyOptions{
		Title:   new(title),
		Key:     new(publicKey),
		CanPush: new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return DeployKey{}, err
	}
	return DeployKey{ID: created.ID, Title: created.Title, Key: created.Key}, nil
}

// deleteDeployKey removes the key and tolerates one a case deleted.
func deleteDeployKey(ctx context.Context, client *gitlabclient.Client, projectID, keyID int64) error {
	_, err := client.GL().DeployKeys.DeleteDeployKey(projectID, keyID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting deploy key %d of project %d: %w", keyID, projectID, err)
	}
	return nil
}
