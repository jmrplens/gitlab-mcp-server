package jobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintVerifyJobID is the 404 hint shared by job tools.
const hintVerifyJobID = "verify job_id with job.list"

// maxTraceBytes limits the trace log returned by [Trace] so a single
// response cannot exceed roughly 100 KB: a longer log keeps its end.
const maxTraceBytes = 100 << 10

// traceWindowBytes is how much of the end of a log [Trace] reads and strips
// of the runner's markup, 1000 KB, ten times the [maxTraceBytes] it returns:
// markup is a fraction of a colored line, so the last 1000 KB of any log a
// runner writes hold more than 100 KB of text.
const traceWindowBytes = 1000 << 10

// maxTraceTailLines bounds the tail_lines a caller may ask [Trace] for. A
// line count is the caller's to choose, and nothing else bounds the walk
// back through the log that counting them is.
const maxTraceTailLines = 10000

// Operation and formatter constants shared by the jobs package.
const (
	toolJobTrace = "jobTrace"
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

// rawListJobs issues a raw REST GET against a jobs list path, decoding the full
// documented response (including the SDK-missing archived/source/runner_manager
// and runner extras) into a slice of [jobAPI]. The supplied opts encode scope,
// include_retried, and pagination via their url struct tags, and the returned
// gl.Response preserves the pagination headers for [toolutil.PaginationFromResponse].
// The request options are the caller's, in the order they run: its
// gl.WithContext, then, on a list GitLab pages by keyset, its
// [toolutil.KeysetContinuation].
func rawListJobs(client *gitlabclient.Client, path string, opts *gl.ListJobsOptions, options ...gl.RequestOptionFunc) ([]*jobAPI, *gl.Response, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, options)
	if err != nil {
		return nil, nil, err
	}
	var jobs []*jobAPI
	resp, err := client.GL().Do(req, &jobs)
	return jobs, resp, err
}

// rawGetJob issues a raw REST GET against a single-job path, decoding the full
// documented response into a [jobAPI].
func rawGetJob(ctx context.Context, client *gitlabclient.Client, path string) (*jobAPI, *gl.Response, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var job jobAPI
	resp, err := client.GL().Do(req, &job)
	return &job, resp, err
}

// rawJobsOutput maps a slice of raw-fetch jobs and the response pagination into
// a [ListOutput].
func rawJobsOutput(jobs []*jobAPI, resp *gl.Response) ListOutput {
	out := make([]Output, len(jobs))
	for i, j := range jobs {
		out[i] = toOutputAPI(j)
	}
	return ListOutput{Jobs: out, Pagination: toolutil.PaginationFromResponse(resp)}
}

