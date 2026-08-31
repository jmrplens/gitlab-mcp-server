package toolutil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

// RateLimiter enforces a token-bucket rate limit on `tools/call` requests.
// A zero RPS disables the limiter (the constructor returns nil and the
// resulting middleware is a no-op).
//
// Limits are advisory; the primary defense remains GitLab's own per-token
// rate limits. The local limiter exists to soften bursts (typical LLM
// retry-loop with a flaky tool can fire dozens of identical calls per
// second) and to give operators a single knob they can tighten when they
// see 429s in practice. HTTP mode enables it by default (10 rps, burst 40);
// stdio leaves it off, since there the bucket would be global to one client.
//
// The limiter shares a single bucket across the server. In HTTP mode each
// server instance from the pool gets its own RateLimiter, so the limit is
// effectively per token and GitLab URL. In stdio mode the bucket is global to
// the single process.
//
// [rate.Limiter] is safe for concurrent use by design, so RateLimiter does
// not need additional synchronization of its own.
type RateLimiter struct {
	limiter *rate.Limiter

	// Throttle reporting. A refusal used to be completely silent: no log line,
	// no counter, nothing. Measured on the shipped default (rps 10, burst 40,
	// no flags), 150 concurrent calls produced 102 refusals inside 1.07s, and
	// the log for the whole run was session chatter and nothing else. The 48
	// served and the 102 refused were byte-identical in it, and LOG_LEVEL=debug
	// did not help: there was nothing to raise the level of.
	//
	// Self-suppressed because refusals are unbounded: their rate is the
	// arrival rate minus the limit, 95 a second in that run, so one line per
	// event would replace a silent limiter with a log flood. One line per
	// window carries the count of everything it stands for.
	reportMu       sync.Mutex
	windowStart    time.Time
	windowRefusals int
	// throttleWindow is how often a refusal is reported. Overridden in tests.
	throttleWindow time.Duration
}

// defaultThrottleWindow is the reporting interval for refusals.
//
// Long enough that a sustained flood costs six lines a minute, short enough
// that a burst is visible while it is happening.
const defaultThrottleWindow = 10 * time.Second

// NewRateLimiter builds a RateLimiter with the given rate (requests per
// second) and burst (maximum concurrent tokens in the bucket). Returns nil
// if rps <= 0, which the middleware treats as "disabled". Burst is clamped
// to a minimum of 1 when rps > 0 to avoid an unusable zero-burst limiter.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	if rps <= 0 {
		return nil
	}
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		limiter:        rate.NewLimiter(rate.Limit(rps), burst),
		throttleWindow: defaultThrottleWindow,
	}
}

// reportRefusal writes at most one line per window, naming the tool that was
// refused and how many refusals the line stands for.
//
// WARN rather than INFO: unlike the other refusals, this one is not the
// caller's doing. The call was well formed and would have succeeded a moment
// earlier or later, and an operator seeing it may need to raise the limit.
func (r *RateLimiter) reportRefusal(ctx context.Context, tool string) {
	if r == nil {
		return
	}

	r.reportMu.Lock()
	window := r.throttleWindow
	if window <= 0 {
		window = defaultThrottleWindow
	}
	now := time.Now()
	if !r.windowStart.IsZero() && now.Sub(r.windowStart) < window {
		r.windowRefusals++
		r.reportMu.Unlock()
		return
	}
	alsoRefused := r.windowRefusals
	r.windowStart = now
	r.windowRefusals = 0
	r.reportMu.Unlock()

	if tool == "" {
		tool = "tools/call"
	}
	slog.WarnContext(ctx, "tool call refused: rate limit exceeded",
		"tool", tool,
		"reason", RefusalRateLimited,
		"limit_rps", float64(r.limiter.Limit()),
		"burst", r.limiter.Burst(),
		"also_refused_since_last_report", alsoRefused,
	)
}

// allow reports whether a single token is currently available. The
// underlying [rate.Limiter] is safe for concurrent use, so no additional
// locking is required here.
func (r *RateLimiter) allow() bool {
	if r == nil || r.limiter == nil {
		return true
	}
	return r.limiter.Allow()
}

