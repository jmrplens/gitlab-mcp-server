//go:build e2e

// snippets_project_test.go covers a project snippet's lifecycle through
// the server: create one with a file, find it in the project's listing,
// read it and its content, retitle it, delete it and check the read is
// refused, once per surface on one shared project.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestProjectSnippet_Lifecycle_CreateListGetContentUpdateDelete creates a
// private snippet in the fixture project on every surface, finds it in the
// listing, reads it and its content back, retitles it, deletes it and
// checks the read is then refused.
//
// Replaces: TestMeta_SnippetsProject
func TestProjectSnippet_Lifecycle_CreateListGetContentUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("projsnippet"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("snippet")
		content := "# " + title + "\n"

		created := harness.Do[snippets.Output](s, actionSnippetProjectCreate, withParams(params, map[string]any{
			"title": title, "visibility": "private",
			"files": []map[string]any{{"file_path": snippetFileName, "content": content}},
		}))
		if created.ID == 0 || created.Title != title {
			e.T.Fatalf("project_create answered %+v, want the snippet %q with an ID", created, title)
		}
		snippetParams := withParams(params, map[string]any{"snippet_id": created.ID})

		listed := harness.Do[snippets.ListOutput](s, actionSnippetProjectList, params)
		if !containsID(snippetIDs(listed.Snippets), created.ID) {
			e.T.Errorf("the project's snippets do not hold %d: %v", created.ID, snippetIDs(listed.Snippets))
		}

		got := harness.Do[snippets.Output](s, actionSnippetProjectGet, snippetParams)
		if got.ID != created.ID || got.Title != title {
			e.T.Errorf("project_get answered %+v, want snippet %d titled %q", got, created.ID, title)
		}
		whole := harness.Do[snippets.ContentOutput](s, actionSnippetProjectContent, snippetParams)
		if whole.SnippetID != created.ID || whole.Content != content {
			e.T.Errorf("project_content answered %+v, want the content of snippet %d", whole, created.ID)
		}

		retitled := harness.Do[snippets.Output](s, actionSnippetProjectUpdate, withParams(snippetParams, map[string]any{"title": "Updated " + title}))
		if retitled.ID != created.ID || retitled.Title != "Updated "+title {
			e.T.Errorf("project_update answered %+v, want snippet %d retitled", retitled, created.ID)
		}

		harness.DoVoid(s, actionSnippetProjectDelete, snippetParams)
		refused := harness.Refused(s, actionSnippetProjectGet, snippetParams, harness.FailureNotFound)
		e.T.Logf("the read of the deleted snippet was refused: %s", firstLine(refused))
	})
}
