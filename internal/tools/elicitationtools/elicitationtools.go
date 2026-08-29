package elicitationtools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// labelCollectingDesc identifies the description-collection step in wrapped errors.
	labelCollectingDesc = "collecting description"
	// fmtDescSummary identifies the fmt desc summary constant used by this package.
	fmtDescSummary = "\n**Description**: %.100s..."
)

// Input types.

// ProjectInput is empty because interactive project creation elicits every field.
type ProjectInput struct{}

// IssueInput is the minimal input for interactive issue creation.
type IssueInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path where the issue will be created"`
}

// MRInput is the minimal input for interactive MR creation.
type MRInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path where the MR will be created"`
}

// ReleaseInput is the minimal input for interactive release creation.
type ReleaseInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path where the release will be created"`
}

// Confirmation helpers for destructive / create tools.

// ConfirmAction delegates to [elicitation.ConfirmAction].
func ConfirmAction(ctx context.Context, req *mcp.CallToolRequest, message string) *mcp.CallToolResult {
	return elicitation.ConfirmAction(ctx, req, message)
}

// CancelledResult delegates to [elicitation.CancelledResult].
func CancelledResult(message string) *mcp.CallToolResult {
	return elicitation.CancelledResult(message)
}

// UnsupportedResult returns a structured error tool result when the
// MCP client does not support elicitation. Suggests alternative
// non-elicitation tools.
func UnsupportedResult(toolName string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(
				"Tool %q requires the MCP elicitation capability. "+
					"Your MCP client does not support elicitation. "+
					"Check your client's MCP documentation for elicitation support.\n\n"+
					"**Alternatives**: Use the standard gitlab_issue action 'create' / "+
					"gitlab_merge_request action 'create' / etc. tools instead.",
				toolName,
			)},
		},
		IsError: true,
	}
}

// parseCSVLabels splits a comma-separated string into trimmed, non-empty labels.
func parseCSVLabels(s string) []string {
	if s == "" {
		return nil
	}
	var labels []string
	for l := range strings.SplitSeq(s, ",") {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			labels = append(labels, trimmed)
		}
	}
	return labels
}

// flowError converts an elicitation prompt error into the error a wizard
// should return: pending input becomes the flow's input-required error
// (surfaced to the client as a multi round-trip request), anything else is
// wrapped with the step label.
func flowError(fl *elicitation.Flow, err error, label string) error {
	if errors.Is(err, elicitation.ErrInputPending) {
		return fl.PendingError()
	}
	return fmt.Errorf("%s: %w", label, err)
}

// newWizardFlow builds the elicitation flow shared by the interactive
// creation wizards, translating flow construction failures and missing
// client support into wizard-level errors.
func newWizardFlow(req *mcp.CallToolRequest) (*elicitation.Flow, error) {
	fl, err := elicitation.FlowFromRequest(req)
	if err != nil {
		return nil, err
	}
	if !fl.IsSupported() {
		return nil, elicitation.ErrElicitationNotSupported
	}
	return fl, nil
}

// Interactive issue creation.

