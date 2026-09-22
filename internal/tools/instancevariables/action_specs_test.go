// action_specs_test.go contains canonical-route tests for instance CI/CD variable actions.
package instancevariables

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

// TestActionSpecs_CallAllRoutes drives all five canonical routes through the
// catalog surface, one per HTTP method the package uses, and asserts each
// returns a result rather than an error: it is the spec's wiring that is under
// test, not the handlers the routes reach.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET "+pathInstanceVars, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+varJSON+`]`)
	})
	handler.HandleFunc("GET "+pathVar1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, varJSON)
	})
	handler.HandleFunc("POST "+pathInstanceVars, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"key":"NEW_VAR","value":"new-val","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""}`)
	})
	handler.HandleFunc("PUT "+pathVar1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"MY_VAR","value":"updated-val","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""}`)
	})
	handler.HandleFunc("DELETE "+pathVar1, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_instance_variable_list", map[string]any{}},
		{"gitlab_instance_variable_get", map[string]any{"key": "MY_VAR"}},
		{"gitlab_instance_variable_create", map[string]any{"key": "NEW_VAR", "value": "new-val", "description": "", "variable_type": "", "protected": false, "masked": false, "raw": false}},
		{"gitlab_instance_variable_update", map[string]any{"key": "MY_VAR", "value": "updated-val", "description": "", "variable_type": "", "protected": false, "masked": false, "raw": false}},
		{"gitlab_instance_variable_delete", map[string]any{"key": "MY_VAR"}},
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

// TestActionSpecs_DeleteError validates that the delete route propagates a
// refusal instead of reporting the deletion done. What the error says is
// asserted where Delete is called directly; here it is that one arrives.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_instance_variable_delete"].Route.Handler(t.Context(), map[string]any{"key": "MY_VAR"})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput validates the DeleteOutput route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathVar1 || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_instance_variable_delete"].Route.Handler(t.Context(), map[string]any{"key": "MY_VAR"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_instance_variable_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_instance_variable_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted instance CI/CD variable." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies that a client declining
// the destructive-action elicitation gets a result back and GitLab is never
// asked: the mock forbids every request, so a delete that ran anyway fails the
// test rather than passing quietly.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_instance_variable_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test instance variable destructive confirmation.",
		Icons:       toolutil.IconVariable,
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
		Name:      "gitlab_instance_variable_delete",
		Arguments: map[string]any{"key": "MY_VAR"},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

// TestInstanceVariableOptions_UnknownActionKeepsGenericDefaults verifies that an
// action name none of the switch's cases matches falls through to the shared
// base rather than to a partly filled spec.
//
// It states the half of the contract the switch cannot state itself: the cases
// only specialize, so what a name it does not know gets is the base and nothing
// else. Until this existed the last comparison in the switch was never once
// evaluated false, so a case label that stopped matching would have left its
// action on these defaults with no test noticing.
func TestInstanceVariableOptions_UnknownActionKeepsGenericDefaults(t *testing.T) {
	opts := instanceVariableOptions("nonexistent_action", "gitlab_unknown_instance_variable")
	if opts.Usage != "Use to execute instancevariables domain action." {
		t.Errorf("Usage = %q, want the generic default", opts.Usage)
	}
	if len(opts.Aliases) != 1 || opts.Aliases[0] != "gitlab_unknown_instance_variable" {
		t.Errorf("Aliases = %v, want only the individual tool name", opts.Aliases)
	}
	if len(opts.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %v, want none", opts.RelatedActions)
	}
	if len(opts.ParameterGuidance) != 0 {
		t.Errorf("ParameterGuidance = %v, want none", opts.ParameterGuidance)
	}
	if opts.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want empty", opts.IndividualTool.Description)
	}
}

