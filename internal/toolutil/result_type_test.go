// result_type_test.go verifies LabelForRevision, which gives a tools/call
// result a receiving middleware makes the resultType revision 2026-07-28
// requires, and pins the go-sdk behavior it works around.
package toolutil

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callNaming is a tools/call as it reaches a receiving middleware, its raw
// params naming revision version in _meta, the way every request of 2026-07-28
// does. A nil version names none, as a request of an earlier revision does.
func callNaming(version any) mcp.Request {
	params := &mcp.CallToolParamsRaw{Name: "search"}
	if version != nil {
		params.SetMeta(map[string]any{mcp.MetaKeyProtocolVersion: version})
	}
	return &mcp.CallToolRequest{Params: params}
}

// decodedCallNaming is a tools/call whose params have already been decoded,
// the other shape [extractToolName] reads, naming revision version in _meta.
func decodedCallNaming(version string) mcp.Request {
	params := &mcp.CallToolParams{Name: "search"}
	params.SetMeta(map[string]any{mcp.MetaKeyProtocolVersion: version})
	return &mcp.ServerRequest[*mcp.CallToolParams]{Params: params}
}

// wireFields writes result the way the SDK sends it and returns its top-level
// fields, so an absent resultType can be told from an empty one. Numbers are
// kept as written, so two that float64 cannot tell apart still differ.
func wireFields(t *testing.T, result mcp.Result) map[string]any {
	t.Helper()
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("the result does not marshal: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.UseNumber()
	var fields map[string]any
	if err = decoder.Decode(&fields); err != nil {
		t.Fatalf("the result is not a JSON object: %v (%s)", err, wire)
	}
	return fields
}

// TestLabelForRevision_EachRequest_LabelsWhereItsRevisionRequiresIt covers the
// one field a request's revision decides, over every shape a request reaches a
// middleware in.
//
// A request naming 2026-07-28 or a later revision in its _meta gets a copy
// carrying resultType "complete", with the text, its annotations and the
// error flag of the refusal it was made from, which is left as it was. Any
// other request gets the refusal itself, unlabeled, which is what the SDK
// sends a client of an earlier revision from its own dispatcher: no request,
// params that are absent or typed nil, a revision earlier than 2026-07-28, a
// revision that is not a string, and a method whose params the middleware does
// not read.
func TestLabelForRevision_EachRequest_LabelsWhereItsRevisionRequiresIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		req     mcp.Request
		labeled bool
	}{
		{name: "no request", req: nil},
		{name: "raw params that are a typed nil", req: &mcp.CallToolRequest{}},
		{name: "raw params naming no revision", req: callNaming(nil)},
		{name: "raw params naming 2025-11-25", req: callNaming("2025-11-25")},
		{name: "raw params naming a revision that is not a string", req: callNaming(20260728)},
		{name: "raw params naming 2026-07-28", req: callNaming("2026-07-28"), labeled: true},
		{name: "raw params naming a later revision", req: callNaming("2027-03-01"), labeled: true},
		{name: "decoded params naming 2026-07-28", req: decodedCallNaming("2026-07-28"), labeled: true},
		{name: "decoded params naming 2025-11-25", req: decodedCallNaming("2025-11-25")},
		{name: "decoded params that are a typed nil", req: &mcp.ServerRequest[*mcp.CallToolParams]{}},
		{name: "a request of another method", req: &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{
			Meta: mcp.Meta{mcp.MetaKeyProtocolVersion: "2026-07-28"},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			refusal := ErrorResultAnnotated("slow down", ContentMutate)
			got := LabelForRevision(tc.req, refusal)

			if _, present := wireFields(t, refusal)["resultType"]; present {
				t.Error("the refusal handed in was given a resultType; want it left as it was and a copy labeled")
			}
			if !tc.labeled {
				if got != refusal {
					t.Errorf("a request that names no revision requiring resultType got %p, want the refusal %p itself", got, refusal)
				}
				return
			}
			fields := wireFields(t, got)
			if fields["resultType"] != completeResultType || fields["isError"] != true {
				t.Errorf("the labeled refusal is %v, want isError true and resultType %q", fields, completeResultType)
			}
			text, isText := got.Content[0].(*mcp.TextContent)
			if len(got.Content) != 1 || !isText || text.Text != "slow down" || text.Annotations == nil ||
				text.Annotations.Priority != ContentMutate.Priority || len(text.Annotations.Audience) != 1 ||
				text.Annotations.Audience[0] != ContentMutate.Audience[0] {
				t.Errorf("the labeled refusal's content is %#v, want the one text block with the refusal's annotations", got.Content)
			}
		})
	}
}

