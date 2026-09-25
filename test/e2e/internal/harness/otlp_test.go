//go:build e2e

// otlp_test.go covers the receiver on its own terms: a payload built by hand,
// posted the way the exporter posts one, and read back as the facts a call
// line joins to.
//
// The whole-binary half is in record_test.go, which is where the two halves
// meet. What is here is the decoding, the filtering and the refusals, each of
// which fails silently if it is wrong: a receiver that dropped every span
// would leave a record that looks complete and says nothing about what ran.

package harness

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
)

// testTraceID is the trace the hand-built spans below belong to.
const testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

// exportTimeout bounds the requests these tests make of their own receiver.
const exportTimeout = 10 * time.Second

// stubSpan builds one span carrying the attributes a call line reads.
func stubSpan(traceID string, attributes map[string]string, code tracepb.Status_StatusCode) *tracepb.Span {
	decoded, err := hex.DecodeString(traceID)
	if err != nil {
		decoded = nil
	}
	span := &tracepb.Span{TraceId: decoded, Name: "tools/call", Status: &tracepb.Status{Code: code}}
	for key, value := range attributes {
		span.Attributes = append(span.Attributes, &commonpb.KeyValue{
			Key:   key,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
		})
	}
	return span
}

// stubRequestSpan builds one GitLab client span the way
// internal/mcpotel.NewTransport makes it: the method attribute and the client
// kind, which together are what the receiver counts on.
func stubRequestSpan(traceID, method string) *tracepb.Span {
	span := stubSpan(traceID, map[string]string{
		string(mcpotel.AttrHTTPRequestMethod): method,
		"server.address":                      "gitlab.example.com",
	}, tracepb.Status_STATUS_CODE_UNSET)
	span.Name = method
	span.Kind = tracepb.Span_SPAN_KIND_CLIENT
	return span
}

// stubOutboundMCPSpan builds the span internal/mcpotel.SendingMiddleware makes
// for one server-initiated MCP request that failed.
//
// It is a client span like a GitLab round trip and carries none of the
// attributes that tell one apart: no http.request.method, and an error.type
// the failure path writes onto it. That is exactly the shape which used to
// reach the merge, so it is built here from the keys the middleware itself
// uses rather than from a plausible-looking set of my own.
func stubOutboundMCPSpan(traceID, method string) *tracepb.Span {
	span := stubSpan(traceID, map[string]string{
		string(mcpotel.AttrMCPMethodName):         method,
		string(mcpotel.AttrNetworkTransport):      "pipe",
		string(mcpotel.AttrErrorType):             "_OTHER",
		string(mcpotel.AttrRPCResponseStatusCode): "-32603",
	}, tracepb.Status_STATUS_CODE_ERROR)
	span.Name = method
	span.Kind = tracepb.Span_SPAN_KIND_CLIENT
	return span
}

// stubServerSpan builds the span the server's own middleware opens for one
// request of a method other than tools/call: the server kind and the method
// name, plus whatever the method adds, and none of the dispatch facts a tool
// call's span carries.
func stubServerSpan(traceID, method string, attributes map[string]string, code tracepb.Status_StatusCode) *tracepb.Span {
	withMethod := map[string]string{string(mcpotel.AttrMCPMethodName): method}
	maps.Copy(withMethod, attributes)
	span := stubSpan(traceID, withMethod, code)
	span.Name = method
	span.Kind = tracepb.Span_SPAN_KIND_SERVER
	return span
}

// exportOf wraps spans in the request an exporter sends.
func exportOf(spans ...*tracepb.Span) *coltracepb.ExportTraceServiceRequest {
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}},
		}},
	}
}