// ListInput defines parameters for listing jobs in a pipeline.
type ListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID     int64                `json:"pipeline_id"              jsonschema:"Pipeline ID to list jobs for,required"`
	Scope          []string             `json:"scope,omitempty"          jsonschema:"Filter by job status: created, pending, preparing, waiting_for_resource, waiting_for_callback, running, success, failed, canceled, canceling, skipped, manual, scheduled"`
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
	Duration          *float64             `json:"duration,omitempty"`
	QueuedDuration    *float64             `json:"queued_duration,omitempty"`
	FailureReason     string               `json:"failure_reason,omitempty"`
	WebURL            string               `json:"web_url"`
	CreatedAt         string               `json:"created_at"`
	StartedAt         string               `json:"started_at,omitempty"`
	FinishedAt        string               `json:"finished_at,omitempty"`
	ArtifactsExpireAt string               `json:"artifacts_expire_at,omitempty"`
	Coverage          *float64             `json:"coverage,omitempty"`
	TagList           []string             `json:"tag_list,omitempty"`
	ErasedAt          string               `json:"erased_at,omitempty"`
	Archived          bool                 `json:"archived,omitempty"`
	Commit            *CommitObject        `json:"commit,omitempty"`
	Pipeline          *PipelineObject      `json:"pipeline,omitempty"`
	Project           *ProjectObject       `json:"project,omitempty"`
	Runner            *RunnerObject        `json:"runner,omitempty"`
	RunnerManager     *RunnerManagerObject `json:"runner_manager,omitempty"`
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
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("jobList: project_id is required. Use project.list to find the ID first, then pass it as project_id")
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

	// Raw REST fetch so the documented archived/source/runner_manager and runner
	// extras (absent from gl.Job) are surfaced 1:1 with the Jobs API.
	path := fmt.Sprintf("projects/%s/pipelines/%d/jobs", gl.PathEscape(string(input.ProjectID)), input.PipelineID)
	jobs, resp, err := rawListJobs(client, path, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("jobList", err, http.StatusNotFound,
			"verify pipeline_id with pipeline.list and that you have Reporter+ role on the project")
	}

	return rawJobsOutput(jobs, resp), nil
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
		return Output{}, errors.New("jobGet: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobGet", "job_id")
	}

	// Raw REST fetch so the documented archived/source/runner_manager and runner
	// extras (absent from gl.Job) are surfaced 1:1 with the Jobs API. A single
	// unmarshal is naturally tolerant of older instances that omit these fields.
	path := fmt.Sprintf("projects/%s/jobs/%d", gl.PathEscape(string(input.ProjectID)), input.JobID)
	j, _, err := rawGetJob(ctx, client, path)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("jobGet", err, http.StatusNotFound,
			"verify job_id with job.list. job_id is the global database ID, not the per-pipeline index")
	}
	return toOutputAPI(j), nil
}

// TraceInput defines parameters for retrieving a job's trace log.
type TraceInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID to get trace log for,required"`
	TailLines int                  `json:"tail_lines,omitempty" jsonschema:"Return only the last N lines of the log, 1 to 10000. Either way at most the last 100 KB of the log is returned"`
}

// TraceOutput holds the trace (log) of a CI/CD job, the end of it when it
// is longer than one response carries. Truncated is set when anything was
// left out, and OmittedBytes counts the bytes of the log, as GitLab stores
// it with the runner's markup, that come before the part returned.
type TraceOutput struct {
	toolutil.HintableOutput
	JobID        int64  `json:"job_id"`
	Trace        string `json:"trace"`
	Truncated    bool   `json:"truncated"`
	OmittedBytes int    `json:"omitted_bytes,omitempty"`
}

