package cilint

import (
	"context"
	"net/http"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Operation names, each used by its handler's validation, request and
// capture errors alike.
const (
	opLintProject = "lint project CI config"
	opLintContent = "lint CI content"
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

// Output represents the result of a CI lint operation: what
// [gitlab.ProjectLintResult] decodes, plus the jobs array GitLab sends when
// include_jobs is set and the SDK does not carry, read from the captured
// response (ADR-0021).
type Output struct {
	toolutil.HintableOutput
	Valid      bool                     `json:"valid"`
	Errors     []string                 `json:"errors,omitempty"`
	Warnings   []string                 `json:"warnings,omitempty"`
	MergedYaml string                   `json:"merged_yaml,omitempty"`
	Includes   []Include                `json:"includes,omitempty"`
	Jobs       []toolutil.LintJobOutput `json:"jobs,omitempty"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format, and
// takes the jobs the capture read beside the SDK.
func toOutput(r *gitlab.ProjectLintResult, extra toolutil.LintExtra) Output {
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
		Jobs:       extra.Jobs,
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
		return Output{}, toolutil.WrapErrWithMessage(opLintProject, err)
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	result, _, err := client.GL().Validate.ProjectLint(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(opLintProject, err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; project must have a .gitlab-ci.yml at the specified ref; ref/content_ref must be a valid branch or tag")
	}
	extra, err := toolutil.CapturedLint(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr(opLintProject, err)
	}
	return toOutput(result, extra), nil
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
		return Output{}, toolutil.WrapErrWithMessage(opLintContent, err)
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	result, _, err := client.GL().Validate.ProjectNamespaceLint(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(opLintContent, err, http.StatusBadRequest,
			"content must be valid YAML; verify project_id provides namespace context for resolving includes; ref must be a valid branch or tag")
	}
	extra, err := toolutil.CapturedLint(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr(opLintContent, err)
	}
	return toOutput(result, extra), nil
}