// completionBurstFactor sizes the completion bucket relative to the tool-call
// one.
//
// Completion is the highest-frequency method a client issues: an editor calls
// it per keystroke, so a bucket tuned for tool execution would refuse ordinary
// typing. The specification asks for both ("Servers SHOULD ... Rate limit
// completion requests", and under Security, "Implementations MUST ... Implement
// appropriate rate limiting"), so the answer is a looser bucket rather than the
// same one or none at all.
const completionBurstFactor = 10

// AttachRateLimit registers a receiving middleware that gates `tools/call` and
// `completion/complete` when their buckets are empty.
//
// The two are gated differently because they fail differently. A refused tool
// call is reported as an MCP tool error result (IsError: true) rather than a
// JSON-RPC error, so the model receives a structured, retryable diagnostic and
// the agent loop can back off. A refused completion returns an empty completion
// instead: the documented contract for this surface is that autocomplete is
// never blocked, and an error in a completion popup is worse than no
// suggestions.
//
// Every other method (initialize, tools/list, resources/*, prompts/*) bypasses
// the limiter. If limiter is nil, this function is a no-op.
func AttachRateLimit(server *mcp.Server, limiter *RateLimiter) {
	if server == nil || limiter == nil {
		return
	}
	completions := limiter.scaled(completionBurstFactor)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "tools/call":
				if !limiter.allow() {
					result := rateLimitedResult(req)
					limiter.reportRefusal(ctx, extractToolName(req))
					return result, nil
				}
			case "completion/complete":
				if !completions.allow() {
					return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{}}}, nil
				}
			}
			return next(ctx, method, req)
		}
	})
}

// scaled returns a limiter with the same rate and burst multiplied by factor,
// for a method that legitimately arrives far more often than a tool call.
// A nil receiver stays nil, which the middleware treats as disabled.
func (r *RateLimiter) scaled(factor int) *RateLimiter {
	if r == nil || r.limiter == nil || factor < 1 {
		return r
	}
	return &RateLimiter{
		limiter: rate.NewLimiter(r.limiter.Limit()*rate.Limit(factor), r.limiter.Burst()*factor),
	}
}

// rateLimitedResult produces an MCP CallToolResult flagged as an error so
// the LLM can self-correct (e.g. backoff and retry). The message names the
// tool when extractable so logs and agent traces stay informative.
func rateLimitedResult(req mcp.Request) *mcp.CallToolResult {
	name := extractToolName(req)
	msg := "rate limit exceeded for tools/call; retry after a short backoff"
	if name != "" {
		msg = fmt.Sprintf("rate limit exceeded for %s; retry after a short backoff", name)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// extractToolName returns the tool name from a tools/call request when
// available. The SDK delivers tools/call params as *mcp.CallToolParamsRaw
// to receiving middleware (the typed *CallToolParams comes later, once the
// handler decodes Arguments).
func extractToolName(req mcp.Request) string {
	if req == nil {
		return ""
	}
	switch p := req.GetParams().(type) {
	case *mcp.CallToolParamsRaw:
		if p != nil {
			return strings.TrimSpace(p.Name)
		}
	case *mcp.CallToolParams:
		if p != nil {
			return strings.TrimSpace(p.Name)
		}
	}
	return ""
}

// ErrInvalidRateLimit is returned by ValidateRateLimit when the parameters
// are inconsistent (e.g. burst < 1 with rps > 0).
var ErrInvalidRateLimit = errors.New("invalid rate limit configuration")

// ValidateRateLimit reports whether the given rps/burst pair forms a
// well-defined limiter configuration. Used by the server entrypoint to
// fail fast on bad CLI input rather than silently disabling the limiter.
func ValidateRateLimit(rps float64, burst int) error {
	if rps < 0 {
		return fmt.Errorf("%w: rate-limit-rps must be >= 0, got %g", ErrInvalidRateLimit, rps)
	}
	if rps > 0 && burst < 1 {
		return fmt.Errorf("%w: rate-limit-burst must be >= 1 when rps > 0, got %d", ErrInvalidRateLimit, burst)
	}
	return nil
}
