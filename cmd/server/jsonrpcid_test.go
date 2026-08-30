package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequestIDFromBody_RecoversOnlyALegalRequestID pins what a refusal is
// allowed to echo back.
//
// "Error responses MUST include the same ID as the request they correspond to
// (except in error cases where the ID could not be read due a malformed
// request)." The exception is what the absent cases below are: a GET carries no
// request, a notification has no id, and a body that does not parse has none to
// find. Everything else must come back exactly as it was sent, which is what
// correlation means: the client matches on the value it wrote, not on this
// server's reading of it.
//
// The rejected shapes matter as much as the accepted ones. Under 2026-07-28 a
// RequestId is a string or an integer, so null, an object and an array are not
// ids; echoing one would produce a body that fails the specification's own
// schema, which is the failure the JSON-RPC error body exists to prevent.
func TestRequestIDFromBody_RecoversOnlyALegalRequestID(t *testing.T) {
	cases := []struct {
		name   string
		method string
		body   string
		want   string
	}{
		{"numeric id", http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "1"},
		{"large numeric id", http.MethodPost, `{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/list"}`, "9007199254740993"},
		{"negative id", http.MethodPost, `{"jsonrpc":"2.0","id":-4,"method":"tools/list"}`, "-4"},
		{"string id", http.MethodPost, `{"jsonrpc":"2.0","id":"req-abc","method":"tools/list"}`, `"req-abc"`},
		{"id after params", http.MethodPost, `{"jsonrpc":"2.0","method":"tools/list","params":{"a":1},"id":7}`, "7"},

		{"notification", http.MethodPost, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, ""},
		// Decode stops at the end of the first value, so without an explicit
		// end-of-input check these would yield an id lifted out of something
		// that is not a request.
		{"trailing garbage", http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"tools/list"} and then some`, ""},
		{"a second message on the same body", http.MethodPost, `{"jsonrpc":"2.0","id":1}{"jsonrpc":"2.0","id":2}`, ""},
		// The two bytes decoder.More() cannot see. It answers "is there another
		// element in the array or object being parsed", which it decides by
		// peeking for anything that is not ] or }, so a stray closing brace
		// slipped past the check that was written first.
		{"a trailing closing brace", http.MethodPost, `{"jsonrpc":"2.0","id":1}}`, ""},
		{"a trailing closing bracket", http.MethodPost, `{"jsonrpc":"2.0","id":1}]`, ""},
		// And the whitespace that must not be mistaken for trailing content: a
		// body ending in a newline is the ordinary case.
		{"a trailing newline is not trailing content", http.MethodPost, "{\"jsonrpc\":\"2.0\",\"id\":1}\n", "1"},
		// An object that does not announce itself as JSON-RPC is not a message,
		// which is the rule the stdio filter already applies.
		{"no jsonrpc member", http.MethodPost, `{"id":1,"method":"tools/list"}`, ""},
		{"wrong jsonrpc version", http.MethodPost, `{"jsonrpc":"1.0","id":1,"method":"tools/list"}`, ""},
		{"null id", http.MethodPost, `{"jsonrpc":"2.0","id":null,"method":"tools/list"}`, ""},
		{"object id", http.MethodPost, `{"jsonrpc":"2.0","id":{"a":1},"method":"tools/list"}`, ""},
		{"array id", http.MethodPost, `{"jsonrpc":"2.0","id":[1],"method":"tools/list"}`, ""},
		{"boolean id", http.MethodPost, `{"jsonrpc":"2.0","id":true,"method":"tools/list"}`, ""},
		{"not JSON", http.MethodPost, `not json at all`, ""},
		{"a JSON array, which batching used to allow", http.MethodPost, `[{"jsonrpc":"2.0","id":1}]`, ""},
		{"empty body", http.MethodPost, ``, ""},

		// A GET opens the standalone SSE stream and a DELETE terminates a
		// session; neither carries a request to correlate with.
		{"GET", http.MethodGet, ``, ""},
		{"DELETE", http.MethodDelete, ``, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tc.method, "/mcp", strings.NewReader(tc.body))
			if got := string(requestIDFromBody(req)); got != tc.want {
				t.Errorf("requestIDFromBody() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRequestIDFromBody_IsBounded checks that the probe cannot be used to make
// the server buffer an arbitrary body.
//
// It runs on requests that are being refused, including unauthenticated ones,
// so an unbounded read would let anyone who can reach the endpoint allocate as
// much as they can send. Past the bound the id is simply not recovered, which
// is the spec's own "could not be read" case and leaves the member out.
func TestRequestIDFromBody_IsBounded(t *testing.T) {
	oversized := `{"jsonrpc":"2.0","method":"tools/list","params":{"pad":"` +
		strings.Repeat("A", maxIDProbeBytes+1024) + `"},"id":1}`

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(oversized))
	if got := string(requestIDFromBody(req)); got != "" {
		t.Errorf("requestIDFromBody() = %q on a body past the probe bound, want the member omitted", got)
	}
}

// TestRequestIDFromBody_NilRequest guards the callers that have no request at
// all, so the helper can be used unconditionally.
func TestRequestIDFromBody_NilRequest(t *testing.T) {
	if got := requestIDFromBody(nil); got != nil {
		t.Errorf("requestIDFromBody(nil) = %q, want nil", got)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Body = nil
	if got := requestIDFromBody(req); got != nil {
		t.Errorf("requestIDFromBody() with no body = %q, want nil", got)
	}
}
