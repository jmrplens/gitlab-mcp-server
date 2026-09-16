//go:build e2e

// mirror.go configures a push mirror from one project to another.
//
// The target is a second project on the same instance, reached through the
// address GitLab itself can resolve from inside the Docker network, with the
// run's own token riding in the URL as the oauth2 user. A mirror pointed at
// anything outside would make the fixture depend on a network the run has no
// claim on, and the cases here ask about the mirror record rather than about
// what it pushed.
//
// The mirror is created disabled. An enabled one starts pushing as soon as
// GitLab notices it, which is a background job whose timing would decide what
// a case reads back.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Mirror is a push mirror a builder configured on a project.
type Mirror struct {
	// ID is what every mirror action takes.
	ID int64
	// Enabled is whether GitLab will push through it.
	Enabled bool
}

// NewProjectMirror creates a second project as the target, points a disabled
// push mirror of source at it and registers the mirror's removal.
func NewProjectMirror(e *harness.Env, source Project) Mirror {
	e.T.Helper()

	target := NewProject(e, WithNamePrefix("mirrortarget"))
	targetURL := MirrorTargetURL(e, target)

	mirror, err := retryTransient(e, "create project mirror", createRetries, func() (Mirror, error) {
		return createProjectMirror(e.Ctx, e.Client(), source.ID, targetURL)
	})
	if err != nil {
		e.T.Fatalf("creating a mirror of project %d: %v", source.ID, err)
	}

	e.Defer(fmt.Sprintf("mirror %d of project %s", mirror.ID, source.Path), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectMirror(ctx, e.Client(), source.ID, mirror.ID)
	})
	return mirror
}

// createProjectMirror asks GitLab for the mirror.
func createProjectMirror(ctx context.Context, client *gitlabclient.Client, projectID int64, targetURL string) (Mirror, error) {
	created, _, err := client.GL().ProjectMirrors.AddProjectMirror(projectID, &gl.AddProjectMirrorOptions{
		URL:     new(targetURL),
		Enabled: new(false),
	}, gl.WithContext(ctx))
	if err != nil {
		return Mirror{}, err
	}
	return Mirror{ID: created.ID, Enabled: created.Enabled}, nil
}

// deleteProjectMirror removes the mirror and tolerates one a case deleted.
func deleteProjectMirror(ctx context.Context, client *gitlabclient.Client, projectID, mirrorID int64) error {
	_, err := client.GL().ProjectMirrors.DeleteProjectMirror(projectID, mirrorID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting mirror %d of project %d: %w", mirrorID, projectID, err)
	}
	return nil
}
