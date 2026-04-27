package commits

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestCommitGet_EmbedsCanonicalResource asserts gitlab_commit_get attaches
// an EmbeddedResource block with URI gitlab://project/{id}/commit/{sha}.
func TestCommitGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":"abc123","short_id":"abc123","title":"T","message":"M","author_name":"A","author_email":"a@b","authored_date":"2026-01-01T00:00:00Z","committed_date":"2026-01-01T00:00:00Z","web_url":"https://gitlab.example.com/g/p/-/commit/abc123"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/commits/abc123") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "sha": "abc123"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_commit_get", args, "gitlab://project/42/commit/abc123", toolutil.EnableEmbeddedResources)
}
