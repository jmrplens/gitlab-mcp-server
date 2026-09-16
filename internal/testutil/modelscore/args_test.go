package modelscore

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// The four truths, put to values a model might really send.

// noProduced is the resolver for a comparison that binds nothing to an earlier
// answer. It fails rather than returning nothing, so a test that accidentally
// binds one is told which step it asked for.
func noProduced(ref modelcorpus.Ref) (json.RawMessage, error) {
	return nil, fmt.Errorf("this comparison binds nothing to an earlier answer, and step %d was asked for",
		ref.Step)
}

// TestCompareArgument_AFactTruth_AcceptsEverySpellingTheRecipeProduced is the
// rule that lets a case bind the project by path and a model answer with the
// numeric identifier.
func TestCompareArgument_AFactTruth_AcceptsEverySpellingTheRecipeProduced(t *testing.T) {
	cases := []struct {
		name    string
		sent    any
		matched bool
	}{
		{name: "the spelling the prompt carried", sent: "eval-group/eval-project", matched: true},
		{name: "the other spelling the recipe produced", sent: "42", matched: true},
		{name: "that spelling as a number", sent: 42, matched: true},
		{name: "some other project", sent: "someone/else", matched: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			step := modelcorpus.Step{Args: []modelcorpus.Arg{
				requiredFact("project_id", modelcorpus.FactProjectPath),
			}}
			sent := map[string]json.RawMessage{"project_id": mustJSON(testCase.sent)}

			verdicts := compareArguments(step, sent, defaultFacts(), noProduced)

			if !verdicts[0].Compared {
				t.Fatal("Compared = false, want a value that was sent to be compared")
			}
			if verdicts[0].Matched != testCase.matched {
				t.Errorf("Matched = %v, want %v (reason %q)",
					verdicts[0].Matched, testCase.matched, verdicts[0].Reason)
			}
		})
	}
}

// TestCompareArgument_AFactRecordedUnderOnlyItsOtherSpelling_IsStillAccepted
// covers the world where the recipe produced the identifier and not the path,
// which is what the stimulus then carried.
func TestCompareArgument_AFactRecordedUnderOnlyItsOtherSpelling_IsStillAccepted(t *testing.T) {
	onlyTheIdentifier := map[string]string{modelcorpus.FactProjectID: "42"}
	onlyThePath := map[string]string{modelcorpus.FactProjectPath: "eval-group/eval-project"}

	cases := []struct {
		name    string
		facts   map[string]string
		sent    any
		matched bool
	}{
		{name: "the spelling that was recorded", facts: onlyTheIdentifier, sent: 42, matched: true},
		{
			name:  "a spelling nothing recorded",
			facts: onlyTheIdentifier,
			sent:  "eval-group/eval-project",
		},
		{
			name:    "a world that produced the path and no identifier",
			facts:   onlyThePath,
			sent:    "eval-group/eval-project",
			matched: true,
		},
		{name: "and the identifier it never produced", facts: onlyThePath, sent: 42},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			facts := testCase.facts
			step := modelcorpus.Step{Args: []modelcorpus.Arg{
				requiredFact("project_id", modelcorpus.FactProjectPath),
			}}
			sent := map[string]json.RawMessage{"project_id": mustJSON(testCase.sent)}

			verdicts := compareArguments(step, sent, facts, noProduced)

			if !verdicts[0].Compared {
				t.Fatal("Compared = false, want a value that was sent to be compared")
			}
			if verdicts[0].Matched != testCase.matched {
				t.Errorf("Matched = %v, want %v (reason %q)",
					verdicts[0].Matched, testCase.matched, verdicts[0].Reason)
			}
		})
	}
}

// TestCompareArgument_AFactTheAttemptNeverRecorded_SaysSoRatherThanMissing
// keeps a hole in the record from reading as a model that sent the wrong value.
func TestCompareArgument_AFactTheAttemptNeverRecorded_SaysSoRatherThanMissing(t *testing.T) {
	step := modelcorpus.Step{Args: []modelcorpus.Arg{requiredFact("runner_id", modelcorpus.FactRunnerID)}}
	sent := map[string]json.RawMessage{"runner_id": mustJSON(9)}

	verdicts := compareArguments(step, sent, defaultFacts(), noProduced)

	if verdicts[0].Matched {
		t.Fatal("Matched = true against a fact the attempt never recorded")
	}
	if !strings.Contains(verdicts[0].Reason, modelcorpus.FactRunnerID) {
		t.Errorf("Reason = %q, want it to name the fact that was missing", verdicts[0].Reason)
	}
}

