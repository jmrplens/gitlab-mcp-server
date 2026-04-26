// Package jobs implements MCP tool handlers for GitLab CI/CD job operations
// including list, get, trace log retrieval, cancel, and retry.
// It wraps the Jobs service from client-go v2.
package jobs

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// maxTraceBytes limits trace output to prevent oversized responses.
const maxTraceBytes = 100 * 1024

const (
	toolJobTrace    = "jobTrace"
	fmtCodeFenceEnd = "\n```\n"
)

// ListInput defines parameters for listing jobs in a pipeline.
type ListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID     int64                `json:"pipeline_id"              jsonschema:"Pipeline ID to list jobs for,required"`
	Scope          []string             `json:"scope,omitempty"          jsonschema:"Filter by job status: created, pending, running, failed, success, canceled, skipped, waiting_for_resource, manual"`
	IncludeRetried bool                 `json:"include_retried,omitempty" jsonschema:"Include retried jobs in the response"`
	toolutil.PaginationInput
}

// Output represents a single CI/CD job.
type Output struct {
	toolutil.HintableOutput
	ID                int64    `json:"id"`
	Name              string   `json:"name"`
	Stage             string   `json:"stage"`
	Status            string   `json:"status"`
	Ref               string   `json:"ref"`
	Tag               bool     `json:"tag"`
	AllowFailure      bool     `json:"allow_failure"`
	Duration          float64  `json:"duration"`
	QueuedDuration    float64  `json:"queued_duration"`
	FailureReason     string   `json:"failure_reason,omitempty"`
	WebURL            string   `json:"web_url"`
	PipelineID        int64    `json:"pipeline_id"`
	CreatedAt         string   `json:"created_at"`
	StartedAt         string   `json:"started_at,omitempty"`
	FinishedAt        string   `json:"finished_at,omitempty"`
	ArtifactsExpireAt string   `json:"artifacts_expire_at,omitempty"`
	UserUsername      string   `json:"user_username,omitempty"`
	RunnerID          int64    `json:"runner_id,omitempty"`
	Coverage          float64  `json:"coverage,omitempty"`
	TagList           []string `json:"tag_list,omitempty"`
	ErasedAt          string   `json:"erased_at,omitempty"`
	CommitSHA         string   `json:"commit_sha,omitempty"`
}

// ListOutput holds a paginated list of jobs.
type ListOutput struct {
	toolutil.HintableOutput
	Jobs       []Output                  `json:"jobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of jobs for a pipeline.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("jobList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("jobList", "pipeline_id")
	}

	opts := &gl.ListJobsOptions{}
	if len(input.Scope) > 0 {
		scopes := make([]gl.BuildStateValue, len(input.Scope))
		for i, s := range input.Scope {
			scopes[i] = gl.BuildStateValue(s)
		}
		opts.Scope = &scopes
	}
	if input.IncludeRetried {
		opts.IncludeRetried = new(true)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	jobs, resp, err := client.GL().Jobs.ListPipelineJobs(string(input.ProjectID), input.PipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("jobList", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list and that you have Reporter+ role on the project")
	}

	out := make([]Output, len(jobs))
	for i, j := range jobs {
		out[i] = ToOutput(j)
	}
	return ListOutput{Jobs: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInput defines parameters for retrieving a single job.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to retrieve,required"`
}

// Get retrieves a single job by ID from a GitLab project.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobGet", "job_id")
	}

	j, _, err := client.GL().Jobs.GetJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("jobGet", err, http.StatusNotFound,
			"verify job_id with gitlab_job_list \u2014 job_id is the global database ID, not the per-pipeline index")
	}
	return ToOutput(j), nil
}

// TraceInput defines parameters for retrieving a job's trace log.
type TraceInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to get trace log for,required"`
}

// TraceOutput holds the raw trace (log) output of a CI/CD job.
type TraceOutput struct {
	toolutil.HintableOutput
	JobID     int64  `json:"job_id"`
	Trace     string `json:"trace"`
	Truncated bool   `json:"truncated"`
}

