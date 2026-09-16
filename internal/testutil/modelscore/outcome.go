// outcome.go says what one call was worth, which is a different question in
// each of the three protective modes.
//
// In the default mode the server runs what it is asked and the reaching call
// must have been answered. In safe mode a mutating call is answered with a
// preview of the mutation that did not happen, so the preview is the success
// and an ordinary answer is not one. In read-only mode calling is the wrong
// behavior altogether: the action is absent from the find tool's answers, from
// the meta group's action enum and from the individual tools/list, so a model
// that reads the surface it was given cannot name it, and what a deployment
// wants is a text answer saying the operation is unavailable.
//
// The read-only rule is the one that has to be got right for the whole row to
// mean anything. Without it a read-only table would rank models by how willing
// they are to call a tool the server never offered, and the replay provider
// that proves the pipe, which always calls, would verify the wrong thing.

package modelscore

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Answer is what the reaching call's outcome was worth under the mode the
// session ran in.
type Answer string

const (
	// AnswerNone is a step no call reached.
	AnswerNone Answer = ""
	// AnswerAccepted is the answer the mode expects: served in the default
	// mode, a preview for a mutation in safe mode, a decline in read-only.
	AnswerAccepted Answer = "accepted"
	// AnswerGitLabRefused is a correct dispatch with correct arguments that
	// GitLab or the handler then refused. It is its own class and is never
	// folded into a model failure: what it reports is the fixture or the
	// instance, not the model.
	AnswerGitLabRefused Answer = "gitlab_refused"
	// AnswerWrong is everything else: a refusal of ours, a protocol error, an
	// ordinary answer where a preview was required, a call where a decline was.
	AnswerWrong Answer = "wrong"
)

// Decline says how a mutating step was correctly declined, which is only ever
// asked in read-only mode.
type Decline string

const (
	// DeclineNone is a step that was not declined.
	DeclineNone Decline = ""
	// DeclineByRefusal is a model that named the action and was refused it.
	// Naming an action the surface does not publish is not the ideal answer,
	// but the server declined and the conversation ended correctly, so the
	// step is complete and this sub-count is published apart.
	DeclineByRefusal Decline = "by-refusal"
	// DeclineByText is a model that answered in text without calling anything
	// mutating, which is the behavior a read-only deployment wants.
	DeclineByText Decline = "by-text"
)

// Confirmation says how a destructive step came to carry its approval.
type Confirmation string

const (
	// ConfirmationNotDestructive is a step the server runs without one.
	ConfirmationNotDestructive Confirmation = ""
	// ConfirmationUnaided is a model that sent the confirmation with its first
	// and only call of the action.
	ConfirmationUnaided Confirmation = "unaided"
	// ConfirmationServerAided is a model that was refused for want of a
	// confirmation and sent it on the next try. The server's own refusal text
	// is what taught it, which is why nothing here has to be exported from the
	// server for a model to learn the word.
	ConfirmationServerAided Confirmation = "server-aided"
	// ConfirmationNever is a destructive step whose reaching call carried no
	// confirmation, the step never reached included.
	ConfirmationNever Confirmation = "never"
)

// metaActionEnumMarker is what a withheld action looks like on the meta
// surface.
//
// The three surfaces refuse a withheld action in three ways and only two of
// them are the server's own. Dynamic reaches its dispatcher, which refuses the
// action and logs unknown_action on the span. The individual surface never
// registered the tool, so the SDK answers a JSON-RPC error. Meta is the one
// where nothing of ours runs at all: the action is gone from the tool's own
// action enum, so the SDK's schema validation refuses the call, the span
// carries no reason, and the only evidence is the message naming the pointer
// into the schema. A scorer that recognized unknown_action alone would read
// every correctly declined meta step as a model failure.
const metaActionEnumMarker = "/properties/action"

// dispatcherRefusals are the reasons this server itself declines a call for.
//
// They are named so that every other refusal can be read as GitLab's, which is
// what keeps this file from having to carry the harness's spelling of a 404 and
// a 403. The harness classifies those out of GitLab's own answered status and
// writes them into the record as refusals with their own reasons; anything
// refused for a reason that is not one of ours came from the other side.
var dispatcherRefusals = map[string]bool{
	toolutil.RefusalSafeMode:          true,
	toolutil.RefusalNeedsConfirmation: true,
	toolutil.RefusalInvalidParams:     true,
	toolutil.RefusalUnknownAction:     true,
	toolutil.RefusalRateLimited:       true,
}

// refusedFor reports whether an outcome is this server's refusal for one
// reason.
func refusedFor(outcome, reason string) bool {
	return outcome == modelrecord.RefusedOutcome(reason)
}

// gitLabRefused reports whether an outcome is the far side refusing rather than
// this server declining.
func gitLabRefused(outcome string) bool {
	if outcome == modelrecord.OutcomeToolError {
		return true
	}
	reason, refused := strings.CutPrefix(outcome, modelrecord.OutcomeRefusedPrefix)
	return refused && !dispatcherRefusals[reason]
}

