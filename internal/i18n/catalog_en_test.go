// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// frozenEnglish is one row of the proof that moving a text into the catalog
// changed nothing a client receives in English: a key, the dotted value it is
// stable under, the arguments a call site passes, and the text the server
// sent before the move.
type frozenEnglish struct {
	key    Key
	dotted string
	args   []Arg
	want   string
}

// Sample values the frozen rows render with. A placeholder replaces an fmt
// verb, so the old literal is rendered through fmt with the same values the
// key is rendered with, and the two have to agree byte for byte.
const (
	sampleProject    = "group/app"
	sampleTitle      = "Fix the login"
	sampleText       = "First line of the description"
	sampleLabels     = "bug, ui"
	sampleSource     = "feature/login"
	sampleTarget     = "main"
	sampleTag        = "v1.2.0"
	sampleName       = "Release one"
	sampleVisibility = "private"
	sampleBranch     = "trunk"
	sampleTool       = `delete "it"`
	sampleAction     = "delete"
	sampleURL        = "https://mirror.example.com/app.git"
	sampleRegex      = "^release/"
	samplePath       = "group/app"
	sampleNamespace  = "group/archive"
	sampleFile       = "app-1.2.0.tgz"
	sampleIID        = 42
	sampleCount      = 3
	sampleRead       = 65536
	sampleTotal      = 1048576
	sampleAttempt    = 7
	sampleID         = 1234
	sampleIndex      = 2
	sampleSetting    = "prevent_secrets"
)

