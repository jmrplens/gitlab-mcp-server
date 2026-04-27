package groups

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestGroupGet_EmbedsCanonicalResource asserts gitlab_group_get attaches
// an EmbeddedResource block with URI gitlab://group/{id}.
func TestGroupGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":10,"name":"G","path":"g","full_path":"g","web_url":"https://gitlab.example.com/groups/g","visibility":"private"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"group_id": "10"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_group_get", args, "gitlab://group/10", toolutil.EnableEmbeddedResources)
}