// postExport sends one payload to a receiver the way the exporter does, and
// returns the status it answered.
func postExport(t *testing.T, url, path string, body []byte, encoding string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building the export request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}

	client := &http.Client{Timeout: exportTimeout}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("posting the export: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// startTestReceiver starts a receiver of this test's own, apart from the one
// the sessions share.
func startTestReceiver(t *testing.T) *spanReceiver {
	t.Helper()

	received, err := startSpanReceiver()
	if err != nil {
		t.Fatalf("starting the receiver: %v", err)
	}
	t.Cleanup(func() { _ = received.server.Close() })
	return received
}

// marshalExport encodes an export, failing the test when it cannot.
func marshalExport(t *testing.T, export *coltracepb.ExportTraceServiceRequest) []byte {
	t.Helper()

	body, err := proto.Marshal(export)
	if err != nil {
		t.Fatalf("encoding the export: %v", err)
	}
	return body
}

// TestSpanReceiver_IssuedTrace_KeepsWhatTheServerSaid is the join the whole
// record rests on: the harness stamps a trace, the server's span carries it
// back, and these are the five facts read off it.
func TestSpanReceiver_IssuedTrace_KeepsWhatTheServerSaid(t *testing.T) {
	received := startTestReceiver(t)
	conn := &sessionConn{}
	received.issue(testTraceID, conn)

	span := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrGenAIToolName): "gitlab_environment",
		string(mcpotel.AttrActionID):      "environment.protected_get",
		string(mcpotel.AttrDomain):        "environment",
		string(mcpotel.AttrRefusalReason): "safe_mode",
		string(mcpotel.AttrErrorType):     "tool_error",
	}, tracepb.Status_STATUS_CODE_ERROR)

	if status := postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(span)), ""); status != http.StatusOK {
		t.Fatalf("the receiver answered %d, want 200", status)
	}

	kept, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the receiver kept nothing for a trace the harness issued")
	}
	record := kept.dispatch
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "tool", got: record.tool, want: "gitlab_environment"},
		{name: "action", got: record.action, want: "environment.protected_get"},
		{name: "domain", got: record.domain, want: "environment"},
		{name: "refusal reason", got: record.refusalReason, want: "safe_mode"},
		{name: "error type", got: record.errorType, want: "tool_error"},
		{name: "status", got: record.status, want: tracepb.Status_STATUS_CODE_ERROR.String()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("%s = %q, want %q", testCase.name, testCase.got, testCase.want)
			}
		})
	}
	if !received.observed.Load() {
		t.Error("the receiver did not mark itself as having observed a dispatch")
	}
	if !conn.dispatchObserved.Load() {
		t.Error("the server span landed and the session its call went to was not marked dispatch-observed")
	}
}

// TestSpanReceiver_TraceItNeverIssued_IsDropped keeps the map to the calls
// this harness made.
//
// A child exports every span it makes, including the ones its own startup
// produces. Keeping those would grow a map nothing ever reads and, worse,
// would let a span from work no test asked for answer a lookup.
func TestSpanReceiver_TraceItNeverIssued_IsDropped(t *testing.T) {
	received := startTestReceiver(t)

	span := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrActionID): "issue.list",
	}, tracepb.Status_STATUS_CODE_OK)
	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(span)), "")

	if _, arrived := received.lookup(testTraceID); arrived {
		t.Error("the receiver kept a trace the harness never stamped into a call")
	}
}

// TestSpanReceiver_ChildSpans_DoNotOverwriteTheServerSpan covers the shape
// every real trace has.
//
// One MCP call produces a server span carrying the action and a child span per
// GitLab request, and those children carry none of these attributes. Merging
// one would replace the facts with emptiness, which is a record that says a
// call dispatched nothing. They are counted instead, which is what turns "this
// package issued something" into "this action issued something".
func TestSpanReceiver_ChildSpans_DoNotOverwriteTheServerSpan(t *testing.T) {
	received := startTestReceiver(t)
	received.issue(testTraceID, nil)

	server := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrActionID): "issue.list",
		string(mcpotel.AttrDomain):   "issue",
	}, tracepb.Status_STATUS_CODE_OK)
	child := stubRequestSpan(testTraceID, http.MethodGet)

	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(child, server, child)), "")

	kept, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the receiver kept nothing for the trace")
	}
	if kept.dispatch.action != "issue.list" || kept.dispatch.domain != "issue" {
		t.Errorf("the record is %+v, want the server span's own facts", kept.dispatch)
	}
	if kept.requests != 2 {
		t.Errorf("the trace counted %d GitLab requests, want 2", kept.requests)
	}
}

