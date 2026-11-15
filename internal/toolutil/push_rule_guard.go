// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// push_rule_guard.go decides when an edit of push rules relaxes them, and
// asks before it does. The rule is one for a project's push rules and a
// group's, which GitLab edits through two routes that take the same thirteen
// settings, so each domain package reads its own rule and hands it here.

package toolutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/elicitation"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// PushRuleRelaxConfirmID identifies this confirmation inside a multi
// round-trip elicitation flow, keeping it distinct from the destructive-action
// prompt a destructive route raises. One key serves the project's edit and
// the group's, which ask the same question.
const PushRuleRelaxConfirmID = "push_rule_relax"

// PushRuleRelaxUsage is what the usage of both edits tells a model about the
// confirmation, so the project's and the group's cannot say different things.
const PushRuleRelaxUsage = "An edit that turns a rule off, removes a pattern or replaces it with another, or raises or removes max_file_size relaxes what the rules reject, so it needs confirm=true (or an approved elicitation prompt): pass it only after the user approves. An edit that only adds or tightens rules asks nothing."

// PushRuleState is the push rule GitLab holds for a project or a group: the
// thirteen settings both edit routes take, as the read publishes them.
//
// Every setting restricts what may be pushed, and each has a value that
// restricts nothing: a switch that is off, a pattern that is empty, and a
// max_file_size of 0, which GitLab reads as no limit of the rule's own.
type PushRuleState struct {
	DenyDeleteTag              bool
	MemberCheck                bool
	PreventSecrets             bool
	CommitCommitterCheck       bool
	CommitCommitterNameCheck   bool
	RejectUnsignedCommits      bool
	RejectNonDCOCommits        bool
	CommitMessageRegex         string
	CommitMessageNegativeRegex string
	BranchNameRegex            string
	AuthorEmailRegex           string
	FileNameRegex              string
	MaxFileSize                int64
}

// PushRuleEdit is what one edit sends: a nil field is a setting the call does
// not name, which GitLab leaves as it is.
type PushRuleEdit struct {
	DenyDeleteTag              *bool
	MemberCheck                *bool
	PreventSecrets             *bool
	CommitCommitterCheck       *bool
	CommitCommitterNameCheck   *bool
	RejectUnsignedCommits      *bool
	RejectNonDCOCommits        *bool
	CommitMessageRegex         *string
	CommitMessageNegativeRegex *string
	BranchNameRegex            *string
	AuthorEmailRegex           *string
	FileNameRegex              *string
	MaxFileSize                *int64
}

// pushRuleSwitch is one on/off setting of an edit beside the value GitLab
// holds for it.
type pushRuleSwitch struct {
	name    string
	sent    *bool
	current bool
}

// pushRulePattern is one regex setting of an edit beside the value GitLab
// holds for it.
type pushRulePattern struct {
	name    string
	sent    *string
	current string
}

// switches lists the seven on/off settings in the fixed order every question
// names them in.
func (e PushRuleEdit) switches(current PushRuleState) []pushRuleSwitch {
	return []pushRuleSwitch{
		{name: "deny_delete_tag", sent: e.DenyDeleteTag, current: current.DenyDeleteTag},
		{name: "member_check", sent: e.MemberCheck, current: current.MemberCheck},
		{name: "prevent_secrets", sent: e.PreventSecrets, current: current.PreventSecrets},
		{name: "commit_committer_check", sent: e.CommitCommitterCheck, current: current.CommitCommitterCheck},
		{name: "commit_committer_name_check", sent: e.CommitCommitterNameCheck, current: current.CommitCommitterNameCheck},
		{name: "reject_unsigned_commits", sent: e.RejectUnsignedCommits, current: current.RejectUnsignedCommits},
		{name: "reject_non_dco_commits", sent: e.RejectNonDCOCommits, current: current.RejectNonDCOCommits},
	}
}

