// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package i18n

// english is the source catalog: every key is written here first, and every
// translation holds exactly these keys with the same placeholders.
//
// Every text here is the English the server sent before issue 1547 moved it
// into the catalog, byte for byte, which TestEnglishCatalog_MatchesTheLiteralsItReplaced
// holds it to. That includes the field titles, which are the property keys a
// client showed when a field had no title of its own.
//
// Each template is one string literal, however long: a template joined with +
// is an expression at package level, which no coverage profile reaches, so
// the mutation gate would read each + as code no test covers.
var english = map[Key]string{
	ElicitationSchemaConfirmTitle:    "Confirm",
	ElicitationSchemaSelectionTitle:  "Selection",
	ElicitationSchemaSelectionsTitle: "Selections",

	InteractiveFieldTitle:         "title",
	InteractiveFieldDescription:   "description",
	InteractiveFieldLabels:        "labels",
	InteractiveFieldSourceBranch:  "source_branch",
	InteractiveFieldTargetBranch:  "target_branch",
	InteractiveFieldTagName:       "tag_name",
	InteractiveFieldName:          "name",
	InteractiveFieldDefaultBranch: "default_branch",

	InteractivePromptLabels:           "Enter comma-separated labels (or leave empty)",
	InteractiveSummaryDescription:     "\n**Description**: {description}",
	InteractiveSummaryLabels:          "\n**Labels**: {labels}",
	InteractiveProgressOptionalFields: "Collecting optional fields...",

	InteractiveIssueCreateProgressDetails:     "Collecting issue details...",
	InteractiveIssueCreatePromptTitle:         "Enter the issue title",
	InteractiveIssueCreatePromptDescription:   "Enter the issue description (Markdown supported, or leave empty)",
	InteractiveIssueCreatePromptConfidential:  "Should this issue be confidential?",
	InteractiveIssueCreateProgressConfirming:  "Confirming issue creation...",
	InteractiveIssueCreateSummary:             "Create issue in project {project}?\n\n**Title**: {title}",
	InteractiveIssueCreateSummaryConfidential: "\n**Confidential**: Yes",
	InteractiveIssueCreateProgressCreating:    "Creating issue...",
	InteractiveIssueCreateProgressDone:        "Issue created",

	InteractiveMrCreateProgressBranches:          "Collecting branch information...",
	InteractiveMrCreatePromptSourceBranch:        "Enter the source branch name",
	InteractiveMrCreatePromptTargetBranch:        "Enter the target branch name (e.g. main, develop)",
	InteractiveMrCreateProgressDetails:           "Collecting MR details...",
	InteractiveMrCreatePromptTitle:               "Enter the merge request title",
	InteractiveMrCreatePromptDescription:         "Enter the MR description (Markdown supported, or leave empty)",
	InteractiveMrCreatePromptRemoveSourceBranch:  "Remove source branch after merge?",
	InteractiveMrCreatePromptSquash:              "Squash commits on merge?",
	InteractiveMrCreateProgressConfirming:        "Confirming MR creation...",
	InteractiveMrCreateSummary:                   "Create merge request in project {project}?\n\n**Title**: {title}\n**Source**: {source} -> **Target**: {target}",
	InteractiveMrCreateSummaryRemoveSourceBranch: "\n**Remove source branch**: Yes",
	InteractiveMrCreateSummarySquash:             "\n**Squash commits**: Yes",
	InteractiveMrCreateProgressCreating:          "Creating merge request...",
	InteractiveMrCreateProgressDone:              "Merge request created",

	InteractiveReleaseCreateProgressDetails:     "Collecting release details...",
	InteractiveReleaseCreatePromptTagName:       "Enter the tag name for the release (must already exist)",
	InteractiveReleaseCreatePromptName:          "Enter the release name/title",
	InteractiveReleaseCreateProgressDescription: "Collecting release description...",
	InteractiveReleaseCreatePromptDescription:   "Enter the release description/notes (Markdown supported, or leave empty)",
	InteractiveReleaseCreateProgressConfirming:  "Confirming release creation...",
	InteractiveReleaseCreateSummary:             "Create release in project {project}?\n\n**Tag**: {tag}\n**Name**: {name}",
	InteractiveReleaseCreateProgressCreating:    "Creating release...",
	InteractiveReleaseCreateProgressDone:        "Release created",

	InteractiveProjectCreateProgressDetails:            "Collecting project details...",
	InteractiveProjectCreatePromptName:                 "Enter the project name",
	InteractiveProjectCreatePromptDescription:          "Enter the project description (or leave empty)",
	InteractiveProjectCreateProgressSettings:           "Collecting project settings...",
	InteractiveProjectCreatePromptVisibility:           "Select the project visibility",
	InteractiveProjectCreatePromptInitializeWithReadme: "Initialize the repository with a README file?",
	InteractiveProjectCreatePromptDefaultBranch:        "Enter the default branch name (or leave empty for the instance's default branch)",
	InteractiveProjectCreateProgressConfirming:         "Confirming project creation...",
	InteractiveProjectCreateSummary:                    "Create new GitLab project?\n\n**Name**: {name}\n**Visibility**: {visibility}",
	InteractiveProjectCreateSummaryReadme:              "\n**README**: Yes",
	InteractiveProjectCreateSummaryDefaultBranch:       "\n**Default Branch**: {branch}",
	InteractiveProjectCreateProgressCreating:           "Creating project...",
	InteractiveProjectCreateProgressDone:               "Project created",

	ConfirmDestructiveStandalone:      "Confirm destructive action {tool}?",
	ConfirmDestructiveMeta:            "Confirm {tool}/{action}? This action may be irreversible.",
	ConfirmDestructiveIndividual:      "Confirm {tool}? This action may be irreversible.",
	ConfirmPullMirrorOverwrite:        "Configure project {project} to pull from {source} and overwrite diverged branches? Every branch of this project that has diverged from that repository is replaced by its version on the next sync, and the replaced commits cannot be recovered through the API.",
	ConfirmPullMirrorConfiguredSource: "its configured mirror source",
	ConfirmWorkItemClear:              "Remove {losses} from work item {path}#{iid}? This replaces the list and cannot be undone from the response.",
	ConfirmWorkItemAssigneesUnknown:   "every assignee (the current list could not be read)",
	ConfirmWorkItemAssignees:          "all {count} assignees",
	ConfirmWorkItemCrmContacts:        "every CRM contact",

	CommonListAnd: " and ",

	ProgressUploadReading:       "Read {read} / {total} bytes, preparing the upload",
	ProgressUploadHandover:      "Read {read} bytes, uploading to GitLab",
	ProgressDownloadBytes:       "Downloaded {written} bytes",
	ProgressDownloadOfTotal:     "Downloaded {written} / {total} bytes",
	ProgressPipelineWaitPoll:    "Polling pipeline #{pipeline} (attempt {attempt}, status check)...",
	ProgressJobWaitPoll:         "Polling job #{job} (attempt {attempt}, status check)...",
	ProgressGroupTransferWait:   "Waiting for GitLab to move the group",
	ProgressProjectTransferWait: "Waiting for GitLab to move the project to {namespace}",
	ProgressPackagePublishFile:  "Publishing file {index} of {count}: {name}",
	ProgressPackagePublishDone:  "Published {published} of {count} files",
}
