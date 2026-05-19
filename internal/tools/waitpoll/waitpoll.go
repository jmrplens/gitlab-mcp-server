// Package waitpoll provides shared polling loops for wait-style tools.
package waitpoll

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// DurationFunc converts user-facing seconds into the duration used by timers.
type DurationFunc func(seconds int) time.Duration

// Options configures a polling loop for one wait-style tool.
type Options[T any] struct {
	Request         *mcp.CallToolRequest
	IntervalSeconds int
	TimeoutSeconds  int
	FailOnError     *bool
	PollDuration    DurationFunc
	ProgressMessage func(attempt int) string
	Poll            func(context.Context) (T, error)
	Status          func(T) string
	FailureError    func(T) error
}

// Result reports the final item and polling metadata.
type Result[T any] struct {
	Item        T
	WaitedFor   string
	PollCount   int
	FinalStatus string
	TimedOut    bool
}

// Poll polls until the item reaches a terminal status, the context is canceled,
// or the configured timeout expires.
func Poll[T any](ctx context.Context, opts Options[T]) (Result[T], error) {
	interval := toolutil.ClampPollInterval(opts.IntervalSeconds)
	timeout := toolutil.ClampPollTimeout(opts.TimeoutSeconds)
	failOnError := true
	if opts.FailOnError != nil {
		failOnError = *opts.FailOnError
	}
	duration := opts.PollDuration
	if duration == nil {
		duration = func(seconds int) time.Duration { return time.Duration(seconds) * time.Second }
	}

	tracker := progress.FromRequest(opts.Request)
	deadline := time.After(duration(timeout))
	ticker := time.NewTicker(duration(interval))
	defer ticker.Stop()

	startTime := time.Now()
	pollCount := 0

	for {
		pollCount++
		tracker.Update(ctx, float64(pollCount), 0, opts.ProgressMessage(pollCount))

		item, err := opts.Poll(ctx)
		if err != nil {
			return Result[T]{}, err
		}

		status := opts.Status(item)
		if toolutil.IsTerminalStatus(status) {
			result := Result[T]{
				Item:        item,
				WaitedFor:   time.Since(startTime).Round(time.Second).String(),
				PollCount:   pollCount,
				FinalStatus: status,
			}
			if failOnError && (status == "failed" || status == "canceled") {
				return result, opts.FailureError(item)
			}
			return result, nil
		}

		select {
		case <-ctx.Done():
			return Result[T]{}, ctx.Err()
		case <-deadline:
			return Result[T]{
				Item:        item,
				WaitedFor:   time.Since(startTime).Round(time.Second).String(),
				PollCount:   pollCount,
				FinalStatus: status,
				TimedOut:    true,
			}, nil
		case <-ticker.C:
		}
	}
}
