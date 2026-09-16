package modelscore

import "testing"

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
func TestAggregate_OnlyACompletedAttempt_CountsInTheCompletionColumn(t *testing.T) {
	totals := Aggregate([]Verdict{
		completedVerdict(),
		failedVerdict(),
		{Case: "MT-006", Outcome: OutcomeGitLabRefused},
	})

	if got := totals.Completion.String(); got != "1 / 3" {
		t.Errorf("Completion = %q, want one completed attempt of the three that ran", got)
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