// Trace retrieves the raw log output of a CI/CD job, truncated at 100KB.
func Trace(ctx context.Context, client *gitlabclient.Client, input TraceInput) (TraceOutput, error) {
	if err := ctx.Err(); err != nil {
		return TraceOutput{}, err
	}
	if input.ProjectID == "" {
		return TraceOutput{}, errors.New("jobTrace: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return TraceOutput{}, toolutil.ErrRequiredInt64(toolJobTrace, "job_id")
	}

	reader, _, err := client.GL().Jobs.GetTraceFile(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		return TraceOutput{}, toolutil.WrapErrWithStatusHint(toolJobTrace, err, http.StatusNotFound,
			"verify job_id; trace logs are unavailable if the job has not started yet or its logs have been erased/expired")
	}

	buf := make([]byte, maxTraceBytes+1)
	n, err := io.ReadFull(reader, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return TraceOutput{}, toolutil.WrapErrWithMessage(toolJobTrace, err)
	}

	truncated := n > maxTraceBytes
	if truncated {
		n = maxTraceBytes
	}

	return TraceOutput{
		JobID:     input.JobID,
		Trace:     string(buf[:n]),
		Truncated: truncated,
	}, nil
}

// ActionInput defines parameters for job cancel/retry actions.
type ActionInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to act on,required"`
}

// Cancel cancels a running job.
func Cancel(ctx context.Context, client *gitlabclient.Client, input ActionInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobCancel: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobCancel", "job_id")
	}

	j, _, err := client.GL().Jobs.CancelJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobCancel", err,
				"cancelling jobs requires Developer+ role on the project; the job may also be in a non-cancellable state (already finished/canceled)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobCancel", err, http.StatusNotFound,
			"verify job_id with gitlab_job_list \u2014 only running/pending jobs can be cancelled")
	}
	return ToOutput(j), nil
}

// Retry retries a failed or canceled job.
func Retry(ctx context.Context, client *gitlabclient.Client, input ActionInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobRetry: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobRetry", "job_id")
	}

	j, _, err := client.GL().Jobs.RetryJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobRetry", err,
				"retrying jobs requires Developer+ role; only failed or canceled jobs can be retried (running/successful jobs cannot)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobRetry", err, http.StatusNotFound,
			"verify job_id with gitlab_job_list")
	}
	return ToOutput(j), nil
}

// ToOutput converts a GitLab API [gl.Job] to MCP output format.
func ToOutput(j *gl.Job) Output {
	out := Output{
		ID:             j.ID,
		Name:           j.Name,
		Stage:          j.Stage,
		Status:         j.Status,
		Ref:            j.Ref,
		Tag:            j.Tag,
		AllowFailure:   j.AllowFailure,
		Duration:       j.Duration,
		QueuedDuration: j.QueuedDuration,
		FailureReason:  j.FailureReason,
		WebURL:         j.WebURL,
		PipelineID:     j.Pipeline.ID,
		Coverage:       j.Coverage,
		TagList:        j.TagList,
	}
	if j.CreatedAt != nil {
		out.CreatedAt = j.CreatedAt.Format(time.RFC3339)
	}
	if j.StartedAt != nil {
		out.StartedAt = j.StartedAt.Format(time.RFC3339)
	}
	if j.FinishedAt != nil {
		out.FinishedAt = j.FinishedAt.Format(time.RFC3339)
	}
	if j.ArtifactsExpireAt != nil {
		out.ArtifactsExpireAt = j.ArtifactsExpireAt.Format(time.RFC3339)
	}
	if j.User != nil {
		out.UserUsername = j.User.Username
	}
	if j.Runner.ID != 0 {
		out.RunnerID = j.Runner.ID
	}
	if j.ErasedAt != nil {
		out.ErasedAt = j.ErasedAt.Format(time.RFC3339)
	}
	if j.Commit != nil {
		out.CommitSHA = j.Commit.ID
	}
	return out
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// TASK-024: additional job handlers
// ---------------------------------------------------------------------------.

// maxArtifactBytes limits artifact content returned to prevent oversized responses.
const maxArtifactBytes = 1 * 1024 * 1024

// ListProjectInput defines parameters for listing all jobs in a project.
type ListProjectInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	Scope          []string             `json:"scope,omitempty"          jsonschema:"Filter by job status: created, pending, running, failed, success, canceled, skipped, waiting_for_resource, manual"`
	IncludeRetried bool                 `json:"include_retried,omitempty" jsonschema:"Include retried jobs in the response"`
	toolutil.PaginationInput
}

