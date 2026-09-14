package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
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
	// The ce fixture records no commit git could resolve, so its cell is the
	// em dash rather than a truncated value that reads like a revision.
	if !strings.Contains(page, "`deadbeef`") {
		t.Error("the page does not carry the ce fixture's short commit")
	}
	if !strings.Contains(page, "| `dynamic`") {
		t.Error("the page carries no per-surface state table")
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
