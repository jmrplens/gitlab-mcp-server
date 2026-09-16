// aggregate.go adds verdicts up into the seven columns a row publishes.
//
// Every column is a numerator and a denominator, both printed, and a dash when
// the denominator is empty. That is not a formatting preference. The tables
// this replaces printed percentages over denominators nobody could see, so a
// column reading 100% over one attempt and a column reading 100% over ninety
// looked the same, and a column with nothing behind it read as a perfect score.
// A rate is available to a caller that wants one, and it says whether there was
// anything to divide.

package modelscore

import "strconv"

// Ratio is one column: what happened over what could have.
type Ratio struct {
	Numerator   int
	Denominator int
}

// Known reports whether there was anything to divide.
func (r Ratio) Known() bool { return r.Denominator > 0 }

// Rate returns the quotient and whether there was one at all. A caller that
// ignores the second value and prints the first is printing zero for "nothing
// was measured", which is the mistake this signature exists to make loud.
func (r Ratio) Rate() (float64, bool) {
	if !r.Known() {
		return 0, false
	}
	return float64(r.Numerator) / float64(r.Denominator), true
}

// String renders the column as a reader sees it.
func (r Ratio) String() string {
	if !r.Known() {
		return emptyColumn
	}
	return strconv.Itoa(r.Numerator) + " / " + strconv.Itoa(r.Denominator)
}

// emptyColumn is what a column with an empty denominator prints.
const emptyColumn = "-"

// Overhead is the cost a model paid to get where it got, with its two halves
// apart.
//
// They are apart because they mean different things. A discovery call is the
// dynamic surface's declared cost: the catalog is not in the tool list, so a
// model has to search it, and that is the design rather than a failure. An
// invalid_params refusal is a model learning a parameter name from a rejection,
// which is what the default opaque meta schema leaves it to do, and it is the
// number that says what that mode costs. One rate over both would hide each
// inside the other.
type Overhead struct {
	// Discovery is how many catalog searches were made.
	Discovery int
	// InvalidParams is how many calls this server refused for their arguments.
	InvalidParams int
	// Steps is how many steps were reached, which is what the cost is per.
	Steps int
}

// Ratio folds the two halves into the published column.
func (o Overhead) Ratio() Ratio {
	return Ratio{Numerator: o.Discovery + o.InvalidParams, Denominator: o.Steps}
}

// Totals is one row: the seven columns, the counts behind them, and what was
// left out of them.
type Totals struct {
	// Attempts is how many attempts were scored into this row, the five
	// outcomes counted apart below excluded.
	Attempts int
	// Skipped is how many attempts never ran because the instance did not meet
	// the case's needs.
	Skipped int
	// Unobserved is how many attempts rest on a call whose dispatch was never
	// observed. They are counted here and nowhere else: a verdict about a call
	// the server's span never described is a claim about what was asked.
	Unobserved int
	// ProviderErrors is how many attempts the provider would not answer, and
	// HarnessErrors how many this side broke. Neither is the model's, which is
	// what the record says of them in so many words, so neither is in any
	// column.
	ProviderErrors int
	HarnessErrors  int
	// GitLabRefused is how many attempts were dispatched as the case declares,
	// with the arguments it declares, and refused by GitLab. What that reports
	// is the instance or the fixture, so it is counted here rather than folded
	// into a column a model is read by.
	GitLabRefused int
	// Outcomes is how many attempts ended each way, the five counts above
	// included, so a reader can see the shape of a row and not only its rates.
	Outcomes map[Outcome]int
	// Declines is how many steps were correctly declined each way, published
	// apart because declining in text is what a read-only deployment wants and
	// being refused the action is only the conversation ending correctly.
	Declines map[Decline]int
	// Confirmations is how many destructive steps carried their approval each
	// way.
	Confirmations map[Confirmation]int

	// Reached is steps reached or correctly declined over non-optional steps
	// declared.
	Reached Ratio
	// AcceptedFirstTime is steps whose first call about them was the one that
	// reached them, over steps reached.
	AcceptedFirstTime Ratio
	// ArgumentFidelity is arguments whose value matched their truth over
	// arguments declared comparable. An authored value is in neither.
	ArgumentFidelity Ratio
	// Confirmation is destructive steps whose reaching call carried the
	// confirmation over destructive steps declared.
	Confirmation Ratio
	// Unaided is attempts completed with no refusal of ours anywhere over
	// attempts run.
	Unaided Ratio
	// Completion is attempts completed over attempts run.
	Completion Ratio
	// Clean is attempts that went right end to end with no help, over attempts
	// run. It is the headline figure and is a conjunction of the others rather
	// than an average of them: see [Verdict.Clean] for why there is no weighted
	// score here.
	Clean Ratio
	// Overhead is what the model spent getting there.
	Overhead Overhead
}