// TestCompareArgument_ALiteralTruth_ComparesTheValueAndNotItsSpelling says what
// "exactly" means for a value the prompt states: the value, whether the model
// sent it as text or as a number.
func TestCompareArgument_ALiteralTruth_ComparesTheValueAndNotItsSpelling(t *testing.T) {
	cases := []struct {
		name    string
		literal string
		sent    any
		matched bool
	}{
		{name: "the same text", literal: "production", sent: "production", matched: true},
		{name: "other text", literal: "production", sent: "staging", matched: false},
		{name: "a number as text", literal: "10", sent: "10", matched: true},
		{name: "a number as a number", literal: "10", sent: 10, matched: true},
		{name: "a number a provider spelled with a fraction", literal: "10", sent: 10.0, matched: true},
		{name: "another number", literal: "10", sent: 11, matched: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			step := modelcorpus.Step{Args: []modelcorpus.Arg{{
				Name:     "per_page",
				Truth:    modelcorpus.Truth{Literal: testCase.literal},
				Required: true,
			}}}
			sent := map[string]json.RawMessage{"per_page": mustJSON(testCase.sent)}

			verdicts := compareArguments(step, sent, defaultFacts(), noProduced)

			if verdicts[0].Matched != testCase.matched {
				t.Errorf("Matched = %v, want %v (sent %q, want %q)",
					verdicts[0].Matched, testCase.matched, verdicts[0].Sent, verdicts[0].Want)
			}
		})
	}
}

// TestCompareArgument_AMissingArgument_IsAMissOnlyWhenItWasRequired
func TestCompareArgument_AMissingArgument_IsAMissOnlyWhenItWasRequired(t *testing.T) {
	cases := []struct {
		name     string
		required bool
		compared bool
	}{
		{name: "required", required: true, compared: true},
		{name: "optional", required: false, compared: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			step := modelcorpus.Step{Args: []modelcorpus.Arg{{
				Name:     "per_page",
				Truth:    modelcorpus.Truth{Literal: "10"},
				Required: testCase.required,
			}}}

			verdicts := compareArguments(step, map[string]json.RawMessage{}, defaultFacts(), noProduced)

			if verdicts[0].Compared != testCase.compared {
				t.Errorf("Compared = %v, want %v", verdicts[0].Compared, testCase.compared)
			}
			if verdicts[0].Matched {
				t.Error("Matched = true for an argument that was never sent")
			}
			if verdicts[0].Want != "10" {
				t.Errorf("Want = %q, want the truth reported even though nothing was sent", verdicts[0].Want)
			}
		})
	}
}

// TestCompareArgument_AnArgumentSentAsNull_IsNotSent says what a hole is: a
// model that sent the argument as null sent nothing for it, and reading the
// word as a value would report a mismatch where the argument is missing.
func TestCompareArgument_AnArgumentSentAsNull_IsNotSent(t *testing.T) {
	step := modelcorpus.Step{Args: []modelcorpus.Arg{
		requiredFact("project_id", modelcorpus.FactProjectPath),
	}}
	sent := map[string]json.RawMessage{"project_id": json.RawMessage(`null`)}

	verdicts := compareArguments(step, sent, defaultFacts(), noProduced)

	if verdicts[0].Reason != "the argument was not sent" {
		t.Errorf("Reason = %q, want the missing-argument reason", verdicts[0].Reason)
	}
}

// TestCompareArgument_ATruthThatIsNoneOfTheFour_IsReported covers the shape the
// corpus gate refuses and a hand-written key can still carry.
func TestCompareArgument_ATruthThatIsNoneOfTheFour_IsReported(t *testing.T) {
	step := modelcorpus.Step{Args: []modelcorpus.Arg{{Name: "anything", Required: true}}}
	sent := map[string]json.RawMessage{"anything": mustJSON("a value")}

	verdicts := compareArguments(step, sent, defaultFacts(), noProduced)

	if verdicts[0].Truth != TruthNone {
		t.Fatalf("Truth = %q, want %q", verdicts[0].Truth, TruthNone)
	}
	if verdicts[0].Matched || verdicts[0].Reason == "" {
		t.Errorf("matched=%v reason=%q, want a miss that says the case declares no truth",
			verdicts[0].Matched, verdicts[0].Reason)
	}
}

// producedKey is a two-step key whose second step binds to the first step's
// answer, which is the shape every delete-what-you-just-made case has.
func producedKey() modelcorpus.Key {
	return modelcorpus.Key{Steps: []modelcorpus.Step{
		{
			Action: "issue.create",
			Args: []modelcorpus.Arg{
				requiredFact("project_id", modelcorpus.FactProjectPath),
				{Name: "title", Truth: modelcorpus.Truth{Authored: true}, Required: true},
			},
			Produces: []string{"iid"},
		},
		{
			Action: "issue.delete",
			Args: []modelcorpus.Arg{
				requiredFact("project_id", modelcorpus.FactProjectPath),
				{
					Name:     "issue_iid",
					Truth:    modelcorpus.Truth{Produced: modelcorpus.Ref{Step: 1, Field: "iid"}},
					Required: true,
				},
			},
		},
	}}
}

