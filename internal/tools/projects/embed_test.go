package projects

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestProjectGet_EmbedsCanonicalResource asserts gitlab_project_get attaches
// an EmbeddedResource block with URI gitlab://project/{id}.
func TestProjectGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":42,"name":"P","path":"p","path_with_namespace":"g/p","default_branch":"main","web_url":"https://gitlab.example.com/g/p","visibility":"private"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_project_get", args, "gitlab://project/42", toolutil.EnableEmbeddedResources)
}