// IssueCreate guides the user through creating a GitLab issue via
// step-by-step elicitation prompts for title, description, labels, and
// confidentiality, then confirms before calling [issues.Create]. On multi
// round-trip sessions each prompt travels as an input request and the
// handler is re-invoked with the accumulated answers.
func IssueCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input IssueInput) (issues.Output, error) {
	if input.ProjectID == "" {
		return issues.Output{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	fl, err := newWizardFlow(req)
	if err != nil {
		return issues.Output{}, err
	}

	tracker.Step(ctx, 1, 4, "Collecting issue details...")

	title, err := fl.PromptText(ctx, "title", "Enter the issue title", "title")
	if err != nil {
		return issues.Output{}, flowError(fl, err, "collecting title")
	}

	description, err := fl.PromptText(ctx, "description", "Enter the issue description (Markdown supported, or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return issues.Output{}, flowError(fl, err, labelCollectingDesc)
	}

	tracker.Step(ctx, 2, 4, "Collecting optional fields...")

	labelsStr, err := fl.PromptText(ctx, "labels", "Enter comma-separated labels (or leave empty)", "labels")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return issues.Output{}, flowError(fl, err, "collecting labels")
	}
	labels := parseCSVLabels(labelsStr)

	confidentialChoice, err := confirmOptionalBool(ctx, fl, "confidential", "Should this issue be confidential?", "confidentiality")
	if err != nil {
		return issues.Output{}, err
	}
	confidential := confidentialChoice.Value

	tracker.Step(ctx, 3, 4, "Confirming issue creation...")

	summary := fmt.Sprintf("Create issue in project %s?\n\n**Title**: %s",
		toolutil.EscapeConsentValue(string(input.ProjectID)), toolutil.EscapeConsentValue(title))
	if description != "" {
		summary += fmt.Sprintf(fmtDescSummary, toolutil.EscapeConsentValue(description))
	}
	if len(labels) > 0 {
		summary += "\n**Labels**: " + toolutil.EscapeConsentValue(strings.Join(labels, ", "))
	}
	if confidential != nil && *confidential {
		summary += "\n**Confidential**: Yes"
	}

	if confirmErr := confirmCreation(ctx, fl, summary, "issue creation"); confirmErr != nil {
		return issues.Output{}, confirmErr
	}

	tracker.Step(ctx, 4, 4, "Creating issue...")

	created, err := issues.Create(ctx, client, issues.CreateInput{
		ProjectID:    input.ProjectID,
		Title:        title,
		Description:  description,
		Labels:       labels,
		Confidential: confidential,
	})
	if err != nil {
		return created, err
	}
	// Step is 1-based and Update is 0-based, so the last Step above reports 3
	// of 4. Without this the bar stops one short and stays there, which reads
	// as a wizard that hung on its final call.
	tracker.Done(ctx, 4, "Issue created")
	return created, nil
}

// Interactive MR creation.

// MRCreate guides the user through creating a GitLab merge request
// via step-by-step elicitation prompts for branches, title, description,
// labels, and merge options, then confirms before calling [mergerequests.Create].
// On multi round-trip sessions each prompt travels as an input request and
// the handler is re-invoked with the accumulated answers.
func MRCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input MRInput) (mergerequests.Output, error) {
	if input.ProjectID == "" {
		return mergerequests.Output{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	fl, err := newWizardFlow(req)
	if err != nil {
		return mergerequests.Output{}, err
	}

	tracker.Step(ctx, 1, 5, "Collecting branch information...")

	sourceBranch, err := fl.PromptText(ctx, "source_branch", "Enter the source branch name", "source_branch")
	if err != nil {
		return mergerequests.Output{}, flowError(fl, err, "collecting source branch")
	}

	targetBranch, err := fl.PromptText(ctx, "target_branch", "Enter the target branch name (e.g. main, develop)", "target_branch")
	if err != nil {
		return mergerequests.Output{}, flowError(fl, err, "collecting target branch")
	}

	tracker.Step(ctx, 2, 5, "Collecting MR details...")

	title, err := fl.PromptText(ctx, "title", "Enter the merge request title", "title")
	if err != nil {
		return mergerequests.Output{}, flowError(fl, err, "collecting title")
	}

	description, err := fl.PromptText(ctx, "description", "Enter the MR description (Markdown supported, or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return mergerequests.Output{}, flowError(fl, err, labelCollectingDesc)
	}

	tracker.Step(ctx, 3, 5, "Collecting optional fields...")

	labels, removeSource, squash, err := collectMROptions(ctx, fl)
	if err != nil {
		return mergerequests.Output{}, err
	}

	tracker.Step(ctx, 4, 5, "Confirming MR creation...")

	summary := buildMRSummary(mrSummaryParams{
		ProjectID:    input.ProjectID,
		Title:        title,
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		Description:  description,
		Labels:       labels,
		RemoveSource: removeSource,
		Squash:       squash,
	})

	if confirmErr := confirmCreation(ctx, fl, summary, "merge request creation"); confirmErr != nil {
		return mergerequests.Output{}, confirmErr
	}

	tracker.Step(ctx, 5, 5, "Creating merge request...")

	created, err := mergerequests.Create(ctx, client, mergerequests.CreateInput{
		ProjectID:          input.ProjectID,
		SourceBranch:       sourceBranch,
		TargetBranch:       targetBranch,
		Title:              title,
		Description:        description,
		Labels:             labels,
		RemoveSourceBranch: removeSource,
		Squash:             squash,
	})
	if err != nil {
		return created, err
	}
	tracker.Done(ctx, 5, "Merge request created")
	return created, nil
}

// collectMROptions asks for optional merge request labels and merge behavior.
func collectMROptions(ctx context.Context, fl *elicitation.Flow) (_ []string, _, _ *bool, _ error) {
	labelsStr, err := fl.PromptText(ctx, "labels", "Enter comma-separated labels (or leave empty)", "labels")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return nil, nil, nil, flowError(fl, err, "collecting labels")
	}
	labels := parseCSVLabels(labelsStr)

	removeSourceChoice, err := confirmOptionalBool(ctx, fl, "remove_source_branch", "Remove source branch after merge?", "source branch removal")
	if err != nil {
		return nil, nil, nil, err
	}
	removeSource := removeSourceChoice.Value

	squashChoice, err := confirmOptionalBool(ctx, fl, "squash", "Squash commits on merge?", "squash option")
	if err != nil {
		return nil, nil, nil, err
	}
	squash := squashChoice.Value

	return labels, removeSource, squash, nil
}

// optionalBoolChoice holds optional bool choice data for the elicitationtools package.
type optionalBoolChoice struct {
	Value *bool
}

// confirmOptionalBool returns an optional boolean prompt result and reports an
// error when the user cancels the flow or the answer is still pending.
func confirmOptionalBool(ctx context.Context, fl *elicitation.Flow, id, prompt, field string) (optionalBoolChoice, error) {
	confirmed, err := fl.Confirm(ctx, id, prompt)
	if err == nil {
		return optionalBoolChoice{Value: &confirmed}, nil
	}
	if errors.Is(err, elicitation.ErrDeclined) {
		return optionalBoolChoice{}, nil
	}
	return optionalBoolChoice{}, flowError(fl, err, "collecting "+field)
}

// confirmCreation runs the final confirmation prompt of a creation wizard
// and converts declines, cancellations, and pending input into the
// appropriate wizard error.
func confirmCreation(ctx context.Context, fl *elicitation.Flow, summary, operation string) error {
	confirmed, err := fl.Confirm(ctx, elicitation.ConfirmExchangeID, summary)
	if err != nil {
		if errors.Is(err, elicitation.ErrInputPending) {
			return fl.PendingError()
		}
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			return fmt.Errorf("%s canceled by user: %w", operation, err)
		}
		return fmt.Errorf("%s confirmation failed: %w", operation, err)
	}
	if !confirmed {
		return fmt.Errorf("%s canceled by user: %w", operation, elicitation.ErrCancelled)
	}
	return nil
}

