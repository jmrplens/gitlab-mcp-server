// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestEntryLabel_Categories_NameTheRowOrTheCategory verifies how a report
// names an entry.
func TestEntryLabel_Categories_NameTheRowOrTheCategory(t *testing.T) {
	if got := (Entry{Row: 6, Category: CategoryUpstream}).Label(); got != "row 6" {
		t.Errorf("upstream label = %q, want row 6", got)
	}
	if got := (Entry{Category: CategoryProbe}).Label(); got != CategoryProbe {
		t.Errorf("raw reason label = %q, want %q", got, CategoryProbe)
	}
}

// TestEntries_ACallerEditingTheResult_LeavesTheTableAsItWas verifies the
// table is handed out as a copy, each entry's lists included, so a reader
// cannot change what the next one reads by replacing an entry or by writing
// into one of its sites, unseen functions, replaced methods or predicates.
func TestEntries_ACallerEditingTheResult_LeavesTheTableAsItWas(t *testing.T) {
	first := Entries()
	if len(first) != len(upstreamWaits)+len(rawReasons) || first[0].Row != upstreamWaits[0].Row {
		t.Fatalf("Entries() = %d entries starting at row %d, want the upstream waits first", len(first), first[0].Row)
	}
	first[0] = Entry{}
	if Entries()[0].Row != upstreamWaits[0].Row {
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
		{Row: 7, Category: CategoryUpstream, Replaces: []string{"Stats", "Stats.Get"}, Reason: "counts"},
		{Category: CategoryURLBuilt, Replaces: []string{"Stats.Get", "Archive.Fetch"}, Reason: "a URL"},
	}
	wantMethods := map[string]string{"Stats.Get": "row 7: counts", "Archive.Fetch": "url-built: a URL"}
	if got := CoveredRawMethods(entries); !reflect.DeepEqual(got, wantMethods) {
		t.Errorf("CoveredRawMethods = %v, want %v", got, wantMethods)
	}
	wantServices := map[string]string{"Stats": "row 7: counts"}
	if got := CoveredRawServices(entries); !reflect.DeepEqual(got, wantServices) {
		t.Errorf("CoveredRawServices = %v, want %v", got, wantServices)
	}
}

// TestProblems_EntriesWrittenWrong_AreEachNamed verifies what is wrong with
// an entry whatever the tree says: an upstream wait with no row or no
// predicate, an undefined category, a raw reason with a row or a predicate,
// an entry with no site, and a method two entries replace. A well formed
// entry, with sites or unseen sites alone, has none.
func TestProblems_EntriesWrittenWrong_AreEachNamed(t *testing.T) {
	predicate := []Predicate{LacksKey{"T", "k"}}
	cases := []struct {
		name    string
		entries []Entry
		want    []string
	}{
		{name: "well formed", entries: []Entry{
			{Row: 6, Category: CategoryUpstream, Sites: []string{"a.B"}, Predicates: predicate, Replaces: []string{"S.M"}},
			{Category: CategoryProbe, Unseen: []string{"fixture.probe"}},
			{Category: CategoryURLBuilt, Sites: []string{"a.C"}, Unseen: []string{"a.D"}},
		}},
		{name: "no row", entries: []Entry{{Category: CategoryUpstream, Sites: []string{"a.B"}, Predicates: predicate}}, want: []string{"upstream wait row 0 names no register row"}},
		{name: "no predicate", entries: []Entry{{Row: 6, Category: CategoryUpstream, Sites: []string{"a.B"}}}, want: []string{"upstream wait row 6 carries no predicate"}},
		{name: "undefined category", entries: []Entry{{Category: "decided", Sites: []string{"a.B"}}}, want: []string{`wait decided: category "decided" is not one the table defines`}},
		{name: "raw reason with a row", entries: []Entry{{Row: 6, Category: CategoryProbe, Sites: []string{"a.B"}}}, want: []string{"raw reason probe carries a row or a predicate, which only an upstream wait takes"}},
		{name: "raw reason with a predicate", entries: []Entry{{Category: CategoryProbe, Sites: []string{"a.B"}, Predicates: predicate}}, want: []string{"raw reason probe carries a row or a predicate, which only an upstream wait takes"}},
		{name: "no site", entries: []Entry{{Category: CategoryProbe}}, want: []string{"wait probe names no site"}},
		{name: "replaced twice", entries: []Entry{
			{Category: CategoryProbe, Sites: []string{"a.B"}, Replaces: []string{"S.M"}},
			{Category: CategoryURLBuilt, Sites: []string{"a.C"}, Replaces: []string{"S.M"}},
		}, want: []string{"wait url-built replaces S.M, which probe replaces too"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Problems(tc.entries); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Problems = %q, want %q", got, tc.want)
			}
		})
	}
}

// testRegister is a stand-in register: three summary rows, two of them
// linking to a section, prose that mentions a row, a row whose number is not
// spaced as the table spaces it, and sections at two depths.
const testRegister = "| # | Project | Title |\n" +
	"| - | ------- | ----- |\n" +
	"| 6 | client-go | [`SetWidgetFlagOptions` lacks `omitempty`](#setwidgetflagoptions-lacks-omitempty) |\n" +
	"| 140 | client-go | [Five structs, one section: answering -32700](#five-structs-one-section-answering--32700) |\n" +
	"| 141 | client-go | no link |\n" +
	"row 7 is mentioned in prose\n" +
	"|7| unspaced |\n" +
	"\n" +
	"### `SetWidgetFlagOptions` lacks `omitempty`\n" +
	"\n" +
	"The flag's body is posted by setFeature.\n" +
	"\n" +
	"#### A deeper heading stays in the section\n" +
	"\n" +
	"### Five structs, one section: answering -32700\n" +
	"\n" +
	"`Deployment` and `Job`.\n" +
	"\n" +
	"## Another project\n" +
	"\n" +
	"### Unlinked\n" +
	"\n" +
	"### `SetWidgetFlagOptions` lacks `omitempty`\n" +
	"\n" +
	"A later twin, linked by another anchor.\n"

