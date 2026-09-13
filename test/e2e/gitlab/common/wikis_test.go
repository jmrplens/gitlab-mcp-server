//go:build e2e

// wikis_test.go covers a project's wiki through the server: a page's
// create, read, listing, update and delete, and an attachment uploaded to
// the wiki's own repository. Every surface writes a page of its own in one
// shared project, since the delete at the end consumes it.

package common

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// wikiSlugs lists the slugs of a wiki page listing.
func wikiSlugs(pages []wikis.Output) []string {
	slugs := make([]string, 0, len(pages))
	for _, page := range pages {
		slugs = append(slugs, page.Slug)
	}
	return slugs
}

// TestWiki_Lifecycle_CreateGetListUpdateDelete creates a wiki page on
// every surface, reads it back by its slug, finds it in the listing,
// rewrites its content, deletes it and checks the read is then refused.
//
// Replaces: TestIndividual_Wikis, TestMeta_Wikis
func TestWiki_Lifecycle_CreateGetListUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("wiki"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("page")
		content := "# " + title + "\n\nWritten by the e2e suite.\n"

		// The slug is what the page is addressed by and is the title GitLab
		// was given; the title it answers is the human form of that slug,
		// with the separators turned into spaces, so the title the call was
		// made with is asserted on the slug and the title is then held to
		// what the create answered.
		created := harness.Do[wikis.Output](s, actionWikiCreate, withParams(params, map[string]any{"title": title, "content": content}))
		if created.Slug != title || created.Title == "" {
			e.T.Fatalf("wiki create answered %+v, want the page slugged %q with a title", created, title)
		}
		pageParams := withParams(params, map[string]any{"slug": created.Slug})

		got := harness.Do[wikis.Output](s, actionWikiGet, pageParams)
		if got.Slug != created.Slug || got.Title != created.Title || got.Content != content {
			e.T.Errorf("wiki get answered %+v, want the page %q titled %q with the content just written", got, created.Slug, created.Title)
		}

		listed := harness.Do[wikis.ListOutput](s, actionWikiList, params)
		if !slices.Contains(wikiSlugs(listed.WikiPages), created.Slug) {
			e.T.Errorf("the project's wiki pages do not hold %q: %v", created.Slug, wikiSlugs(listed.WikiPages))
		}

		updated := harness.Do[wikis.Output](s, actionWikiUpdate, withParams(pageParams, map[string]any{"content": content + "\nUpdated.\n"}))
		if updated.Slug != created.Slug || updated.Content != content+"\nUpdated.\n" {
			e.T.Errorf("wiki update answered %+v, want the page %q with the text just written", updated, created.Slug)
		}

		harness.DoVoid(s, actionWikiDelete, pageParams)
		refused := harness.Refused(s, actionWikiGet, pageParams, harness.FailureNotFound)
		e.T.Logf("the read of the deleted page was refused: %s", firstLine(refused))
	})
}

// TestWiki_UploadAttachment_AnswersTheFileAndItsMarkdown creates a page so
// the wiki repository exists, then uploads a small text file to it on every
// surface and reads the file's name, path and Markdown link off the answer.
//
// Replaces: TestMeta_WikiUploadAttachment
func TestWiki_UploadAttachment_AnswersTheFileAndItsMarkdown(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("wikiupload"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}

		page := harness.Do[wikis.Output](s, actionWikiCreate, withParams(params, map[string]any{
			"title": e.Name("page"), "content": "The page the attachment hangs off.",
		}))
		if page.Slug == "" {
			e.T.Fatalf("wiki create answered %+v, want a page with a slug", page)
		}

		filename := e.Name("attachment") + ".txt"
		uploaded := harness.Do[wikis.AttachmentOutput](s, actionWikiUploadAttachment, withParams(params, map[string]any{
			"filename": filename, "content_base64": base64.StdEncoding.EncodeToString([]byte("attached by the e2e suite\n")),
		}))
		switch {
		case uploaded.FileName != filename:
			e.T.Errorf("upload_attachment answered %+v, want the file %q", uploaded, filename)
		case uploaded.FilePath == "" || !strings.HasSuffix(uploaded.FilePath, filename):
			e.T.Errorf("upload_attachment answered the path %q, want one ending in %q", uploaded.FilePath, filename)
		case !strings.Contains(uploaded.Markdown, filename):
			e.T.Errorf("upload_attachment answered the markdown %q, want a link naming %q", uploaded.Markdown, filename)
		}
	})
}
