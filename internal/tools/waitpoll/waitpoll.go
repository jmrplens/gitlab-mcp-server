// Package waitpoll provides shared polling loops for wait-style tools.
package waitpoll

import (
	"context"
	"errors"
	"fmt"
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
	if opts.Poll == nil {
		return Result[T]{}, errors.New("waitpoll: poll callback is required")
	}
	if opts.Status == nil {
		return Result[T]{}, errors.New("waitpoll: status callback is required")
	}

	interval := toolutil.ClampPollInterval(opts.IntervalSeconds)
	timeout := toolutil.ClampPollTimeout(opts.TimeoutSeconds)
	failOnError := pollFailOnError(opts.FailOnError)
	duration := pollDurationFunc(opts.PollDuration)
	progressMessage := pollProgressMessage(opts.ProgressMessage)

	tracker := progress.FromRequest(opts.Request)
	timeoutDuration := duration(timeout)
	deadlineAt := time.Now().Add(timeoutDuration)
	deadline := time.NewTimer(timeoutDuration)
	defer deadline.Stop()
	ticker := time.NewTicker(duration(interval))
	defer ticker.Stop()

	startTime := time.Now()
	pollCount := 0
	var lastItem T
	var lastStatus string

	for {
		if time.Until(deadlineAt) <= 0 {
			return timedOutPollResult(lastItem, startTime, pollCount, lastStatus), nil
		}
		if err := ctx.Err(); err != nil {
			return Result[T]{}, err
		}

		pollCount++
		tracker.Update(ctx, float64(pollCount), 0, progressMessage(pollCount))

		pollCtx, cancel := context.WithDeadline(ctx, deadlineAt)
		item, err := opts.Poll(pollCtx)
		cancel()
		if err != nil {
			if pollReachedDeadline(ctx, err, deadlineAt) {
				return timedOutPollResult(lastItem, startTime, pollCount, lastStatus), nil
			}
			return Result[T]{}, err
		}

		status := opts.Status(item)
		lastItem = item
		lastStatus = status
		if toolutil.IsTerminalStatus(status) {
			return terminalPollResult(item, startTime, pollCount, status, failOnError, opts.FailureError)
		}

		select {
		case <-ctx.Done():
			return Result[T]{}, ctx.Err()
		case <-deadline.C:
			return timedOutPollResult(item, startTime, pollCount, status), nil
		case <-ticker.C:
		}
	}
}

func pollReachedDeadline(ctx context.Context, err error, deadlineAt time.Time) bool {
	return errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil && !time.Now().Before(deadlineAt)
}

func timedOutPollResult[T any](item T, startTime time.Time, pollCount int, status string) Result[T] {
	return Result[T]{
		Item:        item,
		WaitedFor:   time.Since(startTime).Round(time.Second).String(),
		PollCount:   pollCount,
		FinalStatus: status,
		TimedOut:    true,
	}
}

func terminalPollResult[T any](item T, startTime time.Time, pollCount int, status string, failOnError bool, failureError func(T) error) (Result[T], error) {
	result := Result[T]{
		Item:        item,
		WaitedFor:   time.Since(startTime).Round(time.Second).String(),
		PollCount:   pollCount,
		FinalStatus: status,
	}
	if failOnError && (status == "failed" || status == "canceled") {
		if failureError == nil {
			return result, fmt.Errorf("waitpoll: terminal status %q requires failure error callback", status)
		}
		return result, failureError(item)
	}
	return result, nil
}

func pollFailOnError(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func pollDurationFunc(duration DurationFunc) DurationFunc {
	if duration != nil {
		return duration
	}
	return func(seconds int) time.Duration { return time.Duration(seconds) * time.Second }
}

func pollProgressMessage(message func(attempt int) string) func(attempt int) string {
	if message != nil {
		return message
	}
	return func(int) string { return "" }
}
