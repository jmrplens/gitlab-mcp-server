// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// guarded_call.go is the confirmation a handler asks for when only some of
// an action's calls do something no other call undoes, or hand something to
// a party the call names, and the arguments decide which. The action is not
// classified destructive, since that would ask before every ordinary call,
// and its handler asks here instead, on the precedence of destructive
// actions.

package toolutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/elicitation"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// Confirmation identifiers of the guarded calls. Each identifies its question
// inside a multi round-trip elicitation flow and is the key an elicitation
// hook matches, distinct from the destructive-action prompt a destructive
// route raises. Actions that ask one question share one key.
const (
	// VariableValueConfirmID is the question the three CI/CD variable updates
	// ask before they replace a value.
	VariableValueConfirmID = "variable_value_replace"
	// UserAccessConfirmID is the question user.modify asks before it replaces
	// what an account signs in or is recovered with.
	UserAccessConfirmID = "user_access_replace"
	// HookDeliveryConfirmID is the question the webhook edits and the calls
	// that set one URL variable or custom header ask.
	HookDeliveryConfirmID = "hook_delivery_change"
	// StatusCheckConfirmID is the question the edit of an external status
	// check asks.
	StatusCheckConfirmID = "status_check_delivery_change"
	// ImportOverwriteConfirmID is the question a project import asks before
	// it overwrites the project at its path.
	ImportOverwriteConfirmID = "project_import_overwrite"
	// CommitForceConfirmID is the question a commit asks before it resets its
	// branch.
	CommitForceConfirmID = "commit_force"
	// ExportUploadConfirmID is the question a project export asks before
	// GitLab uploads the archive to a URL.
	ExportUploadConfirmID = "project_export_upload"
)

// Usage sentences of the guarded calls, written once so the actions that ask
// one question cannot describe it differently.
const (
	// VariableValueUsage is what the three CI/CD variable updates say.
	VariableValueUsage = "Sending value replaces the value the variable holds, which no call restores, so it needs confirm=true (or an approved elicitation prompt): pass it only after the user approves. An update that sends no value asks nothing."
	// HookAddUsage is what the three actions that add a webhook say.
	HookAddUsage = "Destructive: from then on GitLab posts the events the hook subscribes to, to the host named in url, and deleting the hook does not recall what was sent, so confirm with the user first."
	// HookEditUsage is what the three webhook edits say.
	HookEditUsage = "An edit that points the hook at another url, sends a token or signing token, or replaces a URL variable or custom header the hook already has changes where its events go or loses a secret that cannot be read back, so it needs confirm=true (or an approved elicitation prompt): pass it only after the user approves. An edit of events, name, description, branch filter or SSL verification asks nothing."
	// HookSetValueUsage is what the calls that set one URL variable or one
	// custom header say.
	HookSetValueUsage = "Replacing a key the hook already has loses its current value, which cannot be read back, so it needs confirm=true (or an approved elicitation prompt): pass it only after the user approves. A new key asks nothing."
)

// GuardedCallAnswered reports whether a guarded call needs nobody's answer:
// the operator switched confirmation off for the server, or the call carries
// confirm=true. A guard that reads GitLab to decide checks this first, since
// a call nobody is going to be asked about does not need to know what it is
// about to change.
func GuardedCallAnswered(ctx context.Context) bool {
	return IsYOLOMode() || ExplicitConfirmFromRequest(RequestFromContext(ctx))
}

// ConfirmGuardedCall asks the caller to confirm one call of an action that is
// not destructive, and returns nil when the call may proceed.
//
// op is the operation label the refusals open with, the handler's own.
// confirmID names the question in the elicitation flow. question is what is
// asked, and declined is what the refusal says after the operation label when
// the person answers no: what was declined and the call that asks nothing.
//
// The question has two readers: the person it is asked of, in the request's
// language, and the model, which reads it in English as the refusal's opening
// sentence when the client cannot ask. Whoever builds it leaves out every
// value that is a secret or can carry one: a variable's value, a token, a
// password, and of a URL everything but its origin.
//
// Confirmation follows the precedence of destructive actions: YOLO mode and
// an explicit confirm=true proceed, otherwise the caller is elicited. A
// client without elicitation gets an actionable error telling it to send the
// call again with confirm=true, so the call never goes out silently.
func ConfirmGuardedCall(ctx context.Context, op, confirmID string, question i18n.Message, declined string) error {
	if GuardedCallAnswered(ctx) {
		return nil
	}
	req := RequestFromContext(ctx)
	flow, flowErr := elicitation.FlowFromRequest(ctx, req)
	if flowErr != nil || !flow.IsSupported() {
		return fmt.Errorf(
			"%s: %s Re-send the same call with confirm=true once the user has approved it",
			op, question.English(),
		)
	}

	confirmed, err := flow.Confirm(ctx, confirmID, question.Text(ctx))
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
		return fmt.Errorf("%s: %s", op, declined)
	}
	return nil
}

