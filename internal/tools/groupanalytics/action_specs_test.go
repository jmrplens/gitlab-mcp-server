// action_specs_test.go contains integration tests for the group analytics tool
// closures in ActionSpecs routes with a mock GitLab API.
package groupanalytics

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