// patterns lists the five regex settings in the fixed order every question
// names them in.
//
// Three of them say what must match and two what must not, and the guard
// reads all five alike: empty restricts nothing in either sense, since GitLab
// applies a pattern only when one is present.
func (e PushRuleEdit) patterns(current PushRuleState) []pushRulePattern {
	return []pushRulePattern{
		{name: "commit_message_regex", sent: e.CommitMessageRegex, current: current.CommitMessageRegex},
		{name: "commit_message_negative_regex", sent: e.CommitMessageNegativeRegex, current: current.CommitMessageNegativeRegex},
		{name: "branch_name_regex", sent: e.BranchNameRegex, current: current.BranchNameRegex},
		{name: "author_email_regex", sent: e.AuthorEmailRegex, current: current.AuthorEmailRegex},
		{name: "file_name_regex", sent: e.FileNameRegex, current: current.FileNameRegex},
	}
}

// MayRelax reports whether the edit sends a value that can relax a rule,
// which is all the arguments say alone: a switch turned off, any pattern, or
// any max_file_size. Whether it does relax one depends on the rule GitLab
// holds, which [PushRuleEdit.Relaxations] compares it with.
//
// An edit that only turns switches on adds restrictions whatever the rule is,
// so it is never read for and never asked about.
func (e PushRuleEdit) MayRelax() bool {
	for _, setting := range e.switches(PushRuleState{}) {
		if setting.sent != nil && !*setting.sent {
			return true
		}
	}
	for _, setting := range e.patterns(PushRuleState{}) {
		if setting.sent != nil {
			return true
		}
	}
	return e.MaxFileSize != nil
}

// Relaxations names what the edit relaxes in the rule GitLab holds, one
// message per setting, in a fixed order. It is empty when the edit only adds
// restrictions, tightens them or resends what is there.
//
// A switch relaxes when the edit turns off one that is on. A pattern relaxes
// when the edit empties one that is set, and is treated as relaxing when the
// edit replaces one that is set with a different one: whether the new pattern
// accepts more than the old is a question about two regular languages this
// server does not decide, so it asks rather than assume the answer that
// needs no question. A pattern set where there was none is a new restriction.
// max_file_size relaxes when the edit raises a limit that is set or removes
// it by sending 0. A limit set where there was none, or lowered, tightens. A
// negative size relaxes nothing, since GitLab refuses it.
func (e PushRuleEdit) Relaxations(current PushRuleState) []i18n.Message {
	var changes []i18n.Message
	for _, setting := range e.switches(current) {
		if setting.sent != nil && !*setting.sent && setting.current {
			changes = append(changes, i18n.New(i18n.ConfirmPushRuleSwitchOff, i18n.S("setting", setting.name)))
		}
	}
	for _, setting := range e.patterns(current) {
		if setting.sent == nil || setting.current == "" {
			continue
		}
		// An if chain rather than a tagless switch: a switch case carries no
		// statement counter, so a mutation run cannot see its condition.
		if *setting.sent == "" {
			changes = append(changes, i18n.New(i18n.ConfirmPushRulePatternRemoved, i18n.S("setting", setting.name)))
		} else if *setting.sent != setting.current {
			changes = append(changes, i18n.New(i18n.ConfirmPushRulePatternReplaced, i18n.S("setting", setting.name)))
		}
	}
	if e.MaxFileSize == nil || current.MaxFileSize <= 0 {
		return changes
	}
	if *e.MaxFileSize == 0 {
		return append(changes, i18n.New(i18n.ConfirmPushRuleSizeRemoved, i18n.N("from", current.MaxFileSize)))
	}
	if *e.MaxFileSize > current.MaxFileSize {
		return append(changes, i18n.New(i18n.ConfirmPushRuleSizeRaised,
			i18n.N("from", current.MaxFileSize), i18n.N("to", *e.MaxFileSize)))
	}
	return changes
}

// UnreadRelaxations names every setting the edit may relax, for a rule that
// could not be read: the settings [PushRuleEdit.MayRelax] counts, in the
// order [PushRuleEdit.Relaxations] names them.
func (e PushRuleEdit) UnreadRelaxations() []i18n.Message {
	var changes []i18n.Message
	unread := func(setting string) {
		changes = append(changes, i18n.New(i18n.ConfirmPushRuleUnread, i18n.S("setting", setting)))
	}
	for _, setting := range e.switches(PushRuleState{}) {
		if setting.sent != nil && !*setting.sent {
			unread(setting.name)
		}
	}
	for _, setting := range e.patterns(PushRuleState{}) {
		if setting.sent != nil {
			unread(setting.name)
		}
	}
	if e.MaxFileSize != nil {
		unread("max_file_size")
	}
	return changes
}

