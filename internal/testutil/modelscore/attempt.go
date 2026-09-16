// attempt.go is the pure function this package exists for: one attempt's lines
// and one case's key in, one verdict out.
//
// Nothing here writes anything and nothing here reads a stimulus. A verdict is
// recomputed from the record every time a report is generated, so a rule
// corrected today re-scores every run ever recorded, which is what the
// Markdown-then-parse loop of the evaluator this replaces made impossible.

package modelscore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Outcome is what became of one attempt.
type Outcome string

const (
	// OutcomeCompleted is every non-optional step reached in order or
	// correctly declined, every compared argument matched, every reaching call
	// answered as the mode expects, and the recipe's own check passed.
	OutcomeCompleted Outcome = "completed"
	// OutcomeFailed is an attempt that ran and did not.
	OutcomeFailed Outcome = "failed"
	// OutcomeGitLabRefused is an attempt whose every step was dispatched with
	// the right arguments and at least one of which GitLab then refused. It is
	// its own outcome rather than a failure, because what it reports is the
	// instance or the fixture and not the model.
	OutcomeGitLabRefused Outcome = "gitlab_refused"
	// OutcomeOverBudget is an attempt that hit the turn cap.
	OutcomeOverBudget Outcome = "over_budget"
	// OutcomeMalformed is an attempt whose model emitted a tool call that
	// could not be parsed.
	OutcomeMalformed Outcome = "malformed"
	// OutcomeProviderError is an attempt the provider would not answer.
	OutcomeProviderError Outcome = "provider_error"
	// OutcomeHarnessError is an attempt this side broke.
	OutcomeHarnessError Outcome = "harness_error"
	// OutcomeSkipped is an attempt that never ran because the runtime did not
	// meet the case's needs. It is in no denominator: a licensed case on a
	// community instance is absent for a reason, and counting it would rank a
	// model by the license of the instance it was measured on.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeUnobserved is an attempt at least one of whose steps was decided
	// by a call whose span never arrived. Every such verdict would be about
	// what the model asked for rather than about what ran, so the attempt is
	// counted apart and appears in no published numerator or denominator.
	OutcomeUnobserved Outcome = "unobserved"
)

// StepVerdict is one declared step and what became of it.
type StepVerdict struct {
	// Position is the step's 1-based place in the key.
	Position int
	// Name is the canonical action the step declares, or the name of the tool
	// when the step names one registered outside the catalog.
	Name string
	// Standalone is whether Name is a tool rather than an action.
	Standalone bool
	// Optional is whether a completed attempt had to reach it.
	Optional bool
	// Destructive is whether the server refuses it without a confirmation.
	Destructive bool
	// Mutating is whether it changes anything, which is what a protective mode
	// withholds or previews.
	Mutating bool
	// Known is whether the catalog at this tier has it at all. A step that
	// names nothing is a corpus defect rather than a model failure, and it is
	// reported as one.
	Known bool
	// Reached is whether a call named it.
	Reached bool
	// MatchedBy is which reading matched: the server's dispatch, the model's
	// request, or the name of a standalone tool.
	MatchedBy MatchKind
	// Requested is the action the reaching call named.
	Requested string
	// Dispatched is the action the server said it ran for that call.
	Dispatched string
	// Rewritten is whether the two differ, which is the alias case: the model
	// named one action and the dispatcher ran another.
	Rewritten bool
	// Call is the index of the reaching call among the attempt's calls, zero
	// when no call reached the step.
	Call int
	// CallOutcome is that call's own outcome, in the record's vocabulary.
	CallOutcome string
	// Answer is what that outcome was worth under the mode.
	Answer Answer
	// Decline says how a mutating step was correctly declined, which is only
	// ever set in read-only mode.
	Decline Decline
	// Confirmation says how a destructive step came to carry its approval.
	Confirmation Confirmation
	// Arguments are the declared arguments and what became of each.
	Arguments []ArgumentVerdict
	// AcceptedFirstTime is whether the first call about this step was the one
	// that reached it and was accepted.
	AcceptedFirstTime bool
	// Retries is how many calls about this step came before the reaching one.
	Retries int
	// Observed is whether the verdict rests on a call whose span arrived. A
	// step decided by an unobserved call is a claim about what was asked.
	Observed bool
	// Complete is whether this step went the way the case and the mode expect.
	Complete bool
	// Reason says why it did not, empty when it did.
	Reason string
}

