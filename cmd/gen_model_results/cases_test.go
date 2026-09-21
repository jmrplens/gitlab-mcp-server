package main

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore"
)

// distinctTotals is one case's scoring with a different number in every field,
// so a reader that filled one figure from another's source is a failure rather
// than a coincidence.
//
// Every fixture that scores a real shard has one attempt and one step, which
// makes every count a 1 and every column a 1 over 1: under those figures any
// two of them could change places and nothing would notice. That is the defect
// the withdrawn tables shipped, and it is why the numbers here are all
// different from one another.
func distinctTotals() modelscore.Totals {
	return modelscore.Totals{
		Attempts:       9,
		Skipped:        2,
		Unobserved:     3,
		ProviderErrors: 4,
		HarnessErrors:  5,
		GitLabRefused:  6,
		Outcomes: map[modelscore.Outcome]int{
			modelscore.OutcomeCompleted: 7,
			modelscore.OutcomeFailed:    8,
		},
		Declines:          map[modelscore.Decline]int{modelscore.DeclineByText: 10},
		Confirmations:     map[modelscore.Confirmation]int{modelscore.ConfirmationUnaided: 11},
		Clean:             modelscore.Ratio{Numerator: 20, Denominator: 30},
		Reached:           modelscore.Ratio{Numerator: 21, Denominator: 31},
		AcceptedFirstTime: modelscore.Ratio{Numerator: 22, Denominator: 32},
		ArgumentFidelity:  modelscore.Ratio{Numerator: 23, Denominator: 33},
		Confirmation:      modelscore.Ratio{Numerator: 24, Denominator: 34},
		Unaided:           modelscore.Ratio{Numerator: 25, Denominator: 35},
		Completion:        modelscore.Ratio{Numerator: 26, Denominator: 36},
		Overhead:          modelscore.Overhead{Discovery: 27, InvalidParams: 28, Steps: 37},
	}
}

// distinctCase is what a row stores for one case scored as [distinctTotals],
// with the three figures the scorer has no room for given values of their own.
func distinctCase() caseFigures {
	one := caseFigures{
		Counts:  countsOf(distinctTotals(), nil),
		Columns: columnsOf(distinctTotals()),
		Tokens:  tokens{Input: 1200, Output: 41, CacheCreated: 900, CacheRead: 1100},
	}
	one.Counts.Turns = 13
	one.Counts.Shown = &shown{Min: 96, Max: 312, Overflowed: 2}
	return one
}

// TestTotalsOf_ReadsBackEveryFigureTheRecordPublished is the claim the comment
// on [caseFigures] makes and nothing asserted: the round trip into the scorer's
// own totals is exact.
//
// It is the direction a merge travels, and the only direction no test took. The
// forward half is held whole by TestColumnsOf_AndCountsOf_PutEveryFigureUnder\
// TheNameItWasComputedFor; here a case read back out of the record with two of
// its counts exchanged -- skipped for unobserved, say, or a provider error for
// a harness error -- would be a straight-line assignment with no branch to
// flip, so neither gate can see it, and it surfaces only once a second run is
// merged into the row.
func TestTotalsOf_ReadsBackEveryFigureTheRecordPublished(t *testing.T) {
	want := distinctTotals()
	if got := totalsOf(distinctCase()); !reflect.DeepEqual(got, want) {
		t.Errorf("totals read back as\n got %+v\nwant %+v", got, want)
	}
}

// TestSumCases_OneCase_PublishesExactlyWhatThatCaseCarried is the other end of
// the same round trip, over the function a merged row's published blocks are
// re-derived by.
//
// A row assembled from one case must publish that case's own figures. Anything
// else means the sum is not the identity on a single term, and every merged row
// is then wrong by whatever the difference is -- silently, because the figures
// stay plausible.
func TestSumCases_OneCase_PublishesExactlyWhatThatCaseCarried(t *testing.T) {
	one := distinctCase()
	gotCounts, gotColumns, gotTokens := sumCases(map[string]caseFigures{fixtureCase: one})

	if !reflect.DeepEqual(gotCounts, one.Counts) {
		t.Errorf("counts\n got %+v\nwant %+v", gotCounts, one.Counts)
	}
	if gotColumns != one.Columns {
		t.Errorf("columns\n got %+v\nwant %+v", gotColumns, one.Columns)
	}
	if gotTokens != one.Tokens {
		t.Errorf("tokens\n got %+v\nwant %+v", gotTokens, one.Tokens)
	}
}

