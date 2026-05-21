package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/waitpoll"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var pollDuration = func(seconds int) time.Duration { return time.Duration(seconds) * time.Second }

// WaitInput defines parameters for waiting on a job to complete.
type WaitInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	JobID           int64                `json:"job_id"                    jsonschema:"Job ID to wait for,required"`
	IntervalSeconds int                  `json:"interval_seconds,omitempty" jsonschema:"Polling interval in seconds (5-60, default 10)"`
	TimeoutSeconds  int                  `json:"timeout_seconds,omitempty"  jsonschema:"Maximum wait time in seconds (1-3600, default 300)"`
	FailOnError     *bool                `json:"fail_on_error,omitempty"    jsonschema:"Return isError when job ends in failed/canceled status (default true)"`
}

// WaitOutput holds the result of waiting for a job.
type WaitOutput struct {
	toolutil.HintableOutput
	Job         Output `json:"job"`
	WaitedFor   string `json:"waited_for"`
	PollCount   int    `json:"poll_count"`
	FinalStatus string `json:"final_status"`
	TimedOut    bool   `json:"timed_out"`
}

// Wait polls a job until it reaches a terminal state or the timeout is reached.
// It sends MCP progress notifications to keep the client informed during polling.
func Wait(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input WaitInput) (WaitOutput, error) {
	if err := ctx.Err(); err != nil {
		return WaitOutput{}, err
	}
	if input.ProjectID == "" {
		return WaitOutput{}, errors.New("jobWait: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.JobID <= 0 {
		return WaitOutput{}, toolutil.ErrRequiredInt64("jobWait", "job_id")
	}

	result, err := waitpoll.Poll(ctx, waitpoll.Options[Output]{
		Request:         req,
		IntervalSeconds: input.IntervalSeconds,
		TimeoutSeconds:  input.TimeoutSeconds,
		FailOnError:     input.FailOnError,
		PollDuration:    pollDuration,
		ProgressMessage: func(attempt int) string {
			return fmt.Sprintf("Polling job #%d (attempt %d, status check)…", input.JobID, attempt)
		},
		Poll: func(pollCtx context.Context) (Output, error) {
			j, _, err := client.GL().Jobs.GetJob(string(input.ProjectID), input.JobID, gl.WithContext(pollCtx))
			if err != nil {
				return Output{}, toolutil.WrapErrWithStatusHint("jobWait", err, http.StatusNotFound,
					"verify project_id and job_id with gitlab_job_list; the job may have been deleted or expired during polling")
			}
			return ToOutput(j), nil
		},
		Status: func(out Output) string { return out.Status },
		FailureError: func(out Output) error {
			return fmt.Errorf("jobWait: job #%d finished with status %q", input.JobID, out.Status)
		},
	})
	if err != nil {
		return waitOutputFromResult(result), err
	}
	return waitOutputFromResult(result), nil
}

func waitOutputFromResult(result waitpoll.Result[Output]) WaitOutput {
	return WaitOutput{
		Job:         result.Item,
		WaitedFor:   result.WaitedFor,
		PollCount:   result.PollCount,
		FinalStatus: result.FinalStatus,
		TimedOut:    result.TimedOut,
	}
}
