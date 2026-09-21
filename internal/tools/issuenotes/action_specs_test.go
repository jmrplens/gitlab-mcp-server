// action_specs_test.go contains canonical-route tests for issue note actions.
package issuenotes

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// genericIssueNoteUsage is the placeholder Usage issueNoteOptions starts every
// action with, before decorateIssueNoteMeta replaces it with the action's own.
// Both the metadata guard and the unknown-tool test below are about this exact
// string, so it is named once here rather than spelled in each.
const genericIssueNoteUsage = "Use to execute issuenotes domain action."

// TestActionSpecs_DiscoveryMetadata guards the 1:1 audit R-META metadata: every
// issue note action must have a non-generic Usage, natural-language aliases,
// a RelatedActions list naming its nearest sibling, parameter guidance for the
// inputs a model has to bind, and a "Returns: … See also: …" individual-tool
// description.
//
// It asks whether the metadata is present and not whether each ID resolves:
// that is make check-action-ids, over the whole catalog, which a package-local
// test cannot answer because the catalog is not built here.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	const genericUsage = genericIssueNoteUsage

	tests := []struct {
		tool         string
		wantAlias    string
		wantRelated  string
		guidedParams []string
	}{
		{"gitlab_issue_note_create", "comment on issue", actionIssueNoteList, []string{"project_id", "issue_iid", "body"}},
		{"gitlab_issue_note_list", "list issue comments", actionIssueNoteGet, []string{"project_id", "issue_iid", "order_by"}},
		{"gitlab_issue_note_get", "get issue comment", actionIssueNoteList, []string{"project_id", "issue_iid", "note_id"}},
		{"gitlab_issue_note_update", "edit issue comment", actionIssueNoteGet, []string{"project_id", "issue_iid", "note_id", "body"}},
		{"gitlab_issue_note_delete", "delete issue comment", actionIssueNoteGet, []string{"project_id", "issue_iid", "note_id"}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec := byTool[tt.tool]
			if spec.Usage == "" || spec.Usage == genericUsage {
				t.Errorf("Usage = %q, want action-specific", spec.Usage)
			}
			if !slices.Contains(spec.Aliases, tt.wantAlias) {
				t.Errorf("Aliases = %v, want to contain %q", spec.Aliases, tt.wantAlias)
			}
			if !slices.Contains(spec.RelatedActions, tt.wantRelated) {
				t.Errorf("RelatedActions = %v, want to contain %q", spec.RelatedActions, tt.wantRelated)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("IndividualTool.Description = %q, want Returns:/See also:", desc)
			}
			for _, p := range tt.guidedParams {
				if _, ok := spec.ParameterGuidance[p]; !ok {
					t.Errorf("ParameterGuidance missing %q", p)
				}
			}
		})
	}
}

// TestActionSpecs_Classification_MatchesTheActionItRoutes pins the three
// booleans the spec constructor sets, which nothing else here reads back.
// Choosing the wrong constructor in ActionSpecs is a straight-line
// substitution with no branch to flip, so both gates stay green while a write
// is published as a read: note_create built with issueNoteReadSpec passed the
// whole suite until this test, and ReadOnly is exactly what --read-only and a
// read_api token narrow the surface by, so such a spec keeps posting comments
// on a deployment that asked to serve reads only. Idempotent goes with it
// because a create announced as idempotent invites a model to retry a call
// whose outcome it could not observe, leaving two copies of one comment on the
// issue.
func TestActionSpecs_Classification_MatchesTheActionItRoutes(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	tests := []struct {
		tool        string
		readOnly    bool
		destructive bool
		idempotent  bool
	}{
		{tool: "gitlab_issue_note_create"},
		{tool: "gitlab_issue_note_list", readOnly: true, idempotent: true},
		{tool: "gitlab_issue_note_get", readOnly: true, idempotent: true},
		{tool: "gitlab_issue_note_update", idempotent: true},
		{tool: "gitlab_issue_note_delete", destructive: true, idempotent: true},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec := byTool[tt.tool]
			if spec.ReadOnly != tt.readOnly {
				t.Errorf("ReadOnly = %v, want %v", spec.ReadOnly, tt.readOnly)
			}
			if spec.Destructive != tt.destructive {
				t.Errorf("Destructive = %v, want %v", spec.Destructive, tt.destructive)
			}
			if spec.Idempotent != tt.idempotent {
				t.Errorf("Idempotent = %v, want %v", spec.Idempotent, tt.idempotent)
			}
		})
	}
}