// TestSpanReceiver_FailedRequestSpan_IsCountedAndNotMerged is the case that
// made the two kinds of span worth telling apart at all.
//
// A GitLab call that never got a response carries error.type, which is one of
// the five attributes the dispatch record is made of. Merged, it would report
// the action as having failed when the handler went on to answer, and it would
// mark the trace dispatch-observed before the server had said anything.
func TestSpanReceiver_FailedRequestSpan_IsCountedAndNotMerged(t *testing.T) {
	received := startTestReceiver(t)
	conn := &sessionConn{}
	received.issue(testTraceID, conn)

	failed := stubRequestSpan(testTraceID, http.MethodPost)
	failed.Attributes = append(failed.Attributes, &commonpb.KeyValue{
		Key:   string(mcpotel.AttrErrorType),
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "_OTHER"}},
	})
	failed.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR}

	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(failed)), "")

	kept, arrived := received.lookup(testTraceID)
	if arrived {
		t.Error("a GitLab request span alone marked the trace as dispatch-observed")
	}
	if kept.requests != 1 {
		t.Errorf("the trace counted %d GitLab requests, want 1", kept.requests)
	}
	if kept.dispatch.carriesFacts() {
		t.Errorf("the request span wrote %+v into the dispatch record", kept.dispatch)
	}
	if received.observed.Load() {
		t.Error("a request span alone told the receiver a dispatch had been observed")
	}
	if conn.dispatchObserved.Load() {
		t.Error("a request span alone marked the session dispatch-observed")
	}
}

// TestSpanReceiver_FailedOutboundMCPSpan_IsDroppedNotMerged covers the other
// producer of client spans, which the GitLab case left unguarded.
//
// internal/mcpotel.SendingMiddleware opens a client span per server-initiated
// request — an elicitation, a sampling call, a progress notification — and the
// suite drives all three. A failed one carries error.type and no
// http.request.method, so it is not a GitLab request and used to be merged:
// the dispatch record grew an error the action never had, lookup reported
// arrival from a span naming no action, and the receiver marked itself
// dispatch-observed. All three are asserted here because all three are
// silent — a call line written from an empty record still looks like a line.
func TestSpanReceiver_FailedOutboundMCPSpan_IsDroppedNotMerged(t *testing.T) {
	received := startTestReceiver(t)
	conn := &sessionConn{}
	received.issue(testTraceID, conn)

	postExport(t, received.url, "/v1/traces",
		marshalExport(t, exportOf(stubOutboundMCPSpan(testTraceID, "elicitation/create"))), "")

	kept, arrived := received.lookup(testTraceID)
	if arrived {
		t.Error("an outbound MCP span alone marked the trace as dispatch-observed")
	}
	if kept.dispatch.carriesFacts() {
		t.Errorf("the outbound MCP span wrote %+v into the dispatch record", kept.dispatch)
	}
	if kept.requests != 0 {
		t.Errorf("an outbound MCP span counted as %d GitLab requests, want 0", kept.requests)
	}
	if received.observed.Load() {
		t.Error("an outbound MCP span alone told the receiver a dispatch had been observed")
	}
	if conn.dispatchObserved.Load() {
		t.Error("an outbound MCP span alone marked the session dispatch-observed")
	}
}

// TestSpanReceiver_OutboundMCPSpanThatSucceeded_DoesNotArrive is the case the
// arrival rule made worth pinning on its own.
//
// The server's own span is recognized by mcp.method.name, and the span
// internal/mcpotel.SendingMiddleware opens for a server-initiated request
// carries that attribute too, on the trace of the tools/call that provoked it.
// A successful elicitation's span carries nothing else, so the kind is all
// that keeps it from being taken for the tools/call's own span: without it the
// trace would arrive before the tools/call's span did, the flush would write
// that call with no dispatched action, and the assertion about what ran would
// be skipped. The failed case above guards the same condition for a span that
// also carries error.type; this is the one a working elicitation produces.
func TestSpanReceiver_OutboundMCPSpanThatSucceeded_DoesNotArrive(t *testing.T) {
	received := startTestReceiver(t)
	conn := &sessionConn{}
	received.issue(testTraceID, conn)

	outbound := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrMCPMethodName):    "elicitation/create",
		string(mcpotel.AttrNetworkTransport): "pipe",
	}, tracepb.Status_STATUS_CODE_UNSET)
	outbound.Kind = tracepb.Span_SPAN_KIND_CLIENT
	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(outbound)), "")

	if _, arrived := received.lookup(testTraceID); arrived {
		t.Error("a successful outbound MCP span marked the trace as arrived before the server's own span")
	}
	if received.observed.Load() {
		t.Error("a successful outbound MCP span told the receiver a server span had been observed")
	}
	if conn.dispatchObserved.Load() {
		t.Error("a successful outbound MCP span marked the session dispatch-observed")
	}
}

