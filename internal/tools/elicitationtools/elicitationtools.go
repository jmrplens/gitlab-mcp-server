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
	// fmtCollectingDesc identifies the fmt collecting desc constant used by this package.
	fmtCollectingDesc = "collecting description: %w"
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

// Interactive issue creation.

// IssueCreate guides the user through creating a GitLab issue via
// step-by-step elicitation prompts for title, description, labels, and
// confidentiality, then confirms before calling [issues.Create].
func IssueCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input IssueInput) (issues.Output, error) {
	if input.ProjectID == "" {
		return issues.Output{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	ec := elicitation.FromRequest(req)

	if !ec.IsSupported() {
		return issues.Output{}, elicitation.ErrElicitationNotSupported
	}

	tracker.Step(ctx, 1, 4, "Collecting issue details...")

	title, err := ec.PromptText(ctx, "Enter the issue title", "title")
	if err != nil {
		return issues.Output{}, fmt.Errorf("collecting title: %w", err)
	}

	description, err := ec.PromptText(ctx, "Enter the issue description (Markdown supported, or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return issues.Output{}, fmt.Errorf(fmtCollectingDesc, err)
	}

	tracker.Step(ctx, 2, 4, "Collecting optional fields...")

	labelsStr, err := ec.PromptText(ctx, "Enter comma-separated labels (or leave empty)", "labels")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return issues.Output{}, fmt.Errorf("collecting labels: %w", err)
	}
	labels := parseCSVLabels(labelsStr)

	confidentialChoice, err := confirmOptionalBool(ctx, ec, "Should this issue be confidential?", "confidentiality")
	if err != nil {
		return issues.Output{}, err
	}
	confidential := confidentialChoice.Value

	tracker.Step(ctx, 3, 4, "Confirming issue creation...")

	summary := fmt.Sprintf("Create issue in project %s?\n\n**Title**: %s", input.ProjectID, title)
	if description != "" {
		summary += fmt.Sprintf(fmtDescSummary, description)
	}
	if len(labels) > 0 {
		summary += "\n**Labels**: " + strings.Join(labels, ", ")
	}
	if confidential != nil && *confidential {
		summary += "\n**Confidential**: Yes"
	}

	confirmed, err := ec.Confirm(ctx, summary)
	if err != nil {
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			return issues.Output{}, fmt.Errorf("issue creation canceled by user: %w", err)
		}
		return issues.Output{}, fmt.Errorf("issue creation confirmation failed: %w", err)
	}
	if !confirmed {
		return issues.Output{}, fmt.Errorf("issue creation canceled by user: %w", elicitation.ErrCancelled)
	}

	tracker.Step(ctx, 4, 4, "Creating issue...")

	return issues.Create(ctx, client, issues.CreateInput{
		ProjectID:    input.ProjectID,
		Title:        title,
		Description:  description,
		Labels:       strings.Join(labels, ","),
		Confidential: confidential,
	})
}

// Interactive MR creation.

