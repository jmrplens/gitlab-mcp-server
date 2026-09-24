//go:build e2e

// snippet.go builds a personal snippet, which is the one object a snippet
// storage move addresses and the one the gitlab://snippet/{snippet_id}
// resource reads.
//
// A personal snippet belongs to the user rather than to a project or a group,
// so nothing that is torn down takes it along: whoever makes one deletes it,
// and the sweep is what finds one whose deletion never ran.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The one file every personal snippet fixture carries.
const (
	snippetFileName = "e2e.txt"
	snippetContent  = "e2e snippet fixture\n"
)

// Snippet is a personal snippet a builder created.
type Snippet struct {
	// ID is what the snippet actions take.
	ID int64
	// Title is what it was created as.
	Title string
}

// NewSnippet creates a private personal snippet with one file and registers
// its deletion on the Env.
//
// It arms the run's exit sweep like the builders of projects, groups and
// users do: a package whose only lasting fixture is a personal snippet would
// otherwise have none, and a deletion that failed would leave the snippet on
// the instance with nothing to say so.
func NewSnippet(e *harness.Env) Snippet {
	e.T.Helper()
	armSweep(e)

	title := e.Name("snippet")
	snippet, err := retryTransient(e, "create snippet "+title, createRetries, func() (Snippet, error) {
		return createPersonalSnippet(e.Ctx, e.Client(), title, "e2e: "+e.T.Name())
	})
	if err != nil {
		e.T.Fatalf("creating snippet %q: %v", title, err)
	}

	e.Defer("snippet "+title, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deletePersonalSnippet(ctx, e.Client(), snippet.ID, snippet.Title)
	})
	return snippet
}

// createPersonalSnippet asks GitLab for a private personal snippet carrying
// the one fixture file.
func createPersonalSnippet(ctx context.Context, client *gitlabclient.Client, title, description string) (Snippet, error) {
	created, _, err := client.GL().Snippets.CreateSnippet(&gl.CreateSnippetOptions{
		Title:       new(title),
		FileName:    new(snippetFileName),
		Description: new(description),
		Content:     new(snippetContent),
		Visibility:  new(gl.PrivateVisibility),
	}, gl.WithContext(ctx))
	if err != nil {
		return Snippet{}, err
	}
	return Snippet{ID: created.ID, Title: created.Title}, nil
}

// deletePersonalSnippet removes the snippet and tolerates one a case deleted.
// A refusal names the snippet by its title as well as its ID, since the title
// is what says which run and which test it belonged to.
func deletePersonalSnippet(ctx context.Context, client *gitlabclient.Client, id int64, title string) error {
	_, err := client.GL().Snippets.DeleteSnippet(id, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting snippet %d (%s): %w", id, title, err)
	}
	return nil
}