// TestSpanReceiver_ServerSpanOfANonToolMethod_ArrivesWithoutDispatchFacts is
// the arrival this receiver used to refuse, and the reason every flush holding
// a resource read, a prompt or a completion waited ten seconds.
//
// The server spans every method it handles, and a resource read's span says
// which method it was and which resource it read, in attributes none of which
// is a dispatch fact. It has to arrive all the same, with an empty dispatch
// record, and mark the session its call went to: the session's telemetry works
// whatever it was asked.
func TestSpanReceiver_ServerSpanOfANonToolMethod_ArrivesWithoutDispatchFacts(t *testing.T) {
	received := startTestReceiver(t)
	conn := &sessionConn{}
	received.issue(testTraceID, conn)

	read := stubServerSpan(testTraceID, methodReadResource,
		map[string]string{string(mcpotel.AttrResourceRef): "digest-of-a-uri"}, tracepb.Status_STATUS_CODE_UNSET)
	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(read)), "")

	kept, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the server span of a resource read did not count as arriving")
	}
	if kept.dispatch.carriesFacts() || kept.dispatch.namesCall() {
		t.Errorf("a resource read's span wrote %+v into the dispatch record, want no dispatch facts", kept.dispatch)
	}
	if want := tracepb.Status_STATUS_CODE_UNSET.String(); kept.dispatch.status != want {
		t.Errorf("the dispatch status is %q, want the span's own %q", kept.dispatch.status, want)
	}
	if !received.observed.Load() {
		t.Error("a server span arrived and the receiver still says it has observed none")
	}
	if !conn.dispatchObserved.Load() {
		t.Error("a server span arrived and the session its call went to was not marked dispatch-observed")
	}
}

// TestSpanReceiver_SpanWithNeitherMethodNorFacts_IsDropped pins the lower edge
// of the server-span rule: a span of an issued trace that is not a client span
// but carries neither mcp.method.name nor a dispatch fact is somebody's
// internal span, and says nothing about whether the server reported on the
// call.
func TestSpanReceiver_SpanWithNeitherMethodNorFacts_IsDropped(t *testing.T) {
	received := startTestReceiver(t)
	conn := &sessionConn{}
	received.issue(testTraceID, conn)

	internal := stubSpan(testTraceID, map[string]string{"component": "cache"}, tracepb.Status_STATUS_CODE_OK)
	internal.Kind = tracepb.Span_SPAN_KIND_INTERNAL
	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(internal)), "")

	if len(received.all()) != 0 {
		t.Errorf("the receiver kept %d traces from a span that is neither a server span nor a GitLab request",
			len(received.all()))
	}
	if _, arrived := received.lookup(testTraceID); arrived {
		t.Error("a span carrying neither the method nor a dispatch fact marked the trace as arrived")
	}
	if conn.dispatchObserved.Load() {
		t.Error("a span carrying neither the method nor a dispatch fact marked the session dispatch-observed")
	}
}

// TestSpanReceiver_FailedOutboundMCPSpan_LeavesTheServerSpanIntact is the same
// defect seen from the side a reader of the record would notice it.
//
// An elicitation that the client refused fails the outbound request while the
// handler goes on to answer, so the trace carries both spans. Merging the
// outbound one reported the action as failed, which is a false negative in
// exactly the place the record exists to be believed.
func TestSpanReceiver_FailedOutboundMCPSpan_LeavesTheServerSpanIntact(t *testing.T) {
	received := startTestReceiver(t)
	received.issue(testTraceID, nil)

	server := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrActionID): "issue.create",
		string(mcpotel.AttrDomain):   "issue",
	}, tracepb.Status_STATUS_CODE_OK)
	outbound := stubOutboundMCPSpan(testTraceID, "elicitation/create")

	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(outbound, server, outbound)), "")

	kept, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the receiver kept nothing for the trace")
	}
	if kept.dispatch.action != "issue.create" {
		t.Errorf("the record names action %q, want issue.create", kept.dispatch.action)
	}
	if kept.dispatch.errorType != "" {
		t.Errorf("the outbound MCP span wrote error type %q over a successful dispatch", kept.dispatch.errorType)
	}
	if kept.dispatch.status != tracepb.Status_STATUS_CODE_OK.String() {
		t.Errorf("the dispatch status is %q, want %q", kept.dispatch.status, tracepb.Status_STATUS_CODE_OK.String())
	}
}

