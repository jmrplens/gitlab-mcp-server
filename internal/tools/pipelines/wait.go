// wait.go implements a server-side polling tool that waits for a pipeline
// to reach a terminal state, sending MCP progress notifications during polling.

package pipelines

import (
	"context"
	"errors"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	minInterval     = 5
	maxInterval     = 60
	defaultInterval = 10
	minTimeout      = 1
	maxTimeout      = 3600
	defaultTimeout  = 300
)

// terminalStatuses contains pipeline statuses that indicate the pipeline has finished.
var terminalStatuses = map[string]bool{
	"success":  true,
	"failed":   true,
	"canceled": true,
	"skipped":  true,
	"manual":   true,
}

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

func clampInterval(v int) int {
	if v < minInterval {
		return defaultInterval
	}
	if v > maxInterval {
		return maxInterval
	}
	return v
}

func clampTimeout(v int) int {
	if v < minTimeout {
		return defaultTimeout
	}
	if v > maxTimeout {
		return maxTimeout
	}
	return v
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

	interval := clampInterval(input.IntervalSeconds)
	timeout := clampTimeout(input.TimeoutSeconds)
	failOnError := true
	if input.FailOnError != nil {
		failOnError = *input.FailOnError
	}

	tracker := progress.FromRequest(req)
	deadline := time.After(time.Duration(timeout) * time.Second)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	pollCount := 0

	for {
		pollCount++
		tracker.Update(ctx, float64(pollCount), 0, fmt.Sprintf("Polling pipeline #%d (attempt %d, status check)…", input.PipelineID, pollCount))

		p, _, err := client.GL().Pipelines.GetPipeline(string(input.ProjectID), input.PipelineID, gl.WithContext(ctx))
		if err != nil {
			return WaitOutput{}, toolutil.WrapErrWithMessage("pipelineWait", err)
		}

		detail := DetailToOutput(p)
		if terminalStatuses[detail.Status] {
			elapsed := time.Since(startTime).Round(time.Second)
			out := WaitOutput{
				Pipeline:    detail,
				WaitedFor:   elapsed.String(),
				PollCount:   pollCount,
				FinalStatus: detail.Status,
			}
			if failOnError && (detail.Status == "failed" || detail.Status == "canceled") {
				return out, fmt.Errorf("pipelineWait: pipeline #%d finished with status %q", input.PipelineID, detail.Status)
			}
			return out, nil
		}

		select {
		case <-ctx.Done():
			return WaitOutput{}, ctx.Err()
		case <-deadline:
			elapsed := time.Since(startTime).Round(time.Second)
			return WaitOutput{
				Pipeline:    detail,
				WaitedFor:   elapsed.String(),
				PollCount:   pollCount,
				FinalStatus: detail.Status,
				TimedOut:    true,
			}, nil
		case <-ticker.C:
			// Continue polling
		}
	}
}
