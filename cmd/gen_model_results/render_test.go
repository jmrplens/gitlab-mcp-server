package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twoModels is a published table: one configuration, two models, figures that
// differ in every column so a renderer that printed one row's numbers twice
// would be caught.
func twoModels() []row {
	first := row{
		Key: rowKey{
			Model: "anthropic:test-model", Surface: "dynamic", Mode: "default", Tier: "ultimate",
			CorpusDigest: "corpus-1", ContractDigest: "contract-1", ToolSchemaDigest: "tools-1", Repeat: 1,
		},
		Provenance: provenance{
			Commit: fixtureCommit, Date: "2026-09-16", GitLabVersion: "19.3.0", Edition: "enterprise",
			Tier: "ultimate", TierConfirmed: true, Surface: "dynamic", Mode: "default",
			CapabilitySurface: "full", TokenScopes: []string{"api"}, ServedTools: 2,
			Provider: "anthropic", Model: "test-model",
			RequestOptions: map[string]string{"temperature": "0", "max_tokens": "4096"},
			Repeat:         1, CorpusDigest: "corpus-1", ContractDigest: "contract-1",
		},
		Counts: counts{
			Attempts: 10, Turns: 31, Skipped: 2, Unobserved: 1, ProviderErrors: 1, HarnessErrors: 0,
			GitLabRefused: 1, Outcomes: map[string]int{"completed": 8, "failed": 2},
		},
		Columns: columns{
			Reached:           ratio{Numerator: 18, Denominator: 20},
			AcceptedFirstTime: ratio{Numerator: 15, Denominator: 18},
			ArgumentFidelity:  ratio{Numerator: 22, Denominator: 24},
			Confirmation:      ratio{Numerator: 3, Denominator: 4},
			Unaided:           ratio{Numerator: 7, Denominator: 10},
			Completion:        ratio{Numerator: 8, Denominator: 10},
			Overhead:          overhead{Discovery: 9, InvalidParams: 3, Steps: 18},
		},
		Tokens: tokens{Input: 120000, Output: 4200, CacheCreated: 30000, CacheRead: 88000},
	}
	second := first
	second.Key.Model = "openai:other-model"
	second.Provenance.Provider = "openai"
	second.Provenance.Model = "other-model"
	second.Provenance.RequestOptions = map[string]string{"reasoning_effort": "medium", "max_tokens": "4096"}
	second.Counts = counts{Attempts: 10, Turns: 40, Outcomes: map[string]int{"completed": 5, "failed": 5}}
	second.Columns = columns{
		Reached:           ratio{Numerator: 14, Denominator: 20},
		AcceptedFirstTime: ratio{Numerator: 9, Denominator: 14},
		ArgumentFidelity:  ratio{Numerator: 17, Denominator: 24},
		Confirmation:      ratio{},
		Unaided:           ratio{Numerator: 3, Denominator: 10},
		Completion:        ratio{Numerator: 5, Denominator: 10},
		Overhead:          overhead{Discovery: 14, InvalidParams: 11, Steps: 14},
	}
	second.Tokens = tokens{Input: 180000, Output: 9100, CacheCreated: 0, CacheRead: 0}
	return []row{first, second}
}

// blockNamed finds a declared block by its start marker, so a test names the
// block a reader sees rather than an index into a table that may be reordered.
func blockNamed(t *testing.T, start string) block {
	t.Helper()
	for _, one := range blocks {
		if one.Start == start {
			return one
		}
	}
	t.Fatalf("no block starts with %q", start)
	return block{}
}