// frozenEnglishRows holds every English entry against the literal it
// replaced, copied from commit 8f339f4e8, the base of issue 1547's stack.
// A row whose old text was an fmt template keeps that template and renders it
// with the sample values, except where the template was one %s at the end
// (the description row), which is written as the concatenation it renders
// to; confirm.destructive.standalone quoted the tool name with %q, which is
// what strconv.Quote writes, so the call site passes the quoted name and the
// row renders the old template with the bare one. The pull mirror questions
// written after that stack are [pullMirrorQuestionRows], the push rule
// question's are [pushRuleQuestionRows] and those of the guards that ask about
// one call of an action are [guardedCallQuestionRows], all appended here so
// the tests below hold every entry.
func frozenEnglishRows() []frozenEnglish {
	rows := []frozenEnglish{
		{key: ElicitationSchemaConfirmTitle, dotted: "elicitation.schema.confirm.title", want: "Confirm"},
		{key: ElicitationSchemaSelectionTitle, dotted: "elicitation.schema.selection.title", want: "Selection"},
		{key: ElicitationSchemaSelectionsTitle, dotted: "elicitation.schema.selections.title", want: "Selections"},
		{key: InteractiveFieldTitle, dotted: "interactive.field.title", want: "title"},
		{key: InteractiveFieldDescription, dotted: "interactive.field.description", want: "description"},
		{key: InteractiveFieldLabels, dotted: "interactive.field.labels", want: "labels"},
		{key: InteractiveFieldSourceBranch, dotted: "interactive.field.source_branch", want: "source_branch"},
		{key: InteractiveFieldTargetBranch, dotted: "interactive.field.target_branch", want: "target_branch"},
		{key: InteractiveFieldTagName, dotted: "interactive.field.tag_name", want: "tag_name"},
		{key: InteractiveFieldName, dotted: "interactive.field.name", want: "name"},
		{key: InteractiveFieldDefaultBranch, dotted: "interactive.field.default_branch", want: "default_branch"},
		{key: InteractivePromptLabels, dotted: "interactive.prompt.labels", want: "Enter comma-separated labels (or leave empty)"},
		{
			key: InteractiveSummaryDescription, dotted: "interactive.summary.description",
			args: []Arg{S("description", sampleText)},
			want: "\n**Description**: " + sampleText,
		},
		{
			key: InteractiveSummaryLabels, dotted: "interactive.summary.labels",
			args: []Arg{S("labels", sampleLabels)},
			want: "\n**Labels**: " + sampleLabels,
		},
		{key: InteractiveProgressOptionalFields, dotted: "interactive.progress.optional_fields", want: "Collecting optional fields..."},
		{key: InteractiveIssueCreateProgressDetails, dotted: "interactive.issue_create.progress.details", want: "Collecting issue details..."},
		{key: InteractiveIssueCreatePromptTitle, dotted: "interactive.issue_create.prompt.title", want: "Enter the issue title"},
		{
			key: InteractiveIssueCreatePromptDescription, dotted: "interactive.issue_create.prompt.description",
			want: "Enter the issue description (Markdown supported, or leave empty)",
		},
		{key: InteractiveIssueCreatePromptConfidential, dotted: "interactive.issue_create.prompt.confidential", want: "Should this issue be confidential?"},
		{key: InteractiveIssueCreateProgressConfirming, dotted: "interactive.issue_create.progress.confirming", want: "Confirming issue creation..."},
		{
			key: InteractiveIssueCreateSummary, dotted: "interactive.issue_create.summary",
			args: []Arg{S("project", sampleProject), S("title", sampleTitle)},
			want: fmt.Sprintf("Create issue in project %s?\n\n**Title**: %s", sampleProject, sampleTitle),
		},
		{key: InteractiveIssueCreateSummaryConfidential, dotted: "interactive.issue_create.summary.confidential", want: "\n**Confidential**: Yes"},
		{key: InteractiveIssueCreateProgressCreating, dotted: "interactive.issue_create.progress.creating", want: "Creating issue..."},
		{key: InteractiveIssueCreateProgressDone, dotted: "interactive.issue_create.progress.done", want: "Issue created"},
		{key: InteractiveMrCreateProgressBranches, dotted: "interactive.mr_create.progress.branches", want: "Collecting branch information..."},
		{key: InteractiveMrCreatePromptSourceBranch, dotted: "interactive.mr_create.prompt.source_branch", want: "Enter the source branch name"},
		{
			key: InteractiveMrCreatePromptTargetBranch, dotted: "interactive.mr_create.prompt.target_branch",
			want: "Enter the target branch name (e.g. main, develop)",
		},
		{key: InteractiveMrCreateProgressDetails, dotted: "interactive.mr_create.progress.details", want: "Collecting MR details..."},
		{key: InteractiveMrCreatePromptTitle, dotted: "interactive.mr_create.prompt.title", want: "Enter the merge request title"},
		{
			key: InteractiveMrCreatePromptDescription, dotted: "interactive.mr_create.prompt.description",
			want: "Enter the MR description (Markdown supported, or leave empty)",
		},
		{key: InteractiveMrCreatePromptRemoveSourceBranch, dotted: "interactive.mr_create.prompt.remove_source_branch", want: "Remove source branch after merge?"},
		{key: InteractiveMrCreatePromptSquash, dotted: "interactive.mr_create.prompt.squash", want: "Squash commits on merge?"},
		{key: InteractiveMrCreateProgressConfirming, dotted: "interactive.mr_create.progress.confirming", want: "Confirming MR creation..."},
		{
			key: InteractiveMrCreateSummary, dotted: "interactive.mr_create.summary",
			args: []Arg{S("project", sampleProject), S("title", sampleTitle), S("source", sampleSource), S("target", sampleTarget)},
			want: fmt.Sprintf("Create merge request in project %s?\n\n**Title**: %s\n**Source**: %s -> **Target**: %s",
				sampleProject, sampleTitle, sampleSource, sampleTarget),
		},
		{key: InteractiveMrCreateSummaryRemoveSourceBranch, dotted: "interactive.mr_create.summary.remove_source_branch", want: "\n**Remove source branch**: Yes"},
		{key: InteractiveMrCreateSummarySquash, dotted: "interactive.mr_create.summary.squash", want: "\n**Squash commits**: Yes"},
		{key: InteractiveMrCreateProgressCreating, dotted: "interactive.mr_create.progress.creating", want: "Creating merge request..."},
		{key: InteractiveMrCreateProgressDone, dotted: "interactive.mr_create.progress.done", want: "Merge request created"},
		{key: InteractiveReleaseCreateProgressDetails, dotted: "interactive.release_create.progress.details", want: "Collecting release details..."},
		{
			key: InteractiveReleaseCreatePromptTagName, dotted: "interactive.release_create.prompt.tag_name",
			want: "Enter the tag name for the release (must already exist)",
		},
		{key: InteractiveReleaseCreatePromptName, dotted: "interactive.release_create.prompt.name", want: "Enter the release name/title"},
		{key: InteractiveReleaseCreateProgressDescription, dotted: "interactive.release_create.progress.description", want: "Collecting release description..."},
		{
			key: InteractiveReleaseCreatePromptDescription, dotted: "interactive.release_create.prompt.description",
			want: "Enter the release description/notes (Markdown supported, or leave empty)",
		},
		{key: InteractiveReleaseCreateProgressConfirming, dotted: "interactive.release_create.progress.confirming", want: "Confirming release creation..."},
		{
			key: InteractiveReleaseCreateSummary, dotted: "interactive.release_create.summary",
			args: []Arg{S("project", sampleProject), S("tag", sampleTag), S("name", sampleName)},
			want: fmt.Sprintf("Create release in project %s?\n\n**Tag**: %s\n**Name**: %s", sampleProject, sampleTag, sampleName),
		},
		{key: InteractiveReleaseCreateProgressCreating, dotted: "interactive.release_create.progress.creating", want: "Creating release..."},
		{key: InteractiveReleaseCreateProgressDone, dotted: "interactive.release_create.progress.done", want: "Release created"},
		{key: InteractiveProjectCreateProgressDetails, dotted: "interactive.project_create.progress.details", want: "Collecting project details..."},
		{key: InteractiveProjectCreatePromptName, dotted: "interactive.project_create.prompt.name", want: "Enter the project name"},
		{
			key: InteractiveProjectCreatePromptDescription, dotted: "interactive.project_create.prompt.description",
			want: "Enter the project description (or leave empty)",
		},
		{key: InteractiveProjectCreateProgressSettings, dotted: "interactive.project_create.progress.settings", want: "Collecting project settings..."},
		{key: InteractiveProjectCreatePromptVisibility, dotted: "interactive.project_create.prompt.visibility", want: "Select the project visibility"},
		{
			key: InteractiveProjectCreatePromptInitializeWithReadme, dotted: "interactive.project_create.prompt.initialize_with_readme",
			want: "Initialize the repository with a README file?",
		},
		{
			key: InteractiveProjectCreatePromptDefaultBranch, dotted: "interactive.project_create.prompt.default_branch",
			want: "Enter the default branch name (or leave empty for the instance's default branch)",
		},
		{key: InteractiveProjectCreateProgressConfirming, dotted: "interactive.project_create.progress.confirming", want: "Confirming project creation..."},
		{
			key: InteractiveProjectCreateSummary, dotted: "interactive.project_create.summary",
			args: []Arg{S("name", sampleName), S("visibility", sampleVisibility)},
			want: fmt.Sprintf("Create new GitLab project?\n\n**Name**: %s\n**Visibility**: %s", sampleName, sampleVisibility),
		},
		{key: InteractiveProjectCreateSummaryReadme, dotted: "interactive.project_create.summary.readme", want: "\n**README**: Yes"},
		{
			key: InteractiveProjectCreateSummaryDefaultBranch, dotted: "interactive.project_create.summary.default_branch",
			args: []Arg{S("branch", sampleBranch)},
			want: "\n**Default Branch**: " + sampleBranch,
		},
		{key: InteractiveProjectCreateProgressCreating, dotted: "interactive.project_create.progress.creating", want: "Creating project..."},
		{key: InteractiveProjectCreateProgressDone, dotted: "interactive.project_create.progress.done", want: "Project created"},
		{
			key: ConfirmDestructiveStandalone, dotted: "confirm.destructive.standalone",
			args: []Arg{S("tool", strconv.Quote(sampleTool))},
			want: fmt.Sprintf("Confirm destructive action %q?", sampleTool),
		},
		{
			key: ConfirmDestructiveMeta, dotted: "confirm.destructive.meta",
			args: []Arg{S("tool", sampleTool), S("action", sampleAction)},
			want: fmt.Sprintf("Confirm %s/%s? This action may be irreversible.", sampleTool, sampleAction),
		},
		{
			key: ConfirmDestructiveIndividual, dotted: "confirm.destructive.individual",
			args: []Arg{S("tool", sampleTool)},
			want: fmt.Sprintf("Confirm %s? This action may be irreversible.", sampleTool),
		},
		{
			key: ConfirmPullMirrorOverwrite, dotted: "confirm.pull_mirror.overwrite",
			args: []Arg{S("project", sampleProject), S("source", sampleURL)},
			want: fmt.Sprintf(
				"Configure project %s to pull from %s and overwrite diverged branches?"+
					" Every branch the mirror pulls that has diverged from that repository is replaced by its version on the next sync,"+
					" and the replaced commits cannot be recovered through the API.",
				sampleProject, sampleURL,
			),
		},
		{key: ConfirmPullMirrorConfiguredSource, dotted: "confirm.pull_mirror.configured_source", want: "its configured mirror source"},
		{
			key: ConfirmWorkItemClear, dotted: "confirm.work_item.clear",
			args: []Arg{
				L("losses", New(ConfirmWorkItemAssignees, N("count", sampleCount)), New(ConfirmWorkItemCrmContacts)),
				S("path", samplePath), N("iid", sampleIID),
			},
			want: fmt.Sprintf(
				"Remove %s from work item %s#%d? This replaces the list and cannot be undone from the response.",
				strings.Join([]string{fmt.Sprintf("all %d assignees", sampleCount), "every CRM contact"}, " and "), samplePath, sampleIID,
			),
		},
		{
			key: ConfirmWorkItemAssigneesUnknown, dotted: "confirm.work_item.assignees_unknown",
			want: "every assignee (the current list could not be read)",
		},
		{
			key: ConfirmWorkItemAssignees, dotted: "confirm.work_item.assignees",
			args: []Arg{N("count", sampleCount)},
			want: fmt.Sprintf("all %d assignees", sampleCount),
		},
		{key: ConfirmWorkItemCrmContacts, dotted: "confirm.work_item.crm_contacts", want: "every CRM contact"},
		{key: CommonListAnd, dotted: "common.list.and", want: " and "},
		{
			key: ProgressUploadReading, dotted: "progress.upload.reading",
			args: []Arg{N("read", sampleRead), N("total", sampleTotal)},
			want: fmt.Sprintf("Read %d / %d bytes, preparing the upload", sampleRead, sampleTotal),
		},
		{
			key: ProgressUploadHandover, dotted: "progress.upload.handover",
			args: []Arg{N("read", sampleTotal)},
			want: fmt.Sprintf("Read %d bytes, uploading to GitLab", sampleTotal),
		},
		{
			key: ProgressDownloadBytes, dotted: "progress.download.bytes",
			args: []Arg{N("written", sampleRead)},
			want: fmt.Sprintf("Downloaded %d bytes", sampleRead),
		},
		{
			key: ProgressDownloadOfTotal, dotted: "progress.download.of_total",
			args: []Arg{N("written", sampleRead), N("total", sampleTotal)},
			want: fmt.Sprintf("Downloaded %d / %d bytes", sampleRead, sampleTotal),
		},
		{
			key: ProgressPipelineWaitPoll, dotted: "progress.pipeline_wait.poll",
			args: []Arg{N("pipeline", sampleID), N("attempt", sampleAttempt)},
			want: fmt.Sprintf("Polling pipeline #%d (attempt %d, status check)...", sampleID, sampleAttempt),
		},
		{
			key: ProgressJobWaitPoll, dotted: "progress.job_wait.poll",
			args: []Arg{N("job", sampleID), N("attempt", sampleAttempt)},
			want: fmt.Sprintf("Polling job #%d (attempt %d, status check)...", sampleID, sampleAttempt),
		},
		{key: ProgressGroupTransferWait, dotted: "progress.group_transfer.wait", want: "Waiting for GitLab to move the group"},
		{
			key: ProgressProjectTransferWait, dotted: "progress.project_transfer.wait",
			args: []Arg{S("namespace", sampleNamespace)},
			want: "Waiting for GitLab to move the project to " + sampleNamespace,
		},
		{
			key: ProgressPackagePublishFile, dotted: "progress.package_publish.file",
			args: []Arg{N("index", sampleIndex), N("count", sampleCount), S("name", sampleFile)},
			want: fmt.Sprintf("Publishing file %d of %d: %s", sampleIndex, sampleCount, sampleFile),
		},
		{
			key: ProgressPackagePublishDone, dotted: "progress.package_publish.done",
			args: []Arg{N("published", sampleIndex), N("count", sampleCount)},
			want: fmt.Sprintf("Published %d of %d files", sampleIndex, sampleCount),
		},
	}
	rows = append(rows, pullMirrorQuestionRows()...)
	rows = append(rows, pushRuleQuestionRows()...)
	rows = append(rows, detachQuestionRows()...)
	return append(rows, guardedCallQuestionRows()...)
}

