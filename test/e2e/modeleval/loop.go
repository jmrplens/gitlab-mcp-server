//go:build e2e

// loop.go is the conversation: contract and prompt out, tool calls back,
// each one sent to the real server and its answer fed to the next turn.
//
// Everything it decides is an ending. It never repairs a malformed tool call,
// never re-asks a turn the model answered badly, never tells a model that a
// parameter name was wrong, and never tries a second spelling of a call the
// server refused. Each of those is a hint the model did not earn, and the
// evaluator this replaces did all four: it rewrote arguments, retried with
// corrections, and published the corrected trajectory as the model's. What is
// retried here is the request, and only when the provider itself would not
// serve it.
//
// A turn with no tool call ends the attempt and is recorded as such, with no
// verdict attached. On a read-only surface that ending is the correct answer
// and on every other one it is a failure, which is a question for the scorer;
// the record says what happened.

package modeleval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// The turn cap: how long one attempt's conversation may go on.
//
// It is a multiple of the case's own steps plus a margin, and the multiple is
// larger on dynamic because reaching one catalog action there is two calls,
// find and execute. The margin is what a model may spend on a retry after a
// refusal and on the closing turn that answers in text. Past it the attempt
// ends over_budget, which is a verdict about the model rather than a failure
// of the harness: a model that has not finished in three times the work the
// case asks for is not about to.
const (
	// turnsPerStepDynamic is find, execute and one retry.
	turnsPerStepDynamic = 3
	// turnsPerStepDirect is the call and one retry, on the two surfaces where
	// a tool is named directly.
	turnsPerStepDirect = 2
	// turnMargin is the slack every attempt gets on top.
	turnMargin = 4
)

// providerTries is how many times one request is sent before the attempt is
// given up as a provider error.
//
// Three, and only for a rate limit or a server error: a request the provider
// rejected as malformed is this side's fault and sending it again would spend
// the budget twice to learn the same thing.
const providerTries = 3

// providerBackoff is how long the runner waits before trying a refused request
// again.
//
// It is a variable so a test can drive the retry path without waiting, which is
// the only way the second and third tries are reachable offline. It is not
// configurable: a run's own patience is not a measurement.
var providerBackoff = func(try int) time.Duration {
	return time.Duration(try*try) * time.Second
}

// turnCap returns how many provider requests one attempt may make.
func turnCap(steps int, surface harness.Surface) int {
	perStep := turnsPerStepDirect
	if surface == harness.SurfaceDynamic {
		perStep = turnsPerStepDynamic
	}
	if steps < 1 {
		steps = 1
	}
	return perStep*steps + turnMargin
}

// conversation is one attempt: one model, one case, one surface, one session.
type conversation struct {
	// adapter is the model, already built and credentialed.
	adapter provider.Provider
	// dispatch sends one call the model chose and returns what happened.
	//
	// It is a function rather than the session itself, and the reason is what
	// this file is: the loop's job is to decide how an attempt ends, and every
	// one of those endings should be reachable from a test without a GitLab, a
	// server child and a fixture. The runner binds it to the session; a test
	// binds it to a table of answers.
	dispatch func(context.Context, callPosition, harness.ModelCall, string) (harness.ModelAnswer, *modelrecord.Call)
	// tools is the served tool list as the model is shown it.
	tools []provider.Tool
	// contract is the surface's own introduction.
	contract string
	// stimulus is the case as it was put.
	stimulus stimulus
	// caseID is the corpus case, which the fake replays and nothing else
	// reads.
	caseID string
	// surface is the surface the session serves.
	surface harness.Surface
	// attempt is the identifier every line of this attempt carries.
	attempt string
	// cap is the turn cap computed for this case and surface.
	cap int
	// requested resolves the canonical action a call names, over the whole
	// catalog rather than the served one.
	requested func(tool string, arguments json.RawMessage) string
	// charge is told what each turn was billed for, so a run can stop at its
	// budget. Nil when there is no budget.
	charge func(modelrecord.Usage)
}

// conversationResult is what one attempt produced: how it ended and every line
// it wrote.
type conversationResult struct {
	// EndedBy is one of the modelrecord Ended* constants.
	EndedBy string
	// Reason says why, empty on a completed attempt.
	Reason string
	// Turns are the provider requests, in the order they were made.
	Turns []*modelrecord.Turn
	// Calls are the tools/call lines, in the order they were sent.
	Calls []*modelrecord.Call
}