// TestScore_AProducedTruth_IsReadFromTheRecordedAnswer is what makes a run
// re-scorable: the value the second step had to carry is in the record, so the
// binding is checked months later without asking GitLab anything.
func TestScore_AProducedTruth_IsReadFromTheRecordedAnswer(t *testing.T) {
	created := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"title":      "Evaluate schema discovery",
	}

	cases := []struct {
		name    string
		iid     any
		opts    []callOption
		want    Outcome
		reasons string
	}{
		{
			name: "the model carried the identifier the answer gave it",
			iid:  17,
			opts: []callOption{resulting(map[string]any{"iid": 17})},
			want: OutcomeCompleted,
		},
		{
			name:    "the model carried some other identifier",
			iid:     99,
			opts:    []callOption{resulting(map[string]any{"iid": 17})},
			want:    OutcomeFailed,
			reasons: "step 1",
		},
		{
			name:    "the answer was truncated at the record's cap",
			iid:     17,
			opts:    []callOption{resulting(map[string]any{"iid": 17}), truncated()},
			want:    OutcomeFailed,
			reasons: "cut off",
		},
		{
			name:    "the answer carried no such field",
			iid:     17,
			opts:    []callOption{resulting(map[string]any{"id": 17})},
			want:    OutcomeFailed,
			reasons: "no field",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			deleting := map[string]any{
				"project_id": defaultFacts()[modelcorpus.FactProjectPath],
				"issue_iid":  testCase.iid,
			}
			trip := trajectory{calls: []modelrecord.Call{
				dynamicExecute(1, "issue.create", "issue.create", created, testCase.opts...),
				dynamicConfirm(2, "issue.delete", deleting),
			}}

			verdict := trip.score(t, producedKey())

			if verdict.Outcome != testCase.want {
				t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, testCase.want)
			}
			if testCase.reasons != "" && !strings.Contains(verdict.Reason, testCase.reasons) {
				t.Errorf("Reason = %q, want it to name %q", verdict.Reason, testCase.reasons)
			}
		})
	}
}

// TestScore_AProducedTruthBoundToAStepThatNeverRan_SaysSo covers the two
// bindings nothing can resolve: a step the key does not have, and a step the
// model never reached.
func TestScore_AProducedTruthBoundToAStepThatNeverRan_SaysSo(t *testing.T) {
	deleting := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"issue_iid":  17,
	}
	trip := trajectory{calls: []modelrecord.Call{dynamicConfirm(1, "issue.delete", deleting)}}

	verdict := trip.score(t, producedKey())

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	if !strings.Contains(verdict.Reason, "step 1") {
		t.Errorf("Reason = %q, want it to name the step that answered nothing", verdict.Reason)
	}
}

// TestScore_AProducedTruthBoundToANullField_SaysTheAnswerCarriedNoValue keeps
// a field GitLab sent as null from being compared as though it were a value.
func TestScore_AProducedTruthBoundToANullField_SaysTheAnswerCarriedNoValue(t *testing.T) {
	created := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"title":      "Evaluate schema discovery",
	}
	deleting := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"issue_iid":  17,
	}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "issue.create", "issue.create", created,
			resulting(map[string]any{"iid": nil})),
		dynamicConfirm(2, "issue.delete", deleting),
	}}

	verdict := trip.score(t, producedKey())

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	if !strings.Contains(verdict.Reason, "no value") {
		t.Errorf("Reason = %q, want it to say the answer carried no value there", verdict.Reason)
	}
}

// TestEqualValue_AnAbsentValueMatchesNothing covers the guard that keeps a hole
// from matching an empty truth.
func TestEqualValue_AnAbsentValueMatchesNothing(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "nothing against a value", raw: ``, want: "production"},
		{name: "nothing against nothing", raw: ``, want: ""},
		{name: "null against nothing", raw: `null`, want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if equalValue(json.RawMessage(testCase.raw), testCase.want) {
				t.Error("equalValue = true for an argument that was never sent")
			}
		})
	}
}

