package branches

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestBranchGet_EmbedsCanonicalResource asserts gitlab_branch_get attaches
// an EmbeddedResource block with URI gitlab://project/{id}/branch/{name}.
func TestBranchGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"name":"main","protected":true,"merged":false,"default":true,"web_url":"https://gitlab.example.com/p/-/tree/main","commit":{"id":"abc"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/branches/main") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "branch_name": "main"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_branch_get", args, "gitlab://project/42/branch/main", toolutil.EnableEmbeddedResources)
}
