package releases

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestReleaseGet_EmbedsCanonicalResource asserts gitlab_release_get attaches
// an EmbeddedResource block with URI gitlab://project/{id}/release/{tag}
// and application/json MIME type when the embed toggle is enabled, and
// omits the block when it is disabled.
func TestReleaseGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"tag_name":"v1.0.0","name":"R","description":"","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-02T00:00:00Z","author":{"username":"alice"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/releases/v1.0.0") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "tag_name": "v1.0.0"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_release_get", args, "gitlab://project/42/release/v1.0.0", toolutil.EnableEmbeddedResources)
}
