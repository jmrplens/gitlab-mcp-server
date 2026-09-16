//go:build e2e

// tag.go builds a repository tag, the object a tag case reads or deletes.
//
// The tag is annotated rather than lightweight: GitLab answers a lightweight
// tag with an empty message, and a case asking about the message would then
// be asking about nothing.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// tagMessage is what every fixture tag is annotated with.
const tagMessage = "e2e tag fixture"

// Tag is a tag a builder created.
type Tag struct {
	// Name is what every tag action takes.
	Name string
	// Ref is what it points at.
	Ref string
}

// NewTag tags the project's default branch and registers the tag's deletion.
func NewTag(e *harness.Env, project Project) Tag {
	e.T.Helper()

	name := e.Name("tag")
	tag, err := retryTransient(e, "create tag "+name, createRetries, func() (Tag, error) {
		return createTag(e.Ctx, e.Client(), project.ID, name, project.DefaultBranch)
	})
	if err != nil {
		e.T.Fatalf("creating tag %q in project %d: %v", name, project.ID, err)
	}

	e.Defer("tag "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteTag(ctx, e.Client(), project.ID, tag.Name)
	})
	return tag
}

// createTag asks GitLab for the tag.
func createTag(ctx context.Context, client *gitlabclient.Client, projectID int64, name, ref string) (Tag, error) {
	created, _, err := client.GL().Tags.CreateTag(projectID, &gl.CreateTagOptions{
		TagName: new(name),
		Ref:     new(ref),
		Message: new(tagMessage),
	}, gl.WithContext(ctx))
	if err != nil {
		return Tag{}, err
	}
	return Tag{Name: created.Name, Ref: ref}, nil
}

// deleteTag removes the tag and tolerates one a case deleted.
func deleteTag(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) error {
	_, err := client.GL().Tags.DeleteTag(projectID, name, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting tag %q of project %d: %w", name, projectID, err)
	}
	return nil
}

// TagExists reports whether the project still holds the tag, which is what a
// case that deletes one is verified against.
func TagExists(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) (bool, error) {
	_, _, err := client.GL().Tags.GetTag(projectID, name, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading tag %q of project %d: %w", name, projectID, err)
	}
	return true, nil
}
