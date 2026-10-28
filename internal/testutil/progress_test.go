// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// progress_test.go covers the progress recorder: what it records, in what
// order, what it leaves out, and how an elicitation made during the call is
// answered.

package testutil

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// notifyProgress sends one progress notification for req, reporting a
// session that refuses it. The comparison of what was recorded is what fails
// the test then, so the report does not abort it.
func notifyProgress(ctx context.Context, t *testing.T, req *mcp.CallToolRequest, progress float64, message string) {
	t.Helper()
	if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
		ProgressToken: req.Params.GetProgressToken(),
		Progress:      progress,
		Message:       message,
	}); err != nil {
		t.Errorf("NotifyProgress(%q): %v", message, err)
	}
}

// TestProgressMessages_EveryNotificationTheCallSends_IsReturnedInOrder pins
// the recorder's one promise: every progress notification the call sent is
// returned, in the order it was sent, as soon as the call returns. The call
// is given a request that asks for progress on an initialized session, which
// is what makes a handler's tracker active.
func TestProgressMessages_EveryNotificationTheCallSends_IsReturnedInOrder(t *testing.T) {
	var token any
	got := ProgressMessages(t, nil, func(ctx context.Context, req *mcp.CallToolRequest) {
		token = req.Params.GetProgressToken()
		if req.Session.InitializeParams() == nil {
			t.Error("the session the call was given is not initialized, so a tracker built from it would be inactive")
		}
		notifyProgress(ctx, t, req, 1, "first")
		notifyProgress(ctx, t, req, 2, "second")
	})
	if token != progressMarker {
		t.Errorf("the request's progress token = %v, want %q", token, progressMarker)
	}
	if want := []string{"first", "second"}; !slices.Equal(got, want) {
		t.Errorf("ProgressMessages() = %q, want %q", got, want)
	}
}

// TestProgressMessages_ACallThatElicits_IsAnsweredByTheHandlerAndOnlyItsProgressIsRecorded
// pins that an elicitation the call makes reaches the handler it was given
// and is answered within the call, and that the request it is sent as, which
// passes the same middleware, is not taken for a progress message.
func TestProgressMessages_ACallThatElicits_IsAnsweredByTheHandlerAndOnlyItsProgressIsRecorded(t *testing.T) {
	var (
		mu    sync.Mutex
		asked []string
	)
	elicit := func(_ context.Context, params *mcp.ElicitParams) (*mcp.ElicitResult, error) {
		mu.Lock()
		asked = append(asked, params.Message)
		mu.Unlock()
		return &mcp.ElicitResult{Action: "decline"}, nil
	}

	var action string
	got := ProgressMessages(t, elicit, func(ctx context.Context, req *mcp.CallToolRequest) {
		notifyProgress(ctx, t, req, 1, "before")
		res, err := req.Session.Elicit(ctx, &mcp.ElicitParams{Message: "Proceed?"})
		if err != nil {
			t.Errorf("Elicit: %v", err)
			return
		}
		action = res.Action
		notifyProgress(ctx, t, req, 2, "after")
	})

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"Proceed?"}; !slices.Equal(asked, want) {
		t.Errorf("the handler was asked %q, want %q", asked, want)
	}
	if action != "decline" {
		t.Errorf("the call was answered %q, want the handler's decline", action)
	}
	if want := []string{"before", "after"}; !slices.Equal(got, want) {
		t.Errorf("ProgressMessages() = %q, want %q", got, want)
	}
}

// TestProgressMessages_AnElicitationNoHandlerWasGivenFor_FailsTheCall pins
// what a call given no handler is answered when it asks something anyway: an
// error the call returns, naming what happened, rather than a nil handler
// called on the client's goroutine, which would end the whole test binary.
func TestProgressMessages_AnElicitationNoHandlerWasGivenFor_FailsTheCall(t *testing.T) {
	var err error
	got := ProgressMessages(t, nil, func(ctx context.Context, req *mcp.CallToolRequest) {
		_, err = req.Session.Elicit(ctx, &mcp.ElicitParams{Message: "Proceed?"})
	})
	if err == nil || !strings.Contains(err.Error(), errUnexpectedElicitation.Error()) {
		t.Errorf("Elicit() error = %v, want it to carry %q", err, errUnexpectedElicitation)
	}
	if len(got) != 0 {
		t.Errorf("ProgressMessages() = %q, want nothing recorded", got)
	}
}
