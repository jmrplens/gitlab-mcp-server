//go:build e2e

// snippets_test.go covers a personal snippet's create and delete through
// the server, which the old suite made only to build the snippet its
// storage move scenario stood on.

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

// TestSnippet_CreateAndDelete_GoneAfterwards creates a private personal
// snippet with one file on every surface, deletes it and checks the read
// is then refused. The fixture library's deletion is registered behind the
// server's, for a delete that did not happen.
//
// Replaces: TestMeta_StorageMoves
func TestSnippet_CreateAndDelete_GoneAfterwards(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		title := e.Name("snippet")

		created := harness.Do[snippets.Output](s, actionSnippetCreate, map[string]any{
			"title": title, "visibility": "private",
			"files": []map[string]any{{"file_path": "notes.md", "content": "# " + title + "\n"}},
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

		harness.DoVoid(s, actionSnippetDelete, map[string]any{"snippet_id": created.ID})
		refused := harness.Refused(s, actionSnippetGet, map[string]any{"snippet_id": created.ID}, harness.FailureNotFound)
		e.T.Logf("the read of the deleted snippet was refused: %s", firstLine(refused))
	})
}
