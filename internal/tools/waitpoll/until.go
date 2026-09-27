package waitpoll

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/progress"
)

// The timing project.transfer and group.transfer wait with. GitLab 19.4 made
// both transfers asynchronous: the request answers with the object where it
// still is, and a background worker moves it seconds later. Both handlers read
// the object back until it sits where the transfer put it, and they share one
// timing so the two descriptions that state it stay true together.
//
// The bound keeps the whole call, the transfer request included, below the 60
// seconds a common MCP client waits for a tool call before giving up on it
// (the TypeScript SDK's default request timeout). A wait that ran into that
// timeout would hand the model a client-side failure instead of the answer
// that says the move is still queued, which is the one it can act on.
const (
	// TransferBound is how long a transfer handler waits for the move.
	TransferBound = 45 * time.Second
	// TransferInterval is the time between two reads of the moved object.
	TransferInterval = 2 * time.Second
)

// UntilOptions configures [Until]: how to read an object GitLab changes in the
// background, and how to tell that the change has landed.
type UntilOptions[T any] struct {
	// Request carries the caller's progress token. Nil sends no progress.
	Request *mcp.CallToolRequest
	// Interval is the time before each read, the first included: the caller
	// has just been told the change was accepted, so a read at once would only
	// find it pending.
	Interval time.Duration
	// Bound is how long the whole wait may take.
	Bound time.Duration
	// Message is the progress text sent before each read.
	Message string
	// Read fetches the object's current state.
	Read func(context.Context) (T, error)
	// Landed reports whether a state Read returned is the one waited for.
	Landed func(T) bool
}

// Until reads an object every Interval until Landed accepts what Read
// returned, and answers that state and true. It answers the zero T and false
// when Bound runs out first, and the caller's context error when that context
// ends first: a caller that went away wants no answer, whether the change
// landed or not.
//
// A read that fails is not an answer and does not end the wait. The change
// being waited for was already accepted, so one read GitLab fails to serve (a
// 502 from a proxy, a timeout) says nothing about it and the next may succeed.
// A failure that persists runs the wait to Bound, which the caller reports as
// a change not yet applied, the same answer a slow worker produces.
//
// Each read runs under the bound as well as the caller's context, so a read
// still in flight when the bound runs out is abandoned rather than awaited.
func Until[T any](ctx context.Context, opts UntilOptions[T]) (state T, landed bool, err error) {
	waitCtx, cancel := context.WithTimeout(ctx, opts.Bound)
	defer cancel()
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	tracker := progress.FromRequest(opts.Request)

	for attempt := 1; ; attempt++ {
		select {
		case <-waitCtx.Done():
			return state, false, ctx.Err()
		case <-ticker.C:
		}
		tracker.Update(ctx, float64(attempt), 0, opts.Message)
		current, readErr := opts.Read(waitCtx)
		if readErr == nil && opts.Landed(current) {
			return current, true, nil
		}
	}
}