// Verdict is one attempt scored.
type Verdict struct {
	// Case is the corpus case identifier.
	Case string
	// Model is the provider specification that was asked.
	Model string
	// Surface, Mode and Tier are the row this attempt belongs to.
	Surface string
	Mode    string
	Tier    string
	// Session is the session label that served it.
	Session string
	// Repeat is which run of this attempt it was, from 1.
	Repeat int
	// Outcome is what became of it.
	Outcome Outcome
	// Reason says why, for every outcome but completed.
	Reason string
	// Steps are the declared steps and what became of each.
	Steps []StepVerdict
	// Discovery is how many catalog searches the model made. It is the
	// declared cost of the dynamic surface and is never a failure.
	Discovery int
	// InvalidParams is how many calls this server refused for their arguments,
	// which is a model learning a parameter name from a refusal. A refusal
	// that was itself a correct decline is not counted here: on the meta
	// surface a withheld action is refused by schema validation, and counting
	// that as a retry would charge a model for declining correctly.
	InvalidParams int
	// Unaided is whether the attempt completed with no refusal of ours
	// anywhere: no rejected arguments and no refused confirmation.
	Unaided bool
	// Verified is whether every recipe check against GitLab passed. An attempt
	// with no check recorded is verified by default, since there was nothing
	// to fail.
	Verified bool
}

// Score is the whole of this package's contract: one attempt's record and one
// case's key in, one verdict out.
//
// The error is for a record this tree cannot read rather than for a model that
// did badly: a surface that is not one of the three, a tier the catalog cannot
// be built at, a catalog that will not build. A model that did badly is a
// verdict, not an error.
func Score(attempt Attempt, key modelcorpus.Key) (Verdict, error) {
	facts, err := catalogFor(attempt.surface(), attempt.tier())
	if err != nil {
		return Verdict{}, fmt.Errorf("score attempt %q of case %q: %w",
			attempt.Line.ID, attempt.Line.Case, err)
	}

	one := scorer{attempt: attempt, facts: facts, mode: attempt.mode()}
	one.events = attempt.events(facts)
	one.matches = matchSteps(key, one.events)

	verdict := Verdict{
		Case:      attempt.Line.Case,
		Model:     attempt.Line.Model,
		Surface:   attempt.surface(),
		Mode:      one.mode,
		Tier:      attempt.tier(),
		Session:   attempt.Line.Session,
		Repeat:    attempt.Line.Repeat,
		Discovery: countDiscovery(one.events),
		Verified:  verified(attempt.Verifies),
		Steps:     make([]StepVerdict, 0, len(one.matches)),
	}
	for _, match := range one.matches {
		verdict.Steps = append(verdict.Steps, one.step(match))
	}
	verdict.InvalidParams = countInvalidParams(one.events, verdict.Steps)
	verdict.Outcome, verdict.Reason = outcomeOf(attempt, verdict)
	verdict.Unaided = verdict.Outcome == OutcomeCompleted && unaided(one.events, verdict.Steps)
	return verdict, nil
}

// ScoreCase is [Score] for a caller that may not hold a key: it resolves the
// answer for the case the attempt names and scores against that.
//
// It exists for the runner. A run is meant to say what each attempt came to as
// it ends, and the corpus's boundary refuses a run that reads an answer, so
// the two statements could not both be true while the only entry point took a
// key. This package is one of the sanctioned readers, so the lookup happens
// here and what crosses back is a verdict about an attempt that has already
// ended: nothing a model was shown can be derived from it, and nothing written
// to the record comes from it either, since the record stays observation and a
// report re-scores it from the corpus at HEAD.
//
// A case the corpus does not have is an error rather than an empty verdict. A
// zero verdict reads as a model that did nothing, which is a measurement, and
// an attempt naming a case nobody wrote is not one.
func ScoreCase(attempt Attempt) (Verdict, error) {
	key, known := modelcorpus.Keys()[attempt.Line.Case]
	if !known {
		return Verdict{}, fmt.Errorf("score attempt %q: the corpus has no case %q",
			attempt.Line.ID, attempt.Line.Case)
	}
	return Score(attempt, key)
}