// ListProject retrieves a paginated list of all jobs in a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("jobListProject: project_id is required")
	}
	opts := &gl.ListJobsOptions{}
	if len(input.Scope) > 0 {
		scopes := make([]gl.BuildStateValue, len(input.Scope))
		for i, s := range input.Scope {
			scopes[i] = gl.BuildStateValue(s)
		}
		opts.Scope = &scopes
	}
	if input.IncludeRetried {
		opts.IncludeRetried = new(true)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	jbs, resp, err := client.GL().Jobs.ListProjectJobs(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("jobListProject", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get and that you have Reporter+ role")
	}
	out := make([]Output, len(jbs))
	for i, j := range jbs {
		out[i] = ToOutput(j)
	}
	return ListOutput{Jobs: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// BridgeListInput defines parameters for listing pipeline bridge (trigger) jobs.
type BridgeListInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID int64                `json:"pipeline_id" jsonschema:"Pipeline ID to list bridge jobs for,required"`
	Scope      []string             `json:"scope,omitempty" jsonschema:"Filter by job status: created, pending, running, failed, success, canceled, skipped, manual"`
	toolutil.PaginationInput
}

// BridgeOutput represents a pipeline bridge (trigger) job.
type BridgeOutput struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	Stage              string  `json:"stage"`
	Status             string  `json:"status"`
	Ref                string  `json:"ref"`
	Tag                bool    `json:"tag"`
	AllowFailure       bool    `json:"allow_failure"`
	Duration           float64 `json:"duration"`
	QueuedDuration     float64 `json:"queued_duration"`
	FailureReason      string  `json:"failure_reason,omitempty"`
	WebURL             string  `json:"web_url"`
	Coverage           float64 `json:"coverage,omitempty"`
	UserUsername       string  `json:"user_username,omitempty"`
	CreatedAt          string  `json:"created_at"`
	StartedAt          string  `json:"started_at,omitempty"`
	FinishedAt         string  `json:"finished_at,omitempty"`
	DownstreamPipeline int64   `json:"downstream_pipeline_id,omitempty"`
}

// BridgeListOutput holds a paginated list of bridge jobs.
type BridgeListOutput struct {
	toolutil.HintableOutput
	Bridges    []BridgeOutput            `json:"bridges"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// BridgeToOutput converts a GitLab API Bridge to MCP output format.
func BridgeToOutput(b *gl.Bridge) BridgeOutput {
	out := BridgeOutput{
		ID:             b.ID,
		Name:           b.Name,
		Stage:          b.Stage,
		Status:         b.Status,
		Ref:            b.Ref,
		Tag:            b.Tag,
		AllowFailure:   b.AllowFailure,
		Duration:       b.Duration,
		QueuedDuration: b.QueuedDuration,
		FailureReason:  b.FailureReason,
		WebURL:         b.WebURL,
		Coverage:       b.Coverage,
	}
	if b.User != nil {
		out.UserUsername = b.User.Username
	}
	if b.CreatedAt != nil {
		out.CreatedAt = b.CreatedAt.Format(time.RFC3339)
	}
	if b.StartedAt != nil {
		out.StartedAt = b.StartedAt.Format(time.RFC3339)
	}
	if b.FinishedAt != nil {
		out.FinishedAt = b.FinishedAt.Format(time.RFC3339)
	}
	if b.DownstreamPipeline != nil {
		out.DownstreamPipeline = b.DownstreamPipeline.ID
	}
	return out
}

// ListBridges retrieves a paginated list of bridge (trigger) jobs for a pipeline.
func ListBridges(ctx context.Context, client *gitlabclient.Client, input BridgeListInput) (BridgeListOutput, error) {
	if err := ctx.Err(); err != nil {
		return BridgeListOutput{}, err
	}
	if input.ProjectID == "" {
		return BridgeListOutput{}, errors.New("jobListBridges: project_id is required")
	}
	if input.PipelineID <= 0 {
		return BridgeListOutput{}, toolutil.ErrRequiredInt64("jobListBridges", "pipeline_id")
	}
	opts := &gl.ListJobsOptions{}
	if len(input.Scope) > 0 {
		scopes := make([]gl.BuildStateValue, len(input.Scope))
		for i, s := range input.Scope {
			scopes[i] = gl.BuildStateValue(s)
		}
		opts.Scope = &scopes
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	bridges, resp, err := client.GL().Jobs.ListPipelineBridges(string(input.ProjectID), input.PipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		return BridgeListOutput{}, toolutil.WrapErrWithStatusHint("jobListBridges", err, http.StatusNotFound,
			"verify pipeline_id with gitlab_pipeline_list \u2014 bridges only exist for pipelines that trigger downstream/multi-project pipelines")
	}
	out := make([]BridgeOutput, len(bridges))
	for i, b := range bridges {
		out[i] = BridgeToOutput(b)
	}
	return BridgeListOutput{Bridges: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ArtifactsOutput holds artifact content (base64-encoded) and metadata.
type ArtifactsOutput struct {
	toolutil.HintableOutput
	JobID     int64  `json:"job_id,omitempty"`
	Size      int    `json:"size"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// GetArtifacts downloads the artifacts archive for a specific job.
func GetArtifacts(ctx context.Context, client *gitlabclient.Client, input GetInput) (ArtifactsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactsOutput{}, err
	}
	if input.ProjectID == "" {
		return ArtifactsOutput{}, errors.New("jobGetArtifacts: project_id is required")
	}

	if input.JobID <= 0 {
		return ArtifactsOutput{}, toolutil.ErrRequiredInt64("jobGetArtifacts", "job_id")
	}

	reader, _, err := client.GL().Jobs.GetJobArtifacts(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		return ArtifactsOutput{}, toolutil.WrapErrWithStatusHint("jobGetArtifacts", err, http.StatusNotFound,
			"verify job_id; the job may have no artifacts, or its artifacts may have expired (controlled by .gitlab-ci.yml expire_in)")
	}
	return readArtifactContent(reader, input.JobID)
}

// DownloadArtifactsInput defines parameters for downloading artifacts by ref and job name.
type DownloadArtifactsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RefName   string               `json:"ref_name"   jsonschema:"Branch or tag name"`
	JobName   string               `json:"job"        jsonschema:"Job name to download artifacts from"`
}

