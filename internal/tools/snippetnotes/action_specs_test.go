// action_specs_test.go contains canonical-route tests for snippet note actions.
package snippetnotes

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// snippetDomain is the catalog group these actions are projected under,
// gitlab_snippet, spelled out here rather than read back from the package so
// the canonical-ID assertion cannot move with the thing it checks.
const snippetDomain = "snippet"

// TestActionSpecs_CallAllRoutes exercises every snippet note tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := snippetNotesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, snippetNotesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_snippet_note_list", map[string]any{"project_id": testProjectID, "snippet_id": 1}},
		{"gitlab_snippet_note_get", map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100}},
		{"gitlab_snippet_note_create", map[string]any{"project_id": testProjectID, "snippet_id": 1, "body": "test"}},
		{"gitlab_snippet_note_update", map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100, "body": "updated"}},
		{"gitlab_snippet_note_delete", map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100}},
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

// TestActionSpecs_DeleteError verifies that the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := snippetNotesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_snippet_note_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": "my-project", "snippet_id": 10, "note_id": 1,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := snippetNotesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, snippetNotesActionHandler())))

	result, err := byTool["gitlab_snippet_note_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID, "snippet_id": 1, "note_id": 100,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_snippet_note_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_snippet_note_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted note 100 from snippet 1 in project myproject." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := snippetNotesSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_snippet_note_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test snippet note destructive confirmation.",
		Icons:       toolutil.IconSnippet,
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
		Name:      "gitlab_snippet_note_delete",
		Arguments: map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

// TestActionSpecs_EachToolCarriesItsOwnDiscoveryMetadata holds every snippet
// note tool to the related actions and the parameter guidance of the action it
// actually routes.
//
// decorateSnippetNoteMeta picks all of it with one equality test per tool over
// a shared name, and nothing asserted any of it: a branch matching a sibling's
// name would advertise the delete tool as "Add a comment (note) to a project
// snippet", offer it guidance for a body it does not accept, and point a model
// at the wrong follow-up, with the whole suite still green. The wanted values
// are spelled out here rather than read back from the spec, so the assertion
// cannot move with the thing under test.
func TestActionSpecs_EachToolCarriesItsOwnDiscoveryMetadata(t *testing.T) {
	byTool := snippetNotesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t))))

	tests := []struct {
		tool string
		// wantGuidance is the complete set of parameters the action offers
		// guidance for. A tool that gains a sibling's parameter fails here.
		wantGuidance []string
		// wantRelated is the follow-up a model is sent to, in order.
		wantRelated []string
	}{
		{
			tool:         "gitlab_snippet_note_list",
			wantGuidance: []string{"project_id", "snippet_id", "order_by"},
			wantRelated:  []string{actionSnippetNoteGet, actionSnippetNoteCreate, actionSnippetGet},
		},
		{
			tool:         "gitlab_snippet_note_get",
			wantGuidance: []string{"project_id", "snippet_id", "note_id"},
			wantRelated:  []string{actionSnippetNoteList, actionSnippetNoteUpdate, actionSnippetNoteDelete},
		},
		{
			tool:         "gitlab_snippet_note_create",
			wantGuidance: []string{"project_id", "snippet_id", "body", "created_at"},
			wantRelated:  []string{actionSnippetGet, actionSnippetNoteList, actionSnippetNoteGet},
		},
		{
			tool:         "gitlab_snippet_note_update",
			wantGuidance: []string{"project_id", "snippet_id", "note_id", "body"},
			wantRelated:  []string{actionSnippetNoteGet, actionSnippetNoteList, actionSnippetNoteDelete},
		},
		{
			tool:         "gitlab_snippet_note_delete",
			wantGuidance: []string{"project_id", "snippet_id", "note_id"},
			wantRelated:  []string{actionSnippetNoteGet, actionSnippetNoteList, actionSnippetList},
		},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec := byTool[tt.tool]
			assertGuidanceParameters(t, spec, tt.wantGuidance)
			if !slices.Equal(spec.RelatedActions, tt.wantRelated) {
				t.Errorf("RelatedActions = %v, want %v", spec.RelatedActions, tt.wantRelated)
			}
			// A tool the switch never matched keeps the placeholder usage and
			// its own name as its only alias, which is what this rules out.
			if spec.Usage == genericSnippetNoteUsage {
				t.Errorf("Usage is still the placeholder %q", genericSnippetNoteUsage)
			}
			if slices.Equal(spec.Aliases, []string{tt.tool}) {
				t.Error("Aliases are still the tool's own name, so the tool is undecorated")
			}
			if !strings.Contains(spec.IndividualTool.Description, "See also:") {
				t.Errorf("IndividualTool.Description = %q, want a 'See also:' clause", spec.IndividualTool.Description)
			}
		})
	}
}