// Trace retrieves the log of a CI/CD job via the GitLab Jobs trace API
// (GET /projects/:id/jobs/:job_id/trace), without the runner's terminal
// markup ([toolutil.StripLogMarkup]), the last tail_lines of it when the
// caller asked for them, and its last [maxTraceBytes] at most
// ([logTail]), reading only the end of the log ([traceTail]). It used to
// return the first [maxTraceBytes], and a job's failure is almost always at
// the end of its log.
func Trace(ctx context.Context, client *gitlabclient.Client, input TraceInput) (TraceOutput, error) {
	if err := ctx.Err(); err != nil {
		return TraceOutput{}, err
	}
	if input.ProjectID == "" {
		return TraceOutput{}, errors.New("jobTrace: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return TraceOutput{}, toolutil.ErrRequiredInt64(toolJobTrace, "job_id")
	}
	if input.TailLines < 0 || input.TailLines > maxTraceTailLines {
		return TraceOutput{}, fmt.Errorf("%s: tail_lines must be between 1 and %d, or left out for the whole log", toolJobTrace, maxTraceTailLines)
	}

	reader, _, err := client.GL().Jobs.GetTraceFile(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsDownloadTooLarge(err) {
			return TraceOutput{}, toolutil.WrapErrWithHint(toolJobTrace, err,
				"the log is larger than this server reads from GitLab in one response; read it from GitLab directly (GET /projects/:id/jobs/:job_id/trace)")
		}
		return TraceOutput{}, toolutil.WrapErrWithStatusHint(toolJobTrace, err, http.StatusNotFound,
			"verify job_id; trace logs are unavailable if the job has not started yet or its logs have been erased/expired")
	}

	log, omitted := traceTail(reader, input.TailLines, traceWindowBytes)
	return TraceOutput{
		JobID:        input.JobID,
		Trace:        log,
		Truncated:    omitted > 0,
		OmittedBytes: omitted,
	}, nil
}

// traceTail returns the end of the log r holds as [Trace] returns it, and
// how many bytes of the log, as GitLab stores it, come before that end, or
// 0 when nothing was left out.
//
// It reads only the last window bytes of the log and strips the markup of
// those alone, so a call costs a few windows whatever the log's size, up to
// the client's response ceiling; it used to copy the whole log and strip all
// of it to keep 100 KB. The part read starts at the first line that begins
// inside it, where the markup strips as it would in the whole log, since no
// sequence or marker spans a line break, or at a character when it holds no
// line break before its last byte. The end of the log is then exactly what
// stripping the whole log gives whenever the part read holds more text than
// [maxTraceBytes]; a part that holds less, all markup but a little text, is
// returned whole and what comes before it is counted as left out.
//
// client-go hands the body back already read, so its size is known and the
// read cannot fail.
func traceTail(r *bytes.Reader, tailLines, window int) (log string, omitted int) {
	size := int(r.Size())
	start := max(size-window, 0)
	buf := make([]byte, size-start)
	_, _ = r.ReadAt(buf, int64(start)) // a bytes.Reader holding size bytes fills buf whole
	skip := 0
	if start > 0 {
		skip = bytes.IndexByte(buf[:len(buf)-1], '\n') + 1
		for skip < len(buf)-1 && !utf8.RuneStart(buf[skip]) {
			skip++
		}
	}
	raw := string(buf[skip:])
	text := toolutil.StripLogMarkup(raw)
	log = text
	if tailLines > 0 {
		log = lastLines(log, tailLines)
	}
	log = logTail(log, maxTraceBytes)
	if start == 0 && len(log) == len(text) {
		return log, 0
	}
	return log, start + skip + rawOffset(raw, text, len(text)-len(log))
}

// rawOffset returns where, in raw, the log as written, the part of text, its
// stripped form, from n on begins: past as many line breaks as text[:n]
// holds, since stripping keeps every one of them, and then, when n falls
// inside a line, as far into that line as [toolutil.LogMarkupRawOffset]
// maps it. raw from there strips to text from n. Counting line breaks reads
// the part once and allocates nothing, where mapping the whole part through
// the markup would cost another pass of the regular expressions over it.
func rawOffset(raw, text string, n int) int {
	lineStart := strings.LastIndexByte(text[:n], '\n') + 1
	offset := 0
	for range strings.Count(text[:lineStart], "\n") {
		offset += strings.IndexByte(raw[offset:], '\n') + 1
	}
	if lineStart == n {
		return offset
	}
	line, _, _ := strings.Cut(raw[offset:], "\n")
	return offset + toolutil.LogMarkupRawOffset(line, n-lineStart)
}

// lastLines returns the last n lines of s, keeping a final line break, and
// s whole when it has no more than n lines.
func lastLines(s string, n int) string {
	cut := len(strings.TrimSuffix(s, "\n"))
	for range n {
		cut = strings.LastIndexByte(s[:cut], '\n')
		if cut < 0 {
			return s
		}
	}
	return s[cut+1:]
}

// logTail returns the last limit bytes of s at most. The kept part starts at
// a line, the first one that begins inside the last limit bytes, which is the
// first of them when the byte before them ends a line, so no line is shown
// cut at its start; a log with no line break there is cut at a character
// instead, so what is kept of valid UTF-8 stays valid. The last byte is
// always kept, whatever bytes come before it.
func logTail(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	start := len(s) - limit
	if s[start-1] != '\n' {
		if i := strings.IndexByte(s[start:len(s)-1], '\n'); i >= 0 {
			start += i + 1
		}
	}
	for start < len(s)-1 && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
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
// request carries force=true, which cancels a job in a non-cancellable
// state (requires GitLab v17.2+).
//
// It always calls the options variant, with a non-nil [gl.CancelJobOptions].
// That variant is the only binding that carries force: client-go marks it
// deprecated only because it plans to fold the option into CancelJob in v4.
// CancelJob itself delegates to it with a nil options pointer, which reaches
// the body as the four bytes `null` recorded in entry 51 of
// docs/development/upstream-bugs.md, so a call without force sends the empty
// object `{}` here instead.
func Cancel(ctx context.Context, client *gitlabclient.Client, input CancelInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("jobCancel: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobCancel", "job_id")
	}

	opts := &gl.CancelJobOptions{}
	if input.Force {
		opts.Force = new(true)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	j, _, err := client.GL().Jobs.CancelJobWithOptions(string(input.ProjectID), input.JobID, opts, gl.WithContext(ctx)) //nolint:staticcheck // SA1019: the only binding that carries force until client-go v4 folds it into CancelJob, and a non-nil value keeps a call without force from sending null (entry 51 of docs/development/upstream-bugs.md).
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobCancel", err,
				"canceling jobs requires Developer+ role on the project; the job may also be in a non-cancellable state (already finished/canceled). Use force:true to override (requires GitLab v17.2+)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobCancel", err, http.StatusNotFound,
			"verify job_id with job.list. Only running/pending jobs can be cancelled; use force:true for non-cancellable states")
	}
	return capturedJob(j, captured)
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
		return Output{}, errors.New("jobRetry: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}

	if input.JobID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("jobRetry", "job_id")
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	j, _, err := client.GL().Jobs.RetryJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobRetry", err,
				"retrying jobs requires Developer+ role; only failed or canceled jobs can be retried (running/successful jobs cannot)")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobRetry", err, http.StatusNotFound,
			hintVerifyJobID)
	}
	return capturedJob(j, captured)
}

