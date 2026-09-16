//go:build e2e

// wiki.go builds a wiki page in a project.
//
// GitLab derives the slug from the title and does not echo the title back
// unchanged in every release, so the builder returns both and a case
// addresses the page by the slug GitLab answered with rather than by one
// computed here.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// wikiContent is what every fixture page holds.
const wikiContent = "# e2e wiki fixture\n\nThis page is built by the end-to-end fixture library.\n"

// WikiPage is a wiki page a builder created.
type WikiPage struct {
	// Slug is how every wiki action addresses it.
	Slug string
	// Title is what it was created as.
	Title string
	// Content is what it holds.
	Content string
}

// NewWikiPage creates a Markdown wiki page in the project and registers its
// deletion.
func NewWikiPage(e *harness.Env, project Project) WikiPage {
	e.T.Helper()

	title := e.Name("wiki")
	page, err := retryTransient(e, "create wiki page "+title, createRetries, func() (WikiPage, error) {
		return createWikiPage(e.Ctx, e.Client(), project.ID, title)
	})
	if err != nil {
		e.T.Fatalf("creating wiki page %q in project %d: %v", title, project.ID, err)
	}

	e.Defer("wiki page "+page.Slug, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteWikiPage(ctx, e.Client(), project.ID, page.Slug)
	})
	return page
}

// createWikiPage asks GitLab for the page.
func createWikiPage(ctx context.Context, client *gitlabclient.Client, projectID int64, title string) (WikiPage, error) {
	created, _, err := client.GL().Wikis.CreateWikiPage(projectID, &gl.CreateWikiPageOptions{
		Title:   new(title),
		Content: new(wikiContent),
		Format:  new(gl.WikiFormatMarkdown),
	}, gl.WithContext(ctx))
	if err != nil {
		return WikiPage{}, err
	}
	return WikiPage{Slug: created.Slug, Title: created.Title, Content: created.Content}, nil
}

// deleteWikiPage removes the page and tolerates one a case deleted.
func deleteWikiPage(ctx context.Context, client *gitlabclient.Client, projectID int64, slug string) error {
	_, err := client.GL().Wikis.DeleteWikiPage(projectID, slug, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting wiki page %q of project %d: %w", slug, projectID, err)
	}
	return nil
}
