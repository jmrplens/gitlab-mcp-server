// match.go decides which call reached which step.
//
// The order is the key's. Step i is reached by the first event after step
// i-1's event that names it, so a model that ran the right actions in the wrong
// order reaches fewer steps than one that ran them in the right order, which is
// the difference the column is for.
//
// What "names it" means is the whole of this file, and it is two readings with
// a precedence rather than one. The server's own dispatch is the first, because
// it is the only thing that says what ran: gitlab_environment with action get
// and an environment name runs protected_get, and a scorer reading the request
// would credit an action that never happened. The model's request is the
// fallback, for the calls where the span names no action at all, and those are
// not an edge case: on every surface, a call a protective mode withheld is
// refused before any dispatch is recorded, so the request is the only reading
// such a call has and read-only rows could not be scored without it.

package modelscore

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"

// MatchKind says which reading matched a call to a step.
//
// It is published on every reached step rather than kept here, because the pair
// is what makes an alias rewrite visible: a step matched by the dispatch whose
// requested action differs is a model that named one action and ran another,
// which is a fact about the surface and not about the model.
type MatchKind string

const (
	// MatchDispatched is a step reached by the action the server said it ran.
	MatchDispatched MatchKind = "dispatched"
	// MatchRequested is a step reached by the action the call named, which is
	// the only reading a withheld or refused call has.
	MatchRequested MatchKind = "requested"
	// MatchTool is a step reached by the name of a tool registered outside the
	// catalog, which belongs to no action and so has no other reading.
	MatchTool MatchKind = "tool"
)

// stepMatch is one step of the key and the calls that were about it.
type stepMatch struct {
	// step is the key's own declaration.
	step modelcorpus.Step
	// position is the step's 1-based place in the key, which is what a
	// Produced truth refers to.
	position int
	// reached is whether any call named it.
	reached bool
	// by is which reading matched, meaningful only when reached.
	by MatchKind
	// event is the reaching call, valid only when reached.
	event Event
	// prior are the calls about this step that came before the reaching one:
	// a refused confirmation, a rejected parameter name, a retry. They are
	// what "accepted first time" is read from and where the server-aided
	// confirmation class is found.
	prior []Event
	// unobservedInWindow is whether the window a step nothing reached was
	// looked for in holds a call whose span never arrived. Such a call names
	// the step by what the model requested and by nothing else, so a dispatch
	// that did not arrive might have named it and "nothing named this step" is
	// then a claim about a request. It is meaningless when reached.
	unobservedInWindow bool
}

// matchSteps walks the key against the events in order.
//
// A discovery call is skipped rather than considered and rejected, which is
// this file's half of keeping the find call out of every column: it names no
// action, so it could not match a step anyway, and skipping it says so once
// here instead of relying on that.
//
// A call the server refused expecting another is skipped too, and that one is
// not obvious. Read literally, "the first call that names the step" would make
// the refusal the reaching call, and then a destructive step refused for want
// of a confirmation and re-sent with one could never be scored as reached at
// all: the confirmation would sit on a call that came after the step was
// already decided. What those two refusals mean is that the server has asked
// for the call again, so they are the step's history and the call that was
// answered is the one that reached it. Every other answer, an ordinary result,
// a preview, a refusal of the action itself, ends the step where it fell.
func matchSteps(key modelcorpus.Key, events []Event) []stepMatch {
	matches := make([]stepMatch, 0, len(key.Steps))
	from := 0
	for index, step := range key.Steps {
		match := stepMatch{step: step, position: index + 1}
		for at := from; at < len(events); at++ {
			if events[at].Discovery {
				continue
			}
			by, names := namesStep(events[at], step)
			if !names || retried(events[at]) {
				continue
			}
			match.reached = true
			match.by = by
			match.event = events[at]
			match.prior = priorEvents(events[from:at], step)
			from = at + 1
			break
		}
		if !match.reached {
			// The window stays where it was: a step nothing answered consumes
			// no calls, so a later step is still matched against everything
			// after the last step that was answered.
			match.prior = priorEvents(events[from:], step)
			match.unobservedInWindow = anyUnobserved(events[from:])
		}
		matches = append(matches, match)
	}
	return matches
}

// priorEvents returns the calls in a window that were about one step.
//
// The window is everything since the previous step was reached, so a model that
// tried this step twice has both tries here and a model that tried some other
// action in between has that one left out.
func priorEvents(window []Event, step modelcorpus.Step) []Event {
	var prior []Event
	for _, event := range window {
		if event.Discovery {
			continue
		}
		if _, names := namesStep(event, step); names {
			prior = append(prior, event)
		}
	}
	return prior
}

// anyUnobserved reports whether a window holds a call whose span never arrived.
//
// A discovery call is left out for the reason this file skips it everywhere
// else: the find tool dispatches no action, so a span that failed to arrive for
// one cannot have named a step, and counting it would leave a model that
// searched once unable to be told it missed a step.
func anyUnobserved(window []Event) bool {
	for _, event := range window {
		if !event.Discovery && !event.Observed {
			return true
		}
	}
	return false
}

// namesStep reports whether one call names one step, and by which reading.
//
// The requested reading is consulted only when the span named no action, which
// is the precedence stated at the top of this file. A call whose span named a
// different action is not this step whatever the model asked for: that is the
// rewrite case, and reading past the dispatch there would credit both steps to
// one call.
func namesStep(event Event, step modelcorpus.Step) (MatchKind, bool) {
	if step.Standalone != "" {
		return MatchTool, namesTool(event, step.Standalone)
	}
	action := string(step.Action)
	if event.Dispatched != "" {
		return MatchDispatched, event.Dispatched == action
	}
	if event.Requested != "" && event.Requested == action {
		return MatchRequested, true
	}
	return "", false
}

// namesTool reports whether a call named one tool registered outside the
// catalog. The server's own word comes first here too: a span naming the tool
// is what ran, and the name the model typed is read only when no span arrived.
func namesTool(event Event, tool string) bool {
	if event.DispatchedTool != "" {
		return event.DispatchedTool == tool
	}
	return event.Tool == tool
}
