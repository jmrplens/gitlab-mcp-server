package modelscore

import (
	"reflect"
	"testing"
)

// The seven columns, added up from verdicts written here rather than scored, so
// the arithmetic is held to numbers a reader of this file can check.

// TestRatio_PrintsBothNumbersAndADashForNothing is the fix for a published
// table where a column over one attempt and a column over ninety looked the
// same, and a column with nothing behind it read as a perfect score.
func TestRatio_PrintsBothNumbersAndADashForNothing(t *testing.T) {
	cases := []struct {
		name  string
		ratio Ratio
		text  string
		rate  float64
		known bool
	}{
		{name: "some of many", ratio: Ratio{Numerator: 3, Denominator: 4}, text: "3 / 4", rate: 0.75, known: true},
		{name: "none of many", ratio: Ratio{Denominator: 4}, text: "0 / 4", known: true},
		{name: "all of many", ratio: Ratio{Numerator: 4, Denominator: 4}, text: "4 / 4", rate: 1, known: true},
		{name: "nothing at all", ratio: Ratio{}, text: emptyColumn},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.ratio.String(); got != testCase.text {
				t.Errorf("String() = %q, want %q", got, testCase.text)
			}
			rate, known := testCase.ratio.Rate()
			if known != testCase.known {
				t.Fatalf("Rate reported known = %v, want %v", known, testCase.known)
			}
			if known && rate != testCase.rate {
				t.Errorf("Rate() = %v, want %v", rate, testCase.rate)
			}
			if testCase.ratio.Known() != testCase.known {
				t.Errorf("Known() = %v, want %v", testCase.ratio.Known(), testCase.known)
			}
		})
	}
}

// TestOverhead_FoldsItsTwoHalvesAndKeepsThemVisible: the two are added for the
// column and kept apart on the row, because a search is the dynamic surface's
// declared cost and a rejected argument is what an opaque schema costs.
func TestOverhead_FoldsItsTwoHalvesAndKeepsThemVisible(t *testing.T) {
	overhead := Overhead{Discovery: 2, InvalidParams: 1, Steps: 2}

	if got := overhead.Ratio().String(); got != "3 / 2" {
		t.Errorf("Ratio() = %q, want %q", got, "3 / 2")
	}
	if overhead.Discovery != 2 || overhead.InvalidParams != 1 {
		t.Error("the two halves must stay readable on the row")
	}
}

// completedVerdict is an attempt that did everything the case asked, with one
// argument missed so the fidelity column has something to divide.
func completedVerdict() Verdict {
	return Verdict{
		Case:          "MT-001",
		Outcome:       OutcomeCompleted,
		Unaided:       true,
		Verified:      true,
		Discovery:     2,
		InvalidParams: 1,
		Steps: []StepVerdict{
			{
				Position:          1,
				Name:              "project.get",
				Reached:           true,
				Observed:          true,
				Complete:          true,
				AcceptedFirstTime: true,
				Arguments: []ArgumentVerdict{
					{Name: "project_id", Compared: true, Matched: true},
					{Name: "statistics", Compared: true, Matched: true},
					{Name: "title", Truth: TruthAuthored},
				},
			},
			{
				Position:     2,
				Name:         "issue.delete",
				Reached:      true,
				Observed:     true,
				Complete:     true,
				Destructive:  true,
				Mutating:     true,
				Confirmation: ConfirmationUnaided,
				Retries:      1,
				Arguments: []ArgumentVerdict{
					{Name: "project_id", Compared: true, Matched: true},
					{Name: "issue_iid", Compared: true},
				},
			},
		},
	}
}

// failedVerdict is an attempt that reached nothing, which is what gives the
// reached and completion columns a denominator larger than their numerator.
func failedVerdict() Verdict {
	return Verdict{
		Case:    "MT-002",
		Outcome: OutcomeFailed,
		Steps: []StepVerdict{{
			Position:     1,
			Name:         "issue.delete",
			Observed:     true,
			Destructive:  true,
			Mutating:     true,
			Confirmation: ConfirmationNever,
			Reason:       "no call named this step",
		}},
	}
}

