// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// modify_guard.go guards the administrator's edit of a user that replaces
// what the account signs in or is recovered with: its password, its email
// address, its external identity.

package users

import (
	"context"
	"strconv"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmAccessReplaced asks the caller to confirm an edit that replaces
// what a user signs in or recovers the account with, and returns nil when
// the edit may proceed.
//
// Modifying a user is not destructive by itself, and is deliberately not
// classified as such: the name, the profile, the limits and the admin,
// auditor and external flags are each undone by the same action with the
// previous value, which the read publishes. Three settings are not:
//
//   - password replaces a credential no call reads or restores, which is
//     what a rotation does to a token, and lets whoever sent it sign in as
//     the user,
//   - email makes another mailbox the account's, and a mailbox that confirms
//     an address can reset the account's password, with or without
//     skip_reconfirmation, since the confirmation is sent to the new address,
//   - provider and extern_uid replace the external identity the account
//     signs in with.
//
// The arguments alone decide, so nothing is read: the password and the
// external UID cannot be compared with what GitLab holds, and asking about
// an address that happens to be the current one costs a question rather than
// a read on every edit. The question names the address, which the person
// approving has to judge, and never the password or the external UID.
func confirmAccessReplaced(ctx context.Context, input ModifyInput) error {
	var changes []i18n.Message
	if input.Password != "" {
		changes = append(changes, i18n.New(i18n.ConfirmUserAccessPassword))
	}
	if input.Email != "" {
		changes = append(changes, i18n.New(i18n.ConfirmUserAccessEmail, i18n.S("email", input.Email)))
	}
	if input.Provider != "" || input.ExternUID != "" {
		changes = append(changes, i18n.New(i18n.ConfirmUserAccessIdentity))
	}
	if len(changes) == 0 {
		return nil
	}
	question := i18n.New(i18n.ConfirmUserAccess,
		i18n.S("id", strconv.FormatInt(input.UserID, 10)), i18n.L("changes", changes...))
	return toolutil.ConfirmGuardedCall(ctx, "modify_user", toolutil.UserAccessConfirmID, question,
		"the user declined replacing the account's password, email address or external identity; send the edit without them, or leave the account as it is")
}