// guardedCallQuestionRows holds the questions of the guards that ask about one
// call of an action that is not destructive: the targets they name, and for
// each guard its question and every change it can name. They replaced no
// literal, so each row pins the English it was written with. No row carries a
// value that is a secret, and every address is an origin.
func guardedCallQuestionRows() []frozenEnglish {
	const (
		origin   = "https://hooks.example.com/..."
		current  = "https://ci.example.com"
		variable = "DEPLOY_URL"
		address  = "new@example.com"
		field    = "url_variables"
		// quoted is a line of a text as a question shows it: fenced in
		// backticks by the guard, so it cannot pass for the server's words.
		quoted = "`/move group/app`"
	)
	project := New(ConfirmTargetProject, S("id", samplePath))
	return []frozenEnglish{
		{key: ConfirmTargetProject, dotted: "confirm.target.project", args: []Arg{S("id", samplePath)}, want: "project " + samplePath},
		{key: ConfirmTargetGroup, dotted: "confirm.target.group", args: []Arg{S("id", samplePath)}, want: "group " + samplePath},
		{key: ConfirmTargetInstance, dotted: "confirm.target.instance", want: "this GitLab instance"},
		{
			key: ConfirmVariableValue, dotted: "confirm.variable.value",
			args: []Arg{S("key", variable), M("target", project)},
			want: "Replace the value of the CI/CD variable " + variable + " in project " + samplePath +
				"? The value it holds now is overwritten and no call restores it.",
		},
		{
			key: ConfirmUserAccess, dotted: "confirm.user_access.replace",
			args: []Arg{S("id", "1234"), L("changes", New(ConfirmUserAccessPassword), New(ConfirmUserAccessIdentity))},
			want: "Change how user 1234 signs in or recovers the account? This call replaces the password, which no call reads or restores" +
				" and replaces the external identity the account signs in with.",
		},
		{
			key: ConfirmUserAccessPassword, dotted: "confirm.user_access.password",
			want: "replaces the password, which no call reads or restores",
		},
		{
			key: ConfirmUserAccessEmail, dotted: "confirm.user_access.email",
			args: []Arg{S("email", address)},
			want: "sets the email address to " + address + ", whose mailbox can then reset the password",
		},
		{
			key: ConfirmUserAccessIdentity, dotted: "confirm.user_access.identity",
			want: "replaces the external identity the account signs in with",
		},
		{
			key: ConfirmHookDelivery, dotted: "confirm.hook.delivery",
			args: []Arg{
				S("hook", "7"), M("target", project),
				L("changes", New(ConfirmHookURL, S("url", origin), S("current", current)), New(ConfirmHookToken, S("token", "token"))),
			},
			want: "Change where webhook 7 in project " + samplePath + " delivers, or a secret it sends? This call points it at " +
				origin + " instead of " + current + " and replaces its token, which cannot be read back.",
		},
		{
			key: ConfirmHookURL, dotted: "confirm.hook.url",
			args: []Arg{S("url", origin), S("current", current)},
			want: "points it at " + origin + " instead of " + current,
		},
		{
			key: ConfirmHookURLSameHost, dotted: "confirm.hook.url_same_host",
			args: []Arg{S("url", origin)},
			want: "keeps it on " + origin + " and changes the path or the query it delivers to there",
		},
		{
			key: ConfirmHookURLUnread, dotted: "confirm.hook.url_unread",
			args: []Arg{S("url", origin)},
			want: "may point it at another host, " + origin + " (the webhook could not be read)",
		},
		{
			key: ConfirmHookToken, dotted: "confirm.hook.token",
			args: []Arg{S("token", "signing_token")},
			want: "replaces its signing_token, which cannot be read back",
		},
		{
			key: ConfirmHookValue, dotted: "confirm.hook.value",
			args: []Arg{S("field", field), S("key", "tenant")},
			want: "replaces the " + field + " entry tenant, whose current value cannot be read back",
		},
		{
			key: ConfirmHookValueUnread, dotted: "confirm.hook.value_unread",
			args: []Arg{S("field", field), S("key", "tenant")},
			want: "may replace the " + field + " entry tenant (the webhook could not be read)",
		},
		{
			key: ConfirmImportOverwrite, dotted: "confirm.import.overwrite",
			args: []Arg{S("path", samplePath)},
			want: "Import into " + samplePath + " and overwrite the project that is already there?" +
				" GitLab replaces the existing project with the archive, and no call brings it back.",
		},
		{
			key: ConfirmCommitForce, dotted: "confirm.commit.force",
			args: []Arg{S("branch", sampleBranch), M("target", project)},
			want: "Create this commit on branch " + sampleBranch + " in project " + samplePath + " with force?" +
				" The branch is reset to its starting point plus the new commit," +
				" and the commits it holds that the starting point does not are dropped from it.",
		},
		{
			key: ConfirmExportUpload, dotted: "confirm.export.upload",
			args: []Arg{M("target", project), S("url", origin)},
			want: "Export project " + samplePath + " and have GitLab upload the archive to " + origin + "?" +
				" That host receives the whole project: its repository, issues, merge requests and settings.",
		},
		{
			key: ConfirmStatusCheck, dotted: "confirm.status_check.delivery",
			args: []Arg{
				S("check", "3"), M("target", project),
				L("changes", New(ConfirmStatusCheckURL, S("url", origin)), New(ConfirmStatusCheckSecret)),
			},
			want: "Change where external status check 3 in project " + samplePath + " sends merge requests, or its shared secret?" +
				" This call points it at " + origin + " and replaces its shared secret, which cannot be read back.",
		},
		{
			key: ConfirmStatusCheckURL, dotted: "confirm.status_check.url",
			args: []Arg{S("url", origin)}, want: "points it at " + origin,
		},
		{
			key: ConfirmStatusCheckSecret, dotted: "confirm.status_check.secret",
			want: "replaces its shared secret, which cannot be read back",
		},
		{
			key: ConfirmQuickActions, dotted: "confirm.quick_action.run",
			args: []Arg{L("effects", New(ConfirmQuickActionMove, S("line", quoted)), New(ConfirmQuickActionMore, N("count", 3)))},
			want: "Send this text with the quick actions it carries? GitLab runs them when it saves the text, so this call" +
				" moves the item to another project or group (" + quoted + ") and runs 3 more quick actions of these kinds.",
		},
		{
			key: ConfirmQuickActionMove, dotted: "confirm.quick_action.move",
			args: []Arg{S("line", quoted)}, want: "moves the item to another project or group (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionMerge, dotted: "confirm.quick_action.merge",
			args: []Arg{S("line", quoted)}, want: "merges the merge request, at once or when its checks pass (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionPromote, dotted: "confirm.quick_action.promote",
			args: []Arg{S("line", quoted)}, want: "closes the issue and makes an epic of it (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionTicket, dotted: "confirm.quick_action.convert_to_ticket",
			args: []Arg{S("line", quoted)},
			want: "converts the issue to a Service Desk ticket whose author is an outside email address (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionAddEmail, dotted: "confirm.quick_action.add_email",
			args: []Arg{S("line", quoted)},
			want: "adds outside email addresses that receive the comments of the issue (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionPublish, dotted: "confirm.quick_action.publish",
			args: []Arg{S("line", quoted)}, want: "publishes the issue on the status page (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionUnapprove, dotted: "confirm.quick_action.unapprove",
			args: []Arg{S("line", quoted)}, want: "withdraws the caller's approval of the merge request (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionUnlink, dotted: "confirm.quick_action.unlink",
			args: []Arg{S("line", quoted)}, want: "removes the link between this item and another (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionRemoveParent, dotted: "confirm.quick_action.remove_parent",
			args: []Arg{S("line", quoted)}, want: "takes the item out of its epic or its parent (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionRemoveChild, dotted: "confirm.quick_action.remove_child",
			args: []Arg{S("line", quoted)}, want: "takes a child out of the item (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionUnassign, dotted: "confirm.quick_action.unassign",
			args: []Arg{S("line", quoted)}, want: "removes every assignee of the item (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionRemoveContacts, dotted: "confirm.quick_action.remove_contacts",
			args: []Arg{S("line", quoted)}, want: "removes CRM contacts from the item (" + quoted + ")",
		},
		{
			key: ConfirmQuickActionMore, dotted: "confirm.quick_action.more",
			args: []Arg{N("count", 3)}, want: "runs 3 more quick actions of these kinds",
		},
	}
}

