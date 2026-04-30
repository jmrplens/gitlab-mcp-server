// wait.go implements a server-side polling tool that waits for a CI/CD job
// to reach a terminal state, sending MCP progress notifications during polling.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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

	interval := toolutil.ClampPollInterval(input.IntervalSeconds)
	timeout := toolutil.ClampPollTimeout(input.TimeoutSeconds)
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
		tracker.Update(ctx, float64(pollCount), 0, fmt.Sprintf("Polling job #%d (attempt %d, status check)…", input.JobID, pollCount))

		j, _, err := client.GL().Jobs.GetJob(string(input.ProjectID), input.JobID, gl.WithContext(ctx))
		if err != nil {
			return WaitOutput{}, toolutil.WrapErrWithStatusHint("jobWait", err, http.StatusNotFound,
				"verify project_id and job_id with gitlab_job_list; the job may have been deleted or expired during polling")
		}

		out := ToOutput(j)
		if toolutil.IsTerminalStatus(out.Status) {
			elapsed := time.Since(startTime).Round(time.Second)
			result := WaitOutput{
				Job:         out,
				WaitedFor:   elapsed.String(),
				PollCount:   pollCount,
				FinalStatus: out.Status,
			}
			if failOnError && (out.Status == "failed" || out.Status == "canceled") {
				return result, fmt.Errorf("jobWait: job #%d finished with status %q", input.JobID, out.Status)
			}
			return result, nil
		}

		select {
		case <-ctx.Done():
			return WaitOutput{}, ctx.Err()
		case <-deadline:
			elapsed := time.Since(startTime).Round(time.Second)
			return WaitOutput{
				Job:         out,
				WaitedFor:   elapsed.String(),
				PollCount:   pollCount,
				FinalStatus: out.Status,
				TimedOut:    true,
			}, nil
		case <-ticker.C:
			// Continue polling
		}
	}
}