// TestSpanReceiver_EmptyTraceID_IsNeitherIssuedNorKept pins the invariant the
// flush's lookup leans on: nothing is ever held under the empty trace id, so a
// call that carries no trace can be looked up like any other and is never
// reported as arrived. Neither half may break it: an empty id is not issued,
// and a span with no trace id is dropped however much it looks like the
// server's own.
//
// Each half has a subtest of its own, because with both guards in place a
// test of the two together cannot see the second: a span with no trace id is
// dropped by the issued lookup anyway, since the first guard left nothing
// issued under the empty id. The second subtest therefore puts an entry there
// by hand, which is the state the absorb guard alone has to answer.
func TestSpanReceiver_EmptyTraceID_IsNeitherIssuedNorKept(t *testing.T) {
	untracedSpan := func() *tracepb.Span {
		span := stubServerSpan(testTraceID, methodReadResource, nil, tracepb.Status_STATUS_CODE_UNSET)
		span.TraceId = nil
		return span
	}
	assertNothingArrived := func(t *testing.T, received *spanReceiver, conn *sessionConn) {
		t.Helper()
		if len(received.all()) != 0 {
			t.Errorf("the receiver kept %d trace(s) from a span carrying no trace id", len(received.all()))
		}
		if _, arrived := received.lookup(""); arrived {
			t.Error("the empty trace id reads as arrived")
		}
		if conn.dispatchObserved.Load() || received.observed.Load() {
			t.Error("a span carrying no trace id marked the session or the receiver as observed")
		}
	}

	t.Run("an empty id is not issued", func(t *testing.T) {
		received := startTestReceiver(t)
		conn := &sessionConn{}

		received.issue("", conn)
		received.absorbSpan(untracedSpan())

		received.mu.Lock()
		issued := len(received.issued)
		received.mu.Unlock()
		if issued != 0 {
			t.Errorf("the receiver issued %d trace(s) for an empty id", issued)
		}
		assertNothingArrived(t, received, conn)
	})
	t.Run("a span with no trace id is dropped even under an issued empty id", func(t *testing.T) {
		received := startTestReceiver(t)
		conn := &sessionConn{}
		received.mu.Lock()
		received.issued[""] = conn
		received.mu.Unlock()

		received.absorbSpan(untracedSpan())

		assertNothingArrived(t, received, conn)
	})
}

// TestSpanReceiver_RequestSpanOnAnUnissuedTrace_IsDropped keeps the count to
// the calls this harness made.
//
// The server makes GitLab calls of its own at startup — the tier probe and the
// scope probe both go through the instrumented transport — and those are on
// traces nobody stamped. Counting them would put requests on a trace no action
// names, and they are dropped for the same reason a stray server span is.
func TestSpanReceiver_RequestSpanOnAnUnissuedTrace_IsDropped(t *testing.T) {
	received := startTestReceiver(t)

	postExport(t, received.url, "/v1/traces",
		marshalExport(t, exportOf(stubRequestSpan(testTraceID, http.MethodGet))), "")

	if len(received.all()) != 0 {
		t.Errorf("the receiver kept %d traces it never issued", len(received.all()))
	}
}

// TestSpanReceiver_ServerSpanCarryingAMethod_IsNotCountedAsARequest pins the
// half of the discriminator that is not the attribute.
//
// The HTTP server middleware records http.request.method too, on the span it
// opens for an inbound POST. That span is a root of its own trace today,
// because the harness stamps its traceparent into _meta rather than into a
// header, and a change on either side would put it on this trace. The kind is
// what keeps it out of the count whatever happens there.
func TestSpanReceiver_ServerSpanCarryingAMethod_IsNotCountedAsARequest(t *testing.T) {
	received := startTestReceiver(t)
	received.issue(testTraceID, nil)

	inbound := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrHTTPRequestMethod): http.MethodPost,
		string(mcpotel.AttrActionID):          "issue.list",
	}, tracepb.Status_STATUS_CODE_OK)
	inbound.Kind = tracepb.Span_SPAN_KIND_SERVER

	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(inbound)), "")

	kept, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the receiver kept nothing for the trace")
	}
	if kept.requests != 0 {
		t.Errorf("a server span counted as %d GitLab requests, want 0", kept.requests)
	}
}

