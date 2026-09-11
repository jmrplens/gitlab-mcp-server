package mcpotel

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/trace"
)

// TestMetaCarrier_KeysNamesOnlyThePropagationKeys covers the method that decides
// what a propagator believes is present.
//
// It is asked for the keys this carrier can supply, not for everything in
// _meta, and the distinction matters: _meta carries the protocol version and
// whatever else a client put there, and reporting those as propagation keys
// would describe a carrier that cannot deliver them.
func TestMetaCarrier_KeysNamesOnlyThePropagationKeys(t *testing.T) {
	carrier := metaCarrier{meta: map[string]any{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"baggage":     "key=value",
		// Present, and not a propagation key.
		"io.modelcontextprotocol/protocolVersion": "2026-07-28",
		// Present under the right name and the wrong type, which the getter
		// already declines, so the key list must decline it too.
		"tracestate": 42,
	}}

	keys := carrier.Keys()

	want := map[string]bool{"traceparent": true, "baggage": true}
	for _, key := range keys {
		if !want[key] {
			t.Errorf("Keys reported %q, which this carrier cannot supply", key)
		}
		delete(want, key)
	}
	for missing := range want {
		t.Run(missing, func(t *testing.T) {
			t.Errorf("Keys did not report %q, which is present and readable", missing)
		})
	}
}

// TestMetaCarrier_SetIsInert pins the deliberate emptiness, which is a decision
// rather than an omission.
//
// Writing a traceparent into a response would hand every caller the identifiers
// of this server's internal spans, which is the outward leak the W3C security
// section warns about. The propagator interface requires the method; nothing
// here may implement it.
func TestMetaCarrier_SetIsInert(t *testing.T) {
	meta := map[string]any{}
	carrier := metaCarrier{meta: meta}

	carrier.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	if len(meta) != 0 {
		t.Errorf("Set wrote %v; a response must not carry this server's span identifiers", meta)
	}
}

// TestParamsOf_ATypedNilIsNotParams pins the distinction production found the
// hard way.
//
// The SDK hands a receiving middleware a Params interface holding a typed nil
// pointer whenever the wire omitted the params member, which is allowed for
// every list method and for notifications/initialized. An interface holding a
// nil pointer is not a nil interface, so the obvious check passes and the next
// method call dereferences the pointer.
func TestParamsOf_ATypedNilIsNotParams(t *testing.T) {
	t.Parallel()

	for name, req := range requestsWithoutParams() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// The premise. If the SDK ever starts handing out a genuinely nil
			// interface, this test should say so rather than keep passing for
			// a reason that no longer holds.
			if req.GetParams() == nil {
				t.Fatal("GetParams already returns a nil interface; the case being guarded no longer exists")
			}
			if got := paramsOf(req); got != nil {
				t.Errorf("paramsOf returned %#v for a typed nil, so callers will dereference it", got)
			}
		})
	}
}

// TestMiddleware_AListRequestWithoutParamsIsServed is the whole middleware
// against the shape that was panicking.
//
// A hosted deployment logged "recovered a panic while handling a request" a
// hundred times in a day, on tools/list, prompts/list, resources/list and
// notifications/initialized: every method a client issues at startup. The
// middleware is installed whether or not telemetry is exported, so the failure
// never depended on the feature being on.
//
// Nothing here recovers, on purpose. In the server the panic is recovered too,
// and being recovered is exactly what let it run for a day unnoticed.
func TestMiddleware_AListRequestWithoutParamsIsServed(t *testing.T) {
	for name, req := range requestsWithoutParams() {
		t.Run(name, func(t *testing.T) {
			reached := false
			handler := Middleware(Options{ProtocolVersions: admitted})(
				func(context.Context, string, mcp.Request) (mcp.Result, error) {
					reached = true
					return &mcp.ListToolsResult{}, nil
				},
			)

			if _, err := handler(context.Background(), name, req); err != nil {
				t.Fatalf("handling %s: %v", name, err)
			}
			if !reached {
				t.Error("the request never reached the handler below the middleware")
			}
		})
	}
}

// requestsWithoutParams returns one request per method whose params the wire is
// allowed to omit, in the shape the SDK builds when it does.
func requestsWithoutParams() map[string]mcp.Request {
	return map[string]mcp.Request{
		"tools/list":     &mcp.ListToolsRequest{},
		"prompts/list":   &mcp.ListPromptsRequest{},
		"resources/list": &mcp.ListResourcesRequest{},
	}
}

// traceStateOf parses a tracestate a test built, failing when the W3C parser
// refuses it: a refused value reaches the bound as an empty state, and a test
// of the bound would then pass on nothing.
func traceStateOf(t *testing.T, raw string) trace.TraceState {
	t.Helper()

	state, err := trace.ParseTraceState(raw)
	if err != nil {
		t.Fatalf("the W3C parser refused the test's tracestate: %v", err)
	}
	return state
}

