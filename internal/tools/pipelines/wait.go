package pipelines

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
		return WaitOutput{}, errors.New("pipelineWait: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.PipelineID <= 0 {
		return WaitOutput{}, toolutil.ErrRequiredInt64("pipelineWait", "pipeline_id")
	}

	result, err := waitpoll.Poll(ctx, waitpoll.Options[DetailOutput]{
		Request:         req,
		IntervalSeconds: input.IntervalSeconds,
		TimeoutSeconds:  input.TimeoutSeconds,
		FailOnError:     input.FailOnError,
		PollDuration:    pollDuration,
		ProgressMessage: func(attempt int) string {
			return fmt.Sprintf("Polling pipeline #%d (attempt %d, status check)…", input.PipelineID, attempt)
		},
		Poll: func(pollCtx context.Context) (DetailOutput, error) {
			p, _, err := client.GL().Pipelines.GetPipeline(string(input.ProjectID), input.PipelineID, gl.WithContext(pollCtx))
			if err != nil {
				return DetailOutput{}, toolutil.WrapErrWithStatusHint("pipelineWait", err, http.StatusNotFound, "verify project_id and pipeline_id with gitlab_pipeline_list")
			}
			return DetailToOutput(p), nil
		},
		Status: func(detail DetailOutput) string { return detail.Status },
		FailureError: func(detail DetailOutput) error {
			return fmt.Errorf("pipelineWait: pipeline #%d finished with status %q", input.PipelineID, detail.Status)
		},
	})
	if err != nil {
		return waitOutputFromResult(result), err
	}
	return waitOutputFromResult(result), nil
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
