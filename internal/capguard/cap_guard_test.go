// cap_guard_test.go verifies the Undeclared middleware refuses exactly the
// gated methods with the SDK's own method-not-found code and passes every
// other method through to the wrapped handler unchanged.
//
// Four properties are asserted apart from that table because the table
// cannot see them: the refusal carries the reserved -32601 as a number rather
// than as whichever constant produced it, an ungated call reaches the wrapped
// handler with the caller's own context and request, membership of the gate
// is by whole method name, and a gate built from an empty list withholds
// nothing.
package capguard

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errInner is the sentinel a passthrough case plants in the wrapped
// handler to prove the middleware returns inner errors unchanged.
var errInner = errors.New("inner handler failed")

// wireMethodNotFound is the code JSON-RPC 2.0 reserves for a method that
// "does not exist / is not available", written as the number that travels on
// the wire rather than as the SDK constant the middleware writes it with.
// Spelling it out is the whole point of the test that reads it.
const wireMethodNotFound = -32601

// carriedKey types the value a passthrough case plants in the context to
// prove the caller's own context reaches the wrapped handler.
type carriedKey struct{}

// TestUndeclared_GatedAndUngatedMethods_RefusesOnlyGated verifies gated methods answer -32601
// without reaching the next handler, and ungated methods pass through with
// the inner handler's result — or its error — intact.
func TestUndeclared_GatedAndUngatedMethods_RefusesOnlyGated(t *testing.T) {
	var reached []string
	inner := func(_ context.Context, method string, _ mcp.Request) (mcp.Result, error) {
		reached = append(reached, method)
		if method == "tools/call" {
			return nil, errInner
		}
		return &mcp.ListToolsResult{}, nil
	}
	handler := Undeclared("logging/setLevel", "prompts/list")(inner)

	tests := []struct {
		method       string
		wantGated    bool
		wantInnerErr bool
	}{
		{"logging/setLevel", true, false},
		{"prompts/list", true, false},
		{"tools/list", false, false},
		{"resources/list", false, false},
		{"tools/call", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result, err := handler(context.Background(), tt.method, nil)
			switch {
			case tt.wantGated:
				assertGated(t, tt.method, result, err)
			case tt.wantInnerErr:
				if !errors.Is(err, errInner) {
					t.Fatalf("%s error = %v, want the inner handler's error unchanged", tt.method, err)
				}
			default:
				assertPassthrough(t, tt.method, result, err)
			}
		})
	}

	for _, m := range reached {
		if m == "logging/setLevel" || m == "prompts/list" {
			t.Errorf("gated method %s reached the inner handler", m)
		}
	}
}

// assertGated checks a refused call: -32601 and no result.
func assertGated(t *testing.T, method string, result mcp.Result, err error) {
	t.Helper()
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("%s error = %v, want jsonrpc.Error with code %d", method, err, jsonrpc.CodeMethodNotFound)
	}
	if result != nil {
		t.Errorf("%s result = %v, want nil", method, result)
	}
}

// assertPassthrough checks an ungated call reached the inner handler and
// its result came back intact.
func assertPassthrough(t *testing.T, method string, result mcp.Result, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s error = %v, want passthrough", method, err)
	}
	if result == nil {
		t.Errorf("%s result = nil, want the inner handler's result", method)
	}
}

// TestUndeclared_Refusal_CarriesTheReservedWireCode holds the refusal to the
// literal -32601 rather than to the constant the middleware writes it with.
//
// Every other assertion here compares the returned code against
// jsonrpc.CodeMethodNotFound, which is the same constant the production line
// uses to produce it, so both sides move together: with that value re-spelled
// the whole file still passes while the server answers a number that no
// longer means "method not found". Nothing else in the tree covers it either
// — internal/mcpotel hardcodes -32601 of its own to classify this refusal as
// a caller fault rather than a server error, so the two would silently
// disagree and a gated method would start counting against the server's own
// error rate. The number is the contract with the client; the constant is
// only how this package spells it.
func TestUndeclared_Refusal_CarriesTheReservedWireCode(t *testing.T) {
	unreached := func(_ context.Context, method string, _ mcp.Request) (mcp.Result, error) {
		t.Errorf("gated method %s reached the inner handler", method)
		return &mcp.ListToolsResult{}, nil
	}
	handler := Undeclared("logging/setLevel")(unreached)

	_, err := handler(context.Background(), "logging/setLevel", nil)

	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want a *jsonrpc.Error", err)
	}
	if rpcErr.Code != wireMethodNotFound {
		t.Errorf("refusal code = %d, want the reserved %d: a client reads this number, not the constant behind it",
			rpcErr.Code, wireMethodNotFound)
	}
}

