package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
)

// oneRow is a row that agrees with its siblings on everything, so a test below
// can differ from it in one field and say what that one field decides.
func oneRow() row {
	return row{Key: rowKey{
		Model:            fixtureModel,
		Surface:          "dynamic",
		Mode:             "default",
		Tier:             "ultimate",
		CorpusDigest:     "corpus-1",
		ContractDigest:   fixtureContract,
		ToolSchemaDigest: fixtureTools,
		Repeat:           1,
	}}
}

// TestCrossVendorKey_DiffersInEveryFieldButTheModel is the whole of the
// cross-vendor rule: the model is what such a table varies, and everything else
// puts a row in a table of its own.
func TestCrossVendorKey_DiffersInEveryFieldButTheModel(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*row)
		apart bool
	}{
		{"another model", func(r *row) { r.Key.Model = "openai:other" }, false},
		{"another surface", func(r *row) { r.Key.Surface = "meta" }, true},
		{"another mode", func(r *row) { r.Key.Mode = "read-only" }, true},
		{"another tier", func(r *row) { r.Key.Tier = "free" }, true},
		{"a tier pin", func(r *row) { r.Key.TierPin = "premium" }, true},
		{"another meta schema", func(r *row) { r.Key.MetaParamSchema = "compact" }, true},
		{"another slice size", func(r *row) { r.Key.SliceSize = 128 }, true},
		{"another corpus", func(r *row) { r.Key.CorpusDigest = "corpus-2" }, true},
		{"another contract", func(r *row) { r.Key.ContractDigest = "contract-2" }, true},
		{"another tool schema", func(r *row) { r.Key.ToolSchemaDigest = "tools-2" }, true},
		{"another repeat", func(r *row) { r.Key.Repeat = 3 }, true},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			other := oneRow()
			one.edit(&other)
			apart := crossVendorKey(oneRow()) != crossVendorKey(other)
			if apart != one.apart {
				t.Errorf("rows differing by %s are in %d table(s), want %d",
					one.name, tables(apart), tables(one.apart))
			}
		})
	}
}

// tables spells a boolean as the number of tables it produces, so the failure
// above reads as the decision it is about.
func tables(apart bool) int {
	if apart {
		return 2
	}
	return 1
}

// TestCrossSurfaceKey_VariesTheSurfaceAndWhatTheSurfaceDecides is the second
// rule, and the one a single key could not have expressed: a cross-surface
// table must vary the surface, and with it the three things a surface decides
// for itself, while holding everything else.
func TestCrossSurfaceKey_VariesTheSurfaceAndWhatTheSurfaceDecides(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*row)
		apart bool
	}{
		{"another surface", func(r *row) { r.Key.Surface = "meta" }, false},
		{"another meta schema", func(r *row) { r.Key.MetaParamSchema = "opaque" }, false},
		{"another slice size", func(r *row) { r.Key.SliceSize = 128 }, false},
		{"another tool schema", func(r *row) { r.Key.ToolSchemaDigest = "tools-2" }, false},
		{"another model", func(r *row) { r.Key.Model = "openai:other" }, true},
		{"another mode", func(r *row) { r.Key.Mode = "safe" }, true},
		{"another tier", func(r *row) { r.Key.Tier = "free" }, true},
		{"a tier pin", func(r *row) { r.Key.TierPin = "premium" }, true},
		{"another corpus", func(r *row) { r.Key.CorpusDigest = "corpus-2" }, true},
		{"another contract", func(r *row) { r.Key.ContractDigest = "contract-2" }, true},
		{"another repeat", func(r *row) { r.Key.Repeat = 3 }, true},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			other := oneRow()
			one.edit(&other)
			apart := crossSurfaceKey(oneRow()) != crossSurfaceKey(other)
			if apart != one.apart {
				t.Errorf("rows differing by %s are in %d comparison(s), want %d",
					one.name, tables(apart), tables(one.apart))
			}
		})
	}
}

