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
)

// frozenEnglishRows holds every English entry against the literal it
// replaced, copied from commit 8f339f4e8, the base of issue 1547's stack.
// A row whose old text was an fmt template keeps that template and renders it
// with the sample values, except where the template was one %s at the end
// (the description row), which is written as the concatenation it renders
// to; confirm.destructive.standalone quoted the tool name with %q, which is
// what strconv.Quote writes, so the call site passes the quoted name and the
// row renders the old template with the bare one.
func frozenEnglishRows() []frozenEnglish {
	return []frozenEnglish{
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
					" Every branch of this project that has diverged from that repository is replaced by its version on the next sync,"+
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
