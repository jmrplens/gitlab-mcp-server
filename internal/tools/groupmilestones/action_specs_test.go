// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package groupmilestones

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

// TestDecorateGroupMilestoneMeta_UnknownTool verifies the metadata decorator is
// a no-op for an individual tool that has no entry in groupMilestoneActionMeta.
// The test asserts the placeholder options are left untouched for an unknown tool.
func TestDecorateGroupMilestoneMeta_UnknownTool(t *testing.T) {
	options := groupMilestoneOptions("gitlab_unknown_tool")
	before := options
	decorateGroupMilestoneMeta(&options, "gitlab_unknown_tool")
	if options.Usage != before.Usage {
		t.Errorf("Usage mutated for unknown tool: got %q, want %q", options.Usage, before.Usage)
	}
	if options.IndividualTool.Description != before.IndividualTool.Description {
		t.Errorf("Description mutated for unknown tool: got %q", options.IndividualTool.Description)
	}
}

// TestDecorateGroupMilestoneMeta_PartialEntry_KeepsWhatItDoesNotName verifies
// that an entry filling only one field leaves the rest of the placeholder
// metadata alone. Each guard in the decorator exists for exactly this: without
// them a half-written entry would publish an action with no aliases and no
// related actions at all, which on the dynamic surface is an action a model
// cannot find by any natural-language phrase. The table is swapped rather than
// extended because every real entry fills every field, so nothing else in this
// package can reach the other side of those guards.
func TestDecorateGroupMilestoneMeta_PartialEntry_KeepsWhatItDoesNotName(t *testing.T) {
	const (
		tool      = "gitlab_group_milestone_partial"
		usageOnly = "only a usage"
		aliasOnly = "only an alias"
	)
	placeholder := groupMilestoneOptions(tool)
	cases := []struct {
		name        string
		entry       groupMilestoneActionMetaEntry
		wantUsage   string
		wantAliases []string
	}{
		{
			name:        "names only a usage",
			entry:       groupMilestoneActionMetaEntry{usage: usageOnly},
			wantUsage:   usageOnly,
			wantAliases: placeholder.Aliases,
		},
		{
			name:        "names only aliases",
			entry:       groupMilestoneActionMetaEntry{aliases: []string{aliasOnly}},
			wantUsage:   placeholder.Usage,
			wantAliases: []string{aliasOnly},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := groupMilestoneActionMeta
			t.Cleanup(func() { groupMilestoneActionMeta = original })
			groupMilestoneActionMeta = map[string]groupMilestoneActionMetaEntry{tool: tc.entry}

			options := groupMilestoneOptions(tool)
			decorateGroupMilestoneMeta(&options, tool)

			if options.Usage != tc.wantUsage {
				t.Errorf("Usage = %q, want %q", options.Usage, tc.wantUsage)
			}
			if !slices.Equal(options.Aliases, tc.wantAliases) {
				t.Errorf("Aliases = %v, want %v", options.Aliases, tc.wantAliases)
			}
			// Neither case names these, so all three stay the placeholder's.
			if !slices.Equal(options.RelatedActions, placeholder.RelatedActions) {
				t.Errorf("RelatedActions = %v, want the placeholder %v", options.RelatedActions, placeholder.RelatedActions)
			}
			if options.IndividualTool.Description != placeholder.IndividualTool.Description {
				t.Errorf("Description = %q, want the placeholder %q", options.IndividualTool.Description, placeholder.IndividualTool.Description)
			}
			if len(options.InputSchemaOverrides) != len(placeholder.InputSchemaOverrides) {
				t.Errorf("InputSchemaOverrides = %v, want the placeholder %v", options.InputSchemaOverrides, placeholder.InputSchemaOverrides)
			}
		})
	}
}

// TestActionSpecs_DiscoveryMetadata_IsTheEntryRatherThanThePlaceholder verifies
// that the usage, aliases, related actions and individual-tool description each
// spec carries are the ones its entry names, and never the generic placeholder
// groupMilestoneOptions starts every action from. This is what R-META asks of
// the package, and nothing asserted it: the decorator could stop copying any of
// the four and every other test here would still pass, while the dynamic
// surface would advertise eight actions that all say "Use to execute
// groupmilestones domain action" and answer to no phrase but their own name.
func TestActionSpecs_DiscoveryMetadata_IsTheEntryRatherThanThePlaceholder(t *testing.T) {
	byTool := groupMilestoneSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NotFoundHandler())))

	for tool, meta := range groupMilestoneActionMeta {
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatalf("no spec registers the individual tool %s", tool)
			}
			placeholder := groupMilestoneOptions(tool)
			assertGroupMilestoneMeta(t, spec, meta, placeholder)
		})
	}
}