// detachQuestionRows holds the questions asked before an update takes an
// item out of its epic or its parent, or removes every assignee of an issue,
// a merge request or an epic. They replaced no literal, so each row pins the
// English it was written with.
func detachQuestionRows() []frozenEnglish {
	project := New(ConfirmTargetProject, S("id", "`42`"))
	assignees := New(ConfirmWorkItemAssignees, N("count", sampleCount))
	return []frozenEnglish{
		{
			key: ConfirmIssueEpicRemove, dotted: "confirm.parent.issue_epic_remove",
			args: []Arg{N("iid", sampleIID), M("target", project)},
			want: "Take issue 42 in project `42` out of its epic? The issue is no longer a child of the epic, as when it is removed from the epic.",
		},
		{
			key: ConfirmWorkItemParentRemove, dotted: "confirm.parent.work_item_remove",
			args: []Arg{S("path", "`"+samplePath+"`"), N("iid", sampleIID)},
			want: "Take work item `group/app`#42 out of its parent? The work item is no longer a child of its parent, as when it is removed from the parent.",
		},
		{
			key: ConfirmEpicParentRemove, dotted: "confirm.parent.epic_remove",
			args: []Arg{S("path", "`group`"), N("iid", sampleIID)},
			want: "Take epic `group`&42 out of its parent epic? The epic is no longer a child of its parent, as when it is removed from the parent.",
		},
		{
			key: ConfirmIssueAssigneesClear, dotted: "confirm.assignees.issue_clear",
			args: []Arg{L("losses", assignees), N("iid", sampleIID), M("target", project)},
			want: "Remove all 3 assignees from issue 42 in project `42`? This replaces the list and cannot be undone from the response.",
		},
		{
			key: ConfirmMergeRequestAssigneesClear, dotted: "confirm.assignees.merge_request_clear",
			args: []Arg{L("losses", assignees), N("iid", sampleIID), M("target", project)},
			want: "Remove all 3 assignees from merge request 42 in project `42`? This replaces the list and cannot be undone from the response.",
		},
		{
			key: ConfirmEpicAssigneesClear, dotted: "confirm.assignees.epic_clear",
			args: []Arg{L("losses", New(ConfirmWorkItemAssigneesUnknown)), S("path", "`group`"), N("iid", sampleIID)},
			want: "Remove every assignee (the current list could not be read) from epic `group`&42? This replaces the list and cannot be undone from the response.",
		},
	}
}

