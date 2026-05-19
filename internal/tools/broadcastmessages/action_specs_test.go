// action_specs_test.go contains canonical-route tests for broadcast message delete behavior.
package broadcastmessages

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_DeleteError verifies that the delete route returns an error when the GitLab API fails.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := broadcastMessageSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_delete_broadcast_message"].Route.Handler(t.Context(), map[string]any{"id": float64(1)})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}
