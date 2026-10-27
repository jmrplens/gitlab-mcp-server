// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package pipelines

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/waitpoll"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var pollDuration = func(seconds int) time.Duration { return time.Duration(seconds) * time.Second }

// WaitInput defines parameters for waiting on a pipeline to complete.
type WaitInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID      int64                `json:"pipeline_id"               jsonschema:"Pipeline ID to wait for,required"`
	IntervalSeconds int                  `json:"interval_seconds,omitempty" jsonschema:"Polling interval in seconds (5-60, default 10)"`
	TimeoutSeconds  int                  `json:"timeout_seconds,omitempty"  jsonschema:"Maximum wait time in seconds (1-3600, default 300)"`
	FailOnError     *bool                `json:"fail_on_error,omitempty"    jsonschema:"Return isError when pipeline ends in failed/canceled status (default true)"`
}

// WaitOutput holds the result of waiting for a pipeline.
type WaitOutput struct {
	toolutil.HintableOutput
	Pipeline    DetailOutput `json:"pipeline"`
	WaitedFor   string       `json:"waited_for"`
	PollCount   int          `json:"poll_count"`
	FinalStatus string       `json:"final_status"`
	TimedOut    bool         `json:"timed_out"`
}

// Wait polls a pipeline until it reaches a terminal state or the timeout is reached.
// It sends MCP progress notifications to keep the client informed during polling.
func Wait(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input WaitInput) (WaitOutput, error) {
	if err := ctx.Err(); err != nil {
		return WaitOutput{}, err
	}
	if input.ProjectID == "" {
		return WaitOutput{}, errors.New("pipelineWait: project_id is required. Use project.list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return WaitOutput{}, toolutil.ErrRequiredInt64("pipelineWait", "pipeline_id")
	}

	//gitlab:request mandatory: the wait reads the pipeline before it can report a status, so the read runs on every call that answers
	result, err := waitpoll.Poll(ctx, waitpoll.Options[DetailOutput]{
		Request:         req,
		IntervalSeconds: input.IntervalSeconds,
		TimeoutSeconds:  input.TimeoutSeconds,
		FailOnError:     input.FailOnError,
		PollDuration:    pollDuration,
		ProgressMessage: func(attempt int) string {
			return fmt.Sprintf("Polling pipeline #%d (attempt %d, status check)...", input.PipelineID, attempt)
		},
		Poll: func(pollCtx context.Context) (DetailOutput, error) {
			pollCtx, captured := gitlabclient.WithResponseCapture(pollCtx)
			p, _, err := client.GL().Pipelines.GetPipeline(string(input.ProjectID), input.PipelineID, gl.WithContext(pollCtx))
			if err != nil {
				return DetailOutput{}, toolutil.WrapErrWithStatusHint("pipelineWait", err, http.StatusNotFound, "verify project_id and pipeline_id with pipeline.list")
			}
			return capturedDetail("pipelineWait", p, captured)
		},
		Status: func(detail DetailOutput) string { return detail.Status },
		FailureError: func(detail DetailOutput) error {
			return pipelineFailure(ctx, client, input, detail.Status)
		},
	})
	if err != nil {
		return waitOutputFromResult(result), err
	}
	return waitOutputFromResult(result), nil
}

// failedJobsListed is how many failed jobs a failed pipeline's error names:
// one page, which keeps the answer one request long however large the
// pipeline is, and says when there are more.
const failedJobsListed = 20

// pipelineFailure is the error a wait answers a pipeline that finished failed
// or canceled with.
//
// A failed pipeline's error names the jobs that failed, each with its stage and
// the failure_reason GitLab gives it, and the step that reads a job's log: the
// status alone left a caller nothing to act on (issue 1333). The jobs are read
// only when the pipeline failed, since a canceled one has none to name, and a
// list GitLab does not give leaves the status sentence on its own, because the
// wait already knows the outcome it was asked for.
func pipelineFailure(ctx context.Context, client *gitlabclient.Client, input WaitInput, status string) error {
	finished := fmt.Sprintf("pipelineWait: pipeline #%d finished with status %q", input.PipelineID, status)
	if status != "failed" {
		return errors.New(finished)
	}
	scope := []gl.BuildStateValue{gl.Failed}
	//gitlab:request optional: the failed jobs are read only when the pipeline failed, and the wait answers without them when GitLab does not list them
	jobs, resp, err := client.GL().Jobs.ListPipelineJobs(string(input.ProjectID), input.PipelineID, &gl.ListJobsOptions{
		PerPage: failedJobsListed,
		Scope:   &scope,
	}, gl.WithContext(ctx))
	if err != nil || len(jobs) == 0 {
		return errors.New(finished)
	}
	named := make([]string, 0, len(jobs))
	for _, job := range jobs {
		named = append(named, failedJobName(job))
	}
	more := ""
	if resp.NextPage > 0 {
		more = ", and more: list them with job.list and scope failed"
	}
	return fmt.Errorf("%s. Failed jobs: %s%s. Read a failed job's log with job.trace, passing its job_id",
		finished, strings.Join(named, "; "), more)
}

// failedJobName names a failed job by its name, its ID, its stage and the
// failure_reason GitLab gives it, leaving the reason out when GitLab gives
// none.
func failedJobName(job *gl.Job) string {
	if job.FailureReason == "" {
		return fmt.Sprintf("%s (job %d, stage %s)", job.Name, job.ID, job.Stage)
	}
	return fmt.Sprintf("%s (job %d, stage %s, failure_reason %s)", job.Name, job.ID, job.Stage, job.FailureReason)
}

func waitOutputFromResult(result waitpoll.Result[DetailOutput]) WaitOutput {
	return WaitOutput{
		Pipeline:    result.Item,
		WaitedFor:   result.WaitedFor,
		PollCount:   result.PollCount,
		FinalStatus: result.FinalStatus,
		TimedOut:    result.TimedOut,
	}
}
