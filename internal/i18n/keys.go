// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

// The titles the elicitation package gives the one property of the forms it
// builds itself: a yes or no question and a choice among options.
const (
	// ElicitationSchemaConfirmTitle titles the boolean of a yes or no question.
	ElicitationSchemaConfirmTitle Key = "elicitation.schema.confirm.title"
	// ElicitationSchemaSelectionTitle titles the property of a single choice.
	ElicitationSchemaSelectionTitle Key = "elicitation.schema.selection.title"
	// ElicitationSchemaSelectionsTitle titles the property of a multiple choice.
	ElicitationSchemaSelectionsTitle Key = "elicitation.schema.selections.title"
)

// The titles of the text fields the guided creation flows ask for. Each
// English title is the property's own key, which is what a client showed
// before a field had a title of its own.
const (
	// InteractiveFieldTitle titles the field that asks for a title.
	InteractiveFieldTitle Key = "interactive.field.title"
	// InteractiveFieldDescription titles the field that asks for a description.
	InteractiveFieldDescription Key = "interactive.field.description"
	// InteractiveFieldLabels titles the field that asks for labels.
	InteractiveFieldLabels Key = "interactive.field.labels"
	// InteractiveFieldSourceBranch titles the field that asks for a merge
	// request's source branch.
	InteractiveFieldSourceBranch Key = "interactive.field.source_branch"
	// InteractiveFieldTargetBranch titles the field that asks for a merge
	// request's target branch.
	InteractiveFieldTargetBranch Key = "interactive.field.target_branch"
	// InteractiveFieldTagName titles the field that asks for a release's tag.
	InteractiveFieldTagName Key = "interactive.field.tag_name"
	// InteractiveFieldName titles the field that asks for a name.
	InteractiveFieldName Key = "interactive.field.name"
	// InteractiveFieldDefaultBranch titles the field that asks for a new
	// project's default branch.
	InteractiveFieldDefaultBranch Key = "interactive.field.default_branch"
)

// The texts more than one guided creation flow shows.
const (
	// InteractivePromptLabels asks for comma-separated labels.
	InteractivePromptLabels Key = "interactive.prompt.labels"
	// InteractiveSummaryDescription is the consent summary's description row,
	// {description} the escaped excerpt.
	InteractiveSummaryDescription Key = "interactive.summary.description"
	// InteractiveSummaryLabels is the consent summary's labels row, {labels}
	// the escaped list.
	InteractiveSummaryLabels Key = "interactive.summary.labels"
	// InteractiveProgressOptionalFields reports the step that asks for the
	// optional fields.
	InteractiveProgressOptionalFields Key = "interactive.progress.optional_fields"
)

// The texts of the guided issue creation, interactive.issue_create.
const (
	// InteractiveIssueCreateProgressDetails reports the step that asks for the
	// issue's title and description.
	InteractiveIssueCreateProgressDetails Key = "interactive.issue_create.progress.details"
	// InteractiveIssueCreatePromptTitle asks for the issue's title.
	InteractiveIssueCreatePromptTitle Key = "interactive.issue_create.prompt.title"
	// InteractiveIssueCreatePromptDescription asks for the issue's description.
	InteractiveIssueCreatePromptDescription Key = "interactive.issue_create.prompt.description"
	// InteractiveIssueCreatePromptConfidential asks whether the issue is
	// confidential.
	InteractiveIssueCreatePromptConfidential Key = "interactive.issue_create.prompt.confidential"
	// InteractiveIssueCreateProgressConfirming reports the consent step.
	InteractiveIssueCreateProgressConfirming Key = "interactive.issue_create.progress.confirming"
	// InteractiveIssueCreateSummary opens the consent summary, {project} and
	// {title} escaped.
	InteractiveIssueCreateSummary Key = "interactive.issue_create.summary"
	// InteractiveIssueCreateSummaryConfidential is the summary's row for a
	// confidential issue.
	InteractiveIssueCreateSummaryConfidential Key = "interactive.issue_create.summary.confidential"
	// InteractiveIssueCreateProgressCreating reports the creation request.
	InteractiveIssueCreateProgressCreating Key = "interactive.issue_create.progress.creating"
	// InteractiveIssueCreateProgressDone reports the issue created.
	InteractiveIssueCreateProgressDone Key = "interactive.issue_create.progress.done"
)

