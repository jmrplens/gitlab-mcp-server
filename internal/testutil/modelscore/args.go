// args.go compares the values a model sent against the four truths a case
// declares.
//
// This is the whole of what replaced the old evaluator's parameter-name list.
// There, a step declared the names an answer had to carry and the check was
// that they were present, so a call naming project_id and sending the wrong
// project passed; the column that reported it was called argument fidelity.
// Here a case says where each right value comes from, and nothing about a value
// is inferred from the prompt: a recipe fact, a value the prompt states
// verbatim, a field of an earlier answer, or a value the model composes and
// nobody compares.

package modelscore

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
)

// TruthKind names where an argument's right value came from.
type TruthKind string

const (
	// TruthFact is a value the recipe built and the stimulus was rendered with.
	TruthFact TruthKind = "fact"
	// TruthLiteral is a value the prompt states verbatim.
	TruthLiteral TruthKind = "literal"
	// TruthProduced is a field of an earlier step's answer.
	TruthProduced TruthKind = "produced"
	// TruthAuthored is a value the model writes, which is reported and never
	// compared.
	TruthAuthored TruthKind = "authored"
	// TruthNone is a truth that is none of the four. The corpus gate refuses
	// one, so it cannot arrive from a corpus in this tree; it is a kind of its
	// own so that a hand-written key in a test is reported rather than
	// silently compared against the empty string.
	TruthNone TruthKind = "none"
)

// ArgumentVerdict is one declared argument and what became of it.
type ArgumentVerdict struct {
	// Name is the argument as the action's input schema spells it.
	Name string
	// Truth is where its right value came from.
	Truth TruthKind
	// Required is whether a completed attempt had to send it.
	Required bool
	// Compared is whether the value was put to a truth at all. An authored
	// value never is, and neither is an optional argument the model left out.
	Compared bool
	// Matched is whether it matched, meaningful only when Compared.
	Matched bool
	// Sent is what the model sent, rendered as the string the comparison was
	// made on. It is empty when the argument was absent.
	Sent string
	// Want is the value the truth resolved to, for a reader of a failure. A
	// fact with several accepted spellings names them all.
	Want string
	// Reason says why a compared argument did not match, or why one could not
	// be compared at all: a truncated answer, a field the answer does not
	// carry, a fact the recipe did not produce.
	Reason string
}

// factSpellings names the further facts a truth of one key is also satisfied
// by.
//
// A project is the case that decides the shape: an argument that takes either
// the numeric ID or the full path is satisfied by either, so a case binding
// project_path is not a claim that the model must have used the path. The
// relation is deliberately one-way. A case binds the numeric spelling only
// where the action's schema types the argument as an integer, and an integer
// argument refuses a path, so accepting the path there would pass a call GitLab
// would refuse.
//
// It lives here rather than in the corpus because it is a scoring rule: what a
// prompt hands the model is the one value the fact renders as, and which other
// spellings are still right is a question only a comparison asks.
var factSpellings = map[string][]string{
	modelcorpus.FactProjectPath: {modelcorpus.FactProjectID},
	modelcorpus.FactGroupPath:   {modelcorpus.FactGroupID},
}

// producedValue resolves a field of an earlier step's answer, or says why it
// cannot be.
type producedValue func(ref modelcorpus.Ref) (json.RawMessage, error)

// compareArguments puts every declared argument of one step to its truth.
//
// sent is the object the surface puts the action's own arguments in, already
// located; facts are the values the recipe produced for this attempt.
func compareArguments(
	step modelcorpus.Step,
	sent map[string]json.RawMessage,
	facts map[string]string,
	produced producedValue,
) []ArgumentVerdict {
	verdicts := make([]ArgumentVerdict, 0, len(step.Args))
	for _, arg := range step.Args {
		verdicts = append(verdicts, compareArgument(arg, sent, facts, produced))
	}
	return verdicts
}