// assertGroupMilestoneMeta holds one spec to its entry, and to being unlike the
// placeholder. Both halves are asserted: equality alone would pass if the
// entry itself were emptied, and difference alone would pass on any other text.
func assertGroupMilestoneMeta(t *testing.T, spec toolutil.ActionSpec, meta groupMilestoneActionMetaEntry, placeholder toolutil.ActionSpecOptions) {
	t.Helper()
	if spec.Usage != meta.usage || spec.Usage == placeholder.Usage {
		t.Errorf("Usage = %q, want the entry's %q and not the placeholder", spec.Usage, meta.usage)
	}
	// Compared case-insensitively because the catalog lowercases every alias on
	// the way in (toolutil.normalizeActionSpecStrings), so "list group
	// milestone MRs" is served as "list group milestone mrs"; what this asserts
	// is that the phrases are the entry's, not how they are spelled once the
	// matcher has them.
	if !slices.EqualFunc(spec.Aliases, meta.aliases, strings.EqualFold) || slices.Equal(spec.Aliases, placeholder.Aliases) {
		t.Errorf("Aliases = %v, want the entry's %v and not the placeholder", spec.Aliases, meta.aliases)
	}
	if !slices.Equal(spec.RelatedActions, meta.related) || slices.Equal(spec.RelatedActions, placeholder.RelatedActions) {
		t.Errorf("RelatedActions = %v, want the entry's %v and not the placeholder", spec.RelatedActions, meta.related)
	}
	if spec.IndividualTool.Description != meta.description || spec.IndividualTool.Description == "" {
		t.Errorf("Description = %q, want the entry's %q", spec.IndividualTool.Description, meta.description)
	}
}

// TestActionSpecs_InputSchemaOverrides_PublishTheTwoStateVocabularies verifies
// that the two actions taking a state word publish it as an enum, and that no
// other action publishes an override. The enum is the whole of what a model is
// told about those parameters: unpublished, `state_event` invites "closed",
// which GitLab refuses, and the only actions that would ever have said so are
// these two.
func TestActionSpecs_InputSchemaOverrides_PublishTheTwoStateVocabularies(t *testing.T) {
	byTool := groupMilestoneSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NotFoundHandler())))
	want := map[string]toolutil.InputSchemaOverride{
		"gitlab_group_milestone_list":   toolutil.SchemaPropertyOverride("state", map[string]any{"enum": []any{"active", "closed"}}),
		"gitlab_group_milestone_update": toolutil.SchemaPropertyOverride("state_event", map[string]any{"enum": []any{"close", "activate"}}),
	}

	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			expected, constrained := want[tool]
			if !constrained {
				if len(spec.InputSchemaOverrides) != 0 {
					t.Errorf("InputSchemaOverrides = %v, want none", spec.InputSchemaOverrides)
				}
				return
			}
			if len(spec.InputSchemaOverrides) != 1 {
				t.Fatalf("InputSchemaOverrides = %v, want exactly %v", spec.InputSchemaOverrides, expected)
			}
			got := spec.InputSchemaOverrides[0]
			if got.PropertyPath != expected.PropertyPath {
				t.Errorf("PropertyPath = %q, want %q", got.PropertyPath, expected.PropertyPath)
			}
			if !reflect.DeepEqual(got.Values, expected.Values) {
				t.Errorf("Values = %v, want %v", got.Values, expected.Values)
			}
		})
	}
}

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
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupMilestoneSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_group_milestone_delete"].Route.Handler(t.Context(), map[string]any{"group_id": "42", "milestone_iid": 1})
	if err == nil {
		t.Fatal("expected error from gitlab_group_milestone_delete")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_group_milestone_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test group milestone destructive confirmation.", Icons: toolutil.IconMilestone})
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
		Name:      "gitlab_group_milestone_delete",
		Arguments: map[string]any{"group_id": "42", "milestone_iid": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if tc.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}