// DownloadArtifacts downloads the artifacts archive for a ref and job name.
func DownloadArtifacts(ctx context.Context, client *gitlabclient.Client, input DownloadArtifactsInput) (ArtifactsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactsOutput{}, err
	}
	if input.ProjectID == "" {
		return ArtifactsOutput{}, errors.New("jobDownloadArtifacts: project_id is required")
	}
	if input.RefName == "" {
		return ArtifactsOutput{}, errors.New("jobDownloadArtifacts: ref_name is required")
	}
	opts := &gl.DownloadArtifactsFileOptions{}
	if input.JobName != "" {
		opts.Job = new(input.JobName)
	}

	reader, _, err := client.GL().Jobs.DownloadArtifactsFile(string(input.ProjectID), input.RefName, opts, gl.WithContext(ctx))
	if err != nil {
		return ArtifactsOutput{}, toolutil.WrapErrWithStatusHint("jobDownloadArtifacts", err, http.StatusNotFound,
			"no successful job with the given job_name found on this ref with non-expired artifacts \u2014 verify ref_name exists and the latest successful pipeline produced artifacts")
	}
	return readArtifactContent(reader, 0)
}

// SingleArtifactInput defines parameters for downloading a single artifact file by job ID.
type SingleArtifactInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	JobID        int64                `json:"job_id"         jsonschema:"Job ID,required"`
	ArtifactPath string               `json:"artifact_path"  jsonschema:"Path to the artifact file within the archive"`
}

// SingleArtifactOutput holds the content of a single artifact file.
type SingleArtifactOutput struct {
	toolutil.HintableOutput
	JobID        int64  `json:"job_id,omitempty"`
	ArtifactPath string `json:"artifact_path"`
	Size         int    `json:"size"`
	Content      string `json:"content"`
	Truncated    bool   `json:"truncated"`
}