// TestLabelForRevision_NilResult_StaysNil covers a middleware with nothing to
// answer: a nil result is handed back as nil at 2026-07-28 too, rather than as
// an empty result the round trip would have made of it.
func TestLabelForRevision_NilResult_StaysNil(t *testing.T) {
	t.Parallel()
	if got := LabelForRevision(callNaming("2026-07-28"), nil); got != nil {
		t.Errorf("a nil result came back as %#v, want nil", got)
	}
}

// beyondFloat64 is an integer float64 cannot hold exactly, so a value that
// went through a decoding into any comes back as a neighbor of it.
const beyondFloat64 int64 = 1<<53 + 1

// samplingRequest is a sampling request a tool result asks the client for,
// which the SDK's decoding rebuilds as a CreateMessageWithToolsParams.
//
//nolint:staticcheck // SA1019: sampling is deprecated upstream and still a request a result can carry, and its decoded type is what the case covers
func samplingRequest() *mcp.CreateMessageParams {
	return &mcp.CreateMessageParams{
		MaxTokens: 64,
		Messages:  []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "Summarize the issue"}}},
	}
}

// TestLabelForRevision_EachResult_GoesOutAsItWasBesideTheDispatchersLabel
// covers what the copy holds at 2026-07-28: on the wire, the result it was
// made from plus the resultType the SDK's dispatcher would have given it, and
// nothing else changed.
//
// A result carrying InputRequests is labeled "input_required", as the
// dispatcher labels one, since labeled complete it would tell a client that
// trusts the label to drop the requests: an elicitation, a sampling request,
// which decoding alone would rebuild as another type, and an empty map, the
// SDK's load-shedding answer, which the round trip alone would drop. Any other
// is labeled "complete", the fields decoding rebuilds included: no content,
// which would come back as an empty list, and structured content or a _meta
// holding a number past float64's exact range, which would come back rounded.
// The requests are the result's own, the _meta a copy the SDK can write its
// server's name into without reaching the result, and the result handed in is
// left as it was. A request of an earlier revision gets the result itself.
func TestLabelForRevision_EachResult_GoesOutAsItWasBesideTheDispatchersLabel(t *testing.T) {
	t.Parallel()
	text := []mcp.Content{&mcp.TextContent{Text: "done"}}
	cases := []struct {
		name       string
		result     func() *mcp.CallToolResult
		resultType string
	}{
		{name: "asking for an elicitation", resultType: inputRequiredResultType, result: func() *mcp.CallToolResult {
			return &mcp.CallToolResult{RequestState: "state-1", InputRequests: mcp.InputRequestMap{"project": &mcp.ElicitParams{
				Message: "Which project?",
				RequestedSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"path": map[string]any{"type": "string"}},
				},
			}}}
		}},
		{name: "asking for a sampling", resultType: inputRequiredResultType, result: func() *mcp.CallToolResult {
			return &mcp.CallToolResult{RequestState: "state-1", InputRequests: mcp.InputRequestMap{"summary": samplingRequest()}}
		}},
		{name: "shedding load with an empty map", resultType: inputRequiredResultType, result: func() *mcp.CallToolResult {
			return &mcp.CallToolResult{RequestState: "state-1", InputRequests: mcp.InputRequestMap{}}
		}},
		{name: "with no content", resultType: completeResultType, result: func() *mcp.CallToolResult {
			return &mcp.CallToolResult{IsError: true}
		}},
		{name: "with structured content past float64", resultType: completeResultType, result: func() *mcp.CallToolResult {
			return &mcp.CallToolResult{Content: text, StructuredContent: map[string]any{"id": beyondFloat64}}
		}},
		{name: "with a _meta past float64", resultType: completeResultType, result: func() *mcp.CallToolResult {
			return &mcp.CallToolResult{Content: text, Meta: mcp.Meta{"trace": beyondFloat64}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := tc.result()
			before := wireFields(t, result)

			if got := LabelForRevision(callNaming("2025-11-25"), result); got != result {
				t.Errorf("a request of an earlier revision got %p, want the result %p itself", got, result)
			}
			got := LabelForRevision(callNaming("2026-07-28"), result)
			after := wireFields(t, got)
			if after["resultType"] != tc.resultType {
				t.Errorf("resultType = %v, want %q, the label the SDK's dispatcher gives this result", after["resultType"], tc.resultType)
			}
			delete(after, "resultType")
			if !reflect.DeepEqual(after, before) {
				t.Errorf("beside its resultType the copy is %v on the wire, want %v, the result it was made from", after, before)
			}
			for id, request := range result.InputRequests {
				if got.InputRequests[id] != request {
					t.Errorf("input request %q is %#v in the copy, want the result's own %#v", id, got.InputRequests[id], request)
				}
			}
			// What the SDK does to the _meta of a result it sends.
			if got.Meta != nil {
				got.Meta[mcp.MetaKeyServerInfo] = "the SDK's server"
			}
			if !reflect.DeepEqual(wireFields(t, result), before) {
				t.Error("the result handed in was changed, or reached through the copy; want it left as it was")
			}
		})
	}
}

