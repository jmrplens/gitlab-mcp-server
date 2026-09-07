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

// RateLimiter enforces a token-bucket rate limit on the methods that cost a
// deployment something: those reaching GitLab with the caller's credential,
// plus `completion/complete` and `tools/list`, each on the bucket
// [AttachRateLimit] describes. A zero RPS disables the limiter (the
// constructor returns nil and the resulting middleware is a no-op).
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

	// catalogOnce guards the slower-refilling bucket tools/list draws on, kept
	// for the same reason the completion one is: derived per request it would
	// arrive full on every call and meter nothing.
	catalogOnce sync.Once
	catalog     *RateLimiter
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

// methodToolsList is the catalog listing, metered on a bucket of its own for a
// reason none of the methods above share: it reaches no upstream at all, and
// spends instead the processor every tenant of this process is waiting for.
const methodToolsList = "tools/list"

// rateLimitedErrorCode is the JSON-RPC code a refused resource or prompt
// request carries. It mirrors HTTP 429 the way the transport gates' code
// does, so a client sees one number for "come back later" on both layers.
const rateLimitedErrorCode = -42900

// RateLimitRefusalPrefix and rateLimitRetrySuffix are how every refusal this
// limiter writes begins and ends, whichever of the two wire shapes carries it.
//
// The prefix is exported because a refused tools/call arrives as a successful
// JSON-RPC result whose only distinguishing mark is this text: there is no
// code and no _meta on that shape, so anything that has to tell a refusal from
// a handler failure matches the wording. cmd/bench_resources' fairness
// scenario does exactly that, and reading the constant rather than a copy of
// the sentence makes a change of wording a compile-visible edit here instead
// of a silent reclassification of every refusal as a failure there.
const (
	RateLimitRefusalPrefix = "rate limit exceeded for "
	rateLimitRetrySuffix   = "; retry after a short backoff"
)

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

// catalogDivisor sizes the tools/list bucket relative to the tool-call one.
//
// Listing is the mirror image of completion, in the refill and only there.
// Completion arrives ten times as often as a tool call and costs almost nothing
// to answer, so its bucket refills ten times as fast; a listing arrives once at
// connect and again when the catalog changes, and on the individual surface it
// marshals about 3.2 MB and is the majority of that surface's processor time,
// so its bucket refills a tenth as fast. That is what bounds a client listing
// in a loop, which is the whole reason the bucket exists.
//
// The burst is deliberately not divided with it. What the burst decides is how
// many listings a credential may make at once, and a credential is shared by
// however many clients an operator pointed at one token: a gateway holding one
// credential for a whole population is a deployment shape this project
// documents, and those clients reconnect together after a restart or a deploy.
// Dividing the burst as well would make discovery the tightest budget in the
// deployment (four listings in hand on the HTTP defaults, against forty tool
// calls) and refuse one of those clients its very first listing, which is worse
// than a refused tool call because no model is in the loop to read the message
// and back off. The sustained cost is bounded by the refill either way.
//
// The figures the divisor is sized against are the individual surface's, the
// expensive one; on the default dynamic surface a listing is two tools, so
// there the bucket is conservative rather than tight.
const catalogDivisor = 10

// CatalogListingRPS is the refill rate a listing draws on when the tool-call
// bucket is configured at rps.
//
// Exported so the entrypoint can announce both figures at startup: the listing
// rate is the other number an operator can meet in a refusal, and the divisor
// that produces it belongs here rather than copied into a log statement. The
// burst is the configured one, unchanged; [catalogDivisor] records why.
func CatalogListingRPS(rps float64) float64 {
	return rps / catalogDivisor
}

