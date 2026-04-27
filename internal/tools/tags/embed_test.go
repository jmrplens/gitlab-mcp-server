package tags

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestTagGet_EmbedsCanonicalResource asserts gitlab_tag_get attaches an
// EmbeddedResource block with URI gitlab://project/{id}/tag/{name}.
func TestTagGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"name":"v1.0.0","message":"Release v1.0.0","target":"abcdef","protected":false,"commit":{"id":"abcdef","created_at":"2026-01-01T00:00:00Z"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/tags/v1.0.0") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "tag_name": "v1.0.0"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_tag_get", args, "gitlab://project/42/tag/v1.0.0", toolutil.EnableEmbeddedResources)
}