// TestBoundTraceState_TruncatesToTheBoundAndNoFurther verifies both edges of the
// tracestate bound: a value at the bound is left alone and reported unchanged,
// and a value over it loses whole entries from the end until it fits.
//
// Each edge is its own way to be wrong. Dropping an entry that already fit
// throws away vendor state the bound was chosen to keep, and reporting a change
// that did not happen makes the caller rebuild a context for nothing. At the
// other end, a head entry can exceed the bound by itself, since W3C allows a
// 256-byte key beside a 256-byte value, and keeping the head is then no reason
// to exceed the bound.
func TestBoundTraceState_TruncatesToTheBoundAndNoFurther(t *testing.T) {
	t.Parallel()

	// Two members whose encoding is exactly the bound: "a=" and 256 bytes, the
	// comma, then "b=" and whatever is left.
	atTheBound := "a=" + strings.Repeat("v", 256) + ",b=" + strings.Repeat("v", maxTraceStateBytes-2-256-3)
	if len(atTheBound) != maxTraceStateBytes {
		t.Fatalf("the fixture is %d bytes, want exactly the %d-byte bound", len(atTheBound), maxTraceStateBytes)
	}

	tests := []struct {
		name          string
		raw           string
		want          string
		wantTruncated bool
	}{
		{
			name: "a tracestate exactly at the bound is left alone",
			raw:  atTheBound,
			want: atTheBound,
		},
		{
			name:          "one entry over the bound loses that entry and no more",
			raw:           atTheBound + ",c=x",
			want:          atTheBound,
			wantTruncated: true,
		},
		{
			name:          "a head entry larger than the bound goes too",
			raw:           strings.Repeat("k", 256) + "=" + strings.Repeat("v", 256),
			want:          "",
			wantTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bounded, truncated := boundTraceState(traceStateOf(t, tt.raw))
			if got := bounded.String(); got != tt.want {
				t.Errorf("bounded to %d bytes, want the %d-byte %.40q...", len(got), len(tt.want), tt.want)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
		})
	}
}

// TestSanitizeRemoteContext_ALocalParentIsLeftExactlyAsItIs verifies that the
// bounds apply only to a span context that arrived from outside the process.
//
// An ambient span this server created is its own: its sampling decision came
// from this process's sampler and its tracestate from this process's code, so
// neither is a caller's claim to bound. Treating it as remote would also stamp
// it remote, which tells a sampler an inbound trace arrived when none did.
func TestSanitizeRemoteContext_ALocalParentIsLeftExactlyAsItIs(t *testing.T) {
	t.Parallel()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("parsing the trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("parsing the span id: %v", err)
	}
	// Unsampled and over the tracestate bound: both are things the sanitizer
	// changes on a remote parent, so either would show if it touched this one.
	local := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceState: traceStateOf(t, maximalTraceState()),
	})
	ctx := trace.ContextWithSpanContext(context.Background(), local)

	got := trace.SpanContextFromContext(sanitizeRemoteContext(ctx, false))

	if !got.Equal(local) {
		t.Errorf("the local parent came back as %v (remote=%v, sampled=%v, %d bytes of tracestate), want it untouched",
			got, got.IsRemote(), got.IsSampled(), len(got.TraceState().String()))
	}
}

// valueParams is a Params implementation that is a struct value rather than a
// pointer. The SDK allows one: embedding its ParamsBase by pointer promotes
// every method the interface asks for, the unexported ones included.
type valueParams struct {
	*mcp.ParamsBase
}

// TestParamsOf_AParamsValueThatIsNotAPointer_IsReturned verifies that the
// typed-nil check asks a pointer whether it is nil and asks nothing of any
// other kind of value.
//
// reflect's IsNil panics on a struct, so a check that skipped the kind would
// turn a custom params type into a panic on every request carrying one, on the
// receiving path of every method, which is the failure the typed-nil check was
// written to end.
func TestParamsOf_AParamsValueThatIsNotAPointer_IsReturned(t *testing.T) {
	t.Parallel()

	params := valueParams{ParamsBase: &mcp.ParamsBase{}}

	got := paramsOf(&mcp.ServerRequest[mcp.Params]{Params: params})

	if _, same := got.(valueParams); !same {
		t.Errorf("paramsOf = %#v, want the params value the request carries", got)
	}
}

// TestParamsOf_NoRequestAtAll_IsNotParams covers the entry guard, which is what
// keeps every caller of paramsOf free of its own nil check.
//
// The middleware asks for a request's params on the receiving path of every
// method, including the notifications the SDK delivers without one, so a nil
// dereference here would be a panic per request rather than a rare edge.
func TestParamsOf_NoRequestAtAll_IsNotParams(t *testing.T) {
	t.Parallel()

	if got := paramsOf(nil); got != nil {
		t.Errorf("paramsOf(nil) = %#v, want nil", got)
	}
	// A request carrying the Params interface itself rather than one of its
	// implementations: GetParams then hands back a genuinely nil interface,
	// which is the case the typed-nil check above it must not swallow.
	if got := paramsOf(&mcp.ServerRequest[mcp.Params]{}); got != nil {
		t.Errorf("paramsOf(a request with no params at all) = %#v, want nil", got)
	}
}
