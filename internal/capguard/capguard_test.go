// capguard_test.go verifies the Undeclared middleware refuses exactly the
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

// TestUndeclared_GatedAndPassthrough verifies gated methods answer -32601
// without reaching the next handler, and ungated methods pass through with
// the inner handler's result intact.
func TestUndeclared_GatedAndPassthrough(t *testing.T) {
	var reached []string
	inner := func(_ context.Context, method string, _ mcp.Request) (mcp.Result, error) {
		reached = append(reached, method)
		return &mcp.ListToolsResult{}, nil
	}
	handler := Undeclared("logging/setLevel", "prompts/list")(inner)

	tests := []struct {
		method    string
		wantGated bool
	}{
		{"logging/setLevel", true},
		{"prompts/list", true},
		{"tools/list", false},
		{"resources/list", false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result, err := handler(context.Background(), tt.method, nil)
			if tt.wantGated {
				var rpcErr *jsonrpc.Error
				if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
					t.Fatalf("%s error = %v, want jsonrpc.Error with code %d", tt.method, err, jsonrpc.CodeMethodNotFound)
				}
				if result != nil {
					t.Errorf("%s result = %v, want nil", tt.method, result)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s error = %v, want passthrough", tt.method, err)
			}
			if result == nil {
				t.Errorf("%s result = nil, want the inner handler's result", tt.method)
			}
		})
	}

	for _, m := range reached {
		if m == "logging/setLevel" || m == "prompts/list" {
			t.Errorf("gated method %s reached the inner handler", m)
		}
	}
}
