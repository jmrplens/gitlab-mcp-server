package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// pageFixture is a two-runtime document to draw.
func pageFixture(t *testing.T) *coverageRecord {
	t.Helper()
	ce, err := buildRecordEntry(shardReport(t, "ce"))
	if err != nil {
		t.Fatalf("build the ce entry: %v", err)
	}
	ee, err := buildRecordEntry(eeReport())
	if err != nil {
		t.Fatalf("build the ee entry: %v", err)
	}
	return &coverageRecord{
		SchemaVersion: recordSchemaVersion, Note: recordNote,
		Runtimes: map[string]*recordEntry{"ce": ce, "ee": ee},
	}
}

// TestRenderRecordPage_MatchesTheRecord verifies that the page states the
// record's own figures rather than a rounding of them, which is what keeps
// the two artifacts from drifting while both look current.
func TestRenderRecordPage_MatchesTheRecord(t *testing.T) {
	doc := pageFixture(t)
	page := renderRecordPage(doc)
	ce := doc.Runtimes["ce"]
	for _, want := range []string{
		"| `ce`", "| `ee`", "community/free", "enterprise/ultimate",
		ce.RetrievedAt, "18.4.0-ee", "`6bd82ea61e0e`",
		recordRelPath, pageRegenerate, "make check-e2e-coverage-record",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(page, want) {
				t.Errorf("the page does not carry %q", want)
			}
		})
	}
	// The ce fixture records deadbeef, which is shorter than the abbreviation
	// the ee row is cut to, so it is printed whole: a recorded revision is
	// never padded out to look longer than it is, and never cut to a width it
	// is already under.
	if !strings.Contains(page, "`deadbeef`") {
		t.Error("the page does not carry the ce fixture's short commit")
	}
	if !strings.Contains(page, "| `dynamic`") {
		t.Error("the page carries no per-surface state table")
	}
}

// TestRenderRecordPage_Rows_CopiedFromTheEntry verifies the three tables
// cell by cell for one runtime whose values all differ: the coverage row's
// three levels, the provenance row's requirement, status, version, tier
// confirmation and commit, and the fixture row's four flags, drawn from two
// packages whose profiles differ on every pair of flags, since a struct of
// booleans has no single row on which no two agree. The padding the table
// formatter adds is folded before the comparison.
func TestRenderRecordPage_Rows_CopiedFromTheEntry(t *testing.T) {
	doc := &coverageRecord{SchemaVersion: recordSchemaVersion, Note: recordNote, Runtimes: map[string]*recordEntry{"ee": {
		RetrievedAt: "2026-09-12", Runtime: "enterprise/ultimate", Edition: "enterprise", Tier: "ultimate",
		Runs: []runRow{
			{
				Package: "common", Requirement: "any", Status: e2ecalls.RunStarted, GitLabVersion: "18.4.0-ee",
				TierConfirmed: true, Commit: "6bd82ea61e0ee0751e28b0a75954b9e4b86a8648",
				Fixtures: e2ecalls.FixtureProfile{Runner: true, FixtureService: true},
			},
			{
				Package: "ee", Requirement: "licensed", Status: e2ecalls.RunRefused, GitLabVersion: "18.4.1-ee",
				TierConfirmed: false, Commit: "abc123",
				Fixtures: e2ecalls.FixtureProfile{Runner: true, Bitbucket: true},
			},
		},
		Summary: summary{CatalogActions: 20, TestCalls: 7, L1: 5, L2: 4, L3: 2},
	}}}
	page := renderRecordPage(doc)

	rows := map[string]bool{}
	for line := range strings.SplitSeq(page, "\n") {
		rows[strings.Join(strings.Fields(line), " ")] = true
	}
	for _, want := range []string{
		"| `ee` | enterprise/ultimate | 2026-09-12 | 20 | 5 (25.0%) | 4 (20.0%) | 2 (10.0%) | 7 |",
		"| `ee` | `common` | any | started | 18.4.0-ee | yes | `6bd82ea61e0e` |",
		"| `ee` | `ee` | licensed | refused | 18.4.1-ee | no | `abc123` |",
		"| `ee` | `common` | yes | yes | no | no |",
		"| `ee` | `ee` | yes | no | yes | no |",
	} {
		t.Run(want, func(t *testing.T) {
			if !rows[want] {
				t.Errorf("the page carries no row %q:\n%s", want, page)
			}
		})
	}
}

