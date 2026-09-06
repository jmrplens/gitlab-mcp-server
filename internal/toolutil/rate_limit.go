package toolutil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
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

	// completionOnce guards the looser bucket completion/complete draws on.
	// It is derived once and kept, because a bucket rebuilt per request would
	// arrive full every time and meter nothing.
	completionOnce sync.Once
	completion     *RateLimiter
}

// defaultThrottleWindow is the reporting interval for refusals.
//
// Long enough that a sustained flood costs six lines a minute, short enough
// that a burst is visible while it is happening.
const defaultThrottleWindow = 10 * time.Second

// methodToolsCall is the JSON-RPC method this limiter meters. It is also the
// name a refusal is logged under when the call was refused before the tool it
// names could be read.
const methodToolsCall = "tools/call"

// The other methods that reach GitLab with the caller's credential. They
// share tools/call's bucket, because to GitLab a read is a read whichever
// MCP method asked for it, and a limit that metered one door left the other
// three open.
const (
	methodResourcesRead       = "resources/read"
	methodResourcesSubscribe  = "resources/subscribe"
	methodSubscriptionsListen = "subscriptions/listen"
	methodPromptsGet          = "prompts/get"
)

// rateLimitedErrorCode is the JSON-RPC code a refused resource or prompt
// request carries. It mirrors HTTP 429 the way the transport gates' code
// does, so a client sees one number for "come back later" on both layers.
const rateLimitedErrorCode = -42900

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

	// Above the throttle, and deliberately: the log line is throttled because a
	// flood of identical lines helps nobody, but a metric is an aggregate and a
	// refusal that is not counted cannot be seen at all. This is also the one
	// refusal an operator can act on, since it is the deployment's own limit
	// rejecting a call that was well formed.
	mcpotel.RecordRefusal(ctx, RefusalRateLimited)

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
		tool = methodToolsCall
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

// AttachRateLimit registers a receiving middleware that gates every method
// that reaches GitLab with the caller's credential, and `completion/complete`,
// when their buckets are empty.
//
// `tools/call`, `resources/read`, `resources/subscribe`, `subscriptions/listen`
// and `prompts/get` draw on one bucket: each is a request to GitLab on the
// caller's behalf, and a limit that metered tool calls alone left the other
// doors open to the same upstream. They are refused differently because they
// fail differently. A refused tool call is reported as an MCP tool error
// result (IsError: true) rather than a JSON-RPC error, so the model receives a
// structured, retryable diagnostic and the agent loop can back off. A refused
// resource or prompt request is a JSON-RPC error carrying the code that
// mirrors HTTP 429, since those results have no error flag of their own. A
// refused completion returns an empty completion instead: the documented
// contract for this surface is that autocomplete is never blocked, and an
// error in a completion popup is worse than no suggestions.
//
// Every other method (initialize, tools/list, resources/list, prompts/list)
// bypasses the limiter: none of them reaches GitLab. If limiter is nil, this
// function is a no-op.
func AttachRateLimit(server *mcp.Server, limiter *RateLimiter) {
	if limiter == nil {
		return
	}
	AttachRateLimitFunc(server, func(context.Context) *RateLimiter { return limiter })
}

