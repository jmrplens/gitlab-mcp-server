// Package elicitation: flow.go implements the multi round-trip request
// (MRTR, SEP-2322) elicitation flow required by MCP protocol version
// 2026-07-28, where server-initiated elicitation/create requests are
// forbidden while serving a tool call. A Flow transparently selects the
// right mechanism per session:
//
//   - Sessions negotiated at protocol >= 2026-07-28 receive an
//     InputRequests map in the tool result and retry the call with
//     InputResponses populated (handled automatically by SDK client
//     middleware). Answers gathered in earlier rounds are carried in the
//     opaque RequestState so multi-step flows survive handler re-invocation.
//   - Older sessions fall back to the synchronous [Client] path, which
//     issues elicitation/create requests directly.
package elicitation

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// minMRTRProtocolVersion is the first MCP protocol version that forbids
// server-initiated elicitation requests and requires the multi round-trip
// request flow instead (SEP-2322). Protocol versions are ISO dates, so
// lexicographic comparison is chronological.
const minMRTRProtocolVersion = "2026-07-28"

// flowStateVersion versions the RequestState encoding so future changes
// can reject or migrate stale client-echoed state.
const flowStateVersion = 1

// ErrInputPending is returned by [Flow] prompt methods when the answer is
// not yet available and an input request has been queued for the client.
// Handlers must stop and surface [Flow.InputRequiredResult] (or
// [Flow.PendingError] for handlers that return errors) so the client can
// fulfill the request and retry the call.
var ErrInputPending = errors.New("elicitation: input request pending client response")

// InputRequiredError carries an input-required tool result out of handlers
// that report failures through an error return. Surface wrappers unwrap it
// with errors.AsType and return the embedded result instead of an error.
type InputRequiredError struct {
	result *mcp.CallToolResult
}

// Error implements the error interface.
func (e *InputRequiredError) Error() string {
	return "elicitation: tool call requires client input (multi round-trip request)"
}

// Result returns the input-required tool result to send to the client.
func (e *InputRequiredError) Result() *mcp.CallToolResult {
	return e.result
}

// answerRecord stores one fulfilled elicitation exchange so later rounds
// of a multi-step flow can replay it without re-prompting the user.
type answerRecord struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// Flow performs elicitation exchanges for a single tool call, selecting
// the synchronous path for legacy sessions and the multi round-trip path
// for protocol >= 2026-07-28 sessions. Prompt methods take a stable id
// that identifies the exchange across handler re-invocations; ids must be
// unique within one tool call.
type Flow struct {
	legacy  Client
	mrtr    bool
	answers map[string]answerRecord
	pending mcp.InputRequestMap
	// digest identifies the call this flow belongs to, so answers given to
	// one call cannot be presented on another.
	digest string
}

// FlowFromRequest builds a Flow for the current tool call. For multi
// round-trip sessions it decodes the client-echoed RequestState and merges
// the InputResponses from the retried call. It returns an error when the
// echoed state is malformed.
func FlowFromRequest(req *mcp.CallToolRequest) (*Flow, error) {
	f := &Flow{legacy: FromRequest(req), answers: map[string]answerRecord{}}
	if req == nil || req.Session == nil {
		return f, nil
	}
	// Read the revision from the request rather than the session. From
	// 2026-07-28 a client states it per request and may arrive without ever
	// having handshaken, so the session-level value is whatever the first
	// request on that session happened to say. The accessor still falls back to
	// InitializeParams, which is where an older client's value lives.
	if req.ProtocolVersion() < minMRTRProtocolVersion {
		return f, nil
	}
	f.mrtr = true
	if req.Params == nil {
		return f, nil
	}
	f.digest = requestDigest(req.Params.Name, req.Params.Arguments)
	if state := req.Params.RequestState; state != "" {
		answers, err := decodeState(state, f.digest, time.Now())
		if err != nil {
			return nil, err
		}
		maps.Copy(f.answers, answers)
	}
	for id, resp := range req.Params.InputResponses {
		er, ok := resp.(*mcp.ElicitResult)
		if !ok {
			// This flow only ever queues elicitation requests, so any other
			// response type is client misbehavior. Silently skipping it would
			// re-queue the same input request forever; fail instead.
			return nil, fmt.Errorf("elicitation: input response %q has unexpected type %T (want *mcp.ElicitResult)", id, resp)
		}
		f.answers[id] = answerRecord{Action: er.Action, Content: er.Content}
	}
	return f, nil
}

// IsSupported reports whether the MCP client supports elicitation.
func (f *Flow) IsSupported() bool {
	return f.legacy.IsSupported()
}

// UsesMultiRoundTrip reports whether this flow uses the multi round-trip
// request mechanism (protocol >= 2026-07-28) instead of synchronous
// server-initiated elicitation.
func (f *Flow) UsesMultiRoundTrip() bool {
	return f.mrtr
}

