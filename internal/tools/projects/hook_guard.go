// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// hook_guard.go guards the calls that change a project webhook so that it
// delivers elsewhere or loses a secret nobody can read back: an edit that
// sends another URL, a token, or a URL variable or custom header the hook
// already has, and the two calls that set one such key.

package projects

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmHookChange asks the caller to confirm a change to a project webhook
// that redirects it or replaces a secret, and returns nil when the call may
// proceed.
//
// The rule is the one [toolutil.ConfirmHookDeliveryChange] documents, shared
// with the group's webhooks and the system hooks. What this adds is the
// read: the arguments say that a call may redirect the hook or replace a
// value, and only the hook GitLab holds says whether it does, so the guard
// reads it once, when the call sends an address or a key and nothing has
// confirmed it. A call that sends neither is never read for.
//
// A read the caller abandoned is answered with the context's error rather
// than with a question nobody is there to answer.
func confirmHookChange(ctx context.Context, client *gitlabclient.Client, op, projectID string, hookID int64, edit toolutil.HookEdit) error {
	var current *toolutil.HookState
	var readErr error
	if toolutil.HookEditNeedsRead(ctx, edit) {
		hook, _, err := doProjectRequest[projectHookAPI](ctx, client, http.MethodGet, projectHookPath(projectID, hookID), nil)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		current, readErr = hookState(&hook), err
	}
	return toolutil.ConfirmHookDeliveryChange(ctx, op, toolutil.ConfirmTargetProject(projectID), hookID, edit.Changes(current, readErr))
}

// hookState is what the rule reads of the hook GitLab answered with: its
// address and the keys of its two write-only maps.
func hookState(hook *projectHookAPI) *toolutil.HookState {
	state := &toolutil.HookState{URL: hook.URL}
	for _, variable := range hook.URLVariables {
		state.URLVariableKeys = append(state.URLVariableKeys, variable.Key)
	}
	for _, header := range projectHookCustomHeadersToOutput(hook.CustomHeaders) {
		state.CustomHeaderKeys = append(state.CustomHeaderKeys, header.Key)
	}
	return state
}

// hookEdit restates the edit in the terms the shared rule reads.
func (input EditHookInput) hookEdit() toolutil.HookEdit {
	edit := toolutil.HookEdit{
		URL:          input.URL,
		Token:        input.Token != "",
		SigningToken: input.SigningToken != "",
	}
	for _, variable := range input.URLVariables {
		edit.URLVariableKeys = append(edit.URLVariableKeys, variable.Key)
	}
	for _, header := range input.CustomHeaders {
		edit.CustomHeaderKeys = append(edit.CustomHeaderKeys, header.Key)
	}
	return edit
}
