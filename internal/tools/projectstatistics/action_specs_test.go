// action_specs_test.go contains canonical route tests for project statistics.
package projectstatistics

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_CallRouteError covers the project statistics route error path.
func TestActionSpecs_CallRouteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{"project_id": "42"}); err == nil {
		t.Fatal("expected route error")
	}
}