// AttachRateLimit registers a receiving middleware that gates every method
// that reaches GitLab with the caller's credential, plus `completion/complete`
// and `tools/list`, when their buckets are empty.
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
// `tools/list` draws on a third bucket, refilled a tenth as fast as the
// tool-call one and holding the same burst (see [catalogDivisor]), and it is
// metered for a reason none of the others share: it reaches no upstream, it
// spends the processor of a process many tenants share. On the individual
// surface one listing marshals about 3.2 MB and is the majority of that
// surface's processor time, so a client listing in a loop takes the processor
// its co-tenants are waiting for while the shared bucket, which counts requests
// to GitLab, sees nothing at all. Keeping the two apart is also what preserves
// the property the old exemption provided: draining the tool-call bucket never
// refuses a client's discovery. It is refused like the flagless methods above,
// with the code that mirrors HTTP 429, because ListToolsResult carries no error
// flag either. One token is one JSON-RPC request, so a catalog split over
// several pages costs one per page; the server keeps its whole catalog in one
// page, for a different reason recorded where PageSize is set.
//
// Every other method (initialize, resources/list, prompts/list) bypasses the
// limiter: they reach no upstream and cost little to answer, and metering
// something cheap buys nothing and costs a concept. If limiter is nil, this
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
			catalog := limiter.forCatalog()
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
			case methodToolsList:
				if !isInternalInspection(ctx) && !catalog.allow() {
					// Reported on the bucket that refused, so the line carries
					// the rate that actually applied rather than the tool-call
					// one, which refills ten times as fast.
					catalog.reportRefusal(ctx, method)
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

// inspectionKey marks a context as the server's own in-memory session.
type inspectionKey struct{}

// WithInternalInspection marks ctx as belonging to a session the server opens
// against itself, so what it asks for is not charged to a caller's bucket.
//
// The server lists its own tools several times while it starts: to count them,
// to drop the excluded ones, to learn which survived the read-only and
// safe-mode passes, to build the gitlab://tools manifest, and to write the
// server card. Those requests travel the same receiving middlewares a client's
// do, so metering tools/list charged them to the deployment's own bucket, and
// on a server started with --rate-limit-burst=1 the second one was refused and
// the tool manifest resource failed to build. Pass this to the server's
// [mcp.Server.Connect]: the handler context descends from that one, and no
// header or parameter a caller controls reaches it.
//
// Only the catalog bucket consults the mark, because only listings are asked
// for this way. A method that reached GitLab would be spending the credential
// whether the server or a client asked for it, and should still be charged.
func WithInternalInspection(ctx context.Context) context.Context {
	return context.WithValue(ctx, inspectionKey{}, true)
}

// isInternalInspection reports whether ctx carries [WithInternalInspection]'s
// mark.
func isInternalInspection(ctx context.Context) bool {
	marked, _ := ctx.Value(inspectionKey{}).(bool)
	return marked
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

// forCatalog returns the slower-refilling bucket tools/list draws on, deriving
// it once per limiter and keeping it.
//
// Memoized for the reason [RateLimiter.forCompletions] is: with the bucket
// resolved per request, a copy built per call would arrive full every time,
// which is not a slower limit but no limit at all.
func (r *RateLimiter) forCatalog() *RateLimiter {
	if r == nil {
		return nil
	}
	r.catalogOnce.Do(func() { r.catalog = r.slowed(catalogDivisor) })
	return r.catalog
}

// slowed returns a limiter that refills divisor times more slowly than the
// receiver and holds the same burst, for a method that arrives far less often
// than a tool call and costs far more to answer. [catalogDivisor] records why
// the burst is left alone.
//
// A disabled receiver, or a divisor below one, yields nil rather than the
// receiver. Nil is the disabled bucket the middleware already understands,
// while handing back the receiver would alias the listing bucket onto the
// tool-call one, and a drained tool-call bucket would then refuse a client's
// discovery, which is the one thing this design forbids.
func (r *RateLimiter) slowed(divisor int) *RateLimiter {
	if r == nil || r.limiter == nil || divisor < 1 {
		return nil
	}
	return &RateLimiter{
		limiter:        rate.NewLimiter(r.limiter.Limit()/rate.Limit(divisor), r.limiter.Burst()),
		throttleWindow: r.throttleWindow,
	}
}

// scaled returns a limiter with the same rate and burst multiplied by factor,
// for a method that legitimately arrives far more often than a tool call.
// A nil receiver stays nil, which the middleware treats as disabled.
//
// The reporting window comes along, so that a window set on a limiter governs
// every bucket derived from it rather than only the one it was set on.
func (r *RateLimiter) scaled(factor int) *RateLimiter {
	if r == nil || r.limiter == nil || factor < 1 {
		return r
	}
	return &RateLimiter{
		limiter:        rate.NewLimiter(r.limiter.Limit()*rate.Limit(factor), r.limiter.Burst()*factor),
		throttleWindow: r.throttleWindow,
	}
}

// rateLimitedError is the refusal for a method whose result carries no error
// flag: a JSON-RPC error with the 429-mirroring code, and a message that says
// what to do.
func rateLimitedError(method string) error {
	return &jsonrpc.Error{
		Code:    rateLimitedErrorCode,
		Message: RateLimitRefusalPrefix + method + rateLimitRetrySuffix,
	}
}

// rateLimitedResult produces an MCP CallToolResult flagged as an error so
// the LLM can self-correct (e.g. backoff and retry). The message names the
// tool when extractable so logs and agent traces stay informative.
func rateLimitedResult(req mcp.Request) *mcp.CallToolResult {
	name := extractToolName(req)
	msg := RateLimitRefusalPrefix + methodToolsCall + rateLimitRetrySuffix
	if name != "" {
		msg = RateLimitRefusalPrefix + name + rateLimitRetrySuffix
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