// ToOutput converts a GitLab API [gl.Job] into the package's [Output],
// formatting timestamps as RFC 3339 strings and surfacing the embedded
// commit, pipeline, project, runner, user, and artifact sub-objects on
// their canonical keys (nested objects trimmed to the documented Jobs API
// field set per doc/api/jobs.md).
//
// The duration, queued duration and coverage come from measures, what GitLab
// sent for them, because client-go decodes all three as float64 and reads
// the null of a job that never started as 0.
func ToOutput(j *gl.Job, measures toolutil.JobMeasures) Output {
	out := Output{
		ID:             j.ID,
		Name:           j.Name,
		Stage:          j.Stage,
		Status:         j.Status,
		Ref:            j.Ref,
		Tag:            j.Tag,
		AllowFailure:   j.AllowFailure,
		Duration:       measures.Duration,
		QueuedDuration: measures.QueuedDuration,
		FailureReason:  j.FailureReason,
		WebURL:         j.WebURL,
		Coverage:       measures.Coverage,
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

// maxArtifactBytes is the largest artifact returned in one response, 1 MiB:
// an archive or report larger than it comes back with its size and no
// content ([readArtifactContent]), and a single text file with its first
// MiB ([readSingleArtifactContent]).
const maxArtifactBytes = 1 << 20

// ListProjectInput defines parameters for listing all jobs in a project.
type ListProjectInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	Scope          []string             `json:"scope,omitempty"          jsonschema:"Filter by job status: created, pending, preparing, waiting_for_resource, waiting_for_callback, running, success, failed, canceled, canceling, skipped, manual, scheduled"`
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

	// Raw REST fetch so the documented archived/source/runner_manager and runner
	// extras (absent from gl.Job) are surfaced 1:1 with the Jobs API.
	path := fmt.Sprintf("projects/%s/jobs", gl.PathEscape(string(input.ProjectID)))
	jbs, resp, err := rawListJobs(client, path, opts, gl.WithContext(ctx), toolutil.KeysetContinuation(input.KeysetPaginationInput))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("jobListProject", err, http.StatusNotFound,
			"verify the project exists with project.get and that you have Reporter+ role")
	}
	return rawJobsOutput(jbs, resp), nil
}

