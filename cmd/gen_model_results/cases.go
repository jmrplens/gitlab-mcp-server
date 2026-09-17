// What each case contributed to a row, and the arithmetic that lets a row be
// assembled from more than one run.
//
// A row is an aggregate, and until this existed the record did not say what it
// was an aggregate of. That left a fold with two moves and no third: refuse a
// key it already holds, or replace the row wholesale. Neither is what a
// maintainer wants after correcting one case and re-running it: the first
// refuses the update, and the second took a row measured over 258 cases and
// replaced it with one measured over the one case that was re-run, reported
// success, and discarded the rest of a paid run.
//
// So the row carries its cases. A fold that names cases a row already holds
// replaces those entries and leaves the others exactly as they stood, and every
// published figure is re-derived from the merged set. Each entry also says
// which run measured it, because the moment a row can be assembled from two
// runs its provenance stops being one run's and saying otherwise would be the
// same kind of lie this record was rebuilt to stop.

package main

import (
	"maps"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore"
)

// caseFigures is one case's contribution to a row.
//
// It is stored in the record's own vocabulary rather than the scorer's, so the
// committed file stays readable and the scorer's internal type does not become
// a file format. The round trip back to [modelscore.Totals] is exact, which is
// what [totalsOf] exists to keep true: counts and columns between them carry
// every field of Totals, and Turns, Shown and Tokens are the record's own
// additions on top.
type caseFigures struct {
	// Run, Date and Commit name the run that measured this case. They are per
	// case and not per row because a merged row has no single run behind it,
	// and a reader asking "when was this case last measured" is asking exactly
	// this.
	Run    string `json:"run"`
	Date   string `json:"date"`
	Commit string `json:"commit"`
	// Counts, Columns and Tokens are this case's share of the three blocks the
	// row publishes. Every field of all three is additive across cases, which
	// is the property the merge rests on.
	Counts  counts  `json:"counts"`
	Columns columns `json:"columns"`
	Tokens  tokens  `json:"tokens"`
}

// totalsOf reads one case's contribution back as the scorer's own totals, so
// the sum is done by [modelscore.Sum] rather than by a second copy of the rule
// about what is additive.
//
// Turns, Shown and Tokens have no home in Totals and are summed beside it.
func totalsOf(one caseFigures) modelscore.Totals {
	return modelscore.Totals{
		Attempts:          one.Counts.Attempts,
		Skipped:           one.Counts.Skipped,
		Unobserved:        one.Counts.Unobserved,
		ProviderErrors:    one.Counts.ProviderErrors,
		HarnessErrors:     one.Counts.HarnessErrors,
		GitLabRefused:     one.Counts.GitLabRefused,
		Outcomes:          tallyOf[modelscore.Outcome](one.Counts.Outcomes),
		Declines:          tallyOf[modelscore.Decline](one.Counts.Declines),
		Confirmations:     tallyOf[modelscore.Confirmation](one.Counts.Confirmations),
		Reached:           scoreRatio(one.Columns.Reached),
		AcceptedFirstTime: scoreRatio(one.Columns.AcceptedFirstTime),
		ArgumentFidelity:  scoreRatio(one.Columns.ArgumentFidelity),
		Confirmation:      scoreRatio(one.Columns.Confirmation),
		Unaided:           scoreRatio(one.Columns.Unaided),
		Completion:        scoreRatio(one.Columns.Completion),
		Clean:             scoreRatio(one.Columns.Clean),
		Overhead: modelscore.Overhead{
			Discovery:     one.Columns.Overhead.Discovery,
			InvalidParams: one.Columns.Overhead.InvalidParams,
			Steps:         one.Columns.Overhead.Steps,
		},
	}
}

// tallyOf reads a record tally back under the scorer's own key type.
func tallyOf[K ~string](named map[string]int) map[K]int {
	if len(named) == 0 {
		return nil
	}
	tally := make(map[K]int, len(named))
	for name, count := range named {
		tally[K(name)] = count
	}
	return tally
}

