package main

import (
	"strings"
	"testing"
)

// agreeingProvenance is two runs of the same configuration, differing only in
// the two fields a merge is allowed to differ in.
func agreeingProvenance() (standing, incoming provenance) {
	one := provenance{
		Commit:            "1111111111",
		Date:              "2026-09-16",
		GitLabVersion:     "19.4.1",
		Edition:           "community",
		TierConfirmed:     true,
		CapabilitySurface: "full",
		TokenScopes:       []string{"api", "read_api"},
		ServedTools:       2,
		Provider:          "anthropic",
		RequestOptions:    map[string]string{"temperature": "0", "max_tokens": "4096"},
	}
	other := one
	other.Commit, other.Date = "2222222222", "2026-10-07"
	return one, other
}

// TestProvenanceDifference_NamesTheFirstDisagreement walks every field the row
// key does not hold.
//
// The key identifies a measurement; these describe the configuration the
// figures were produced under, and a merge across any of them would publish two
// measurements under one heading with the last run's label on both. The two
// fields that are deliberately not compared are asserted too, because a merge
// that refused them would refuse every merge there is: they differ between any
// two runs, which is the whole point of merging one into another.
func TestProvenanceDifference_NamesTheFirstDisagreement(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		edit  func(*provenance)
		names string
	}{
		{name: "the same configuration", edit: func(*provenance) {}},
		{name: "a newer GitLab", edit: func(p *provenance) { p.GitLabVersion = "19.5.0" }, names: "the GitLab version"},
		{name: "another edition", edit: func(p *provenance) { p.Edition = "enterprise" }, names: "the instance edition"},
		{name: "another capability surface", edit: func(p *provenance) { p.CapabilitySurface = "minimal" }, names: "the capability surface"},
		{name: "another provider", edit: func(p *provenance) { p.Provider = "openai" }, names: "the provider"},
		{name: "a tier nobody confirmed", edit: func(p *provenance) { p.TierConfirmed = false }, names: "whether the tier was confirmed"},
		{name: "a session serving more tools", edit: func(p *provenance) { p.ServedTools = 1091 }, names: "how many tools the session served"},
		{name: "a narrower credential", edit: func(p *provenance) { p.TokenScopes = []string{"read_api"} }, names: "the credential's scopes"},
		{name: "another temperature", edit: func(p *provenance) { p.RequestOptions = map[string]string{"temperature": "1"} }, names: "the request options"},
		{name: "a later commit is not a disagreement", edit: func(p *provenance) { p.Commit = "3333333333" }},
		{name: "a later date is not a disagreement", edit: func(p *provenance) { p.Date = "2027-01-01" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			standing, incoming := agreeingProvenance()
			testCase.edit(&incoming)

			got := provenanceDifference(standing, incoming)
			if testCase.names == "" {
				if got != "" {
					t.Errorf("provenanceDifference = %q, want the merge allowed", got)
				}
				return
			}
			if !strings.Contains(got, testCase.names) {
				t.Errorf("provenanceDifference = %q, want it to name %q", got, testCase.names)
			}
		})
	}
}

// TestRenderOptions_SpellsAMapInAStableOrder keeps a refusal reading the same
// however the maps were built. Map iteration is unordered, so a refusal that
// printed them as they came would differ between two runs of the same fold and
// a reader could not tell a new disagreement from the same one reworded.
func TestRenderOptions_SpellsAMapInAStableOrder(t *testing.T) {
	if got := renderOptions(nil); got != "none" {
		t.Errorf("renderOptions(nil) = %q, want %q", got, "none")
	}
	want := "max_tokens=4096, temperature=0"
	for range 8 {
		if got := renderOptions(map[string]string{"temperature": "0", "max_tokens": "4096"}); got != want {
			t.Fatalf("renderOptions = %q, want %q on every call", got, want)
		}
	}
}

// TestMergeShown_WidensTheSpanAndKeepsSilenceSilent covers the shown span a
// merged row publishes.
//
// A nil span is a run that recorded nothing about what it was shown, which is
// not the same as a run shown nothing: contributing a zero would drop the
// merged row's floor to zero and say the model was once shown no tools at all.
func TestMergeShown_WidensTheSpanAndKeepsSilenceSilent(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		span, other *shown
		want        *shown
	}{
		{name: "neither recorded anything"},
		{name: "only the incoming run did", other: &shown{Min: 96, Max: 128, Overflowed: 1}, want: &shown{Min: 96, Max: 128, Overflowed: 1}},
		{name: "only the standing row did", span: &shown{Min: 96, Max: 128}, want: &shown{Min: 96, Max: 128}},
		{
			name: "both, and the span widens both ways",
			span: &shown{Min: 96, Max: 128, Overflowed: 1}, other: &shown{Min: 64, Max: 312, Overflowed: 2},
			want: &shown{Min: 64, Max: 312, Overflowed: 3},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			// Kept to compare afterwards: a merge that widened the committed
			// row's own span in place would edit the record on a path that may
			// still refuse the merge, leaving a row neither run measured.
			var standingBefore, incomingBefore shown
			if testCase.span != nil {
				standingBefore = *testCase.span
			}
			if testCase.other != nil {
				incomingBefore = *testCase.other
			}

			got := mergeShown(testCase.span, testCase.other)
			switch {
			case testCase.want == nil && got != nil:
				t.Fatalf("mergeShown = %+v, want nothing recorded", got)
			case testCase.want == nil:
				return
			case got == nil:
				t.Fatalf("mergeShown = nil, want %+v", testCase.want)
			case *got != *testCase.want:
				t.Errorf("mergeShown = %+v, want %+v", *got, *testCase.want)
			}
			if testCase.span != nil && *testCase.span != standingBefore {
				t.Errorf("mergeShown edited the standing span in place: %+v, was %+v", *testCase.span, standingBefore)
			}
			if testCase.other != nil && *testCase.other != incomingBefore {
				t.Errorf("mergeShown edited the incoming span in place: %+v, was %+v", *testCase.other, incomingBefore)
			}
		})
	}
}