// TestActionSpecs_ParameterGuidance_DescribesTheParameterItIsKeyedBy holds each
// guidance block against the key it hangs from. TestActionSpecs_DiscoveryMetadata
// above only asks whether a key is present, so until this test the three shared
// blocks could trade places in a map literal and nothing failed — and crossing
// issue_iid with note_id tells a model to put the comment ID where the issue
// number goes, which is a 404 it has no way to read as its own mistake.
// The two body blocks are checked
// for the same reason one level down: create's says what to post and update's
// says the text replaces the whole note, so a swap turns the warning against
// appending into advice on a parameter that appends to nothing.
func TestActionSpecs_ParameterGuidance_DescribesTheParameterItIsKeyedBy(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	shared := []struct {
		tool  string
		param string
		want  toolutil.ParameterGuidance
	}{
		{"gitlab_issue_note_create", "project_id", projectIDGuidance()},
		{"gitlab_issue_note_create", "issue_iid", issueIIDGuidance()},
		{"gitlab_issue_note_list", "project_id", projectIDGuidance()},
		{"gitlab_issue_note_list", "issue_iid", issueIIDGuidance()},
		{"gitlab_issue_note_get", "project_id", projectIDGuidance()},
		{"gitlab_issue_note_get", "issue_iid", issueIIDGuidance()},
		{"gitlab_issue_note_get", "note_id", noteIDGuidance()},
		{"gitlab_issue_note_update", "project_id", projectIDGuidance()},
		{"gitlab_issue_note_update", "issue_iid", issueIIDGuidance()},
		{"gitlab_issue_note_update", "note_id", noteIDGuidance()},
		{"gitlab_issue_note_delete", "project_id", projectIDGuidance()},
		{"gitlab_issue_note_delete", "issue_iid", issueIIDGuidance()},
		{"gitlab_issue_note_delete", "note_id", noteIDGuidance()},
	}
	for _, tt := range shared {
		t.Run(tt.tool+"/"+tt.param, func(t *testing.T) {
			got := byTool[tt.tool].ParameterGuidance[tt.param]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParameterGuidance[%q] = %+v, want %+v", tt.param, got, tt.want)
			}
		})
	}

	// The per-action blocks are written inline, so they are pinned by the one
	// sentence that tells them apart rather than by a second copy of the literal.
	inline := []struct {
		tool            string
		param           string
		wantValueSource string
	}{
		{"gitlab_issue_note_create", "body", "The comment text the user wants to post. Markdown is supported."},
		{"gitlab_issue_note_create", "internal", "Set true only when the user asks for a private/internal note."},
		{"gitlab_issue_note_list", "order_by", "Field to order notes by: created_at or updated_at."},
		{"gitlab_issue_note_update", "body", "The new comment text that replaces the existing note body. Markdown is supported."},
	}
	for _, tt := range inline {
		t.Run(tt.tool+"/"+tt.param, func(t *testing.T) {
			got := byTool[tt.tool].ParameterGuidance[tt.param]
			if got.ValueSource != tt.wantValueSource {
				t.Errorf("ParameterGuidance[%q].ValueSource = %q, want %q", tt.param, got.ValueSource, tt.wantValueSource)
			}
		})
	}
}

// TestIssueNoteOptions_UnknownTool_KeepsTheGenericDefaults states what a tool
// name decorateIssueNoteMeta does not name is left with.
//
// That switch is keyed on five string literals, and nothing ties them to the
// names ActionSpecs hands it: renaming a tool in one place and not the other
// makes its case unreachable, silently. The metadata guard above cannot see
// that, because it looks the spec up by the same name the switch is keyed on
// and would simply stop finding the action. What this pins is that the
// fallthrough leaves the generic placeholder Usage, the bare name as the only
// alias, and no related actions, guidance or individual-tool description —
// the shape cmd/audit_discovery_completeness reports as an undecorated action
// — rather than carrying over whatever the previous case had set.
func TestIssueNoteOptions_UnknownTool_KeepsTheGenericDefaults(t *testing.T) {
	const renamed = "gitlab_issue_note_renamed"

	options := issueNoteOptions(renamed)

	if options.Usage != genericIssueNoteUsage {
		t.Errorf("Usage = %q, want the generic placeholder %q", options.Usage, genericIssueNoteUsage)
	}
	if !slices.Equal(options.Aliases, []string{renamed}) {
		t.Errorf("Aliases = %v, want only %q", options.Aliases, renamed)
	}
	if options.RelatedActions != nil {
		t.Errorf("RelatedActions = %v, want none", options.RelatedActions)
	}
	if options.ParameterGuidance != nil {
		t.Errorf("ParameterGuidance = %v, want none", options.ParameterGuidance)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("IndividualTool.Description = %q, want empty", options.IndividualTool.Description)
	}
}

// TestActionSpecs_CallAllRoutes exercises every issue note tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_note_create", map[string]any{"project_id": testProjectID, "issue_iid": 10, "body": "test note"}},
		{"gitlab_issue_note_list", map[string]any{"project_id": testProjectID, "issue_iid": 10}},
		{"gitlab_issue_note_get", map[string]any{"project_id": testProjectID, "issue_iid": 10, "note_id": 100}},
		{"gitlab_issue_note_update", map[string]any{"project_id": testProjectID, "issue_iid": 10, "note_id": 100, "body": "updated"}},
		{"gitlab_issue_note_delete", map[string]any{"project_id": testProjectID, "issue_iid": 10, "note_id": 100}},
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

// TestActionSpecs_MutationErrors verifies get-404 and delete backend error routes.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		case http.MethodDelete:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{}`)
		}
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_note_get", map[string]any{"project_id": testProjectID, "issue_iid": 1, "note_id": 999}},
		{"gitlab_issue_note_delete", map[string]any{"project_id": testProjectID, "issue_iid": 1, "note_id": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueNotesActionHandler())))

	result, err := byTool["gitlab_issue_note_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID, "issue_iid": 10, "note_id": 100,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_note_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_issue_note_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted note 100 from issue #10 in project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := issueNoteSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_issue_note_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue note destructive confirmation.",
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
		Name:      "gitlab_issue_note_delete",
		Arguments: map[string]any{"project_id": testProjectID, "issue_iid": 1, "note_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func issueNotesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && path == pathIssueNotes:
			testutil.RespondJSON(w, http.StatusCreated, noteJSONSimple)
		case r.Method == http.MethodGet && path == pathIssueNotes:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSONSimple+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathIssueNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
		case r.Method == http.MethodPut && path == pathIssueNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
		case r.Method == http.MethodDelete && path == pathIssueNote100:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func issueNoteSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
