package milestones

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestMilestoneGet_EmbedsCanonicalResource asserts gitlab_milestone_get
// attaches an EmbeddedResource block with URI
// gitlab://project/{id}/milestone/{iid}. The handler resolves IID to global
// ID via ListMilestones with iids filter, then GetMilestone, so two mock
// endpoints are required.
func TestMilestoneGet_EmbedsCanonicalResource(t *testing.T) {
	const listJSON = `[{"id":99,"iid":3,"project_id":42,"title":"M3","description":"","state":"active"}]`
	const getJSON = `{"id":99,"iid":3,"project_id":42,"title":"M3","description":"","state":"active"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/milestones/99"):
			testutil.RespondJSON(w, http.StatusOK, getJSON)
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/milestones"):
			testutil.RespondJSON(w, http.StatusOK, listJSON)
		default:
			http.NotFound(w, r)
		}
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "milestone_iid": 3}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_milestone_get", args, "gitlab://project/42/milestone/3", toolutil.EnableEmbeddedResources)
}