// scoreRatio reads a published ratio back as the scorer's.
func scoreRatio(r ratio) modelscore.Ratio {
	return modelscore.Ratio{Numerator: r.Numerator, Denominator: r.Denominator}
}

// mergeCases returns the cases a row holds after a fold, with the incoming
// run's entries replacing the ones they name and every other entry untouched.
//
// Neither argument is modified: a merge that wrote into the committed row's map
// would leave the record edited even on a path that then refuses the fold.
func mergeCases(held, incoming map[string]caseFigures) map[string]caseFigures {
	merged := make(map[string]caseFigures, len(held)+len(incoming))
	maps.Copy(merged, held)
	maps.Copy(merged, incoming)
	return merged
}

// replacedCases names the cases an incoming fold takes over from a row it
// merges into, in order, so the fold can say what it replaced rather than
// leaving a maintainer to diff the record.
func replacedCases(held, incoming map[string]caseFigures) []string {
	var replaced []string
	for name := range incoming {
		if _, stood := held[name]; stood {
			replaced = append(replaced, name)
		}
	}
	sort.Strings(replaced)
	return replaced
}

// figuresFrom assembles one case's contribution from what its own attempts
// scored.
func figuresFrom(cand candidate, totals modelscore.Totals, attempts []modelscore.Attempt) caseFigures {
	return caseFigures{
		Run:     cand.run.RunID,
		Date:    runDate(cand.run),
		Commit:  cand.run.Commit,
		Counts:  countsOf(totals, attempts),
		Columns: columnsOf(totals),
		Tokens:  tokensOf(attempts),
	}
}

// sumCases re-derives the three published blocks from every case behind a row.
//
// The columns come from one [modelscore.Sum] over the cases rather than from
// averaging their rates: two cases of 100% over one attempt and 50% over ninety
// are not 75% of anything, and summing the halves is the only reading that
// gives the figure scoring both at once would give. Turns, the shown span and
// the four token figures are summed here because they are the record's own and
// Totals has no room for them.
func sumCases(cases map[string]caseFigures) (counts, columns, tokens) {
	var (
		totals modelscore.Totals
		turns  int
		spent  tokens
		span   *shown
	)
	for _, name := range sortedCaseNames(cases) {
		one := cases[name]
		totals = modelscore.Sum(totals, totalsOf(one))
		turns += one.Counts.Turns
		spent.Input += one.Tokens.Input
		spent.Output += one.Tokens.Output
		spent.CacheCreated += one.Tokens.CacheCreated
		spent.CacheRead += one.Tokens.CacheRead
		span = mergeShown(span, one.Counts.Shown)
	}

	rolled := counts{
		Attempts:       totals.Attempts,
		Skipped:        totals.Skipped,
		Unobserved:     totals.Unobserved,
		ProviderErrors: totals.ProviderErrors,
		HarnessErrors:  totals.HarnessErrors,
		GitLabRefused:  totals.GitLabRefused,
		Turns:          turns,
		Outcomes:       namedCounts(totals.Outcomes),
		Declines:       namedCounts(totals.Declines),
		Confirmations:  namedCounts(totals.Confirmations),
		Shown:          span,
	}
	return rolled, columnsOf(totals), spent
}

// sortedCaseNames orders the cases so a merged row is byte-identical however
// the maps were built, which is what keeps a redraw from churning the record.
func sortedCaseNames(cases map[string]caseFigures) []string {
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// mergeShown widens a shown span to cover another one.
//
// A nil span is a run that recorded nothing about what it was shown, which is
// not the same as a run shown nothing, so it contributes no floor and no
// ceiling rather than a zero.
func mergeShown(span, other *shown) *shown {
	if other == nil {
		return span
	}
	if span == nil {
		widened := *other
		return &widened
	}
	widened := *span
	widened.Min = min(widened.Min, other.Min)
	widened.Max = max(widened.Max, other.Max)
	widened.Overflowed += other.Overflowed
	return &widened
}
