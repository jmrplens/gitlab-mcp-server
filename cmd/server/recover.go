package main

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recoverPanics turns a panic in a request handler into a JSON-RPC internal
// error instead of letting it take the process down.
//
// The SDK dispatches each request on its own jsonrpc2 goroutine and does not
// recover, so a nil dereference anywhere below this point is fatal to the whole
// binary. That is survivable for a stdio server owned by one user; it is not
// for the hosted HTTP endpoint, where every tenant sharing the process loses
// their session because one request found a bug. A nil Capabilities pointer in
// internal/elicitation did exactly that, reachable by an ordinary client with
// no misbehavior on its part.
//
// This is a backstop, not a fix. A panic that lands here is a defect: it is
// logged at error level with its stack so it can be found and repaired, and
// recovering only decides that the other tenants keep working in the meantime.
// The stack goes to the log and never into the response, which carries the
// generic internal-error text, because a panic message can name paths, tokens
// or query fragments and the caller is not owed them.
//
// Go cannot recover a runtime throw (out of memory, a concurrent map write, a
// deadlock), so this narrows the fatal set rather than emptying it.
func recoverPanics(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			slog.Error("recovered a panic while handling a request",
				"method", method,
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)
			result = nil
			err = &jsonrpc.Error{
				Code:    jsonrpc.CodeInternalError,
				Message: "internal error",
			}
		}()
		return next(ctx, method, req)
	}
}
