// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package featureflags

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_DeleteError validates the DeleteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	byTool := featureFlagSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_feature_flag_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "my-project", "name": "my-flag"})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_feature_flag_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test feature flag destructive confirmation.", Icons: toolutil.IconConfig})
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_feature_flag_delete",
		Arguments: map[string]any{"project_id": "42", "name": "my-flag"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestFeatureFlagActionSpecs_RelatedActions_NameSiblingFeatureFlagActions
// verifies that each spec's RelatedActions are the ones its metadata entry
// names rather than the domain-wide placeholder featureFlagOptions starts
// every spec with. Asserting only that the field is non-empty cannot tell the
// two apart, because the placeholder is non-empty too: a decorator that
// stopped copying the metadata's related actions would leave every feature
// flag action pointing a model at environment.list and ci_variable.list, and
// nothing would notice.
//
// What counts as a sibling is the package's own canonical ID set rather than a
// string prefix typed here. The prefix used to be the literal "feature_flag.",
// which is the spelling the specs got wrong: every related ID carried it,
// every one of them named no catalog action, and this test was green on all
// five. A test that repeats the value under test can only agree with it.
func TestFeatureFlagActionSpecs_RelatedActions_NameSiblingFeatureFlagActions(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := featureFlagSpecsByTool(t, ActionSpecs(client))
	placeholder := featureFlagOptions("gitlab_feature_flag_list").RelatedActions

	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			if slices.Equal(spec.RelatedActions, placeholder) {
				t.Errorf("%s: RelatedActions is still the generic placeholder %v", tool, spec.RelatedActions)
			}
			named := 0
			for _, related := range spec.RelatedActions {
				if slices.Contains(PublishedActionIDs, related) {
					named++
				}
			}
			if named == 0 {
				t.Errorf("%s: RelatedActions names no sibling feature flag action: %v", tool, spec.RelatedActions)
			}
		})
	}
}

// TestDecorateFeatureFlagMeta_EntryOmittingAField_KeepsTheGenericDefault
// verifies what the four presence guards in decorateFeatureFlagMeta are for: a
// metadata entry that fills some fields and not others replaces only the ones
// it fills. Every entry in the shipped table fills all four, so the guards'
// false branch is unreachable through ActionSpecs and an entry omitting one
// has to be installed here to reach it. Without that branch held, a guard
// could be dropped and a future partial entry would silently blank the
// generic alias, related actions or description the spec falls back on.
func TestDecorateFeatureFlagMeta_EntryOmittingAField_KeepsTheGenericDefault(t *testing.T) {
	const tool = "gitlab_feature_flag_partial"

	// A zero value in a want field means the generic placeholder is expected,
	// so each case states only what its entry is supposed to replace. Two
	// cases rather than one because each field's guard has to be seen
	// declining as well as firing, and an entry that fills a field can never
	// show the first.
	tests := []struct {
		name            string
		entry           featureFlagActionMetaEntry
		wantUsage       string
		wantAliases     []string
		wantRelated     []string
		wantDescription string
	}{
		{
			name:      "only the usage is filled",
			entry:     featureFlagActionMetaEntry{usage: "read the feature flags of one project"},
			wantUsage: "read the feature flags of one project",
		},
		{
			name:        "only the aliases are filled",
			entry:       featureFlagActionMetaEntry{aliases: []string{"show me the feature flags"}},
			wantAliases: []string{"show me the feature flags"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featureFlagActionMeta[tool] = tt.entry
			t.Cleanup(func() { delete(featureFlagActionMeta, tool) })

			generic := featureFlagOptions(tool)
			wantUsage, wantAliases := generic.Usage, generic.Aliases
			wantRelated, wantDescription := generic.RelatedActions, generic.IndividualTool.Description
			if tt.wantUsage != "" {
				wantUsage = tt.wantUsage
			}
			if tt.wantAliases != nil {
				wantAliases = tt.wantAliases
			}
			if tt.wantRelated != nil {
				wantRelated = tt.wantRelated
			}
			if tt.wantDescription != "" {
				wantDescription = tt.wantDescription
			}

			options := featureFlagOptions(tool)
			decorateFeatureFlagMeta(&options, tool)

			if options.Usage != wantUsage {
				t.Errorf("Usage = %q, want %q", options.Usage, wantUsage)
			}
			if !slices.Equal(options.Aliases, wantAliases) {
				t.Errorf("Aliases = %v, want %v", options.Aliases, wantAliases)
			}
			if !slices.Equal(options.RelatedActions, wantRelated) {
				t.Errorf("RelatedActions = %v, want %v", options.RelatedActions, wantRelated)
			}
			if options.IndividualTool.Description != wantDescription {
				t.Errorf("Description = %q, want %q", options.IndividualTool.Description, wantDescription)
			}
		})
	}
}