// TestRenderBlock_ADetailedTable_MatchesTheGoldenPage is the rendering held to
// a page a person read once. Every figure a reader meets is in it: the caption
// that says what the table holds fixed, the seven columns with both halves of
// every ratio, what the columns leave out, the four token numbers and what
// produced each row.
func TestRenderBlock_ADetailedTable_MatchesTheGoldenPage(t *testing.T) {
	rows := twoModels()
	got := renderBlock(blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->"), rows)

	golden := filepath.Join("testdata", "enterprise-dynamic-block.md")
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read the golden page: %v", err)
	}
	if got != strings.TrimRight(string(want), "\n") {
		t.Errorf("the rendered block does not match %s.\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}

// TestRenderBlock_ASummaryTable_CarriesTheColumnsAndNothingElse holds the split
// between the two files: the README says how it went, and the reference page
// says what it was measured on.
func TestRenderBlock_ASummaryTable_CarriesTheColumnsAndNothingElse(t *testing.T) {
	got := renderBlock(blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->"), twoModels())

	if !strings.Contains(got, "18 / 20") {
		t.Error("the summary does not carry the reached column with both its halves")
	}
	for _, absent := range []string{"Token scopes", "Cache created", "What the columns leave out"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(got, absent) {
				t.Errorf("the summary carries %q, which belongs on the reference page", absent)
			}
		})
	}
}

// TestRenderBlock_NoRows_SaysWhereTheWithdrawnTableWent is the state this step
// leaves the repository in, and it is generated rather than left in the file by
// hand: a hand-written block between generated markers is one the gate cannot
// tell from a stale one.
func TestRenderBlock_NoRows_SaysWhereTheWithdrawnTableWent(t *testing.T) {
	for _, one := range blocks {
		if one.Legend {
			// The worked example carries no rows by design and is drawn
			// whether or not anything is published, so it has no empty
			// sentence to say.
			continue
		}
		t.Run(one.Start, func(t *testing.T) {
			got := renderBlock(one, nil)
			if got != one.Empty {
				t.Errorf("an empty block rendered %q, want its declared sentence", got)
			}
			if got == "" {
				t.Error("an empty block says nothing at all, so a reader who followed a link is told nothing")
			}
		})
	}
}

// TestRenderBlock_SelectsBySurfaceAndLicence keeps a row out of every block but
// its own, which is what makes four blocks four measurements rather than one
// table printed four times.
func TestRenderBlock_SelectsBySurfaceAndLicence(t *testing.T) {
	free := twoModels()[0]
	free.Key.Tier = "free"
	free.Provenance.Tier = "free"

	licensedBlock := blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->")
	if renderBlock(licensedBlock, []row{free}) != licensedBlock.Empty {
		t.Error("a Free row was published in the Enterprise block")
	}
	freeBlock := blockNamed(t, "<!-- START MODEL EVAL DYNAMIC SUMMARY -->")
	if renderBlock(freeBlock, []row{free}) == freeBlock.Empty {
		t.Error("a Free row was not published in the CE block")
	}
	metaBlock := blockNamed(t, "<!-- START MODEL EVAL META SUMMARY -->")
	if renderBlock(metaBlock, []row{free}) != metaBlock.Empty {
		t.Error("a dynamic row was published in the meta block")
	}
}

// TestRenderBlock_AnOpaqueMetaRow_CarriesWhatItIsNotComparableWith is the
// sentence section 4.5 of the rebuild plan requires beside such a row, printed
// where the comparison would be made rather than in a footnote.
func TestRenderBlock_AnOpaqueMetaRow_CarriesWhatItIsNotComparableWith(t *testing.T) {
	meta := twoModels()[0]
	meta.Key.Surface = "meta"
	meta.Key.MetaParamSchema = opaqueSchema
	meta.Provenance.Surface = "meta"
	meta.Provenance.MetaParamSchema = opaqueSchema

	got := renderBlock(blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->"), []row{meta})
	if !strings.Contains(got, "withholds parameter") {
		t.Errorf("an opaque meta table does not say what its argument fidelity means:\n%s", got)
	}

	compact := meta
	compact.Key.MetaParamSchema = "compact"
	compact.Provenance.MetaParamSchema = "compact"
	if drawn := renderBlock(blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->"), []row{compact}); strings.Contains(drawn, "withholds parameter") {
		t.Error("a compact meta table carries the opaque sentence, which is not true of it")
	}
}

// TestRenderBlock_TwoConfigurations_AreTwoCaptionedTables is the rule that
// anything the comparison key does not admit goes in a table of its own, naming
// what differs. A protective-mode row beside a default one in one table is the
// fold the withdrawn figures were published through.
func TestRenderBlock_TwoConfigurations_AreTwoCaptionedTables(t *testing.T) {
	rows := twoModels()
	readOnly := rows[0]
	readOnly.Key.Mode = "read-only"
	readOnly.Provenance.Mode = "read-only"

	got := renderBlock(blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->"), append(rows, readOnly))
	if captions := strings.Count(got, "Compared as one table"); captions != 2 {
		t.Errorf("rendered %d captioned table(s), want one per configuration:\n%s", captions, got)
	}
	if !strings.Contains(got, "mode `read-only`") {
		t.Error("the second table does not name the mode that put it apart")
	}
}

// TestCrossSurfaceNotes_NameThePeerRowAndNothingElse is the second comparison
// key made visible to a reader: which row of another surface this one may be
// read beside, and silence when there is none.
func TestCrossSurfaceNotes_NameThePeerRowAndNothingElse(t *testing.T) {
	rows := twoModels()
	peer := rows[0]
	peer.Key.Surface = "meta"
	peer.Key.MetaParamSchema = opaqueSchema
	peer.Key.ToolSchemaDigest = "tools-meta"

	note := crossSurfaceNotes(rows[:1], append(rows, peer))
	if !strings.Contains(note, "`meta` row") {
		t.Errorf("the note does not name the surface the row may be read beside:\n%s", note)
	}
	if !strings.Contains(note, "(meta schema `opaque`)") {
		t.Errorf("the note does not say what the peer decided for itself:\n%s", note)
	}
	if strings.Contains(note, "openai:other-model") {
		t.Error("the note names a model with no peer")
	}
	if got := crossSurfaceNotes(rows[:1], rows); got != "" {
		t.Errorf("a row with no peer produced the note %q", got)
	}
}

// TestRenderBlock_ADetailedTableWithAPeer_CarriesTheCrossSurfaceNote is the
// second comparison key reaching the page a reader holds, rather than living
// only in a test.
func TestRenderBlock_ADetailedTableWithAPeer_CarriesTheCrossSurfaceNote(t *testing.T) {
	rows := twoModels()
	peer := rows[0]
	peer.Key.Surface = "meta"
	peer.Key.MetaParamSchema = opaqueSchema
	peer.Key.ToolSchemaDigest = "tools-meta"
	peer.Provenance.Surface = "meta"

	got := renderBlock(blockNamed(t, "<!-- START MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->"), append(rows, peer))
	if !strings.Contains(got, "Cross-surface comparisons this table takes part in") {
		t.Errorf("the detailed block carries no cross-surface note:\n%s", got)
	}
	if !strings.Contains(got, "may be read beside its `meta` row") {
		t.Errorf("the note does not name the peer surface:\n%s", got)
	}
}

// TestCrossSurfaceNotes_APeerMeasuredTwice_IsNamedOnce keeps the sentence
// readable when one model was measured on one other surface in two sessions.
func TestCrossSurfaceNotes_APeerMeasuredTwice_IsNamedOnce(t *testing.T) {
	rows := twoModels()
	first := rows[0]
	first.Key.Surface = "meta"
	first.Key.ToolSchemaDigest = "tools-meta-1"
	second := first
	second.Key.ToolSchemaDigest = "tools-meta-2"

	note := crossSurfaceNotes(rows[:1], []row{rows[0], first, second})
	if strings.Count(note, "`meta`") != 1 {
		t.Errorf("the note names the peer surface more than once:\n%s", note)
	}
}

// TestUnpublishedRows_AnIndividualRow_IsReportedRatherThanDropped is what
// stands between a measurement and silence. No block publishes the individual
// surface yet, so a record holding such a row would have it scored, committed
// and shown to nobody.
func TestUnpublishedRows_AnIndividualRow_IsReportedRatherThanDropped(t *testing.T) {
	individual := twoModels()[0]
	individual.Key.Surface = "individual"
	individual.Key.SliceSize = 128

	orphans := unpublishedRows(append(twoModels(), individual))
	if len(orphans) != 1 {
		t.Fatalf("got %d unpublished row(s), want the individual one alone", len(orphans))
	}
	if !strings.Contains(orphans[0], "surface=individual") {
		t.Errorf("the reported row %q does not name the surface no block publishes", orphans[0])
	}
}

// TestOverheadCell_PrintsBothHalves keeps the two costs apart: a catalog search
// is the dynamic surface's declared price, and an argument refusal is a model
// learning a name from a rejection.
func TestOverheadCell_PrintsBothHalves(t *testing.T) {
	got := overheadCell(overhead{Discovery: 9, InvalidParams: 3, Steps: 18})
	for _, want := range []string{"12 / 18", "9 find", "3 invalid_params"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("the overhead cell %q does not carry %q", got, want)
			}
		})
	}
	if empty := overheadCell(overhead{}); empty != "-" {
		t.Errorf("an overhead over no step printed %q, want a dash", empty)
	}
}

// TestRatio_String_PrintsBothNumbersAndADashOnNothing is the rule the withdrawn
// tables broke: a rate with nothing behind it read as a perfect score.
func TestRatio_String_PrintsBothNumbersAndADashOnNothing(t *testing.T) {
	if got := (ratio{Numerator: 3, Denominator: 4}).String(); got != "3 / 4" {
		t.Errorf("ratio printed %q, want both numbers", got)
	}
	if got := (ratio{}).String(); got != "-" {
		t.Errorf("an empty denominator printed %q, want a dash", got)
	}
}

// TestRequestOptions_AreSortedAndNeverEmpty keeps a re-render byte for byte
// what the last one was, whatever order a map is walked in.
func TestRequestOptions_AreSortedAndNeverEmpty(t *testing.T) {
	got := requestOptions(map[string]string{"temperature": "0", "max_tokens": "4096"})
	if got != "`max_tokens=4096`, `temperature=0`" {
		t.Errorf("request options printed %q, want them sorted", got)
	}
	if empty := requestOptions(nil); empty != "-" {
		t.Errorf("no options printed %q, want a dash", empty)
	}
}

// TestShortCommit_KeepsAShortOneWhole covers both sides of a cell a reader
// scans: a full revision is trimmed to what they can read, and anything already
// short is left alone rather than padded or cut.
func TestShortCommit_KeepsAShortOneWhole(t *testing.T) {
	if got := shortCommit(fixtureCommit); got != fixtureCommit[:9] {
		t.Errorf("a full revision printed %q", got)
	}
	if got := shortCommit("abc1234"); got != "abc1234" {
		t.Errorf("a short revision printed %q, want it unchanged", got)
	}
}