// pushRuleQuestionRows holds the question of the guard that asks before an
// edit relaxes push rules, the two targets it names and each change it can
// name. They replaced no literal, so each row pins the English it was written
// with.
func pushRuleQuestionRows() []frozenEnglish {
	return []frozenEnglish{
		{
			key: ConfirmPushRuleRelax, dotted: "confirm.push_rule.relax",
			args: []Arg{
				M("target", New(ConfirmPushRuleProject, S("id", samplePath))),
				L("changes", New(ConfirmPushRuleSwitchOff, S("setting", sampleSetting)), New(ConfirmPushRuleSizeRemoved, N("from", sampleCount))),
			},
			want: fmt.Sprintf(
				"Relax the push rules of project %s? This call turns off %s and removes the max_file_size limit of %d MB."+
					" Pushes those rules reject today are accepted from then on.",
				samplePath, sampleSetting, sampleCount,
			),
		},
		{key: ConfirmPushRuleProject, dotted: "confirm.push_rule.project", args: []Arg{S("id", samplePath)}, want: "project " + samplePath},
		{key: ConfirmPushRuleGroup, dotted: "confirm.push_rule.group", args: []Arg{S("id", samplePath)}, want: "group " + samplePath},
		{
			key: ConfirmPushRuleSwitchOff, dotted: "confirm.push_rule.switch_off",
			args: []Arg{S("setting", sampleSetting)}, want: "turns off " + sampleSetting,
		},
		{
			key: ConfirmPushRulePatternRemoved, dotted: "confirm.push_rule.pattern_removed",
			args: []Arg{S("setting", sampleSetting)}, want: "removes " + sampleSetting,
		},
		{
			key: ConfirmPushRulePatternReplaced, dotted: "confirm.push_rule.pattern_replaced",
			args: []Arg{S("setting", sampleSetting)},
			want: "replaces " + sampleSetting + " with another pattern that may accept more",
		},
		{
			key: ConfirmPushRuleSizeRaised, dotted: "confirm.push_rule.size_raised",
			args: []Arg{N("from", sampleCount), N("to", sampleIID)},
			want: fmt.Sprintf("raises max_file_size from %d MB to %d MB", sampleCount, sampleIID),
		},
		{
			key: ConfirmPushRuleSizeRemoved, dotted: "confirm.push_rule.size_removed",
			args: []Arg{N("from", sampleCount)},
			want: fmt.Sprintf("removes the max_file_size limit of %d MB", sampleCount),
		},
		{
			key: ConfirmPushRuleUnread, dotted: "confirm.push_rule.unread",
			args: []Arg{S("setting", sampleSetting)},
			want: "may relax " + sampleSetting + " (the current rules could not be read)",
		},
	}
}