// MRCreate guides the user through creating a GitLab merge request
// via step-by-step elicitation prompts for branches, title, description,
// labels, and merge options, then confirms before calling [mergerequests.Create].
func MRCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input MRInput) (mergerequests.Output, error) {
	if input.ProjectID == "" {
		return mergerequests.Output{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	ec := elicitation.FromRequest(req)

	if !ec.IsSupported() {
		return mergerequests.Output{}, elicitation.ErrElicitationNotSupported
	}

	tracker.Step(ctx, 1, 5, "Collecting branch information...")

	sourceBranch, err := ec.PromptText(ctx, "Enter the source branch name", "source_branch")
	if err != nil {
		return mergerequests.Output{}, fmt.Errorf("collecting source branch: %w", err)
	}

	targetBranch, err := ec.PromptText(ctx, "Enter the target branch name (e.g. main, develop)", "target_branch")
	if err != nil {
		return mergerequests.Output{}, fmt.Errorf("collecting target branch: %w", err)
	}

	tracker.Step(ctx, 2, 5, "Collecting MR details...")

	title, err := ec.PromptText(ctx, "Enter the merge request title", "title")
	if err != nil {
		return mergerequests.Output{}, fmt.Errorf("collecting title: %w", err)
	}

	description, err := ec.PromptText(ctx, "Enter the MR description (Markdown supported, or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return mergerequests.Output{}, fmt.Errorf(fmtCollectingDesc, err)
	}

	tracker.Step(ctx, 3, 5, "Collecting optional fields...")

	labels, removeSource, squash, err := collectMROptions(ctx, ec)
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

	confirmed, err := ec.Confirm(ctx, summary)
	if err != nil {
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			return mergerequests.Output{}, fmt.Errorf("merge request creation canceled by user: %w", err)
		}
		return mergerequests.Output{}, fmt.Errorf("merge request creation confirmation failed: %w", err)
	}
	if !confirmed {
		return mergerequests.Output{}, fmt.Errorf("merge request creation canceled by user: %w", elicitation.ErrCancelled)
	}

	tracker.Step(ctx, 5, 5, "Creating merge request...")

	return mergerequests.Create(ctx, client, mergerequests.CreateInput{
		ProjectID:          input.ProjectID,
		SourceBranch:       sourceBranch,
		TargetBranch:       targetBranch,
		Title:              title,
		Description:        description,
		Labels:             strings.Join(labels, ","),
		RemoveSourceBranch: removeSource,
		Squash:             squash,
	})
}

// collectMROptions asks for optional merge request labels and merge behavior.
func collectMROptions(ctx context.Context, ec elicitation.Client) (_ []string, _, _ *bool, _ error) {
	labelsStr, err := ec.PromptText(ctx, "Enter comma-separated labels (or leave empty)", "labels")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return nil, nil, nil, fmt.Errorf("collecting labels: %w", err)
	}
	labels := parseCSVLabels(labelsStr)

	removeSourceChoice, err := confirmOptionalBool(ctx, ec, "Remove source branch after merge?", "source branch removal")
	if err != nil {
		return nil, nil, nil, err
	}
	removeSource := removeSourceChoice.Value

	squashChoice, err := confirmOptionalBool(ctx, ec, "Squash commits on merge?", "squash option")
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
// error when the user cancels the flow.
func confirmOptionalBool(ctx context.Context, ec elicitation.Client, prompt, field string) (optionalBoolChoice, error) {
	confirmed, err := ec.Confirm(ctx, prompt)
	if err == nil {
		return optionalBoolChoice{Value: &confirmed}, nil
	}
	if errors.Is(err, elicitation.ErrDeclined) {
		return optionalBoolChoice{}, nil
	}
	return optionalBoolChoice{}, fmt.Errorf("collecting %s: %w", field, err)
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
		p.ProjectID, p.Title, p.SourceBranch, p.TargetBranch)
	if p.Description != "" {
		summary += fmt.Sprintf(fmtDescSummary, p.Description)
	}
	if len(p.Labels) > 0 {
		summary += "\n**Labels**: " + strings.Join(p.Labels, ", ")
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
// description, then confirms before calling [releases.Create].
func ReleaseCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input ReleaseInput) (releases.Output, error) {
	if input.ProjectID == "" {
		return releases.Output{}, toolutil.ErrFieldRequired("project_id")
	}

	tracker := progress.FromRequest(req)
	ec := elicitation.FromRequest(req)

	if !ec.IsSupported() {
		return releases.Output{}, elicitation.ErrElicitationNotSupported
	}

	tracker.Step(ctx, 1, 4, "Collecting release details...")

	tagName, err := ec.PromptText(ctx, "Enter the tag name for the release (must already exist)", "tag_name")
	if err != nil {
		return releases.Output{}, fmt.Errorf("collecting tag name: %w", err)
	}

	name, err := ec.PromptText(ctx, "Enter the release name/title", "name")
	if err != nil {
		return releases.Output{}, fmt.Errorf("collecting release name: %w", err)
	}

	tracker.Step(ctx, 2, 4, "Collecting release description...")

	description, err := ec.PromptText(ctx, "Enter the release description/notes (Markdown supported, or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return releases.Output{}, fmt.Errorf(fmtCollectingDesc, err)
	}

	tracker.Step(ctx, 3, 4, "Confirming release creation...")

	summary := fmt.Sprintf("Create release in project %s?\n\n**Tag**: %s\n**Name**: %s",
		input.ProjectID, tagName, name)
	if description != "" {
		summary += fmt.Sprintf(fmtDescSummary, description)
	}

	confirmed, err := ec.Confirm(ctx, summary)
	if err != nil {
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			return releases.Output{}, fmt.Errorf("release creation canceled by user: %w", err)
		}
		return releases.Output{}, fmt.Errorf("release creation confirmation failed: %w", err)
	}
	if !confirmed {
		return releases.Output{}, fmt.Errorf("release creation canceled by user: %w", elicitation.ErrCancelled)
	}

	tracker.Step(ctx, 4, 4, "Creating release...")

	return releases.Create(ctx, client, releases.CreateInput{
		ProjectID:   input.ProjectID,
		TagName:     tagName,
		Name:        name,
		Description: description,
	})
}

