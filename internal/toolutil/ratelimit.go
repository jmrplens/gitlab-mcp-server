// ratelimit.go implements MCP tools/call rate limiting middleware using a
// token bucket shared by a server instance.
package toolutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
// see 429s in practice. Default is off so existing deployments keep their
// current behavior.
//
// The limiter shares a single bucket across the server. In HTTP mode each
// per-token server instance from the pool gets its own RateLimiter, so the
// limit is effectively per-token. In stdio mode the bucket is global to
// the single process.
//
// [rate.Limiter] is safe for concurrent use by design, so RateLimiter does
// not need additional synchronization of its own.
type RateLimiter struct {
	limiter *rate.Limiter
}

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
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
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

// AttachRateLimit registers a receiving middleware that rejects `tools/call`
// requests when the bucket is empty. Rejection is reported as an MCP tool
// error result (IsError: true) rather than a JSON-RPC error so the LLM
// receives a structured, retryable diagnostic and the surrounding agent
// loop can choose to back off and retry.
//
// All other methods (initialize, tools/list, resources/*, prompts/*) bypass
// the limiter — only tool execution is gated. If limiter is nil, this
// function is a no-op.
func AttachRateLimit(server *mcp.Server, limiter *RateLimiter) {
	if server == nil || limiter == nil {
		return
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			if !limiter.allow() {
				return rateLimitedResult(req), nil
			}
			return next(ctx, method, req)
		}
	})
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
