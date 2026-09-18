// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package grouplabels

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
	byTool := groupLabelSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_group_label_delete"].Route.Handler(t.Context(), map[string]any{"group_id": "my-group", "label_id": "bug"})
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
		if spec.IndividualTool.Name == "gitlab_group_label_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test group label destructive confirmation.", Icons: toolutil.IconLabel})
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
		Name:      "gitlab_group_label_delete",
		Arguments: map[string]any{"group_id": "my-group", "label_id": "bug"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestActionSpecs_UnsubscribeError validates the UnsubscribeError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_UnsubscribeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupLabelSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_group_label_unsubscribe"].Route.Handler(t.Context(), map[string]any{"group_id": "my-group", "label_id": "bug"})
	if err == nil {
		t.Fatal("expected error from unsubscribe with failing backend")
	}
}

// TestDecorateGroupLabelMeta_UnknownTool verifies the no-op branch leaves the
// generic placeholder metadata untouched for a tool with no meta entry.
func TestDecorateGroupLabelMeta_UnknownTool(t *testing.T) {
	options := groupLabelOptions("gitlab_unknown_tool")
	decorateGroupLabelMeta(&options, "gitlab_unknown_tool")
	if options.Usage != "Use to execute grouplabels domain action." {
		t.Errorf("Usage = %q, want generic placeholder unchanged", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want empty for unknown tool", options.IndividualTool.Description)
	}
}

// TestGroupLabelActionMeta_AllToolsDecorated verifies every projected group-label
// tool carries non-generic R-META discovery metadata (Usage, aliases, related,
// and a "Returns: … See also: …" individual-tool description).
//
// The aliases and the related actions are held to the table that decorates them
// and to the placeholder groupLabelOptions puts there first, because only the
// second half distinguishes a decoration that ran from one that did not: with
// its guard inverted the spec keeps the tool's own name as its only alias and
// the two generic group actions as everything it relates to, which is the text
// gitlab_find_action searches and the list a model is offered next. Asserting
// the curated values alone would leave that state passing.
func TestGroupLabelActionMeta_AllToolsDecorated(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	for _, spec := range ActionSpecs(client) {
		name := spec.IndividualTool.Name
		if spec.IndividualTool.Description == "" {
			t.Errorf("%s: missing individual-tool description", name)
		}
		if spec.Usage == "Use to execute grouplabels domain action." {
			t.Errorf("%s: Usage still generic placeholder", name)
		}
		meta, ok := groupLabelActionMeta[name]
		if !ok {
			t.Errorf("%s: no discovery metadata entry", name)
			continue
		}
		for _, alias := range meta.aliases {
			if !slices.Contains(spec.Aliases, alias) {
				t.Errorf("%s: alias %q missing from spec aliases %v", name, alias, spec.Aliases)
			}
		}
		if slices.Contains(spec.Aliases, name) {
			t.Errorf("%s: spec still carries the generic tool-name alias, aliases = %v", name, spec.Aliases)
		}
		for _, related := range meta.related {
			if !slices.Contains(spec.RelatedActions, related) {
				t.Errorf("%s: related action %q missing from %v", name, related, spec.RelatedActions)
			}
		}
		if slices.Contains(spec.RelatedActions, "group.issues") {
			t.Errorf("%s: spec still carries the generic related placeholder, related = %v", name, spec.RelatedActions)
		}
	}
}

// TestDecorateGroupLabelMeta_PartialEntry_KeepsWhatTheEntryDoesNotCarry verifies
// each of the four decorations is applied only for a field its entry actually
// holds. The table is hand-written, so an entry added without aliases, without
// related actions, without a usage line or without a description must leave the
// value groupLabelOptions produced rather than replace it with an empty one —
// and an emptied slice is worse than a generic one, since an action with no
// alias at all is one gitlab_find_action cannot reach by any name.
//
// No entry in the table today omits a field, so this is the only test that can
// decide the four guards at all: they are otherwise evaluated one way only.
func TestDecorateGroupLabelMeta_PartialEntry_KeepsWhatTheEntryDoesNotCarry(t *testing.T) {
	const tool = "gitlab_group_label_partial_entry_probe"
	groupLabelActionMeta[tool] = groupLabelActionMetaEntry{}
	t.Cleanup(func() { delete(groupLabelActionMeta, tool) })

	generic := groupLabelOptions(tool)
	options := groupLabelOptions(tool)
	decorateGroupLabelMeta(&options, tool)

	if options.Usage != generic.Usage {
		t.Errorf("Usage = %q, want the generic %q kept", options.Usage, generic.Usage)
	}
	if !slices.Equal(options.Aliases, generic.Aliases) {
		t.Errorf("Aliases = %v, want the generic %v kept", options.Aliases, generic.Aliases)
	}
	if !slices.Equal(options.RelatedActions, generic.RelatedActions) {
		t.Errorf("RelatedActions = %v, want the generic %v kept", options.RelatedActions, generic.RelatedActions)
	}
	if options.IndividualTool.Description != generic.IndividualTool.Description {
		t.Errorf("Description = %q, want the generic %q kept", options.IndividualTool.Description, generic.IndividualTool.Description)
	}
}
