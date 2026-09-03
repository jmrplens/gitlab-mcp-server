package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bindObserveWindow is how long a bound handler waits for a cancellation that
// may never come. It is long enough for one to arrive, since context.AfterFunc
// fires on its own goroutine the moment the carrier ends, and short enough that
// the cases which must not cancel do not each cost a second.
const bindObserveWindow = 200 * time.Millisecond

// carrierRequest builds an MCP request carrying the given header, which is the
// shape the streamable transport hands a receiving middleware. A nil header is
// an HTTP request the SDK filled in with none.
func carrierRequest(header http.Header) mcp.Request {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "gitlab_execute_action"},
		Extra:  &mcp.RequestExtra{Header: header},
	}
}

// stdioRequest builds the shape a request arrives in on a transport that fills
// in no Extra at all.
func stdioRequest() mcp.Request {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}}
}

// carrierHeaderWith returns a header carrying token under [carrierHeader].
func carrierHeaderWith(token string) http.Header {
	h := http.Header{}
	h.Set(carrierHeader, token)
	return h
}

// TestRequestCarriersMiddleware_StampsAPostAndRegistersItsContext pins the
// half of the mechanism that runs in front of the SDK.
//
// Two properties matter and neither is visible from the other half: a POST
// leaves with a token this process minted, whatever the client sent under that
// name, and the token resolves to that POST's context for as long as the POST
// is running. A method that carries no MCP request is stamped with nothing at
// all, so a client cannot present a token on a request this never mints one
// for.
func TestRequestCarriersMiddleware_StampsAPostAndRegistersItsContext(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		sent      string
		wantToken bool
	}{
		{name: "a POST is stamped", method: http.MethodPost, wantToken: true},
		{name: "a POST carrying a forged token is re-stamped", method: http.MethodPost, sent: "forged", wantToken: true},
		{name: "a GET is stripped", method: http.MethodGet, sent: "forged"},
		{name: "a DELETE is stripped", method: http.MethodDelete, sent: "forged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var carriers requestCarriers
			var seen string
			var resolved bool

			handler := carriers.middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = r.Header.Get(carrierHeader)
				resolved = carriers.lookup(seen) != nil
			}))

			req := httptest.NewRequestWithContext(t.Context(), tt.method, "/mcp", http.NoBody)
			if tt.sent != "" {
				req.Header.Set(carrierHeader, tt.sent)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			switch {
			case !tt.wantToken:
				if seen != "" {
					t.Fatalf("a %s left carrying %q, want no token", tt.method, seen)
				}
			case seen == "" || seen == tt.sent:
				t.Fatalf("the POST left carrying %q, want a freshly minted token", seen)
			case !resolved:
				t.Fatalf("token %q did not resolve to the request context", seen)
			}
		})
	}
}

// TestRequestCarriersMiddleware_ForgetsTheTokenWhenTheRequestEnds pins the
// invariant the other half reads: a token that is still in the map means the
// POST carrying it is still running.
//
// The entry is tied to the request context rather than to this handler's
// return, because a handler that returns while a call is still in flight is
// exactly the case being fixed. Anything else would leak an entry per request.
func TestRequestCarriersMiddleware_ForgetsTheTokenWhenTheRequestEnds(t *testing.T) {
	var carriers requestCarriers
	var token string

	handler := carriers.middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		token = r.Header.Get(carrierHeader)
	}))

	ctx, done := context.WithCancel(t.Context())
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", http.NoBody))

	if carriers.lookup(token) == nil {
		t.Fatalf("token %q was forgotten while its request context was still live", token)
	}
	done()

	// context.AfterFunc runs the removal on its own goroutine, so the entry
	// disappears shortly after the context does rather than with it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if carriers.lookup(token) == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("token %q outlived the request that minted it", token)
}

// bindCase is one decision [requestCarriers.bind] makes.
type bindCase struct {
	name string
	// register puts the token in the map before the request is handled.
	register bool
	method   string
	req      mcp.Request
	// endCarrier ends the carrier while the handler is running.
	endCarrier bool
	// wantCancelled is whether the handler should observe a cancelled context.
	wantCancelled bool
}