// mrSummaryParams groups the parameters for building an MR confirmation summary.
type mrSummaryParams struct {
	ProjectID    toolutil.StringOrInt
	Title        string
	SourceBranch string
	TargetBranch string
	Description  string
	Labels       []string
	RemoveSource *bool
	Squash       *bool
}

// buildMRSummary returns a Markdown confirmation summary for an interactive
// merge request creation flow. It includes the project, title, source and target
// branches, and omits optional sections for empty description, labels, remove
// source branch, and squash values.
func buildMRSummary(p mrSummaryParams) string {
	summary := fmt.Sprintf("Create merge request in project %s?\n\n**Title**: %s\n**Source**: %s → **Target**: %s",
		toolutil.EscapeConsentValue(string(p.ProjectID)), toolutil.EscapeConsentValue(p.Title),
		toolutil.EscapeConsentValue(p.SourceBranch), toolutil.EscapeConsentValue(p.TargetBranch))
	if p.Description != "" {
		summary += fmt.Sprintf(fmtDescSummary, toolutil.EscapeConsentValue(p.Description))
	}
	if len(p.Labels) > 0 {
		summary += "\n**Labels**: " + toolutil.EscapeConsentValue(strings.Join(p.Labels, ", "))
	}
	if p.RemoveSource != nil && *p.RemoveSource {
		summary += "\n**Remove source branch**: Yes"
	}
	if p.Squash != nil && *p.Squash {
		summary += "\n**Squash commits**: Yes"
	}
	return summary
}

// Interactive release creation.

