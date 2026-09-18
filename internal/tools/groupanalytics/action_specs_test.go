// action_specs_test.go contains integration tests for the group analytics tool
// closures in ActionSpecs routes with a mock GitLab API.
package groupanalytics

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_DiscoveryMetadata validates that each group analytics action
// carries non-generic R-META discovery metadata: an action-specific Usage that
// is not the generic placeholder, at least one natural-language alias beyond the
// individual-tool name, canonical RelatedActions, and an individual-tool
// description in the "Returns: … See also: …" form.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	const genericUsage = "Use to execute groupanalytics domain action."
	for _, spec := range ActionSpecs(client) {
		tool := spec.IndividualTool.Name
		if spec.Usage == "" || spec.Usage == genericUsage {
			t.Errorf("%s: generic or empty Usage: %q", tool, spec.Usage)
		}
		hasNaturalAlias := false
		for _, alias := range spec.Aliases {
			if alias != tool && alias != spec.Name {
				hasNaturalAlias = true
				break
			}
		}
		if !hasNaturalAlias {
			t.Errorf("%s: no natural-language alias beyond tool name: %v", tool, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: empty RelatedActions", tool)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description missing Returns:/See also: form: %q", tool, desc)
		}
	}
}

// TestDecorateGroupAnalyticsMeta_UnknownToolIsNoOp verifies that the decorator
// leaves options untouched when the individual tool has no metadata entry,
// covering the early-return branch.
func TestDecorateGroupAnalyticsMeta_UnknownToolIsNoOp(t *testing.T) {
	options := groupAnalyticsOptions("gitlab_unknown_tool")
	before := options
	decorateGroupAnalyticsMeta(&options, "gitlab_unknown_tool")
	if options.Usage != before.Usage || options.IndividualTool.Description != before.IndividualTool.Description {
		t.Fatalf("decorateGroupAnalyticsMeta mutated options for unknown tool: %+v", options)
	}
}

// TestDecorateGroupAnalyticsMeta_EntryFillsNothing_LeavesEveryGenericOptionAlone
// verifies each of the four metadata fields is copied only when the entry
// supplies it. Every entry in the real table fills all four, so nothing else
// reaches the other side of those four guards, and a group analytics action
// added with partial metadata would otherwise lose whatever the generic
// options already carried: an entry naming no aliases would replace the
// individual-tool name with nothing, leaving the action reachable by its
// canonical ID alone.
func TestDecorateGroupAnalyticsMeta_EntryFillsNothing_LeavesEveryGenericOptionAlone(t *testing.T) {
	const probe = "gitlab_group_analytics_meta_probe"
	groupAnalyticsActionMeta[probe] = groupAnalyticsActionMetaEntry{}
	t.Cleanup(func() { delete(groupAnalyticsActionMeta, probe) })

	options := toolutil.ActionSpecOptions{
		Usage:          "generic usage",
		Aliases:        []string{"generic alias"},
		RelatedActions: []string{"generic.related"},
	}
	options.IndividualTool.Description = "generic description"
	decorateGroupAnalyticsMeta(&options, probe)

	if options.Usage != "generic usage" {
		t.Errorf("Usage = %q, want the generic one untouched", options.Usage)
	}
	if options.IndividualTool.Description != "generic description" {
		t.Errorf("Description = %q, want the generic one untouched", options.IndividualTool.Description)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "generic alias" {
		t.Errorf("Aliases = %v, want the generic one left as it was", options.Aliases)
	}
	if len(options.RelatedActions) != 1 || options.RelatedActions[0] != "generic.related" {
		t.Errorf("RelatedActions = %v, want the generic one left as it was", options.RelatedActions)
	}
}

// TestActionSpecs_RelatedActions_NameTheTwoSiblingAnalyticsActions verifies each
// analytics action points discovery at the other two, which is the whole reason
// the metadata table overrides RelatedActions at all: the generic options carry
// only the group read, and that is already non-empty, so asserting the list is
// non-empty says nothing about whether the sibling cluster was published. A
// model that found one of the three counts this way finds the other two.
//
// It compares the action part of each entry and not the whole ID, because the
// table spells the siblings with the owner package as prefix
// (groupanalytics.analytics_mr_count) while the catalog projects these actions
// under the group domain (group.analytics_mr_count) — a discrepancy this test
// deliberately neither asserts nor depends on.
func TestActionSpecs_RelatedActions_NameTheTwoSiblingAnalyticsActions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			named := make(map[string]bool, len(spec.RelatedActions))
			for _, related := range spec.RelatedActions {
				named[related[strings.LastIndex(related, ".")+1:]] = true
			}
			for _, sibling := range specs {
				if sibling.Name == spec.Name {
					continue
				}
				if !named[sibling.Name] {
					t.Errorf("RelatedActions %v does not name the sibling %q", spec.RelatedActions, sibling.Name)
				}
			}
		})
	}
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupanalytics" || !spec.ReadOnly || !spec.Idempotent {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"issues_count": 42, "merge_requests_count": 15, "new_members_count": 3}`)
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_recently_created_issues_count", map[string]any{"group_path": "my-group"}},
		{"gitlab_get_recently_created_mr_count", map[string]any{"group_path": "my-group"}},
		{"gitlab_get_recently_added_members_count", map[string]any{"group_path": "my-group"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}
