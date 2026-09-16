//go:build e2e

// feature_flag.go builds a project feature flag.
//
// It is the project-scoped object of the feature flags API, which is a
// different thing from the instance-global switch feature.go pins: that one
// decides what GitLab serves and needs an administrator, this one is content
// a project owns and needs nothing but the project. The two live in separate
// files because reading them as one is exactly the mistake the names invite.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The shape every fixture flag is created in. Version 2 is the only one
// GitLab still creates, and a default strategy is what makes the flag valid
// without naming an environment.
const (
	featureFlagVersion      = "new_version_flag"
	featureFlagStrategyName = "default"
	featureFlagScope        = "*"
)

// ProjectFeatureFlag is a feature flag a builder created on a project.
type ProjectFeatureFlag struct {
	// Name is how every feature flag action addresses it.
	Name string
	// Active says whether it was created switched on.
	Active bool
}

// NewProjectFeatureFlag creates an active feature flag on the project with one
// default strategy, and registers its deletion.
func NewProjectFeatureFlag(e *harness.Env, project Project) ProjectFeatureFlag {
	e.T.Helper()

	name := e.Name("flag")
	flag, err := retryTransient(e, "create feature flag "+name, createRetries, func() (ProjectFeatureFlag, error) {
		return createProjectFeatureFlag(e.Ctx, e.Client(), project.ID, name)
	})
	if err != nil {
		e.T.Fatalf("creating feature flag %q in project %d: %v", name, project.ID, err)
	}

	e.Defer("feature flag "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectFeatureFlag(ctx, e.Client(), project.ID, flag.Name)
	})
	return flag
}

// createProjectFeatureFlag asks GitLab for the flag.
func createProjectFeatureFlag(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) (ProjectFeatureFlag, error) {
	scopes := []*gl.CreateProjectFeatureFlagScopeOptions{{EnvironmentScope: new(featureFlagScope)}}
	strategies := []*gl.CreateFeatureFlagStrategyOptions{{
		Name:   new(featureFlagStrategyName),
		Scopes: &scopes,
	}}
	created, _, err := client.GL().ProjectFeatureFlags.CreateProjectFeatureFlag(projectID, &gl.CreateProjectFeatureFlagOptions{
		Name:        new(name),
		Description: new("e2e feature flag fixture"),
		Version:     new(featureFlagVersion),
		Active:      new(true),
		Strategies:  &strategies,
	}, gl.WithContext(ctx))
	if err != nil {
		return ProjectFeatureFlag{}, err
	}
	return ProjectFeatureFlag{Name: created.Name, Active: created.Active}, nil
}

// deleteProjectFeatureFlag removes the flag and tolerates one a case deleted.
func deleteProjectFeatureFlag(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) error {
	_, err := client.GL().ProjectFeatureFlags.DeleteProjectFeatureFlag(projectID, name, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting feature flag %q of project %d: %w", name, projectID, err)
	}
	return nil
}