// TestInstanceVariableActionSpecs_AllCarryActionSpecificMetadata verifies that
// none of the five canonical actions is left on those generic defaults.
//
// This is the property the unknown-name test above is the counterweight to: a
// case label that no longer matches its action name is invisible at the call
// site, and it costs the action its usage text, its natural-language aliases
// and the "Returns: … See also: …" description every surface lists it by.
func TestInstanceVariableActionSpecs_AllCarryActionSpecificMetadata(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	if len(specs) != 5 {
		t.Fatalf("ActionSpecs returned %d specs, want 5", len(specs))
	}
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if spec.Usage == "Use to execute instancevariables domain action." {
				t.Errorf("Usage = %q, want action-specific text", spec.Usage)
			}
			if len(spec.Aliases) < 2 {
				t.Errorf("Aliases = %v, want natural-language aliases beside the tool name", spec.Aliases)
			}
			if spec.IndividualTool.Description == "" {
				t.Error("IndividualTool.Description is empty, want the action's own description")
			}
		})
	}
}

// TestInstanceVariableActionSpecs_NoActionLinksToItself verifies that no
// action offers itself as somewhere else to go, in its related actions or in
// the "See also" list of the description every individual tool is listed by.
//
// It is the property that catches the switch dealing a case body to the wrong
// action, which nothing else here can: crossing the get and create labels
// leaves both actions carrying action-specific metadata, so the test above is
// still satisfied, and both keep naming real catalog IDs, so the catalog test
// is too. What gives it away is that the metadata then points home: the get
// action offering ci_variable.instance_get, which is where the model already
// is. A self-link is a dead loop whatever put it there, so this states the
// invariant rather than repeating the five curated lists.
func TestInstanceVariableActionSpecs_NoActionLinksToItself(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	if len(specs) != 5 {
		t.Fatalf("ActionSpecs returned %d specs, want 5", len(specs))
	}
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			ownID := "ci_variable." + spec.Name
			if slices.Contains(spec.RelatedActions, ownID) {
				t.Errorf("RelatedActions = %v, want it not to name the action's own ID %q", spec.RelatedActions, ownID)
			}
			_, seeAlso, found := strings.Cut(spec.IndividualTool.Description, "See also:")
			if !found {
				t.Fatalf("Description = %q, want a See also list", spec.IndividualTool.Description)
			}
			if strings.Contains(seeAlso, spec.IndividualTool.Name) {
				t.Errorf("See also = %q, want it not to name the tool's own name %q", seeAlso, spec.IndividualTool.Name)
			}
		})
	}
}

// TestInstanceVariableActionSpecs_EachActionDescribesItsOwnVerb verifies that
// the metadata each action serves opens with the verb that action performs,
// and that its natural-language aliases name that verb too.
//
// It closes the one case-label crossing the self-link test above leaves open.
// Create names list, get and update as its related actions and delete names
// list and get, so exchanging those two labels has neither action pointing at
// itself and neither See-also list naming its own tool: every other test here
// stays satisfied, since both still carry action-specific metadata and both
// still name real catalog IDs. What ships is the create tool listed with
// delete's usage, delete's aliases and a description promising "a success
// confirmation message" for a variable it just created.
//
// The verb is the one word that cannot survive the exchange, and the
// individual tool name is deliberately skipped when reading the aliases: it is
// passed in beside the action name and carries the right verb whichever case
// body filled the entry, so it can vouch for nothing.
func TestInstanceVariableActionSpecs_EachActionDescribesItsOwnVerb(t *testing.T) {
	verbs := map[string]string{
		"instance_list":   "List",
		"instance_get":    "Get",
		"instance_create": "Create",
		"instance_update": "Update",
		"instance_delete": "Delete",
	}
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	if len(specs) != len(verbs) {
		t.Fatalf("ActionSpecs returned %d specs, want %d", len(specs), len(verbs))
	}
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			verb, recorded := verbs[spec.Name]
			if !recorded {
				t.Fatalf("action %q has no verb recorded here", spec.Name)
			}
			if !strings.HasPrefix(spec.Usage, verb+" ") {
				t.Errorf("Usage = %q, want it to open with %q", spec.Usage, verb)
			}
			if !strings.HasPrefix(spec.IndividualTool.Description, verb+" ") {
				t.Errorf("Description = %q, want it to open with %q", spec.IndividualTool.Description, verb)
			}
			spoken := strings.ToLower(verb)
			named := false
			for _, alias := range spec.Aliases {
				if alias != spec.IndividualTool.Name && strings.Contains(alias, spoken) {
					named = true
				}
			}
			if !named {
				t.Errorf("Aliases = %v, want a natural-language one naming %q", spec.Aliases, spoken)
			}
		})
	}
}

func instanceVariableSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
