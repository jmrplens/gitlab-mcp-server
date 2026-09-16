//go:build e2e

// call.go sends one call a model chose and writes down what happened.
//
// It is the seam between the two halves of this rebuild. Above it the turn
// loop knows about providers and prompts and nothing about MCP; below it the
// harness knows about servers and spans and nothing about models. What crosses
// is one call and one line of the record, and the line carries observation
// only: no verdict is written here, so a scoring rule can be corrected later
// and every past run re-scored without spending a token.
//
// Three of the things it records are the ones the evaluator this replaces got
// wrong. The values the model sent are kept, because whether a value was right
// is the question and comparing parameter names answered a different one. The
// server's own dispatch is kept beside the model's request, because a call
// that named one action and ran another proves nothing about the first. And
// the result is kept, because an argument of a later step bound to a field of
// an earlier answer can only be checked against what that answer actually was.

package modeleval

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// callPosition says where in one attempt a call happened.
//
// The index is part of it because it is what makes two identical calls two
// lines: a model that sends the same call twice is paying an overhead the
// record has to show, and without a position the second would be dropped as a
// duplicate of the first.
type callPosition struct {
	// Attempt is the attempt line this call belongs to.
	Attempt string
	// Turn is the provider turn it was made in.
	Turn int
	// Index is its position among the calls of the attempt, from 1.
	Index int
}

// callAsModel sends one call and returns the answer with the line that records
// it.
//
// The answer goes back to the turn loop, which has to hand the model what the
// server said; the line goes to the record. Both come from one send, so what a
// scorer reads months later and what the model read at the time can never be
// two different things.
func callAsModel(
	ctx context.Context,
	session *harness.Session,
	at callPosition,
	call harness.ModelCall,
	requested string,
) (harness.ModelAnswer, *modelrecord.Call) {
	answer := session.CallAsModel(ctx, call)
	return answer, callLine(at, call, requested, answer)
}

// callLine turns one call and its answer into the record's line.
//
// requested is the canonical action the call names, as far as the runner could
// tell. It is passed in rather than derived here because the reading that
// counts is the scorer's, over the whole catalog at the row's tier rather than
// over the set this session happens to serve: an action a protective mode
// removed is absent from the served catalog, so a session-side reading would
// report the one call a read-only row most needs named as naming nothing.
func callLine(
	at callPosition,
	call harness.ModelCall,
	requested string,
	answer harness.ModelAnswer,
) *modelrecord.Call {
	result, resultTruncated := modelrecord.CapResult(structuredResult(answer.Result))
	text, textTruncated := modelrecord.CapText(answerText(answer))

	return &modelrecord.Call{
		Attempt:          at.Attempt,
		Turn:             at.Turn,
		Index:            at.Index,
		Tool:             call.Tool,
		Arguments:        call.Arguments,
		RequestedAction:  requested,
		DispatchedAction: answer.Dispatch.Action,
		DispatchedTool:   answer.Dispatch.Tool,
		RefusalReason:    answer.Dispatch.RefusalReason,
		Outcome:          answer.Outcome,
		Result:           result,
		ResultTruncated:  resultTruncated,
		Text:             text,
		TextTruncated:    textTruncated,
		DurationMS:       float64(answer.Duration.Microseconds()) / 1000,
		TraceID:          answer.TraceID,
		Requests:         answer.Requests,
		DispatchObserved: answer.DispatchObserved,
	}
}

// structuredResult returns the structured content of an answer as JSON, or
// nothing when there was none.
//
// A result that cannot be encoded is recorded as absent rather than as an
// error of its own: it reached the model all the same, the text beside it is
// what the model read, and a line that failed to write would lose the call
// entirely.
func structuredResult(result *mcp.CallToolResult) json.RawMessage {
	if result == nil || result.StructuredContent == nil {
		return nil
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil
	}
	return encoded
}

// answerText returns what the model actually read.
//
// Two answers are not what the harness's own text field holds, and both
// matter. A safe-mode preview has that field replaced with a summary of the
// mutation that did not happen, so the card the model read is recovered from
// the result's own content; and a JSON-RPC error carries no result at all, so
// what the model was handed is the error's message, which is the only text an
// unregistered tool name is refused with.
func answerText(answer harness.ModelAnswer) string {
	if answer.Failure == harness.FailureSafeMode {
		if text := resultText(answer.Result); text != "" {
			return text
		}
	}
	if answer.Text != "" {
		return answer.Text
	}
	if answer.Err != nil {
		return answer.Err.Error()
	}
	return ""
}

// resultText returns the first text block of a result.
func resultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			return text.Text
		}
	}
	return ""
}
