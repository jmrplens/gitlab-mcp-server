//go:build e2e

// project_alias.go builds a project alias, and reserves the name a case
// creates one under.
//
// An alias names a project instance-wide, so the name cannot be a literal in
// a case: one case run three times against one instance would have the second
// and third attempts refused for a name the first took. Both halves are here
// for that reason, and the reservation registers the same removal the builder
// does, since what a case creates under the reserved name has to go too.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// ProjectAlias is an alias a builder created for a project.
type ProjectAlias struct {
	// Name is the alias itself, which every alias action takes.
	Name string
	// ProjectID is the project it points at.
	ProjectID int64
}

// NewProjectAlias creates an alias for the project and registers its removal.
func NewProjectAlias(e *harness.Env, project Project) ProjectAlias {
	e.T.Helper()

	name := ReserveProjectAliasName(e)
	alias, err := retryTransient(e, "create project alias "+name, createRetries, func() (ProjectAlias, error) {
		return createProjectAlias(e.Ctx, e.Client(), project.ID, name)
	})
	if err != nil {
		e.T.Fatalf("creating alias %q for project %d: %v", name, project.ID, err)
	}
	return alias
}

// ReserveProjectAliasName returns an alias name nothing on the instance has
// claimed and registers its removal, so a case that creates one leaves
// nothing behind.
//
// The removal is registered here rather than by the caller because the name is
// what identifies an alias: by the time a case has created one, the only
// handle on it is this string.
func ReserveProjectAliasName(e *harness.Env) string {
	e.T.Helper()

	name := e.Name("alias")
	e.Defer("project alias "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectAlias(ctx, e.Client(), name)
	})
	return name
}

// createProjectAlias asks GitLab for the alias.
func createProjectAlias(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) (ProjectAlias, error) {
	created, _, err := client.GL().ProjectAliases.CreateProjectAlias(&gl.CreateProjectAliasOptions{
		Name:      new(name),
		ProjectID: projectID,
	}, gl.WithContext(ctx))
	if err != nil {
		return ProjectAlias{}, err
	}
	return ProjectAlias{Name: created.Name, ProjectID: created.ProjectID}, nil
}

// deleteProjectAlias removes the alias and tolerates one a case deleted or
// never created.
func deleteProjectAlias(ctx context.Context, client *gitlabclient.Client, name string) error {
	_, err := client.GL().ProjectAliases.DeleteProjectAlias(name, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting project alias %q: %w", name, err)
	}
	return nil
}