// TestSpanReceiver_MetricsAndLogs_AreAcknowledgedAndDropped pins why the
// catch-all answers 200.
//
// Every child exports its metrics and its logs to the same endpoint. Refusing
// them would make the exporter retry the same payload for the rest of the run,
// which costs the server under test a background goroutine and its log a
// warning per batch, for signals nothing here reads.
func TestSpanReceiver_MetricsAndLogs_AreAcknowledgedAndDropped(t *testing.T) {
	received := startTestReceiver(t)

	for _, path := range []string{"/v1/metrics", "/v1/logs"} {
		t.Run(path, func(t *testing.T) {
			if status := postExport(t, received.url, path, []byte("not a trace export"), ""); status != http.StatusOK {
				t.Errorf("the receiver answered %d for %s, want 200", status, path)
			}
		})
	}
	if len(received.all()) != 0 {
		t.Errorf("the receiver kept %d traces from payloads that carried none", len(received.all()))
	}
}

// TestSpanReceiver_UndecodableTrace_IsRefused checks that a payload the
// receiver cannot read is reported rather than silently counted as an export
// that said nothing.
func TestSpanReceiver_UndecodableTrace_IsRefused(t *testing.T) {
	received := startTestReceiver(t)

	if status := postExport(t, received.url, "/v1/traces", []byte("this is not protobuf at all"), ""); status != http.StatusBadRequest {
		t.Errorf("the receiver answered %d for a payload that is not an export, want 400", status)
	}
	if status := postExport(t, received.url, "/v1/traces", []byte("not gzip either"), "gzip"); status != http.StatusBadRequest {
		t.Errorf("the receiver answered %d for a body that claims gzip and is not, want 400", status)
	}
}

// TestSpanReceiver_GzippedExport_IsRead covers the compression the exporter
// does not use today.
//
// The Go OTLP/HTTP exporter sends uncompressed protobuf unless it is told
// otherwise, and this suite does not tell it otherwise. Handling gzip anyway
// removes a way for the whole record to go silently empty if that default ever
// changes, and this is what keeps that handling honest.
func TestSpanReceiver_GzippedExport_IsRead(t *testing.T) {
	received := startTestReceiver(t)
	received.issue(testTraceID, nil)

	span := stubSpan(testTraceID, map[string]string{string(mcpotel.AttrActionID): "issue.list"},
		tracepb.Status_STATUS_CODE_OK)

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(marshalExport(t, exportOf(span))); err != nil {
		t.Fatalf("compressing the export: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the compressor: %v", err)
	}

	if status := postExport(t, received.url, "/v1/traces", compressed.Bytes(), "gzip"); status != http.StatusOK {
		t.Fatalf("the receiver answered %d for a gzipped export, want 200", status)
	}
	if kept, arrived := received.lookup(testTraceID); !arrived || kept.dispatch.action != "issue.list" {
		t.Errorf("the gzipped export was not read: arrived=%t record=%+v", arrived, kept.dispatch)
	}
}

// TestDispatchRecord_Merge_KeepsTheFirstNonEmptyValue pins the rule that lets
// several spans of one trace add up.
func TestDispatchRecord_Merge_KeepsTheFirstNonEmptyValue(t *testing.T) {
	first := dispatchRecord{action: "issue.list", status: "STATUS_CODE_OK"}
	second := dispatchRecord{action: "issue.get", domain: "issue", errorType: "tool_error"}

	merged := first.merge(second)

	if merged.action != "issue.list" {
		t.Errorf("action = %q, want the first span's own", merged.action)
	}
	if merged.domain != "issue" || merged.errorType != "tool_error" {
		t.Errorf("the merge dropped what only the second span carried: %+v", merged)
	}
	if merged.status != "STATUS_CODE_OK" {
		t.Errorf("status = %q, want the first span's own", merged.status)
	}
}

