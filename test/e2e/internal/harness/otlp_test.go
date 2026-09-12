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
	received.issue(testTraceID)

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

	record, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the receiver kept nothing for a trace the harness issued")
	}
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
// call dispatched nothing.
func TestSpanReceiver_ChildSpans_DoNotOverwriteTheServerSpan(t *testing.T) {
	received := startTestReceiver(t)
	received.issue(testTraceID)

	server := stubSpan(testTraceID, map[string]string{
		string(mcpotel.AttrActionID): "issue.list",
		string(mcpotel.AttrDomain):   "issue",
	}, tracepb.Status_STATUS_CODE_OK)
	child := stubSpan(testTraceID, map[string]string{"http.request.method": "GET"}, tracepb.Status_STATUS_CODE_UNSET)

	postExport(t, received.url, "/v1/traces", marshalExport(t, exportOf(child, server, child)), "")

	record, arrived := received.lookup(testTraceID)
	if !arrived {
		t.Fatal("the receiver kept nothing for the trace")
	}
	if record.action != "issue.list" || record.domain != "issue" {
		t.Errorf("the record is %+v, want the server span's own facts", record)
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
	received.issue(testTraceID)

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
	if record, arrived := received.lookup(testTraceID); !arrived || record.action != "issue.list" {
		t.Errorf("the gzipped export was not read: arrived=%t record=%+v", arrived, record)
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

// TestDispatchRecord_CarriesFacts_IsWhatDecidesASpanIsWorthKeeping checks the
// predicate the child-span rule rests on.
func TestDispatchRecord_CarriesFacts_IsWhatDecidesASpanIsWorthKeeping(t *testing.T) {
	cases := []struct {
		name   string
		record dispatchRecord
		want   bool
	}{
		{name: "nothing at all", record: dispatchRecord{}, want: false},
		{name: "a status and nothing else", record: dispatchRecord{status: "STATUS_CODE_OK"}, want: false},
		{name: "an action", record: dispatchRecord{action: "issue.list"}, want: true},
		{name: "a refusal", record: dispatchRecord{refusalReason: "safe_mode"}, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.record.carriesFacts(); got != testCase.want {
				t.Errorf("carriesFacts() = %t, want %t", got, testCase.want)
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