// assertGuidanceParameters verifies that spec offers parameter guidance for
// exactly want, each entry carrying something a model can act on.
func assertGuidanceParameters(t *testing.T, spec toolutil.ActionSpec, want []string) {
	t.Helper()
	got := slices.Sorted(maps.Keys(spec.ParameterGuidance))
	if wantSorted := slices.Sorted(slices.Values(want)); !slices.Equal(got, wantSorted) {
		t.Errorf("ParameterGuidance parameters = %v, want %v", got, wantSorted)
	}
	for _, param := range want {
		if spec.ParameterGuidance[param].ValueSource == "" {
			t.Errorf("ParameterGuidance[%q] has no ValueSource", param)
		}
	}
}

// TestActionSpecs_CanonicalIDs_NameTheActionsThisPackageBuilds ties the
// domain.action constants the RelatedActions are written from to the specs the
// package really returns.
//
// Nothing in the repository validates a related action: the discovery audit
// only counts an empty one, so a misspelled ID passes every gate and answers a
// model "unknown action" the moment it follows the hint. The two IDs naming
// the snippet itself belong to the snippets package and cannot be resolved
// from here.
func TestActionSpecs_CanonicalIDs_NameTheActionsThisPackageBuilds(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	built := make(map[string]bool, len(specs))
	for _, spec := range specs {
		built[snippetDomain+"."+spec.Name] = true
	}

	for _, id := range []string{
		actionSnippetNoteList, actionSnippetNoteGet, actionSnippetNoteCreate,
		actionSnippetNoteUpdate, actionSnippetNoteDelete,
	} {
		t.Run(id, func(t *testing.T) {
			if !built[id] {
				t.Errorf("%q names no action this package builds; it has %v", id, slices.Sorted(maps.Keys(built)))
			}
		})
	}
}

// TestDecorateSnippetNoteMeta_UnknownTool_KeepsTheGenericPlaceholder states
// that the decoration is keyed on the individual tool name and on nothing
// else: a name this package does not decorate is left with the catalog
// placeholder rather than handed a neighboring action's usage, guidance and
// follow-ups by a default branch.
func TestDecorateSnippetNoteMeta_UnknownTool_KeepsTheGenericPlaceholder(t *testing.T) {
	options := snippetNoteOptions("gitlab_snippet_note_unknown")

	if options.Usage != genericSnippetNoteUsage {
		t.Errorf("Usage = %q, want the placeholder %q", options.Usage, genericSnippetNoteUsage)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %v, want none", options.RelatedActions)
	}
	if len(options.ParameterGuidance) != 0 {
		t.Errorf("ParameterGuidance = %v, want none", options.ParameterGuidance)
	}
}

func snippetNotesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathSnippetNotes:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathSnippetNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
		case r.Method == http.MethodPost && path == pathSnippetNotes:
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)
		case r.Method == http.MethodPut && path == pathSnippetNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
		case r.Method == http.MethodDelete && path == pathSnippetNote100:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func snippetNotesSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
