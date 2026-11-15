// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// TestEntryLabel_Categories_NameTheFirstSiteOrTheCategory verifies how a
// report names an entry: an upstream wait by its first site, an unseen one
// when it lists no site, and the category alone when it lists neither, each
// followed by the first method or service it replaces when it replaces any;
// any other entry by its category.
func TestEntryLabel_Categories_NameTheFirstSiteOrTheCategory(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
		want  string
	}{
		{name: "upstream with sites", entry: Entry{Category: CategoryUpstream, Sites: []string{"features.Set", "fixture.setFeature"}, Unseen: []string{"a.Hint"}}, want: "upstream features.Set"},
		{name: "upstream with unseen sites alone", entry: Entry{Category: CategoryUpstream, Unseen: []string{"a.Hint"}}, want: "upstream a.Hint"},
		{name: "upstream with no site", entry: Entry{Category: CategoryUpstream}, want: "upstream"},
		{name: "upstream replacing methods", entry: Entry{Category: CategoryUpstream, Sites: []string{"projects.doProjectRequest"}, Replaces: []string{"Projects.AddProjectHook", "Projects.GetProjectHook"}}, want: "upstream projects.doProjectRequest for Projects.AddProjectHook"},
		{name: "upstream replacing a method with no site", entry: Entry{Category: CategoryUpstream, Replaces: []string{"Stats"}}, want: "upstream for Stats"},
		{name: "raw reason", entry: Entry{Category: CategoryProbe, Sites: []string{"fixture.probeREST"}}, want: CategoryProbe},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.entry.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEntries_ACallerEditingTheResult_LeavesTheTableAsItWas verifies the
// table is handed out as a copy, each entry's lists included, so a reader
// cannot change what the next one reads by replacing an entry or by writing
// into one of its sites, unseen functions, replaced methods or predicates.
func TestEntries_ACallerEditingTheResult_LeavesTheTableAsItWas(t *testing.T) {
	first := Entries()
	if len(first) != len(upstreamWaits)+len(rawReasons) || first[0].Label() != upstreamWaits[0].Label() {
		t.Fatalf("Entries() = %d entries starting at %q, want the upstream waits first", len(first), first[0].Label())
	}
	first[0] = Entry{}
	if Entries()[0].Label() != upstreamWaits[0].Label() {
		t.Error("editing what Entries returned changed the table")
	}
	want := fmt.Sprintf("%+v", Entries())
	var edited [4]bool
	for _, entry := range Entries() {
		edited[0] = overwriteFirst(entry.Sites, "edited.Site") || edited[0]
		edited[1] = overwriteFirst(entry.Unseen, "edited.Unseen") || edited[1]
		edited[2] = overwriteFirst(entry.Replaces, "Edited.Method") || edited[2]
		edited[3] = overwriteFirst(entry.Predicates, nil) || edited[3]
	}
	if slices.Contains(edited[:], false) {
		t.Fatalf("edited sites, unseen, replaces, predicates = %v, want an entry of the table listing each", edited)
	}
	if got := fmt.Sprintf("%+v", Entries()); got != want {
		t.Error("writing into the lists of what Entries returned changed the table")
	}
}

// overwriteFirst writes value over the first element of list and reports
// whether it had one to write over.
func overwriteFirst[T any](list []T, value T) bool {
	if len(list) == 0 {
		return false
	}
	list[0] = value
	return true
}

// TestCoveredRaw_Entries_SplitMethodsFromServicesAndKeepTheFirst verifies
// what R-ACTION and the service rule derive: a dotted key is a method, a bare
// one a service, each with the entry that replaces it and why, and a key two
// entries replace keeps the first.
func TestCoveredRaw_Entries_SplitMethodsFromServicesAndKeepTheFirst(t *testing.T) {
	entries := []Entry{
		{Category: CategoryUpstream, Sites: []string{"stats.fetch"}, Replaces: []string{"Stats", "Stats.Get"}, Reason: "counts"},
		{Category: CategoryURLBuilt, Replaces: []string{"Stats.Get", "Archive.Fetch"}, Reason: "a URL"},
	}
	wantMethods := map[string]string{"Stats.Get": "upstream stats.fetch for Stats: counts", "Archive.Fetch": "url-built: a URL"}
	if got := CoveredRawMethods(entries); !reflect.DeepEqual(got, wantMethods) {
		t.Errorf("CoveredRawMethods = %v, want %v", got, wantMethods)
	}
	wantServices := map[string]string{"Stats": "upstream stats.fetch for Stats: counts"}
	if got := CoveredRawServices(entries); !reflect.DeepEqual(got, wantServices) {
		t.Errorf("CoveredRawServices = %v, want %v", got, wantServices)
	}
}

// TestProblems_EntriesWrittenWrong_AreEachNamed verifies what is wrong with
// an entry whatever the tree says: an upstream wait with no predicate, an
// undefined category, a raw reason with a predicate, an entry with no site,
// and a method two entries replace. A well formed entry, with sites or
// unseen sites alone, has none.
func TestProblems_EntriesWrittenWrong_AreEachNamed(t *testing.T) {
	predicate := []Predicate{LacksKey{"T", "k"}}
	cases := []struct {
		name    string
		entries []Entry
		want    []string
	}{
		{name: "well formed", entries: []Entry{
			{Category: CategoryUpstream, Sites: []string{"a.B"}, Predicates: predicate, Replaces: []string{"S.M"}},
			{Category: CategoryProbe, Unseen: []string{"fixture.probe"}},
			{Category: CategoryURLBuilt, Sites: []string{"a.C"}, Unseen: []string{"a.D"}},
		}},
		{name: "no predicate", entries: []Entry{{Category: CategoryUpstream, Sites: []string{"a.B"}}}, want: []string{"upstream wait upstream a.B carries no predicate"}},
		{name: "undefined category", entries: []Entry{{Category: "decided", Sites: []string{"a.B"}}}, want: []string{`wait decided: category "decided" is not one the table defines`}},
		{name: "raw reason with a predicate", entries: []Entry{{Category: CategoryProbe, Sites: []string{"a.B"}, Predicates: predicate}}, want: []string{"raw reason probe carries a predicate, which only an upstream wait takes"}},
		{name: "no site", entries: []Entry{{Category: CategoryProbe}}, want: []string{"wait probe names no site"}},
		{name: "replaced twice", entries: []Entry{
			{Category: CategoryProbe, Sites: []string{"a.B"}, Replaces: []string{"S.M"}},
			{Category: CategoryURLBuilt, Sites: []string{"a.C"}, Replaces: []string{"S.M"}},
		}, want: []string{"wait url-built replaces S.M, which probe replaces too"}},
		{name: "two upstream waits labeled alike", entries: []Entry{
			{Category: CategoryUpstream, Sites: []string{"a.B"}, Predicates: predicate},
			{Category: CategoryUpstream, Sites: []string{"a.B", "a.C"}, Predicates: predicate},
		}, want: []string{"two upstream waits are both labeled upstream a.B"}},
		{name: "one site, two replaced methods", entries: []Entry{
			{Category: CategoryUpstream, Sites: []string{"a.B"}, Predicates: predicate, Replaces: []string{"S.M"}},
			{Category: CategoryUpstream, Sites: []string{"a.B"}, Predicates: predicate, Replaces: []string{"S.N"}},
		}},
		{name: "raw reasons sharing a category", entries: []Entry{
			{Category: CategoryProbe, Sites: []string{"a.B"}},
			{Category: CategoryProbe, Sites: []string{"a.C"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Problems(tc.entries); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Problems = %q, want %q", got, tc.want)
			}
		})
	}
}