// run drives the conversation to one of its endings.
func (c *conversation) run(ctx context.Context) conversationResult {
	result := conversationResult{EndedBy: modelrecord.EndedCompleted}
	messages := []provider.Message{{Role: provider.RoleUser, Text: c.stimulus.Text}}
	// The structured content of every call made so far, oldest first. It
	// travels beside the conversation rather than in it, because what the
	// conversation carries is the text a model reads and a key that binds one
	// step's argument to a field of an earlier answer names the structured
	// content next to it. The runner holds both; a model holds one.
	var produced []json.RawMessage

	for index := 1; index <= c.cap; index++ {
		response, turns, err := c.ask(ctx, index, messages, produced)
		result.Turns = append(result.Turns, turns...)
		if err != nil {
			result.EndedBy = modelrecord.EndedProviderError
			result.Reason = err.Error()
			return result
		}
		if response.Malformed() {
			result.EndedBy = modelrecord.EndedMalformed
			result.Reason = "the model emitted a tool call whose arguments are not JSON"
			return result
		}

		calls := response.ToolCalls()
		if len(calls) == 0 {
			result.EndedBy = endedInProse(len(result.Calls))
			result.Reason = spokenText(response.Blocks)
			return result
		}

		messages = append(messages, assistantTurn(response))
		answers := make([]provider.ToolResult, 0, len(calls))
		for _, call := range calls {
			line, answer := c.send(ctx, index, len(result.Calls)+1, call)
			result.Calls = append(result.Calls, line)
			produced = append(produced, line.Result)
			answers = append(answers, toolResultOf(call, answer))
		}
		messages = append(messages, toolTurn(answers))
	}

	result.EndedBy = modelrecord.EndedOverBudget
	result.Reason = fmt.Sprintf("the conversation reached its cap of %d turns", c.cap)
	return result
}

// ask sends one turn, retrying a request the provider itself would not serve.
//
// Every try is a turn line, which is what keeps a latency or a token figure
// honest: a request that succeeded on the third try cost three requests, and a
// record holding only the one that worked would say it cost one.
func (c *conversation) ask(
	ctx context.Context,
	index int,
	messages []provider.Message,
	produced []json.RawMessage,
) (provider.Response, []*modelrecord.Turn, error) {
	request := provider.Request{
		Tools:    c.tools,
		System:   c.contract,
		Messages: messages,
		Replay: provider.Replay{
			Case:     c.caseID,
			Surface:  string(c.surface),
			Facts:    c.stimulus.Facts,
			Produced: produced,
		},
	}

	var turns []*modelrecord.Turn
	var lastErr error
	for try := 1; try <= providerTries; try++ {
		response, err := c.adapter.Call(ctx, request)
		turns = append(turns, turnLine(c.attempt, index, try, response))
		if c.charge != nil {
			c.charge(response.Usage)
		}
		if err == nil {
			return response, turns, nil
		}
		lastErr = err
		if !response.Retryable() || try == providerTries {
			return response, turns, err
		}
		select {
		case <-ctx.Done():
			return response, turns, ctx.Err()
		case <-time.After(providerBackoff(try)):
		}
	}
	return provider.Response{}, turns, lastErr
}

// send makes one call the model chose and records what the server did with it.
func (c *conversation) send(
	ctx context.Context,
	turn, index int,
	call modelrecord.Block,
) (*modelrecord.Call, harness.ModelAnswer) {
	answer, line := c.dispatch(ctx,
		callPosition{Attempt: c.attempt, Turn: turn, Index: index},
		harness.ModelCall{Tool: call.Tool, Arguments: call.Arguments},
		c.requested(call.Tool, call.Arguments))
	return line, answer
}

// sendThrough binds a conversation's dispatch to one session.
func sendThrough(
	session *harness.Session,
) func(context.Context, callPosition, harness.ModelCall, string) (harness.ModelAnswer, *modelrecord.Call) {
	return func(
		ctx context.Context,
		at callPosition,
		call harness.ModelCall,
		requested string,
	) (harness.ModelAnswer, *modelrecord.Call) {
		return callAsModel(ctx, session, at, call, requested)
	}
}

// turnLine writes down one provider request and its answer.
func turnLine(attempt string, index, try int, response provider.Response) *modelrecord.Turn {
	return &modelrecord.Turn{
		Attempt:       attempt,
		Index:         index,
		Try:           try,
		RequestDigest: response.RequestDigest,
		Blocks:        response.Blocks,
		Usage:         response.Usage,
		Status:        response.Status,
		Detail:        response.Detail,
		LatencyMS:     response.LatencyMS,
	}
}

