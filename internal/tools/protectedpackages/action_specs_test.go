// action_specs_test.go contains canonical-route tests for package protection rule actions.
package protectedpackages

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every package protection rule tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedPackagesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_package_protection_rules", map[string]any{"project_id": testProjectID}},
		{"gitlab_create_package_protection_rule", map[string]any{"project_id": testProjectID, "package_name_pattern": "@scope/pkg*", "package_type": "npm"}},
		{"gitlab_update_package_protection_rule", map[string]any{"project_id": testProjectID, "rule_id": 1}},
		{"gitlab_delete_package_protection_rule", map[string]any{"project_id": testProjectID, "rule_id": 1}},
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

// TestActionSpecs_DeleteError verifies the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	_, err := byTool["gitlab_delete_package_protection_rule"].Route.Handler(t.Context(), map[string]any{
		"project_id": "1",
		"rule_id":    1,
	})
	if err == nil {
		t.Fatal("expected delete route error")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedPackagesActionHandler())))

	result, err := byTool["gitlab_delete_package_protection_rule"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID,
		"rule_id":    1,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_package_protection_rule) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_package_protection_rule) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted package protection rule 1 from project myproject." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_package_protection_rule"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test package protection rule destructive confirmation.",
		Icons:       toolutil.IconShield,
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
		Name:      "gitlab_delete_package_protection_rule",
		Arguments: map[string]any{"project_id": "1", "rule_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// catalogDomainPrefix is the domain half of this package's canonical action
// IDs. The specs carry only the action half; `buildPackageActionSpecs` in
// internal/tools/action_specs.go folds them into the gitlab_package group,
// and importing that from here would be a cycle.
const catalogDomainPrefix = "package."

// TestActionSpecs_RelatedActionsNameActionsThisPackageDefines verifies every
// sibling this package recommends in its own domain is an action it really
// registers. Nothing else checks these strings: audit_discovery_completeness
// only counts an empty list, so a misspelled ID ships green and answers a
// model "unknown action" the moment it follows the hint.
func TestActionSpecs_RelatedActionsNameActionsThisPackageDefines(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	defined := make(map[string]bool, len(specs))
	for _, spec := range specs {
		defined[catalogDomainPrefix+spec.Name] = true
	}

	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatalf("%s names no related action", spec.Name)
			}
			for _, related := range spec.RelatedActions {
				domain, action, ok := strings.Cut(related, ".")
				if !ok || domain == "" || action == "" {
					t.Errorf("related action %q is not a domain.action id", related)
					continue
				}
				if strings.HasPrefix(related, catalogDomainPrefix) && !defined[related] {
					t.Errorf("related action %q names no action this package defines", related)
				}
			}
		})
	}
}

// TestActionSpecs_EveryToolCarriesItsOwnDiscoveryMetadata verifies each spec
// picked up a case in applyProtectedPackageDiscovery rather than falling
// through to the shared defaults. An action added without its case would
// otherwise ship with the placeholder usage and no description, which is the
// one thing a model reads before choosing it.
func TestActionSpecs_EveryToolCarriesItsOwnDiscoveryMetadata(t *testing.T) {
	shared := protectedPackageOptions("gitlab_not_a_package_protection_tool")

	for _, spec := range ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t))) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if spec.Usage == shared.Usage {
				t.Errorf("usage is still the shared placeholder %q", spec.Usage)
			}
			if spec.IndividualTool.Description == "" {
				t.Error("individual tool carries no description")
			}
			if len(spec.Aliases) < 2 {
				t.Errorf("aliases = %v, want the natural-language set", spec.Aliases)
			}
		})
	}
}

// TestActionSpecs_MarkdownHintsNameRegisteredTools verifies each tool the two
// formatters tell a model to reach for is one this package registers. The card
// and table assertions compare whole strings, so they would follow a rename
// straight into a hint that names nothing, and a model reading it is told to
// call a tool the server does not serve.
func TestActionSpecs_MarkdownHintsNameRegisteredTools(t *testing.T) {
	registered := map[string]bool{}
	for _, spec := range ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t))) {
		registered[spec.IndividualTool.Name] = true
	}

	rendered := map[string]string{
		"card":  FormatOutputMarkdown(Output{ID: 1, PackageNamePattern: "a*", PackageType: "npm"}),
		"table": FormatListMarkdown(ListOutput{Rules: []Output{{ID: 1, PackageNamePattern: "a*", PackageType: "npm"}}}),
	}
	for name, md := range rendered {
		t.Run(name, func(t *testing.T) {
			named := 0
			for field := range strings.FieldsSeq(md) {
				tool := strings.Trim(field, "`")
				if !strings.HasPrefix(tool, "gitlab_") {
					continue
				}
				named++
				if !registered[tool] {
					t.Errorf("hint names %q, which this package does not register", tool)
				}
			}
			if named == 0 {
				t.Error("no tool named in the hints; the scan found nothing to check")
			}
		})
	}
}

// TestProtectedPackageOptions_UnknownToolKeepsTheSharedDefaults pins the other
// side of that switch: a name outside the table is left with the placeholder
// usage and no related actions, so a forgotten case is a visible hole rather
// than metadata borrowed from whichever arm ran last.
func TestProtectedPackageOptions_UnknownToolKeepsTheSharedDefaults(t *testing.T) {
	options := protectedPackageOptions("gitlab_not_a_package_protection_tool")

	if options.Usage != "Use to execute protectedpackages domain action." {
		t.Errorf("Usage = %q, want the shared placeholder", options.Usage)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %v, want none", options.RelatedActions)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want empty", options.IndividualTool.Description)
	}
}

func protectedPackagesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathRules:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+ruleJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodPost && path == pathRules:
			testutil.RespondJSON(w, http.StatusCreated, ruleJSON)
		case r.Method == http.MethodPatch && path == pathRule1:
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
		case r.Method == http.MethodDelete && path == pathRule1:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func protectedPackageSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
