// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// withOtherSurfaces serves the root-combinator rule's further listings from
// listings instead of the registered surfaces until the test ends.
func withOtherSurfaces(t *testing.T, listings []surfaceListing) {
	t.Helper()
	original := listOtherSurfaces
	t.Cleanup(func() { listOtherSurfaces = original })
	listOtherSurfaces = func(*gitlabclient.Client) []surfaceListing { return listings }
}

// TestAuditRootCombinators_FlagsWhatTheRootCarries verifies the rule reports
// each tool whose input schema carries a combinator or a conditional at its
// root, naming every one it carries under the surface's label, and passes a
// tool whose combinators sit inside a property, which no client refuses.
func TestAuditRootCombinators_FlagsWhatTheRootCarries(t *testing.T) {
	branch := map[string]any{"required": []any{"name"}}
	listed := []*mcp.Tool{
		{Name: "gitlab_widget_any", InputSchema: map[string]any{"type": "object", "anyOf": []any{branch}}},
		{Name: "gitlab_widget_clean", InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"value": map[string]any{"oneOf": []any{map[string]any{"type": "string"}}},
		}}},
		{Name: "gitlab_widget_two", InputSchema: map[string]any{"type": "object", "oneOf": []any{branch}, "not": branch}},
	}

	got := auditRootCombinators(listed, "individual")
	want := []violation{
		{"gitlab_widget_any", rootCombinatorCategory, "individual tool inputSchema carries anyOf at its root, which the Anthropic Messages API refuses for the whole tool list"},
		{"gitlab_widget_two", rootCombinatorCategory, "individual tool inputSchema carries oneOf, not at its root, which the Anthropic Messages API refuses for the whole tool list"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditRootCombinators() = %+v\nwant %+v", got, want)
	}
}

// TestAuditOtherSurfacesRootCombinators_ReadsEveryListingUnderItsLabel
// verifies every further listing is read, in its order, and its violations
// carry its own label.
func TestAuditOtherSurfacesRootCombinators_ReadsEveryListingUnderItsLabel(t *testing.T) {
	allOf := map[string]any{"type": "object", "allOf": []any{map[string]any{"required": []any{"action"}}}}
	withOtherSurfaces(t, []surfaceListing{
		{kind: "meta full", tools: []*mcp.Tool{{Name: "gitlab_gadget", InputSchema: allOf}}},
		{kind: "dynamic", tools: []*mcp.Tool{cleanTool("gitlab_find_action")}},
		{kind: "meta gitlab.com", tools: []*mcp.Tool{{Name: "gitlab_orbit", InputSchema: allOf}}},
	})

	got := auditOtherSurfacesRootCombinators(nil)
	want := []violation{
		{"gitlab_gadget", rootCombinatorCategory, "meta full tool inputSchema carries allOf at its root, which the Anthropic Messages API refuses for the whole tool list"},
		{"gitlab_orbit", rootCombinatorCategory, "meta gitlab.com tool inputSchema carries allOf at its root, which the Anthropic Messages API refuses for the whole tool list"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("auditOtherSurfacesRootCombinators() = %+v\nwant %+v", got, want)
	}
}

// TestListOtherSurfaces_ServedSurfaces_CarryNoRootCombinator lists what the
// rule reads beyond the metadata view's two listings, as the server serves
// it: the meta surface under compact and full, whose root allOf of if/then
// conditionals the rule was written for, on both instance classes, the
// dynamic pair, and GitLab.com's individual and default meta surfaces. Each
// listing carries tools, none of them with a root combinator, a compact or
// full one serves the $defs those modes serve, so it was listed in its mode
// and not in the default one, and the meta mode is left as it was found.
func TestListOtherSurfaces_ServedSurfaces_CarryNoRootCombinator(t *testing.T) {
	before := tools.MetaParamSchema()
	listings := listOtherSurfaces(stubClient(t))

	var kinds []string
	for _, listing := range listings {
		kinds = append(kinds, listing.kind)
		if len(listing.tools) == 0 {
			t.Errorf("the %s listing carries no tool, so the rule read nothing there", listing.kind)
		}
		if vs := auditRootCombinators(listing.tools, listing.kind); len(vs) > 0 {
			t.Errorf("the %s listing serves root combinators: %+v", listing.kind, vs)
		}
		inline := strings.HasPrefix(listing.kind, "meta compact") || strings.HasPrefix(listing.kind, "meta full")
		if inline && !slices.ContainsFunc(listing.tools, servesDefinitions) {
			t.Errorf("the %s listing serves no $defs, so it was not listed in that mode", listing.kind)
		}
	}
	if want := []string{"meta compact", "meta compact gitlab.com", "meta full", "meta full gitlab.com", "dynamic", "individual gitlab.com", "meta gitlab.com"}; !slices.Equal(kinds, want) {
		t.Errorf("listings = %v, want %v", kinds, want)
	}
	if after := tools.MetaParamSchema(); after != before {
		t.Errorf("meta parameter-schema mode after the listings = %q, want the %q it was", after, before)
	}
}

// servesDefinitions reports whether tool's input schema carries $defs at its
// root, as a meta tool's does under the compact and full modes alone.
func servesDefinitions(tool *mcp.Tool) bool {
	schema, _ := tool.InputSchema.(map[string]any)
	_, defined := schema["$defs"]
	return defined
}