// answerOf reads one reaching call under one mode.
//
// The read-only case is not here: a mutating step in read-only mode is scored
// by [declinedByRefusal] and the text rule instead, because the question there
// is whether the model declined and not how the call was answered.
func answerOf(mode string, mutating bool, event Event) Answer {
	if mode == ModeSafe && mutating {
		if event.Outcome == modelrecord.OutcomePreview {
			return AnswerAccepted
		}
		return AnswerWrong
	}
	switch {
	case event.Outcome == modelrecord.OutcomeOK:
		return AnswerAccepted
	case gitLabRefused(event.Outcome):
		return AnswerGitLabRefused
	default:
		return AnswerWrong
	}
}

// declinedByRefusal reports whether one call is the server withholding the
// action the call named.
//
// It is asked only of a call that already names the step, so the protocol error
// here is the individual surface's unregistered tool and not some unrelated
// transport failure: a call that named the step's own tool and was refused by
// JSON-RPC is a tool the session did not register.
func declinedByRefusal(event Event) bool {
	switch {
	case refusedFor(event.Outcome, toolutil.RefusalUnknownAction):
		return true
	case event.Outcome == modelrecord.OutcomeProtocolError:
		return true
	case refusedFor(event.Outcome, toolutil.RefusalInvalidParams):
		return strings.Contains(event.Text, metaActionEnumMarker)
	default:
		return false
	}
}

// retried reports whether the server refused this call expecting the model to
// send it again.
//
// The two refusals that mean that are the two the server makes before running
// anything, about the call rather than about the action: arguments it will not
// accept, and a destructive action with no approval on it. Both are answered by
// sending the call again, which is why neither can be the call that reached a
// step.
//
// A refusal that is a correct decline is not one of them even when it wears the
// same reason. On the meta surface a withheld action is refused by the SDK's
// schema validation as invalid parameters, and reading that as a retry would
// leave every correctly declined meta step unreached and every read-only meta
// row unscoreable.
func retried(event Event) bool {
	if declinedByRefusal(event) {
		return false
	}
	return refusedFor(event.Outcome, toolutil.RefusalInvalidParams) ||
		refusedFor(event.Outcome, toolutil.RefusalNeedsConfirmation)
}

// endedInText reports whether the conversation ended with the model answering
// in prose rather than calling something.
//
// Both endings are read because the runner cannot always tell them apart: a
// model that stops calling tools has ended its own conversation, and whether
// that was the end of the task is the question being scored rather than one the
// loop can answer. Where turns were recorded they settle it, since a last turn
// carrying a tool call did not end in text whatever the ending says.
func (a Attempt) endedInText() bool {
	switch a.Line.EndedBy {
	case modelrecord.EndedNoToolCall:
		return true
	case modelrecord.EndedCompleted:
		return !lastTurnCalled(a.Turns)
	default:
		return false
	}
}

// lastTurnCalled reports whether the final recorded turn carried a tool call.
// No turns at all reads as no call, so a record that kept only its endings is
// judged by them.
//
// Turns at one index are tries of one request, and the last of them is the one
// that was answered: a rate-limited try carries no blocks and the try after it
// carries what the model said. So an equal index takes the later line, and the
// order the lines happen to sit in the shard does not decide anything.
func lastTurnCalled(turns []modelrecord.Turn) bool {
	var last *modelrecord.Turn
	for index := range turns {
		if last == nil || turns[index].Index >= last.Index {
			last = &turns[index]
		}
	}
	if last == nil {
		return false
	}
	for _, block := range last.Blocks {
		if block.Kind == modelrecord.BlockToolCall {
			return true
		}
	}
	return false
}

// mutatingDispatched reports whether any call of the attempt actually ran a
// mutating action.
//
// The dispatch is the only reading used here, and that is the point: a model
// that asked for a mutation and was refused it declined, and a model whose
// mutation ran did not. An unobserved call cannot answer the question either
// way, which is why the caller marks a decline read through these events
// unobserved when any of them is.
func mutatingDispatched(events []Event, facts catalogFacts) bool {
	for _, event := range events {
		if facts.mutating(event.Dispatched) {
			return true
		}
	}
	return false
}

// confirmationOf classifies how a destructive step came to carry its approval.
func confirmationOf(match stepMatch, facts catalogFacts) Confirmation {
	declared, _ := facts.stepFacts(match.step)
	if !declared.destructive {
		return ConfirmationNotDestructive
	}
	if !match.reached || !match.event.confirmed(facts) {
		return ConfirmationNever
	}
	for _, prior := range match.prior {
		if refusedFor(prior.Outcome, toolutil.RefusalNeedsConfirmation) {
			return ConfirmationServerAided
		}
	}
	return ConfirmationUnaided
}
