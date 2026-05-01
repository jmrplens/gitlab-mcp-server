package pipelines

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing pipelines in a GitLab project.
type ListInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	Scope         string               `json:"scope,omitempty"         jsonschema:"Filter by scope (running, pending, finished, branches, tags)"`
	Status        string               `json:"status,omitempty"        jsonschema:"Filter by status (created, waiting_for_resource, preparing, pending, running, success, failed, canceled, skipped, manual, scheduled)"`
	Source        string               `json:"source,omitempty"        jsonschema:"Filter by source (push, web, trigger, schedule, api, external, pipeline, chat, merge_request_event)"`
	Ref           string               `json:"ref,omitempty"           jsonschema:"Filter by branch or tag name"`
	SHA           string               `json:"sha,omitempty"           jsonschema:"Filter by commit SHA"`
	Name          string               `json:"name,omitempty"          jsonschema:"Filter by pipeline name"`
	Username      string               `json:"username,omitempty"      jsonschema:"Filter by username that triggered the pipeline"`
	YamlErrors    bool                 `json:"yaml_errors,omitempty"   jsonschema:"Return only pipelines with YAML errors"`
	OrderBy       string               `json:"order_by,omitempty"      jsonschema:"Order by field (id, status, ref, updated_at, user_id)"`
	Sort          string               `json:"sort,omitempty"          jsonschema:"Sort direction (asc, desc)"`
	CreatedAfter  string               `json:"created_after,omitempty" jsonschema:"Return pipelines created after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Return pipelines created before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter  string               `json:"updated_after,omitempty" jsonschema:"Return pipelines updated after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore string               `json:"updated_before,omitempty" jsonschema:"Return pipelines updated before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	toolutil.PaginationInput
}

// Output represents a single pipeline in the list response.
type Output struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Name      string `json:"name"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListOutput holds a paginated list of pipelines.
type ListOutput struct {
	toolutil.HintableOutput
	Pipelines  []Output                  `json:"pipelines"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// buildListOpts maps ListInput fields to the GitLab API list
