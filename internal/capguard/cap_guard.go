package capguard

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Undeclared returns receiving middleware that answers the given methods
// with JSON-RPC -32601, the code JSON-RPC 2.0 reserves for a method that
// "does not exist / is not available" and the one the SDK itself returns
// for the optional handlers it can gate (an unwired resources/subscribe,
// completion/complete without a CompletionHandler). Every other method
// passes through untouched.
//
// Gate a method here only while its capability is withheld from the
// handshake: gating a declared capability's method would create the same
// contradiction this middleware removes, pointing the other way.
func Undeclared(methods ...string) mcp.Middleware {
	gated := make(map[string]bool, len(methods))
	for _, m := range methods {
		gated[m] = true
	}
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if gated[method] {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
			}
			return next(ctx, method, req)
		}
	}
}