// TestAggregate_TheSevenColumns_EachOverItsOwnDenominator is the arithmetic,
// with every column's two numbers deliberately different.
func TestAggregate_TheSevenColumns_EachOverItsOwnDenominator(t *testing.T) {
	totals := Aggregate([]Verdict{
		completedVerdict(),
		failedVerdict(),
		{Case: "MT-003", Outcome: OutcomeSkipped},
		{Case: "MT-005", Outcome: OutcomeUnobserved, Steps: []StepVerdict{{Position: 1, Reached: true}}},
	})

	cases := []struct {
		name string
		got  Ratio
		want string
		why  string
	}{
		{
			name: "reached",
			got:  totals.Reached,
			want: "2 / 3",
			why:  "two of the three non-optional steps of the two attempts that ran were reached",
		},
		{
			name: "accepted first time",
			got:  totals.AcceptedFirstTime,
			want: "1 / 2",
			why:  "of the two steps reached, one was answered on the first call about it",
		},
		{
			name: "argument fidelity",
			got:  totals.ArgumentFidelity,
			want: "3 / 4",
			why:  "four arguments were comparable, the authored one is in neither number",
		},
		{
			name: "confirmation",
			got:  totals.Confirmation,
			want: "1 / 2",
			why:  "two destructive steps were declared and one carried its approval",
		},
		{
			name: "unaided completion",
			got:  totals.Unaided,
			want: "1 / 2",
			why:  "one of the two attempts that ran completed with no refusal of ours",
		},
		{name: "completion", got: totals.Completion, want: "1 / 2", why: "one of the two attempts completed"},
		{
			name: "overhead",
			got:  totals.Overhead.Ratio(),
			want: "3 / 2",
			why:  "two searches and one rejected argument over the two steps reached",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.got.String(); got != testCase.want {
				t.Errorf("%s = %q, want %q: %s", testCase.name, got, testCase.want, testCase.why)
			}
		})
	}
}

// TestAggregate_SkippedAndUnobservedAttempts_AreCountedApart says what is left
// out of every column above and why each is left out.
//
// A skipped attempt never ran, so counting it would rank a model by the license
// of the instance it was measured on. An unobserved one ran and was not seen, so
// every number it could contribute would be a claim about a request.
func TestAggregate_SkippedAndUnobservedAttempts_AreCountedApart(t *testing.T) {
	totals := Aggregate([]Verdict{
		completedVerdict(),
		failedVerdict(),
		{Case: "MT-003", Outcome: OutcomeSkipped},
		{Case: "MT-005", Outcome: OutcomeUnobserved, Steps: []StepVerdict{{Position: 1, Reached: true}}},
	})

	if totals.Attempts != 2 {
		t.Errorf("Attempts = %d, want the two that ran and were seen", totals.Attempts)
	}
	if totals.Skipped != 1 || totals.Unobserved != 1 {
		t.Errorf("Skipped = %d and Unobserved = %d, want one each", totals.Skipped, totals.Unobserved)
	}
	if totals.Completion.Denominator != 2 {
		t.Errorf("the completion column is over %d attempts, want 2", totals.Completion.Denominator)
	}
	if totals.Outcomes[OutcomeSkipped] != 1 || totals.Outcomes[OutcomeUnobserved] != 1 {
		t.Error("the outcome counts must still show what was left out")
	}
	if totals.Outcomes[OutcomeCompleted] != 1 || totals.Outcomes[OutcomeFailed] != 1 {
		t.Error("the outcome counts must show the shape of the row and not only its rates")
	}
}