// TestLabelForRevision_ResultThatCannotMakeTheRoundTrip_IsHandedBackUnlabeled
// covers the two ways the round trip can fail, neither of which a refusal this
// server makes can reach: a result encoding/json cannot write, and one the
// SDK's decoding refuses to read back. Each is handed back as it came, which is
// the answer the SDK would have sent.
func TestLabelForRevision_ResultThatCannotMakeTheRoundTrip_IsHandedBackUnlabeled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		result *mcp.CallToolResult
	}{
		{name: "one that cannot be written", result: &mcp.CallToolResult{
			Meta:    mcp.Meta{"unwritable": make(chan int)},
			Content: []mcp.Content{&mcp.TextContent{Text: "slow down"}},
		}},
		{name: "one that cannot be read back", result: &mcp.CallToolResult{Content: []mcp.Content{nil}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := LabelForRevision(callNaming("2026-07-28"), tc.result); got != tc.result {
				t.Errorf("got %p, want the result %p handed back", got, tc.result)
			}
		})
	}
}

// resultTypeFix is what a failure of the pin below asks the go-sdk bump to do:
// everything the workaround added, which the bump retires in one change.
const resultTypeFix = "Retire the workaround in this same pull request. " +
	"Delete internal/toolutil/result_type.go and this test file. " +
	"Return the refusals as they are built in internal/toolutil/rate_limit.go (attachRateLimitFunc, with the " +
	"comment above the call) and cmd/server/held.go (heldRequestsRefusal), and drop the req parameter " +
	"heldRequestsRefusal gained only for the label, at its calls in held.go and held_test.go. " +
	"Delete TestAttachRateLimit_ToolRefusal_CarriesTheResultTypeOfItsRevision in internal/toolutil/rate_limit_test.go " +
	"and TestHeldRequestsMiddleware_ToolRefusal_CarriesTheResultTypeOfItsRevision in cmd/server/held_test.go, " +
	"or drive them through an SDK server: both read the middleware's return before the SDK labels it, so both " +
	"fail once the refusals are returned as built. Move the helpers rate_limit_test.go borrows (callNaming and wireFields from this file, " +
	"completeResultType from result_type.go) into it, or delete them with their users. " +
	"Remove what is said about the label from the doc comments of AttachRateLimit (rate_limit.go) and " +
	"heldRequestsRefusal (held.go), from the comment on row RTC-001 in internal/tenancy/decisions_allow.go, from " +
	"the paragraph on F-20 in docs/development/tenant-policy-spec.md, from Transport end-to-end modules in CLAUDE.md, from " +
	"HTTP transport module in test/e2e/README.md and from the header of test/e2e/http/result_type_test.go. " +
	"Mark row 66 of docs/development/upstream-bugs.md merged with the version and retire its workaround, and " +
	"keep TestRateLimitedToolCall_EachRevision_CarriesTheResultTypeTheServedCallDoes in test/e2e/http passing."

