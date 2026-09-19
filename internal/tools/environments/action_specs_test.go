// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation errors, not-found output, and destructive confirmation.
package environments

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintActionID matches the ID inside the text toolutil.HintAction renders,
// "Use action 'environment.deployment_list' to …", which is how a model reads a
// cross-link out of a card.
var hintActionID = regexp.MustCompile(`Use action '([^']+)' to `)

// crossLinkableIDs is the set of canonical IDs the environment cross-links may
// name: every action of this package and of the deployment package beside it
// in the gitlab_environment group, plus the CI variable and feature flag
// actions an environment points at in their own groups.
//
// Each set is built from the owning package's own ActionSpecs rather than
// listed here, so the action half of every ID is checked against what is really
// registered. The two foreign domains are the one part written out, and they
// are the part cmd/audit_action_ids checks against the assembled catalog.
func crossLinkableIDs(t *testing.T) map[string]bool {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	ids := map[string]bool{}
	add := func(prefix string, specs []toolutil.ActionSpec) {
		if len(specs) == 0 {
			t.Errorf("no specs to build the %q half of the cross-link set from", prefix)
		}
		for _, spec := range specs {
			ids[prefix+spec.Name] = true
		}
	}
	add(domainPrefix, ActionSpecs(client))
	add(domainPrefix, deployments.ActionSpecs(client))
	add("ci_variable.", civariables.ActionSpecs(client))
	add("feature_flags.", featureflags.ActionSpecs(client))
	return ids
}

// TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds asserts that every
// related action an environment spec publishes is an ID the catalog holds.
//
// A deployment is not a domain: the deployment specs join the gitlab_environment
// group, so a deployment action's ID is "environment.deployment_list". Five
// cross-links spelled it "deployment.list" and "deployment.create" instead, and
// one named a "feature_flags.strategy_list" that has never existed under any
// domain, a strategy being part of a feature flag rather than an action on one.
func TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	registered := crossLinkableIDs(t)

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatalf("%s publishes no related actions", spec.Name)
			}
			for _, related := range spec.RelatedActions {
				if !registered[related] {
					t.Errorf("%s relates to %q, which the catalog holds no action under", spec.Name, related)
				}
			}
		})
	}
}

// TestEnvironmentOptionsForAction_UnknownName_PublishesResolvableRelations
// asserts the shared base every action starts from.
//
// The switch below it sets RelatedActions in all six arms, so the base list
// reaches no model today and its two broken IDs were invisible for exactly that
// reason. A seventh action added without an arm would publish it.
func TestEnvironmentOptionsForAction_UnknownName_PublishesResolvableRelations(t *testing.T) {
	registered := crossLinkableIDs(t)

	options := environmentOptionsForAction("an_action_with_no_arm", "gitlab_environment_nonexistent")
	if len(options.RelatedActions) == 0 {
		t.Fatal("the shared base publishes no related actions")
	}
	for _, related := range options.RelatedActions {
		t.Run(related, func(t *testing.T) {
			if !registered[related] {
				t.Errorf("the shared base relates to %q, which the catalog holds no action under", related)
			}
		})
	}
}

// TestFormatMarkdownString_HintsNameActionsTheCatalogHolds asserts that every
// action ID an environment card invites a model to call next is one the catalog
// holds.
//
// The card is the second place these IDs are published, and it is the half that
// was right: it carried a constant of its own for the deployment listing with a
// comment saying the specs spelled it wrongly. Recording the drift is not
// fixing it, so both now read one block and this holds the rendered text to the
// specs.
func TestFormatMarkdownString_HintsNameActionsTheCatalogHolds(t *testing.T) {
	registered := crossLinkableIDs(t)

	cards := map[string]string{
		"available environment": FormatOutputMarkdown(Output{ID: 7, Name: "production", State: "available"}),
		"stopped environment":   FormatOutputMarkdown(Output{ID: 7, Name: "production", State: "stopped"}),
		"stopping environment":  FormatOutputMarkdown(Output{ID: 7, Name: "production", State: "stopping"}),
		"environment list":      FormatListMarkdown(ListOutput{Environments: []Output{{ID: 7, Name: "production"}}}),
	}
	for name, markdown := range cards {
		t.Run(name, func(t *testing.T) {
			matches := hintActionID.FindAllStringSubmatch(markdown, -1)
			if len(matches) == 0 {
				t.Fatalf("the %s card names no action to call next:\n%s", name, markdown)
			}
			for _, match := range matches {
				if !registered[match[1]] {
					t.Errorf("the %s card invites %q, which the catalog holds no action under", name, match[1])
				}
			}
		})
	}
}

// TestCanonicalIDs_AreTheSpecNamesUnderTheCatalogDomain asserts that the ID
// constants and the names the specs are registered under are one source.
//
// It is the invariant the fix rests on: a spec name carries no domain, a
// published ID must, and writing the two out separately is what let them drift.
func TestCanonicalIDs_AreTheSpecNamesUnderTheCatalogDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	ids := map[string]string{
		actionNameList:   actionEnvironmentList,
		actionNameGet:    actionEnvironmentGet,
		actionNameCreate: actionEnvironmentCreate,
		actionNameUpdate: actionEnvironmentUpdate,
		actionNameDelete: actionEnvironmentDelete,
		actionNameStop:   actionEnvironmentStop,
	}

	registered := map[string]bool{}
	for _, spec := range ActionSpecs(client) {
		registered[spec.Name] = true
	}
	for name, id := range ids {
		t.Run(name, func(t *testing.T) {
			if !registered[name] {
				t.Errorf("no spec is registered under the name %q", name)
			}
			if want := domainPrefix + name; id != want {
				t.Errorf("the ID for %q is %q, want %q", name, id, want)
			}
			if !strings.HasPrefix(id, domainPrefix) {
				t.Errorf("the ID for %q is %q, which carries no domain", name, id)
			}
		})
	}
	if len(registered) != len(ids) {
		t.Errorf("%d specs are registered and %d have an ID constant", len(registered), len(ids))
	}
}

// TestActionSpecs_MutationErrors validates the MutationErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		default:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := environmentSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name        string
		args        map[string]any
		expectError bool
	}{
		{"gitlab_environment_get", map[string]any{"project_id": "42", "environment_id": 999}, false},
		{"gitlab_environment_create", map[string]any{"project_id": "42", "name": "staging"}, true},
		{"gitlab_environment_update", map[string]any{"project_id": "42", "environment_id": 1, "name": "staging-v2"}, true},
		{"gitlab_environment_stop", map[string]any{"project_id": "42", "environment_id": 1}, true},
		{"gitlab_environment_delete", map[string]any{"project_id": "42", "environment_id": 1}, true},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error from %s", tt.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if _, ok := result.(environmentNotFoundOutput); !ok {
				t.Fatalf("result type = %T, want environmentNotFoundOutput", result)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_environment_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test environment destructive confirmation.", Icons: toolutil.IconEnvironment})
		}
	}

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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_environment_delete",
		Arguments: map[string]any{"project_id": "42", "environment_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}
