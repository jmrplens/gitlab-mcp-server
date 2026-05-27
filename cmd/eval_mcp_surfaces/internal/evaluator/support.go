package evaluator

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/evalrun"
)

var (
	evalElicitationReleaseTag   atomic.Value
	evalElicitationSourceBranch atomic.Value
)

// liveUniqueSuffix returns unique suffix for live evaluation runs.
func liveUniqueSuffix() string {
	return evalrun.UniqueSuffix()
}

// waitForContext waits for context to become available.
func waitForContext(ctx context.Context, interval time.Duration) error {
	return evalrun.WaitForContext(ctx, interval)
}

// options holds options data for the evaluator package.