// ReleaseCreate guides the user through creating a GitLab release
// via step-by-step elicitation prompts for tag name, release name, and
// description, then confirms before calling [releases.Create]. On multi
// round-trip sessions each prompt travels as an input request and the
// handler is re-invoked with the accumulated answers.
func ReleaseCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input ReleaseInput) (releases.Output, error) {
	if input.ProjectID == "" {
		return releases.Output{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	fl, err := newWizardFlow(req)
	if err != nil {
		return releases.Output{}, err
	}

	tracker.Step(ctx, 1, 4, "Collecting release details...")

	tagName, err := fl.PromptText(ctx, "tag_name", "Enter the tag name for the release (must already exist)", "tag_name")
	if err != nil {
		return releases.Output{}, flowError(fl, err, "collecting tag name")
	}

	name, err := fl.PromptText(ctx, "name", "Enter the release name/title", "name")
	if err != nil {
		return releases.Output{}, flowError(fl, err, "collecting release name")
	}

	tracker.Step(ctx, 2, 4, "Collecting release description...")

	description, err := fl.PromptText(ctx, "description", "Enter the release description/notes (Markdown supported, or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return releases.Output{}, flowError(fl, err, labelCollectingDesc)
	}

	tracker.Step(ctx, 3, 4, "Confirming release creation...")

	summary := fmt.Sprintf("Create release in project %s?\n\n**Tag**: %s\n**Name**: %s",
		toolutil.EscapeConsentValue(string(input.ProjectID)), toolutil.EscapeConsentValue(tagName), toolutil.EscapeConsentValue(name))
	if description != "" {
		summary += fmt.Sprintf(fmtDescSummary, toolutil.EscapeConsentValue(description))
	}

	if confirmErr := confirmCreation(ctx, fl, summary, "release creation"); confirmErr != nil {
		return releases.Output{}, confirmErr
	}

	tracker.Step(ctx, 4, 4, "Creating release...")

	created, err := releases.Create(ctx, client, releases.CreateInput{
		ProjectID:   input.ProjectID,
		TagName:     tagName,
		Name:        name,
		Description: description,
	})
	if err != nil {
		return created, err
	}
	tracker.Done(ctx, 4, "Release created")
	return created, nil
}

// Interactive project creation.

// ProjectCreate guides the user through creating a GitLab project
// via step-by-step elicitation prompts for name, description, visibility,
// README initialization, and default branch, then confirms before calling
// [projects.Create]. On multi round-trip sessions each prompt travels as an
// input request and the handler is re-invoked with the accumulated answers.
func ProjectCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, _ ProjectInput) (projects.Output, error) {
	tracker := progress.FromRequest(req)
	fl, err := newWizardFlow(req)
	if err != nil {
		return projects.Output{}, err
	}

	tracker.Step(ctx, 1, 4, "Collecting project details...")

	name, err := fl.PromptText(ctx, "name", "Enter the project name", "name")
	if err != nil {
		return projects.Output{}, flowError(fl, err, "collecting project name")
	}

	description, err := fl.PromptText(ctx, "description", "Enter the project description (or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return projects.Output{}, flowError(fl, err, labelCollectingDesc)
	}

	tracker.Step(ctx, 2, 4, "Collecting project settings...")

	visibility, err := fl.SelectOne(ctx, "visibility", "Select the project visibility", []string{"private", "internal", "public"})
	if err != nil {
		return projects.Output{}, flowError(fl, err, "collecting visibility")
	}

	initReadmeChoice, err := confirmOptionalBool(ctx, fl, "initialize_with_readme", "Initialize the repository with a README file?", "README initialization")
	if err != nil {
		return projects.Output{}, err
	}
	initReadme := initReadmeChoice.Value != nil && *initReadmeChoice.Value

	defaultBranch, err := fl.PromptText(ctx, "default_branch", "Enter the default branch name (or leave empty for 'main')", "default_branch")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return projects.Output{}, flowError(fl, err, "collecting default branch")
	}

	tracker.Step(ctx, 3, 4, "Confirming project creation...")

	summary := fmt.Sprintf("Create new GitLab project?\n\n**Name**: %s\n**Visibility**: %s",
		toolutil.EscapeConsentValue(name), toolutil.EscapeConsentValue(visibility))
	if description != "" {
		summary += fmt.Sprintf(fmtDescSummary, toolutil.EscapeConsentValue(description))
	}
	if initReadme {
		summary += "\n**README**: Yes"
	}
	if defaultBranch != "" {
		summary += "\n**Default Branch**: " + toolutil.EscapeConsentValue(defaultBranch)
	}

	if confirmErr := confirmCreation(ctx, fl, summary, "project creation"); confirmErr != nil {
		return projects.Output{}, confirmErr
	}

	tracker.Step(ctx, 4, 4, "Creating project...")

	created, err := projects.Create(ctx, client, projects.CreateInput{
		Name:                 name,
		Description:          description,
		Visibility:           visibility,
		InitializeWithReadme: initReadme,
		DefaultBranch:        defaultBranch,
	})
	if err != nil {
		return created, err
	}
	tracker.Done(ctx, 4, "Project created")
	return created, nil
}