// TestGroupBySurface_RowsDifferingInSurfaceAndNothingElse_AreOneComparison is
// the table the first draft of this rule would have forbidden: one model,
// measured on the three surfaces, is one comparison and not three.
func TestGroupBySurface_RowsDifferingInSurfaceAndNothingElse_AreOneComparison(t *testing.T) {
	dynamic := oneRow()
	meta := oneRow()
	meta.Key.Surface = "meta"
	meta.Key.MetaParamSchema = opaqueSchema
	meta.Key.ToolSchemaDigest = "tools-meta"
	individual := oneRow()
	individual.Key.Surface = "individual"
	individual.Key.SliceSize = 128
	individual.Key.ToolSchemaDigest = "tools-individual"

	groups := groupBySurface([]row{meta, individual, dynamic})
	if len(groups) != 1 {
		t.Fatalf("got %d comparisons, want the three surfaces of one model in one", len(groups))
	}
	if got := groups[0].surfaces(); got != 3 {
		t.Errorf("the comparison names %d surface(s), want 3", got)
	}
	if got := groups[0].Rows[0].Key.Surface; got != "dynamic" {
		t.Errorf("the comparison begins with %q, want it ordered by surface", got)
	}
	if strings.Contains(groups[0].Caption, "surface") {
		t.Errorf("the caption %q names the surface, which a cross-surface comparison varies", groups[0].Caption)
	}
}

// TestGroupByVendor_OrdersTheTablesAndTheirRows keeps a re-render of one record
// byte for byte what the last one was, which is what lets a gate compare the
// pages at all.
func TestGroupByVendor_OrdersTheTablesAndTheirRows(t *testing.T) {
	first := oneRow()
	first.Key.Model = "zzz:model"
	second := oneRow()
	second.Key.Model = "aaa:model"
	third := oneRow()
	third.Key.Surface = "meta"

	groups := groupByVendor([]row{first, third, second})
	if len(groups) != 2 {
		t.Fatalf("got %d tables, want the two surfaces apart", len(groups))
	}
	if groups[0].Rows[0].Key.Model != "aaa:model" {
		t.Errorf("the first table begins with %q, want it ordered by model", groups[0].Rows[0].Key.Model)
	}
	if !strings.Contains(groups[0].Caption, "surface `dynamic`") {
		t.Errorf("the caption %q does not name what the table holds fixed", groups[0].Caption)
	}
}

// TestVendorKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist keeps a caption
// from claiming a pin, a schema mode or a slice a row never had.
func TestVendorKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist(t *testing.T) {
	plain := crossVendorKey(oneRow()).String()
	for _, absent := range []string{"pinned", "meta schema", "slice"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(plain, absent) {
				t.Errorf("the caption %q names %q, which this row has none of", plain, absent)
			}
		})
	}

	pinned := oneRow()
	pinned.Key.TierPin = "premium"
	pinned.Key.MetaParamSchema = "compact"
	pinned.Key.SliceSize = 128
	full := crossVendorKey(pinned).String()
	for _, present := range []string{"pinned to `premium`", "meta schema `compact`", "slice of 128 tools"} {
		t.Run(present, func(t *testing.T) {
			if !strings.Contains(full, present) {
				t.Errorf("the caption %q does not name %q", full, present)
			}
		})
	}
}

// TestSurfaceKey_String_NamesTheModelAndThePin is what a cross-surface note
// prints, and the only place a reader is told what two rows of different
// surfaces agree on.
func TestSurfaceKey_String_NamesTheModelAndThePin(t *testing.T) {
	pinned := oneRow()
	pinned.Key.TierPin = "premium"
	got := crossSurfaceKey(pinned).String()
	for _, present := range []string{"model `" + fixtureModel + "`", "pinned to `premium`", "repeat 1"} {
		t.Run(present, func(t *testing.T) {
			if !strings.Contains(got, present) {
				t.Errorf("the note %q does not name %q", got, present)
			}
		})
	}
}