// DownloadSingleArtifact downloads a single artifact file from a job.
func DownloadSingleArtifact(ctx context.Context, client *gitlabclient.Client, input SingleArtifactInput) (SingleArtifactOutput, error) {
	if err := ctx.Err(); err != nil {
		return SingleArtifactOutput{}, err
	}
	if input.ProjectID == "" {
		return SingleArtifactOutput{}, errors.New("jobDownloadSingleArtifact: project_id is required")
	}
	if input.ArtifactPath == "" {
		return SingleArtifactOutput{}, errors.New("jobDownloadSingleArtifact: artifact_path is required")
	}
	if input.JobID <= 0 {
		return SingleArtifactOutput{}, toolutil.ErrRequiredInt64("jobDownloadSingleArtifact", "job_id")
	}

	reader, _, err := client.GL().Jobs.DownloadSingleArtifactsFile(string(input.ProjectID), input.JobID, input.ArtifactPath, gl.WithContext(ctx))
	if err != nil {
		return SingleArtifactOutput{}, toolutil.WrapErrWithStatusHint("jobDownloadSingleArtifact", err, http.StatusNotFound,
			"artifact_path not found within the job artifact archive, or job artifacts have expired \u2014 use gitlab_job_get_artifacts to list available paths")
	}
	return readSingleArtifactContent(reader, input.JobID, input.ArtifactPath)
}

// SingleArtifactRefInput defines parameters for downloading a single artifact file by ref.
type SingleArtifactRefInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	RefName      string               `json:"ref_name"       jsonschema:"Branch or tag name"`
	ArtifactPath string               `json:"artifact_path"  jsonschema:"Path to the artifact file within the archive"`
	JobName      string               `json:"job"            jsonschema:"Job name"`
}

// DownloadSingleArtifactByRef downloads a single artifact file by ref and job name.
func DownloadSingleArtifactByRef(ctx context.Context, client *gitlabclient.Client, input SingleArtifactRefInput) (SingleArtifactOutput, error) {
	if err := ctx.Err(); err != nil {
		return SingleArtifactOutput{}, err
	}
	if input.ProjectID == "" {
		return SingleArtifactOutput{}, errors.New("jobDownloadSingleArtifactByRef: project_id is required")
	}
	if input.RefName == "" {
		return SingleArtifactOutput{}, errors.New("jobDownloadSingleArtifactByRef: ref_name is required")
	}
	if input.ArtifactPath == "" {
		return SingleArtifactOutput{}, errors.New("jobDownloadSingleArtifactByRef: artifact_path is required")
	}

	reader, _, err := client.GL().Jobs.DownloadSingleArtifactsFileByTagOrBranch(
		string(input.ProjectID), input.RefName, input.ArtifactPath,
		&gl.DownloadArtifactsFileOptions{Job: new(input.JobName)},
		gl.WithContext(ctx),
	)
	if err != nil {
		return SingleArtifactOutput{}, toolutil.WrapErrWithStatusHint("jobDownloadSingleArtifactByRef", err, http.StatusNotFound,
			"no successful job with the given name on this ref produced an artifact at artifact_path, or artifacts expired")
	}
	return readSingleArtifactContent(reader, 0, input.ArtifactPath)
}

// Erase erases a job's trace and artifacts.
func Erase(ctx context.Context, client *gitlabclient.Client, input ActionInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobErase: project_id is required")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobErase", "job_id")
	}

	j, _, err := client.GL().Jobs.EraseJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobErase", err,
				"erasing jobs requires Maintainer+ role and the job must be in a finished state (success/failed/canceled) \u2014 erase wipes the trace log and artifacts")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobErase", err, http.StatusNotFound,
			"verify job_id with gitlab_job_list")
	}
	return ToOutput(j), nil
}

// KeepArtifacts prevents artifacts from being deleted when expiration is set.
func KeepArtifacts(ctx context.Context, client *gitlabclient.Client, input ActionInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobKeepArtifacts: project_id is required")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobKeepArtifacts", "job_id")
	}

	j, _, err := client.GL().Jobs.KeepArtifacts(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobKeepArtifacts", err,
				"keeping artifacts requires Maintainer+ role; this clears the artifact's expire_at so they are retained indefinitely")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobKeepArtifacts", err, http.StatusNotFound,
			"verify job_id; the job must have artifacts that have not yet been expired/erased")
	}
	return ToOutput(j), nil
}

// PlayInput defines parameters for running a manual job with optional variables.
type PlayInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to run,required"`
	Variables []JobVariableInput   `json:"variables,omitempty" jsonschema:"Job variables to pass"`
}

// JobVariableInput represents a variable to pass when playing a job.
type JobVariableInput struct {
	Key          string `json:"key"           jsonschema:"Variable key,required"`
	Value        string `json:"value"         jsonschema:"Variable value"`
	VariableType string `json:"variable_type,omitempty" jsonschema:"Variable type (env_var or file, default: env_var)"`
}

