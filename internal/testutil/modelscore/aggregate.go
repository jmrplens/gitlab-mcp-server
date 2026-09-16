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
	// Attempts is how many attempts were scored into this row, skipped and
	// unobserved ones excluded.
	Attempts int
	// Skipped is how many attempts never ran because the instance did not meet
	// the case's needs.
	Skipped int
	// Unobserved is how many attempts rest on a call whose dispatch was never
	// observed. They are counted here and nowhere else: a verdict about a call
	// the server's span never described is a claim about what was asked.
	Unobserved int
	// Outcomes is how many attempts ended each way, the two counts above
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
	// Overhead is what the model spent getting there.
	Overhead Overhead
}

// Aggregate adds a set of verdicts into one row.
//
// A skipped attempt is counted and then left out of everything: it never ran,
// so it is in no denominator, and a row that counted it would rank a model by
// the license of the instance it was measured on. An unobserved attempt is left
// out on the other ground, that nothing about it can be said.
func Aggregate(verdicts []Verdict) Totals {
	totals := Totals{
		Outcomes:      map[Outcome]int{},
		Declines:      map[Decline]int{},
		Confirmations: map[Confirmation]int{},
	}
	for _, verdict := range verdicts {
		totals.Outcomes[verdict.Outcome]++
		switch verdict.Outcome {
		case OutcomeSkipped:
			totals.Skipped++
			continue
		case OutcomeUnobserved:
			totals.Unobserved++
			continue
		default:
		}
		totals.count(verdict)
	}
	return totals
}

// count adds one attempt that ran and was observed.
func (t *Totals) count(verdict Verdict) {
	t.Attempts++
	t.Completion.Denominator++
	t.Unaided.Denominator++
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
