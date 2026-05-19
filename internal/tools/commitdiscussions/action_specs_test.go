// action_specs_test.go contains canonical-route tests for commit discussion delete behavior.
package commitdiscussions

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_DeleteNoteError verifies that the delete note route returns an error when the GitLab API fails.
func TestActionSpecs_DeleteNoteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := commitDiscussionSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_delete_commit_discussion_note"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "commit_sha": "abc123", "discussion_id": "d1", "note_id": 1})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}
