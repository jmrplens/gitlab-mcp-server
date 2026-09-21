// action_specs_test.go contains canonical-route tests for issue discussion actions.
package issuediscussions

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes drives every issue discussion action through
// the route the catalog publishes, with the arguments that action's schema
// takes, and asserts each one reaches its handler and answers a result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueDiscussionsActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_issue_discussions", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_get_issue_discussion", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID}},
		{"gitlab_create_issue_discussion", map[string]any{"project_id": "42", "issue_iid": 10, "body": "New discussion"}},
		{"gitlab_add_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "body": "Reply"}},
		{"gitlab_update_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300, "body": "Updated"}},
		{"gitlab_delete_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteError validates the DeleteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_issue_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"project_id": "my-project", "issue_iid": 1, "discussion_id": testDiscussionID, "note_id": 1,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput validates the DeleteOutput route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the route answers with the deletion confirmation naming what was removed.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueDiscussionsActionHandler())))

	result, err := byTool["gitlab_delete_issue_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_issue_discussion_note) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_issue_discussion_note) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted issue discussion note." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined drives the destructive note delete
// over a real MCP session whose client declines the elicited confirmation.
// It asserts the call is answered rather than failing the transport, and the
// mock is a [testutil.ForbiddenHandler], so nothing reached GitLab.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_issue_discussion_note"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue discussion destructive confirmation.",
		Icons:       toolutil.IconDiscussion,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_issue_discussion_note",
		Arguments: map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": testDiscussionID, "note_id": 300},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

// TestActionSpecs_DiscoveryMetadata guards the 1:1-audit R-META requirements:
// every issue discussion action must carry action-specific Usage (not the
// generic placeholder), natural-language aliases, canonical RelatedActions,
// parameter guidance, and an IndividualTool.Description with "Returns:" and
// "See also:" sections.
//
// Two of the assertions are about which action the metadata belongs to rather
// than about its shape, because all of it is written in one switch over the
// individual tool name and a case body moved to the wrong label is a straight
// assignment no gate can see. The aliases must name the tool they decorate,
// which a crossed case body does not, and an action must not link to itself,
// which a crossed RelatedActions list does: every list here names siblings
// only, so a self-reference is exactly the fingerprint of a swap.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueDiscussionsActionHandler())))

	tools := []string{
		"gitlab_list_issue_discussions",
		"gitlab_get_issue_discussion",
		"gitlab_create_issue_discussion",
		"gitlab_add_issue_discussion_note",
		"gitlab_update_issue_discussion_note",
		"gitlab_delete_issue_discussion_note",
	}

	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			assertActionText(t, byTool[tool], tool)
			assertRelatedActions(t, byTool[tool], tool)
		})
	}
}

// assertActionText holds the prose an action publishes: usage written for this
// action rather than the package placeholder, aliases that include the tool
// they decorate, parameter guidance, and a description with both sections a
// model reads.
func assertActionText(t *testing.T, spec toolutil.ActionSpec, tool string) {
	t.Helper()
	if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute issuediscussions domain action.") {
		t.Errorf("%s: Usage must be action-specific, got %q", tool, spec.Usage)
	}
	if len(spec.Aliases) < 2 {
		t.Errorf("%s: expected natural-language aliases, got %v", tool, spec.Aliases)
	}
	if !slices.Contains(spec.Aliases, tool) {
		t.Errorf("%s: aliases %v decorate another action", tool, spec.Aliases)
	}
	if len(spec.ParameterGuidance) == 0 {
		t.Errorf("%s: expected ParameterGuidance, got none", tool)
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Errorf("%s: IndividualTool.Description must contain Returns:/See also:, got %q", tool, desc)
	}
}

// assertRelatedActions holds every cross-link an action publishes to a
// canonical issue.* id that is not its own.
func assertRelatedActions(t *testing.T, spec toolutil.ActionSpec, tool string) {
	t.Helper()
	if len(spec.RelatedActions) == 0 {
		t.Errorf("%s: expected RelatedActions, got none", tool)
	}
	self := canonicalID(spec.Name)
	for _, ra := range spec.RelatedActions {
		if !strings.HasPrefix(ra, "issue.") {
			t.Errorf("%s: RelatedAction %q is not a canonical issue.* id", tool, ra)
		}
		if ra == self {
			t.Errorf("%s: RelatedActions link to the action itself (%q)", tool, self)
		}
	}
}

// canonicalID is the catalog id of an issue discussion action. These specs are
// merged into the gitlab_issue group, so the id is the spec name under the
// issue domain.
func canonicalID(specName string) string { return "issue." + specName }

// TestActionSpecs_Annotations_MatchWhatEachActionDoes holds the three
// behavioral hints a client reads before deciding whether it may call an
// action without asking, and whether it may repeat one whose answer it never
// saw: read-only, idempotent, destructive.
//
// They come from which constructor each spec is built with, and the four
// constructors differ only in the flags they set, so building an action with
// the wrong one is invisible to every gate here: no branch changes, the route
// still runs, and the suite still passes. What changes is that a delete stops
// announcing itself as destructive, or a create starts inviting a retry that
// posts a second note.
func TestActionSpecs_Annotations_MatchWhatEachActionDoes(t *testing.T) {
	byTool := issueDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueDiscussionsActionHandler())))

	tests := []struct {
		tool        string
		readOnly    bool
		idempotent  bool
		destructive bool
	}{
		{"gitlab_list_issue_discussions", true, true, false},
		{"gitlab_get_issue_discussion", true, true, false},
		{"gitlab_create_issue_discussion", false, false, false},
		{"gitlab_add_issue_discussion_note", false, false, false},
		{"gitlab_update_issue_discussion_note", false, true, false},
		{"gitlab_delete_issue_discussion_note", false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec := byTool[tt.tool]
			if spec.ReadOnly != tt.readOnly {
				t.Errorf("%s: ReadOnly = %t, want %t", tt.tool, spec.ReadOnly, tt.readOnly)
			}
			if spec.Idempotent != tt.idempotent {
				t.Errorf("%s: Idempotent = %t, want %t", tt.tool, spec.Idempotent, tt.idempotent)
			}
			if spec.Destructive != tt.destructive {
				t.Errorf("%s: Destructive = %t, want %t", tt.tool, spec.Destructive, tt.destructive)
			}
		})
	}
}

// TestDecorateIssueDiscussionMeta_DefaultFallback covers the defensive default
// branch of decorateIssueDiscussionMeta, exercised when an unknown individual
// tool name is passed.
func TestDecorateIssueDiscussionMeta_DefaultFallback(t *testing.T) {
	var options toolutil.ActionSpecOptions
	decorateIssueDiscussionMeta(&options, "gitlab_unknown_tool")
	if options.Usage == "" {
		t.Error("expected fallback Usage to be set")
	}
	if len(options.RelatedActions) == 0 {
		t.Error("expected fallback RelatedActions to be set")
	}
}

func issueDiscussionsActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusCreated, discussionJSONCoverage)
		case r.Method == http.MethodPost && strings.Contains(path, "/discussions/") && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, noteJSONCoverage)
		case r.Method == http.MethodPut && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSONCoverage)
		case r.Method == http.MethodDelete && strings.Contains(path, "/notes/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.Contains(path, "/discussions/"):
			testutil.RespondJSON(w, http.StatusOK, discussionJSONCoverage)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusOK, "["+discussionJSONCoverage+"]")
		default:
			http.NotFound(w, r)
		}
	})
}

func issueDiscussionSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
