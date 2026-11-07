// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/toolvisibility"
)

// attachCallChecks installs the two tools/call checks that answer a call
// before the argument limit and the SDK's validation, at the place in the
// receiving chain they share: right after the argument limit is attached, so
// they run before it, and right before the rate limiter is, so they run
// inside it and inside the held-call count.
//
// A call a fine-grained session may not run is answered with the reason, so
// one cause gets one refusal whatever the arguments, and a withheld call
// spends its credential's token and holds its slot like every other refused
// call (register row AUT-007). A call the session's narrowing explains, a meta
// call naming an action its group withheld (issue 1282), is answered the same
// way, where the SDK would refuse it as invalid params and say nothing of
// why. The narrowed check is added first, so it runs after the fine-grained
// one: a fine-grained session's refusal of the action it names comes first.
func attachCallChecks(server *mcp.Server, gate *readinessGate, actions func() *toolvisibility.ToolActions) {
	server.AddReceivingMiddleware(narrowedCalls(gate, actions))
	server.AddReceivingMiddleware(fineGrainedCalls(gate, actions))
}

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
	return callsAfterReadiness(gate, toolvisibility.CallMiddleware(actions))
}

// narrowedCalls is the tools/call check that answers a call the session's
// narrowing explains before the SDK validates its arguments
// ([toolvisibility.NarrowedCallMiddleware], issue 1282), made to wait for the
// readiness gate for the reason [fineGrainedCalls] does: until registration
// is done there is no index to answer from, and a call let through then would
// be refused by the SDK's validation without its cause.
func narrowedCalls(gate *readinessGate, actions func() *toolvisibility.ToolActions) mcp.Middleware {
	return callsAfterReadiness(gate, toolvisibility.NarrowedCallMiddleware(actions))
}

// callsAfterReadiness makes a tools/call check wait for the readiness gate
// first on a transport the gate holds, and runs every other method straight
// through it.
func callsAfterReadiness(gate *readinessGate, check mcp.Middleware) mcp.Middleware {
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