// compareArgument puts one argument to its truth.
func compareArgument(
	arg modelcorpus.Arg,
	sent map[string]json.RawMessage,
	facts map[string]string,
	produced producedValue,
) ArgumentVerdict {
	verdict := ArgumentVerdict{Name: arg.Name, Truth: truthKind(arg.Truth), Required: arg.Required}
	if verdict.Truth == TruthAuthored {
		verdict.Sent, _ = canonical(sent[arg.Name])
		return verdict
	}

	value, present := canonical(sent[arg.Name])
	verdict.Sent = value
	accepted, reason := resolveWant(arg.Truth, facts, produced)
	verdict.Want = strings.Join(accepted, " or ")

	if !present {
		if !arg.Required {
			return verdict
		}
		verdict.Compared = true
		verdict.Reason = "the argument was not sent"
		return verdict
	}

	verdict.Compared = true
	if reason != "" {
		verdict.Reason = reason
		return verdict
	}
	for _, want := range accepted {
		if equalValue(sent[arg.Name], want) {
			verdict.Matched = true
			return verdict
		}
	}
	verdict.Reason = mismatchReason(arg.Truth)
	return verdict
}

// truthKind names which of the four a truth is.
func truthKind(truth modelcorpus.Truth) TruthKind {
	switch {
	case truth.Authored:
		return TruthAuthored
	case truth.Fact != "":
		return TruthFact
	case truth.Literal != "":
		return TruthLiteral
	case !truth.Produced.IsZero():
		return TruthProduced
	default:
		return TruthNone
	}
}

// resolveWant returns the values a truth accepts, or the reason it resolves to
// none.
//
// It is one function for both readings of a truth, the one an argument that was
// sent is compared against and the one an argument that was not sent is
// reported against, so the two cannot come to say different things about the
// same declaration.
func resolveWant(
	truth modelcorpus.Truth,
	facts map[string]string,
	produced producedValue,
) (accepted []string, reason string) {
	switch truthKind(truth) {
	case TruthFact:
		values := factValues(truth.Fact, facts)
		if len(values) == 0 {
			return nil, fmt.Sprintf("the attempt recorded no fact named %q, so there is nothing to "+
				"compare this argument with", truth.Fact)
		}
		return values, ""
	case TruthLiteral:
		return []string{truth.Literal}, ""
	case TruthProduced:
		answer, err := produced(truth.Produced)
		if err != nil {
			return nil, err.Error()
		}
		want, present := canonical(answer)
		if !present {
			return nil, fmt.Sprintf("step %d answered with no value at %q",
				truth.Produced.Step, truth.Produced.Field)
		}
		return []string{want}, ""
	default:
		return nil, "the case declares no truth for this argument"
	}
}

// mismatchReason says why a value that was sent is not the right one, in the
// terms of the truth it was put to.
func mismatchReason(truth modelcorpus.Truth) string {
	switch truthKind(truth) {
	case TruthFact:
		return "the value is none of the spellings the recipe produced"
	case TruthLiteral:
		return "the value is not the one the prompt states"
	default:
		return fmt.Sprintf("the value is not what step %d answered at %q",
			truth.Produced.Step, truth.Produced.Field)
	}
}

// factValues returns every value a fact truth accepts. The named fact comes
// first, because it is the one the stimulus was rendered with and so the one a
// reader of a failure expects to see.
func factValues(key string, facts map[string]string) []string {
	var accepted []string
	if value, produced := facts[key]; produced {
		accepted = append(accepted, value)
	}
	for _, spelling := range factSpellings[key] {
		if value, produced := facts[spelling]; produced {
			accepted = append(accepted, value)
		}
	}
	return accepted
}

// canonical renders a JSON value as the string a comparison is made on, and
// reports whether there was a value at all.
//
// A JSON null is no value: a model that sent an argument as null sent nothing
// for it, and reading it as the text "null" would compare a hole against a
// fixture value and report a mismatch where the argument is simply missing.
func canonical(raw json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	return trimmed, true
}

// equalValue reports whether a sent value is the wanted one.
//
// Numbers are compared as numbers, which is the one place a textual comparison
// would be wrong rather than merely strict: an argument that takes an ID is
// typed as an integer on some actions and as either spelling on others, so a
// model sending 42 and a fixture holding "42" agree about the project, and a
// provider that spells an integer back as 42.0 has still named it.
func equalValue(raw json.RawMessage, want string) bool {
	sent, present := canonical(raw)
	if !present {
		return false
	}
	if sent == want {
		return true
	}
	left, leftIsNumber := strconv.ParseFloat(sent, 64)
	right, rightIsNumber := strconv.ParseFloat(want, 64)
	return leftIsNumber == nil && rightIsNumber == nil && left == right
}
