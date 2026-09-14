//go:build e2e

// release.go builds a release, the object a group's release listing
// aggregates over its projects. A release stands on a tag, so the builder
// makes the tag first, on the project's default branch.

package fixture

import (
	"context"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// releaseAlreadyExists is what GitLab answers a release create for a tag that
// already carries one, from Releases::CreateService.
const releaseAlreadyExists = "Release already exists"

// Release is a release a builder created, as a test refers to it.
type Release struct {
	// TagName is the tag the release stands on, which is also how the
	// release actions address it.
	TagName string
	// Name is the release's title.
	Name string
}

// NewRelease creates a tag on the project's default branch under a name
// scoped to the run, and a release on it. Both go with the project, so
// nothing is registered.
func NewRelease(e *harness.Env, project Project, prefix string) Release {
	e.T.Helper()

	tagName := e.Name(prefix)
	release, err := retryTransient(e, "create release "+tagName, createRetries, func() (Release, error) {
		return createRelease(e.Ctx, e.Client(), project.ID, tagName, project.DefaultBranch, "e2e: "+e.T.Name())
	})
	if err != nil {
		e.T.Fatalf("creating a release on tag %q in project %d: %v", tagName, project.ID, err)
	}
	return release
}

// createRelease makes the tag and then the release standing on it, which is
// the whole of what NewRelease asks GitLab for.
//
// The order is the point: GitLab refuses a release whose tag does not
// exist, and answers that refusal as a 404 about the tag rather than about
// the release, so a builder that sent the two the other way round would
// fail with a message about the wrong object.
func createRelease(ctx context.Context, client *gitlabclient.Client, projectID int64, tagName, ref, description string) (Release, error) {
	_, _, err := client.GL().Tags.CreateTag(projectID, &gl.CreateTagOptions{
		TagName: new(tagName),
		Ref:     new(ref),
	}, gl.WithContext(ctx))
	// A tag this call already made is not a failure: the whole pair is
	// retried as one, so the second attempt of a release whose first
	// attempt failed after the tag finds the tag there and carries on.
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return Release{}, fmt.Errorf("creating tag %q: %w", tagName, err)
	}

	name := "Release " + tagName
	created, _, err := client.GL().Releases.CreateRelease(projectID, &gl.CreateReleaseOptions{
		TagName:     new(tagName),
		Name:        new(name),
		Description: new(description),
	}, gl.WithContext(ctx))
	// A release this call already made is not a failure either, for the same
	// reason the tag is not: an attempt that created the release and then lost
	// the answer is retried, and GitLab refuses the second one with the 409
	// Releases::CreateService answers. The existing release is what the caller
	// asked for, so it is read back rather than reported.
	if err != nil && strings.Contains(err.Error(), releaseAlreadyExists) {
		existing, _, getErr := client.GL().Releases.GetRelease(projectID, tagName, gl.WithContext(ctx))
		if getErr != nil {
			return Release{}, fmt.Errorf("reading the release on tag %q that already exists: %w", tagName, getErr)
		}
		return Release{TagName: existing.TagName, Name: existing.Name}, nil
	}
	if err != nil {
		return Release{}, err
	}
	return Release{TagName: created.TagName, Name: created.Name}, nil
}