// The texts of the guided merge request creation, interactive.mr_create.
const (
	// InteractiveMrCreateProgressBranches reports the step that asks for the
	// branches.
	InteractiveMrCreateProgressBranches Key = "interactive.mr_create.progress.branches"
	// InteractiveMrCreatePromptSourceBranch asks for the source branch.
	InteractiveMrCreatePromptSourceBranch Key = "interactive.mr_create.prompt.source_branch"
	// InteractiveMrCreatePromptTargetBranch asks for the target branch.
	InteractiveMrCreatePromptTargetBranch Key = "interactive.mr_create.prompt.target_branch"
	// InteractiveMrCreateProgressDetails reports the step that asks for the
	// title and description.
	InteractiveMrCreateProgressDetails Key = "interactive.mr_create.progress.details"
	// InteractiveMrCreatePromptTitle asks for the merge request's title.
	InteractiveMrCreatePromptTitle Key = "interactive.mr_create.prompt.title"
	// InteractiveMrCreatePromptDescription asks for the merge request's
	// description.
	InteractiveMrCreatePromptDescription Key = "interactive.mr_create.prompt.description"
	// InteractiveMrCreatePromptRemoveSourceBranch asks whether the merge
	// removes the source branch.
	InteractiveMrCreatePromptRemoveSourceBranch Key = "interactive.mr_create.prompt.remove_source_branch"
	// InteractiveMrCreatePromptSquash asks whether the merge squashes.
	InteractiveMrCreatePromptSquash Key = "interactive.mr_create.prompt.squash"
	// InteractiveMrCreateProgressConfirming reports the consent step.
	InteractiveMrCreateProgressConfirming Key = "interactive.mr_create.progress.confirming"
	// InteractiveMrCreateSummary opens the consent summary, {project}, {title},
	// {source} and {target} escaped.
	InteractiveMrCreateSummary Key = "interactive.mr_create.summary"
	// InteractiveMrCreateSummaryRemoveSourceBranch is the summary's row for a
	// merge that removes the source branch.
	InteractiveMrCreateSummaryRemoveSourceBranch Key = "interactive.mr_create.summary.remove_source_branch"
	// InteractiveMrCreateSummarySquash is the summary's row for a merge that
	// squashes.
	InteractiveMrCreateSummarySquash Key = "interactive.mr_create.summary.squash"
	// InteractiveMrCreateProgressCreating reports the creation request.
	InteractiveMrCreateProgressCreating Key = "interactive.mr_create.progress.creating"
	// InteractiveMrCreateProgressDone reports the merge request created.
	InteractiveMrCreateProgressDone Key = "interactive.mr_create.progress.done"
)

// The texts of the guided release creation, interactive.release_create.
const (
	// InteractiveReleaseCreateProgressDetails reports the step that asks for
	// the tag and the name.
	InteractiveReleaseCreateProgressDetails Key = "interactive.release_create.progress.details"
	// InteractiveReleaseCreatePromptTagName asks for the release's tag.
	InteractiveReleaseCreatePromptTagName Key = "interactive.release_create.prompt.tag_name"
	// InteractiveReleaseCreatePromptName asks for the release's name.
	InteractiveReleaseCreatePromptName Key = "interactive.release_create.prompt.name"
	// InteractiveReleaseCreateProgressDescription reports the step that asks
	// for the description.
	InteractiveReleaseCreateProgressDescription Key = "interactive.release_create.progress.description"
	// InteractiveReleaseCreatePromptDescription asks for the release's notes.
	InteractiveReleaseCreatePromptDescription Key = "interactive.release_create.prompt.description"
	// InteractiveReleaseCreateProgressConfirming reports the consent step.
	InteractiveReleaseCreateProgressConfirming Key = "interactive.release_create.progress.confirming"
	// InteractiveReleaseCreateSummary opens the consent summary, {project},
	// {tag} and {name} escaped.
	InteractiveReleaseCreateSummary Key = "interactive.release_create.summary"
	// InteractiveReleaseCreateProgressCreating reports the creation request.
	InteractiveReleaseCreateProgressCreating Key = "interactive.release_create.progress.creating"
	// InteractiveReleaseCreateProgressDone reports the release created.
	InteractiveReleaseCreateProgressDone Key = "interactive.release_create.progress.done"
)

// The texts of the guided project creation, interactive.project_create.
const (
	// InteractiveProjectCreateProgressDetails reports the step that asks for
	// the name and description.
	InteractiveProjectCreateProgressDetails Key = "interactive.project_create.progress.details"
	// InteractiveProjectCreatePromptName asks for the project's name.
	InteractiveProjectCreatePromptName Key = "interactive.project_create.prompt.name"
	// InteractiveProjectCreatePromptDescription asks for the project's
	// description.
	InteractiveProjectCreatePromptDescription Key = "interactive.project_create.prompt.description"
	// InteractiveProjectCreateProgressSettings reports the step that asks for
	// the settings.
	InteractiveProjectCreateProgressSettings Key = "interactive.project_create.progress.settings"
	// InteractiveProjectCreatePromptVisibility asks for the visibility.
	InteractiveProjectCreatePromptVisibility Key = "interactive.project_create.prompt.visibility"
	// InteractiveProjectCreatePromptInitializeWithReadme asks whether the
	// repository starts with a README.
	InteractiveProjectCreatePromptInitializeWithReadme Key = "interactive.project_create.prompt.initialize_with_readme"
	// InteractiveProjectCreatePromptDefaultBranch asks for the default branch.
	InteractiveProjectCreatePromptDefaultBranch Key = "interactive.project_create.prompt.default_branch"
	// InteractiveProjectCreateProgressConfirming reports the consent step.
	InteractiveProjectCreateProgressConfirming Key = "interactive.project_create.progress.confirming"
	// InteractiveProjectCreateSummary opens the consent summary, {name} and
	// {visibility} escaped.
	InteractiveProjectCreateSummary Key = "interactive.project_create.summary"
	// InteractiveProjectCreateSummaryReadme is the summary's row for a
	// repository that starts with a README.
	InteractiveProjectCreateSummaryReadme Key = "interactive.project_create.summary.readme"
	// InteractiveProjectCreateSummaryDefaultBranch is the summary's default
	// branch row, {branch} escaped.
	InteractiveProjectCreateSummaryDefaultBranch Key = "interactive.project_create.summary.default_branch"
	// InteractiveProjectCreateProgressCreating reports the creation request.
	InteractiveProjectCreateProgressCreating Key = "interactive.project_create.progress.creating"
	// InteractiveProjectCreateProgressDone reports the project created.
	InteractiveProjectCreateProgressDone Key = "interactive.project_create.progress.done"
)