// pullMirrorQuestionRows holds the questions of the pull mirror guard that
// came after issue 1547's stack, for the mirror a call leaves off and for one
// whose branch filter it widens, with the names of the branches the second
// asks about. They replaced no literal, so each row pins the English it was
// written with.
func pullMirrorQuestionRows() []frozenEnglish {
	return []frozenEnglish{
		{
			key: ConfirmPullMirrorOverwriteWhileOff, dotted: "confirm.pull_mirror.overwrite_while_off",
			args: []Arg{S("project", sampleProject), S("source", sampleURL)},
			want: fmt.Sprintf(
				"Leave the pull mirror of project %s off and set it to overwrite diverged branches from %s once it is turned on?"+
					" From the first sync after that, every branch the mirror pulls that has diverged from that repository is replaced by its version,"+
					" and the replaced commits cannot be recovered through the API.",
				sampleProject, sampleURL,
			),
		},
		{
			key: ConfirmPullMirrorOverwriteWiden, dotted: "confirm.pull_mirror.overwrite_widen",
			args: []Arg{
				S("project", sampleProject), M("branches", New(ConfirmPullMirrorBranchesAll)), S("source", sampleURL),
			},
			want: fmt.Sprintf(
				"Let the pull mirror of project %s, which overwrites diverged branches, pull every branch from %s?"+
					" Every branch it pulls that has diverged from that repository, the ones it skipped until now included,"+
					" is replaced by its version on the next sync, and the replaced commits cannot be recovered through the API.",
				sampleProject, sampleURL,
			),
		},
		{key: ConfirmPullMirrorBranchesAll, dotted: "confirm.pull_mirror.branches_all", want: "every branch"},
		{key: ConfirmPullMirrorBranchesProtected, dotted: "confirm.pull_mirror.branches_protected", want: "the protected branches"},
		{
			key: ConfirmPullMirrorBranchesMatching, dotted: "confirm.pull_mirror.branches_matching",
			args: []Arg{S("regex", strconv.Quote(sampleRegex))},
			want: fmt.Sprintf("the branches whose names match %q", sampleRegex),
		},
	}
}