// scorer is one attempt under one catalog: everything scoring a step needs,
// held once rather than threaded through every function that wants a piece of
// it.
type scorer struct {
	attempt Attempt
	facts   catalogFacts
	mode    string
	events  []Event
	matches []stepMatch
}

// step decides what became of one step.
func (s scorer) step(match stepMatch) StepVerdict {
	declared, known := s.facts.stepFacts(match.step)
	verdict := StepVerdict{
		Position:    match.position,
		Name:        stepName(match.step),
		Standalone:  match.step.Standalone != "",
		Optional:    match.step.Optional,
		Destructive: declared.destructive,
		Mutating:    known && !declared.readOnly,
		Known:       known,
		Reached:     match.reached,
		Retries:     len(match.prior),
		Observed:    true,
	}
	if !known {
		verdict.Reason = fmt.Sprintf("the catalog at this tier has no %s, so the case cannot be scored "+
			"against it", verdict.Name)
		return verdict
	}

	if match.reached {
		s.scoreReached(&verdict, match)
	} else if s.mode == ModeReadOnly && verdict.Mutating {
		s.scoreTextDecline(&verdict)
	}

	if !verdict.Reached && verdict.Decline == DeclineNone {
		verdict.Complete = match.step.Optional
		if !verdict.Complete {
			verdict.Reason = unreachedReason(match)
			// "Nothing named this step" is read from what the calls in the
			// window named, so a call whose span never arrived leaves it
			// unanswerable: the dispatch that did not arrive might have named
			// the step, which is exactly the alias case, where the model asks
			// for one action and the server runs another. An optional step is
			// not asked, because nothing was owed there and no column reads it.
			verdict.Observed = !match.unobservedInWindow
		}
	}
	verdict.Confirmation = confirmationOf(match, s.facts)
	return verdict
}

// scoreReached fills in a step some call named.
func (s scorer) scoreReached(verdict *StepVerdict, match stepMatch) {
	event := match.event
	verdict.MatchedBy = match.by
	verdict.Requested = event.Requested
	verdict.Dispatched = event.Dispatched
	verdict.Rewritten = event.Requested != "" && event.Dispatched != "" && event.Requested != event.Dispatched
	verdict.Call = event.Index
	verdict.CallOutcome = event.Outcome
	verdict.Observed = event.Observed
	verdict.Arguments = compareArguments(
		match.step,
		event.actionArguments(s.facts),
		s.attempt.Line.Facts,
		producedFrom(s.matches),
	)

	if s.mode == ModeReadOnly && verdict.Mutating {
		if declinedByRefusal(event) {
			verdict.Decline = DeclineByRefusal
			verdict.Answer = AnswerAccepted
		} else {
			verdict.Answer = AnswerWrong
			verdict.Reason = "read-only withholds this action and the model was served rather than declined"
		}
	} else {
		verdict.Answer = answerOf(s.mode, verdict.Mutating, event)
	}

	verdict.AcceptedFirstTime = len(match.prior) == 0 && verdict.Answer == AnswerAccepted
	verdict.Complete = verdict.Answer == AnswerAccepted && argumentsMatched(verdict.Arguments)
	if verdict.Complete || verdict.Reason != "" {
		return
	}
	verdict.Reason = stepReason(*verdict)
}

// scoreTextDecline fills in a mutating step nothing named, under read-only,
// where declining in text is the right answer.
//
// The two conditions are the rule of section 4.2: the conversation ended in
// prose, and nothing this attempt sent actually ran a mutation. An attempt that
// mutated something unrelated and then declined fails, because what it did is
// not what a read-only deployment asked for.
func (s scorer) scoreTextDecline(verdict *StepVerdict) {
	if !s.attempt.endedInText() {
		return
	}
	if mutatingDispatched(s.events, s.facts) {
		verdict.Reason = "the attempt dispatched a mutating action and then declined in text"
		return
	}
	verdict.Decline = DeclineByText
	verdict.Answer = AnswerAccepted
	verdict.Complete = true
	verdict.AcceptedFirstTime = true
	// The rule reads every call's dispatch, so a call whose span never arrived
	// leaves it unanswerable: nothing here can tell a model that declined from
	// one whose mutation was simply not seen.
	verdict.Observed = allObserved(s.events)
}