// The confirmations of an action that cannot be undone. Each is asked of a
// person and, when the client cannot ask, sent to the model in English as the
// refusal's opening sentence.
const (
	// ConfirmDestructiveStandalone asks before a destructive standalone tool
	// runs, {tool} the quoted tool name.
	ConfirmDestructiveStandalone Key = "confirm.destructive.standalone"
	// ConfirmDestructiveMeta asks before a destructive action of a meta tool
	// runs, {tool} the tool and {action} the action.
	ConfirmDestructiveMeta Key = "confirm.destructive.meta"
	// ConfirmDestructiveIndividual asks before a destructive individual tool
	// runs, {tool} the tool.
	ConfirmDestructiveIndividual Key = "confirm.destructive.individual"
	// ConfirmPullMirrorOverwrite asks before a pull mirror is armed to
	// overwrite diverged branches, {project} the project and {source} what it
	// pulls from.
	ConfirmPullMirrorOverwrite Key = "confirm.pull_mirror.overwrite"
	// ConfirmPullMirrorConfiguredSource names the source a pull mirror already
	// has, for a call that does not change it.
	ConfirmPullMirrorConfiguredSource Key = "confirm.pull_mirror.configured_source"
	// ConfirmWorkItemClear asks before an update empties a work item's lists,
	// {losses} what it removes, {path} and {iid} the work item.
	ConfirmWorkItemClear Key = "confirm.work_item.clear"
	// ConfirmWorkItemAssigneesUnknown names the assignees a clear removes when
	// the current list could not be read.
	ConfirmWorkItemAssigneesUnknown Key = "confirm.work_item.assignees_unknown"
	// ConfirmWorkItemAssignees names the {count} assignees a clear removes.
	ConfirmWorkItemAssignees Key = "confirm.work_item.assignees"
	// ConfirmWorkItemCrmContacts names the CRM contacts a clear removes.
	ConfirmWorkItemCrmContacts Key = "confirm.work_item.crm_contacts"
)

// CommonListAnd is the conjunction [L] joins a list with.
const CommonListAnd Key = "common.list.and"

// The progress messages of long operations.
const (
	// ProgressUploadReading reports {read} of {total} bytes read before an
	// upload.
	ProgressUploadReading Key = "progress.upload.reading"
	// ProgressUploadHandover reports the {read} bytes handed to the upload.
	ProgressUploadHandover Key = "progress.upload.handover"
	// ProgressDownloadBytes reports {written} bytes of a download of unknown
	// size.
	ProgressDownloadBytes Key = "progress.download.bytes"
	// ProgressDownloadOfTotal reports {written} of {total} bytes downloaded.
	ProgressDownloadOfTotal Key = "progress.download.of_total"
	// ProgressPipelineWaitPoll reports one read of the {pipeline} a wait
	// polls, the {attempt}th.
	ProgressPipelineWaitPoll Key = "progress.pipeline_wait.poll"
	// ProgressJobWaitPoll reports one read of the {job} a wait polls, the
	// {attempt}th.
	ProgressJobWaitPoll Key = "progress.job_wait.poll"
	// ProgressGroupTransferWait reports the wait for a group's transfer.
	ProgressGroupTransferWait Key = "progress.group_transfer.wait"
	// ProgressProjectTransferWait reports the wait for a project's transfer to
	// {namespace}.
	ProgressProjectTransferWait Key = "progress.project_transfer.wait"
	// ProgressPackagePublishFile reports file {index} of {count}, {name},
	// being published.
	ProgressPackagePublishFile Key = "progress.package_publish.file"
	// ProgressPackagePublishDone reports {published} of {count} files
	// published.
	ProgressPackagePublishDone Key = "progress.package_publish.done"
)
