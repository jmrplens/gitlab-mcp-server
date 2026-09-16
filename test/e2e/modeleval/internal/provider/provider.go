//go:build e2e

// provider.go is the contract every adapter answers: one request in, one
// response out, in the vocabulary the observation record already uses.
//
// The blocks and the usage are modelrecord's own types rather than copies of
// them. A second vocabulary here would have to be translated on the way to the
// record, and a translation is where two readings of one answer come from; the
// record is the thing a verdict is computed from months later, so it is the
// thing an adapter returns.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// The adapters this package has, named as a spec names them.
const (
	// Anthropic is the Messages API.
	Anthropic = "anthropic"
	// OpenAI is the chat completions API.
	OpenAI = "openai"
	// Qwen is DashScope's OpenAI-compatible chat completions API.
	Qwen = "qwen"
	// Google is the Gemini generateContent API.
	Google = "google"
	// Fake is the adapter that talks to nobody and replays a corpus key.
	Fake = "fake"
)

// The roles a message carries. They are the three the conversation actually
// has, and each adapter spells them its own way: a tool result is a user
// message carrying tool_result blocks on Anthropic, one message per result with
// the tool role on the OpenAI-compatible APIs, and a user content carrying
// functionResponse parts on Gemini.
const (
	// RoleUser is what the runner asked.
	RoleUser = "user"
	// RoleAssistant is what the model answered, as blocks.
	RoleAssistant = "assistant"
	// RoleTool is what the server answered a tool call with.
	RoleTool = "tool"
)

// defaultMaxTokens bounds one answer when a spec names no ceiling.
//
// It is generous rather than tight, for a reason that is about measurement and
// not about caution: an answer cut off at the ceiling reaches the record as a
// malformed or absent tool call, so a ceiling set too low makes a model look
// worse than it is, which is the one misreading this whole record exists to
// prevent. A reasoning model spends most of a turn thinking and is billed for
// that inside this same ceiling, so the figure has to leave room for a turn
// that is mostly reasoning. Output tokens are billed as used rather than as
// allowed, so the headroom costs nothing when it is not taken.
const defaultMaxTokens = 8192

// maxResponseBytes bounds what is read back from a provider, so a runaway
// answer cannot exhaust the machine running the evaluation.
const maxResponseBytes = 8 << 20

// Tool is one tool as every adapter sends it.
//
// The schema is normalized once by [NewTool] and is then sent verbatim by all
// four adapters. That is what lets the four digests of one tool list agree, and
// an adapter that rewrites a schema of its own is the defect the digest exists
// to show.
type Tool struct {
	// Name is the tool name the model calls.
	Name string
	// Description is what the server published for it.
	Description string
	// Schema is the input schema, normalized.
	Schema json.RawMessage
}

// ToolResult is what one tools/call answered, as the model reads it.
type ToolResult struct {
	// CallID is the identifier the model's own tool call carried, which is
	// how every provider pairs a result with its call.
	CallID string
	// Tool is the tool that was called, which Gemini needs by name in the
	// response part and the others do not.
	Tool string
	// Content is the text the model reads.
	Content string
	// IsError says the call was answered with an error, which a provider
	// marks so the model can see it failed.
	IsError bool
}

// Message is one turn of the conversation, in a shape no provider uses.
//
// It is neutral on purpose: the adapters differ most in how a tool result is
// fed back, and that difference is the one place an adapter breaks on the
// second turn, so it is expressed once here and translated four times rather
// than the other way round.
type Message struct {
	// Role is one of [RoleUser], [RoleAssistant] and [RoleTool].
	Role string
	// Text is prose, on a user or assistant message.
	Text string
	// Blocks are what the model emitted on an assistant message, which is
	// fed back so a provider that requires its own tool-call identifiers
	// gets them.
	Blocks []modelrecord.Block
	// Echo is the provider's own rendering of this assistant turn, from
	// [Response.Echo], and is preferred over Blocks when it is there.
	//
	// It exists because an assistant turn carries more than the record
	// keeps, and a provider refuses the next turn without it: Gemini signs a
	// thinking model's function call with a thoughtSignature that must come
	// back on the following request, and Anthropic signs a thinking block
	// the same way. Rebuilding the turn from the blocks would drop both, and
	// the failure lands on the second turn, which is the turn the old
	// evaluator's adapters were never tested on.
	//
	// It travels as the bytes the provider sent and goes back out as those
	// same bytes, through [wireValue]. See there for why a re-marshal is not
	// good enough.
	Echo json.RawMessage
	// Results are the tool results a [RoleTool] message carries.
	Results []ToolResult
}

