// cap_guard_test.go verifies the Undeclared middleware refuses exactly the
// gated methods with the SDK's own method-not-found code and passes every
// other method through to the wrapped handler unchanged.
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