// TestSumCases_TwoCases_AddsEveryFigureAndWidensTheSpan holds the arithmetic a
// merged row rests on.
//
// The two cases here differ in every number, so a sum that reached for the
// wrong field lands somewhere no addition of these two could. The columns
// are summed rather than averaged on purpose: two cases of 100% over one
// attempt and 50% over ninety are not 75% of anything, and adding the halves is
// the only reading that gives the figure scoring both at once would give.
func TestSumCases_TwoCases_AddsEveryFigureAndWidensTheSpan(t *testing.T) {
	first := distinctCase()
	second := distinctCase()
	second.Counts.Attempts = 1
	second.Counts.Skipped = 0
	second.Counts.Turns = 4
	second.Tokens = tokens{Input: 300, Output: 7, CacheCreated: 0, CacheRead: 50}
	second.Counts.Shown = &shown{Min: 64, Max: 128, Overflowed: 1}
	second.Columns.Reached = ratio{Numerator: 1, Denominator: 2}

	gotCounts, gotColumns, gotTokens := sumCases(map[string]caseFigures{fixtureCase: first, "MT-003": second})

	if gotCounts.Attempts != 10 || gotCounts.Skipped != 2 || gotCounts.Turns != 17 {
		t.Errorf("counts = %d attempt(s), %d skipped over %d turn(s), want 10, 2 and 17",
			gotCounts.Attempts, gotCounts.Skipped, gotCounts.Turns)
	}
	wantShown := shown{Min: 64, Max: 312, Overflowed: 3}
	if gotCounts.Shown == nil || *gotCounts.Shown != wantShown {
		t.Errorf("shown = %+v, want %+v", gotCounts.Shown, wantShown)
	}
	wantReached := ratio{Numerator: 22, Denominator: 33}
	if gotColumns.Reached != wantReached {
		t.Errorf("reached = %+v, want %+v: the halves are added rather than the rates averaged", gotColumns.Reached, wantReached)
	}
	wantTokens := tokens{Input: 1500, Output: 48, CacheCreated: 900, CacheRead: 1150}
	if gotTokens != wantTokens {
		t.Errorf("tokens\n got %+v\nwant %+v", gotTokens, wantTokens)
	}
}

// TestReplacedCases_NamesOnlyWhatTheRowAlreadyHeld is what a fold reports it
// took over, and the reason a re-run of one case is a merge rather than a
// replacement.
//
// A case the standing row never measured is added and replaces nothing, and
// saying otherwise would tell a maintainer that figures were overwritten when
// none were. The order is the record's own, because a maintainer comparing two
// runs of the same fold should not have to read a list that moves.
func TestReplacedCases_NamesOnlyWhatTheRowAlreadyHeld(t *testing.T) {
	held := map[string]caseFigures{"MT-002": {Run: "run-one"}, "MT-008": {Run: "run-one"}}
	incoming := map[string]caseFigures{"MT-008": {Run: "run-two"}, "MT-002": {Run: "run-two"}, "MT-013": {Run: "run-two"}}

	want := []string{"MT-002", "MT-008"}
	if got := replacedCases(held, incoming); !reflect.DeepEqual(got, want) {
		t.Errorf("replaced %v, want %v: MT-013 is new to the row and takes nothing over", got, want)
	}
	if got := replacedCases(held, map[string]caseFigures{"MT-013": {Run: "run-two"}}); len(got) != 0 {
		t.Errorf("replaced %v, want nothing: no incoming case was already held", got)
	}

	// The union is what the row goes on to publish, and neither argument is
	// touched: a merge that wrote into the committed row's map would leave the
	// record edited even on a path that then refuses the fold.
	merged := mergeCases(held, incoming)
	if len(merged) != 3 || merged["MT-002"].Run != "run-two" || merged["MT-013"].Run != "run-two" {
		t.Errorf("merged into %+v, want the three cases with the incoming run's figures where it measured", merged)
	}
	if len(held) != 2 || held["MT-002"].Run != "run-one" {
		t.Errorf("the standing row's cases were edited in place: %+v", held)
	}
}
