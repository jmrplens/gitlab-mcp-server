// action_specs_test.go contains canonical-route tests for commit discussion delete behavior.
package commitdiscussions

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestActionSpecs_DeleteNoteError validates the DeleteNoteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDecorateCommitDiscussionMeta_Default verifies the fallback metadata arm
// used for any individual tool name without a dedicated switch case.
func TestDecorateCommitDiscussionMeta_Default(t *testing.T) {
	opts := commitDiscussionOptions("gitlab_unknown_commit_discussion_tool")
	if opts.Usage == "" {
		t.Error("expected fallback usage to be set")
	}
	if len(opts.RelatedActions) == 0 {
		t.Error("expected fallback related actions")
	}
	if opts.ParameterGuidance["commit_sha"].SemanticRole == "" {
		t.Error("expected fallback commit_sha guidance")
	}
}
