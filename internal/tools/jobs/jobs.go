package jobs

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// hintVerifyJobID is the 404 hint shared by job tools.
const hintVerifyJobID = "verify job_id with gitlab_job_list"

// maxTraceBytes limits the trace log returned by [Trace] so a single
// response cannot exceed roughly 100 KB.
const maxTraceBytes = 100 * 1024

// Operation and formatter constants shared by the jobs package.
const (
	toolJobTrace    = "jobTrace"
	fmtCodeFenceEnd = "\n```\n"
)

// applyScope maps the input scope status strings onto the SDK
// ListJobsOptions.Scope ([]BuildStateValue under the url tag scope[]),
// leaving Scope nil when no statuses were requested.
func applyScope(opts *gl.ListJobsOptions, scope []string) {
	if len(scope) == 0 {
		return
	}
	scopes := make([]gl.BuildStateValue, len(scope))
	for i, s := range scope {
		scopes[i] = gl.BuildStateValue(s)
	}
	opts.Scope = &scopes
}

// applyListOpts copies offset, keyset, and ordering parameters onto the
// embedded gl.ListOptions of a ListJobsOptions, setting only the values the
// caller supplied.
func applyListOpts(opts *gl.ListJobsOptions, page toolutil.PaginationInput, keyset toolutil.KeysetPaginationInput, orderBy, sort string) {
	toolutil.ApplyListOptions(&opts.ListOptions, page, keyset)
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
}

// ListInput defines parameters for listing jobs in a pipeline.
type ListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID     int64                `json:"pipeline_id"              jsonschema:"Pipeline ID to list jobs for,required"`
	Scope          []string             `json:"scope,omitempty"          jsonschema:"Filter by job status: created, pending, running, failed, success, canceled, skipped, waiting_for_resource, manual"`
	IncludeRetried bool                 `json:"include_retried,omitempty" jsonschema:"Include retried jobs in the response"`
	OrderBy        string               `json:"order_by,omitempty"        jsonschema:"Column to order keyset-paginated results by"`
	Sort           string               `json:"sort,omitempty"            jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output represents a single CI/CD job.
type Output struct {
	toolutil.HintableOutput
	ID                int64                `json:"id"`
	Name              string               `json:"name"`
	Stage             string               `json:"stage"`
	Status            string               `json:"status"`
	Ref               string               `json:"ref"`
	Tag               bool                 `json:"tag"`
	AllowFailure      bool                 `json:"allow_failure"`
	Duration          float64              `json:"duration"`
	QueuedDuration    float64              `json:"queued_duration"`
	FailureReason     string               `json:"failure_reason,omitempty"`
	WebURL            string               `json:"web_url"`
	PipelineID        int64                `json:"pipeline_id"` // TODO(1to1): flattened convenience scalar; prune in batched minor-extras cleanup (pipeline object carries id)
	CreatedAt         string               `json:"created_at"`
	StartedAt         string               `json:"started_at,omitempty"`
	FinishedAt        string               `json:"finished_at,omitempty"`
	ArtifactsExpireAt string               `json:"artifacts_expire_at,omitempty"`
	Coverage          float64              `json:"coverage,omitempty"`
	TagList           []string             `json:"tag_list,omitempty"`
	ErasedAt          string               `json:"erased_at,omitempty"`
	Commit            *CommitObject        `json:"commit,omitempty"`
	Pipeline          *PipelineObject      `json:"pipeline,omitempty"`
	Project           *ProjectObject       `json:"project,omitempty"`
	Runner            *RunnerObject        `json:"runner,omitempty"`
	User              *UserObject          `json:"user,omitempty"`
	Artifacts         []ArtifactObject     `json:"artifacts,omitempty"`
	ArtifactsFile     *ArtifactsFileObject `json:"artifacts_file,omitempty"`
}

// ListOutput holds a paginated list of jobs.
type ListOutput struct {
	toolutil.HintableOutput
	Jobs       []Output                  `json:"jobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of CI/CD jobs for a specific pipeline
// via the GitLab Jobs API (GET /projects/:id/pipelines/:pipeline_id/jobs).
// Optional filters narrow by job status (scope) and whether to include
// retried jobs.
//
//nolint:dupl // structurally parallel to ListBridges by design (distinct SDK call, output type, and not-found hint; no shared return type without erasing the typed []Output vs []BridgeOutput shapes).
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
	applyScope(opts, input.Scope)
	if input.IncludeRetried {
		opts.IncludeRetried = new(true)
	}
	applyListOpts(opts, input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)

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

