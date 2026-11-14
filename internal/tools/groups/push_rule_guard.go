// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// push_rule_guard.go guards the edit of a group's push rules that relaxes
// one in force: a rule turned off, a pattern removed or replaced, a file size
// limit raised or removed.

package groups

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// confirmPushRuleRelaxation asks the caller to confirm an edit that would
// relax the group's push rules, and returns nil when the edit may proceed.
//
// The rule is the one [toolutil.ConfirmPushRuleRelaxation] documents, shared
// with the project's edit. What this adds is the read: the arguments say that
// an edit may relax a rule, and only the rule GitLab holds says whether it
// does, so the guard reads it once, when the edit sends something that can
// relax and nothing has confirmed the call. An edit that only turns rules on
// is never read for.
//
// A read the caller abandoned is answered with the context's error rather
// than with a question nobody is there to answer.
func confirmPushRuleRelaxation(ctx context.Context, client *gitlabclient.Client, input EditPushRuleInput) error {
	edit := input.pushRuleEdit()
	if !toolutil.PushRuleEditNeedsRead(ctx, edit) {
		return nil
	}
	rule, _, readErr := client.GL().Groups.GetGroupPushRules(string(input.GroupID), gl.WithContext(ctx))
	if err := ctx.Err(); err != nil {
		return err
	}
	target := i18n.New(i18n.ConfirmPushRuleGroup, i18n.S("id", string(input.GroupID)))
	return toolutil.ConfirmPushRuleRelaxation(ctx, "groupEditPushRule", target,
		edit.RelaxationsAfterRead(pushRuleState(rule), readErr))
}

// pushRuleEdit restates the edit in the terms the shared rule reads.
func (input EditPushRuleInput) pushRuleEdit() toolutil.PushRuleEdit {
	return toolutil.PushRuleEdit{
		DenyDeleteTag:              input.DenyDeleteTag,
		MemberCheck:                input.MemberCheck,
		PreventSecrets:             input.PreventSecrets,
		CommitCommitterCheck:       input.CommitCommitterCheck,
		CommitCommitterNameCheck:   input.CommitCommitterNameCheck,
		RejectUnsignedCommits:      input.RejectUnsignedCommits,
		RejectNonDCOCommits:        input.RejectNonDCOCommits,
		CommitMessageRegex:         input.CommitMessageRegex,
		CommitMessageNegativeRegex: input.CommitMessageNegativeRegex,
		BranchNameRegex:            input.BranchNameRegex,
		AuthorEmailRegex:           input.AuthorEmailRegex,
		FileNameRegex:              input.FileNameRegex,
		MaxFileSize:                input.MaxFileSize,
	}
}

// pushRuleState is the rule GitLab answered the read with, or nil when the
// read answered none. The group route answers 404 for a group without push
// rules, so a nil rule arrives with the error that says so.
func pushRuleState(rule *gl.GroupPushRules) *toolutil.PushRuleState {
	if rule == nil {
		return nil
	}
	return &toolutil.PushRuleState{
		DenyDeleteTag:              rule.DenyDeleteTag,
		MemberCheck:                rule.MemberCheck,
		PreventSecrets:             rule.PreventSecrets,
		CommitCommitterCheck:       rule.CommitCommitterCheck,
		CommitCommitterNameCheck:   rule.CommitCommitterNameCheck,
		RejectUnsignedCommits:      rule.RejectUnsignedCommits,
		RejectNonDCOCommits:        rule.RejectNonDCOCommits,
		CommitMessageRegex:         rule.CommitMessageRegex,
		CommitMessageNegativeRegex: rule.CommitMessageNegativeRegex,
		BranchNameRegex:            rule.BranchNameRegex,
		AuthorEmailRegex:           rule.AuthorEmailRegex,
		FileNameRegex:              rule.FileNameRegex,
		MaxFileSize:                rule.MaxFileSize,
	}
}
