// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// testKey is the credential every adapter test is built with. It is not a
// secret: nothing here leaves the process.
const testKey = "test-key-not-a-credential"

// recorder is an httptest backend that answers with canned bodies and keeps
// every request it was sent.
//
// It records rather than asserts because it runs on the server's own goroutine,
// where a failed assertion may not abort: the test reads the bodies back
// afterwards, on its own goroutine, which is the contract
// .github/instructions/test-goroutines.instructions.md states.
type recorder struct {
	mu      sync.Mutex
	bodies  []json.RawMessage
	paths   []string
	headers []http.Header

	answers []string
	status  []int
}

// newRecorder answers each request with the next body, repeating the last.
func newRecorder(answers ...string) *recorder {
	return &recorder{answers: answers}
}

// withStatus sets the status codes to answer with, repeating the last.
func (r *recorder) withStatus(codes ...int) *recorder {
	r.status = codes
	return r
}

// server starts the backend and returns its URL.
func (r *recorder) server(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := readAll(request)
		r.mu.Lock()
		index := len(r.bodies)
		r.bodies = append(r.bodies, body)
		r.paths = append(r.paths, request.URL.Path)
		r.headers = append(r.headers, request.Header.Clone())
		answer := at(r.answers, index, "{}")
		code := atInt(r.status, index, http.StatusOK)
		r.mu.Unlock()

		if err != nil {
			t.Errorf("reading the request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// sent returns the nth body the backend received, decoded into a map.
func (r *recorder) sent(t *testing.T, index int) map[string]any {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if index >= len(r.bodies) {
		t.Fatalf("the backend saw %d requests, want at least %d", len(r.bodies), index+1)
	}
	var decoded map[string]any
	if err := json.Unmarshal(r.bodies[index], &decoded); err != nil {
		t.Fatalf("request %d is not a JSON object: %v", index, err)
	}
	return decoded
}

// count returns how many requests the backend saw.
func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bodies)
}

// readAll reads one request body.
func readAll(request *http.Request) (json.RawMessage, error) {
	defer func() { _ = request.Body.Close() }()
	return io.ReadAll(request.Body)
}

// at returns the element at index, or the last one, or a fallback.
func at(values []string, index int, fallback string) string {
	switch {
	case len(values) == 0:
		return fallback
	case index < len(values):
		return values[index]
	default:
		return values[len(values)-1]
	}
}

// atInt is at for status codes.
func atInt(values []int, index, fallback int) int {
	switch {
	case len(values) == 0:
		return fallback
	case index < len(values):
		return values[index]
	default:
		return values[len(values)-1]
	}
}

// adapterFor builds one adapter against a backend.
func adapterFor(t *testing.T, raw, endpoint string) Provider {
	t.Helper()
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	built, err := New(Config{Spec: spec, APIKey: testKey, Endpoint: endpoint})
	if err != nil {
		t.Fatalf("building %q: %v", raw, err)
	}
	return built
}

// sampleTools is one tool list, carrying the schema shapes the normalization
// has an opinion about.
func sampleTools(t *testing.T) []Tool {
	t.Helper()
	find, err := NewTool("gitlab_find_action", "Find an action.", json.RawMessage(
		`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","title":"find",
		  "properties":{"query":{"type":"string","maxLength":256}},"required":["query"],
		  "additionalProperties":false}`,
	))
	if err != nil {
		t.Fatalf("building the find tool: %v", err)
	}
	execute, err := NewTool("gitlab_execute_action", "Execute an action.", json.RawMessage(
		`{"type":"object","properties":{"action":{"type":"string"},
		  "params":{"type":["object","null"],"additionalProperties":true},
		  "confirm":{"type":"boolean"}},"required":["action","params"]}`,
	))
	if err != nil {
		t.Fatalf("building the execute tool: %v", err)
	}
	return []Tool{find, execute}
}

// conversation is a two-turn exchange: the prompt, the model's call, and the
// server's answer to it.
func conversation(callID, tool string) []Message {
	return []Message{
		{Role: RoleUser, Text: "List the open issues."},
		{Role: RoleAssistant, Blocks: []modelrecord.Block{{
			Kind:      modelrecord.BlockToolCall,
			Tool:      tool,
			CallID:    callID,
			Arguments: json.RawMessage(`{"action":"issue.list","params":{"project_id":7}}`),
		}}},
		{Role: RoleTool, Results: []ToolResult{{CallID: callID, Tool: tool, Content: "| IID | Title |"}}},
	}
}

func TestNewTool_DropsWhatOneProviderRefusesAndKeepsTheRest(t *testing.T) {
	tool, err := NewTool("gitlab_find_action", "Find.", json.RawMessage(
		`{"$schema":"x","title":"t","additionalProperties":false,"type":"object",
		  "properties":{"query":{"type":["string","null"],"maxLength":256,"title":"q"}},
		  "required":["query"]}`,
	))
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	var schema map[string]any
	if decodeErr := json.Unmarshal(tool.Schema, &schema); decodeErr != nil {
		t.Fatalf("the normalized schema is not JSON: %v", decodeErr)
	}
	for _, dropped := range []string{"$schema", "title", "additionalProperties"} {
		t.Run(dropped, func(t *testing.T) {
			if _, present := schema[dropped]; present {
				t.Errorf("the normalized schema still carries %q", dropped)
			}
		})
	}
	if schema["type"] != "object" {
		t.Errorf("type = %v, want object", schema["type"])
	}
	properties, _ := schema["properties"].(map[string]any)
	query, _ := properties["query"].(map[string]any)
	if query["type"] != "string" {
		t.Errorf("the union type was not collapsed: %v", query["type"])
	}
	if query["maxLength"] == nil {
		t.Error("normalization dropped maxLength, which every provider understands")
	}
	if _, present := query["title"]; present {
		t.Error("normalization kept a nested title")
	}
	if len(schema["required"].([]any)) != 1 {
		t.Errorf("required = %v, want one name", schema["required"])
	}
}

func TestNewTool_LeavesAloneWhatItCannotImprove(t *testing.T) {
	// A union of nothing but null is not a type any provider can read, and
	// guessing one would send a schema the server never published. It is
	// carried through as it is, so the provider's own refusal says so.
	tool, err := NewTool("t", "d", json.RawMessage(`{"type":["null"],"properties":{}}`))
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	if !strings.Contains(string(tool.Schema), `["null"]`) {
		t.Errorf("schema = %s, want the union carried through", tool.Schema)
	}

	// A schema that is a bare value rather than an object is carried through
	// for the same reason.
	scalar, err := NewTool("t", "d", json.RawMessage(`true`))
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	if string(scalar.Schema) != "true" {
		t.Errorf("schema = %s, want it unchanged", scalar.Schema)
	}
}

func TestNewTool_RefusesWhatCannotBeSent(t *testing.T) {
	if _, err := NewTool("", "no name", nil); err == nil {
		t.Error("a tool with no name was accepted")
	}
	if _, err := NewTool("t", "bad schema", json.RawMessage(`{"type":`)); err == nil {
		t.Error("a schema that is not JSON was accepted")
	}
	tool, err := NewTool("t", "no schema", nil)
	if err != nil {
		t.Fatalf("NewTool with no schema: %v", err)
	}
	if string(tool.Schema) != `{"type":"object"}` {
		t.Errorf("schema = %s, want the empty object every provider requires", tool.Schema)
	}
}

func TestNew_RefusesAProviderItDoesNotHaveAndOneWithNoCredential(t *testing.T) {
	spec, err := ParseSpec("anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	_, credentialErr := New(Config{Spec: spec})
	if credentialErr == nil {
		t.Error("an adapter with no credential was built")
	} else if !strings.Contains(credentialErr.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("the refusal does not name the setting to fill: %v", credentialErr)
	}

	// With a credential, so what refuses it is the provider rather than the
	// missing key.
	if _, unknownErr := New(Config{Spec: Spec{Provider: "mistral", Model: "m"}, APIKey: testKey}); unknownErr == nil {
		t.Error("an unknown provider was built")
	} else if !strings.Contains(unknownErr.Error(), `unsupported provider "mistral"`) {
		t.Errorf("the refusal does not name the provider it does not have: %v", unknownErr)
	}

	// The fake needs no credential, which is what lets a pipe run start with
	// nothing configured.
	fake, err := ParseSpec("fake:perfect")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if _, fakeErr := New(Config{Spec: fake}); fakeErr != nil {
		t.Errorf("the fake was refused for having no credential: %v", fakeErr)
	}
}

func TestNew_EveryAdapterIsBuiltWithItsOwnEndpointAndKeyName(t *testing.T) {
	for _, one := range []struct{ spec, key string }{
		{"anthropic:claude-haiku-4-5-20251001", "ANTHROPIC_API_KEY"},
		{"openai:gpt-5.4-nano", "OPENAI_API_KEY"},
		{"qwen:qwen3.8-flash", "QWEN_API_KEY"},
		{"google:gemini-flash-latest", "GOOGLE_API_KEY"},
	} {
		t.Run(one.spec, func(t *testing.T) {
			spec, err := ParseSpec(one.spec)
			if err != nil {
				t.Fatalf("ParseSpec: %v", err)
			}
			name, needed := KeyName(spec.Provider)
			if !needed || name != one.key {
				t.Errorf("KeyName(%s) = %q, %v, want %q, true", spec.Provider, name, needed, one.key)
			}
			built, err := New(Config{Spec: spec, APIKey: testKey})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if built.Spec().Raw != one.spec {
				t.Errorf("Spec().Raw = %q, want %q", built.Spec().Raw, one.spec)
			}
		})
	}
	if name, needed := KeyName(Fake); needed || name != "" {
		t.Errorf("KeyName(fake) = %q, %v, want \"\", false", name, needed)
	}
}

func TestResponse_ReadsItsOwnBlocks(t *testing.T) {
	answer := Response{Blocks: []modelrecord.Block{
		{Kind: modelrecord.BlockText, Text: "thinking about it"},
		{Kind: modelrecord.BlockToolCall, Tool: "gitlab_execute_action", Arguments: json.RawMessage(`{}`)},
		{Kind: modelrecord.BlockToolCall, Tool: "gitlab_execute_action", Raw: "{action:"},
	}}
	if !answer.Malformed() {
		t.Error("a call with no arguments is not reported malformed")
	}
	if calls := answer.ToolCalls(); len(calls) != 2 {
		t.Errorf("ToolCalls() returned %d calls, want 2", len(calls))
	}

	well := Response{Blocks: []modelrecord.Block{
		{Kind: modelrecord.BlockToolCall, Tool: "t", Arguments: json.RawMessage(`{"a":1}`)},
	}}
	if well.Malformed() {
		t.Error("a well-formed call is reported malformed")
	}
}

func TestResponse_RetryableOnlyWhereSendingAgainCouldHelp(t *testing.T) {
	for status, want := range map[string]bool{
		modelrecord.TurnRateLimited:    true,
		modelrecord.TurnServerError:    true,
		modelrecord.TurnRequestError:   false,
		modelrecord.TurnTransportError: false,
		modelrecord.TurnOK:             false,
	} {
		t.Run(status, func(t *testing.T) {
			if got := (Response{Status: status}).Retryable(); got != want {
				t.Errorf("Retryable() = %v for %s, want %v", got, status, want)
			}
		})
	}
}

func TestCall_ClassifiesAProviderFailureWithoutRetryingIt(t *testing.T) {
	for _, one := range []struct {
		name   string
		code   int
		body   string
		status string
	}{
		{
			"rate limited", http.StatusTooManyRequests, `{"error":{"type":"rate_limit","message":"slow down"}}`,
			modelrecord.TurnRateLimited,
		},
		{
			"server error", http.StatusBadGateway, `{"error":{"type":"upstream","message":"nope"}}`,
			modelrecord.TurnServerError,
		},
		{
			"bad request", http.StatusBadRequest, `{"error":{"type":"invalid_request","message":"no"}}`,
			modelrecord.TurnRequestError,
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			backend := newRecorder(one.body).withStatus(one.code)
			adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

			answer, err := adapter.Call(t.Context(), Request{Messages: conversation("c1", "gitlab_execute_action")})
			if err == nil {
				t.Fatal("a failed request was reported as a success")
			}
			if answer.Status != one.status {
				t.Errorf("Status = %q, want %q", answer.Status, one.status)
			}
			if answer.HTTPStatus != one.code {
				t.Errorf("HTTPStatus = %d, want %d", answer.HTTPStatus, one.code)
			}
			if answer.Detail == "" {
				t.Error("a failed request kept no detail, so the record could not say why")
			}
			if len(answer.RequestBody) == 0 || answer.RequestDigest == "" {
				t.Error("a failed request kept no body or digest, so the trace loses the turn")
			}
			// The adapter sends once. Retrying is the runner's decision, and
			// an adapter that retried would spend the budget the runner is
			// counting.
			if backend.count() != 1 {
				t.Errorf("the backend saw %d requests, want exactly 1", backend.count())
			}
		})
	}
}

func TestCall_KeepsTheCredentialOutOfWhatItRecords(t *testing.T) {
	backend := newRecorder(`{"error":{"type":"authentication_error","message":"invalid key ` +
		testKey + `"}}`).withStatus(http.StatusUnauthorized)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("an unauthenticated request was reported as a success")
	}
	if strings.Contains(answer.Detail, testKey) {
		t.Errorf("the detail carries the credential: %q", answer.Detail)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("the error carries the credential: %v", err)
	}
	if !strings.Contains(answer.Detail, "REDACTED") {
		t.Errorf("the detail does not say a value was taken out: %q", answer.Detail)
	}
}

