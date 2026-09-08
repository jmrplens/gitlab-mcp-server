package apiexposes

import (
	"reflect"
	"testing"
)

// TestParseFeatures_ListsAreMappedToTheirTier verifies the reading of the
// feature table: every symbol under a tier's list lands under that tier, a
// starter list counts as premium, the global list is its own tier, a symbol
// in two lists keeps the highest, and a constant that is not an array is
// passed over.
func TestParseFeatures_ListsAreMappedToTheirTier(t *testing.T) {
	got := ParseFeatures([]byte(testFeatures + "    OTHER_FEATURES = %i[\n      unranked\n    ].freeze\n"))

	want := map[string]string{
		"elastic_search":                  TierGlobal,
		"merge_pipelines":                 TierPremium,
		"epics":                           TierPremium,
		"security_orchestration_policies": TierUltimate,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFeatures() = %v, want %v", got, want)
	}
}

// TestLicensedFeatures_ReadsEverySpellingOnce verifies the symbols a
// condition is read for: the three spellings the entities use, in order,
// without repeats, and nothing from a method that is not a license check.
func TestLicensedFeatures_ReadsEverySpellingOnce(t *testing.T) {
	condition := "->(p, _) { p.feature_available?(:a) && p.licensed_feature_available?( :b ) && ::License.feature_available?(:a) && p.enabled?(:c) }"

	if got := licensedFeatures(condition); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("licensedFeatures() = %v, want [a b]", got)
	}
	if got := licensedFeatures("->(_, o) { o[:x] }"); got != nil {
		t.Errorf("licensedFeatures() = %v, want nothing for a condition naming no feature", got)
	}
}

// TestTierOf_TakesTheHighestKnownTier verifies that a field behind several
// features is the tier of the most demanding one, and that a symbol the table
// does not list contributes nothing.
func TestTierOf_TakesTheHighestKnownTier(t *testing.T) {
	features := map[string]string{"a": TierPremium, "b": TierUltimate, "g": TierGlobal}
	cases := []struct {
		name    string
		symbols []string
		want    string
	}{
		{name: "premium alone", symbols: []string{"a"}, want: TierPremium},
		{name: "ultimate wins over premium", symbols: []string{"a", "b"}, want: TierUltimate},
		{name: "premium wins over global", symbols: []string{"g", "a"}, want: TierPremium},
		{name: "an unlisted symbol has no tier", symbols: []string{"issues"}, want: ""},
		{name: "nothing asked", symbols: nil, want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := tierOf(testCase.symbols, features); got != testCase.want {
				t.Errorf("tierOf(%v) = %q, want %q", testCase.symbols, got, testCase.want)
			}
		})
	}
}
