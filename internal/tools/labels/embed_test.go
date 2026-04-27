package labels

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestLabelGet_EmbedsCanonicalResource asserts gitlab_label_get attaches
// an EmbeddedResource block with URI gitlab://project/{id}/label/{label_id}.
func TestLabelGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":5,"name":"bug","color":"#d9534f","description":"Bug report","open_issues_count":5,"open_merge_requests_count":1}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/labels/5") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "label_id": "5"}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_label_get", args, "gitlab://project/42/label/5", toolutil.EnableEmbeddedResources)
}