// TestRenderRecordPage_TablesAreAlreadyFormatted verifies that the page
// arrives in the form cmd/format_md_tables would put it in. Everything under
// docs/ is held to that formatter by make audit-docs, so a generated page
// that needed reformatting would fail the check on the first push.
func TestRenderRecordPage_TablesAreAlreadyFormatted(t *testing.T) {
	page := renderRecordPage(pageFixture(t))
	formatted, changed := docgen.FormatMarkdownTables(page)
	if changed || formatted != page {
		t.Error("the rendered page is not in the table formatter's normal form")
	}
}

// TestRenderRecordPage_EmptyRecord_StillAPage verifies that a document with
// no runtimes renders the prose and the empty tables rather than panicking,
// which is the shape a first write into a fresh checkout passes through.
func TestRenderRecordPage_EmptyRecord_StillAPage(t *testing.T) {
	page := renderRecordPage(&coverageRecord{SchemaVersion: recordSchemaVersion})
	if !strings.HasPrefix(page, "# E2E Coverage\n") {
		t.Errorf("the page does not open with its title:\n%s", page[:min(len(page), 80)])
	}
	if !strings.Contains(page, "## Coverage") || !strings.Contains(page, "## Refreshing this page") {
		t.Error("the page lost a section when the record held no runtime")
	}
}

// TestRenderRecordPage_NoCapabilities_DrawsNoCapabilityTable verifies that a
// runtime measured before the capability cells existed draws its surface
// table and nothing under it: a header introducing an empty table would read
// as a run that watched nothing rather than as a record that says nothing.
func TestRenderRecordPage_NoCapabilities_DrawsNoCapabilityTable(t *testing.T) {
	doc := pageFixture(t)
	for _, entry := range doc.Runtimes {
		entry.Summary.Capabilities = nil
	}
	page := renderRecordPage(doc)
	folded := strings.Join(strings.Fields(page), " ")

	for _, absent := range []string{"| Capability | asserted |", "| Capability surface |", "recorded before the grain above"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(folded, absent) {
				t.Errorf("the page carries %q for runtimes whose records hold no capability histogram", absent)
			}
		})
	}
	if !strings.Contains(page, "### ce") {
		t.Error("the page lost the runtime whose capability table it left out")
	}
}

// pageRows folds the padding out of every line of a page, so a test can look
// for a table row by its cells rather than by the widths the formatter chose.
func pageRows(page string) map[string]bool {
	rows := map[string]bool{}
	for line := range strings.SplitSeq(page, "\n") {
		rows[strings.Join(strings.Fields(line), " ")] = true
	}
	return rows
}

// runtimeSection is one runtime's part of the States section: from its
// heading to the next one, or to the end of the section.
func runtimeSection(t *testing.T, page, key string) string {
	t.Helper()
	_, section, found := strings.Cut(page, "### "+key+"\n")
	if !found {
		t.Fatalf("the page has no section for %s", key)
	}
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}
	section, _, _ = strings.Cut(section, "\n## ")
	return section
}

// TestRenderRecordPage_GrainTable_StatesEachKind verifies the table the page
// states the grain of every capability kind in, drawn from the model the fold
// keys its cells with.
func TestRenderRecordPage_GrainTable_StatesEachKind(t *testing.T) {
	rows := pageRows(renderRecordPage(pageFixture(t)))
	for _, want := range []string{
		"| Capability | One cell per item per |",
		"| `completions` | capability surface |",
		"| `elicitation` | surface x mode |",
		"| `modes` | surface x mode |",
		"| `prompts` | capability surface |",
		"| `resources` | capability surface |",
		"| `subscriptions` | capability surface |",
		"| `tool_manifest` | surface x mode x capability surface |",
	} {
		t.Run(want, func(t *testing.T) {
			if !rows[want] {
				t.Errorf("the page carries no row %q", want)
			}
		})
	}
}

// TestRenderRecordPage_CapabilitySurfaces_DrawnFromTheRows verifies that an
// entry recorded at the capability grain draws the rows its histogram is
// counted against, cell by cell, in its own section and says nothing of an
// older grain. The ee entry's figures all differ, so no two columns could be
// exchanged and still read right.
func TestRenderRecordPage_CapabilitySurfaces_DrawnFromTheRows(t *testing.T) {
	doc := pageFixture(t)
	doc.Runtimes["ee"].CapabilitySurfaces = []capabilitySurfaceRow{
		{Capabilities: "full", Sessions: 7, Shapes: 6, Resources: 43, Prompts: 37, Completions: 5, SubscribableKinds: 26},
		{Capabilities: "minimal", Sessions: 3, Shapes: 2, Completions: 1},
	}
	page := renderRecordPage(doc)
	ee := runtimeSection(t, page, "ee")
	rows := pageRows(ee)

	for _, want := range []string{
		"| Capability surface | Sessions | Shapes | Resources | Prompts | Completions | Subscribable kinds |",
		"| `full` | 7 | 6 | 43 | 37 | 5 | 26 |",
		"| `minimal` | 3 | 2 | 0 | 0 | 1 | 0 |",
	} {
		t.Run(want, func(t *testing.T) {
			if !rows[want] {
				t.Errorf("the ee section carries no row %q:\n%s", want, ee)
			}
		})
	}
	if strings.Contains(ee, "recorded before the grain above") {
		t.Error("the ee section calls an entry that carries its rows older than the grain")
	}
	if !strings.Contains(ee, "| `prompts`") {
		t.Error("the ee section lost its capability histogram")
	}
}

