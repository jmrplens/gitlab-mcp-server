//go:build e2e

// badge.go builds a project badge, the small object a badge case reads,
// renders or deletes.
//
// The two URLs are deliberately not addresses of anything: GitLab stores them
// as text and never fetches them, so pointing them at the fixture service
// would suggest a delivery that does not happen.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The badge's two URLs, spelled once so a test reading one back knows what
// it is comparing with.
const (
	badgeLinkURL  = "https://example.invalid/e2e/badge"
	badgeImageURL = "https://example.invalid/e2e/badge.svg"
)

// Badge is a project badge a builder created.
type Badge struct {
	// ID is what the badge actions take.
	ID int64
	// Name is what it was created as.
	Name string
	// LinkURL and ImageURL are the two halves GitLab stores.
	LinkURL  string
	ImageURL string
}

// NewProjectBadge adds a badge to the project and registers its deletion.
func NewProjectBadge(e *harness.Env, project Project) Badge {
	e.T.Helper()

	name := e.Name("badge")
	badge, err := retryTransient(e, "create project badge "+name, createRetries, func() (Badge, error) {
		return createProjectBadge(e.Ctx, e.Client(), project.ID, name)
	})
	if err != nil {
		e.T.Fatalf("creating badge %q in project %d: %v", name, project.ID, err)
	}

	e.Defer("project badge "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectBadge(ctx, e.Client(), project.ID, badge.ID)
	})
	return badge
}

// createProjectBadge asks GitLab for the badge.
func createProjectBadge(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) (Badge, error) {
	created, _, err := client.GL().ProjectBadges.AddProjectBadge(projectID, &gl.AddProjectBadgeOptions{
		Name:     new(name),
		LinkURL:  new(badgeLinkURL),
		ImageURL: new(badgeImageURL),
	}, gl.WithContext(ctx))
	if err != nil {
		return Badge{}, err
	}
	return Badge{ID: created.ID, Name: created.Name, LinkURL: created.LinkURL, ImageURL: created.ImageURL}, nil
}

// deleteProjectBadge removes the badge and tolerates one a case deleted.
func deleteProjectBadge(ctx context.Context, client *gitlabclient.Client, projectID, badgeID int64) error {
	_, err := client.GL().ProjectBadges.DeleteProjectBadge(projectID, badgeID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting badge %d of project %d: %w", badgeID, projectID, err)
	}
	return nil
}
