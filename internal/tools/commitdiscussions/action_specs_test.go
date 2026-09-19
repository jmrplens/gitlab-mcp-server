// action_specs_test.go contains canonical-route tests for commit discussion delete behavior.
package commitdiscussions

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
)

// repositoryDomainIDs is the set of canonical IDs the gitlab_repository group
// really holds for the two packages a commit discussion cross-links to: this
// one and the commit package it points at for the commit itself.
//
// Both are built from their own ActionSpecs rather than listed here, because a
// list written beside the constants would agree with them whatever either said.
// They share one prefix because they share one catalog group, so there is no
// second spelling of the domain to drift either.
func repositoryDomainIDs(t *testing.T) map[string]bool {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	ids := map[string]bool{}
	for _, spec := range ActionSpecs(client) {
		ids[canonicalID(spec.Name)] = true
	}
	for _, spec := range commits.ActionSpecs(client) {
		ids[canonicalID(spec.Name)] = true
	}
	return ids
}

// TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds asserts that every
// related action a commit discussion spec publishes is an ID the catalog
// really holds.
//
// Sixteen of them named a commit_discussion domain that does not exist. These
// specs join the gitlab_repository group beside the commit ones, so the ID of
// every action here is "repository.commit_discussion_…"; the two cross-links
// that already read "repository." were right, which is what made the rest look
// deliberate. The spec names and the IDs are now one constant block, and this
// holds the published IDs to the specs the two packages register.
func TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	registered := repositoryDomainIDs(t)

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatalf("%s publishes no related actions", spec.Name)
			}
			for _, related := range spec.RelatedActions {
				if !registered[related] {
					t.Errorf("%s relates to %q, which the repository group holds no action under", spec.Name, related)
				}
			}
		})
	}
}

// TestDecorateCommitDiscussionMeta_UnknownTool_PublishesResolvableRelations
// asserts the fallback arm too.
//
// Every individual tool has an arm of its own, so the default arm is reached by
// nothing today and is exactly the kind of code a rename silently starts using.
// Its two cross-links are held to the same set as the rest.
func TestDecorateCommitDiscussionMeta_UnknownTool_PublishesResolvableRelations(t *testing.T) {
	registered := repositoryDomainIDs(t)

	options := commitDiscussionOptions("gitlab_commit_discussion_tool_that_does_not_exist")
	if len(options.RelatedActions) == 0 {
		t.Fatal("the fallback arm publishes no related actions")
	}
	for _, related := range options.RelatedActions {
		t.Run(related, func(t *testing.T) {
			if !registered[related] {
				t.Errorf("the fallback arm relates to %q, which the repository group holds no action under", related)
			}
		})
	}
}

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
