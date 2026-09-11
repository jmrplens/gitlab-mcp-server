package toolutil

import (
	"reflect"
	"testing"
)

// TestApplyActionMeta_OverlaysNonZeroFields verifies that ApplyActionMeta
// replaces only the populated fields of an entry, copies Aliases and Related
// defensively, and leaves option defaults untouched for zero-value fields.
func TestApplyActionMeta_OverlaysNonZeroFields(t *testing.T) {
	options := ActionSpecOptions{
		Usage:          "default usage",
		Aliases:        []string{"tool_name"},
		RelatedActions: []string{"default.related"},
	}
	meta := ActionMetaEntry{
		Usage:       "specific usage",
		Aliases:     []string{"alias one", "alias two"},
		Related:     []string{"domain.other"},
		Guidance:    map[string]ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
		Description: "Returns: a thing.",
	}
	ApplyActionMeta(&options, meta)

	if options.Usage != "specific usage" {
		t.Errorf("Usage = %q, want %q", options.Usage, "specific usage")
	}
	if !reflect.DeepEqual(options.Aliases, []string{"alias one", "alias two"}) {
		t.Errorf("Aliases = %v", options.Aliases)
	}
	if !reflect.DeepEqual(options.RelatedActions, []string{"domain.other"}) {
		t.Errorf("RelatedActions = %v", options.RelatedActions)
	}
	if options.IndividualTool.Description != "Returns: a thing." {
		t.Errorf("Description = %q", options.IndividualTool.Description)
	}
	if _, ok := options.ParameterGuidance["project_id"]; !ok {
		t.Errorf("ParameterGuidance missing project_id: %v", options.ParameterGuidance)
	}

	// Mutating the source slice must not affect the applied options (defensive copy).
	meta.Aliases[0] = "mutated"
	if options.Aliases[0] == "mutated" {
		t.Errorf("Aliases were not copied defensively")
	}
}

// TestApplyActionMeta_ZeroEntryIsNoOp verifies that a zero-value entry (the
// result of a missing metadata-map lookup) leaves the option defaults intact.
func TestApplyActionMeta_ZeroEntryIsNoOp(t *testing.T) {
	options := ActionSpecOptions{Usage: "keep", Aliases: []string{"keep"}}
	ApplyActionMeta(&options, ActionMetaEntry{})
	if options.Usage != "keep" || !reflect.DeepEqual(options.Aliases, []string{"keep"}) {
		t.Errorf("zero entry mutated options: %+v", options)
	}
}

// TestApplyActionMeta_NilOptionsSafe verifies ApplyActionMeta does not panic
// when handed a nil options pointer.
func TestApplyActionMeta_NilOptionsSafe(t *testing.T) {
	ApplyActionMeta(nil, ActionMetaEntry{Usage: "x"})
}

// TestApplyActionMeta_EmptyCollections_KeepOptionDefaults verifies that an
// entry carrying an allocated but empty Related slice or Guidance map is
// treated as the absent value it is, leaving the option defaults in place.
// A domain table that builds its rows programmatically produces exactly that
// shape, and overwriting with it would silently erase the shared defaults the
// action was registered with: the related-action list would become nil and
// the parameter guidance would become an empty map.
func TestApplyActionMeta_EmptyCollections_KeepOptionDefaults(t *testing.T) {
	options := ActionSpecOptions{
		RelatedActions:    []string{"default.related"},
		ParameterGuidance: map[string]ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
	}
	ApplyActionMeta(&options, ActionMetaEntry{
		Related:  []string{},
		Guidance: map[string]ParameterGuidance{},
	})

	if !reflect.DeepEqual(options.RelatedActions, []string{"default.related"}) {
		t.Errorf("RelatedActions = %#v, want the default list", options.RelatedActions)
	}
	if role := options.ParameterGuidance["project_id"].SemanticRole; role != "scope_project" {
		t.Errorf("ParameterGuidance[project_id].SemanticRole = %q, want scope_project", role)
	}
}