// TestReadRows_ARegister_ReadsEachSummaryRowWithTheSectionItLinksTo verifies
// a row is read off the first cell of a table row, with its text: the line
// itself and the section its first link names, up to the next heading as
// deep or shallower, a deeper one inside it, and the first of two headings
// spelled alike. A row that links nowhere is its line alone, and a register
// that cannot be read is an error.
func TestReadRows_ARegister_ReadsEachSummaryRowWithTheSectionItLinksTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upstream-bugs.md")
	if err := os.WriteFile(path, []byte(testRegister), 0o600); err != nil {
		t.Fatalf("write the register: %v", err)
	}
	rows, err := ReadRows(path)
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("ReadRows read rows %v, want 6, 140 and 141", rows)
	}
	cases := []struct {
		row          int
		holds, lacks []string
	}{
		{row: 6, holds: []string{"| 6 | client-go |", "posted by setFeature", "A deeper heading stays"}, lacks: []string{"Deployment", "later twin"}},
		{row: 140, holds: []string{"| 140 |", "`Deployment` and `Job`"}, lacks: []string{"setFeature", "Another project", "Unlinked"}},
		{row: 141, holds: []string{"| 141 | client-go | no link |"}, lacks: []string{"\n"}},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.row), func(t *testing.T) {
			text, ok := rows[tc.row]
			if !ok {
				t.Fatalf("row %d not read", tc.row)
			}
			for _, fragment := range tc.holds {
				if !strings.Contains(text, fragment) {
					t.Errorf("row %d text %q lacks %q", tc.row, text, fragment)
				}
			}
			for _, fragment := range tc.lacks {
				if strings.Contains(text, fragment) {
					t.Errorf("row %d text %q holds %q", tc.row, text, fragment)
				}
			}
		})
	}
	if _, absentErr := ReadRows(filepath.Join(dir, "absent.md")); absentErr == nil || !strings.Contains(absentErr.Error(), "read the upstream register") {
		t.Errorf("ReadRows(absent) = %v, want a read error", absentErr)
	}
}

// TestHeadingAnchor_Headings_AreSpelledAsTheRegisterLinksThem verifies the
// anchor a heading is linked by: lower case, a space a hyphen, a hyphen and
// an underscore kept, and every other mark (a backtick, a comma, a colon, a
// parenthesis, a slash, a dot) dropped.
func TestHeadingAnchor_Headings_AreSpelledAsTheRegisterLinksThem(t *testing.T) {
	cases := map[string]string{
		"`ListPipelineTriggerJobsOptions` models no `include_retried`, which": "listpipelinetriggerjobsoptions-models-no-include_retried-which",
		"A malformed message ends the session instead of answering -32700":    "a-malformed-message-ends-the-session-instead-of-answering--32700",
		"MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)":               "mcp-go-sdk-githubcommodelcontextprotocolgo-sdk",
		"Row 7: 1,234 ñ": "row-7-1234-ñ",
	}
	for heading, want := range cases {
		t.Run(heading, func(t *testing.T) {
			if got := headingAnchor(heading); got != want {
				t.Errorf("headingAnchor(%q) = %q, want %q", heading, got, want)
			}
		})
	}
}

// namedEntry is an entry naming one of everything a row is held to mention.
var namedEntry = Entry{
	Row: 6, Category: CategoryUpstream,
	Sites:    []string{"features.Set", "fixture.setFeature", "widgets.lister.Fetch"},
	Unseen:   []string{"protectedpackages.Update"},
	Replaces: []string{"Features.SetFeatureFlag", "Epics"},
	Predicates: []Predicate{
		WritesAlways{"SetFeatureFlagOptions", "Key", EncoderJSON},
		NoMethodFor{"POST /orbit/mcp"},
		NoMethodFor{"GET /projects/:/repository/archive.:"},
		DecodesInto{"Projects.CreateProjectForkRelation", "*ProjectForkRelation"},
	},
}

// TestEntryNames_AnEntry_ListsWhatItsRowHasToMention verifies what an
// entry's register row is held to mention: what each predicate is about and
// each replaced method or service, a method also by its name alone (but not
// a route, whose path is no method even with a dot in it), and each site in
// full, an unexported function also by its name alone, since an exported one
// (Set, Update) is a word any section may hold.
func TestEntryNames_AnEntry_ListsWhatItsRowHasToMention(t *testing.T) {
	want := []string{
		"CreateProjectForkRelation", "Epics", "Features.SetFeatureFlag",
		"GET /projects/:/repository/archive.:", "POST /orbit/mcp",
		"Projects.CreateProjectForkRelation", "SetFeatureFlag", "SetFeatureFlagOptions",
		"features.Set", "fixture.setFeature", "protectedpackages.Update", "setFeature", "widgets.lister.Fetch",
	}
	if got := namedEntry.Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("Names() =\n%q\nwant\n%q", got, want)
	}
}

// TestEntryNamedIn_Texts_MatchOnlyAWholeName verifies a row mentions an
// entry when it holds one of its names as a whole word, at either end of the
// text or between marks, and not as the start of a longer name.
func TestEntryNamedIn_Texts_MatchOnlyAWholeName(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{text: "the flag is posted by `setFeature`", want: true},
		{text: "setFeature", want: true},
		{text: "the route POST /orbit/mcp.", want: true},
		{text: "a setFeatureFlag call and features.Settings"},
		{text: "nothing here"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			if got := namedEntry.NamedIn(tc.text); got != tc.want {
				t.Errorf("NamedIn(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