func TestCall_BoundsWhatItKeepsOfABadDay(t *testing.T) {
	// A provider having a bad day answers a 500 with a page of HTML, and this
	// detail is committed and read by people.
	backend := newRecorder(strings.Repeat("x", maxDetailBytes*2)).withStatus(http.StatusInternalServerError)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("a 500 was read as an answer")
	}
	if len(answer.Detail) > maxDetailBytes+len("...(truncated)") {
		t.Errorf("the detail is %d bytes, want it bounded at %d", len(answer.Detail), maxDetailBytes)
	}
	if !strings.HasSuffix(answer.Detail, "...(truncated)") {
		t.Error("the detail was cut without saying so")
	}
}

// TestCall_ReportsATransportFailureAsOne verifies every adapter reports a
// request that got no answer as a transport failure, with no HTTP status.
func TestCall_ReportsATransportFailureAsOne(t *testing.T) {
	for _, spec := range []string{
		"anthropic:claude-haiku-4-5-20251001", "openai:gpt-5.4-nano", "google:gemini-flash-latest",
	} {
		t.Run(spec, func(t *testing.T) {
			// A dead endpoint hangs up before any answer and keeps its port,
			// so the call fails in transport and no other listener can answer
			// it instead.
			adapter := adapterFor(t, spec, netfixture.DeadEndpoint(t))
			answer, err := adapter.Call(t.Context(), Request{})
			if err == nil {
				t.Fatal("an unreachable provider was reported as a success")
			}
			if answer.Status != modelrecord.TurnTransportError {
				t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnTransportError)
			}
			if answer.HTTPStatus != 0 {
				t.Errorf("HTTPStatus = %d, want 0 for a request that never got one", answer.HTTPStatus)
			}
		})
	}
}

