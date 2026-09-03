package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
