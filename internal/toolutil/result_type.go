package toolutil

import (
	"encoding/json"
	"maps"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resultTypeRevision is the first protocol revision whose results carry
// resultType.
const resultTypeRevision = "2026-07-28"

// completeResultType is the resultType of a result that asks the client for
// nothing more.
const completeResultType = "complete"

// inputRequiredResultType is the resultType of a result that asks the client
// for input before the call can complete.
const inputRequiredResultType = "input_required"

// LabelForRevision returns result, which a receiving middleware answers the
// tools/call req with in place of the SDK's tool dispatcher, carrying the
// resultType req's revision requires.
//
// Revision 2026-07-28 requires resultType on every result. go-sdk v1.8.0 sets
// it on a tools/call result only inside its own tool dispatcher, because such a
// result can also be input_required, and labels after the middleware chain
// returns only the result types that can never be input_required. So a tool
// result a middleware makes, which is what a refusal is, went to a 2026-07-28
// client without the field while every call the dispatcher served carried
// "complete". The SDK keeps the field unexported behind an unexported setter,
// and its own decoding is the one public way to set it: the result is written
// with every field it has and its resultType, and read back through
// [mcp.CallToolResult.UnmarshalJSON].
//
// The resultType is chosen the way the SDK's dispatcher chooses it, by
// whether the result carries InputRequests: "input_required" when it does,
// even an empty map, which the SDK reads as load shedding, and "complete"
// when it does not. Labeled "complete", a result asking for input would go
// out as {"resultType":"complete","inputRequests":...}, and a client that
// trusts the label drops the requests; left as it was, it would go out with
// no label, breaking the requirement this exists to meet.
//
// A request of an earlier revision gets result as it is, which is what the SDK
// sends such a client from its own dispatcher (see [namesResultTypeRevision]
// for where the two tests differ), and so does a nil result, which has nothing
// to label. A result that cannot make the round trip is handed back unlabeled
// too, the answer the SDK would have sent: no refusal this server makes is one,
// since each is a text block and an error flag.
//
// It is the workaround for row 66 of docs/development/upstream-bugs.md, which
// go-sdk fixed after v1.8.0 in e40f35d, the merge of its pull request 1226.
// TestSDK_MiddlewareToolResult_GoesOutUnlabeled fails on the bump that brings
// the fix and says what to delete.
func LabelForRevision(req mcp.Request, result *mcp.CallToolResult) *mcp.CallToolResult {
	if result == nil || !namesResultTypeRevision(req) {
		return result
	}
	resultType := completeResultType
	if result.InputRequests != nil {
		resultType = inputRequiredResultType
	}
	labeled, err := withResultType(result, resultType)
	if err != nil {
		return result
	}
	return labeled
}

// namesResultTypeRevision reports whether req names, in the _meta of its
// params, a revision whose results must carry resultType.
//
// It is the test go-sdk v1.8.0 applies to a request before it labels, once the
// middleware chain returns, the result types it labels there
// (validateRequestMeta in mcp/shared.go), and the one e40f35d applies to every
// result: the revision a request names in its _meta, compared as a string,
// which is how the dated revisions order. A request that names none comes from
// a client that negotiated an earlier revision at initialize, since a
// 2026-07-28 client names its revision on every request.
//
// v1.8.0's tool dispatcher asks instead which revision the session recorded
// when it began, from initialize or from the first request's _meta
// (clientSupportsMultiRoundTrip in mcp/mrtr.go). The two answers differ only
// where that revision and a request's _meta disagree, in either direction. A
// client that asks for 2026-07-28 in initialize and is negotiated down gets
// the dispatcher's label and not this one, and the revision it speaks does not
// require the field. A client that negotiated an earlier revision and then
// names 2026-07-28 in a request's _meta, which v1.8.0 accepts over stdio
// (stateful HTTP refuses such a request and stateless HTTP holds it to its
// header), gets this label and not the dispatcher's, which is what the
// revision it names requires and what e40f35d sends.
//
// The params are read by their concrete type, as [extractToolName] reads them,
// because a typed nil behind the Params interface panics in GetMeta.
func namesResultTypeRevision(req mcp.Request) bool {
	if req == nil {
		return false
	}
	var meta mcp.Meta
	switch p := req.GetParams().(type) {
	case *mcp.CallToolParamsRaw:
		if p != nil {
			meta = p.Meta
		}
	case *mcp.CallToolParams:
		if p != nil {
			meta = p.Meta
		}
	}
	version, _ := meta[mcp.MetaKeyProtocolVersion].(string)
	return version >= resultTypeRevision
}

// toolResultFields is a tool result without the SDK's methods, so encoding/json
// writes its exported fields by their tags instead of calling
// [mcp.CallToolResult.MarshalJSON], which writes resultType as it stands. It is
// the trick the SDK's own MarshalJSON plays on the same struct.
type toolResultFields mcp.CallToolResult

// withResultType returns a copy of result whose resultType is resultType, and
// the error that kept it from being one, when it is not.
//
// The copy is what [mcp.CallToolResult.UnmarshalJSON] reads back from result's
// fields written beside resultType, so it carries every field the wire does,
// and not the error a handler recorded with SetError, which no client ever
// sees. Its content, structured content, input requests and _meta are then
// result's own again, because decoding rebuilds each of them rather than
// keeping it: a nil content comes back empty, an empty map of requests, which
// the SDK reads as load shedding, does not come back at all, a sampling
// request comes back as the type a client reads, and a number past float64's
// exact range comes back rounded. The _meta is a copy of result's map, since
// the SDK writes its server's name into the _meta of a result it sends.
func withResultType(result *mcp.CallToolResult, resultType string) (*mcp.CallToolResult, error) {
	wire, err := json.Marshal(struct {
		*toolResultFields
		ResultType string `json:"resultType"`
	}{(*toolResultFields)(result), resultType})
	if err != nil {
		return nil, err
	}
	labeled := new(mcp.CallToolResult)
	err = labeled.UnmarshalJSON(wire)
	if err != nil {
		return nil, err
	}
	labeled.Content = result.Content
	labeled.StructuredContent = result.StructuredContent
	labeled.InputRequests = result.InputRequests
	labeled.Meta = maps.Clone(result.Meta)
	return labeled, nil
}
