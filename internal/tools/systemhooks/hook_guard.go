// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// hook_guard.go guards the calls that change a system hook so that it
// delivers elsewhere or loses a secret nobody can read back: an edit that
// sends another URL or a token, and the call that sets a URL variable the
// hook already has.

package systemhooks

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmHookChange asks the caller to confirm a change to a system hook that
// redirects it or replaces a secret, and returns nil when the call may
// proceed.
//
// The rule is the one [toolutil.ConfirmHookDeliveryChange] documents, shared
// with the webhooks of a project and of a group. A system hook delivers the
// events of the whole instance, the creation of users and projects among
// them, so the address it posts to is the widest of the three. What this
// adds is the read: the arguments say that a call may redirect the hook or
// replace a value, and only the hook GitLab holds says whether it does, so
// the guard reads it once, when the call sends an address or a key and
// nothing has confirmed it. A call that sends neither is never read for.
//
// A read the caller abandoned is answered with the context's error rather
// than with a question nobody is there to answer.
func confirmHookChange(ctx context.Context, client *gitlabclient.Client, op string, hookID int64, edit toolutil.HookEdit) error {
	var current *toolutil.HookState
	var readErr error
	if toolutil.HookEditNeedsRead(ctx, edit) {
		hook, _, err := client.GL().SystemHooks.GetHook(hookID, gl.WithContext(ctx))
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		current, readErr = hookState(hook), err
	}
	return toolutil.ConfirmHookDeliveryChange(ctx, op, toolutil.ConfirmTargetInstance(), hookID, edit.Changes(current, readErr))
}

// hookState is what the rule reads of the hook GitLab answered with: its
// address and the keys of its two write-only maps. A read that failed
// answers no hook, which the rule is told by the read's error.
func hookState(hook *gl.Hook) *toolutil.HookState {
	if hook == nil {
		return nil
	}
	state := &toolutil.HookState{URL: hook.URL}
	for _, variable := range hook.URLVariables {
		state.URLVariableKeys = append(state.URLVariableKeys, variable.Key)
	}
	for _, header := range hook.CustomHeaders {
		state.CustomHeaderKeys = append(state.CustomHeaderKeys, header.Key)
	}
	return state
}