// Aggregate adds a set of verdicts into one row.
//
// Five outcomes are counted and then left out of every column, each on its own
// ground. A skipped attempt never ran, so it is in no denominator, and a row
// that counted it would rank a model by the license of the instance it was
// measured on. An unobserved attempt ran and was not seen, so nothing about it
// can be said at all. The other three are left out because none of them is the
// model's: the provider would not answer, this side broke, or GitLab refused a
// call the model dispatched with the arguments the case declares. Counting any
// of those three would publish a provider's outage, a bug of ours or an
// instance's fixture as a model that did not complete the task, which is the
// fold section 4.2 refuses in so many words.
//
// Their steps go with them. A step of such an attempt says as little as the
// attempt does: the work stopped where the outage or the refusal fell, so the
// steps after it were never reached for a reason that is not the model's, and
// counting them would move the charge from the completion column into the
// reached one rather than dropping it.
func Aggregate(verdicts []Verdict) Totals {
	totals := Totals{
		Outcomes:      map[Outcome]int{},
		Declines:      map[Decline]int{},
		Confirmations: map[Confirmation]int{},
	}
	for _, verdict := range verdicts {
		totals.Outcomes[verdict.Outcome]++
		if totals.countApart(verdict.Outcome) {
			continue
		}
		totals.count(verdict)
	}
	return totals
}

// countApart records one attempt that is counted and then left out of every
// column, and reports whether this outcome was one of those.
//
// The five are kept in one place so that the reasons above are stated once and
// the columns below cannot come to disagree with them: a sixth outcome that
// ought to be apart is added here, and every denominator follows.
func (t *Totals) countApart(outcome Outcome) bool {
	switch outcome {
	case OutcomeSkipped:
		t.Skipped++
	case OutcomeUnobserved:
		t.Unobserved++
	case OutcomeProviderError:
		t.ProviderErrors++
	case OutcomeHarnessError:
		t.HarnessErrors++
	case OutcomeGitLabRefused:
		t.GitLabRefused++
	default:
		return false
	}
	return true
}

// count adds one attempt that ran, was seen, and is the model's to answer for.
func (t *Totals) count(verdict Verdict) {
	t.Attempts++
	t.Completion.Denominator++
	t.Unaided.Denominator++
	t.Clean.Denominator++
	if verdict.Clean() {
		t.Clean.Numerator++
	}
	if verdict.Outcome == OutcomeCompleted {
		t.Completion.Numerator++
	}
	if verdict.Unaided {
		t.Unaided.Numerator++
	}
	t.Overhead.Discovery += verdict.Discovery
	t.Overhead.InvalidParams += verdict.InvalidParams

	for _, step := range verdict.Steps {
		t.countStep(step)
	}
}

