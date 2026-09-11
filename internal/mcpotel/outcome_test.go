package mcpotel

import (
	"maps"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/telemetry"
)

// realSubscribeFailure is the message the hosted deployment produced when a
// subscribe named a resource it could not read.
//
// Kept verbatim rather than invented, because the point of the test is that a
// message this server writes itself can carry a project path, and a made-up
// example would be arguing the case rather than showing it.
const realSubscribeFailure = "subscriptions: initial read of gitlab://project/jmrp%2Fgitlab-mcp-server: " +
	"subscriptions: resource inaccessible: Resource not found"

// TestStatusDescription_CarriesNoResourceURI covers a second carrier for a value
// the identity policy governs everywhere else.
//
// Which resource a request named goes on an attribute that the policy chooses:
// a keyed digest by default, the URI itself only under full. A status
// description is free text, so a URI embedded in a message arrives whatever the
// policy said. One governed carrier and one ungoverned one is not a policy.
//
// The code previously asserted the opposite in a comment: that these messages
// are safe because this server writes them rather than GitLab. True about where
// the text comes from, and not about what it contains.
func TestStatusDescription_CarriesNoResourceURI(t *testing.T) {
	span := runOnce(t, Options{}, "resources/subscribe",
		&mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: "gitlab://project/82077663"}},
		nil,
		// -32603 rather than a caller-fault code: those record no description
		// at all, which is why this leak was invisible in production traffic.
		&jsonrpc.Error{Code: -32603, Message: realSubscribeFailure},
	)

	description := span.Status().Description
	if description == "" {
		t.Fatal("a failed call recorded no status description; the convention asks for the JSON-RPC message")
	}
	if strings.Contains(description, "jmrp%2Fgitlab-mcp-server") {
		t.Errorf("the status description carries the project path:\n%s", description)
	}
	// The diagnosis has to survive: replacing the URI is the point, dropping
	// the message would take the reason with it.
	if !strings.Contains(description, "resource inaccessible") {
		t.Errorf("the redaction removed the diagnosis as well:\n%s", description)
	}
}