// TestAggregate_AStepDecidedByAnUnobservedCall_IsInNoColumn covers the same
// rule one level down, for an attempt whose outcome came from its ending rather
// than from its steps.
func TestAggregate_AStepDecidedByAnUnobservedCall_IsInNoColumn(t *testing.T) {
	totals := Aggregate([]Verdict{{
		Case:    "MT-002",
		Outcome: OutcomeOverBudget,
		Steps: []StepVerdict{
			{Position: 1, Reached: true, Observed: true, Complete: true, AcceptedFirstTime: true},
			{Position: 2, Reached: true, Observed: false, Complete: true},
		},
	}})

	if got := totals.Reached.String(); got != "1 / 1" {
		t.Errorf("Reached = %q, want only the step that was seen", got)
	}
}

// TestAggregate_AnOptionalStepAndAServerAidedConfirmation_AreCountedAsDeclared
// covers the two shapes the columns treat specially.
//
// An optional step is in no denominator, because nothing was owed; a
// server-aided confirmation is in the confirmation numerator, because the model
// did approve the action, and the class beside it says the server had to ask.
func TestAggregate_AnOptionalStepAndAServerAidedConfirmation_AreCountedAsDeclared(t *testing.T) {
	totals := Aggregate([]Verdict{{
		Outcome: OutcomeCompleted,
		Steps: []StepVerdict{
			{
				Position: 1, Name: "issue.delete", Reached: true, Observed: true, Complete: true,
				Destructive: true, Mutating: true, Confirmation: ConfirmationServerAided, Retries: 1,
			},
			{Position: 2, Name: "issue.list", Optional: true, Observed: true, Complete: true},
		},
	}})

	if got := totals.Reached.String(); got != "1 / 1" {
		t.Errorf("Reached = %q, want the optional step in neither number", got)
	}
	if got := totals.Confirmation.String(); got != "1 / 1" {
		t.Errorf("Confirmation = %q, want the approval counted however it was learned", got)
	}
	if totals.Confirmations[ConfirmationServerAided] != 1 {
		t.Errorf("confirmations = %v, want the class recorded beside the column", totals.Confirmations)
	}
}

// TestAggregate_OnlyACompletedAttempt_CountsInTheCompletionColumn is the column
// a published table is read by, so what goes into its numerator is worth one
// test of its own rather than being implied by a fixture.
//
// The third attempt is over budget, which is the model's own: it had the turns
// and did not finish inside them, so it is in the denominator and not in the
// numerator. The endings that are not the model's are the test below.
func TestAggregate_OnlyACompletedAttempt_CountsInTheCompletionColumn(t *testing.T) {
	totals := Aggregate([]Verdict{
		completedVerdict(),
		failedVerdict(),
		{Case: "MT-006", Outcome: OutcomeOverBudget},
	})

	if got := totals.Completion.String(); got != "1 / 3" {
		t.Errorf("Completion = %q, want one completed attempt of the three that ran", got)
	}
}

// apartVerdict is an attempt that ended in a way that is not the model's, with
// a step of its own so that the steps-go-with-it half of the rule is measured
// rather than implied.
func apartVerdict(outcome Outcome) Verdict {
	return Verdict{
		Case:    "MT-007",
		Outcome: outcome,
		Steps: []StepVerdict{{
			Position:          1,
			Name:              "project.get",
			Reached:           true,
			Observed:          true,
			Complete:          true,
			AcceptedFirstTime: true,
			Arguments:         []ArgumentVerdict{{Name: "project_id", Compared: true, Matched: true}},
		}},
	}
}