// TestRenderRecordPage_EntryBeforeTheGrain_NamesTheOlderGrain verifies what
// the page says of an entry recorded before the capability grain: in its own
// section, the grain its histogram was counted at and the target that
// re-records it, and no capability surface table, since it has no rows to draw
// one from. The runtime beside it, which carries its rows, says nothing of it.
func TestRenderRecordPage_EntryBeforeTheGrain_NamesTheOlderGrain(t *testing.T) {
	doc := pageFixture(t)
	doc.Runtimes["ee"].CapabilitySurfaces = nil
	page := renderRecordPage(doc)
	ee, ce := runtimeSection(t, page, "ee"), runtimeSection(t, page, "ce")

	want := "This entry was recorded before the grain above: each of its capability rows counts every item " +
		"once per surface x mode, whatever the kind, so an item every shape served is as many cells as there " +
		"are shapes. `make e2e-coverage-record-ee` re-records it at the grain above."
	if !strings.Contains(ee, want) {
		t.Errorf("the ee section does not name the older grain:\n%s", ee)
	}
	if strings.Contains(ee, "| Capability surface") {
		t.Error("the ee section drew a capability surface table with no rows to draw it from")
	}
	if !strings.Contains(ee, "| `prompts`") {
		t.Error("the ee section lost its capability histogram")
	}
	if strings.Contains(ce, "recorded before the grain above") || !strings.Contains(ce, "| Capability surface") {
		t.Errorf("the ce section, which carries its rows, = \n%s", ce)
	}
}

// TestRecordPageOrder_UnknownKey_ComesAfterTheKnownOnes verifies that a key
// nothing here writes is still drawn, and drawn last: the page is a reading
// of the record and hiding an entry would make the two disagree.
func TestRecordPageOrder_UnknownKey_ComesAfterTheKnownOnes(t *testing.T) {
	doc := pageFixture(t)
	doc.Runtimes["self-hosted"] = doc.Runtimes["ce"]
	got := recordPageOrder(doc)
	if want := []string{"ce", "ee", "self-hosted"}; !slices.Equal(got, want) {
		t.Errorf("recordPageOrder() = %q, want %q", got, want)
	}
}

// TestRecordPageOrder_NilEntry_IsSkippedWhenDrawn verifies that a key whose
// entry is null draws nothing rather than dereferencing it; the check
// reports the empty entry as a finding, and the page must still render so
// the comparison beside it can run.
func TestRecordPageOrder_NilEntry_IsSkippedWhenDrawn(t *testing.T) {
	doc := pageFixture(t)
	doc.Runtimes["ee"] = nil
	page := renderRecordPage(doc)
	if strings.Contains(page, "enterprise/ultimate") {
		t.Error("the page drew a runtime whose entry is null")
	}
	if !strings.Contains(page, "| `ce`") {
		t.Error("the page lost the runtime that is there")
	}
}

// TestShortCommit_Cases verifies the three shapes a recorded revision takes.
func TestShortCommit_Cases(t *testing.T) {
	cases := []struct {
		name   string
		commit string
		want   string
	}{
		{name: "none recorded", commit: "", want: "—"},
		{name: "already short", commit: "deadbeef", want: "`deadbeef`"},
		{name: "a full revision", commit: "6bd82ea61e0ee0751e28b0a75954b9e4b86a8648", want: "`6bd82ea61e0e`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortCommit(tc.commit); got != tc.want {
				t.Errorf("shortCommit(%q) = %q, want %q", tc.commit, got, tc.want)
			}
		})
	}
}

// TestYesNo_Cases verifies how a recorded flag is spelled.
func TestYesNo_Cases(t *testing.T) {
	if yesNo(true) != "yes" || yesNo(false) != "no" {
		t.Errorf("yesNo() = %q/%q, want yes/no", yesNo(true), yesNo(false))
	}
}
