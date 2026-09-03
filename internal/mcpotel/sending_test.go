package mcpotel

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// sendOnce drives the sending middleware for one outbound call.
func sendOnce(t *testing.T, method string, handlerErr error) sdktrace.ReadOnlySpan {
	t.Helper()

	recorder := newRecorder(t)
	handler := SendingMiddleware(Options{Surface: "dynamic", Transport: "pipe"})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, handlerErr
		},
	)
	_, _ = handler(context.Background(), method, callToolRequest("unused", nil, nil))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want exactly 1", len(spans))
	}
	return spans[0]
}

// TestSendingMiddleware_IsClientKind pins the half of the design that is easiest
// to get backwards.
//
// The MCP convention splits client and server spans by INITIATOR, not by role.
// This server receives a tools/call and initiates an elicitation, so one process
// produces both kinds. Recording an elicitation as SERVER would put this server
// on the wrong side of its own trace: a service map would show inbound traffic
// that never arrived.
func TestSendingMiddleware_IsClientKind(t *testing.T) {
	span := sendOnce(t, "elicitation/create", nil)

	if got := span.SpanKind(); got != trace.SpanKindClient {
		t.Errorf("span kind = %v, want CLIENT for a request this server initiated", got)
	}
	if got := span.Name(); got != "elicitation/create" {
		t.Errorf("span name = %q, want the method", got)
	}
	value, ok := attrOf(span, AttrMCPMethodName)
	if !ok || value.AsString() != "elicitation/create" {
		t.Errorf("%s = %v, want the method name", AttrMCPMethodName, value.AsString())
	}
}

// TestSendingMiddleware_EveryErrorCodeIsAFailure is the rule that differs from
// the receiving side, asserted on the exact codes the receiving side exempts.
//
// "All JSON-RPC error codes SHOULD be considered errors" is the client-side
// rule, and it contradicts the server-side exemption deliberately: a
// method-not-found is not a receiver's failure, but when WE are the caller a
// client that cannot serve our elicitation is precisely what an operator needs
// to see. If these ever start passing as non-failures, the two rules have been
// folded into one.
func TestSendingMiddleware_EveryErrorCodeIsAFailure(t *testing.T) {
	for _, code := range []int64{-32700, -32600, -32601, -32602, -32002, -32603} {
		t.Run(codeString(code), func(t *testing.T) {
			span := sendOnce(t, "elicitation/create", &jsonrpc.Error{Code: code, Message: "no"})

			if got := span.Status().Code; got != codes.Error {
				t.Errorf("status = %v for code %d, want Error on the client side", got, code)
			}
			value, ok := attrOf(span, AttrErrorType)
			if !ok {
				t.Fatalf("error.type is absent for code %d", code)
			}
			if value.AsString() != codeString(code) {
				t.Errorf("error.type = %q, want %q", value.AsString(), codeString(code))
			}
		})
	}
}

// TestSendingMiddleware_TheReceivingSideStillExempts guards the pair rather
// than either half.
//
// The two rules are one call apart and opposite. This drives the same code
// through both middlewares and asserts they disagree, which is the only
// assertion that fails if somebody unifies them for tidiness.
func TestSendingMiddleware_TheReceivingSideStillExempts(t *testing.T) {
	const callerFault = -32601

	sent := sendOnce(t, "elicitation/create", &jsonrpc.Error{Code: callerFault, Message: "no"})
	received := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		nil, &jsonrpc.Error{Code: callerFault, Message: "no"})

	if sent.Status().Code != codes.Error {
		t.Errorf("a client-side %d is not a failure; the strict rule was lost", callerFault)
	}
	if received.Status().Code != codes.Unset {
		t.Errorf("a server-side %d is a failure; the exemption was lost and every model typo now counts as a server error", callerFault)
	}
}

// TestSendingMiddleware_SuccessLeavesTheStatusUnset covers the MUST that
// applies to every span whatever its kind: status is left unset when nothing
// failed, and Ok is never set.
func TestSendingMiddleware_SuccessLeavesTheStatusUnset(t *testing.T) {
	span := sendOnce(t, "notifications/resources/updated", nil)

	if got := span.Status().Code; got != codes.Unset {
		t.Errorf("status = %v on a delivered notification, want Unset", got)
	}
	if _, ok := attrOf(span, AttrErrorType); ok {
		t.Error("error.type is set on a successful send")
	}
}

// TestSendingMiddleware_TransportFailureIsClassifiedAsOther pins the fallback
// for the failure a notification can actually have.
//
// A notification carries no response and therefore no error code, so the only
// thing that can go wrong is delivery. The Go error text must not become
// error.type: the registry requires that attribute to be predictable and low
// cardinality, and a transport error carries an address.
func TestSendingMiddleware_TransportFailureIsClassifiedAsOther(t *testing.T) {
	span := sendOnce(t, "notifications/resources/updated",
		errors.New("write tcp 10.0.0.4:52134->10.0.0.9:443: broken pipe"))

	value, ok := attrOf(span, AttrErrorType)
	if !ok {
		t.Fatal("error.type is absent on a failed send")
	}
	if value.AsString() != ErrorTypeOther {
		t.Errorf("error.type = %q, want %q; the transport error text must not become the classification",
			value.AsString(), ErrorTypeOther)
	}
}

// TestIsNotification covers the only way to tell a notification from a request
// by name, which is the protocol's own prefix convention.
func TestIsNotification(t *testing.T) {
	for method, want := range map[string]bool{
		"notifications/resources/updated":    true,
		"notifications/tools/list_changed":   true,
		"notifications/progress":             true,
		"elicitation/create":                 false,
		"sampling/createMessage":             false,
		"roots/list":                         false,
		"tools/call":                         false,
		"":                                   false,
		"notifications":                      false,
		"my/notifications/resources/updated": false,
	} {
		t.Run(method, func(t *testing.T) {
			if got := IsNotification(method); got != want {
				t.Errorf("IsNotification(%q) = %v, want %v", method, got, want)
			}
		})
	}
}

// TestSendingMiddleware_APanickingHandler_IsStillMeasured covers the deferred
// record, which exists for the one outcome the ordinary path cannot report.
//
// A panic unwinding through the middleware skips the record call below it, so
// without this the client instrument would undercount exactly the calls most
// worth counting: the span would end with no outcome and no measurement would
// be taken. The panic itself is deliberately not recovered here — it is
// re-raised past the middleware, and this test catches it the way the caller
// above would have to.
func TestSendingMiddleware_APanickingHandler_IsStillMeasured(t *testing.T) {
	recorder := newRecorder(t)
	handler := SendingMiddleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			panic("the handler below the middleware panicked")
		},
	)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the middleware swallowed the panic; whoever is below it must decide what a panic means")
			}
		}()
		_, _ = handler(context.Background(), "elicitation/create", &mcp.ListToolsRequest{})
	}()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want the one the panic unwound through", len(spans))
	}
	if got := spans[0].Status().Code; got != codes.Error {
		t.Errorf("span status = %v, want an error: a panicking call is not a success", got)
	}
	if _, ok := attrOf(spans[0], AttrErrorType); !ok {
		t.Errorf("%s is absent; the outcome of a panicking call was never classified", AttrErrorType)
	}
}
