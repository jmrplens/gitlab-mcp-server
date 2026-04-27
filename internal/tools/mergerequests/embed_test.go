package mergerequests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestMRGet_EmbedsCanonicalResource asserts gitlab_mr_get attaches an
// EmbeddedResource block with URI gitlab://project/{id}/mr/{iid}.
func TestMRGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":100,"iid":5,"project_id":42,"title":"T","description":"","state":"opened","source_branch":"f","target_branch":"main","author":{"username":"a"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/merge_requests/5") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "mr_iid": 5}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_mr_get", args, "gitlab://project/42/mr/5", toolutil.EnableEmbeddedResources)
}