// Play triggers a manual job (plays it).
func Play(ctx context.Context, client *gitlabclient.Client, input PlayInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobPlay: project_id is required")
	}
	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobPlay", "job_id")
	}
	opts := &gl.PlayJobOptions{}
	if len(input.Variables) > 0 {
		vars := make([]*gl.JobVariableOptions, len(input.Variables))
		for i, v := range input.Variables {
			jv := &gl.JobVariableOptions{
				Key:   new(v.Key),
				Value: new(v.Value),
			}
			if v.VariableType != "" {
				jv.VariableType = new(gl.VariableTypeValue(v.VariableType))
			}
			vars[i] = jv
		}
		opts.JobVariablesAttributes = &vars
	}

	j, _, err := client.GL().Jobs.PlayJob(string(input.ProjectID), input.JobID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("jobPlay", err,
				"job is not in a playable state \u2014 only manual jobs (rules: when: manual) that have not yet run can be played; use gitlab_job_retry for finished jobs")
		}
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobPlay", err,
				"playing manual jobs requires Developer+ role; protected branches/environments may require Maintainer+")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobPlay", err, http.StatusNotFound,
			"verify job_id with gitlab_job_list")
	}
	return ToOutput(j), nil
}

// DeleteArtifactsInput defines parameters for deleting artifacts from a single job.
type DeleteArtifactsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to delete artifacts from,required"`
}

// DeleteArtifacts deletes the artifacts for a specific job.
func DeleteArtifacts(ctx context.Context, client *gitlabclient.Client, input DeleteArtifactsInput) error {
	if input.ProjectID == "" {
		return errors.New("jobDeleteArtifacts: project_id is required")
	}
	if input.JobID <= 0 {
		return toolutil.ErrRequiredInt64("jobDeleteArtifacts", "job_id")
	}
	_, err := client.GL().Jobs.DeleteArtifacts(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("jobDeleteArtifacts", err,
				"deleting artifacts requires Maintainer+ role; the job must be in a finished state")
		}
		return toolutil.WrapErrWithStatusHint("jobDeleteArtifacts", err, http.StatusNotFound,
			"verify job_id; the job may have no artifacts to delete")
	}
	return nil
}

// DeleteProjectArtifactsInput defines parameters for deleting all artifacts in a project.
type DeleteProjectArtifactsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DeleteProjectArtifacts deletes all artifacts in a project.
func DeleteProjectArtifacts(ctx context.Context, client *gitlabclient.Client, input DeleteProjectArtifactsInput) error {
	if input.ProjectID == "" {
		return errors.New("jobDeleteProjectArtifacts: project_id is required")
	}
	_, err := client.GL().Jobs.DeleteProjectArtifacts(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("jobDeleteProjectArtifacts", err,
				"bulk-deleting all project artifacts requires Maintainer+ role \u2014 this is irreversible across all jobs in the project")
		}
		return toolutil.WrapErrWithStatusHint("jobDeleteProjectArtifacts", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	return nil
}

// readArtifactContent reads artifact bytes from a reader with a size limit.
func readArtifactContent(reader io.Reader, jobID int64) (ArtifactsOutput, error) {
	buf := make([]byte, maxArtifactBytes+1)
	n, err := io.ReadFull(reader, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return ArtifactsOutput{}, toolutil.WrapErrWithMessage("readArtifact", err)
	}
	truncated := n > maxArtifactBytes
	if truncated {
		n = maxArtifactBytes
	}
	return ArtifactsOutput{
		JobID:     jobID,
		Size:      n,
		Content:   base64.StdEncoding.EncodeToString(buf[:n]),
		Truncated: truncated,
	}, nil
}

// readSingleArtifactContent reads a single artifact file content with a size limit.
func readSingleArtifactContent(reader io.Reader, jobID int64, path string) (SingleArtifactOutput, error) {
	buf := make([]byte, maxArtifactBytes+1)
	n, err := io.ReadFull(reader, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return SingleArtifactOutput{}, toolutil.WrapErrWithMessage("readSingleArtifact", err)
	}
	truncated := n > maxArtifactBytes
	if truncated {
		n = maxArtifactBytes
	}
	return SingleArtifactOutput{
		JobID:        jobID,
		ArtifactPath: path,
		Size:         n,
		Content:      string(buf[:n]),
		Truncated:    truncated,
	}, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters for new types
// ---------------------------------------------------------------------------.
