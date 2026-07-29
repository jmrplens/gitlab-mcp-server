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
	"encoding/json"
	"errors"
	"fmt"
	"maps"

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

// flowState is the JSON payload round-tripped through the opaque
// RequestState field between rounds of a multi round-trip flow.
type flowState struct {
	Version int                     `json:"v"`
	Answers map[string]answerRecord `json:"answers,omitempty"`
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
	iparams := req.Session.InitializeParams()
	if iparams == nil || iparams.ProtocolVersion < minMRTRProtocolVersion {
		return f, nil
	}
	f.mrtr = true
	if req.Params == nil {
		return f, nil
	}
	if state := req.Params.RequestState; state != "" {
		var st flowState
		if err := json.Unmarshal([]byte(state), &st); err != nil {
			return nil, fmt.Errorf("elicitation: invalid requestState: %w", err)
		}
		if st.Version != flowStateVersion {
			return nil, fmt.Errorf("elicitation: unsupported requestState version %d", st.Version)
		}
		maps.Copy(f.answers, st.Answers)
	}
	for id, resp := range req.Params.InputResponses {
		er, ok := resp.(*mcp.ElicitResult)
		if !ok {
			continue
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
	return parseConfirmContent(content), nil
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
	return parseSelectMultiContent(content, options)
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
	return parseNumberContent(content, fieldName)
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
	elicitationID, err := newElicitationID()
	if err != nil {
		return fmt.Errorf("elicitation: failed to generate elicitationId: %w", err)
	}
	if f.pending == nil {
		f.pending = mcp.InputRequestMap{}
	}
	f.pending[id] = &mcp.ElicitParams{
		Mode:          "url",
		Message:       message,
		URL:           targetURL,
		ElicitationID: elicitationID,
	}
	return ErrInputPending
}

// InputRequiredResult builds the tool result carrying the queued input
// requests plus the accumulated answers encoded as opaque RequestState.
// The result must be returned as-is, with no content added: the protocol
// forbids mixing content and inputRequests in one result.
func (f *Flow) InputRequiredResult() *mcp.CallToolResult {
	state, err := json.Marshal(flowState{Version: flowStateVersion, Answers: f.answers})
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("elicitation: failed to encode request state: %v", err)}},
			IsError: true,
		}
	}
	return &mcp.CallToolResult{
		InputRequests: f.pending,
		RequestState:  string(state),
	}
}

// PendingError wraps [Flow.InputRequiredResult] in an [InputRequiredError]
// for handlers that report outcomes through an error return.
func (f *Flow) PendingError() error {
	return &InputRequiredError{result: f.InputRequiredResult()}
}