// stepName is what a verdict calls a step: the canonical action, or the tool
// when the step names one registered outside the catalog.
func stepName(step modelcorpus.Step) string {
	if step.Standalone != "" {
		return step.Standalone
	}
	return string(step.Action)
}

// unreachedReason says why nothing answered a step.
//
// The two are worth telling apart. A step with no calls about it at all is a
// model that never tried; a step with calls that were all refused and re-asked
// is a model that tried and gave up, which is the shape a destructive step
// takes when the confirmation never came.
func unreachedReason(match stepMatch) string {
	if len(match.prior) == 0 {
		return "no call named this step"
	}
	last := match.prior[len(match.prior)-1]
	return fmt.Sprintf("%d call(s) named this step and the server asked for each again (%s); none was answered",
		len(match.prior), last.Outcome)
}

// stepReason says why a reached step did not go the way the case expects.
func stepReason(verdict StepVerdict) string {
	if verdict.Answer != AnswerAccepted {
		if verdict.Answer == AnswerGitLabRefused {
			return "the action ran and GitLab refused it: " + verdict.CallOutcome
		}
		return "the call was answered " + verdict.CallOutcome + ", which is not what this mode expects"
	}
	var missed []string
	for _, argument := range verdict.Arguments {
		if argument.Compared && !argument.Matched {
			missed = append(missed, argument.Name+" ("+argument.Reason+")")
		}
	}
	return "the arguments did not carry the values the case declares: " + strings.Join(missed, ", ")
}

// argumentsMatched reports whether every argument that was compared matched.
func argumentsMatched(arguments []ArgumentVerdict) bool {
	for _, argument := range arguments {
		if argument.Compared && !argument.Matched {
			return false
		}
	}
	return true
}

// producedFrom builds the resolver a Produced truth is read through.
//
// It reads the recorded answer of an earlier step rather than asking GitLab
// anything, which is what lets a run be re-scored months later. A truncated
// answer fails the binding naming the truncation: the cap is documented on the
// record, and a binding that silently missed because the field was cut off
// would read as a model that sent the wrong value.
func producedFrom(matches []stepMatch) producedValue {
	return func(ref modelcorpus.Ref) (json.RawMessage, error) {
		if ref.Step < 1 || ref.Step > len(matches) {
			return nil, fmt.Errorf("step %d is not a step of this case", ref.Step)
		}
		earlier := matches[ref.Step-1]
		if !earlier.reached {
			return nil, fmt.Errorf("step %d was never reached, so it answered nothing to bind to", ref.Step)
		}
		if earlier.event.ResultTruncated {
			return nil, fmt.Errorf("step %d answered with more than the record keeps, so %q was cut off "+
				"rather than absent", ref.Step, ref.Field)
		}
		value, found := fieldAt(earlier.event.Result, ref.Field)
		if !found {
			return nil, fmt.Errorf("step %d answered with no field %q", ref.Step, ref.Field)
		}
		return value, nil
	}
}

// fieldAt walks a dotted path into a recorded answer. Every segment names a
// field of an object, which is what every binding in the corpus is: an
// identifier at the top of an answer, or one inside the single object the
// answer wraps it in.
func fieldAt(result json.RawMessage, path string) (json.RawMessage, bool) {
	value := result
	for segment := range strings.SplitSeq(path, ".") {
		next, found := decodeObject(value)[segment]
		if !found {
			return nil, false
		}
		value = next
	}
	return value, true
}

