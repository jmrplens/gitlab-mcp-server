package actioncompat

import (
	"reflect"
	"testing"
)

// TestActionAliases_PublishedOrder_IsByCanonicalThenAlias asserts the order
// ActionAliases returns, which is the order every reader downstream inherits:
// ApplyToActionSpecs groups the table by canonical ID and appends each group's
// aliases in this order, so it decides the sequence of ActionAliasSpec entries
// a tool's compatibility policy publishes.
//
// It states the ordering as a property of the whole published table rather than
// by naming the first two entries, because the comparator has two independent
// decisions in it and a fixture whose alias order happens to agree with its
// canonical order cannot tell them apart. The counters make the claim
// non-vacuous: the table must really contain both an adjacent pair that changes
// canonical and an adjacent pair that shares one, or the assertion would hold
// over an ordering that never exercised either branch.
func TestActionAliases_PublishedOrder_IsByCanonicalThenAlias(t *testing.T) {
	aliases := ActionAliases()
	if len(aliases) < 2 {
		t.Fatalf("ActionAliases() returned %d entries, want at least 2 to compare an order", len(aliases))
	}

	canonicalSteps, aliasSteps := 0, 0
	for index := 1; index < len(aliases); index++ {
		previous, current := aliases[index-1], aliases[index]
		switch {
		case previous.Canonical > current.Canonical:
			t.Fatalf("entry %d canonical %q sorts after entry %d canonical %q", index-1, previous.Canonical, index, current.Canonical)
		case previous.Canonical < current.Canonical:
			canonicalSteps++
		case previous.Alias >= current.Alias:
			t.Fatalf("entries %d and %d share canonical %q but aliases %q and %q are not strictly ascending",
				index-1, index, current.Canonical, previous.Alias, current.Alias)
		default:
			aliasSteps++
		}
	}

	if canonicalSteps == 0 {
		t.Error("no adjacent pair changes canonical, so the canonical half of the order was never exercised")
	}
	if aliasSteps == 0 {
		t.Error("no adjacent pair shares a canonical, so the alias tie-break was never exercised")
	}
}

// TestCloneActionAliases_CanonicalOutranksAlias_OrdersByCanonical pins the
// precedence between the comparator's two keys with a table whose alias order
// contradicts its canonical order. Every entry of the shipped table agrees on
// both keys, so a comparator that ignored the canonical entirely would sort the
// real data identically and nothing would notice; these two rows are the only
// way to state that the canonical decides first.
func TestCloneActionAliases_CanonicalOutranksAlias_OrdersByCanonical(t *testing.T) {
	aliases := cloneActionAliases([]ActionAlias{
		{Alias: "a.first_by_alias", Canonical: "z.last_by_canonical"},
		{Alias: "z.last_by_alias", Canonical: "a.first_by_canonical"},
	})

	gotCanonicals := []string{aliases[0].Canonical, aliases[1].Canonical}
	wantCanonicals := []string{"a.first_by_canonical", "z.last_by_canonical"}
	if !reflect.DeepEqual(gotCanonicals, wantCanonicals) {
		t.Fatalf("canonical order = %#v, want %#v: the canonical must decide before the alias", gotCanonicals, wantCanonicals)
	}
}

// TestCloneActionAliases_EqualKeys_KeepDeclarationOrder asserts the stability
// the clone is built on: two entries that compare equal on both keys come back
// in the order the table declares them.
//
// That is why the sort is sort.SliceStable rather than sort.Slice. The shipped
// table declares no alias twice today, so nothing else in the package can
// observe the choice, and a comparator that answered "true" for two equal keys
// would silently reverse such a pair the day one is added: the reason and the
// source a reader is shown would then be the second declaration's.
func TestCloneActionAliases_EqualKeys_KeepDeclarationOrder(t *testing.T) {
	aliases := cloneActionAliases([]ActionAlias{
		{Alias: "same.alias", Canonical: "same.canonical", Source: SourceCompatibility},
		{Alias: "same.alias", Canonical: "same.canonical", Source: SourceStandalone},
	})

	gotSources := []string{aliases[0].Source, aliases[1].Source}
	wantSources := []string{SourceCompatibility, SourceStandalone}
	if !reflect.DeepEqual(gotSources, wantSources) {
		t.Fatalf("source order = %#v, want %#v: equal keys must keep declaration order", gotSources, wantSources)
	}
}

// TestCompactStrings_LengthBoundary_CollapsesTwoEqualValues covers the length
// guard at the value it turns on. compactStrings is what makes
// NormalizeActionAlias able to answer at all: the index behind it collects one
// canonical per declaration of an alias, and the caller reads the count, so an
// alias declared twice for a single canonical must arrive as one match or the
// alias is rejected as ambiguous against itself. Two entries is the smallest
// input where that can happen and the exact length the guard decides.
func TestCompactStrings_LengthBoundary_CollapsesTwoEqualValues(t *testing.T) {
	testCases := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "two equal values collapse to one", values: []string{"issue.list", "issue.list"}, want: []string{"issue.list"}},
		{name: "two distinct values are both kept", values: []string{"issue.list", "issue.update"}, want: []string{"issue.list", "issue.update"}},
		{name: "one value is returned unchanged", values: []string{"issue.list"}, want: []string{"issue.list"}},
		{name: "no values are returned unchanged", values: []string{}, want: []string{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := compactStrings(testCase.values); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("compactStrings(%#v) = %#v, want %#v", testCase.values, got, testCase.want)
			}
		})
	}
}
