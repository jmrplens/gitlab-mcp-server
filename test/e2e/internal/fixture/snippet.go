//go:build e2e

// snippet.go builds a personal snippet, which is the one object a snippet
// storage move addresses and nothing else in the suite needs.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
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
func NewSnippet(e *harness.Env) Snippet {
	e.T.Helper()

	title := e.Name("snippet")
	snippet, err := retryTransient(e, "create snippet "+title, createRetries, func() (Snippet, error) {
		created, _, err := e.Client().GL().Snippets.CreateSnippet(&gl.CreateSnippetOptions{
			Title:       new(title),
			FileName:    new("e2e.txt"),
			Description: new("e2e: " + e.T.Name()),
			Content:     new("e2e snippet fixture\n"),
			Visibility:  new(gl.PrivateVisibility),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Snippet{}, err
		}
		return Snippet{ID: created.ID, Title: created.Title}, nil
	})
	if err != nil {
		e.T.Fatalf("creating snippet %q: %v", title, err)
	}

	e.Defer("snippet "+title, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, deleteErr := e.Client().GL().Snippets.DeleteSnippet(snippet.ID, gl.WithContext(ctx))
		if deleteErr != nil && !IsStatus(deleteErr, http.StatusNotFound) {
			return fmt.Errorf("deleting snippet %d: %w", snippet.ID, deleteErr)
		}
		return nil
	})
	return snippet
}