// BridgeListInput defines parameters for listing pipeline bridge (trigger) jobs.
type BridgeListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID     int64                `json:"pipeline_id" jsonschema:"Pipeline ID to list bridge jobs for,required"`
	Scope          []string             `json:"scope,omitempty" jsonschema:"Filter by job status: created, pending, preparing, waiting_for_resource, waiting_for_callback, running, success, failed, canceled, canceling, skipped, manual, scheduled"`
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
	Duration           *float64            `json:"duration,omitempty"`
	QueuedDuration     *float64            `json:"queued_duration,omitempty"`
	FailureReason      string              `json:"failure_reason,omitempty"`
	WebURL             string              `json:"web_url"`
	Coverage           *float64            `json:"coverage,omitempty"`
	CreatedAt          string              `json:"created_at"`
	StartedAt          string              `json:"started_at,omitempty"`
	FinishedAt         string              `json:"finished_at,omitempty"`
	ErasedAt           string              `json:"erased_at,omitempty"`
	Commit             *CommitObject       `json:"commit,omitempty"`
	Pipeline           *PipelineInfoObject `json:"pipeline,omitempty"`
	User               *UserObject         `json:"user,omitempty"`
	DownstreamPipeline *PipelineInfoObject `json:"downstream_pipeline,omitempty"`
	// Project carries the one key GitLab renders under it on a job: whether
	// the job token may reach outside this project.
	Project *ProjectObject `json:"project,omitempty"`
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
// on their canonical keys (nested objects trimmed to the documented Jobs API
// field set per doc/api/jobs.md).
func BridgeToOutput(b *gl.Bridge, extra toolutil.BridgeExtra) BridgeOutput {
	out := BridgeOutput{
		ID:                 b.ID,
		Name:               b.Name,
		Stage:              b.Stage,
		Status:             b.Status,
		Ref:                b.Ref,
		Tag:                b.Tag,
		AllowFailure:       b.AllowFailure,
		Duration:           extra.Duration,
		QueuedDuration:     extra.QueuedDuration,
		FailureReason:      b.FailureReason,
		WebURL:             b.WebURL,
		Coverage:           extra.Coverage,
		Commit:             commitObject(b.Commit),
		Pipeline:           pipelineInfoValueObject(b.Pipeline),
		User:               userObject(b.User),
		DownstreamPipeline: pipelineInfoObject(b.DownstreamPipeline),
		Project:            bridgeProjectObject(extra.Project),
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	bridges, resp, err := client.GL().Jobs.ListPipelineBridges(string(input.ProjectID), input.PipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		return BridgeListOutput{}, toolutil.WrapErrWithStatusHint("jobListBridges", err, http.StatusNotFound,
			"verify pipeline_id with pipeline.list. Bridges only exist for pipelines that trigger downstream/multi-project pipelines")
	}
	extras, err := toolutil.CapturedBridges(captured, len(bridges))
	if err != nil {
		return BridgeListOutput{}, toolutil.WrapErr("jobListBridges", err)
	}
	out := make([]BridgeOutput, len(bridges))
	for i, b := range bridges {
		out[i] = BridgeToOutput(b, extras[i])
	}
	return BridgeListOutput{Bridges: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ArtifactsOutput holds an artifact base64-encoded and its size in bytes.
// Truncated is set when the artifact was larger than one response carries,
// and then Content is empty: a prefix of an archive cannot be opened.
type ArtifactsOutput struct {
	toolutil.HintableOutput
	JobID     int64  `json:"job_id,omitempty"`
	Size      int    `json:"size"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// ArtifactsInput defines parameters for downloading one job's artifacts.
type ArtifactsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID     int64                `json:"job_id"     jsonschema:"Job ID whose artifacts to download,required"`
	FileType  string               `json:"file_type,omitempty" jsonschema:"Which of the job's artifacts to download: archive (the default, the zip of the job's artifacts:paths) or a report type such as junit, cobertura, sast or dotenv. GitLab reads it from 19.4, and an older instance ignores it and answers with the archive"`
}

// GetArtifacts downloads one artifact of a specific CI/CD job via the GitLab
// Jobs artifacts API (GET /projects/:id/jobs/:job_id/artifacts): the archive
// by default, or the report file_type names, base64-encoded into the
// response when it fits [maxArtifactBytes] ([readArtifactContent]).
//
// It calls the options variant because the other binding sends no file_type.
// client-go marks that variant deprecated only because it plans to fold the
// options into GetJobArtifacts in v4, and says to use it meanwhile whenever
// the options are needed. A GET encodes the options as a query string, where
// an unset file_type is left out, so a call naming none asks for exactly what
// it asked for before.
func GetArtifacts(ctx context.Context, client *gitlabclient.Client, input ArtifactsInput) (ArtifactsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactsOutput{}, err
	}
	if input.ProjectID == "" {
		return ArtifactsOutput{}, errors.New("jobGetArtifacts: project_id is required")
	}

	if input.JobID <= 0 {
		return ArtifactsOutput{}, toolutil.ErrRequiredInt64("jobGetArtifacts", "job_id")
	}

	opts := &gl.GetJobArtifactsOptions{}
	if input.FileType != "" {
		opts.FileType = new(gl.ArtifactFileTypeValue(input.FileType))
	}
	reader, _, err := client.GL().Jobs.GetJobArtifactsWithOptions(string(input.ProjectID), input.JobID, opts, gl.WithContext(ctx)) //nolint:staticcheck // SA1019: the only binding that carries file_type until client-go v4 folds it into GetJobArtifacts.
	if err != nil {
		if toolutil.IsDownloadTooLarge(err) {
			return ArtifactsOutput{}, toolutil.WrapErrWithHint("jobGetArtifacts", err, errArtifactTooLarge)
		}
		return ArtifactsOutput{}, toolutil.WrapErrWithStatusHint("jobGetArtifacts", err, http.StatusNotFound,
			"verify job_id and file_type; the job may have no artifact of that type, or its artifacts may have expired (controlled by .gitlab-ci.yml expire_in)")
	}
	return readArtifactContent(reader, input.JobID), nil
}

// DownloadArtifactsInput defines parameters for downloading artifacts by ref and job name.
type DownloadArtifactsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RefName   string               `json:"ref_name"   jsonschema:"Branch or tag name,required"`
	JobName   string               `json:"job"        jsonschema:"Job name to download artifacts from,required"`
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
	// GitLab requires the job on this route: the ref alone names a pipeline,
	// and the job is what picks whose artifacts come back.
	if input.JobName == "" {
		return ArtifactsOutput{}, errors.New("jobDownloadArtifacts: job is required")
	}
	opts := &gl.DownloadArtifactsFileOptions{Job: new(input.JobName)}

	reader, _, err := client.GL().Jobs.DownloadArtifactsFile(string(input.ProjectID), input.RefName, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsDownloadTooLarge(err) {
			return ArtifactsOutput{}, toolutil.WrapErrWithHint("jobDownloadArtifacts", err, errRefArtifactTooLarge)
		}
		return ArtifactsOutput{}, toolutil.WrapErrWithStatusHint("jobDownloadArtifacts", err, http.StatusNotFound,
			"no successful job with the given job_name found on this ref with non-expired artifacts. Verify ref_name exists and the latest successful pipeline produced artifacts")
	}
	return readArtifactContent(reader, 0), nil
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
	Encoding     string `json:"encoding,omitempty"`
	Content      string `json:"content"`
	Truncated    bool   `json:"truncated"`
}

// DownloadSingleArtifact downloads a single artifact file by job ID
// and artifact path via the GitLab Jobs artifacts API
// (GET /projects/:id/jobs/:job_id/artifacts/:artifact_path), as text or
// base64 by what the file holds ([readSingleArtifactContent]).
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
		if toolutil.IsDownloadTooLarge(err) {
			return SingleArtifactOutput{}, toolutil.WrapErrWithHint("jobDownloadSingleArtifact", err, errSingleArtifactTooLarge)
		}
		return SingleArtifactOutput{}, toolutil.WrapErrWithStatusHint("jobDownloadSingleArtifact", err, http.StatusNotFound,
			"artifact_path not found within the job artifact archive, or job artifacts have expired. The paths are the job's artifacts:paths in .gitlab-ci.yml")
	}
	return readSingleArtifactContent(reader, input.JobID, input.ArtifactPath), nil
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
// (GET /projects/:id/jobs/artifacts/:ref_name/raw/:artifact_path), read as
// [DownloadSingleArtifact] reads it.
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
		if toolutil.IsDownloadTooLarge(err) {
			return SingleArtifactOutput{}, toolutil.WrapErrWithHint("jobDownloadSingleArtifactByRef", err, errSingleArtifactTooLarge)
		}
		return SingleArtifactOutput{}, toolutil.WrapErrWithStatusHint("jobDownloadSingleArtifactByRef", err, http.StatusNotFound,
			"no successful job with the given name on this ref produced an artifact at artifact_path, or artifacts expired")
	}
	return readSingleArtifactContent(reader, 0, input.ArtifactPath), nil
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	j, _, err := client.GL().Jobs.EraseJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobErase", err,
				"erasing jobs requires Maintainer+ role and the job must be in a finished state (success/failed/canceled). Erase wipes the trace log and artifacts")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobErase", err, http.StatusNotFound,
			hintVerifyJobID)
	}
	return capturedJob(j, captured)
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	j, _, err := client.GL().Jobs.KeepArtifacts(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobKeepArtifacts", err,
				"keeping artifacts requires Maintainer+ role; this clears the artifact's expire_at so they are retained indefinitely")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobKeepArtifacts", err, http.StatusNotFound,
			"verify job_id; the job must have artifacts that have not yet been expired/erased")
	}
	return capturedJob(j, captured)
}

// PlayInput defines parameters for running a manual job with optional variables.
type PlayInput struct {
	ProjectID              toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	JobID                  int64                `json:"job_id"     jsonschema:"Job ID to run,required"`
	JobVariablesAttributes []JobVariableInput   `json:"job_variables_attributes,omitempty" jsonschema:"Job variables to inject into the manual job run"`
	JobInputs              map[string]any       `json:"job_inputs,omitempty" jsonschema:"Job input parameters keyed by input name. Values may be string, number, boolean, or array of strings, matching the inputs declared in .gitlab-ci.yml spec:inputs."`
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
	// No input converts to an empty map, which the option's omitempty leaves
	// out of the request exactly as it leaves out a nil one, so a guard on the
	// length could not be observed.
	inputs, err := toolutil.BuildPipelineInputs(input.JobInputs)
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("jobPlay", err,
			"job inputs must be string, number, boolean, or array of strings")
	}
	opts := &gl.PlayJobOptions{JobInputs: inputs}
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	j, _, err := client.GL().Jobs.PlayJob(string(input.ProjectID), input.JobID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("jobPlay", err,
				"job is not in a playable state. Only manual jobs (rules: when: manual) that have not yet run can be played; use job.retry for finished jobs")
		}
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("jobPlay", err,
				"playing manual jobs requires Developer+ role; protected branches/environments may require Maintainer+")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("jobPlay", err, http.StatusNotFound,
			hintVerifyJobID)
	}
	return capturedJob(j, captured)
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
				"bulk-deleting all project artifacts requires Maintainer+ role. This is irreversible across all jobs in the project")
		}
		return toolutil.WrapErrWithStatusHint("jobDeleteProjectArtifacts", err, http.StatusNotFound,
			"verify the project exists with project.get")
	}
	return nil
}

// errArtifactTooLarge is the way out for an archive or report the download
// by job ID will not return, whichever ceiling stopped it: one file of the
// archive can be read on its own, and its path is one of the job's
// artifacts:paths, since no action lists the entries of an archive.
const errArtifactTooLarge = "the artifact is too large to return in one MCP response; read one file of it with job.download_single_artifact, taking its path from the job's artifacts:paths in .gitlab-ci.yml"

// errRefArtifactTooLarge is [errArtifactTooLarge] for the download by ref,
// whose caller holds a ref and a job name and no job ID: the one-file read
// that takes the same two.
const errRefArtifactTooLarge = "the artifact is too large to return in one MCP response; read one file of it with job.download_single_artifact_by_ref for the same ref and job, taking its path from the job's artifacts:paths in .gitlab-ci.yml"

// errSingleArtifactTooLarge is the way out for one file the client-wide
// response ceiling stopped before any of it was read.
const errSingleArtifactTooLarge = "the file is larger than this server reads from GitLab in one response; download it from GitLab directly (GET /projects/:id/jobs/:job_id/artifacts/*artifact_path)"

// The encodings a single artifact's content is returned in.
const (
	encodingText   = "text"
	encodingBase64 = "base64"
)

// readArtifactContent returns a job's archive, or the report file_type
// named, base64-encoded when it holds at most [maxArtifactBytes], and its
// real size with Truncated set and no content when it is larger.
//
// A prefix is never returned: the zip archive keeps its central directory at
// the end, so its first MiB cannot be opened at all, and a gzip report's
// prefix cannot be decompressed whole either. It used to be returned anyway,
// base64-encoded, as some 350k tokens a model could do nothing with. client-go
// hands the body back already read, so its size is known without counting.
func readArtifactContent(reader *bytes.Reader, jobID int64) ArtifactsOutput {
	size := int(reader.Size())
	out := ArtifactsOutput{JobID: jobID, Size: size, Truncated: size > maxArtifactBytes}
	if out.Truncated {
		return out
	}
	data := make([]byte, size)
	_, _ = reader.Read(data) // a bytes.Reader holding size bytes fills data whole
	out.Content = base64.StdEncoding.EncodeToString(data)
	return out
}

// readSingleArtifactContent returns one artifact file: as text when its
// bytes are valid UTF-8, base64-encoded otherwise, the encoding field saying
// which, so a binary file reaches the caller byte for byte. Its bytes used to
// be put into a JSON string as they were, and every invalid UTF-8 byte came
// back as U+FFFD.
//
// A file over [maxArtifactBytes] reports its real size with Truncated set.
// Text keeps its first MiB, cut back to the last whole character so that what
// it returns is still valid UTF-8, since a prefix of a log or a report is
// readable; a binary keeps nothing, since a prefix of one is not.
func readSingleArtifactContent(reader *bytes.Reader, jobID int64, path string) SingleArtifactOutput {
	size := int(reader.Size())
	out := SingleArtifactOutput{JobID: jobID, ArtifactPath: path, Size: size, Truncated: size > maxArtifactBytes}
	data := make([]byte, min(size, maxArtifactBytes))
	_, _ = reader.Read(data) // a bytes.Reader holding at least len(data) bytes fills it whole
	if out.Truncated {
		data = withoutCutRune(data)
	}
	switch {
	case utf8.Valid(data):
		out.Encoding, out.Content = encodingText, string(data)
	case !out.Truncated:
		out.Encoding, out.Content = encodingBase64, base64.StdEncoding.EncodeToString(data)
	}
	return out
}

// withoutCutRune drops the bytes of a character a byte limit cut in two
// from the end of data, at most utf8.UTFMax-1 of them: what is left of a
// multi-byte character decodes as a one-byte error rune, and a whole one
// does not.
func withoutCutRune(data []byte) []byte {
	for range utf8.UTFMax - 1 {
		if r, n := utf8.DecodeLastRune(data); r != utf8.RuneError || n != 1 {
			break
		}
		data = data[:len(data)-1]
	}
	return data
}

// ---------------------------------------------------------------------------
// Markdown formatters for new types
// ---------------------------------------------------------------------------.
