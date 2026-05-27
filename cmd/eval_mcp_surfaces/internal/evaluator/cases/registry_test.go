package cases

import "testing"

// TestAll_ReturnsIndependentCaseDefinitions verifies that All returns cloned case
// definitions instead of package-owned slices.
//
// The test mutates scalar and nested slice fields from one All call, then calls
// All again and asserts the original catalog values are still intact. This
// protects the evaluator runtime from accidentally changing the static case
// registry while preparing surface-specific tasks.
func TestAll_ReturnsIndependentCaseDefinitions(t *testing.T) {
	first := All()
	if len(first) == 0 || len(first[0].Steps) == 0 {
		t.Fatalf("All() = %#v, want at least one case with one step", first)
	}

	first[0].ID = "changed"
	first[0].Steps[0].RequiredParams = append(first[0].Steps[0].RequiredParams, "changed_param")
	first[0].Presets = append(first[0].Presets, "changed-preset")

	second := All()
	if second[0].ID == "changed" {
		t.Fatalf("All()[0].ID = %q, want original ID", second[0].ID)
	}
	for _, param := range second[0].Steps[0].RequiredParams {
		if param == "changed_param" {
			t.Fatalf("All()[0].Steps[0].RequiredParams = %v, want cloned params", second[0].Steps[0].RequiredParams)
		}
	}
	for _, preset := range second[0].Presets {
		if preset == "changed-preset" {
			t.Fatalf("All()[0].Presets = %v, want cloned presets", second[0].Presets)
		}
	}
}