// TestEnglishCatalog_MatchesTheLiteralsItReplaced is the proof that issue
// 1547 changed nothing a client receives in English: every key renders, with
// the values its call site passes, exactly the text the server sent before
// the text moved into the catalog, and keeps the dotted value it was given.
func TestEnglishCatalog_MatchesTheLiteralsItReplaced(t *testing.T) {
	for _, row := range frozenEnglishRows() {
		t.Run(row.dotted, func(t *testing.T) {
			if string(row.key) != row.dotted {
				t.Errorf("the key's value is %q, want the stable %q", row.key, row.dotted)
			}
			if _, ok := english[row.key]; !ok {
				t.Fatalf("%s has no English template", row.key)
			}
			if got := New(row.key, row.args...).In(English); got != row.want {
				t.Errorf("%s renders\n%q\nwant the literal it replaced\n%q", row.key, got, row.want)
			}
		})
	}
}

// TestEnglishCatalog_EveryEntryHasAFrozenRow holds the proof above to the
// whole catalog: an English entry added without a row would be a text whose
// English nobody compared with what it replaced, and a key listed twice would
// hide that another went unlisted.
func TestEnglishCatalog_EveryEntryHasAFrozenRow(t *testing.T) {
	var listed []Key
	for _, row := range frozenEnglishRows() {
		if slices.Contains(listed, row.key) {
			t.Errorf("%s has two frozen rows", row.key)
		}
		listed = append(listed, row.key)
	}
	slices.Sort(listed)
	if want := slices.Sorted(maps.Keys(english)); !slices.Equal(listed, want) {
		t.Errorf("the frozen rows list %v, want every English entry %v", listed, want)
	}
}