// TestRedactResourceURIs_LeavesEverythingElseAlone keeps the substitution from
// becoming a blunt instrument that eats ordinary messages.
func TestRedactResourceURIs_LeavesEverythingElseAlone(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{
			name: "no URI is untouched",
			in:   "rate limit exceeded, retry after 30s",
			want: "rate limit exceeded, retry after 30s",
		},
		{
			name: "a bare URI",
			in:   "cannot read gitlab://project/1",
			want: "cannot read gitlab://[redacted]",
		},
		{
			name: "two URIs in one message",
			in:   "gitlab://project/1 and gitlab://group/2 both failed",
			want: "gitlab://[redacted] and gitlab://[redacted] both failed",
		},
		{
			name: "a quoted URI stops at the quote",
			in:   `read "gitlab://project/1" failed`,
			want: `read "gitlab://[redacted]" failed`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactResourceURIs(tc.in); got != tc.want {
				t.Errorf("redactResourceURIs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestStatusDescription_SurvivesAnOrdinaryFailure guards the other direction:
// the description is still recorded, so the redaction did not silently become a
// removal.
func TestStatusDescription_SurvivesAnOrdinaryFailure(t *testing.T) {
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_execute_action", nil, nil), nil,
		&jsonrpc.Error{Code: -32603, Message: "internal error"})

	if got := span.Status().Description; got != "internal error" {
		t.Errorf("status description = %q, want the JSON-RPC message unchanged", got)
	}
}

// TestOutcome_MetricAttributes_CarryWhatTheResponseSaidAndNothingElse verifies
// the outcome's contribution to the duration metric for each kind of ending.
//
// A label present with an empty value is a series of its own, so a success
// must add neither key rather than two empty ones, and a caller fault must add
// its status code without an error.type it was never given. Getting this wrong
// in either direction splits one population across series, or merges a failed
// one into the successes, and neither shows as an error anywhere.
func TestOutcome_MetricAttributes_CarryWhatTheResponseSaidAndNothingElse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome outcome
		want    map[attribute.Key]string
	}{
		{name: "a success adds nothing", outcome: classify(&mcp.CallToolResult{}, nil), want: map[attribute.Key]string{}},
		{
			name:    "a caller fault adds its code alone",
			outcome: classify(nil, &jsonrpc.Error{Code: -32601, Message: "no"}),
			want:    map[attribute.Key]string{AttrRPCResponseStatusCode: "-32601"},
		},
		{
			name:    "a server error adds its code and its classification",
			outcome: classify(nil, &jsonrpc.Error{Code: -32603, Message: "no"}),
			want:    map[attribute.Key]string{AttrErrorType: "-32603", AttrRPCResponseStatusCode: "-32603"},
		},
		{
			name:    "a tool error adds its classification alone",
			outcome: classify(&mcp.CallToolResult{IsError: true}, nil),
			want:    map[attribute.Key]string{AttrErrorType: ErrorTypeToolError},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := map[attribute.Key]string{}
			for _, kv := range tt.outcome.metricAttributes() {
				got[kv.Key] = kv.Value.AsString()
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("metric attributes = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOutcome_NameIsUnverified_ForBothAnswersToAnUnknownName verifies the two
// codes that let the metric bucket a caller-chosen tool or prompt name.
//
// The SDK answers a name that resolves to nothing with method-not-found or with
// invalid-params depending on the method, so either must bucket the name. A
// server error is not one of them: the handler ran, so the name was real, and
// bucketing it would hide which tool is failing.
func TestOutcome_NameIsUnverified_ForBothAnswersToAnUnknownName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "method not found", err: &jsonrpc.Error{Code: -32601, Message: "no"}, want: true},
		{name: "invalid params", err: &jsonrpc.Error{Code: -32602, Message: "no"}, want: true},
		{name: "an internal error", err: &jsonrpc.Error{Code: -32603, Message: "no"}, want: false},
		{name: "a success", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := classify(&mcp.CallToolResult{}, tt.err).nameIsUnverified(); got != tt.want {
				t.Errorf("nameIsUnverified = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsErrorResult_ATypedNilIsNotAFailure verifies that a Result holding a nil
// *CallToolResult is read as no failure rather than dereferenced.
//
// An interface holding a typed nil pointer is not a nil interface, which is the
// shape that once panicked a hundred times a day in paramsOf. A handler that
// returns a nil result with no error hands the middleware exactly that, and a
// classification that read IsError through it would panic on the one path every
// tool call takes.
func TestIsErrorResult_ATypedNilIsNotAFailure(t *testing.T) {
	t.Parallel()

	var nothing *mcp.CallToolResult
	if isErrorResult(nothing) {
		t.Error("a nil CallToolResult was classified as a tool error")
	}
}

// TestRedactResourceURIs_AgreesWithTheHandler guards a rule stated in two
// places, which is a rule that drifts.
//
// The same substitution runs on a span's status description here and on every
// exported log record in internal/telemetry, and neither package can import the
// other: this one takes the OpenTelemetry API and never the SDK, and that one is
// built on the SDK. A test can import both, so the constraint is checkable even
// though the production code cannot state it.
func TestRedactResourceURIs_AgreesWithTheHandler(t *testing.T) {
	inputs := []string{
		"",
		"nothing to redact",
		"read gitlab://project/1",
		`read "gitlab://project/1/mr/2" failed`,
		"gitlab://project/1 and gitlab://group/2",
		"subscriptions: initial read of gitlab://project/jmrp%2Fgitlab-mcp-server: not found",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			here := redactResourceURIs(input)
			there := telemetry.RedactResourceURIs(input)
			if here != there {
				t.Errorf("the two implementations disagree on %q: mcpotel gives %q and telemetry gives %q",
					input, here, there)
			}
		})
	}
}