// TestAggregate_AnEndingThatIsNotTheModels_IsInNoDenominator is the rule that
// keeps a provider's outage, a bug of ours and an instance's fixture out of the
// columns a model is ranked by.
//
// The record says of the first two that they "are not the model's at all", and
// section 4.2 says of the third that a GitLab refusal after a correct dispatch
// is "never folded into a model failure". A completion column counting them in
// its denominator folds all three in: the attempt is counted as run and not
// completed, which is what a reader takes for a model that did not do the task.
// Each is counted beside the columns instead, and its steps go with it.
func TestAggregate_AnEndingThatIsNotTheModels_IsInNoDenominator(t *testing.T) {
	cases := []struct {
		name    string
		outcome Outcome
		counter func(Totals) int
	}{
		{
			name:    "the provider would not answer",
			outcome: OutcomeProviderError,
			counter: func(totals Totals) int { return totals.ProviderErrors },
		},
		{
			name:    "this side broke",
			outcome: OutcomeHarnessError,
			counter: func(totals Totals) int { return totals.HarnessErrors },
		},
		{
			name:    "GitLab refused a call the case declares",
			outcome: OutcomeGitLabRefused,
			counter: func(totals Totals) int { return totals.GitLabRefused },
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			totals := Aggregate([]Verdict{
				completedVerdict(),
				failedVerdict(),
				apartVerdict(testCase.outcome),
			})

			if totals.Attempts != 2 {
				t.Errorf("Attempts = %d, want the two the model is answerable for", totals.Attempts)
			}
			if got := totals.Completion.String(); got != "1 / 2" {
				t.Errorf("Completion = %q, want the third attempt in neither number", got)
			}
			if got := totals.Unaided.String(); got != "1 / 2" {
				t.Errorf("Unaided = %q, want the third attempt in neither number", got)
			}
			if got := totals.Reached.String(); got != "2 / 3" {
				t.Errorf("Reached = %q, want the third attempt's step left out with it", got)
			}
			if got := testCase.counter(totals); got != 1 {
				t.Errorf("the count beside the columns = %d, want the one attempt", got)
			}
			if totals.Outcomes[testCase.outcome] != 1 {
				t.Errorf("outcomes = %v, want the ending still visible on the row", totals.Outcomes)
			}
		})
	}
}

// TestAggregate_ADestructiveStepNeverConfirmed_IsInTheDenominatorOnly says what
// the confirmation column measures: a destructive step the model reached
// without approving is the case the column exists to show.
func TestAggregate_ADestructiveStepNeverConfirmed_IsInTheDenominatorOnly(t *testing.T) {
	totals := Aggregate([]Verdict{{
		Outcome: OutcomeFailed,
		Steps: []StepVerdict{{
			Position: 1, Name: "issue.delete", Reached: true, Observed: true,
			Destructive: true, Mutating: true, Confirmation: ConfirmationNever,
		}},
	}})

	if got := totals.Confirmation.String(); got != "0 / 1" {
		t.Errorf("Confirmation = %q, want the step declared and no approval counted", got)
	}
}

// TestAggregate_NothingAtAll_RendersEveryColumnAsADash is the state a row is in
// before anything has run, and the one the old tables printed as zero.
func TestAggregate_NothingAtAll_RendersEveryColumnAsADash(t *testing.T) {
	totals := Aggregate(nil)

	columns := map[string]Ratio{
		"reached":             totals.Reached,
		"accepted first time": totals.AcceptedFirstTime,
		"argument fidelity":   totals.ArgumentFidelity,
		"confirmation":        totals.Confirmation,
		"unaided":             totals.Unaided,
		"completion":          totals.Completion,
		"overhead":            totals.Overhead.Ratio(),
	}
	for name, column := range columns {
		t.Run(name, func(t *testing.T) {
			if got := column.String(); got != emptyColumn {
				t.Errorf("%s = %q, want %q", name, got, emptyColumn)
			}
		})
	}
}

// TestAggregate_TheTwoWaysOfDeclining_AreCountedApart keeps the sub-counts the
// read-only row publishes beside its completion figure: declining in text is
// what a deployment wants, and being refused the action is the conversation
// ending correctly.
func TestAggregate_TheTwoWaysOfDeclining_AreCountedApart(t *testing.T) {
	totals := Aggregate([]Verdict{{
		Outcome: OutcomeCompleted,
		Steps: []StepVerdict{
			{Position: 1, Observed: true, Complete: true, Mutating: true, Decline: DeclineByText},
			{
				Position: 2, Observed: true, Complete: true, Mutating: true, Reached: true,
				Decline: DeclineByRefusal,
			},
		},
	}})

	if totals.Declines[DeclineByText] != 1 || totals.Declines[DeclineByRefusal] != 1 {
		t.Errorf("declines = %v, want one of each", totals.Declines)
	}
	if got := totals.Reached.String(); got != "2 / 2" {
		t.Errorf("Reached = %q, want a correctly declined step counted as reached", got)
	}
}