// TestCall_ReportsAnAnswerCutShortAsATransportFailure verifies an answer
// whose body ends before the length it declared is a transport failure that
// keeps the status it arrived with, rather than an answer read in part.
func TestCall_ReportsAnAnswerCutShortAsATransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The server closes a connection whose handler wrote less than the
		// length it declared, so the client meets the end of the body early.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":`))
	}))
	t.Cleanup(server.Close)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", server.URL)

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil || !strings.Contains(err.Error(), "read the anthropic answer") {
		t.Fatalf("err = %v, want the read of the answer to fail", err)
	}
	if answer.Status != modelrecord.TurnTransportError || answer.HTTPStatus != http.StatusOK {
		t.Errorf("Status = %q, HTTPStatus = %d, want %q with the 200 that arrived",
			answer.Status, answer.HTTPStatus, modelrecord.TurnTransportError)
	}
}

// TestExchange_ARequestThatCannotBeBuilt_IsTheCallersFailure verifies the two
// ways a request fails before it is sent, a payload that does not encode and
// an endpoint that does not parse, are request errors that send nothing.
func TestExchange_ARequestThatCannotBeBuilt_IsTheCallersFailure(t *testing.T) {
	adapter := base{spec: Spec{Provider: Anthropic}, client: http.DefaultClient}
	for _, one := range []struct {
		name     string
		endpoint string
		payload  any
		want     string
	}{
		{"a payload that does not encode", "http://never-dialed.invalid", map[string]any{"f": func() {}}, "marshal the anthropic request"},
		{"an endpoint that does not parse", "://no-scheme", map[string]any{}, "build the anthropic request"},
	} {
		t.Run(one.name, func(t *testing.T) {
			answer, raw, err := adapter.exchange(t.Context(), one.endpoint, nil, one.payload)
			if err == nil || !strings.Contains(err.Error(), one.want) {
				t.Fatalf("err = %v, want it to say %q", err, one.want)
			}
			if answer.Status != modelrecord.TurnRequestError || answer.Detail == "" || raw != nil {
				t.Errorf("answer = %+v, raw = %q, want a request error that says why and carries no body", answer, raw)
			}
		})
	}
}

