// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// hook_guard.go decides when a call that changes a webhook changes where its
// events are delivered or loses a secret nobody can read back, and asks
// before it does. The rule is one for a project's webhooks, a group's and
// the instance's system hooks, which GitLab edits through three routes that
// take the same settings, so each domain package reads its own hook and
// hands it here.

package toolutil

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// Names of the two write-only maps of a webhook, as the question spells them.
const (
	hookFieldURLVariables  = "url_variables"
	hookFieldCustomHeaders = "custom_headers"
)

// HookState is what GitLab publishes of a webhook that the rule reads: the
// address it delivers to, and the keys of its URL variables and custom
// headers. GitLab never returns their values, nor the two tokens.
type HookState struct {
	URL              string
	URLVariableKeys  []string
	CustomHeaderKeys []string
}

// HookEdit is what one call sends of the settings the rule reads. An empty
// URL and a false token are settings the call does not name, which GitLab
// leaves as they are. A call that sets one URL variable or one custom header
// is an edit that names that one key.
type HookEdit struct {
	URL              string
	Token            bool
	SigningToken     bool
	URLVariableKeys  []string
	CustomHeaderKeys []string
}

// needsRead reports whether the edit sends something only the hook GitLab
// holds can judge: an address, which redirects the delivery only if it
// differs, or a key, which loses a value only if the hook already has it.
func (e HookEdit) needsRead() bool {
	return e.URL != "" || len(e.URLVariableKeys) > 0 || len(e.CustomHeaderKeys) > 0
}

// HookEditNeedsRead reports whether the guard has to read the hook before
// the edit is sent: the edit sends an address or a key, and nothing has
// confirmed the call yet. An edit that sends only a token is asked about
// without a read, since GitLab does not say whether the hook has one, and an
// edit of events, name, description, branch filter or SSL verification is
// neither read for nor asked about.
func HookEditNeedsRead(ctx context.Context, edit HookEdit) bool {
	return edit.needsRead() && !GuardedCallAnswered(ctx)
}

// Changes names what the edit changes, given what reading the hook answered:
// the hook, or nil when it was not read, and the error of the read.
//
// A hook that does not exist has nothing to redirect and no secret to lose,
// and the write that follows is refused for the same reason, so a 404 asks
// nothing. Any other failure of the read names the address and every key the
// edit sends as unread: a guard that cannot see the hook must not assume the
// edit is harmless. A token is named whatever the read answered.
//
// The address is shown as its origin alone, since the path of a webhook URL
// is where its secret usually lives, and a key is shown without its value.
func (e HookEdit) Changes(current *HookState, readErr error) []i18n.Message {
	if IsHTTPStatus(readErr, http.StatusNotFound) {
		return nil
	}
	var changes []i18n.Message
	unread := readErr != nil || current == nil
	if e.URL != "" {
		if unread {
			changes = append(changes, i18n.New(i18n.ConfirmHookURLUnread, i18n.S("url", RedactURLToOrigin(e.URL))))
		} else if e.URL != current.URL {
			changes = append(changes, i18n.New(i18n.ConfirmHookURL,
				i18n.S("url", RedactURLToOrigin(e.URL)), i18n.S("current", RedactURLToOrigin(current.URL))))
		}
	}
	if e.Token {
		changes = append(changes, i18n.New(i18n.ConfirmHookToken, i18n.S("token", "token")))
	}
	if e.SigningToken {
		changes = append(changes, i18n.New(i18n.ConfirmHookToken, i18n.S("token", "signing_token")))
	}
	var held HookState
	if !unread {
		held = *current
	}
	changes = append(changes, hookKeyChanges(hookFieldURLVariables, e.URLVariableKeys, held.URLVariableKeys, unread)...)
	changes = append(changes, hookKeyChanges(hookFieldCustomHeaders, e.CustomHeaderKeys, held.CustomHeaderKeys, unread)...)
	return changes
}

// hookKeyChanges names each key of field the edit sends that the hook
// already holds, or every key it sends when the hook could not be read.
func hookKeyChanges(field string, sent, held []string, unread bool) []i18n.Message {
	var changes []i18n.Message
	for _, key := range sent {
		if unread {
			changes = append(changes, i18n.New(i18n.ConfirmHookValueUnread, i18n.S("field", field), i18n.S("key", key)))
		} else if slices.Contains(held, key) {
			changes = append(changes, i18n.New(i18n.ConfirmHookValue, i18n.S("field", field), i18n.S("key", key)))
		}
	}
	return changes
}

// ConfirmHookDeliveryChange asks the caller to confirm a call that changes
// where a webhook delivers or replaces a secret it sends. op is the
// operation label the refusals open with, the handler's own; target names
// the project, the group or the instance; hookID is the hook; changes is
// what [HookEdit.Changes] found. With no change it asks nothing.
//
// Editing a webhook is not destructive by itself, and is deliberately not
// classified as such: turning an event off, renaming the hook or narrowing
// its branch filter hands nothing over, and a prompt in front of them is one
// in front of switching a hostile hook off. Two things an edit can do are
// not undone by another edit. Pointing the hook at another address sends the
// events it subscribes to, to a host the call names, which is what adding a
// webhook does and why that is destructive. And the token, the signing
// token, and the values of URL variables and custom headers are write-only:
// GitLab never returns them, so the one an edit replaces is gone unless the
// caller kept it, which is what the destructive deletes of a URL variable
// and of a custom header lose. A URL variable can also hold the host, so
// replacing one can redirect the hook as a new address does.
//
// It returns nil when the call may proceed.
func ConfirmHookDeliveryChange(ctx context.Context, op string, target i18n.Message, hookID int64, changes []i18n.Message) error {
	if len(changes) == 0 {
		return nil
	}
	question := i18n.New(i18n.ConfirmHookDelivery,
		i18n.S("hook", strconv.FormatInt(hookID, 10)), i18n.M("target", target), i18n.L("changes", changes...))
	return ConfirmGuardedCall(ctx, op, HookDeliveryConfirmID, question,
		"the user declined changing where the webhook delivers or the secrets it sends; send only the settings that do neither, or leave the webhook as it is")
}