// wireValue is one piece of a request: the bytes a provider returned, when the
// piece is that provider's own answer handed back, and a value this package
// built otherwise.
//
// It exists because re-marshaling an echo is not echoing it. A turn rebuilt
// through a struct written here carries the fields that struct has room for and
// silently drops the rest, and the dropped ones are exactly what a provider
// signs and then demands back: the signature of an Anthropic thinking block,
// the thought marker of a Gemini part, whatever an OpenAI-compatible endpoint
// attaches to an assistant message. Nothing local fails when one goes missing.
// The request is accepted, the provider refuses the turn after it, and the
// adapter looks correct in every test that drives one turn.
//
// A value this package assembled is carried as the value rather than as bytes,
// so a marshaling failure fails the whole request the way it always did,
// instead of dropping one turn out of a conversation and sending the rest.
type wireValue struct {
	// raw is what a provider sent, written out verbatim when it is there.
	raw json.RawMessage
	// built is what this package assembled, marshaled when raw is empty.
	built any
}

// wireRaw carries bytes a provider returned.
func wireRaw(raw json.RawMessage) wireValue { return wireValue{raw: raw} }

// wireBuilt carries a value this package assembled.
func wireBuilt(built any) wireValue { return wireValue{built: built} }

// MarshalJSON writes the provider's own bytes when this value has them.
func (v wireValue) MarshalJSON() ([]byte, error) {
	if len(v.raw) > 0 {
		return v.raw, nil
	}
	return json.Marshal(v.built)
}

// Replay is what the fake needs and no real adapter ever reads.
//
// It is on the request rather than on the fake's constructor because the fake
// is a [Provider] like the others and the runner's loop must not have to know
// which one it holds. Nothing in it is an answer: the case identifier and the
// facts are both halves of the stimulus the runner already has, and the fake
// looks the key up itself, which is why the runner stays unable to read one.
type Replay struct {
	// Case is the corpus case the attempt is running.
	Case string
	// Surface is the surface the session serves, which decides how the fake
	// spells a call.
	Surface string
	// Facts are the fixture values the stimulus was rendered with.
	Facts map[string]string
	// Produced is the structured result of each call this attempt has
	// already made, oldest first.
	//
	// It is here because a key may bind one step's argument to a field of an
	// earlier step's answer, and what the conversation carries is the text
	// the model reads rather than the structured content beside it. The
	// runner holds both; a model holds one.
	Produced []json.RawMessage
}

// Request is one exchange with a model.
type Request struct {
	// Tools is the served tool list, normalized by [NewTool].
	Tools []Tool
	// System is the surface contract.
	System string
	// Messages is the conversation so far, oldest first.
	Messages []Message
	// Replay is read by the fake and by nothing else.
	Replay Replay
}

// Response is what one request produced, whether or not it succeeded.
//
// It is returned beside an error rather than instead of one: a failed request
// is a turn the record keeps, with its status, its detail and its bodies, and a
// runner that only saw the error would have nothing to write down about the
// three quarters of a rate-limited retry sequence that failed.
type Response struct {
	// Blocks are what the model emitted.
	Blocks []modelrecord.Block
	// Echo is this turn in the provider's own shape, to be handed back as
	// [Message.Echo] on the next request. See that field for why.
	Echo json.RawMessage
	// Usage is what the request was billed for.
	Usage modelrecord.Usage
	// Status is one of the modelrecord Turn* constants.
	Status string
	// Detail carries a failed request's message.
	Detail string
	// RequestDigest is the digest of the request body as sent.
	RequestDigest string
	// RequestBody is the body as sent, for the trace.
	RequestBody json.RawMessage
	// ResponseBody is the body as received, for the trace. A body that is
	// not JSON is not kept here; the detail carries what it said.
	ResponseBody json.RawMessage
	// HTTPStatus is the provider's status code, zero when the request never
	// got one.
	HTTPStatus int
	// LatencyMS is how long the provider took.
	LatencyMS float64
}

// Malformed reports whether the model emitted a tool call whose arguments are
// not JSON.
//
// It is a reading of the blocks rather than a status of its own, because that
// is what it is: the provider answered, the request was fine, and the model
// wrote something no parser accepts. The attempt ends there and nothing is
// repaired.
func (r Response) Malformed() bool {
	return slices.ContainsFunc(r.Blocks, func(block modelrecord.Block) bool {
		return block.Kind == modelrecord.BlockToolCall && len(block.Arguments) == 0
	})
}

// ToolCalls returns the tool calls of an answer, in order.
func (r Response) ToolCalls() []modelrecord.Block {
	var calls []modelrecord.Block
	for _, block := range r.Blocks {
		if block.Kind == modelrecord.BlockToolCall {
			calls = append(calls, block)
		}
	}
	return calls
}

// Retryable reports whether a failed request is one the runner may send again.
//
// Rate and server errors are; a request the provider rejected as malformed is
// this side's fault and sending it again would only spend the budget twice.
func (r Response) Retryable() bool {
	return r.Status == modelrecord.TurnRateLimited || r.Status == modelrecord.TurnServerError
}