// TestRequestCarriersBind_CancelsACallWhenItsCarrierGoesAway covers the half
// that runs inside the MCP server.
//
// The cases are the decisions the middleware makes: a call bound to a live
// carrier runs under a context that ends with it; a call whose token names no
// carrier is already too late to answer and is cancelled at once; a
// notification is left alone, because a POST does not wait for one and its
// handler can legitimately outlive the carrier; and a request that arrived on a
// transport stamping no token (stdio, or the in-memory transport the e2e suite
// drives) is left alone as well.
func TestRequestCarriersBind_CancelsACallWhenItsCarrierGoesAway(t *testing.T) {
	tests := []bindCase{
		{
			name: "a call is cancelled when its carrier ends", register: true,
			method: "tools/call", req: carrierRequest(carrierHeaderWith("live")),
			endCarrier: true, wantCancelled: true,
		},
		{
			name:   "a call whose carrier is already gone is cancelled at once",
			method: "tools/call", req: carrierRequest(carrierHeaderWith("stale")),
			wantCancelled: true,
		},
		{
			name: "a notification is left alone", register: true,
			method: "notifications/initialized", req: carrierRequest(carrierHeaderWith("live")),
			endCarrier: true,
		},
		{
			name:   "a request with no token is left alone",
			method: "tools/call", req: carrierRequest(http.Header{}),
		},
		{
			name:   "a request with no extra is left alone",
			method: "tools/call", req: stdioRequest(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := runBoundHandler(t, tt)
			switch {
			case !tt.wantCancelled:
				if observed.Err() != nil {
					t.Fatalf("the handler context was cancelled (%v) when it should have been left alone", observed.Err())
				}
			case observed.Err() == nil:
				t.Fatalf("the handler context was not cancelled")
			default:
				if cause := context.Cause(observed); !errors.Is(cause, errCallerGone) {
					t.Errorf("cancellation cause = %v, want %v", cause, errCallerGone)
				}
			}
		})
	}
}

// runBoundHandler drives one case through [requestCarriers.bind] and returns
// the context the handler was given, after it has had a chance to be cancelled.
func runBoundHandler(t *testing.T, tc bindCase) context.Context {
	t.Helper()

	var carriers requestCarriers
	carrierCtx, endCarrier := context.WithCancel(t.Context())
	defer endCarrier()
	if tc.register {
		carriers.contexts.Store(carrierTokenOf(tc.req), carrierCtx)
	}

	observed := make(chan context.Context, 1)
	handler := carriers.bind(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		observed <- ctx
		if tc.endCarrier {
			endCarrier()
		}
		select {
		case <-ctx.Done():
		case <-time.After(bindObserveWindow):
		}
		return &mcp.CallToolResult{}, nil
	})

	if _, err := handler(t.Context(), tc.method, tc.req); err != nil {
		t.Fatalf("the handler returned %v", err)
	}
	return <-observed
}

// TestRequestCarriersBind_LeavesTheContextIntactAfterTheCall checks that the
// cancellation the middleware installs does not outlive the call it belongs to.
//
// The bound context is cancelled on the way out whatever happened, so nothing
// derived from it stays alive; the context handed in from above must be
// untouched, since it belongs to the connection and other requests share it.
func TestRequestCarriersBind_LeavesTheContextIntactAfterTheCall(t *testing.T) {
	var carriers requestCarriers
	carrierCtx, endCarrier := context.WithCancel(t.Context())
	defer endCarrier()
	carriers.contexts.Store("live", carrierCtx)

	observed := make(chan context.Context, 1)
	handler := carriers.bind(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		observed <- ctx
		return &mcp.CallToolResult{}, nil
	})

	parent, cancelParent := context.WithCancel(t.Context())
	defer cancelParent()
	if _, err := handler(parent, "tools/call", carrierRequest(carrierHeaderWith("live"))); err != nil {
		t.Fatalf("the handler returned %v", err)
	}

	if bound := <-observed; bound.Err() == nil {
		t.Errorf("the bound context outlived the call")
	}
	if parent.Err() != nil {
		t.Errorf("the connection context was cancelled by a call that ended normally: %v", parent.Err())
	}
}

// TestCarrierTokenOf_ReadsOnlyWhatTheTransportSupplies pins the accessor
// against the shapes a request arrives in: an HTTP one carrying a header, an
// HTTP one carrying none, and a stdio one with no Extra at all.
func TestCarrierTokenOf_ReadsOnlyWhatTheTransportSupplies(t *testing.T) {
	tests := []struct {
		name string
		req  mcp.Request
		want string
	}{
		{name: "a stamped HTTP request", req: carrierRequest(carrierHeaderWith("abc")), want: "abc"},
		{name: "an HTTP request with no token", req: carrierRequest(http.Header{})},
		{name: "a request with no header", req: carrierRequest(nil)},
		{name: "a request with no extra", req: stdioRequest()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := carrierTokenOf(tt.req); got != tt.want {
				t.Errorf("carrierTokenOf = %q, want %q", got, tt.want)
			}
		})
	}
}