// AttachRateLimitFunc is [AttachRateLimit] for a server whose bucket depends on
// the request.
//
// It exists because one server now answers for every credential of a
// configuration shape, while the limit is per credential: a bucket captured at
// registration would be one budget shared by every tenant, so the noisiest of
// them would refuse everybody else's calls. The resolver reads the bucket the
// request's own pool entry owns, and returning nil means this request is not
// limited, which is what an unbound request on a shape server and the stdio
// default both want.
func AttachRateLimitFunc(server *mcp.Server, resolve func(context.Context) *RateLimiter) {
	if server == nil || resolve == nil {
		return
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			limiter := resolve(ctx)
			completions := limiter.forCompletions()
			switch method {
			case methodToolsCall:
				if !limiter.allow() {
					result := rateLimitedResult(req)
					limiter.reportRefusal(ctx, extractToolName(req))
					return result, nil
				}
			case methodResourcesRead, methodResourcesSubscribe, methodSubscriptionsListen, methodPromptsGet:
				if !limiter.allow() {
					limiter.reportRefusal(ctx, method)
					return nil, rateLimitedError(method)
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

// forCompletions returns the looser bucket completion/complete draws on,
// deriving it once per limiter and keeping it.
//
// The derivation used to happen at registration, where there was exactly one
// limiter per server and nowhere else to put it. With the bucket resolved per
// request it has to live on the limiter itself: a scaled copy built per call
// would be a fresh full bucket every time, which is not a looser limit but no
// limit at all.
func (r *RateLimiter) forCompletions() *RateLimiter {
	if r == nil {
		return nil
	}
	r.completionOnce.Do(func() { r.completion = r.scaled(completionBurstFactor) })
	return r.completion
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

// rateLimitedError is the refusal for a method whose result carries no error
// flag: a JSON-RPC error with the 429-mirroring code, and a message that says
// what to do.
func rateLimitedError(method string) error {
	return &jsonrpc.Error{
		Code:    rateLimitedErrorCode,
		Message: fmt.Sprintf("rate limit exceeded for %s; retry after a short backoff", method),
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

// DefaultMaxArgumentDepth is the nesting ceiling applied to a tools/call
// arguments object.
//
// No schema this server registers nests anywhere near it — the deepest is a
// few objects inside a few arrays — so it is a ceiling on shapes nothing
// legitimate produces rather than a budget a caller could plausibly spend.
const DefaultMaxArgumentDepth = 64

// AttachArgumentLimits registers a receiving middleware that refuses a
// tools/call whose arguments nest deeper than maxDepth, before the SDK
// decodes them.
//
// # Why this is not the HTTP body cap
//
// The SDK hands tools/call arguments to middleware as raw bytes and unmarshals
// them into a map[string]any only when it applies the tool's schema. That
// decoder (github.com/segmentio/encoding/json, through the SDK's internal json
// package) has no maximum-nesting guard, where the standard library refuses
// past 10000, and it is quadratic in nesting depth: measured on v0.5.4, an
// 18 KB value nested 9000 deep costs over a second of CPU, and a 4 MiB body
// admits depth in the millions. The decode runs ahead of additionalProperties
// validation and ahead of the handler, so read-only mode, safe mode and the
// tool's own logic are all downstream of the burn, and the per-second rate
// limiter counts requests rather than cycles.
//
// A body-size cap alone only narrows the window: 256 KiB still admits depth
// 128000. The bound that closes it is on the shape, and it belongs in a
// receiving middleware rather than in the HTTP front door so that stdio — the
// transport with no body cap at all — is covered by the same check.
//
// The scan is a single linear pass over bytes the SDK has already framed, so
// the guard costs about a microsecond on an ordinary call. A non-positive
// maxDepth disables it; a nil server is a no-op.
func AttachArgumentLimits(server *mcp.Server, maxDepth int) {
	if server == nil || maxDepth <= 0 {
		return
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == methodToolsCall {
				if raw, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && raw != nil &&
					ExceedsJSONDepth(raw.Arguments, maxDepth) {
					mcpotel.RecordRefusal(ctx, RefusalInvalidParams)
					slog.WarnContext(ctx, "tool call refused: arguments nest too deeply",
						"tool", extractToolName(req),
						"max_depth", maxDepth,
						"bytes", len(raw.Arguments),
					)
					return nil, InvalidParams(fmt.Errorf(
						"arguments nest deeper than %d levels; flatten the value and call again", maxDepth,
					))
				}
			}
			return next(ctx, method, req)
		}
	})
}

// ExceedsJSONDepth reports whether raw nests containers deeper than limit.
//
// Convenience wrapper over [JSONDepthScanner] for a value that is already
// whole in memory.
func ExceedsJSONDepth(raw []byte, limit int) bool {
	scanner := NewJSONDepthScanner(limit)
	scanner.Scan(raw)
	return scanner.Exceeded()
}

// JSONDepthScanner measures the nesting depth of a JSON value one chunk at a
// time, so a body can be judged as it streams rather than after it is
// buffered.
//
// It counts brackets and braces outside string literals, which is all that
// nesting depth is, and deliberately does not validate the JSON: a scanner
// that also parsed would be a second, divergent implementation of a decoder
// this server already runs twice. Miscounting a malformed document is harmless
// because the decoder behind it refuses that document anyway, and the count is
// never an undercount for the shape this exists to stop.
//
// The zero value is not usable; construct one with [NewJSONDepthScanner].
type JSONDepthScanner struct {
	limit    int
	depth    int
	exceeded bool
	inString bool
	escaped  bool
}

// NewJSONDepthScanner returns a scanner that trips once nesting passes limit.
func NewJSONDepthScanner(limit int) *JSONDepthScanner {
	return &JSONDepthScanner{limit: limit}
}

// Scan folds the next chunk of bytes into the measurement and reports whether
// the limit has been passed. State carries across calls, so the chunk
// boundaries an io.Reader happens to produce do not change the answer.
func (s *JSONDepthScanner) Scan(chunk []byte) bool {
	if s == nil || s.exceeded {
		return s != nil && s.exceeded
	}
	for _, c := range chunk {
		if s.inString {
			switch {
			case s.escaped:
				s.escaped = false
			case c == '\\':
				s.escaped = true
			case c == '"':
				s.inString = false
			}
			continue
		}
		switch c {
		case '"':
			s.inString = true
		case '[', '{':
			s.depth++
			if s.depth > s.limit {
				s.exceeded = true
				return true
			}
		case ']', '}':
			s.depth--
		}
	}
	return false
}

// Exceeded reports whether any chunk scanned so far passed the limit.
func (s *JSONDepthScanner) Exceeded() bool { return s != nil && s.exceeded }