// Get retrieves a single CI/CD job by its global ID via the GitLab
// Jobs API (GET /projects/:id/jobs/:job_id).
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

// Trace retrieves the raw log output of a CI/CD job via the GitLab
// Jobs trace API (GET /projects/:id/jobs/:job_id/trace). The trace is
// truncated to [maxTraceBytes] and the Truncated flag is set when the
// log was longer than that.
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

// CancelInput defines parameters for canceling a job, with optional force cancel.
type CancelInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to cancel,required"`
	Force     bool                 `json:"force,omitempty" jsonschema:"Force cancel even if the job is already in a non-cancellable state"`
}

// Cancel cancels a running CI/CD job via the GitLab Jobs cancel API
// (POST /projects/:id/jobs/:job_id/cancel). When Force is true, the
// call uses [gl.CancelJobOptions] to cancel jobs in non-cancellable
// states (requires GitLab v17.2+).
func Cancel(ctx context.Context, client *gitlabclient.Client, input CancelInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobCancel: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobCancel", "job_id")
	}

	var j *gl.Job
	var err error
	if input.Force {
		//nolint:staticcheck // CancelJobWithOptions is the only way to pass Force until v4.0 merges it into CancelJob.
		j, _, err = client.GL().Jobs.CancelJobWithOptions(string(input.ProjectID), input.JobID, &gl.CancelJobOptions{Force: new(true)}, gl.WithContext(ctx))
	} else {
		j, _, err = client.GL().Jobs.CancelJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	}
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobCancel", err,
				"canceling jobs requires Developer+ role on the project; the job may also be in a non-cancellable state (already finished/canceled) \u2014 use force:true to override (requires GitLab v17.2+)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobCancel", err, http.StatusNotFound,
			"verify job_id with gitlab_job_list \u2014 only running/pending jobs can be cancelled; use force:true for non-cancellable states")
	}
	return ToOutput(j), nil
}

// Retry retries a failed or canceled CI/CD job via the GitLab Jobs
// retry API (POST /projects/:id/jobs/:job_id/retry). Only jobs in
// failed or canceled states can be retried; running or successful jobs
// return 403.
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
			hintVerifyJobID)
	}
	return ToOutput(j), nil
}