// assistantTurn is the model's own turn, fed back so the next request carries
// it.
//
// The provider's own rendering goes back where there is one: an assistant turn
// carries more than the record keeps, and a provider that signed a thinking
// block or a function call demands that signature back on the following
// request. Rebuilding the turn from the blocks would drop it, and the failure
// would land on the second turn.
func assistantTurn(response provider.Response) provider.Message {
	return provider.Message{
		Role:   provider.RoleAssistant,
		Blocks: response.Blocks,
		Echo:   response.Echo,
	}
}

// toolTurn is what the server answered, as one message.
func toolTurn(results []provider.ToolResult) provider.Message {
	return provider.Message{Role: provider.RoleTool, Results: results}
}

// toolResultOf is what the model is handed back for one call.
//
// The text is the harness's reading of the answer rather than the record's,
// because the record's is capped: the model reads what the server wrote and a
// scorer reads the head of it, and handing the model the capped copy would
// make the cap part of the measurement.
//
// A call the server did not answer with a result is fed back as an error, so
// the model can see it failed. That includes a refusal, which is the one thing
// a read-only surface most wants a model to notice.
func toolResultOf(call modelrecord.Block, answer harness.ModelAnswer) provider.ToolResult {
	text := answerText(answer)
	if strings.TrimSpace(text) == "" {
		text = "The server answered with no text."
	}
	return provider.ToolResult{
		CallID:  call.CallID,
		Tool:    call.Tool,
		Content: text,
		IsError: answer.Outcome != modelrecord.OutcomeOK && answer.Outcome != modelrecord.OutcomePreview,
	}
}

// endedInProse says how an attempt that finished in text is written down.
//
// The loop cannot tell whether the task was done, because it cannot read the
// answer: what it can tell is whether the model ever called anything. A model
// that worked and then said what it had done ended its own conversation, which
// is completed; a model that answered in text having called nothing answered
// instead of acting, which is the other ending and the one a read-only
// deployment wants.
//
// Neither is a verdict. The scorer reads both endings and settles the question
// from the turns, because the two are the same event seen from two distances,
// and whether prose was the right answer is a question about the mode the
// session ran in rather than about the conversation.
func endedInProse(calls int) string {
	if calls > 0 {
		return modelrecord.EndedCompleted
	}
	return modelrecord.EndedNoToolCall
}

// spokenText returns the prose of an answer, which is what a turn with no tool
// call says.
func spokenText(blocks []modelrecord.Block) string {
	for _, block := range blocks {
		if block.Kind == modelrecord.BlockText && strings.TrimSpace(block.Text) != "" {
			return block.Text
		}
	}
	return ""
}

// costOf is what one usage costs at one price, in US dollars.
//
// Four numbers rather than one, because a cache read is an order of magnitude
// cheaper than the write that created it and a single figure over a cached
// conversation reads as a model being frugal when it is being repeated.
func costOf(usage modelrecord.Usage, price modelrecord.Price) float64 {
	const perMillion = 1_000_000.0
	return (float64(usage.Input)*price.InputPerMillionUSD +
		float64(usage.Output)*price.OutputPerMillionUSD +
		float64(usage.CacheCreated)*price.CacheWritePerMillionUSD +
		float64(usage.CacheRead)*price.CacheReadPerMillionUSD) / perMillion
}

// budget is what the run has spent and what it may.
//
// It is computed from the record's own usage as the run goes rather than
// estimated in advance, because a conversation's length is the thing being
// measured: an estimate would have to assume a number of turns, and a model
// that takes twice as many is exactly the outcome a budget exists to bound.
type budget struct {
	// limit is the ceiling in US dollars, zero for none.
	limit float64

	mu sync.Mutex
	// spent is what the usage recorded so far comes to.
	spent float64
}

// newBudget returns the accountant for one run.
func newBudget(limit float64) *budget { return &budget{limit: limit} }

// charge adds one request's usage at one model's price.
//
// A model with no price charges nothing, which is what MODELEVAL_UNPRICED asks
// for: a run allowed to start without a price cannot be stopped by a budget,
// and pretending otherwise would stop it at the wrong moment.
func (b *budget) charge(usage modelrecord.Usage, price *modelrecord.Price) {
	if price == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spent += costOf(usage, *price)
}

// Spent returns what the run has cost so far.
func (b *budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// Exhausted reports whether the ceiling has been reached.
//
// The check is before an attempt and never in the middle of one: a
// conversation stopped halfway is an attempt nobody can score, so the last
// attempt a budget allows is allowed to finish and the ceiling is a floor on
// what the run costs rather than a ceiling on it.
func (b *budget) Exhausted() bool {
	if b.limit <= 0 {
		return false
	}
	return b.Spent() >= b.limit
}