// RelaxationsAfterRead names what the edit relaxes, given what reading the
// rule answered: the rule, or nil when GitLab answered that there is none,
// and the error of the read.
//
// A rule that does not exist has nothing to relax, and GitLab says so in two
// ways: the project route answers 200 with no rule, and the group route, or
// either route for an object the caller cannot see, answers 404. The write
// that follows is refused for the same reason, so the edit goes on to the
// answer GitLab gives it. Any other failure of the read names every setting
// the edit may relax as unread: a guard that cannot see the rule must not
// assume the edit is harmless.
func (e PushRuleEdit) RelaxationsAfterRead(current *PushRuleState, readErr error) []i18n.Message {
	if IsHTTPStatus(readErr, http.StatusNotFound) {
		return nil
	}
	if readErr != nil {
		return e.UnreadRelaxations()
	}
	if current == nil {
		return nil
	}
	return e.Relaxations(*current)
}

// PushRuleEditNeedsRead reports whether the guard has to read the rule
// GitLab holds before the edit is sent: the edit may relax a rule, and
// nothing has confirmed it yet.
//
// The two ways through are checked before the read, as for the other guards
// that ask: with confirmation switched off for the server, or confirm=true on the
// call, nobody is going to be asked, so the call does not need to know what
// it is about to relax.
func PushRuleEditNeedsRead(ctx context.Context, edit PushRuleEdit) bool {
	if !edit.MayRelax() {
		return false
	}
	return !IsYOLOMode() && !ExplicitConfirmFromRequest(RequestFromContext(ctx))
}

// ConfirmPushRuleRelaxation asks the caller to confirm an edit that relaxes
// push rules. op is the operation label the refusals open with, the
// handler's own; target names the project or the group; changes is what
// [PushRuleEdit.RelaxationsAfterRead] found. With no change it asks nothing.
//
// Editing push rules is not destructive by itself, and is deliberately not
// classified as such: most edits add a restriction or tighten one, and a
// prompt raised by every edit is a prompt a caller learns to pass through.
// What an edit can take away is a restriction that is in force, after which
// GitLab accepts the pushes it rejected: commits that carry secrets, unsigned
// commits, deleted tags, files of any size. Deleting the rules does that to
// all of them at once and is destructive. This asks about the same effect
// one setting at a time, so neither action is a way around the question the
// other asks.
//
// The question has two readers: the person it is asked of, in the request's
// language, and the model, which reads it in English as the refusal's opening
// sentence when the client cannot ask. It names each setting and the sizes,
// and never a pattern: the one GitLab holds is text from GitLab, and the one
// the call sends is the caller's.
//
// Confirmation follows the precedence of destructive actions, whose first
// two steps [PushRuleEditNeedsRead] takes before the rule is read: YOLO mode
// and an explicit confirm=true proceed, otherwise the caller is elicited.
// Clients without elicitation get an actionable error telling them to resend
// with confirm=true, so a rule is never relaxed silently.
//
// It returns nil when the edit may proceed.
func ConfirmPushRuleRelaxation(ctx context.Context, op string, target i18n.Message, changes []i18n.Message) error {
	if len(changes) == 0 {
		return nil
	}
	question := i18n.New(i18n.ConfirmPushRuleRelax, i18n.M("target", target), i18n.L("changes", changes...))

	req := RequestFromContext(ctx)
	flow, flowErr := elicitation.FlowFromRequest(ctx, req)
	if flowErr != nil || !flow.IsSupported() {
		return fmt.Errorf(
			"%s: %s Re-send the same call with confirm=true once the user has approved it",
			op, question.English(),
		)
	}

	confirmed, err := flow.Confirm(ctx, PushRuleRelaxConfirmID, question.Text(ctx))
	// A chain of ifs rather than a switch: a switch case carries no statement
	// counter, so a mutation run reported each condition as not covered
	// whatever the tests did.
	if errors.Is(err, elicitation.ErrInputPending) {
		return flow.PendingError()
	}
	if err != nil {
		return fmt.Errorf("%s: confirmation failed: %w", op, err)
	}
	if !confirmed {
		return fmt.Errorf("%s: the user declined relaxing the push rules; send only the settings that add or tighten a rule, or leave them as they are", op)
	}
	return nil
}