// filledTotals returns a Totals whose every field carries a distinct non-zero
// value, so doubling it tells a field [Sum] adds from one it forgot.
func filledTotals() Totals {
	return Totals{
		Attempts:          11,
		Skipped:           12,
		Unobserved:        13,
		ProviderErrors:    14,
		HarnessErrors:     15,
		GitLabRefused:     16,
		Outcomes:          map[Outcome]int{OutcomeCompleted: 17},
		Declines:          map[Decline]int{DeclineByText: 18},
		Confirmations:     map[Confirmation]int{ConfirmationUnaided: 19},
		Reached:           Ratio{Numerator: 20, Denominator: 21},
		AcceptedFirstTime: Ratio{Numerator: 22, Denominator: 23},
		ArgumentFidelity:  Ratio{Numerator: 24, Denominator: 25},
		Confirmation:      Ratio{Numerator: 26, Denominator: 27},
		Unaided:           Ratio{Numerator: 28, Denominator: 29},
		Completion:        Ratio{Numerator: 30, Denominator: 31},
		Clean:             Ratio{Numerator: 35, Denominator: 36},
		Overhead:          Overhead{Discovery: 32, InvalidParams: 33, Steps: 34},
	}
}

// TestSum_TouchesEveryField holds the sum to the struct.
//
// A published row can be assembled from more than one run, and the arithmetic
// that does it is [Sum]. A field added to Totals and forgotten there
// would not fail anything: the row would simply publish a figure that counted
// one run and not the other, which reads as a real measurement and is the
// class of defect this whole record exists to stop. So every field is walked
// by reflection rather than by a list somebody has to remember to extend, and
// a kind this walk cannot check fails rather than passing unseen.
func TestSum_TouchesEveryField(t *testing.T) {
	filled := filledTotals()
	doubled := Sum(filled, filled)

	want := reflect.ValueOf(filled)
	got := reflect.ValueOf(doubled)
	for i := range want.NumField() {
		name := want.Type().Field(i).Name
		t.Run(name, func(t *testing.T) {
			assertDoubled(t, name, want.Field(i), got.Field(i))
		})
	}
}

// assertDoubled checks one field, recursing into a nested struct and into the
// values of a tally, and failing on a kind it does not know how to compare.
func assertDoubled(t *testing.T, name string, want, got reflect.Value) {
	t.Helper()
	switch want.Kind() {
	case reflect.Int:
		if want.Int() == 0 {
			t.Fatalf("%s is zero in the fixture, so doubling it proves nothing; give it a value", name)
		}
		if got.Int() != want.Int()*2 {
			t.Errorf("%s = %d, want %d: Sum does not add this field", name, got.Int(), want.Int()*2)
		}
	case reflect.Struct:
		for i := range want.NumField() {
			assertDoubled(t, name+"."+want.Type().Field(i).Name, want.Field(i), got.Field(i))
		}
	case reflect.Map:
		if want.Len() == 0 {
			t.Fatalf("%s is empty in the fixture, so doubling it proves nothing", name)
		}
		for _, key := range want.MapKeys() {
			left, right := want.MapIndex(key), got.MapIndex(key)
			if !right.IsValid() {
				t.Errorf("%s[%v] is absent after Sum", name, key)
				continue
			}
			if right.Int() != left.Int()*2 {
				t.Errorf("%s[%v] = %d, want %d", name, key, right.Int(), left.Int()*2)
			}
		}
	default:
		t.Fatalf("%s is a %s, which this walk cannot check: teach it, or Sum may be silently dropping the field", name, want.Kind())
	}
}
