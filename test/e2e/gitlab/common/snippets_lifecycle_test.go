//go:build e2e

// snippets_lifecycle_test.go covers the rest of a personal snippet's
// lifecycle through the server: the read, its content as a whole and as one
// file at a ref, the listings it does and does not appear in, the retitle,
// and the delete. The create, the delete and the read afterwards live in
// snippets_test.go, which an earlier port wrote.

package common

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The one file every snippet created here carries, and the branch a
// snippet repository is born with, which is what a file read names.
const (
	snippetFileName = "notes.md"
	snippetRef      = "main"
)

// snippetIDs lists the IDs of a snippet listing.
func snippetIDs(listed []snippets.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, snippet := range listed {
		ids = append(ids, snippet.ID)
	}
	return ids
}

// TestSnippet_Lifecycle_GetContentListUpdateDelete creates a private
// personal snippet with one file on every surface, reads it back, reads
// its content whole and as the one file at its ref, finds it among the
// caller's snippets and not among the public ones, retitles it, deletes it
// and checks the read is then refused.
//
// Replaces: TestIndividual_Snippets, TestMeta_Snippets, TestMeta_SnippetsPersonal
func TestSnippet_Lifecycle_GetContentListUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		title := e.Name("snippet")
		content := "# " + title + "\n"

		created := harness.Do[snippets.Output](s, actionSnippetCreate, map[string]any{
			"title": title, "visibility": "private",
			"files": []map[string]any{{"file_path": snippetFileName, "content": content}},
		})
		if created.ID == 0 || created.Title != title {
			e.T.Fatalf("snippet create answered %+v, want the snippet %q with an ID", created, title)
		}
		e.Defer("snippet "+title, func(ctx context.Context) error {
			_, err := e.Client().GL().Snippets.DeleteSnippet(created.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})
		params := map[string]any{"snippet_id": created.ID}

		got := harness.Do[snippets.Output](s, actionSnippetGet, params)
		if got.ID != created.ID || got.Title != title || got.Visibility != "private" {
			e.T.Errorf("snippet get answered %+v, want the private snippet %d titled %q", got, created.ID, title)
		}

		whole := harness.Do[snippets.ContentOutput](s, actionSnippetContent, params)
		if whole.SnippetID != created.ID || whole.Content != content {
			e.T.Errorf("snippet content answered %+v, want the content of snippet %d", whole, created.ID)
		}
		file := harness.Do[snippets.FileContentOutput](s, actionSnippetFileContent, withParams(params, map[string]any{"ref": snippetRef, "file_name": snippetFileName}))
		if file.SnippetID != created.ID || file.FileName != snippetFileName || file.Content != content {
			e.T.Errorf("snippet file_content answered %+v, want %s of snippet %d at %s", file, snippetFileName, created.ID, snippetRef)
		}

		mine := harness.Do[snippets.ListOutput](s, actionSnippetList, nil)
		if !containsID(snippetIDs(mine.Snippets), created.ID) {
			e.T.Errorf("the caller's snippets do not hold %d: %v", created.ID, snippetIDs(mine.Snippets))
		}
		public := harness.Do[snippets.ListOutput](s, actionSnippetExplore, nil)
		if containsID(snippetIDs(public.Snippets), created.ID) {
			e.T.Errorf("the public snippets hold the private snippet %d: %v", created.ID, snippetIDs(public.Snippets))
		}

		retitled := harness.Do[snippets.Output](s, actionSnippetUpdate, withParams(params, map[string]any{"title": "Updated " + title}))
		if retitled.ID != created.ID || retitled.Title != "Updated "+title {
			e.T.Errorf("snippet update answered %+v, want snippet %d retitled", retitled, created.ID)
		}

		harness.DoVoid(s, actionSnippetDelete, params)
		refused := harness.Refused(s, actionSnippetGet, params, harness.FailureNotFound)
		e.T.Logf("the read of the deleted snippet was refused: %s", firstLine(refused))
	})
}

// TestSnippet_ListAll_AnAdministratorSeesAPrivateOne lists every snippet
// of the instance on every surface, which only an administrator may, and
// finds the fixture's private personal snippet among them.
//
// Replaces: TestMeta_SnippetsPersonal
func TestSnippet_ListAll_AnAdministratorSeesAPrivateOne(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, fixture.NewSnippet, func(e *harness.Env, surface harness.Surface, snippet fixture.Snippet) {
		s := e.On(surface)

		listed := harness.Do[snippets.ListOutput](s, actionSnippetListAll, nil)
		if !containsID(snippetIDs(listed.Snippets), snippet.ID) {
			e.T.Errorf("the instance's snippets do not hold the private snippet %d: %v", snippet.ID, snippetIDs(listed.Snippets))
		}
	})
}
