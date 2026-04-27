package wikis

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestWikiGet_EmbedsCanonicalResource asserts gitlab_wiki_get attaches an
// EmbeddedResource block with URI gitlab://project/{id}/wiki/{slug}.
func TestWikiGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"title":"Home","slug":"Home","format":"markdown","content":"hello","encoding":"UTF-8"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/wikis/Home") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "slug": "Home"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_wiki_get", args, "gitlab://project/42/wiki/Home", toolutil.EnableEmbeddedResources)
}