// Interactive project creation.

// ProjectCreate guides the user through creating a GitLab project
// via step-by-step elicitation prompts for name, description, visibility,
// README initialization, and default branch, then confirms before calling
// [projects.Create].
func ProjectCreate(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, _ ProjectInput) (projects.Output, error) {
	tracker := progress.FromRequest(req)
	ec := elicitation.FromRequest(req)

	if !ec.IsSupported() {
		return projects.Output{}, elicitation.ErrElicitationNotSupported
	}

	tracker.Step(ctx, 1, 4, "Collecting project details...")

	name, err := ec.PromptText(ctx, "Enter the project name", "name")
	if err != nil {
		return projects.Output{}, fmt.Errorf("collecting project name: %w", err)
	}

	description, err := ec.PromptText(ctx, "Enter the project description (or leave empty)", "description")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return projects.Output{}, fmt.Errorf(fmtCollectingDesc, err)
	}

	tracker.Step(ctx, 2, 4, "Collecting project settings...")

	visibility, err := ec.SelectOne(ctx, "Select the project visibility", []string{"private", "internal", "public"})
	if err != nil {
		return projects.Output{}, fmt.Errorf("collecting visibility: %w", err)
	}

	initReadmeChoice, err := confirmOptionalBool(ctx, ec, "Initialize the repository with a README file?", "README initialization")
	if err != nil {
		return projects.Output{}, err
	}
	initReadme := initReadmeChoice.Value != nil && *initReadmeChoice.Value

	defaultBranch, err := ec.PromptText(ctx, "Enter the default branch name (or leave empty for 'main')", "default_branch")
	if err != nil && !errors.Is(err, elicitation.ErrDeclined) {
		return projects.Output{}, fmt.Errorf("collecting default branch: %w", err)
	}

	tracker.Step(ctx, 3, 4, "Confirming project creation...")

	summary := fmt.Sprintf("Create new GitLab project?\n\n**Name**: %s\n**Visibility**: %s", name, visibility)
	if description != "" {
		summary += fmt.Sprintf(fmtDescSummary, description)
	}
	if initReadme {
		summary += "\n**README**: Yes"
	}
	if defaultBranch != "" {
		summary += "\n**Default Branch**: " + defaultBranch
	}

	confirmed, err := ec.Confirm(ctx, summary)
	if err != nil {
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			return projects.Output{}, fmt.Errorf("project creation canceled by user: %w", err)
		}
		return projects.Output{}, fmt.Errorf("project creation confirmation failed: %w", err)
	}
	if !confirmed {
		return projects.Output{}, fmt.Errorf("project creation canceled by user: %w", elicitation.ErrCancelled)
	}

	tracker.Step(ctx, 4, 4, "Creating project...")

	return projects.Create(ctx, client, projects.CreateInput{
		Name:                 name,
		Description:          description,
		Visibility:           visibility,
		InitializeWithReadme: initReadme,
		DefaultBranch:        defaultBranch,
	})
}
