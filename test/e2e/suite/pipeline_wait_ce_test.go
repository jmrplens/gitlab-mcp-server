//go:build e2e && !enterprise

// pipeline_wait_ce_test.go contains CE/common pipeline wait helpers used by
// Docker CI runner tests.
package suite

import (
	"context"
	"fmt"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// waitForPipeline polls the GitLab API until the pipeline reaches a terminal
// state (success, failed, canceled, skipped) or the timeout expires. To be
// resilient against slow CI runners in ephemeral Docker environments, this
// helper:
//   - Uses a generous default timeout (15 min).
//   - Polls every 5 s.
//   - Tolerates transient API errors (logs and retries up to 10 consecutive
//     errors before giving up).
//   - Reports the final observed status in timeout errors so runner state is
//     easy to diagnose.
func waitForPipeline(ctx context.Context, t *testing.T, client *gitlabclient.Client, projectID, pipelineID int64, timeout time.Duration) string {
	t.Helper()
	if client == nil {
		t.Fatal("waitForPipeline: GitLab client not configured")
	}
	drainSidekiq(ctx, t, client)
	if timeout == 0 {
		timeout = 900 * time.Second
	}
	const pollInterval = 5 * time.Second
	const maxConsecutiveErrors = 10
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	lastStatus := "unknown"
	consecutiveErrors := 0
	err := Poll(pollCtx, pollInterval, timeout, func() (bool, string, error) {
		p, _, err := client.GL().Pipelines.GetPipeline(projectID, pipelineID, gl.WithContext(pollCtx))
		if err != nil {
			consecutiveErrors++
			state := fmt.Sprintf("pipeline %d in project %d: last_status=%s consecutive_errors=%d/%d error=%v", pipelineID, projectID, lastStatus, consecutiveErrors, maxConsecutiveErrors, err)
			if consecutiveErrors >= maxConsecutiveErrors {
				return false, state, fmt.Errorf("poll pipeline %d in project %d after %d consecutive API errors: %w", pipelineID, projectID, consecutiveErrors, err)
			}
			return false, state, nil
		}
		consecutiveErrors = 0
		lastStatus = p.Status
		state := fmt.Sprintf("pipeline %d in project %d: status=%s", pipelineID, projectID, p.Status)
		if isTerminalPipelineStatus(p.Status) {
			return true, state, nil
		}
		return false, state, nil
	})
	if err != nil {
		t.Fatalf("waitForPipeline: pipeline %d in project %d did not reach terminal status within %s (last status: %s): %v", pipelineID, projectID, timeout, lastStatus, err)
	}
	t.Logf("waitForPipeline: pipeline %d reached terminal status: %s", pipelineID, lastStatus)
	return lastStatus
}

// isTerminalPipelineStatus reports whether status is a GitLab pipeline state
// that will not transition further.
func isTerminalPipelineStatus(status string) bool {
	switch status {
	case "success", "failed", "canceled", "skipped":
		return true
	default:
		return false
	}
}