// Provider is one model, callable.
type Provider interface {
	// Call sends one request and returns what came back.
	Call(ctx context.Context, request Request) (Response, error)
	// Spec is what this provider was configured from.
	Spec() Spec
	// ToolDigest is the digest of a tool list as this adapter would send it,
	// which is what a published row carries so that a provider-specific
	// rewrite of the schemas cannot hide.
	ToolDigest(tools []Tool) string
}

// Config is what building an adapter needs.
type Config struct {
	// Spec is the parsed provider:model;key=value string.
	Spec Spec
	// APIKey is the credential, read by the caller through harness.Setting.
	APIKey string
	// Endpoint overrides the provider's own URL. It is what a test points at
	// an httptest server, and what a Qwen deployment in another region is
	// configured with.
	Endpoint string
	// HTTPClient is the client to send with. The default has a timeout,
	// because a provider that never answers would otherwise hold a run open
	// for as long as the process lives.
	HTTPClient *http.Client
}

// New builds the adapter one spec names.
//
// A real adapter with no credential is refused here rather than at the first
// request: a run that cannot authenticate should say so before it builds a
// world, not after.
func New(cfg Config) (Provider, error) {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	if cfg.Spec.Provider != Fake && strings.TrimSpace(cfg.APIKey) == "" {
		name, _ := KeyName(cfg.Spec.Provider)
		return nil, fmt.Errorf("provider %s has no credential: set %s in the environment or in .env",
			cfg.Spec.Provider, name)
	}

	built := func(fallback string) base {
		return base{
			spec:     cfg.Spec,
			key:      cfg.APIKey,
			client:   client,
			endpoint: orDefault(cfg.Endpoint, fallback),
		}
	}

	switch cfg.Spec.Provider {
	case Anthropic:
		return anthropicAdapter{built(anthropicEndpoint)}, nil
	case OpenAI:
		return openAIAdapter{base: built(openAIEndpoint), maxTokensField: openAIMaxTokensField}, nil
	case Qwen:
		return openAIAdapter{base: built(qwenEndpoint), maxTokensField: qwenMaxTokensField}, nil
	case Google:
		return googleAdapter{built(googleEndpoint)}, nil
	case Fake:
		return newFake(cfg.Spec)
	default:
		return nil, fmt.Errorf("unsupported provider %q, want one of %s",
			cfg.Spec.Provider, strings.Join(Names(), ", "))
	}
}

// Names returns the adapters this package has, in the order a refusal lists
// them.
func Names() []string { return []string{Anthropic, OpenAI, Qwen, Google, Fake} }

// KeyName returns the setting one provider's credential is read from, and
// whether that provider needs one at all.
func KeyName(provider string) (string, bool) {
	switch provider {
	case Anthropic:
		return "ANTHROPIC_API_KEY", true
	case OpenAI:
		return "OPENAI_API_KEY", true
	case Qwen:
		return "QWEN_API_KEY", true
	case Google:
		return "GOOGLE_API_KEY", true
	default:
		return "", false
	}
}

// orDefault returns the override when there is one.
func orDefault(override, fallback string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	return fallback
}

// base is what every HTTP adapter holds.
type base struct {
	spec     Spec
	key      string
	client   *http.Client
	endpoint string
}

// Spec returns what this adapter was configured from.
func (b base) Spec() Spec { return b.spec }

