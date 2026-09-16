//go:build e2e

// external_status_check.go builds a project's external status check.
//
// The external URL is deliberately not an address of anything: GitLab posts
// to it when a merge request changes, and a fixture that pointed it at a
// service would make the check's state depend on whether that service
// answered. What the cases here need is the check record and its identifier,
// which a URL nothing serves gives just as well.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// externalStatusCheckURL is where GitLab would post the check. Nothing
// listens there.
const externalStatusCheckURL = "https://example.invalid/e2e/status-check"

// ExternalStatusCheck is a status check a builder put on a project.
type ExternalStatusCheck struct {
	// ID is what every status check action takes.
	ID int64
	// Name is what it was created as.
	Name string
	// ExternalURL is where GitLab would post.
	ExternalURL string
}

// NewExternalStatusCheck adds a status check to the project and registers its
// removal.
func NewExternalStatusCheck(e *harness.Env, project Project) ExternalStatusCheck {
	e.T.Helper()

	name := e.Name("statuscheck")
	check, err := retryTransient(e, "create external status check "+name, createRetries, func() (ExternalStatusCheck, error) {
		return createExternalStatusCheck(e.Ctx, e.Client(), project.ID, name)
	})
	if err != nil {
		e.T.Fatalf("creating status check %q in project %d: %v", name, project.ID, err)
	}

	e.Defer("external status check "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteExternalStatusCheck(ctx, e.Client(), project.ID, check.ID)
	})
	return check
}

// createExternalStatusCheck asks GitLab for the check.
func createExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) (ExternalStatusCheck, error) {
	created, _, err := client.GL().ExternalStatusChecks.CreateProjectExternalStatusCheck(projectID,
		&gl.CreateProjectExternalStatusCheckOptions{
			Name:        new(name),
			ExternalURL: new(externalStatusCheckURL),
		}, gl.WithContext(ctx))
	if err != nil {
		return ExternalStatusCheck{}, err
	}
	return ExternalStatusCheck{ID: created.ID, Name: created.Name, ExternalURL: created.ExternalURL}, nil
}

// deleteExternalStatusCheck removes the check and tolerates one a case
// deleted.
func deleteExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, projectID, checkID int64) error {
	_, err := client.GL().ExternalStatusChecks.DeleteProjectExternalStatusCheck(projectID, checkID,
		&gl.DeleteProjectExternalStatusCheckOptions{}, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting status check %d of project %d: %w", checkID, projectID, err)
	}
	return nil
}