// TestFailureStatus_ClassifiesEachSideOfEveryBoundary holds the status a
// turn records for each HTTP status on either side of the bounds the
// classification draws.
func TestFailureStatus_ClassifiesEachSideOfEveryBoundary(t *testing.T) {
	for _, one := range []struct {
		code   int
		status string
		failed bool
	}{
		{http.StatusTooManyRequests, modelrecord.TurnRateLimited, true},
		{http.StatusInternalServerError, modelrecord.TurnServerError, true},
		{http.StatusInternalServerError - 1, modelrecord.TurnRequestError, true},
		{http.StatusMultipleChoices, modelrecord.TurnRequestError, true},
		{http.StatusMultipleChoices - 1, modelrecord.TurnOK, false},
		{http.StatusOK, modelrecord.TurnOK, false},
		{http.StatusOK - 1, modelrecord.TurnRequestError, true},
	} {
		t.Run(strconv.Itoa(one.code), func(t *testing.T) {
			status, failed := failureStatus(one.code)
			if status != one.status || failed != one.failed {
				t.Errorf("failureStatus(%d) = %q, %v, want %q, %v", one.code, status, failed, one.status, one.failed)
			}
		})
	}
}

// TestRedact_KeepsADetailOfTheBoundsLengthWholeAndCutsALongerOne verifies
// where a detail is cut, and that an adapter with no credential, the fake's
// case, has nothing to take out of it.
func TestRedact_KeepsADetailOfTheBoundsLengthWholeAndCutsALongerOne(t *testing.T) {
	exact := strings.Repeat("a", maxDetailBytes)
	if got := (base{}).redact("  " + exact + "  "); got != exact {
		t.Errorf("a detail of %d bytes was changed to %d bytes", maxDetailBytes, len(got))
	}
	longer := exact + "b"
	if got := (base{}).redact(longer); got != exact+"...(truncated)" {
		t.Errorf("a detail of %d bytes was kept as %q", len(longer), got[len(got)-20:])
	}
}

