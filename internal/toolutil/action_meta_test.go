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
