// action_spec_guidance_test.go contains unit tests for the scope parameter
// guidance defaults: the [FillScopeParameterGuidance] batch path and the
// per-name defaults returned by defaultScopeParameterGuidance, including the
// zero-value fallback for names outside the scope-suggestive set.
package toolutil

import (
	"reflect"
	"testing"
)

// TestFillScopeParameterGuidance_NonEmptySpecs_AddsDefaults verifies that
// [FillScopeParameterGuidance] iterates a non-empty spec slice and adds the
// default guidance entry for a scope-suggestive input schema parameter that
// lacks explicit guidance.
func TestFillScopeParameterGuidance_NonEmptySpecs_AddsDefaults(t *testing.T) {
	specs := []ActionSpec{{
		Name: "list",
		Route: ActionRoute{
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string"},
				},
			},
		},
	}}

	out := FillScopeParameterGuidance(specs)
	if len(out) != 1 {
		t.Fatalf("FillScopeParameterGuidance() returned %d specs, want 1", len(out))
	}
	guidance, ok := out[0].ParameterGuidance["project_id"]
	if !ok {
		t.Fatal("FillScopeParameterGuidance() did not add project_id guidance")
	}
	if guidance.SemanticRole != "scope_project" {
		t.Errorf("project_id SemanticRole = %q, want %q", guidance.SemanticRole, "scope_project")
	}
}

// TestDefaultScopeParameterGuidance_KnownNames_ReturnsGuidance verifies that
// defaultScopeParameterGuidance returns a populated entry (semantic role and
// value source) for every scope-suggestive parameter name, so the discovery
// auditor's missing_parameter_guidance check is satisfied for all of them.
func TestDefaultScopeParameterGuidance_KnownNames_ReturnsGuidance(t *testing.T) {
	tests := []struct {
		name string
		role string
	}{
		{name: "project_id", role: "scope_project"},
		{name: "group_id", role: "scope_group"},
		{name: "user_id", role: "scope_user"},
		{name: "instance_id", role: "scope_instance"},
		{name: "namespace_id", role: "scope_namespace"},
		{name: "milestone_id", role: "milestone_id"},
		{name: "epic_id", role: "epic_id"},
		{name: "ref", role: "branch_or_tag"},
		{name: "branch", role: "branch_name"},
		{name: "tag", role: "tag_name"},
		{name: "sha", role: "commit_sha"},
		{name: "path", role: "path_component"},
		{name: "iid", role: "scope_iid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultScopeParameterGuidance(tc.name)
			if got.SemanticRole != tc.role {
				t.Errorf("defaultScopeParameterGuidance(%q).SemanticRole = %q, want %q", tc.name, got.SemanticRole, tc.role)
			}
			if got.ValueSource == "" {
				t.Errorf("defaultScopeParameterGuidance(%q).ValueSource is empty", tc.name)
			}
		})
	}
}

// TestDefaultScopeParameterGuidance_UnknownName_ReturnsZero verifies the
// fall-through branch: names outside the scope-suggestive set yield the zero
// ParameterGuidance so callers never attach fabricated guidance.
func TestDefaultScopeParameterGuidance_UnknownName_ReturnsZero(t *testing.T) {
	got := defaultScopeParameterGuidance("definitely_not_a_scope_param")
	if !reflect.DeepEqual(got, ParameterGuidance{}) {
		t.Errorf("defaultScopeParameterGuidance(unknown) = %+v, want zero value", got)
	}
}