// exchange marshals one payload, sends it and reads the answer back.
//
// It is the whole of what the three HTTP adapters share, and it deliberately
// decides nothing: it classifies the status so the runner can tell a rate limit
// from a bad request, and it returns the body for the adapter to decode. No
// retry happens here, because a retry is a decision about an attempt and this
// package does not know what an attempt is.
func (b base) exchange(
	ctx context.Context,
	endpoint string,
	headers map[string]string,
	payload any,
) (Response, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{Status: modelrecord.TurnRequestError, Detail: err.Error()}, nil,
			fmt.Errorf("marshal the %s request: %w", b.spec.Provider, err)
	}
	answer := Response{
		Status:        modelrecord.TurnOK,
		RequestDigest: digestBytes(body),
		RequestBody:   body,
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		answer.Status = modelrecord.TurnRequestError
		answer.Detail = err.Error()
		return answer, nil, fmt.Errorf("build the %s request: %w", b.spec.Provider, err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	started := time.Now()
	// #nosec G704 -- the endpoint is this package's own constant or a value
	// the operator configured, never anything a model wrote.
	response, err := b.client.Do(request)
	answer.LatencyMS = float64(time.Since(started).Microseconds()) / 1000
	if err != nil {
		answer.Status = modelrecord.TurnTransportError
		answer.Detail = b.redact(err.Error())
		return answer, nil, fmt.Errorf("send the %s request: %w", b.spec.Provider, err)
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	answer.HTTPStatus = response.StatusCode
	if err != nil {
		answer.Status = modelrecord.TurnTransportError
		answer.Detail = b.redact(err.Error())
		return answer, nil, fmt.Errorf("read the %s answer: %w", b.spec.Provider, err)
	}
	if json.Valid(raw) {
		answer.ResponseBody = slices.Clone(raw)
	}
	if status, failed := failureStatus(response.StatusCode); failed {
		answer.Status = status
		answer.Detail = b.redact(string(raw))
		return answer, raw, fmt.Errorf("%s answered %d: %s",
			b.spec.Provider, response.StatusCode, answer.Detail)
	}
	return answer, raw, nil
}

// failureStatus classifies an HTTP status the way the record's turn statuses
// do, and reports whether it is a failure at all.
func failureStatus(code int) (string, bool) {
	switch {
	case code == http.StatusTooManyRequests:
		return modelrecord.TurnRateLimited, true
	case code >= http.StatusInternalServerError:
		return modelrecord.TurnServerError, true
	case code < http.StatusOK || code >= http.StatusMultipleChoices:
		return modelrecord.TurnRequestError, true
	default:
		return modelrecord.TurnOK, false
	}
}

// maxDetailBytes bounds what a failure message keeps of a provider's answer.
const maxDetailBytes = 2000

// redact bounds a provider's own message and takes the credential out of it.
//
// Both halves have happened: an authentication failure is answered with a
// message quoting the key that failed, and a provider that is having a bad day
// answers a 500 with a megabyte of HTML. The record is committed and read by
// people, so neither belongs in it.
func (b base) redact(detail string) string {
	if b.key != "" {
		detail = strings.ReplaceAll(detail, b.key, "REDACTED")
	}
	detail = strings.TrimSpace(detail)
	if len(detail) > maxDetailBytes {
		return detail[:maxDetailBytes] + "...(truncated)"
	}
	return detail
}

// failed returns one answer's failure as this package reports it: the response
// keeps its status and its detail, and the error says which adapter and why.
func (b base) failed(answer Response, status, detail string) (Response, error) {
	answer.Status = status
	answer.Detail = b.redact(detail)
	return answer, fmt.Errorf("%s: %s", b.spec.Provider, answer.Detail)
}

// NewTool normalizes one tool's schema into the form every adapter sends.
//
// One normalization, applied before any adapter sees the list, is what makes
// the four digests of one list agree. The alternative, letting each adapter
// massage the schema on its way out, is the arrangement that produced two
// published columns measuring a schema the other two never saw.
//
// What it removes is what Gemini's function-declaration schema has no room for
// and the other three treat as optional: the "$schema" dialect marker, "title",
// and "additionalProperties". A union type is collapsed to its first
// non-"null" member for the same reason. Removing an optional keyword from a
// schema cannot make a call the model would have made invalid; leaving it in
// makes Gemini refuse the request outright.
func NewTool(name, description string, schema json.RawMessage) (Tool, error) {
	if strings.TrimSpace(name) == "" {
		return Tool{}, fmt.Errorf("a tool with no name: %q", description)
	}
	normalized, err := normalizeSchema(schema)
	if err != nil {
		return Tool{}, fmt.Errorf("normalize the schema of %s: %w", name, err)
	}
	return Tool{Name: name, Description: description, Schema: normalized}, nil
}

// normalizeSchema returns one schema in the form every adapter sends.
//
// A schema that is absent becomes the empty object, because a tool without one
// still has to be declared as taking an object: Gemini and OpenAI both refuse a
// declaration whose parameters are null.
func normalizeSchema(schema json.RawMessage) (json.RawMessage, error) {
	if len(schema) == 0 {
		return json.RawMessage(`{"type":"object"}`), nil
	}
	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(normalizeValue(decoded))
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// droppedSchemaKeys are the keywords normalization removes wherever they
// appear.
var droppedSchemaKeys = map[string]bool{
	"$schema":              true,
	"title":                true,
	"additionalProperties": true,
}

// normalizeValue walks one decoded schema.
func normalizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if droppedSchemaKeys[key] {
				continue
			}
			if key == "type" {
				out[key] = normalizeType(child)
				continue
			}
			out[key] = normalizeValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, normalizeValue(child))
		}
		return out
	default:
		return value
	}
}

// normalizeType collapses a union type to the member that carries the value.
//
// A nullable field is spelled ["string","null"] by the schema generator and as
// a plain "string" by every provider's own schema dialect. The null member says
// the field may be omitted, which every one of these dialects expresses by the
// field not being required, so nothing is lost by dropping it.
func normalizeType(value any) any {
	members, isList := value.([]any)
	if !isList {
		return value
	}
	for _, member := range members {
		if name, isString := member.(string); isString && name != "null" {
			return name
		}
	}
	return value
}