// middlewareMadeTools serves one tool through the SDK's dispatcher and answers
// two more from a receiving middleware in its place: "refused" with a tool
// result as a middleware makes it, and "labeled" with the same result passed
// through LabelForRevision. It returns a raw connection to that server over an
// in-memory transport, so a test reads each response as it arrived.
func middlewareMadeTools(t *testing.T) mcp.Connection {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "pin", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "served"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "served"}}}, nil, nil
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			made := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "refused"}}}
			switch extractToolName(req) {
			case "refused":
				return made, nil
			case "labeled":
				return LabelForRevision(req, made), nil
			}
			return next(ctx, method, req)
		}
	})

	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	session, err := server.Connect(t.Context(), serverEnd, nil)
	if err != nil {
		t.Fatalf("connect the server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	conn, err := clientEnd.Connect(t.Context())
	if err != nil {
		t.Fatalf("connect the client end: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// rawModernToolCall sends one tools/call at 2026-07-28 on conn and returns the
// result object of the response as it arrived.
//
// It reads the raw JSON-RPC response because the SDK's own client decodes
// resultType into an unexported field and cannot say whether it arrived.
func rawModernToolCall(t *testing.T, conn mcp.Connection, id int64, tool string) map[string]any {
	t.Helper()
	requestID, err := jsonrpc.MakeID(float64(id))
	if err != nil {
		t.Fatalf("make the request id: %v", err)
	}
	params, err := json.Marshal(map[string]any{
		"name":      tool,
		"arguments": map[string]any{},
		"_meta": map[string]any{
			mcp.MetaKeyProtocolVersion:    resultTypeRevision,
			mcp.MetaKeyClientCapabilities: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("marshal the params: %v", err)
	}
	if err = conn.Write(t.Context(), &jsonrpc.Request{ID: requestID, Method: methodToolsCall, Params: params}); err != nil {
		t.Fatalf("write the %s call: %v", tool, err)
	}
	msg, err := conn.Read(t.Context())
	if err != nil {
		t.Fatalf("read the answer to the %s call: %v", tool, err)
	}
	response, isResponse := msg.(*jsonrpc.Response)
	if !isResponse {
		t.Fatalf("the answer to the %s call is %T, want a response", tool, msg)
	}
	if response.Error != nil {
		t.Fatalf("the %s call was answered with an error: %v", tool, response.Error)
	}
	var result map[string]any
	if err = json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("the %s call's result is not a JSON object: %v (%s)", tool, err, response.Result)
	}
	return result
}

// TestSDK_MiddlewareToolResult_GoesOutUnlabeled pins the go-sdk behavior
// LabelForRevision works around, on a bare SDK server: at 2026-07-28 a
// tools/call result the tool dispatcher makes carries resultType "complete",
// and one a receiving middleware makes in the dispatcher's place carries none.
//
// The served call is the control, which makes the absence about who built the
// result rather than about the revision, the transport or the tool. go-sdk
// fixed this after v1.8.0 in e40f35d, the merge of its pull request 1226, and
// the bump that brings it fails here, saying what the workaround it retires
// is; that is row 66 of docs/development/upstream-bugs.md.
func TestSDK_MiddlewareToolResult_GoesOutUnlabeled(t *testing.T) {
	t.Parallel()
	conn := middlewareMadeTools(t)

	served := rawModernToolCall(t, conn, 1, "served")
	if got := served["resultType"]; got != completeResultType {
		t.Fatalf("a result the dispatcher made at 2026-07-28 carried resultType %v, want %q: the control is broken, so this test says nothing about the middleware's result", got, completeResultType)
	}
	refused := rawModernToolCall(t, conn, 2, "refused")
	if got, present := refused["resultType"]; present {
		t.Errorf("a result a middleware made now carries resultType %v: the go-sdk this module builds against labels it itself, "+
			"which is e40f35d, the merge of go-sdk's pull request 1226, and row 66 of docs/development/upstream-bugs.md. %s", got, resultTypeFix)
	}
}

// TestLabelForRevision_OverTheSDK_ReachesTheWireAsTheServedCallDoes drives the
// workaround through the same bare server: a result a middleware made and
// passed through LabelForRevision reaches a 2026-07-28 client carrying
// resultType "complete", as the call the dispatcher served does, with its error
// flag and its text, so the field set by decoding survives the SDK's own
// encoding on the way out.
func TestLabelForRevision_OverTheSDK_ReachesTheWireAsTheServedCallDoes(t *testing.T) {
	t.Parallel()
	conn := middlewareMadeTools(t)

	labeled := rawModernToolCall(t, conn, 1, "labeled")
	content, _ := labeled["content"].([]any)
	var block map[string]any
	if len(content) == 1 {
		block, _ = content[0].(map[string]any)
	}
	if labeled["resultType"] != completeResultType || labeled["isError"] != true || block["text"] != "refused" {
		t.Errorf("the labeled result reached the client as %v, want resultType %q, isError true and its one text block", labeled, completeResultType)
	}
}