// countStep adds one step to the four step-grained columns.
//
// A step decided by a call whose span never arrived is left out of all four for
// the reason the attempt is: what it would contribute is a claim about a
// request rather than about a dispatch. An attempt like that is already out of
// this function, so what this guard covers is a step of an attempt whose
// outcome came from its ending rather than from its steps.
func (t *Totals) countStep(step StepVerdict) {
	if !step.Observed {
		return
	}
	if step.Decline != DeclineNone {
		t.Declines[step.Decline]++
	}
	if step.Destructive {
		t.Confirmation.Denominator++
		t.Confirmations[step.Confirmation]++
		if step.Confirmation == ConfirmationUnaided || step.Confirmation == ConfirmationServerAided {
			t.Confirmation.Numerator++
		}
	}
	reached := step.Reached || step.Decline != DeclineNone
	if !step.Optional {
		t.Reached.Denominator++
		if reached {
			t.Reached.Numerator++
		}
	}
	if !reached {
		return
	}
	t.Overhead.Steps++
	t.AcceptedFirstTime.Denominator++
	if step.AcceptedFirstTime {
		t.AcceptedFirstTime.Numerator++
	}
	for _, argument := range step.Arguments {
		if !argument.Compared {
			continue
		}
		t.ArgumentFidelity.Denominator++
		if argument.Matched {
			t.ArgumentFidelity.Numerator++
		}
	}
}

// Sum returns the two sets of totals as one.
//
// Every figure a row publishes is a count or a ratio over attempts, and an
// attempt belongs to exactly one case, so totals taken over disjoint sets of
// cases add. That is what lets a published row be assembled from more than one
// run: a case re-measured on its own replaces its own contribution and the
// rest of the row keeps theirs, instead of the whole row being replaced by the
// one case somebody re-ran.
//
// It is here rather than in the command that publishes rows because what is
// additive is a property of the scoring, not of the record: a figure that
// stopped being a sum of its cases would have to stop being added here, and a
// reader looking for that rule should find it beside the rule that computes it.
// TestSum_TouchesEveryField holds this to the struct by reflection, so a field
// added above and forgotten here fails rather than quietly reading low.
//
// It is a function rather than a method because it mutates neither argument,
// and Totals already carries pointer-receiver methods that do: mixing the two
// receivers on one type is how a caller comes to believe a copy was updated.
func Sum(t, other Totals) Totals {
	sum := Totals{
		Attempts:          t.Attempts + other.Attempts,
		Skipped:           t.Skipped + other.Skipped,
		Unobserved:        t.Unobserved + other.Unobserved,
		ProviderErrors:    t.ProviderErrors + other.ProviderErrors,
		HarnessErrors:     t.HarnessErrors + other.HarnessErrors,
		GitLabRefused:     t.GitLabRefused + other.GitLabRefused,
		Outcomes:          addTally(t.Outcomes, other.Outcomes),
		Declines:          addTally(t.Declines, other.Declines),
		Confirmations:     addTally(t.Confirmations, other.Confirmations),
		Reached:           t.Reached.add(other.Reached),
		AcceptedFirstTime: t.AcceptedFirstTime.add(other.AcceptedFirstTime),
		ArgumentFidelity:  t.ArgumentFidelity.add(other.ArgumentFidelity),
		Confirmation:      t.Confirmation.add(other.Confirmation),
		Unaided:           t.Unaided.add(other.Unaided),
		Completion:        t.Completion.add(other.Completion),
		Clean:             t.Clean.add(other.Clean),
		Overhead: Overhead{
			Discovery:     t.Overhead.Discovery + other.Overhead.Discovery,
			InvalidParams: t.Overhead.InvalidParams + other.Overhead.InvalidParams,
			Steps:         t.Overhead.Steps + other.Overhead.Steps,
		},
	}
	return sum
}

// add sums two ratios, which is the numerators and the denominators apart.
//
// A rate is never averaged with another rate here: two rows of 100% over one
// attempt and 50% over ninety are not 75% of anything, and adding the halves is
// the only reading that gives the same answer as scoring both sets at once.
func (r Ratio) add(other Ratio) Ratio {
	return Ratio{
		Numerator:   r.Numerator + other.Numerator,
		Denominator: r.Denominator + other.Denominator,
	}
}

// addTally sums two named tallies into a new map, leaving both arguments
// alone. An empty result stays nil, which is what [Aggregate]'s own readers
// expect of a tally nothing filled.
func addTally[K comparable](left, right map[K]int) map[K]int {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	sum := make(map[K]int, len(left)+len(right))
	for name, count := range left {
		sum[name] += count
	}
	for name, count := range right {
		sum[name] += count
	}
	return sum
}