// options, applying only non-zero values so that unset filters are omitted.
func buildListOpts(input ListInput) *gl.ListProjectPipelinesOptions {
	opts := &gl.ListProjectPipelinesOptions{}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Status != "" {
		opts.Status = new(gl.BuildStateValue(input.Status))
	}
	if input.Source != "" {
		opts.Source = new(input.Source)
	}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	if input.SHA != "" {
		opts.SHA = new(input.SHA)
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.YamlErrors {
		opts.YamlErrors = new(true)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	return opts
}

// List retrieves a paginated list of pipelines for a GitLab project.
// Supports filtering by scope, status, source, ref, SHA, username, and date ranges.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("pipelineList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := buildListOpts(input)
	pipelines, resp, err := client.GL().Pipelines.ListProjectPipelines(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("pipelineList", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get; pipelines require CI/CD enabled and at least one .gitlab-ci.yml run")
	}

	out := make([]Output, len(pipelines))
	for i, p := range pipelines {
		out[i] = ToOutput(p)
	}
	return ListOutput{Pipelines: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ToOutput converts a GitLab API [gl.PipelineInfo] to MCP output format.
func ToOutput(p *gl.PipelineInfo) Output {
	out := Output{
		ID:        p.ID,
		IID:       p.IID,
		ProjectID: p.ProjectID,
		Status:    p.Status,
		Source:    p.Source,
		Ref:       p.Ref,
		SHA:       p.SHA,
		Name:      p.Name,
		WebURL:    p.WebURL,
	}
	if p.CreatedAt != nil {
		out.CreatedAt = p.CreatedAt.Format(time.RFC3339)
	}
	if p.UpdatedAt != nil {
		out.UpdatedAt = p.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// GetInput defines parameters for retrieving a single pipeline.
type GetInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID int64                `json:"pipeline_id" jsonschema:"Pipeline ID to retrieve,required"`
}

// DetailOutput represents a single pipeline with full details.
type DetailOutput struct {
	toolutil.HintableOutput
	ID             int64         `json:"id"`
	IID            int64         `json:"iid"`
	ProjectID      int64         `json:"project_id"`
	Status         string        `json:"status"`
	Source         string        `json:"source"`
	Ref            string        `json:"ref"`
	SHA            string        `json:"sha"`
	BeforeSHA      string        `json:"before_sha,omitempty"`
	Name           string        `json:"name"`
	Tag            bool          `json:"tag"`
	YamlErrors     string        `json:"yaml_errors,omitempty"`
	Duration       int64         `json:"duration"`
	QueuedDuration int64         `json:"queued_duration"`
	Coverage       string        `json:"coverage,omitempty"`
	DetailedStatus *StatusOutput `json:"detailed_status,omitempty"`
	WebURL         string        `json:"web_url"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
	StartedAt      string        `json:"started_at,omitempty"`
	FinishedAt     string        `json:"finished_at,omitempty"`
	CommittedAt    string        `json:"committed_at,omitempty"`
	UserUsername   string        `json:"user_username,omitempty"`
}

// StatusOutput represents the detailed status of a pipeline.
type StatusOutput struct {
	Icon        string `json:"icon"`
	Text        string `json:"text"`
	Label       string `json:"label"`
	Group       string `json:"group"`
	Tooltip     string `json:"tooltip"`
	HasDetails  bool   `json:"has_details"`
	DetailsPath string `json:"details_path,omitempty"`
	Favicon     string `json:"favicon,omitempty"`
}

// Get retrieves a single pipeline by ID from a GitLab project.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("pipelineGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return DetailOutput{}, toolutil.ErrRequiredInt64("pipelineGet", "pipeline_id")
	}

	p, _, err := client.GL().Pipelines.GetPipeline(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		return DetailOutput{}, toolutil.WrapErrWithStatusHint("pipelineGet", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list \u2014 pipeline IDs are project-scoped")
	}
	return DetailToOutput(p), nil
}

// DetailToOutput converts a GitLab API [gl.Pipeline] to MCP output.
func DetailToOutput(p *gl.Pipeline) DetailOutput {
	out := DetailOutput{
		ID:             p.ID,
		IID:            p.IID,
		ProjectID:      p.ProjectID,
		Status:         p.Status,
		Source:         string(p.Source),
		Ref:            p.Ref,
		SHA:            p.SHA,
		BeforeSHA:      p.BeforeSHA,
		Name:           p.Name,
		Tag:            p.Tag,
		YamlErrors:     p.YamlErrors,
		Duration:       p.Duration,
		QueuedDuration: p.QueuedDuration,
		Coverage:       p.Coverage,
		WebURL:         p.WebURL,
	}
	if p.CreatedAt != nil {
		out.CreatedAt = p.CreatedAt.Format(time.RFC3339)
	}
	if p.UpdatedAt != nil {
		out.UpdatedAt = p.UpdatedAt.Format(time.RFC3339)
	}
	if p.StartedAt != nil {
		out.StartedAt = p.StartedAt.Format(time.RFC3339)
	}
	if p.FinishedAt != nil {
		out.FinishedAt = p.FinishedAt.Format(time.RFC3339)
	}
	if p.CommittedAt != nil {
		out.CommittedAt = p.CommittedAt.Format(time.RFC3339)
	}
	if p.User != nil {
		out.UserUsername = p.User.Username
	}
	if p.DetailedStatus != nil {
		out.DetailedStatus = &StatusOutput{
			Icon:        p.DetailedStatus.Icon,
			Text:        p.DetailedStatus.Text,
			Label:       p.DetailedStatus.Label,
			Group:       p.DetailedStatus.Group,
			Tooltip:     p.DetailedStatus.Tooltip,
			HasDetails:  p.DetailedStatus.HasDetails,
			DetailsPath: p.DetailedStatus.DetailsPath,
			Favicon:     p.DetailedStatus.Favicon,
		}
	}
	return out
}

// ActionInput defines parameters for pipeline cancel/retry actions.
type ActionInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID int64                `json:"pipeline_id" jsonschema:"Pipeline ID to act on,required"`
}

// Cancel cancels a running pipeline's jobs.
func Cancel(ctx context.Context, client *gitlabclient.Client, input ActionInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("pipelineCancel: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return DetailOutput{}, toolutil.ErrRequiredInt64("pipelineCancel", "pipeline_id")
	}

	p, _, err := client.GL().Pipelines.CancelPipelineBuild(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return DetailOutput{}, toolutil.WrapErrWithHint("pipelineCancel", err,
				"pipeline may have already completed, or you lack permissions. Use gitlab_pipeline_get to check current status")
		}
		return DetailOutput{}, toolutil.WrapErrWithMessage("pipelineCancel", err)
	}
	return DetailToOutput(p), nil
}

// Retry retries failed jobs in a pipeline.
func Retry(ctx context.Context, client *gitlabclient.Client, input ActionInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("pipelineRetry: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return DetailOutput{}, toolutil.ErrRequiredInt64("pipelineRetry", "pipeline_id")
	}

	p, _, err := client.GL().Pipelines.RetryPipelineBuild(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return DetailOutput{}, toolutil.WrapErrWithHint("pipelineRetry", err,
				"pipeline may still be running, or there are no failed jobs. Use gitlab_pipeline_get to check status")
		}
		return DetailOutput{}, toolutil.WrapErrWithMessage("pipelineRetry", err)
	}
	return DetailToOutput(p), nil
}

// DeleteInput defines parameters for deleting a pipeline.
type DeleteInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID int64                `json:"pipeline_id" jsonschema:"Pipeline ID to delete,required"`
}

// Delete permanently deletes a pipeline and all its jobs.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("pipelineDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return toolutil.ErrRequiredInt64("pipelineDelete", "pipeline_id")
	}

	_, err := client.GL().Pipelines.DeletePipeline(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return toolutil.WrapErrWithHint("pipelineDelete", err,
				"only project owners can delete pipelines, and the pipeline must not be running")
		}
		return toolutil.WrapErrWithMessage("pipelineDelete", err)
	}
	return nil
}

// TASK-023: GetPipelineVariables, GetPipelineTestReport, GetPipelineTestReportSummary, GetLatestPipeline, CreatePipeline, UpdatePipelineMetadata.

// VariableOutput represents a single pipeline variable.
type VariableOutput struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
}

// VariablesOutput holds a list of pipeline variables.
type VariablesOutput struct {
	toolutil.HintableOutput
	Variables []VariableOutput `json:"variables"`
}

// GetVariables retrieves the variables for a specific pipeline.
func GetVariables(ctx context.Context, client *gitlabclient.Client, input GetInput) (VariablesOutput, error) {
	if err := ctx.Err(); err != nil {
		return VariablesOutput{}, err
	}
	if input.ProjectID == "" {
		return VariablesOutput{}, errors.New("pipelineGetVariables: project_id is required")
	}
	if input.PipelineID <= 0 {
		return VariablesOutput{}, toolutil.ErrRequiredInt64("pipelineGetVariables", "pipeline_id")
	}
	vars, _, err := client.GL().Pipelines.GetPipelineVariables(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		return VariablesOutput{}, toolutil.WrapErrWithStatusHint("pipelineGetVariables", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list \u2014 reading variables requires Maintainer+ role on the project")
	}
	out := make([]VariableOutput, len(vars))
	for i, v := range vars {
		out[i] = VariableOutput{
			Key:          v.Key,
			Value:        v.Value,
			VariableType: string(v.VariableType),
		}
	}
	return VariablesOutput{Variables: out}, nil
}

// TestSuiteOutput represents a test suite within a test report.
type TestSuiteOutput struct {
	Name         string  `json:"name"`
	TotalTime    float64 `json:"total_time"`
	TotalCount   int64   `json:"total_count"`
	SuccessCount int64   `json:"success_count"`
	FailedCount  int64   `json:"failed_count"`
	SkippedCount int64   `json:"skipped_count"`
	ErrorCount   int64   `json:"error_count"`
}

// TestReportOutput represents a pipeline test report.
type TestReportOutput struct {
	toolutil.HintableOutput
	TotalTime    float64           `json:"total_time"`
	TotalCount   int64             `json:"total_count"`
	SuccessCount int64             `json:"success_count"`
	FailedCount  int64             `json:"failed_count"`
	SkippedCount int64             `json:"skipped_count"`
	ErrorCount   int64             `json:"error_count"`
	TestSuites   []TestSuiteOutput `json:"test_suites"`
}

// GetTestReport retrieves the test report for a specific pipeline.
func GetTestReport(ctx context.Context, client *gitlabclient.Client, input GetInput) (TestReportOutput, error) {
	if err := ctx.Err(); err != nil {
		return TestReportOutput{}, err
	}
	if input.ProjectID == "" {
		return TestReportOutput{}, errors.New("pipelineGetTestReport: project_id is required")
	}
	if input.PipelineID <= 0 {
		return TestReportOutput{}, toolutil.ErrRequiredInt64("pipelineGetTestReport", "pipeline_id")
	}
	report, _, err := client.GL().Pipelines.GetPipelineTestReport(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		return TestReportOutput{}, toolutil.WrapErrWithStatusHint("pipelineGetTestReport", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list \u2014 test reports require at least one job that uploaded a JUnit-format artifact")
	}
	suites := make([]TestSuiteOutput, len(report.TestSuites))
	for i, s := range report.TestSuites {
		suites[i] = TestSuiteOutput{
			Name:         s.Name,
			TotalTime:    s.TotalTime,
			TotalCount:   s.TotalCount,
			SuccessCount: s.SuccessCount,
			FailedCount:  s.FailedCount,
			SkippedCount: s.SkippedCount,
			ErrorCount:   s.ErrorCount,
		}
	}
	return TestReportOutput{
		TotalTime:    report.TotalTime,
		TotalCount:   report.TotalCount,
		SuccessCount: report.SuccessCount,
		FailedCount:  report.FailedCount,
		SkippedCount: report.SkippedCount,
		ErrorCount:   report.ErrorCount,
		TestSuites:   suites,
	}, nil
}

// TestReportSummaryOutput represents a pipeline test report summary.
type TestReportSummaryOutput struct {
	toolutil.HintableOutput
	TotalTime    float64                  `json:"total_time"`
	TotalCount   int64                    `json:"total_count"`
	SuccessCount int64                    `json:"success_count"`
	FailedCount  int64                    `json:"failed_count"`
	SkippedCount int64                    `json:"skipped_count"`
	ErrorCount   int64                    `json:"error_count"`
	TestSuites   []TestSuiteSummaryOutput `json:"test_suites"`
}

// TestSuiteSummaryOutput represents a test suite summary.
type TestSuiteSummaryOutput struct {
	Name         string  `json:"name"`
	TotalTime    float64 `json:"total_time"`
	TotalCount   int64   `json:"total_count"`
	SuccessCount int64   `json:"success_count"`
	FailedCount  int64   `json:"failed_count"`
	SkippedCount int64   `json:"skipped_count"`
	ErrorCount   int64   `json:"error_count"`
	BuildIDs     []int64 `json:"build_ids"`
}

// GetTestReportSummary retrieves the test report summary for a specific pipeline.
func GetTestReportSummary(ctx context.Context, client *gitlabclient.Client, input GetInput) (TestReportSummaryOutput, error) {
	if err := ctx.Err(); err != nil {
		return TestReportSummaryOutput{}, err
	}
	if input.ProjectID == "" {
		return TestReportSummaryOutput{}, errors.New("pipelineGetTestReportSummary: project_id is required")
	}
	if input.PipelineID <= 0 {
		return TestReportSummaryOutput{}, toolutil.ErrRequiredInt64("pipelineGetTestReportSummary", "pipeline_id")
	}
	summary, _, err := client.GL().Pipelines.GetPipelineTestReportSummary(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
	if err != nil {
		return TestReportSummaryOutput{}, toolutil.WrapErrWithStatusHint("pipelineGetTestReportSummary", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list \u2014 test report summary requires JUnit artifacts uploaded by pipeline jobs")
	}
	suites := make([]TestSuiteSummaryOutput, len(summary.TestSuites))
	for i, s := range summary.TestSuites {
		suites[i] = TestSuiteSummaryOutput{
			Name:         s.Name,
			TotalTime:    s.TotalTime,
			TotalCount:   s.TotalCount,
			SuccessCount: s.SuccessCount,
			FailedCount:  s.FailedCount,
			SkippedCount: s.SkippedCount,
			ErrorCount:   s.ErrorCount,
			BuildIDs:     s.BuildIDs,
		}
	}
	return TestReportSummaryOutput{
		TotalTime:    summary.Total.Time,
		TotalCount:   summary.Total.Count,
		SuccessCount: summary.Total.Success,
		FailedCount:  summary.Total.Failed,
		SkippedCount: summary.Total.Skipped,
		ErrorCount:   summary.Total.Error,
		TestSuites:   suites,
	}, nil
}

// GetLatestInput defines parameters for getting the latest pipeline.
type GetLatestInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ref       string               `json:"ref,omitempty" jsonschema:"Branch or tag name to filter by (defaults to the default branch)"`
}

// GetLatest retrieves the latest pipeline for a project, optionally filtered by ref.
// Falls back to listing pipelines (per_page=1, sort=desc) if the /latest endpoint
// returns 403, which happens for users without Maintainer+ role.
func GetLatest(ctx context.Context, client *gitlabclient.Client, input GetLatestInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("pipelineGetLatest: project_id is required")
	}
	opts := &gl.GetLatestPipelineOptions{}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	p, _, err := client.GL().Pipelines.GetLatestPipeline(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 403) {
			return getLatestFallback(ctx, client, input)
		}
		return DetailOutput{}, toolutil.WrapErrWithMessage("pipelineGetLatest", err)
	}
	return DetailToOutput(p), nil
}

// getLatestFallback lists pipelines sorted desc with per_page=1 to find the latest.
func getLatestFallback(ctx context.Context, client *gitlabclient.Client, input GetLatestInput) (DetailOutput, error) {
	listOpts := &gl.ListProjectPipelinesOptions{
		Sort:    new("desc"),
		OrderBy: new("id"),
	}
	listOpts.PerPage = 1
	if input.Ref != "" {
		listOpts.Ref = new(input.Ref)
	}
	pipelines, _, err := client.GL().Pipelines.ListProjectPipelines(string(input.ProjectID), listOpts, gl.WithContext(ctx))
	if err != nil {
		return DetailOutput{}, toolutil.WrapErrWithMessage("pipelineGetLatest(fallback)", err)
	}
	if len(pipelines) == 0 {
		return DetailOutput{}, errors.New("pipelineGetLatest: no pipelines found for this project")
	}
	detail, _, err := client.GL().Pipelines.GetPipeline(string(input.ProjectID), pipelines[0].ID, gl.WithContext(ctx))
	if err != nil {
		return DetailOutput{}, toolutil.WrapErrWithMessage("pipelineGetLatest(fallback)", err)
	}
	return DetailToOutput(detail), nil
}

// CreateInput defines parameters for creating a new pipeline.
type CreateInput struct {
	ProjectID toolutil.StringOrInt  `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ref       string                `json:"ref"        jsonschema:"Branch or tag name to run the pipeline for"`
	Variables []VariableOptionInput `json:"variables,omitempty" jsonschema:"Pipeline variables to set"`
}

// VariableOptionInput represents a variable to pass when creating a pipeline.
type VariableOptionInput struct {
	Key          string `json:"key"           jsonschema:"Variable key,required"`
	Value        string `json:"value"         jsonschema:"Variable value"`
	VariableType string `json:"variable_type,omitempty" jsonschema:"Variable type (env_var or file, default: env_var)"`
}

// Create creates a new pipeline for the given ref with optional variables.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("pipelineCreate: project_id is required")
	}
	if input.Ref == "" {
		return DetailOutput{}, errors.New("pipelineCreate: ref is required")
	}
	opts := &gl.CreatePipelineOptions{
		Ref: new(input.Ref),
	}
	if len(input.Variables) > 0 {
		vars := make([]*gl.PipelineVariableOptions, len(input.Variables))
		for i, v := range input.Variables {
			pv := &gl.PipelineVariableOptions{
				Key:   new(v.Key),
				Value: new(v.Value),
			}
			if v.VariableType != "" {
				pv.VariableType = new(gl.VariableTypeValue(v.VariableType))
			}
			vars[i] = pv
		}
		opts.Variables = &vars
	}
	p, _, err := client.GL().Pipelines.CreatePipeline(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, 400) {
			return DetailOutput{}, toolutil.WrapErrWithHint("pipelineCreate", err,
				"the project may not have a .gitlab-ci.yml, or the ref does not exist. Use gitlab_file_get to check if .gitlab-ci.yml exists on the target ref")
		}
		return DetailOutput{}, toolutil.WrapErrWithMessage("pipelineCreate", err)
	}
	return DetailToOutput(p), nil
}

// UpdateMetadataInput defines parameters for updating pipeline metadata.
type UpdateMetadataInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID int64                `json:"pipeline_id" jsonschema:"Pipeline ID to update,required"`
	Name       string               `json:"name"        jsonschema:"New pipeline name,required"`
}

// UpdateMetadata updates the metadata (name) of a pipeline.
func UpdateMetadata(ctx context.Context, client *gitlabclient.Client, input UpdateMetadataInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("pipelineUpdateMetadata: project_id is required")
	}
	if input.PipelineID <= 0 {
		return DetailOutput{}, toolutil.ErrRequiredInt64("pipelineUpdateMetadata", "pipeline_id")
	}
	if input.Name == "" {
		return DetailOutput{}, errors.New("pipelineUpdateMetadata: name is required")
	}
	opts := &gl.UpdatePipelineMetadataOptions{
		Name: new(input.Name),
	}
	p, _, err := client.GL().Pipelines.UpdatePipelineMetadata(string(input.ProjectID), input.PipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return DetailOutput{}, toolutil.WrapErrWithHint("pipelineUpdateMetadata", err,
				"updating pipeline metadata (name) requires Developer+ role; pipeline must not be archived")
		}
		return DetailOutput{}, toolutil.WrapErrWithStatusHint("pipelineUpdateMetadata", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list")
	}
	return DetailToOutput(p), nil
}