// ConfirmTargetProject names a project in a guard's question.
func ConfirmTargetProject(id string) i18n.Message {
	return i18n.New(i18n.ConfirmTargetProject, i18n.S("id", id))
}

// ConfirmTargetGroup names a group in a guard's question.
func ConfirmTargetGroup(id string) i18n.Message {
	return i18n.New(i18n.ConfirmTargetGroup, i18n.S("id", id))
}

// ConfirmTargetInstance names the GitLab instance in a guard's question, for
// an object that belongs to no project or group.
func ConfirmTargetInstance() i18n.Message {
	return i18n.New(i18n.ConfirmTargetInstance)
}

// ConfirmImportOverwrite asks before a project import overwrites the project
// that is already at its path, and returns nil when the import may proceed.
// overwrite is the call's flag: without it GitLab refuses a path that is
// taken, the import only ever creates a project, and nothing is asked.
//
// With the flag GitLab replaces the project at that path with the archive,
// its repository, issues and merge requests included, and no call brings the
// replaced project back, which is the effect project.delete is destructive
// for. The arguments alone decide, so nothing is read. path is the namespace
// and the path the import is aimed at, as the call names them.
func ConfirmImportOverwrite(ctx context.Context, op string, overwrite bool, path string) error {
	if !overwrite {
		return nil
	}
	question := i18n.New(i18n.ConfirmImportOverwrite, i18n.S("path", path))
	return ConfirmGuardedCall(ctx, op, ImportOverwriteConfirmID, question,
		"the user declined overwriting the existing project; send the import without overwrite to create a project at a free path")
}

// ConfirmCommitForce asks before a commit is created with force, and returns
// nil when the commit may proceed. force is the call's flag: without it the
// commit is added on top of the branch, repository.commit_revert undoes it,
// and nothing is asked.
//
// With the flag GitLab resets the target branch to the starting point the
// call names, start_branch or start_sha, plus the new commit, so the commits
// the branch held that the starting point does not are dropped from it and no
// action puts them back on the branch. The arguments alone decide, so nothing
// is read.
func ConfirmCommitForce(ctx context.Context, op string, force bool, projectID, branch string) error {
	if !force {
		return nil
	}
	question := i18n.New(i18n.ConfirmCommitForce, i18n.S("branch", branch), i18n.M("target", ConfirmTargetProject(projectID)))
	return ConfirmGuardedCall(ctx, op, CommitForceConfirmID, question,
		"the user declined resetting the branch; send the commit without force to add it on top of the branch")
}

// ConfirmExportUpload asks before a project export is scheduled with an
// upload URL, and returns nil when the export may proceed. uploadURL is the
// address the call names, empty when it names none: an export without one
// stays on the instance for whoever may download it, hands nothing over, and
// asks nothing.
//
// With an address GitLab uploads the finished archive to it: the whole
// project, its repository, issues, merge requests and settings, sent to a
// host the call names, which is more than the push mirror hands over and the
// reason adding one is destructive. The arguments alone decide, so nothing
// is read. The question shows the address as its origin alone, since a
// presigned upload URL carries its credential in the path and the query.
func ConfirmExportUpload(ctx context.Context, op, projectID, uploadURL string) error {
	if uploadURL == "" {
		return nil
	}
	question := i18n.New(i18n.ConfirmExportUpload,
		i18n.M("target", ConfirmTargetProject(projectID)), i18n.S("url", RedactURLToOrigin(uploadURL)))
	return ConfirmGuardedCall(ctx, op, ExportUploadConfirmID, question,
		"the user declined uploading the export to that address; schedule the export without an upload URL and download it from the instance instead")
}

// ConfirmVariableValueReplaced asks before an update replaces the value of a
// CI/CD variable, and returns nil when the update may proceed. sendsValue is
// whether the call names value at all, an empty one included: an update that
// does not asks nothing.
//
// Updating a variable is not destructive by itself, and is deliberately not
// classified as such: protected, masked, raw, the type, the description and
// the scope are each undone by the same action with the previous setting,
// which the read publishes. The value is the exception: no call restores the
// one an update replaces, GitLab never returns a hidden one and the project
// read redacts a masked one, so it comes back only if somebody kept it, and
// deleting the variable, which loses the same value, is destructive. A value
// also decides where a pipeline deploys or publishes to, so replacing it can
// send the next pipeline and its secrets elsewhere.
//
// The guard reads nothing: whether the value differs from the one GitLab
// holds cannot be read for a hidden variable, and a guard that read only the
// others would ask by a rule nobody could predict. The question names the
// variable and never a value.
func ConfirmVariableValueReplaced(ctx context.Context, op string, sendsValue bool, target i18n.Message, key string) error {
	if !sendsValue {
		return nil
	}
	question := i18n.New(i18n.ConfirmVariableValue, i18n.S("key", key), i18n.M("target", target))
	return ConfirmGuardedCall(ctx, op, VariableValueConfirmID, question,
		"the user declined replacing the variable's value; send the update without value to change its other settings, or leave it as it is")
}
