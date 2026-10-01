package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/toolvisibility"
)

// fineGrainedCalls is the tools/call check of register row AUT-007 for one
// server: [toolvisibility.CallMiddleware], made to wait for the readiness gate
// first on a transport the gate holds.
//
// It has to wait because it decides before the call goes further in, and the
// gate is the innermost middleware: a call that arrived while registration was
// still running would otherwise be judged before the tool sets exist, and on
// stdio before the authority is attached, and be let through to the
// dispatcher, whose own check comes only after the SDK has validated the
// arguments. Waiting here is no new wait: the gate would hold the call just as
// long a few middlewares further in, and a call it refuses is refused here
// with the same answer.
func fineGrainedCalls(gate *readinessGate, actions func() *toolvisibility.ToolActions) mcp.Middleware {
	check := toolvisibility.CallMiddleware(actions)
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		checked := check(next)
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" && !gate.isReady() && readinessEnforced(ctx) {
				if err := gate.await(ctx, method); err != nil {
					return nil, err
				}
			}
			return checked(ctx, method, req)
		}
	}
}

// bindProcessClient binds a server's one client to every request it serves,
// which is what [credentialStates.bindCredential] does per request on a server
// shared by many credentials. On stdio, and on any server built for a single
// client, every handler already captures that client, so this changes nothing
// a handler reads; what it adds is that the layers holding no client of their
// own, which read the request's through the context
// ([gitlabclient.AuthorityFrom]), find it on that transport too.
func bindProcessClient(client *gitlabclient.Client) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			return next(gitlabclient.WithClient(ctx, client), method, req)
		}
	}
}