// TestNew_SendsThroughTheClientItIsGivenOrOneWithATimeout verifies the client
// a caller configures is the one the adapter sends with, and that with none
// the adapter's own has the ten-minute timeout that keeps a provider that
// never answers from holding a run open.
func TestNew_SendsThroughTheClientItIsGivenOrOneWithATimeout(t *testing.T) {
	spec, err := ParseSpec("anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	given := &http.Client{Timeout: time.Second}
	for _, one := range []struct {
		name    string
		client  *http.Client
		timeout time.Duration
	}{
		{"a given client", given, time.Second},
		{"no client", nil, 10 * time.Minute},
	} {
		t.Run(one.name, func(t *testing.T) {
			built, newErr := New(Config{Spec: spec, APIKey: testKey, HTTPClient: one.client})
			if newErr != nil {
				t.Fatalf("New: %v", newErr)
			}
			adapter, isAnthropic := built.(anthropicAdapter)
			if !isAnthropic {
				t.Fatalf("New built %T, want the Anthropic adapter", built)
			}
			if one.client != nil && adapter.client != one.client {
				t.Error("the adapter sends through a client of its own instead of the given one")
			}
			if adapter.client.Timeout != one.timeout {
				t.Errorf("the client times out after %s, want %s", adapter.client.Timeout, one.timeout)
			}
		})
	}
}

// TestCall_MeasuresTheLatencyInMilliseconds verifies the latency a turn
// records is the exchange's in milliseconds: never more than the wall time
// the whole call took, measured around it.
func TestCall_MeasuresTheLatencyInMilliseconds(t *testing.T) {
	backend := newRecorder(`{"content":[],"usage":{}}`)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	started := time.Now()
	answer, err := adapter.Call(t.Context(), Request{})
	wall := float64(time.Since(started).Microseconds()) / 1000
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if answer.LatencyMS < 0 || answer.LatencyMS > wall {
		t.Errorf("LatencyMS = %v, want a figure between 0 and the %v ms the call took", answer.LatencyMS, wall)
	}
}

// TestNormalizeType_PicksTheNamedMemberOfAUnion verifies a union is collapsed
// to the type its first named member gives, past a member that is not a name.
func TestNormalizeType_PicksTheNamedMemberOfAUnion(t *testing.T) {
	if got := normalizeType([]any{float64(1), "null", "integer"}); got != "integer" {
		t.Errorf("normalizeType = %#v, want integer", got)
	}
}

func TestCall_CarriesTheCallersDeadline(t *testing.T) {
	backend := newRecorder(`{"content":[]}`)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := adapter.Call(ctx, Request{}); err == nil {
		t.Error("a cancelled call was sent anyway")
	}
}
