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

	if strings.Contains(page, "classified on the same terms") {
		t.Error("the page introduced a capability table for a runtime whose record holds none")
	}
	if !strings.Contains(page, "### ce") {
		t.Error("the page lost the runtime whose capability table it left out")
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
