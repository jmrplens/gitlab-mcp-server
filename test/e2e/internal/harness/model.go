//go:build e2e

// model.go is the one verb a language model's call goes out through.
//
// Every other verb in this package starts from a canonical action and spells
// it onto a surface, because a scenario knows what it wants to run. A model
// does not: it reads the tools the server published and writes a name and an
// argument object of its own, and whether that names anything is the
// measurement. So the call goes out exactly as the model wrote it, the way
// [Session.Raw] sends one, and what comes back is the server's own answer with
// the server's own account of what it did with it.
//
// The second half is what Raw cannot give. A raw call's dispatch is resolved
// at the flush, when the whole test's calls are reconciled against the spans
// at once; a conversation cannot wait that long, because what the server
// dispatched for one call is what the next turn is scored on and by the flush
// the conversation is over. This waits for that one span, under the same
// budget the flush waits under, and hands it back.

package harness

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// purposeModel is what a model's call is recorded under.
//
// It is unexported because no caller chooses it: [Session.CallAsModel] is the
// only thing that produces one, and the four purposes a caller may ask for are
// the ones [For] takes. The record's own spelling is exported, since
// cmd/audit_e2e_coverage has to recognize the word to refuse it credit.
const purposeModel Purpose = e2ecalls.PurposeModel

// ModelCall is one tools/call a model chose, in the model's own words.
type ModelCall struct {
	// Tool is the tool name the model named. It is sent whether or not the
	// session serves a tool by that name: naming one that does not exist is a
	// result, not a mistake to be corrected on the way out.
	Tool string
	// Arguments is the argument object the model produced, as JSON.
	//
	// The bytes travel to the server unchanged rather than through a map,
	// because the values are what is being measured: decoding to map[string]any
	// and re-encoding turns every number into a float64 and can spell a large
	// integer back out in scientific notation, which would be this harness
	// changing the call it is reporting on.
	//
	// The one cost is that the coverage record's argument-name list is empty
	// for these calls, since describeToolRequest reads a map. It is no loss:
	// a model call earns no coverage credit at all.
	Arguments json.RawMessage
}

// DispatchFacts is what the server's own span said about one call: the
// exported copy of dispatchRecord.
//
// It is the whole of what the span carries rather than the part any one reader
// wants, because the point of reading it is that it is the server's account
// and not ours. A refusal is the clearest case: the reason lives on the span
// alone, so a client that had only the result text would be guessing at it
// from the server's prose.
type DispatchFacts struct {
	// Tool is the tool the server saw named. A discovery call carries this and
	// no action, which is how a find is told from an execute on the server's
	// word rather than by matching a name here.
	Tool string
	// Action is the canonical route the dispatcher chose, after every alias
	// rewrite. It is empty for a discovery call, for a call the server refused
	// before dispatching, and on a surface where nothing calls RecordDispatch.
	Action string
	// Domain is the catalog domain of that action.
	Domain string
	// RefusalReason is why the server declined to run what it was asked for,
	// in the server's own vocabulary: unknown_action, invalid_params,
	// needs_confirmation, safe_mode.
	RefusalReason string
	// ErrorType is error.type on the span.
	ErrorType string
	// Status is the span's status code, as the protocol spells it.
	Status string
}

// ModelAnswer is everything one model call produced: what the client saw, and
// what the server said it did.
type ModelAnswer struct {
	// Result is the tool result, nil when the call never got one.
	Result *mcp.CallToolResult
	// Err is the transport or protocol error, nil when the server answered.
	// An unregistered tool name is refused here rather than in a result, which
	// is what naming a tool the surface does not serve looks like.
	Err error
	// Outcome is the class the answer falls into, in the record's vocabulary:
	// ok, preview, tool_error, protocol_error, transport_error, or a refusal
	// spelled with the server's reason.
	Outcome string
	// Failure is the harness's class for an unhappy answer, empty on success.
	// It is the same judgement as Outcome in a form a caller can switch on.
	Failure Failure
	// Text is what the model actually read: the first text block of the
	// result, which is where every refusal this server makes puts its reason.
	Text string
	// Duration is how long the call took.
	Duration time.Duration
	// TraceID is the trace the call was stamped with, and the id the server's
	// span was joined on. Empty when the request could carry no trace.
	TraceID string
	// Dispatch is what that span said. Read it only when DispatchObserved.
	Dispatch DispatchFacts
	// Requests is how many GitLab requests the handler made under this call,
	// counted from the client spans of the same trace. It is a floor: the
	// spans travel through a batching processor that drops silently when its
	// queue overflows.
	Requests int
	// DispatchObserved is whether the server's span arrived at all.
	//
	// It is the difference between a claim about what ran and a claim about
	// what was asked for, and it is false rather than absent so that a reader
	// has to answer it: every verdict derived from a call whose span never came
	// is about the model's request and nothing about the server.
	DispatchObserved bool
}

// CallAsModel sends one call a model chose and returns what happened, the
// server's own dispatch included.
//
// It fails nothing. Every other verb in this package asserts, because a
// scenario that asked for something and did not get it is a defect; here the
// refusal, the wrong tool name and the missing parameter are the measurement,
// and a harness that failed the test on one would be refusing to record the
// thing it was run to find out.
//
// It is a method on Session and not a free function because the surface has
// already done its work by the time it is called: the model read the tools the
// session published and chose among them, so there is nothing left to project.
func (s *Session) CallAsModel(ctx context.Context, call ModelCall) ModelAnswer {
	var traceID string
	// ExpectationAny and no action: the caller declares nothing about the
	// outcome, and there is no canonical action to declare, since what this
	// names is whatever the model typed. assertDispatch therefore has nothing
	// to compare and returns early on the empty action, which is what keeps a
	// model's call from failing the test that ran it.
	attributed := s.attribute(ctx, purposeModel, ExpectationAny, callAttribution{traceOut: &traceID})

	started := time.Now()
	result, err := s.conn.client().CallTool(attributed, &mcp.CallToolParams{
		Name:      call.Tool,
		Arguments: call.Arguments,
	})
	elapsed := time.Since(started)

	// The same classify the record and every assertion use, so the verdict a
	// scorer reads and the outcome the shard carries can never disagree about
	// what an answer was.
	answer := classify(result, err)

	facts, requests, observed := dispatchOf(traceID)
	return ModelAnswer{
		Result:           answer.result,
		Err:              answer.err,
		Outcome:          answer.outcome,
		Failure:          answer.failure,
		Text:             answer.text,
		Duration:         elapsed,
		TraceID:          traceID,
		Dispatch:         facts,
		Requests:         requests,
		DispatchObserved: observed,
	}
}

// dispatchOf waits for one call's span and reports what it said.
//
// The wait is awaitTraces, which is the flush's own, so a run whose telemetry
// never arrives gives up here on the same terms and after the same one full
// budget rather than paying ten seconds a call for the length of a
// conversation.
func dispatchOf(traceID string) (facts DispatchFacts, requests int, observed bool) {
	if traceID == "" {
		return DispatchFacts{}, 0, false
	}
	received := receiverIfStarted()
	if received == nil {
		return DispatchFacts{}, 0, false
	}

	awaitTraces([]string{traceID})
	kept, arrived := received.lookup(traceID)
	if !arrived {
		return DispatchFacts{}, 0, false
	}
	return DispatchFacts{
		Tool:          kept.dispatch.tool,
		Action:        kept.dispatch.action,
		Domain:        kept.dispatch.domain,
		RefusalReason: kept.dispatch.refusalReason,
		ErrorType:     kept.dispatch.errorType,
		Status:        kept.dispatch.status,
	}, kept.requests, true
}