// TestUndeclared_UngatedMethod_ReachesNextWithTheCallerContextAndRequest
// asserts an ungated call arrives at the wrapped handler carrying the very
// context and request the middleware was handed.
//
// The table above cannot see this: it calls every method with a background
// context and a nil request, so a middleware substituting its own for either
// passes it unchanged. Both are load-bearing here — the per-request GitLab
// client is installed on the context and read back by (*gitlab.Client).For,
// and the POST carrying a call cancels it through that same context — so a
// dropped context would surface as every tool call failing with an unbound
// client, far from anything that looks like a middleware defect.
func TestUndeclared_UngatedMethod_ReachesNextWithTheCallerContextAndRequest(t *testing.T) {
	wantReq := &mcp.ServerRequest[*mcp.ListToolsParams]{Params: &mcp.ListToolsParams{}}
	var reached bool
	var gotCarried any
	var gotReq mcp.Request
	inner := func(ctx context.Context, _ string, req mcp.Request) (mcp.Result, error) {
		reached, gotCarried, gotReq = true, ctx.Value(carriedKey{}), req
		return &mcp.ListToolsResult{}, nil
	}
	ctx := context.WithValue(context.Background(), carriedKey{}, "carried")

	if _, err := Undeclared("logging/setLevel")(inner)(ctx, "tools/list", wantReq); err != nil {
		t.Fatalf("tools/list error = %v, want passthrough", err)
	}

	if !reached {
		t.Fatal("tools/list never reached the inner handler")
	}
	if gotCarried != "carried" {
		t.Errorf("context value at the inner handler = %v, want %q: the caller's context did not reach it", gotCarried, "carried")
	}
	if gotReq != mcp.Request(wantReq) {
		t.Errorf("request at the inner handler = %v, want the one the middleware was given", gotReq)
	}
}

// TestUndeclared_NamesNeighboringAGatedMethod_PassThrough asserts the gate
// matches a whole method name and never a prefix, a suffix or a different
// casing.
//
// It holds the membership test to equality so that a later rewrite reaching
// for strings.HasPrefix or a normalizing comparison cannot quietly withdraw
// methods nobody withheld: MCP namespaces methods with a slash, so a prefix
// match on "prompts/list" would take "prompts/list_changed" with it, and the
// refusal is indistinguishable from a method the server genuinely lacks.
func TestUndeclared_NamesNeighboringAGatedMethod_PassThrough(t *testing.T) {
	neighbors := []string{
		"logging/setLevel/extra",
		"logging/setLeve",
		"logging",
		"Logging/setLevel",
		" logging/setLevel",
		"prompts/list_changed",
	}
	handler := Undeclared("logging/setLevel", "prompts/list")(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{}, nil
	})

	for _, method := range neighbors {
		t.Run(method, func(t *testing.T) {
			result, err := handler(context.Background(), method, nil)
			assertPassthrough(t, method, result, err)
		})
	}
}

// TestUndeclared_NoMethodsGated_RefusesNothing asserts that middleware built
// from an empty list withholds nothing.
//
// The caller assembles its list from the capability surface, so the empty
// case is one configuration change away, and the failure it guards against is
// the inverted one: a gate that reads an empty set as "everything is
// undeclared" answers -32601 to every method on the server and looks, from
// the client's side, like a server that supports nothing at all.
func TestUndeclared_NoMethodsGated_RefusesNothing(t *testing.T) {
	handler := Undeclared()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{}, nil
	})

	for _, method := range []string{"logging/setLevel", "prompts/list", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			result, err := handler(context.Background(), method, nil)
			assertPassthrough(t, method, result, err)
		})
	}
}
