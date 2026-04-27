package pipelines

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestPipelineGet_EmbedsCanonicalResource asserts gitlab_pipeline_get
// attaches an EmbeddedResource block with URI
// gitlab://project/{id}/pipeline/{pipeline_id}.
func TestPipelineGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":100,"project_id":42,"status":"success","ref":"main","sha":"abc","web_url":"https://gitlab.example.com/g/p/-/pipelines/100"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/pipelines/100") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "pipeline_id": 100}
	testutil.AssertEmbeddedResource(t, session, ctx, "gitlab_pipeline_get", args, "gitlab://project/42/pipeline/100", toolutil.EnableEmbeddedResources)
}