// exchange resolves one elicitation exchange. On the legacy path it sends
// the request synchronously. On the multi round-trip path it returns the
// recorded answer when available, or queues an input request and returns
// [ErrInputPending].
func (f *Flow) exchange(ctx context.Context, id, message string, schema map[string]any) (map[string]any, error) {
	if !f.mrtr {
		return f.legacy.elicit(ctx, message, schema)
	}
	if rec, ok := f.answers[id]; ok {
		return contentForAction(rec.Action, rec.Content)
	}
	// "Servers MUST NOT send elicitation requests with modes that are not
	// supported by the client." An answer already given is honored above
	// regardless, since refusing to read it would discard the user's own input.
	// The legacy path needs no such check: there the SDK refuses the send.
	if !f.legacy.IsFormSupported() {
		return nil, ErrFormElicitationNotSupported
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.pending == nil {
		f.pending = mcp.InputRequestMap{}
	}
	f.pending[id] = &mcp.ElicitParams{Message: message, RequestedSchema: schema}
	return nil, ErrInputPending
}

// Confirm asks the user a yes/no question. See [Client.Confirm].
func (f *Flow) Confirm(ctx context.Context, id, message string) (bool, error) {
	if !f.IsSupported() {
		return false, ErrElicitationNotSupported
	}
	content, err := f.exchange(ctx, id, message, confirmSchema(message))
	if err != nil {
		return false, err
	}
	confirmed, wellFormed := parseConfirmContent(content)
	if !wellFormed {
		return false, ErrMalformedAnswer
	}
	return confirmed, nil
}

// PromptText asks the user for free-form text input. See [Client.PromptText].
func (f *Flow) PromptText(ctx context.Context, id, message, fieldName string) (string, error) {
	if !f.IsSupported() {
		return "", ErrElicitationNotSupported
	}
	if fieldName == "" {
		fieldName = "value"
	}
	content, err := f.exchange(ctx, id, message, textSchema(message, fieldName))
	if err != nil {
		return "", err
	}
	return parseTextContent(content, fieldName)
}

// SelectOne asks the user to pick one option from a list. See [Client.SelectOne].
func (f *Flow) SelectOne(ctx context.Context, id, message string, options []string) (string, error) {
	if !f.IsSupported() {
		return "", ErrElicitationNotSupported
	}
	if len(options) == 0 {
		return "", errors.New(errOptionsEmpty)
	}
	content, err := f.exchange(ctx, id, message, selectOneSchema(message, options))
	if err != nil {
		return "", err
	}
	return parseSelectOneContent(content, options)
}

// SelectMulti asks the user to pick one or more options from a list.
// See [Client.SelectMulti].
func (f *Flow) SelectMulti(ctx context.Context, id, message string, options []string, minItems, maxItems int) ([]string, error) {
	if !f.IsSupported() {
		return nil, ErrElicitationNotSupported
	}
	if len(options) == 0 {
		return nil, errors.New(errOptionsEmpty)
	}
	content, err := f.exchange(ctx, id, message, selectMultiSchema(message, options, minItems, maxItems))
	if err != nil {
		return nil, err
	}
	return parseSelectMultiContent(content, options, minItems, maxItems)
}

// SelectOneInt asks the user to pick one integer from a list of allowed
// values. See [Client.SelectOneInt].
func (f *Flow) SelectOneInt(ctx context.Context, id, message string, options []int) (int, error) {
	if !f.IsSupported() {
		return 0, ErrElicitationNotSupported
	}
	if len(options) == 0 {
		return 0, errors.New(errOptionsEmpty)
	}
	content, err := f.exchange(ctx, id, message, selectOneIntSchema(message, options))
	if err != nil {
		return 0, err
	}
	return parseSelectOneIntContent(content, options)
}

// PromptNumber asks the user for a numeric input within a range.
// See [Client.PromptNumber].
func (f *Flow) PromptNumber(ctx context.Context, id, message, fieldName string, minVal, maxVal float64) (float64, error) {
	if !f.IsSupported() {
		return 0, ErrElicitationNotSupported
	}
	if fieldName == "" {
		fieldName = "value"
	}
	content, err := f.exchange(ctx, id, message, numberSchema(message, fieldName, minVal, maxVal))
	if err != nil {
		return 0, err
	}
	return parseNumberContent(content, fieldName, minVal, maxVal)
}

// GatherData sends an arbitrary JSON Schema to the client and returns the
// user's response as a map. See [Client.GatherData].
func (f *Flow) GatherData(ctx context.Context, id, message string, schema map[string]any) (map[string]any, error) {
	if !f.IsSupported() {
		return nil, ErrElicitationNotSupported
	}
	return f.exchange(ctx, id, message, schema)
}

// ElicitURL sends a URL-mode elicitation request, directing the user to a
// GitLab page. See [Client.ElicitURL].
func (f *Flow) ElicitURL(ctx context.Context, id, gitlabBaseURL, targetURL, message string) error {
	if !f.mrtr {
		return f.legacy.ElicitURL(ctx, gitlabBaseURL, targetURL, message)
	}
	if !f.IsSupported() {
		return ErrElicitationNotSupported
	}
	if !f.legacy.IsURLSupported() {
		return ErrURLElicitationNotSupported
	}
	if err := validateGitLabURL(gitlabBaseURL, targetURL); err != nil {
		return err
	}
	if rec, ok := f.answers[id]; ok {
		_, err := contentForAction(rec.Action, rec.Content)
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.pending == nil {
		f.pending = mcp.InputRequestMap{}
	}
	// No elicitationId: the MRTR path only exists on protocol 2026-07-28
	// and later, whose schema removed the field (ElicitRequestURLParams is
	// mode/message/url only).
	f.pending[id] = &mcp.ElicitParams{
		Mode:    "url",
		Message: message,
		URL:     targetURL,
	}
	return ErrInputPending
}

// InputRequiredResult builds the tool result carrying the queued input
// requests plus the accumulated answers encoded as opaque RequestState.
// The result must be returned as-is, with no content added: the protocol
// forbids mixing content and inputRequests in one result.
func (f *Flow) InputRequiredResult() *mcp.CallToolResult {
	state, err := encodeState(f.answers, f.digest, time.Now())
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			IsError: true,
		}
	}
	return &mcp.CallToolResult{
		InputRequests: f.pending,
		RequestState:  state,
	}
}

// PendingError wraps [Flow.InputRequiredResult] in an [InputRequiredError]
// for handlers that report outcomes through an error return.
func (f *Flow) PendingError() error {
	return &InputRequiredError{result: f.InputRequiredResult()}
}