// TestSurfaceLabel_SaysWhatARowDecidedForItself is the other half of the
// cross-surface rule: what the key could not hold fixed has to be printed
// beside the row, or an individual row measured on a slice reads as though it
// had been shown the catalog.
func TestSurfaceLabel_SaysWhatARowDecidedForItself(t *testing.T) {
	tests := []struct {
		name string
		edit func(*row)
		want string
	}{
		{name: "a dynamic row decides neither", edit: func(*row) {}},
		{
			name: "an individual row names its slice",
			edit: func(r *row) { r.Key.Surface, r.Key.SliceSize = "individual", 128 },
			want: "slice of 128 tools",
		},
		{
			name: "a meta row names its schema mode",
			edit: func(r *row) { r.Key.Surface, r.Key.MetaParamSchema = "meta", opaqueSchema },
			want: "meta schema `opaque`",
		},
		{
			name: "a run that pinned a schema mode has both on its individual row",
			edit: func(r *row) {
				r.Key.Surface, r.Key.SliceSize, r.Key.MetaParamSchema = "individual", 64, "compact"
			},
			want: "slice of 64 tools, meta schema `compact`",
		},
		// The budget alone is what a reader would otherwise compare two
		// individual rows under, and the slice is chosen per case, so the span
		// the attempts recorded goes beside it.
		{
			name: "an individual row says what its attempts were shown",
			edit: func(r *row) {
				r.Key.Surface, r.Key.SliceSize = "individual", 128
				r.Counts.Shown = &shown{Min: 96, Max: 312, Overflowed: 2}
			},
			want: "slice of 128 tools (shown 96 to 312, 2 over budget)",
		},
		{
			name: "a span with no budget beside it labels nothing",
			edit: func(r *row) { r.Counts.Shown = &shown{Min: 2, Max: 2} },
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			one := oneRow()
			testCase.edit(&one)
			if got := surfaceLabel(one); got != testCase.want {
				t.Errorf("surfaceLabel = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestRowKey_String_IsWhatACollisionIsRefusedBy holds the one property a key's
// spelling has to have: two rows that differ anywhere must spell differently,
// or a collision would hide one measurement behind another.
func TestRowKey_String_IsWhatACollisionIsRefusedBy(t *testing.T) {
	one := oneRow()
	other := oneRow()
	other.Key.SliceSize = 128
	if one.Key.String() == other.Key.String() {
		t.Fatalf("two different rows spell the same key: %q", one.Key.String())
	}
	if !strings.Contains(other.Key.String(), "slice=128") {
		t.Errorf("the key %q does not name the slice it was measured on", other.Key.String())
	}
}

// TestRowKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist keeps the name a
// collision is refused by from claiming a pin, a schema mode or a slice the row
// never had, and from dropping one it did.
func TestRowKey_String_NamesTheOptionalHalvesOnlyWhenTheyExist(t *testing.T) {
	plain := oneRow().Key.String()
	for _, absent := range []string{"tier-pin=", "meta-schema=", "slice="} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(plain, absent) {
				t.Errorf("the key %q names %q, which this row has none of", plain, absent)
			}
		})
	}

	full := oneRow()
	full.Key.TierPin = "premium"
	full.Key.MetaParamSchema = "compact"
	full.Key.SliceSize = 128
	spelled := full.Key.String()
	for _, present := range []string{"tier-pin=premium", "meta-schema=compact", "slice=128", "repeat=1"} {
		t.Run(present, func(t *testing.T) {
			if !strings.Contains(spelled, present) {
				t.Errorf("the key %q does not name %q", spelled, present)
			}
		})
	}
}

// withCases returns a copy of a row measured on the named cases, which is what
// a run narrowed with MODELEVAL_CASES leaves behind.
func withCases(one row, names ...string) row {
	one.Cases = make(map[string]caseFigures, len(names))
	for _, name := range names {
		one.Cases[name] = caseFigures{Run: "run-1", Date: "2026-09-17", Commit: fixtureCommit}
	}
	return one
}

// TestCrossVendorKey_RowsMeasuredOnDifferentCases_AreNotOneTable is the hole
// the case-carrying row closed.
//
// The corpus digest in the key says what the corpus *is*. It never said how
// much of it was asked, and those are different questions: a run narrowed with
// MODELEVAL_CASES produces a row whose key is identical to a full run's, so the
// two sat in one table under a caption asserting they agree on everything that
// matters. A model measured on two cases then read beside one measured on
// twelve as though the comparison were honest, with nothing but a denominator
// to tell them apart.
func TestCrossVendorKey_RowsMeasuredOnDifferentCases_AreNotOneTable(t *testing.T) {
	full := withCases(oneRow(), "MT-002", "MT-003", "MT-008")
	narrowed := withCases(oneRow(), "MT-002")
	narrowed.Key.Model = "openai:another-model"

	if crossVendorKey(full) == crossVendorKey(narrowed) {
		t.Error("a row measured on three cases and one measured on one share a cross-vendor table")
	}

	// The same cases, differently ordered, are the same measurement.
	same := withCases(oneRow(), "MT-008", "MT-002", "MT-003")
	same.Key.Model = "openai:another-model"
	if crossVendorKey(full) != crossVendorKey(same) {
		t.Error("two rows measured on the same three cases were split into separate tables")
	}

	// And the same count of different cases is still a different measurement.
	other := withCases(oneRow(), "MT-002", "MT-003", "MT-013")
	other.Key.Model = "openai:another-model"
	if crossVendorKey(full) == crossVendorKey(other) {
		t.Error("two rows measured on three cases each, but not the same three, share a table")
	}
}

// TestCaseCoverage_SaysHowMuchOfTheCorpusWasAsked checks the sentence a reader
// gets, including the silence for a row that records no cases: a caption is a
// list of what two rows agree on, and a coverage neither recorded is not an
// agreement.
func TestCaseCoverage_SaysHowMuchOfTheCorpusWasAsked(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		coverage caseCoverage
		want     string
	}{
		{name: "records none", coverage: caseCoverage{}, want: ""},
		{name: "part of it", coverage: caseCoverage{Digest: "abc", Cases: 12, Corpus: 258}, want: "12 of 258 cases (`abc`)"},
		{name: "all of it", coverage: caseCoverage{Digest: "abc", Cases: 258, Corpus: 258}, want: "the whole corpus (258 cases)"},
		// A row whose corpus size is zero has not measured the whole of
		// anything. There is deliberately no guard on Corpus for this: the
		// `case 0` arm has already settled that Cases is not zero, so a
		// corpus of zero is a corpus Cases cannot equal, and the equality
		// arm alone keeps a count of cases from reading as a corpus it
		// covered entirely when nothing recorded how large that corpus was.
		{name: "a corpus size nothing recorded", coverage: caseCoverage{Digest: "abc", Cases: 12}, want: "12 of 0 cases (`abc`)"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.coverage.String(); got != testCase.want {
				t.Errorf("String() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestCoverageOf_CountsTheRowsOwnCasesBesideTheCorpusItself pins the two
// numbers the caption above is built from.
//
// They are both ints of one struct read off two different things, so a reader
// that filled each from the other's source produces a caption that is still
// well formed and says the opposite: a row measured on two cases claims a
// corpus of two and reads as having covered all of it. Nothing downstream can
// tell, because two rows transposed the same way still key alike and still sit
// in one table.
func TestCoverageOf_CountsTheRowsOwnCasesBesideTheCorpusItself(t *testing.T) {
	corpus := len(modelcorpus.Keys())
	if corpus < 3 {
		t.Fatalf("the corpus holds %d case(s), too few for this test to tell the two numbers apart", corpus)
	}

	got := coverageOf(withCases(oneRow(), "MT-002", "MT-003"))
	if got.Cases != 2 {
		t.Errorf("the coverage counts %d case(s), want the two the row was measured on", got.Cases)
	}
	if got.Corpus != corpus {
		t.Errorf("the coverage names a corpus of %d, want the %d the corpus at HEAD holds", got.Corpus, corpus)
	}
	if got.Digest == "" {
		t.Error("a row measured on named cases fingerprints none of them")
	}
	if want := "2 of " + strconv.Itoa(corpus) + " cases (`" + got.Digest + "`)"; got.String() != want {
		t.Errorf("the caption reads %q, want %q", got.String(), want)
	}

	// A row that recorded no cases fingerprints nothing, which is the silence
	// the caption keeps rather than an agreement neither row made.
	none := coverageOf(oneRow())
	if none.Digest != "" || none.Cases != 0 || none.Corpus != corpus {
		t.Errorf("a row recording no cases covers %+v, want no digest and no cases beside a corpus of %d", none, corpus)
	}
}