// ToOutput converts a GitLab API [gl.Job] into the package's [Output],
// formatting timestamps as RFC 3339 strings and surfacing the embedded
// commit, pipeline, project, runner, user, and artifact sub-objects on
// their canonical keys (1:1 audit: full nested objects).
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
		Commit:         commitObject(j.Commit),
		Pipeline:       pipelineObject(j.Pipeline),
		Project:        projectObject(j.Project),
		Runner:         runnerObject(j.Runner),
		User:           userObject(j.User),
		Artifacts:      artifactObjects(j.Artifacts),
		ArtifactsFile:  artifactsFileObject(j.ArtifactsFile),
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
	if j.ErasedAt != nil {
		out.ErasedAt = j.ErasedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// TASK-024: additional job handlers
// ---------------------------------------------------------------------------.

// maxArtifactBytes limits artifact content returned by [readArtifactContent]
// and [readSingleArtifactContent] to roughly 1 MB to keep responses bounded.
const maxArtifactBytes = 1 * 1024 * 1024

// ListProjectInput defines parameters for listing all jobs in a project.
type ListProjectInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	Scope          []string             `json:"scope,omitempty"          jsonschema:"Filter by job status: created, pending, running, failed, success, canceled, skipped, waiting_for_resource, manual"`
	IncludeRetried bool                 `json:"include_retried,omitempty" jsonschema:"Include retried jobs in the response"`
	OrderBy        string               `json:"order_by,omitempty"        jsonschema:"Column to order keyset-paginated results by"`
	Sort           string               `json:"sort,omitempty"            jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListProject retrieves a paginated list of all CI/CD jobs in a project
// across pipelines via the GitLab Jobs API
// (GET /projects/:id/jobs). Optional filters narrow by job status and
// whether to include retried jobs.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("jobListProject: project_id is required")
	}
	opts := &gl.ListJobsOptions{}
	applyScope(opts, input.Scope)
	if input.IncludeRetried {
		opts.IncludeRetried = new(true)
	}
	applyListOpts(opts, input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)

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
	ProjectID      toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID     int64                `json:"pipeline_id" jsonschema:"Pipeline ID to list bridge jobs for,required"`
	Scope          []string             `json:"scope,omitempty" jsonschema:"Filter by job status: created, pending, running, failed, success, canceled, skipped, manual"`
	IncludeRetried bool                 `json:"include_retried,omitempty" jsonschema:"Include retried bridge jobs in the response"`
	OrderBy        string               `json:"order_by,omitempty"        jsonschema:"Column to order keyset-paginated results by"`
	Sort           string               `json:"sort,omitempty"            jsonschema:"Sort order for keyset pagination: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// BridgeOutput represents a pipeline bridge (trigger) job.
type BridgeOutput struct {
	ID                 int64               `json:"id"`
	Name               string              `json:"name"`
	Stage              string              `json:"stage"`
	Status             string              `json:"status"`
	Ref                string              `json:"ref"`
	Tag                bool                `json:"tag"`
	AllowFailure       bool                `json:"allow_failure"`
	Duration           float64             `json:"duration"`
	QueuedDuration     float64             `json:"queued_duration"`
	FailureReason      string              `json:"failure_reason,omitempty"`
	WebURL             string              `json:"web_url"`
	Coverage           float64             `json:"coverage,omitempty"`
	CreatedAt          string              `json:"created_at"`
	StartedAt          string              `json:"started_at,omitempty"`
	FinishedAt         string              `json:"finished_at,omitempty"`
	ErasedAt           string              `json:"erased_at,omitempty"`
	Commit             *CommitObject       `json:"commit,omitempty"`
	Pipeline           *PipelineInfoObject `json:"pipeline,omitempty"`
	User               *UserObject         `json:"user,omitempty"`
	DownstreamPipeline *PipelineInfoObject `json:"downstream_pipeline,omitempty"`
}

// BridgeListOutput holds a paginated list of bridge jobs.
type BridgeListOutput struct {
	toolutil.HintableOutput
	Bridges    []BridgeOutput            `json:"bridges"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// BridgeToOutput converts a GitLab API [gl.Bridge] into the package's
// [BridgeOutput], formatting timestamps as RFC 3339 strings and surfacing
// the embedded commit, pipeline, user, and downstream_pipeline sub-objects
// on their canonical keys (1:1 audit: full nested objects).
func BridgeToOutput(b *gl.Bridge) BridgeOutput {
	out := BridgeOutput{
		ID:                 b.ID,
		Name:               b.Name,
		Stage:              b.Stage,
		Status:             b.Status,
		Ref:                b.Ref,
		Tag:                b.Tag,
		AllowFailure:       b.AllowFailure,
		Duration:           b.Duration,
		QueuedDuration:     b.QueuedDuration,
		FailureReason:      b.FailureReason,
		WebURL:             b.WebURL,
		Coverage:           b.Coverage,
		Commit:             commitObject(b.Commit),
		Pipeline:           pipelineInfoValueObject(b.Pipeline),
		User:               userObject(b.User),
		DownstreamPipeline: pipelineInfoObject(b.DownstreamPipeline),
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
	if b.ErasedAt != nil {
		out.ErasedAt = b.ErasedAt.Format(time.RFC3339)
	}
	return out
}

// ListBridges retrieves a paginated list of pipeline bridge (trigger)
// jobs for a pipeline via the GitLab Jobs API
// (GET /projects/:id/pipelines/:pipeline_id/bridges). Bridges only
// exist on pipelines that trigger downstream or multi-project pipelines.
//
//nolint:dupl // structurally parallel to List by design (distinct SDK call, output type, and not-found hint; no shared return type without erasing the typed []Output vs []BridgeOutput shapes).
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
	applyScope(opts, input.Scope)
	if input.IncludeRetried {
		opts.IncludeRetried = new(true)
	}
	applyListOpts(opts, input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)

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

// GetArtifacts downloads the artifacts archive for a specific CI/CD
// job via the GitLab Jobs artifacts API
// (GET /projects/:id/jobs/:job_id/artifacts). The archive is truncated
// to [maxArtifactBytes] and base64-encoded into the response.
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

// DownloadArtifacts downloads the artifacts archive for the latest
// successful job on a given ref via the GitLab Jobs artifacts API
// (GET /projects/:id/jobs/artifacts/:ref_name/download). Useful for
// retrieving the most recent build output for a branch or tag without
// knowing the underlying job ID.
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
	ArtifactPath string               `json:"artifact_path"  jsonschema:"Path to the artifact file within the archive,required"`
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

// DownloadSingleArtifact downloads a single artifact file by job ID
// and artifact path via the GitLab Jobs artifacts API
// (GET /projects/:id/jobs/:job_id/artifacts/:artifact_path). The
// response contains the raw file content (up to [maxArtifactBytes])
// and a Truncated flag if the file was larger.
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
	RefName      string               `json:"ref_name"       jsonschema:"Branch or tag name,required"`
	ArtifactPath string               `json:"artifact_path"  jsonschema:"Path to the artifact file within the archive,required"`
	JobName      string               `json:"job"            jsonschema:"Job name,required"`
}

// DownloadSingleArtifactByRef downloads a single artifact file by ref,
// job name, and artifact path via the GitLab Jobs artifacts API
// (GET /projects/:id/jobs/artifacts/:ref_name/raw/:artifact_path).
// Returns the raw file content (up to [maxArtifactBytes]).
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
	if input.JobName == "" {
		return SingleArtifactOutput{}, errors.New("jobDownloadSingleArtifactByRef: job is required")
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

// Erase erases a CI/CD job's trace log and artifacts via the GitLab
// Jobs erase API (POST /projects/:id/jobs/:job_id/erase). The job must
// be in a finished state; this operation is destructive and requires
// Maintainer+ role.
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
			hintVerifyJobID)
	}
	return ToOutput(j), nil
}

// KeepArtifacts prevents a CI/CD job's artifacts from being deleted
// when an expiration is configured. Calls the GitLab Jobs keep API
// (POST /projects/:id/jobs/:job_id/keep_artifacts), which clears the
// expire_at and retains the artifacts indefinitely. Requires Maintainer+.
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
	ProjectID              toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID                  int64                `json:"job_id"     jsonschema:"Job ID to run,required"`
	JobVariablesAttributes []JobVariableInput   `json:"job_variables_attributes,omitempty" jsonschema:"Job variables to inject into the manual job run"`
}

// JobVariableInput represents a variable to pass when playing a job. It mirrors
// gl.JobVariableOptions (key/value/variable_type).
type JobVariableInput struct {
	Key          string `json:"key"           jsonschema:"Variable key,required"`
	Value        string `json:"value"         jsonschema:"Variable value"`
	VariableType string `json:"variable_type,omitempty" jsonschema:"Variable type (env_var or file, default: env_var)"`
}

// Play triggers a manual CI/CD job via the GitLab Jobs play API
// (POST /projects/:id/jobs/:job_id/play) with optional job variables.
// Only jobs defined as "manual" in .gitlab-ci.yml and that have not
// yet run can be played; use [Retry] for jobs that already finished.
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
	if len(input.JobVariablesAttributes) > 0 {
		vars := make([]*gl.JobVariableOptions, len(input.JobVariablesAttributes))
		for i, v := range input.JobVariablesAttributes {
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
			hintVerifyJobID)
	}
	return ToOutput(j), nil
}

// DeleteArtifactsInput defines parameters for deleting artifacts from a single job.
type DeleteArtifactsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to delete artifacts from,required"`
}

// DeleteArtifacts deletes the artifacts for a specific CI/CD job via
// the GitLab Jobs artifacts API (DELETE /projects/:id/jobs/:job_id/artifacts).
// Requires Maintainer+ role and a finished job.
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

// DeleteProjectArtifacts deletes every artifact in a project via the
// GitLab Jobs artifacts API (DELETE /projects/:id/artifacts). The
// operation is irreversible and requires Maintainer+ role.
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

// readArtifactContent reads up to [maxArtifactBytes] from a job
// artifact stream and base64-encodes the bytes. Sets the Truncated
// flag when the underlying reader had more data than the limit.
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

// readSingleArtifactContent reads up to [maxArtifactBytes] from a
// single artifact file stream and returns the raw bytes (not
// base64-encoded). Sets the Truncated flag when the file was larger.
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