// verified reports whether every recipe check against GitLab passed.
func verified(checks []modelrecord.Verify) bool {
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

// countDiscovery counts the catalog searches an attempt made.
func countDiscovery(events []Event) int {
	count := 0
	for _, event := range events {
		if event.Discovery {
			count++
		}
	}
	return count
}

// countInvalidParams counts the calls this server refused for their arguments,
// leaving out the ones that were themselves correct declines.
func countInvalidParams(events []Event, steps []StepVerdict) int {
	declines := map[int]bool{}
	for _, step := range steps {
		if step.Decline == DeclineByRefusal {
			declines[step.Call] = true
		}
	}
	count := 0
	for _, event := range events {
		if refusedFor(event.Outcome, toolutil.RefusalInvalidParams) && !declines[event.Index] {
			count++
		}
	}
	return count
}

// unaided reports whether the attempt got there with no refusal of ours: no
// arguments rejected and no confirmation demanded.
func unaided(events []Event, steps []StepVerdict) bool {
	declines := map[int]bool{}
	for _, step := range steps {
		if step.Decline == DeclineByRefusal {
			declines[step.Call] = true
		}
	}
	for _, event := range events {
		if declines[event.Index] {
			continue
		}
		if refusedFor(event.Outcome, toolutil.RefusalInvalidParams) ||
			refusedFor(event.Outcome, toolutil.RefusalNeedsConfirmation) {
			return false
		}
	}
	return true
}

// allObserved reports whether every call of the attempt had its span arrive.
func allObserved(events []Event) bool {
	for _, event := range events {
		if !event.Observed {
			return false
		}
	}
	return true
}

// outcomeOf decides what became of the attempt as a whole.
//
// The endings the runner recorded come first and are not second-guessed: an
// attempt that hit the turn cap is over budget whatever its steps did, because
// the conversation it would have had is not in the record. What is derived from
// the steps is only the ending where the model finished talking.
func outcomeOf(attempt Attempt, verdict Verdict) (outcome Outcome, reason string) {
	switch attempt.Line.EndedBy {
	case modelrecord.EndedSkipped:
		return OutcomeSkipped, attempt.Line.Reason
	case modelrecord.EndedProviderError:
		return OutcomeProviderError, attempt.Line.Reason
	case modelrecord.EndedHarnessError:
		return OutcomeHarnessError, attempt.Line.Reason
	case modelrecord.EndedMalformed:
		return OutcomeMalformed, attempt.Line.Reason
	case modelrecord.EndedOverBudget:
		return OutcomeOverBudget, attempt.Line.Reason
	default:
		return outcomeFromSteps(verdict)
	}
}

// outcomeFromSteps reads the verdict of an attempt that finished talking.
//
// A refusal by GitLab carries forward, which is the half a step-by-step reading
// gets wrong. A case's steps are a sequence, so a step GitLab refused takes the
// steps after it with it: the identifier the next call needed was never
// answered, and the model had nothing to send. Charging those to the model
// would fold the instance into the very column the refusal is its own class to
// stay out of. A later step the model did reach and get wrong is still a
// failure, because that one it could have got right.
func outcomeFromSteps(verdict Verdict) (outcome Outcome, reason string) {
	refused := ""
	for _, step := range verdict.Steps {
		if !step.Observed {
			return OutcomeUnobserved, fmt.Sprintf("step %d rests on a call whose dispatch was never "+
				"observed", step.Position)
		}
		if step.Complete {
			continue
		}
		if step.Answer == AnswerGitLabRefused && argumentsMatched(step.Arguments) {
			refused = fmt.Sprintf("step %d (%s)", step.Position, step.Name)
			continue
		}
		// A step nothing reached, after a refusal. Whether it was declined is
		// not asked, because a declined step is complete and this one is not.
		if refused != "" && !step.Reached {
			return OutcomeGitLabRefused, fmt.Sprintf("%s was dispatched as the case declares and GitLab "+
				"refused it, so step %d (%s) was never reached", refused, step.Position, step.Name)
		}
		return OutcomeFailed, fmt.Sprintf("step %d (%s): %s", step.Position, step.Name, step.Reason)
	}
	if !verdict.Verified {
		return OutcomeFailed, "a recipe check against GitLab did not hold afterwards"
	}
	if refused != "" {
		return OutcomeGitLabRefused, "every step was dispatched as the case declares and GitLab refused one"
	}
	return OutcomeCompleted, ""
}
