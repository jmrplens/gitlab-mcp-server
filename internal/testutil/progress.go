// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// progress.go records the progress notifications a server sends, so a test
// can hold each call site of a progress tracker to the message it sends a
// person.

package testutil

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// progressMarker is the progress token every request [ProgressMessages]
// builds asks for progress with.
const progressMarker = "testutil-progress"

// errUnexpectedElicitation answers an elicitation made by a call
// [ProgressMessages] was given no handler for.
var errUnexpectedElicitation = errors.New("the call under test asked for input, and no answer was given for it")

// ProgressLog holds the message of every progress notification a server
// sent, in the order it sent them.
type ProgressLog struct {
	mu       sync.Mutex
	messages []string
}

// RecordProgress adds a sending middleware to server that records in the
// returned log the message of every progress notification it sends from then
// on, on any of its sessions.
//
// The middleware runs on the goroutine that sends the notification, which is
// the handler's, so once the handler returns every message it sent is in the
// log, and nothing has to wait for a client to hand a notification over. The
// log is still guarded, because a handler a real tools/call runs, runs on a
// goroutine of the server's own.
func RecordProgress(server *mcp.Server) *ProgressLog {
	log := &ProgressLog{}
	server.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if params, ok := req.GetParams().(*mcp.ProgressNotificationParams); ok {
				log.mu.Lock()
				log.messages = append(log.messages, params.Message)
				log.mu.Unlock()
			}
			return next(ctx, method, req)
		}
	})
	return log
}

// Messages returns the messages recorded so far, in the order the server sent
// them.
func (l *ProgressLog) Messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.messages)
}

// ProgressMessages runs call on the test goroutine with a tool call request
// that asks for progress, on a session the legacy client of
// [ConnectLegacyElicitationClient] initialized, and returns the message of
// every progress notification the server sent while it ran, in order.
//
// It is for the texts a handler tells a person while it works, which no test
// that passes a nil request reads: such a request has no tracker, so nothing
// is ever sent. The handler is called directly rather than through tools/call
// because a call that elicits would otherwise be answered over the multi
// round-trip exchange, where every round is a request of its own and its
// tracker starts again from the first step; on this session, negotiated at
// 2025-11-25, an elicitation is asked and answered within the one call.
//
// elicit answers each elicitation the call makes. A caller whose call asks
// nothing passes nil, and an elicitation made anyway is answered with an
// error, which the call reports as its own.
func ProgressMessages(t *testing.T, elicit ElicitHandlerFunc, call func(context.Context, *mcp.CallToolRequest)) []string {
	t.Helper()
	if elicit == nil {
		elicit = func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error) {
			return nil, errUnexpectedElicitation
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "progress-test-server", Version: "0.0.1"}, nil)
	log := RecordProgress(server)
	ctx := t.Context()
	session := ConnectLegacyElicitationClient(ctx, t, server, elicit, LegacyClientOptions{})
	call(ctx, &mcp.CallToolRequest{Session: session, Params: &mcp.CallToolParamsRaw{
		Meta: mcp.Meta{"progressToken": progressMarker},
	}})
	return log.Messages()
}