// TestEqualValue_TwoSpellingsOfOneNumber_AreOneValue is the comparison a
// textual one would get wrong: a provider that writes an integer back with a
// fraction has still named the same object.
func TestEqualValue_TwoSpellingsOfOneNumber_AreOneValue(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		want  string
		equal bool
	}{
		{name: "the same digits", raw: `42`, want: "42", equal: true},
		{name: "a fraction that is the same number", raw: `42.0`, want: "42", equal: true},
		{name: "another number", raw: `42.5`, want: "42"},
		{name: "text that is not a number", raw: `"forty-two"`, want: "42"},
		{name: "a number against text that is not one", raw: `42`, want: "forty-two"},
		// Text that is not a number parses as zero, so a truth of "0" would
		// match anything at all if the two guards were read one at a time.
		{name: "text against the number zero", raw: `"forty-two"`, want: "0"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := equalValue(json.RawMessage(testCase.raw), testCase.want); got != testCase.equal {
				t.Errorf("equalValue(%s, %q) = %v, want %v", testCase.raw, testCase.want, got, testCase.equal)
			}
		})
	}
}

// TestProducedFrom_AReferenceOutsideTheKey_IsAnError covers the binding a
// hand-written key can carry and the corpus gate refuses, and tells it from the
// binding that is inside the key and simply had nothing to read.
func TestProducedFrom_AReferenceOutsideTheKey_IsAnError(t *testing.T) {
	resolve := producedFrom([]stepMatch{{position: 1}})

	cases := []struct {
		name  string
		ref   modelcorpus.Ref
		names string
	}{
		{
			name:  "before the first step",
			ref:   modelcorpus.Ref{Step: 0, Field: "id"},
			names: "not a step of this case",
		},
		{
			name:  "after the last step",
			ref:   modelcorpus.Ref{Step: 4, Field: "id"},
			names: "not a step of this case",
		},
		{
			name:  "the last step of the key, which nothing reached",
			ref:   modelcorpus.Ref{Step: 1, Field: "id"},
			names: "never reached",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := resolve(testCase.ref)
			if err == nil {
				t.Fatal("resolving a reference with nothing behind it returned no error")
			}
			if !strings.Contains(err.Error(), testCase.names) {
				t.Errorf("error = %q, want it to say %q", err, testCase.names)
			}
		})
	}
}

// TestFieldAt_WalksTheDottedPathIntoAnAnswer covers the two shapes a binding
// takes in the corpus: a field at the top of an answer, and one inside the
// single object the answer wraps it in.
func TestFieldAt_WalksTheDottedPathIntoAnAnswer(t *testing.T) {
	answer := mustJSON(map[string]any{
		"iid":     7,
		"message": map[string]any{"id": 31},
	})

	cases := []struct {
		name  string
		path  string
		found bool
		value string
	}{
		{name: "a field at the top", path: "iid", found: true, value: "7"},
		{name: "a field one object down", path: "message.id", found: true, value: "31"},
		{name: "a field nothing carries", path: "slug", found: false},
		{name: "a path through a value that is not an object", path: "iid.id", found: false},
		{name: "no path at all", path: "", found: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, found := fieldAt(answer, testCase.path)
			if found != testCase.found {
				t.Fatalf("found = %v, want %v", found, testCase.found)
			}
			if found && string(value) != testCase.value {
				t.Errorf("value = %s, want %s", value, testCase.value)
			}
		})
	}
}

// TestCanonical_RendersEveryJSONValueTheComparisonSees
func TestCanonical_RendersEveryJSONValueTheComparisonSees(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		value   string
		present bool
	}{
		{name: "a string", raw: `"production"`, value: "production", present: true},
		{name: "a number", raw: `42`, value: "42", present: true},
		{name: "a boolean", raw: `true`, value: "true", present: true},
		{name: "an array", raw: `["a","b"]`, value: `["a","b"]`, present: true},
		{name: "null", raw: `null`, present: false},
		{name: "nothing at all", raw: ``, present: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, present := canonical(json.RawMessage(testCase.raw))
			if present != testCase.present {
				t.Fatalf("present = %v, want %v", present, testCase.present)
			}
			if value != testCase.value {
				t.Errorf("value = %q, want %q", value, testCase.value)
			}
		})
	}
}

// TestFactSpellings_NameFactsSomeRecipePromises keeps the table from going
// stale: a spelling no recipe produces would silently stop widening anything.
func TestFactSpellings_NameFactsSomeRecipePromises(t *testing.T) {
	promised := map[string]bool{}
	for _, recipe := range modelcorpus.Recipes() {
		facts, known := modelcorpus.Facts(recipe)
		if !known {
			t.Fatalf("the corpus lists recipe %q and promises nothing for it", recipe)
		}
		for _, fact := range facts {
			promised[fact] = true
		}
	}
	for key, spellings := range factSpellings {
		t.Run(key, func(t *testing.T) {
			if !promised[key] {
				t.Errorf("no recipe promises %q, so accepting other spellings of it widens nothing", key)
			}
			for _, spelling := range spellings {
				if !promised[spelling] {
					t.Errorf("no recipe promises %q, so it can never be an accepted spelling of %q",
						spelling, key)
				}
			}
		})
	}
}