// TestDispatchRecord_CarriesFacts_ReadsEachOfTheFiveAttributes checks the
// half of the server-span rule a span built by hand relies on.
//
// It no longer decides arrival, which is what it was named for: a resource
// read's span carries none of these and arrives all the same. What it still
// decides is whether a non-client span with no mcp.method.name is taken for
// the server's, so each attribute is a case of its own, and the status is the
// one that must not count, since every span has one.
func TestDispatchRecord_CarriesFacts_ReadsEachOfTheFiveAttributes(t *testing.T) {
	cases := []struct {
		name   string
		record dispatchRecord
		want   bool
	}{
		{name: "nothing at all", record: dispatchRecord{}, want: false},
		{name: "a status and nothing else", record: dispatchRecord{status: "STATUS_CODE_OK"}, want: false},
		{name: "a tool", record: dispatchRecord{tool: "gitlab_find_action"}, want: true},
		{name: "an action", record: dispatchRecord{action: "issue.list"}, want: true},
		{name: "a domain", record: dispatchRecord{domain: "issue"}, want: true},
		{name: "a refusal", record: dispatchRecord{refusalReason: "safe_mode"}, want: true},
		{name: "an error type", record: dispatchRecord{errorType: "-32603"}, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.record.carriesFacts(); got != testCase.want {
				t.Errorf("carriesFacts() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestDispatchRecord_NamesCall_IsTheToolOrTheAction checks the predicate a
// dispatch line is written by.
//
// A span naming a tool alone is kept, because that is how a call to the find
// tool reads and it dispatches no action; a span naming only what went wrong,
// which is all a failed resource read's span can say, is not.
func TestDispatchRecord_NamesCall_IsTheToolOrTheAction(t *testing.T) {
	cases := []struct {
		name   string
		record dispatchRecord
		want   bool
	}{
		{name: "nothing at all", record: dispatchRecord{}, want: false},
		{name: "a tool and no action", record: dispatchRecord{tool: "gitlab_find_action"}, want: true},
		{name: "an action and no tool", record: dispatchRecord{action: "issue.list"}, want: true},
		{
			name:   "a failure of a call that named neither",
			record: dispatchRecord{domain: "issue", refusalReason: "safe_mode", errorType: "-32603", status: "STATUS_CODE_ERROR"},
			want:   false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.record.namesCall(); got != testCase.want {
				t.Errorf("namesCall() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestTelemetryVariables_PointEveryChildAtThisProcessesReceiver pins the
// environment that makes the dispatch half of the record exist.
//
// The schedule delay is an integer because the specification defines every
// OTEL_ duration in milliseconds: writing 100ms parses as nothing and silently
// keeps the five-second default, which is longer than a test's whole flush.
func TestTelemetryVariables_PointEveryChildAtThisProcessesReceiver(t *testing.T) {
	vars := telemetryVariables()

	if vars["GITLAB_MCP_TELEMETRY"] != "true" {
		t.Errorf("GITLAB_MCP_TELEMETRY = %q, want true", vars["GITLAB_MCP_TELEMETRY"])
	}
	if vars["OTEL_EXPORTER_OTLP_PROTOCOL"] != "http/protobuf" {
		t.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL = %q, want http/protobuf", vars["OTEL_EXPORTER_OTLP_PROTOCOL"])
	}
	if !strings.HasPrefix(vars["OTEL_EXPORTER_OTLP_ENDPOINT"], "http://127.0.0.1:") {
		t.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT = %q, want a loopback endpoint", vars["OTEL_EXPORTER_OTLP_ENDPOINT"])
	}
	if vars["OTEL_BSP_SCHEDULE_DELAY"] != "100" {
		t.Errorf("OTEL_BSP_SCHEDULE_DELAY = %q, want 100 milliseconds as an integer",
			vars["OTEL_BSP_SCHEDULE_DELAY"])
	}
}

// TestSpanReceiver_IsReachedOnLoopbackOnly checks that nothing outside this
// machine can post to a suite's receiver or read what it holds.
func TestSpanReceiver_IsReachedOnLoopbackOnly(t *testing.T) {
	received := startTestReceiver(t)

	if !strings.HasPrefix(received.url, "http://127.0.0.1:") {
		t.Errorf("the receiver listens on %q, want loopback", received.url)
	}
}
