// Package cilint implements MCP tool handlers for GitLab CI/CD configuration
// linting. It supports validating a project's existing .gitlab-ci.yml and
// arbitrary YAML content via the CI Lint API.
package cilint

import (
	"context"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Input / Output types
// ---------------------------------------------------------------------------.

// ProjectInput holds parameters for linting a project's CI/CD configuration.
type ProjectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ContentRef  string               `json:"content_ref" jsonschema:"Branch or tag to use for the CI configuration content"`
	DryRun      *bool                `json:"dry_run" jsonschema:"Run pipeline creation simulation"`
	DryRunRef   string               `json:"dry_run_ref" jsonschema:"Branch or tag to use as context for the dry run"`
	IncludeJobs *bool                `json:"include_jobs" jsonschema:"Include expanded job list in the response"`
	Ref         string               `json:"ref" jsonschema:"Branch or tag to use for CI includes resolution"`
}

// ContentInput holds parameters for linting arbitrary CI/CD YAML within a project namespace.
type ContentInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path (namespace context),required"`
	Content     string               `json:"content" jsonschema:"CI/CD YAML content to validate,required"`
	DryRun      *bool                `json:"dry_run" jsonschema:"Run pipeline creation simulation"`
	IncludeJobs *bool                `json:"include_jobs" jsonschema:"Include expanded job list in the response"`
	Ref         string               `json:"ref" jsonschema:"Branch or tag to use for CI includes resolution"`
}

// Include represents an include block found in the CI configuration.
type Include struct {
	Type           string `json:"type"`
	Location       string `json:"location"`
	ContextProject string `json:"context_project,omitempty"`
}

// Output represents the result of a CI lint operation.
type Output struct {
	toolutil.HintableOutput
	Valid      bool      `json:"valid"`
	Errors     []string  `json:"errors,omitempty"`
	Warnings   []string  `json:"warnings,omitempty"`
	MergedYaml string    `json:"merged_yaml,omitempty"`
	Includes   []Include `json:"includes,omitempty"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(r *gitlab.ProjectLintResult) Output {
	includes := make([]Include, 0, len(r.Includes))
	for _, inc := range r.Includes {
		includes = append(includes, Include{
			Type:           inc.Type,
			Location:       inc.Location,
			ContextProject: inc.ContextProject,
		})
	}
	return Output{
		Valid:      r.Valid,
		Errors:     r.Errors,
		Warnings:   r.Warnings,
		MergedYaml: r.MergedYaml,
		Includes:   includes,
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// LintProject validates project for the cilint package.
func LintProject(ctx context.Context, client *gitlabclient.Client, input ProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("lint project CI config", err)
	}

	opts := &gitlab.ProjectLintOptions{}
	if input.ContentRef != "" {
		opts.ContentRef = &input.ContentRef
	}
	if input.DryRun != nil {
		opts.DryRun = input.DryRun
	}
	if input.DryRunRef != "" {
		opts.DryRunRef = &input.DryRunRef
	}
	if input.IncludeJobs != nil {
		opts.IncludeJobs = input.IncludeJobs
	}
	if input.Ref != "" {
		opts.Ref = &input.Ref
	}

	result, _, err := client.GL().Validate.ProjectLint(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("lint project CI config", err)
	}
	return toOutput(result), nil
}

// LintContent validates content for the cilint package.
func LintContent(ctx context.Context, client *gitlabclient.Client, input ContentInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if strings.TrimSpace(input.Content) == "" {
		return Output{}, toolutil.ErrFieldRequired("content")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("lint CI content", err)
	}

	opts := &gitlab.ProjectNamespaceLintOptions{
		Content: &input.Content,
	}
	if input.DryRun != nil {
		opts.DryRun = input.DryRun
	}
	if input.IncludeJobs != nil {
		opts.IncludeJobs = input.IncludeJobs
	}
	if input.Ref != "" {
		opts.Ref = &input.Ref
	}

	result, _, err := client.GL().Validate.ProjectNamespaceLint(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("lint CI content", err)
	}
	return toOutput(result), nil
}
