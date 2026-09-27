// rate_limit_test.go verifies the token-bucket rate limiter and the MCP
// receiving middleware that converts over-budget tools/call requests into
// structured tool error results.
package toolutil

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/time/rate"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// TestNewRateLimiter_Disabled verifies that a non-positive rps disables the
// limiter (returns nil, treated as no-op by AttachRateLimit).
func TestNewRateLimiter_Disabled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		rps  float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"large_negative", -100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if l := NewRateLimiter(tc.rps, 10); l != nil {
				t.Errorf("NewRateLimiter(%g, 10) = %v, want nil", tc.rps, l)
			}
		})
	}
}

// TestNewRateLimiter_ClampsBurst verifies that a burst < 1 is clamped to 1
// when rps is positive, since rate.Limiter would otherwise be unusable.
func TestNewRateLimiter_ClampsBurst(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(10, 0)
	if l == nil {
		t.Fatal("NewRateLimiter(10, 0) = nil, want non-nil")
	}
	if !l.allow() {
		t.Error("first allow() = false, want true (burst clamped to 1)")
	}
}

// TestRateLimiter_AllowsBurstThenBlocks verifies that within a single
// second the limiter grants burst tokens then blocks subsequent requests.
func TestRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	t.Parallel()
	l := NewRateLimiter(1, 3)
	for i := range 3 {
		if !l.allow() {
			t.Fatalf("allow() #%d = false, want true within burst", i+1)
		}
	}
	if l.allow() {
		t.Error("allow() after burst = true, want false")
	}
}

// TestRateLimiter_NilSafe verifies that the nil receiver always allows.
func TestRateLimiter_NilSafe(t *testing.T) {
	t.Parallel()
	var l *RateLimiter
	if !l.allow() {
		t.Error("(*RateLimiter)(nil).allow() = false, want true")
	}
	if !(&RateLimiter{}).allow() {
		t.Error("a limiter with no bucket refused; want it to allow, as a nil one does")
	}
}

// TestRateLimiterDerive_NoBucketOrFactorBelowOne_DerivesNothing pins the guards
// of the two derivations: a limiter with no bucket and a divisor or factor
// below one derive no bucket of their own. slowed answers nil, which the
// middleware reads as disabled, and scaled hands back its receiver.
func TestRateLimiterDerive_NoBucketOrFactorBelowOne_DerivesNothing(t *testing.T) {
	t.Parallel()
	bucketless := &RateLimiter{}
	configured := NewRateLimiter(10, 40)
	if got := bucketless.slowed(10); got != nil {
		t.Errorf("a bucketless limiter slowed = %p, want nil", got)
	}
	if got := configured.slowed(0); got != nil {
		t.Errorf("slowed(0) = %p, want nil", got)
	}
	if got := bucketless.scaled(10); got != bucketless {
		t.Errorf("a bucketless limiter scaled = %p, want its receiver %p", got, bucketless)
	}
	if got := configured.scaled(0); got != configured {
		t.Errorf("scaled(0) = %p, want its receiver %p", got, configured)
	}
}

// TestRateLimitedResult_UnnamedTool_NamesTheMethod verifies that a refused
// tools/call whose tool name cannot be read still says what was refused: the
// method, since the tool is unknown.
func TestRateLimitedResult_UnnamedTool_NamesTheMethod(t *testing.T) {
	t.Parallel()
	result := rateLimitedResult(&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "  "}})
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !result.IsError || text.Text != RateLimitRefusalPrefix+methodToolsCall+rateLimitRetrySuffix {
		t.Errorf("refusal = %+v, want an error result naming tools/call", result)
	}
}

// TestAttachArgumentLimits_NothingToJudge_LeavesTheRequestAlone verifies the
// two ways the argument guard has nothing to judge: no server to install on,
// and a tools/call that carries no raw arguments, which is handed on as it
// came.
func TestAttachArgumentLimits_NothingToJudge_LeavesTheRequestAlone(t *testing.T) {
	t.Parallel()
	AttachArgumentLimits(nil, DefaultMaxArgumentDepth)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	served := &mcp.CallToolResult{}
	server.AddReceivingMiddleware(func(mcp.MethodHandler) mcp.MethodHandler {
		return func(context.Context, string, mcp.Request) (mcp.Result, error) { return served, nil }
	})
	AttachArgumentLimits(server, DefaultMaxArgumentDepth)
	var handler mcp.MethodHandler
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		handler = next
		return next
	})
	result, err := handler(t.Context(), methodToolsCall, &mcp.CallToolRequest{})
	if err != nil || result != served {
		t.Errorf("a tools/call with no raw arguments was answered (%v, %v), want it handed on", result, err)
	}
}

// TestAttachRateLimit_NoLimiter verifies that a nil limiter does not register
// any middleware (calling tools/call still succeeds).
func TestAttachRateLimit_NoLimiter(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	AttachRateLimit(server, nil)

	session, ctx := connectClient(t, server)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Errorf("CallTool with nil limiter returned IsError=true: %+v", res)
	}
}

// TestAttachRateLimit_BlocksAfterBurst verifies that once the bucket is
// drained subsequent tools/call requests return IsError with a "rate limit"
// message and do not invoke the underlying handler.
func TestAttachRateLimit_BlocksAfterBurst(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	calls := 0
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Counts how many times the underlying handler runs.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		calls++
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	AttachRateLimit(server, NewRateLimiter(1, 2))

	session, ctx := connectClient(t, server)

	for i := range 2 {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
		if err != nil {
			t.Fatalf("CallTool #%d: %v", i+1, err)
		}
		if res.IsError {
			t.Fatalf("CallTool #%d returned IsError=true within burst", i+1)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if err != nil {
		t.Fatalf("CallTool over-budget: %v", err)
	}
	if !res.IsError {
		t.Fatalf("CallTool over-budget: IsError=false, want true")
	}
	if calls != 2 {
		t.Errorf("handler invocations = %d, want 2 (rate limit must short-circuit)", calls)
	}
	if len(res.Content) == 0 {
		t.Fatal("rate-limited result has empty Content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("rate-limited content type = %T, want *TextContent", res.Content[0])
	}
	if !strings.Contains(text.Text, "rate limit") {
		t.Errorf("rate-limited message = %q, want to contain 'rate limit'", text.Text)
	}
	if !strings.Contains(text.Text, "echo") {
		t.Errorf("rate-limited message = %q, want to contain tool name 'echo'", text.Text)
	}
}

// TestAttachRateLimit_GatesTheOtherDoorsToGitLab verifies resources/read and
// prompts/get draw on the same bucket as tools/call and are refused with the
// JSON-RPC code that mirrors HTTP 429 once it is empty. Each was an unmetered
// proxy to GitLab with the caller's credential while the limiter watched tool
// calls alone.
//
// One server and one bucket: the whole burst is spent through one method and
// the refusal is asserted on each of the others, which is what tells a shared
// bucket from a bucket per method. The subscription methods sit in the same
// switch case; their refusal is pinned on the wire by the HTTP transport
// module, since the SDK's client discards a subscribe response and cannot
// show it here.
func TestAttachRateLimit_GatesTheOtherDoorsToGitLab(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	server.AddResource(&mcp.Resource{URI: "test://one", Name: "one"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "hi"}}}, nil
		})
	server.AddPrompt(&mcp.Prompt{Name: "greet"},
		func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}}}, nil
		})
	AttachRateLimit(server, NewRateLimiter(1, 2))
	session, ctx := connectClient(t, server)

	readResource := func() error {
		_, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "test://one"})
		return err
	}
	getPrompt := func() error {
		_, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "greet"})
		return err
	}

	// The whole burst goes to resources/read.
	for i := range 2 {
		if err := readResource(); err != nil {
			t.Fatalf("resources/read #%d inside the burst: %v", i+1, err)
		}
	}

	assertRefused := func(method string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s over budget succeeded, want the rate-limit refusal", method)
		}
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) {
			t.Fatalf("%s over budget: error %T %v, want a JSON-RPC error", method, err, err)
		}
		if rpcErr.Code != rateLimitedErrorCode {
			t.Errorf("%s over budget: code %d, want %d", method, rpcErr.Code, rateLimitedErrorCode)
		}
		if !strings.Contains(rpcErr.Message, "rate limit") || !strings.Contains(rpcErr.Message, method) {
			t.Errorf("%s over budget: message %q, want it to name the limit and the method", method, rpcErr.Message)
		}
	}

	// Every other door finds the bucket empty.
	assertRefused("prompts/get", getPrompt())
	assertRefused("resources/read", readResource())
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if err != nil {
		t.Fatalf("tools/call over budget: %v", err)
	}
	if !res.IsError {
		t.Fatal("tools/call over budget: IsError=false, want the refusal from the bucket the reads emptied")
	}
}

// TestAttachRateLimit_ListDrawsOnABucketOfItsOwn pins the third bucket, and
// the property the exemption it replaced used to provide.
//
// tools/list bypassed the limiter entirely, on the reason recorded beside the
// switch: it reaches no upstream. That reason was the wrong axis once one
// process served many tenants. A listing on the individual surface marshals
// about 3.2 MB and is the majority of that surface's processor time, so a
// client listing in a loop spends the processor its co-tenants are waiting for
// while the shared bucket, which counts requests to GitLab, sees nothing.
//
// It cannot be metered on the tool-call bucket. Draining that one would then
// refuse a client's discovery, which is exactly what the exemption existed to
// prevent and what the separate bucket keeps. Nor can it be a weighted charge
// on it: rate.AllowN refuses unconditionally whenever the weight is over the
// burst, and the end-to-end suite runs a server with --rate-limit-burst=1.
func TestAttachRateLimit_ListDrawsOnABucketOfItsOwn(t *testing.T) {
	t.Parallel()

	t.Run("draining the tool-call bucket does not refuse a listing", func(t *testing.T) {
		t.Parallel()
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		registerEchoTool(server)
		// A token per thousand seconds, so the bucket these calls empty cannot
		// refill before the listing below arrives. At one per second a slow
		// runner could refill it in between, and the assertion would hold even
		// if the listing were drawing on the bucket it is meant to be apart
		// from.
		AttachRateLimit(server, NewRateLimiter(0.001, 1))

		session, ctx := connectClient(t, server)
		// One token and no refill: three calls in a row leave the tool-call
		// bucket empty whatever the machine's timing.
		for i := range 3 {
			if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"}); err != nil {
				t.Fatalf("CallTool #%d: %v", i+1, err)
			}
		}

		if _, err := session.ListTools(ctx, nil); err != nil {
			t.Fatalf("a listing was refused by the bucket the tool calls emptied: %v", err)
		}
	})

	t.Run("a listing is charged and refused once its own bucket is empty", func(t *testing.T) {
		t.Parallel()
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		registerEchoTool(server)
		// A burst of one carries through to the catalog bucket, refilled a
		// tenth as often as this already glacial rate, so the second listing
		// arrives at an empty bucket on any machine.
		AttachRateLimit(server, NewRateLimiter(0.001, 1))

		session, ctx := connectClient(t, server)
		if _, err := session.ListTools(ctx, nil); err != nil {
			t.Fatalf("the first listing was refused although its bucket was full: %v", err)
		}

		_, err := session.ListTools(ctx, nil)
		if err == nil {
			t.Fatal("a second listing succeeded; tools/list is not being charged to any bucket")
		}
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) {
			t.Fatalf("tools/list over budget: error %T %v, want a JSON-RPC error", err, err)
		}
		if rpcErr.Code != rateLimitedErrorCode {
			t.Errorf("tools/list over budget: code %d, want %d", rpcErr.Code, rateLimitedErrorCode)
		}
		if !strings.Contains(rpcErr.Message, "rate limit") || !strings.Contains(rpcErr.Message, methodToolsList) {
			t.Errorf("tools/list over budget: message %q, want it to name the limit and the method", rpcErr.Message)
		}
	})
}

// TestAttachRateLimit_InternalInspectionIsNotCharged pins that the listings the
// server makes against itself leave the caller's catalog bucket alone.
//
// The server lists its own tools several times while it starts, over in-memory
// sessions that travel the same receiving middlewares a client's requests do.
// Metering tools/list therefore charged startup to the deployment's own bucket,
// and on a server run with --rate-limit-burst=1 the second of those listings
// was refused: the tool manifest resource failed to build before any client had
// connected. That is what [WithInternalInspection] exists for, and this is the
// assertion that keeps a future listing from being added without it.
func TestAttachRateLimit_InternalInspectionIsNotCharged(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	// A burst of one leaves the catalog bucket one token, so a startup that
	// spent tokens would be visible on the second listing.
	AttachRateLimit(server, NewRateLimiter(1, 1))

	for i := range 5 {
		tools, err := ListRegisteredTools(t.Context(), server, "inspection")
		if err != nil {
			t.Fatalf("internal listing #%d was refused: %v", i+1, err)
		}
		if len(tools) == 0 {
			t.Fatalf("internal listing #%d returned no tools", i+1)
		}
	}

	// The token a client came for is still there.
	session, ctx := connectClient(t, server)
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatalf("a client's first listing was refused after the server inspected itself: %v", err)
	}
}

// TestAttachRateLimit_ExemptMethodsStayExempt pins the other half of the
// switch: the methods the limiter deliberately leaves alone.
//
// The decision recorded beside them is that initialize, resources/list and
// prompts/list are small, and that metering something cheap buys nothing and
// costs a concept. Nothing asserted it, so moving one of them into a metered
// case would have passed the whole suite. The buckets are emptied first, since
// a method is only shown to be exempt while there is nothing left to spend.
func TestAttachRateLimit_ExemptMethodsStayExempt(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	server.AddResource(&mcp.Resource{URI: "test://one", Name: "one"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "hi"}}}, nil
		})
	server.AddPrompt(&mcp.Prompt{Name: "greet"},
		func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "hi"}}}}, nil
		})
	// One token in each bucket and a refill of one per thousand seconds, so
	// what is spent below stays spent for the rest of the test.
	AttachRateLimit(server, NewRateLimiter(0.001, 1))

	session, ctx := connectClient(t, server)
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatalf("the first listing was refused although its bucket was full: %v", err)
	}
	if _, err := session.ListTools(ctx, nil); err == nil {
		t.Fatal("the catalog bucket was not emptied; the assertions below would prove nothing")
	}
	res, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if callErr != nil {
		t.Fatalf("tools/call inside the burst: %v", callErr)
	}
	if res.IsError {
		t.Fatal("the first tool call was refused although its bucket was full")
	}
	if res, callErr = session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"}); callErr != nil || !res.IsError {
		t.Fatal("the tool-call bucket was not emptied; the assertions below would prove nothing")
	}

	exempt := []struct {
		method string
		call   func() error
	}{
		{"resources/list", func() error { _, err := session.ListResources(ctx, nil); return err }},
		{"prompts/list", func() error { _, err := session.ListPrompts(ctx, nil); return err }},
	}
	for _, tc := range exempt {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			for i := range 3 {
				if err := tc.call(); err != nil {
					t.Fatalf("%s #%d was refused with every bucket empty: %v", tc.method, i+1, err)
				}
			}
		})
	}

	t.Run("initialize", func(t *testing.T) {
		t.Parallel()
		// A second client on the same server, so a second handshake, with both
		// buckets already empty. connectClient fails the test if the handshake
		// is refused, and this is the one exemption a client cannot work
		// around: a refused initialize is a connection that never opens.
		second, secondCtx := connectClient(t, server)
		if _, err := second.ListResources(secondCtx, nil); err != nil {
			t.Fatalf("the session opened with both buckets empty could not be used: %v", err)
		}
	})
}

// TestAttachRateLimit_InspectionMarkCoversListingsOnly pins how far the mark
// [WithInternalInspection] sets reaches.
//
// It exists so the listings the server makes against itself at startup are not
// charged to the deployment's own bucket. That reasoning stops at listings: a
// method reaching GitLab spends the caller's credential whoever asked for it,
// so a marked session must still be metered for tool calls. Nothing else pins
// the boundary, and widening the check in the switch is a one-word edit.
func TestAttachRateLimit_InspectionMarkCoversListingsOnly(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	AttachRateLimit(server, NewRateLimiter(0.001, 1))

	// Connected the way ListRegisteredTools connects: the handler context
	// descends from the marked one the server side was given.
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(WithInternalInspection(ctx), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, connectErr := client.Connect(ctx, clientTransport, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { _ = session.Close() })

	for i := range 3 {
		if _, err := session.ListTools(ctx, nil); err != nil {
			t.Fatalf("marked listing #%d was refused: %v", i+1, err)
		}
	}

	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"}); err != nil {
		t.Fatalf("the first tool call on a marked session: %v", err)
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
	if err != nil {
		t.Fatalf("the second tool call on a marked session: %v", err)
	}
	if !res.IsError {
		t.Error("a marked session's tool calls are not metered; the mark must exempt listings only, since a call spends the credential whoever asked for it")
	}
}

// TestForCatalog_IsDerivedOnceAndKept verifies the memoization the per-request
// resolver makes necessary, and that the derivation slows the refill without
// shrinking the burst.
//
// A bucket derived per request would arrive full on every call, so tools/list
// would be metered in name only. The burst is the other half: it is what a
// fleet of clients sharing one credential spends when they all connect at
// once, and dividing it too would make the first listing of the fifth client
// through a shared-credential gateway the tightest budget in the deployment.
// The end-to-end suite meets the same edge from below, running a server with
// --rate-limit-burst=1, where anything smaller than one token would refuse
// every listing for as long as it ran.
func TestForCatalog_IsDerivedOnceAndKept(t *testing.T) {
	t.Parallel()

	limiter := NewRateLimiter(10, 40)
	first := limiter.forCatalog()
	if first == nil {
		t.Fatal("forCatalog() returned nil for a configured limiter")
	}
	if second := limiter.forCatalog(); second != first {
		t.Errorf("forCatalog() returned a new bucket on the second call (%p then %p)", first, second)
	}
	if first == limiter {
		t.Error("forCatalog() returned the tool bucket itself, so listings share it")
	}
	if got, want := first.limiter.Burst(), 40; got != want {
		t.Errorf("catalog burst = %d, want %d: the burst is the parent's, so a fleet on one credential can all connect", got, want)
	}
	if got, want := float64(first.limiter.Limit()), 10.0/catalogDivisor; got != want {
		t.Errorf("catalog rps = %g, want %g", got, want)
	}
	// The reporting window travels with the bucket, or a window set on the
	// parent would govern every refusal except a listing's.
	if got, want := first.throttleWindow, limiter.throttleWindow; got != want {
		t.Errorf("catalog throttle window = %v, want the parent's %v", got, want)
	}

	t.Run("the announced listing rate is the bucket's own", func(t *testing.T) {
		t.Parallel()
		// The startup line prints this figure so an operator meets it before a
		// refusal does. Computed anywhere but from the divisor, it would drift
		// from the bucket it claims to describe.
		announced := CatalogListingRPS(10)
		if want := float64(NewRateLimiter(10, 40).forCatalog().limiter.Limit()); announced != want {
			t.Errorf("CatalogListingRPS(10) = %g, want the catalog bucket's own %g", announced, want)
		}
	})

	t.Run("a burst of one stays usable", func(t *testing.T) {
		t.Parallel()
		tight := NewRateLimiter(1, 1).forCatalog()
		if got := tight.limiter.Burst(); got != 1 {
			t.Errorf("catalog burst = %d for a configured burst of 1, want 1; a bucket of zero refuses every listing", got)
		}
		if !tight.allow() {
			t.Error("the catalog bucket refused the first listing on a server configured with --rate-limit-burst=1")
		}
	})

	t.Run("a disabled limiter stays disabled", func(t *testing.T) {
		t.Parallel()
		var absent *RateLimiter
		if got := absent.forCatalog(); got != nil {
			t.Errorf("(*RateLimiter)(nil).forCatalog() = %p, want nil", got)
		}
		if got := absent.slowed(catalogDivisor); got != nil {
			t.Errorf("slowed(nil) = %+v, want nil so the middleware stays a no-op", got)
		}
	})

	t.Run("a bucketless limiter yields nil rather than itself", func(t *testing.T) {
		t.Parallel()
		// Handing back the receiver would alias the listing bucket onto the
		// tool-call one, and a drained tool-call bucket would then refuse
		// discovery, which is the property the separate bucket exists to keep.
		bucketless := &RateLimiter{}
		if got := bucketless.slowed(catalogDivisor); got != nil {
			t.Errorf("slowed() = %p on a limiter with no bucket, want nil rather than the receiver %p", got, bucketless)
		}
		if got := bucketless.forCatalog(); got != nil {
			t.Errorf("forCatalog() = %p on a limiter with no bucket, want nil", got)
		}
	})
}

// TestRateLimiterScaled_MultipliesBothHalvesAndKeepsTheBucketsApart pins what
// a scaled bucket is: the same limiter rate and burst multiplied by the factor,
// in a bucket of its own.
//
// Both halves are asserted because the completion bucket is useless if either
// is wrong: a rate that was divided instead of multiplied refuses ordinary
// typing, and a burst that was not multiplied leaves the first keystrokes of a
// session refused. The factor-of-one case is the one that says a copy is
// returned rather than the receiver, and it is asserted by spending the copy's
// only token and finding the receiver's still there, since two limiters
// sharing one bucket is exactly the aliasing this design forbids.
func TestRateLimiterScaled_MultipliesBothHalvesAndKeepsTheBucketsApart(t *testing.T) {
	t.Parallel()

	t.Run("a factor multiplies the rate and the burst", func(t *testing.T) {
		t.Parallel()
		scaled := NewRateLimiter(2, 3).scaled(completionBurstFactor)
		if scaled == nil {
			t.Fatal("scaled() = nil for a configured limiter")
		}
		if got, want := float64(scaled.limiter.Limit()), 2.0*completionBurstFactor; got != want {
			t.Errorf("scaled rps = %g, want %g", got, want)
		}
		if got, want := scaled.limiter.Burst(), 3*completionBurstFactor; got != want {
			t.Errorf("scaled burst = %d, want %d", got, want)
		}
	})

	t.Run("a factor of one still yields a bucket of its own", func(t *testing.T) {
		t.Parallel()
		limiter := NewRateLimiter(1, 1)
		scaled := limiter.scaled(1)
		if scaled == nil {
			t.Fatal("scaled(1) = nil for a configured limiter")
		}
		if !scaled.allow() {
			t.Fatal("the scaled bucket refused its first request")
		}
		if !limiter.allow() {
			t.Error("spending the scaled bucket's token also spent the receiver's, so the two share one bucket")
		}
	})
}

// TestRateLimiterSlowed_DivisorOfOneKeepsTheRate verifies that the smallest
// divisor the helper accepts is one, which yields a separate bucket refilling
// at the receiver's own rate rather than the nil that means "disabled".
//
// The bound is "at least one", not "more than one": a divisor of one is a
// listing bucket that is merely separate, and answering nil there would disable
// the listing limit altogether while looking like a deliberate configuration.
func TestRateLimiterSlowed_DivisorOfOneKeepsTheRate(t *testing.T) {
	t.Parallel()

	limiter := NewRateLimiter(4, 6)
	slowed := limiter.slowed(1)
	if slowed == nil {
		t.Fatal("slowed(1) = nil, want a bucket refilling at the receiver's own rate")
	}
	if got, want := float64(slowed.limiter.Limit()), 4.0; got != want {
		t.Errorf("slowed rps = %g, want %g", got, want)
	}
	if got, want := slowed.limiter.Burst(), 6; got != want {
		t.Errorf("slowed burst = %d, want %d", got, want)
	}
}

// TestRateLimitedError_CarriesTheNumberThatMirrors429 pins the JSON-RPC code a
// refused resource or prompt request travels out with.
//
// It is spelled as a literal here on purpose. Every other assertion in this
// file compares the code on the wire against the constant, which says the two
// agree and nothing about what the number is; a client telling "come back
// later" from a real failure matches the number, and mirroring HTTP 429 is the
// whole reason it reads -42900 rather than an arbitrary negative.
func TestRateLimitedError_CarriesTheNumberThatMirrors429(t *testing.T) {
	t.Parallel()

	var rpcErr *jsonrpc.Error
	err := rateLimitedError(methodResourcesRead)
	if !errors.As(err, &rpcErr) {
		t.Fatalf("rateLimitedError() = %T %v, want a JSON-RPC error", err, err)
	}
	if rpcErr.Code != -42900 {
		t.Errorf("refusal code = %d, want -42900, the code that mirrors HTTP 429", rpcErr.Code)
	}
	if !strings.HasPrefix(rpcErr.Message, RateLimitRefusalPrefix) || !strings.Contains(rpcErr.Message, methodResourcesRead) {
		t.Errorf("refusal message = %q, want the refusal prefix and the method", rpcErr.Message)
	}
}

// TestExtractToolName verifies the middleware helper handles nil requests,
// raw call params, typed call params, and whitespace-only names.
func TestExtractToolName(t *testing.T) {
	t.Parallel()
	if got := extractToolName(nil); got != "" {
		t.Fatalf("extractToolName(nil) = %q, want empty", got)
	}

	tests := []struct {
		name string
		req  mcp.Request
		want string
	}{
		{
			name: "raw params",
			req:  &mcp.ClientRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: " echo "}},
			want: "echo",
		},
		{
			name: "typed params",
			req:  &mcp.ClientRequest[*mcp.CallToolParams]{Params: &mcp.CallToolParams{Name: "status"}},
			want: "status",
		},
		{
			name: "other params",
			req:  &mcp.ClientRequest[*mcp.ListToolsParams]{Params: &mcp.ListToolsParams{}},
			want: "",
		},
		{
			name: "no raw params",
			req:  &mcp.ClientRequest[*mcp.CallToolParamsRaw]{},
			want: "",
		},
		{
			name: "no typed params",
			req:  &mcp.ClientRequest[*mcp.CallToolParams]{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractToolName(tt.req); got != tt.want {
				t.Fatalf("extractToolName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestValidateRateLimit verifies the validation rules for limiter
// configuration: rps must be >= 0, and burst must be >= 1 when rps > 0.
func TestValidateRateLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		rps     float64
		burst   int
		wantErr bool
	}{
		{"disabled", 0, 0, false},
		{"valid", 10, 5, false},
		{"valid_default_burst", 1, 40, false},
		// The smallest burst the limiter can work with is accepted: the bound
		// is "at least one token", not "more than one". The end-to-end HTTP
		// suite runs a server with --rate-limit-burst=1.
		{"smallest_usable_burst", 1, 1, false},
		{"negative_rps", -1, 1, true},
		{"zero_burst_with_rps", 1, 0, true},
		{"negative_burst_with_rps", 1, -5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRateLimit(tc.rps, tc.burst)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("ValidateRateLimit(%g, %d) err = %v, wantErr %v", tc.rps, tc.burst, err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalidRateLimit) {
				t.Errorf("error %v does not wrap ErrInvalidRateLimit", err)
			}
		})
	}
}

// registerEchoTool adds a no-op echo tool used by rate-limit tests.
func registerEchoTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo tool used for rate-limit middleware verification tests.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
}

// connectClient wires an in-memory transport to server and returns a client
// session ready to issue requests. Cleanup closes the session on test exit.
func connectClient(t *testing.T, server *mcp.Server) (*mcp.ClientSession, context.Context) {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

// TestExtractToolName_NilRequest verifies that extractToolName is safe for
// nil input (returns empty string).
func TestExtractToolName_NilRequest(t *testing.T) {
	t.Parallel()
	if got := extractToolName(nil); got != "" {
		t.Errorf("extractToolName(nil) = %q, want empty", got)
	}
}

// TestAttachRateLimit_GatesCompletionWithoutBlockingIt pins the second method
// the limiter covers, and the different shape of its refusal.
//
// The completion page asks for both: "Servers SHOULD ... Rate limit completion
// requests", and under Security, "Implementations MUST ... Implement
// appropriate rate limiting". Nothing rate limited it. The middleware returned
// early for every method except tools/call, and the only other limiter in the
// tree gates authentication failures per IP, so the highest-frequency method a
// client issues — an editor calls it per keystroke — was ungated everywhere.
//
// It cannot share the tool-call bucket, which is why the factor exists: a
// bucket sized for tool execution would refuse ordinary typing. And it cannot
// fail like a tool call either. The documented contract for this surface is
// that autocomplete is never blocked, so a refusal is an empty completion, not
// an error in a popup.
func TestAttachRateLimit_GatesCompletionWithoutBlockingIt(t *testing.T) {
	t.Parallel()

	// One token per second, burst one: the tool-call bucket empties on the
	// second call, and the completion bucket carries completionBurstFactor
	// times as many.
	limiter := NewRateLimiter(1, 1)

	t.Run("completion is gated", func(t *testing.T) {
		t.Parallel()
		scaled := limiter.scaled(completionBurstFactor)
		allowed := 0
		for range completionBurstFactor * 3 {
			if scaled.allow() {
				allowed++
			}
		}
		if allowed == 0 {
			t.Fatal("the completion bucket refused everything; ordinary typing would stop working")
		}
		if allowed >= completionBurstFactor*3 {
			t.Error("the completion bucket refused nothing; the method is still ungated")
		}
		if allowed <= 1 {
			t.Errorf("allowed %d completions before refusing; the bucket is no looser than the tool-call one", allowed)
		}
	})

	t.Run("a nil limiter stays disabled", func(t *testing.T) {
		t.Parallel()
		var none *RateLimiter
		if got := none.scaled(completionBurstFactor); got != nil {
			t.Errorf("scaled(nil) = %+v, want nil so the middleware stays a no-op", got)
		}
		if !none.allow() {
			t.Error("a disabled limiter must allow everything")
		}
	})
}

// TestRateLimiter_RefusalIsReportedAndSelfSuppressed pins that a throttled
// deployment says so, without saying it ninety-five times a second.
//
// Reproduced on the shipped default before this existed (rps 10, burst 40, no
// flags, no LOG_LEVEL): 150 concurrent calls gave 102 refusals inside 1.07s and
// the log for the whole run was session chatter. The 48 served and the 102
// refused were indistinguishable in it. LOG_LEVEL=debug changed nothing:
// nothing was written at any level, so there was no level to raise.
//
// Suppression is not an optimization. Refusals are unbounded: their rate is
// the arrival rate minus the limit, so a line per refusal would replace a
// silent limiter with a flood, which is the failure mode the specification
// names when it says to rate limit log messages. One line per window carries
// the count of what it stands for, so nothing is lost.
//
// Every case runs inside a testing/synctest bubble, the way the watcher tests
// in internal/subscriptions do, because what a case here asserts is which side
// of a window boundary a refusal fell on. Under the real clock that answer is
// the scheduler's: the case that crossed a boundary had to shrink the window to
// a millisecond and then sleep, which is smaller than a scheduling quantum on a
// loaded host, so the case could cross its own boundary before the code under
// test did and fail for the timing rather than the behavior (issue 822). A
// fake clock moves only when this test sleeps, so the boundary is exact, the
// window keeps its production value in the assertion, and the whole test costs
// no real time.
func TestRateLimiter_RefusalIsReportedAndSelfSuppressed(t *testing.T) {
	tests := []struct {
		name string
		// build returns the limiter under test. A closure rather than a pair
		// of numbers because two cases need a limiter the constructor does not
		// produce: a nil one, and one assembled directly, which is what
		// scaled() hands the completion bucket.
		build func() *RateLimiter
		// refuse drives the refusals this case is about.
		refuse func(*RateLimiter)
		// wantLines is how many refusal lines the whole case may emit.
		wantLines int
		// wantContains are substrings the output must carry.
		wantContains []string
		// wantAbsent are substrings the output must not carry.
		wantAbsent []string
	}{
		{
			name:      "the first refusal in a window is reported",
			build:     func() *RateLimiter { return NewRateLimiter(1, 1) },
			refuse:    func(r *RateLimiter) { r.reportRefusal(context.Background(), "gitlab_execute_action") },
			wantLines: 1,
			wantContains: []string{
				`"level":"WARN"`,
				`"msg":"tool call refused: rate limit exceeded"`,
				`"tool":"gitlab_execute_action"`,
				`"reason":"rate_limited"`,
				`"scope":"credential"`,
				`"limit_rps":1`,
				`"burst":1`,
			},
		},
		{
			// The bucket the whole process shares says so, and carries its own
			// figures named for what they count, which is tools rather than
			// requests: an operator reading the line must not take 3000 for a
			// request rate, or go looking for a flag that raises it.
			name:      "the process's listing bucket names itself",
			build:     newProcessCatalog,
			refuse:    func(r *RateLimiter) { r.reportRefusal(context.Background(), methodToolsList) },
			wantLines: 1,
			wantContains: []string{
				`"msg":"listing refused: rate limit exceeded across the process"`,
				`"method":"tools/list"`,
				`"scope":"process"`,
				`"limit_tools_per_second":3000`,
				`"burst_tools":48000`,
				`"also_refused_since_last_report":0`,
			},
			wantAbsent: []string{`"limit_rps"`, `"burst":`, `"tool":`},
		},
		{
			// A bucket derived from a credential's is still the credential's.
			name:         "a derived bucket keeps the scope it was derived from",
			build:        func() *RateLimiter { return NewRateLimiter(10, 40).forCatalog() },
			refuse:       func(r *RateLimiter) { r.reportRefusal(context.Background(), methodToolsList) },
			wantLines:    1,
			wantContains: []string{`"scope":"credential"`, `"limit_rps":1`},
		},
		{
			name:  "a flood inside the window is counted, not logged",
			build: func() *RateLimiter { return NewRateLimiter(10, 40) },
			refuse: func(r *RateLimiter) {
				// Spread across most of the window rather than fired at one
				// instant: the clock here only moves when this loop sleeps, so
				// a flood with no sleeps in it would say nothing about a window
				// and everything about a single moment. 102 refusals a
				// two-hundredth of a window apart span just over half of one.
				for range 102 {
					r.reportRefusal(context.Background(), "gitlab_execute_action")
					time.Sleep(defaultThrottleWindow / 200)
				}
			},
			wantLines: 1,
		},
		{
			name:  "the next window reports what the last one absorbed",
			build: func() *RateLimiter { return NewRateLimiter(10, 40) },
			refuse: func(r *RateLimiter) {
				// The window is the shipped one, not a shortened stand-in: the
				// fake clock jumps the whole ten seconds the moment this
				// goroutine sleeps, so there is nothing to buy by shrinking it.
				r.reportRefusal(context.Background(), "gitlab_execute_action")
				for range 41 {
					r.reportRefusal(context.Background(), "gitlab_execute_action")
				}
				time.Sleep(defaultThrottleWindow + time.Millisecond)
				r.reportRefusal(context.Background(), "gitlab_execute_action")
			},
			wantLines: 2,
			// Without the count, an operator reading one line per ten seconds
			// has no idea whether it stands for one refusal or a thousand.
			wantContains: []string{`"also_refused_since_last_report":41`},
		},
		{
			// A window is half open: a refusal arriving exactly one window
			// after the reported one opens the next, rather than being counted
			// into a window that has already run its length. Only the fake
			// clock can land on the boundary itself.
			name:  "a refusal exactly one window later is reported",
			build: func() *RateLimiter { return NewRateLimiter(10, 40) },
			refuse: func(r *RateLimiter) {
				r.reportRefusal(context.Background(), "gitlab_execute_action")
				time.Sleep(defaultThrottleWindow)
				r.reportRefusal(context.Background(), "gitlab_execute_action")
			},
			wantLines: 2,
		},
		{
			name:      "a nil limiter reports nothing",
			build:     func() *RateLimiter { return nil },
			refuse:    func(r *RateLimiter) { r.reportRefusal(context.Background(), "gitlab_execute_action") },
			wantLines: 0,
		},
		{
			// extractToolName returns "" for a request shape it does not
			// recognize. The line has to say something an operator can read,
			// and the method is the honest fallback.
			name:         "an unnamed tool still produces a usable line",
			build:        func() *RateLimiter { return NewRateLimiter(1, 1) },
			refuse:       func(r *RateLimiter) { r.reportRefusal(context.Background(), "") },
			wantLines:    1,
			wantContains: []string{`"tool":"tools/call"`},
		},
		{
			// A limiter built without going through NewRateLimiter must not
			// divide by an unset window. The derived buckets carry their
			// parent's, so this is now reachable only by hand, which is
			// exactly how a future derivation that forgot to would arrive.
			//
			// This case and the one under it pin the fallback from opposite
			// sides, because either alone leaves it free. A silence just short
			// of the default says the fallback is at least that long and says
			// nothing about how much longer, so a fallback of an hour would
			// pass it too.
			name:  "a zero window absorbs a refusal just short of the default",
			build: func() *RateLimiter { return &RateLimiter{limiter: rate.NewLimiter(1, 1)} },
			refuse: func(r *RateLimiter) {
				r.reportRefusal(context.Background(), "gitlab_execute_action")
				time.Sleep(defaultThrottleWindow - time.Millisecond)
				r.reportRefusal(context.Background(), "gitlab_execute_action")
			},
			wantLines: 1,
		},
		{
			// The other side: a second line just past the default says the
			// fallback is no longer than it. With the case above, the two
			// bracket it to the millisecond, which is what "falls back to the
			// default" means.
			name:  "a zero window reports again just past the default",
			build: func() *RateLimiter { return &RateLimiter{limiter: rate.NewLimiter(1, 1)} },
			refuse: func(r *RateLimiter) {
				r.reportRefusal(context.Background(), "gitlab_execute_action")
				time.Sleep(defaultThrottleWindow + time.Millisecond)
				r.reportRefusal(context.Background(), "gitlab_execute_action")
			},
			wantLines: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				buf := captureSlog(t)
				tt.refuse(tt.build())

				out := buf.String()
				if got := strings.Count(out, "rate limit exceeded"); got != tt.wantLines {
					t.Fatalf("%d refusal lines, want %d:\n%s", got, tt.wantLines, out)
				}
				for _, want := range tt.wantContains {
					assertContains(t, out, want)
				}
				for _, unwanted := range tt.wantAbsent {
					assertNotContains(t, out, unwanted)
				}
			})
		})
	}
}

// TestRateLimiter_EveryRefusalIsRecordedEvenWhenTheLogIsSuppressed pins the
// split between the two reporting channels.
//
// The log line is throttled to one per window on purpose, because a client in a
// retry loop would otherwise fill the terminal with the same sentence. A metric
// is an aggregate, so the same argument does not apply to it: a refusal that is
// never recorded is one an operator cannot see at any rate. Before this,
// rate_limited was a declared constant that reached a log field and no signal.
func TestRateLimiter_EveryRefusalIsRecordedEvenWhenTheLogIsSuppressed(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	limiter := NewRateLimiter(1, 1)

	// A long window, so the second and third refusals are certainly the
	// suppressed ones and the test asserts suppression rather than racing it.
	limiter.throttleWindow = time.Hour

	const refusals = 3
	for range refusals {
		ctx, span := tp.Tracer("test").Start(context.Background(), "tools/call")
		limiter.reportRefusal(ctx, "gitlab_execute_action")
		span.End()
	}

	marked := 0
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			if attr.Key == mcpotel.AttrRefusalReason && attr.Value.AsString() == RefusalRateLimited {
				marked++
			}
		}
	}
	if marked != refusals {
		t.Errorf("%d of %d refusals carry the reason; the throttle that quiets the log line must not quiet the signal",
			marked, refusals)
	}
}

// TestAttachRateLimit_CompletionOverBudget_AnswersWithNoSuggestions covers what
// a throttled completion request receives on the wire.
//
// Completions arrive as somebody types, so the bucket is ten times the
// tool-call one and the refusal is an empty suggestion list rather than an
// error: an argument-completion request that fails loudly would put an error in
// front of a user who is only typing. The tool-call budget stays untouched by
// it, which is the reason the two buckets are separate at all.
func TestAttachRateLimit_CompletionOverBudget_AnswersWithNoSuggestions(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ServerOptions{
		CompletionHandler: func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{"suggested"}}}, nil
		},
	})
	server.AddPrompt(&mcp.Prompt{
		Name:      "review",
		Arguments: []*mcp.PromptArgument{{Name: "project"}},
	}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{}, nil
	})
	// A token per thousand seconds, so the ten the completion bucket starts
	// with are all it will ever have here. At one per second it refilled faster
	// than thirteen round trips could drain it whenever the machine was busy,
	// and the test then reported the method as ungated.
	AttachRateLimit(server, NewRateLimiter(0.001, 1))

	session, ctx := connectClient(t, server)

	params := &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "review"},
		Argument: mcp.CompleteParamsArgument{Name: "project", Value: "gitl"},
	}
	var served, refused int
	for range completionBurstFactor + 3 {
		res, err := session.Complete(ctx, params)
		if err != nil {
			t.Fatalf("completion/complete: %v", err)
		}
		if len(res.Completion.Values) == 0 {
			refused++
			continue
		}
		served++
	}

	if served == 0 {
		t.Error("every completion was refused; ordinary typing would stop suggesting anything")
	}
	if refused == 0 {
		t.Errorf("no completion was refused after %d requests; the method is ungated", completionBurstFactor+3)
	}
}

// deepArguments is the input schema of the echo tool the argument-limit tests
// register: one free-form member, so the nesting under test is the caller's
// and not the schema's.
type deepArguments struct {
	Extra any `json:"extra,omitempty"`
}

// nestedArrays returns a value that marshals to depth levels of nested JSON
// arrays, the shape whose decode is quadratic in the SDK's JSON package.
func nestedArrays(depth int) any {
	var value any = []any{}
	for range depth - 1 {
		value = []any{value}
	}
	return value
}

// TestExceedsJSONDepth verifies the linear depth scanner: nesting is counted
// across both bracket kinds, brackets inside strings and escaped quotes do not
// count, and the limit is a strict ceiling.
func TestExceedsJSONDepth(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		raw   string
		limit int
		want  bool
	}{
		{"empty", "", 4, false},
		{"scalar", `"x"`, 4, false},
		{"at_the_limit", `[[[[1]]]]`, 4, false},
		{"one_over_the_limit", `[[[[[1]]]]]`, 4, true},
		{"objects_count_too", `{"a":{"b":{"c":{"d":{"e":1}}}}}`, 4, true},
		{"mixed_kinds", `{"a":[{"b":[1]}]}`, 4, false},
		{"brackets_in_a_string_do_not_count", `{"a":"[[[[[[[[[["}`, 4, false},
		{"escaped_quote_does_not_end_the_string", `{"a":"\"[[[[[[["}`, 4, false},
		{"escaped_backslash_ends_the_string", `{"a":"x\\"}`, 4, false},
		{"siblings_do_not_accumulate", `[[1],[2],[3]]`, 2, false},
		{"limit_zero_rejects_any_container", `[]`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ExceedsJSONDepth([]byte(tc.raw), tc.limit); got != tc.want {
				t.Errorf("ExceedsJSONDepth(%q, %d) = %v, want %v", tc.raw, tc.limit, got, tc.want)
			}
		})
	}
}

// TestJSONDepthScanner_ScansAcrossChunkBoundaries verifies that the streaming
// scanner carries its state between calls, so a body split at any byte — which
// is what an io.Reader delivers — is measured the same as the whole.
func TestJSONDepthScanner_ScansAcrossChunkBoundaries(t *testing.T) {
	t.Parallel()
	const raw = `{"a":"[[[[[[[","b":[[[[[[1]]]]]]}`
	for _, tc := range []struct {
		name  string
		size  int
		limit int
		want  bool
	}{
		{"byte_at_a_time_under", 1, 8, false},
		{"byte_at_a_time_over", 1, 4, true},
		{"three_at_a_time_over", 3, 4, true},
		{"whole_body_under", len(raw), 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scanner := NewJSONDepthScanner(tc.limit)
			for start := 0; start < len(raw); start += tc.size {
				end := min(start+tc.size, len(raw))
				scanner.Scan([]byte(raw[start:end]))
			}
			if got := scanner.Exceeded(); got != tc.want {
				t.Errorf("Exceeded() = %v, want %v (chunk size %d, limit %d)", got, tc.want, tc.size, tc.limit)
			}
		})
	}
}

// TestJSONDepthScanner_NilAndAlreadyExceededScansAnswerWithoutReading
// verifies the two states in which Scan reads no bytes at all: a nil scanner,
// and one that has already tripped.
//
// A nil scanner is how a caller that never built one still calls through
// (Exceeded is documented nil-safe too), so it must answer false rather than
// dereference. Once the limit is passed the verdict is final: the chunks that
// follow are the rest of a body already being refused, and continuing to count
// them would let the closing brackets of a deep value bring the depth back
// under the limit and unsay the refusal.
func TestJSONDepthScanner_NilAndAlreadyExceededScansAnswerWithoutReading(t *testing.T) {
	t.Parallel()

	t.Run("a nil scanner reads nothing and trips nothing", func(t *testing.T) {
		t.Parallel()
		var scanner *JSONDepthScanner
		if scanner.Scan([]byte(`[[[[[[[[[[`)) {
			t.Error("(*JSONDepthScanner)(nil).Scan() = true, want false")
		}
		if scanner.Exceeded() {
			t.Error("(*JSONDepthScanner)(nil).Exceeded() = true, want false")
		}
	})

	t.Run("a scanner that has tripped stays tripped", func(t *testing.T) {
		t.Parallel()
		scanner := NewJSONDepthScanner(2)
		if !scanner.Scan([]byte(`[[[`)) {
			t.Fatal("Scan() = false on the chunk that passes the limit, want true")
		}
		if !scanner.Scan([]byte(`]]]`)) {
			t.Error("Scan() = false on the chunk after the limit was passed, want true: the closing brackets unsaid the refusal")
		}
		if !scanner.Exceeded() {
			t.Error("Exceeded() = false after the limit was passed, want true")
		}
	})
}

// TestAttachArgumentLimits_RefusesOverNestedArguments verifies that a
// tools/call whose arguments nest deeper than the cap is refused with
// InvalidParams before the handler runs, and that an ordinary call still
// reaches it.
//
// The nesting is the whole attack: the SDK unmarshals params.arguments into a
// map[string]any with a decoder that has no depth cap and is quadratic in
// nesting, so one 40 KB request buys tens of CPU-seconds. The guard has to
// short-circuit ahead of that decode, which is why the assertion is on the
// handler never running rather than only on the error.
func TestAttachArgumentLimits_RefusesOverNestedArguments(t *testing.T) {
	t.Parallel()
	// depth is the depth of the whole arguments value, the object the SDK
	// decodes, so the nested member under it is one level shallower.
	for _, tc := range []struct {
		name      string
		depth     int
		wantCalls int
		wantErr   bool
		// wantNamesLimit asks that the refusal say what was exceeded.
		//
		// It is false for the deepest case and deliberately: from go-sdk
		// v1.8.0 the transport caps its own buffering, so a message that far
		// over the limit is refused where it is read and the connection ends
		// before this guard is reached. Both refusals stop the call, which is
		// what the handler-invocation count below holds; only the wording
		// belongs to whichever layer got there first. Near the limit — where a
		// caller has a real chance of fixing the request — it is still this
		// guard that answers, and it still has to say so.
		wantNamesLimit bool
	}{
		{name: "shallow", depth: 3, wantCalls: 1},
		{name: "at_the_limit", depth: 8, wantCalls: 1},
		{name: "one_over_the_limit", depth: 9, wantErr: true, wantNamesLimit: true},
		{name: "far_over_the_limit", depth: 4000, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			calls := 0
			mcp.AddTool(server, &mcp.Tool{
				Name:        "echo",
				Description: "Counts how many times the underlying handler runs.",
			}, func(_ context.Context, _ *mcp.CallToolRequest, _ deepArguments) (*mcp.CallToolResult, any, error) {
				calls++
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
			AttachArgumentLimits(server, 8)

			session, ctx := connectClient(t, server)
			_, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "echo",
				Arguments: map[string]any{"extra": nestedArrays(tc.depth - 1)},
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CallTool at depth %d succeeded, want a refusal", tc.depth)
				}
				if tc.wantNamesLimit && !strings.Contains(err.Error(), "nest") {
					t.Errorf("refusal = %q, want it to name the nesting limit", err.Error())
				}
			} else if err != nil {
				t.Fatalf("CallTool at depth %d: %v", tc.depth, err)
			}
			if calls != tc.wantCalls {
				t.Errorf("handler invocations = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

// TestAttachArgumentLimits_LeavesOtherMethodsAlone verifies that a
// non-positive cap disables the guard and that methods other than tools/call
// are never inspected, so discovery keeps working whatever the arguments cap
// is set to.
func TestAttachArgumentLimits_LeavesOtherMethodsAlone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		limit int
	}{
		{"disabled_by_zero", 0},
		{"disabled_by_negative", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			calls := 0
			mcp.AddTool(server, &mcp.Tool{
				Name:        "echo",
				Description: "Counts how many times the underlying handler runs.",
			}, func(_ context.Context, _ *mcp.CallToolRequest, _ deepArguments) (*mcp.CallToolResult, any, error) {
				calls++
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
			})
			AttachArgumentLimits(server, tc.limit)

			session, ctx := connectClient(t, server)
			if _, err := session.ListTools(ctx, nil); err != nil {
				t.Fatalf("ListTools: %v", err)
			}
			if _, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "echo",
				Arguments: map[string]any{"extra": nestedArrays(200)},
			}); err != nil {
				t.Fatalf("CallTool with the guard disabled: %v", err)
			}
			if calls != 1 {
				t.Errorf("handler invocations = %d, want 1", calls)
			}
		})
	}
}

// TestAttachRateLimitFunc_ResolvesABucketPerRequest verifies that the limit is
// a property of the caller rather than of the server.
//
// One MCP server now answers for every credential of a configuration shape, so
// a bucket captured at registration would be one budget shared by every tenant
// and the noisiest of them would refuse everybody else's calls. The resolver is
// what makes each request draw on its own.
func TestAttachRateLimitFunc_ResolvesABucketPerRequest(t *testing.T) {
	t.Parallel()

	type tenantKey struct{}
	// A burst of one, and a refill slow enough that wall clock cannot decide the
	// outcome: at one request per second the noisy tenant's second call is
	// served whenever a second passes between the two, which is a second of a
	// loaded machine's scheduling rather than anything about the code.
	buckets := map[string]*RateLimiter{
		"noisy": NewRateLimiter(0.001, 1),
		"quiet": NewRateLimiter(0.001, 1),
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	for tenant := range buckets {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "echo_" + tenant,
			Description: "Answers so the limiter is the only thing that can refuse.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	}
	AttachRateLimitFunc(server, func(ctx context.Context) *RateLimiter {
		tenant, _ := ctx.Value(tenantKey{}).(string)
		return buckets[tenant]
	})
	// Installed last, so it runs first and every handler below it sees the
	// tenant this call belongs to. It stands in for the credential binding the
	// HTTP layer performs; the tool name stands in for the credential.
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// extractToolName rather than a type assertion of its own: the
			// SDK delivers tools/call params to receiving middleware as
			// *CallToolParamsRaw, and asserting the typed form silently
			// matches nothing.
			name := strings.TrimPrefix(extractToolName(req), "echo_")
			return next(context.WithValue(ctx, tenantKey{}, name), method, req)
		}
	})

	session, ctx := connectClient(t, server)
	call := func(tenant string) bool {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo_" + tenant})
		if err != nil {
			t.Fatalf("CallTool(%s): %v", tenant, err)
		}
		return res.IsError
	}

	if call("noisy") {
		t.Fatal("the noisy tenant's first call was refused within its own burst")
	}
	if !call("noisy") {
		t.Error("the noisy tenant's second call was served, so its bucket is not being drawn on")
	}
	if call("quiet") {
		t.Error("the quiet tenant was refused because another tenant had spent its budget")
	}
}

// TestAttachRateLimitFunc_NothingToInstall verifies the two calls that must
// register no middleware at all: no server, and no resolver.
//
// Each guard is asserted through what happens without it rather than by the
// call returning. Dropping the nil-server check dereferences a nil server here.
// Dropping the nil-resolver check installs a middleware that calls a nil
// function on the first request, so the case that would otherwise assert
// nothing at all drives a real call through the server it built.
func TestAttachRateLimitFunc_NothingToInstall(t *testing.T) {
	t.Parallel()

	t.Run("no server", func(t *testing.T) {
		t.Parallel()
		AttachRateLimitFunc(nil, func(context.Context) *RateLimiter { return nil })
	})

	t.Run("no resolver", func(t *testing.T) {
		t.Parallel()
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		var calls int
		mcp.AddTool(server, &mcp.Tool{
			Name:        "echo",
			Description: "Answers, so that only an installed middleware could refuse or panic.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			calls++
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})

		AttachRateLimitFunc(server, nil)

		session, ctx := connectClient(t, server)
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
		if err != nil {
			t.Fatalf("CallTool: %v; a middleware was installed with no resolver to call", err)
		}
		if res.IsError {
			t.Errorf("the call was refused: %v", res.Content)
		}
		if calls != 1 {
			t.Errorf("handler invocations = %d, want 1", calls)
		}
	})
}

// TestAttachRateLimitFunc_AnUnlimitedRequestIsServed verifies that a resolver
// answering nil means "not limited" rather than "refused".
//
// It is the stdio default and the state of a request on a shared server that
// nothing could attribute, and in both the call has to go through: a limiter
// that refused what it could not identify would turn a missing binding into an
// outage.
func TestAttachRateLimitFunc_AnUnlimitedRequestIsServed(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	calls := 0
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Counts how many times the underlying handler runs.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		calls++
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	AttachRateLimitFunc(server, func(context.Context) *RateLimiter { return nil })

	session, ctx := connectClient(t, server)
	for i := range 3 {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
		if err != nil {
			t.Fatalf("CallTool #%d: %v", i+1, err)
		}
		if res.IsError {
			t.Fatalf("CallTool #%d was refused although no bucket applies", i+1)
		}
	}
	if calls != 3 {
		t.Errorf("handler invocations = %d, want 3", calls)
	}
}

// TestForCompletions_IsDerivedOnceAndKept verifies the memoization the
// per-request resolver made necessary.
//
// The completion bucket used to be derived at registration, where there was one
// limiter per server. Resolved per request, a scaled copy built each time would
// arrive full on every call, which is not a looser limit but no limit at all.
func TestForCompletions_IsDerivedOnceAndKept(t *testing.T) {
	t.Parallel()

	limiter := NewRateLimiter(1, 2)
	first := limiter.forCompletions()
	if first == nil {
		t.Fatal("forCompletions() returned nil for a configured limiter")
	}
	if second := limiter.forCompletions(); second != first {
		t.Errorf("forCompletions() returned a new bucket on the second call (%p then %p)", first, second)
	}
	if first == limiter {
		t.Error("forCompletions() returned the tool bucket itself, so completions share it")
	}

	var absent *RateLimiter
	if got := absent.forCompletions(); got != nil {
		t.Errorf("(*RateLimiter)(nil).forCompletions() = %p, want nil", got)
	}
}

// neverDryLimiter returns a bucket no test or benchmark here can empty: a
// billion tokens a second and a burst of a million. Every request it meets takes
// the admitted path, which is the one every served request pays for.
func neverDryLimiter() *RateLimiter {
	return NewRateLimiter(1e9, 1<<20)
}

// meteredResourceURI is the one resource the metered session serves.
const meteredResourceURI = "test://one"

// meteredMethodCase is one method the allocation pins and the benchmark drive
// the rate-limit middleware with.
type meteredMethodCase struct {
	method string
	// request is what the middleware is handed when it is called directly.
	request mcp.Request
	// call asks for the same method over a client session.
	call func(context.Context, *mcp.ClientSession) error
	// allocs is how many times the middleware allocates for one admitted
	// request of the method, as measured on this tree (Go 1.27.1, go-sdk
	// v1.8.0, golang.org/x/time/rate as go.mod pins it).
	allocs float64
}

// meteredMethodCases is one method for each bucket the middleware charges
// (tools/call and resources/read share the tool-call bucket and are refused on
// different channels, so both are here), and one method it charges to none.
func meteredMethodCases() []meteredMethodCase {
	completion := &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "review"},
		Argument: mcp.CompleteParamsArgument{Name: "project", Value: "gitl"},
	}
	return []meteredMethodCase{
		{
			method:  methodToolsCall,
			request: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "echo"}},
			call: func(ctx context.Context, s *mcp.ClientSession) error {
				_, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "echo"})
				return err
			},
		},
		{
			method:  methodResourcesRead,
			request: &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: meteredResourceURI}},
			call: func(ctx context.Context, s *mcp.ClientSession) error {
				_, err := s.ReadResource(ctx, &mcp.ReadResourceParams{URI: meteredResourceURI})
				return err
			},
		},
		{
			method:  methodToolsList,
			request: &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}},
			call: func(ctx context.Context, s *mcp.ClientSession) error {
				_, err := s.ListTools(ctx, nil)
				return err
			},
		},
		{
			method:  "completion/complete",
			request: &mcp.CompleteRequest{Params: completion},
			call: func(ctx context.Context, s *mcp.ClientSession) error {
				_, err := s.Complete(ctx, completion)
				return err
			},
		},
		{
			method:  "resources/list",
			request: &mcp.ListResourcesRequest{Params: &mcp.ListResourcesParams{}},
			call: func(ctx context.Context, s *mcp.ClientSession) error {
				_, err := s.ListResources(ctx, nil)
				return err
			},
		},
	}
}

// rateLimitMiddlewareUnderTest returns the handler [AttachRateLimitFunc]
// installs, resolving every request to limiter, with a handler behind it that
// answers every method at once with served.
//
// It is the middleware exactly as a request meets it and nothing else: what the
// SDK does to decode a request and dispatch it, which a round trip over a
// session adds and a dependency update moves, is outside it. The handler is
// taken from a middleware added last, which the SDK calls once, as it is added,
// with the handler the ones before it make. The process's listing bucket is
// limiter too rather than the one the binary shares, so a benchmark running
// millions of listings measures the admitted path instead of emptying it.
func rateLimitMiddlewareUnderTest(tb testing.TB, limiter *RateLimiter) (handler mcp.MethodHandler, served mcp.Result) {
	tb.Helper()
	served = &mcp.CallToolResult{}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddReceivingMiddleware(func(mcp.MethodHandler) mcp.MethodHandler {
		return func(context.Context, string, mcp.Request) (mcp.Result, error) { return served, nil }
	})
	attachRateLimitFunc(server, func(context.Context) *RateLimiter { return limiter }, limiter)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		handler = next
		return next
	})
	if handler == nil {
		tb.Fatal("the SDK did not hand the last middleware the handler before it; the middleware cannot be measured on its own")
	}
	return handler, served
}

// meteredSession connects a client over an in-memory transport to a server
// answering every method [meteredMethodCases] asks, with the rate-limit
// middleware resolving every request to limiter, the way [connectClient]
// connects the tests above. It takes a testing.TB so the benchmark can use it.
func meteredSession(tb testing.TB, limiter *RateLimiter) *mcp.ClientSession {
	tb.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ServerOptions{
		CompletionHandler: func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{"suggested"}}}, nil
		},
	})
	registerEchoTool(server)
	server.AddResource(&mcp.Resource{URI: meteredResourceURI, Name: "one"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "hi"}}}, nil
		})
	server.AddPrompt(&mcp.Prompt{Name: "review", Arguments: []*mcp.PromptArgument{{Name: "project"}}},
		func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		})
	attachRateLimitFunc(server, func(context.Context) *RateLimiter { return limiter }, limiter)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		tb.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		tb.Fatalf("client connect: %v", err)
	}
	tb.Cleanup(func() { _ = session.Close() })
	return session
}

// TestAttachRateLimitFunc_AllocationsPerMethod pins how many times the
// rate-limit middleware allocates for one admitted request of each method it
// charges, and of one it charges to none.
//
// The middleware runs in front of every request a client sends, so an
// allocation added to it is paid by every request of every tenant. The pins are
// the counts measured before the register decided which bucket a method is
// charged to, and they hold the middleware to them now that it asks
// tenancy.MeterFor (RTC-001 to RTC-004): asking the register must not allocate
// where the switch it replaced did not.
//
// The middleware is measured on its own, called directly with a handler behind
// it that answers at once (see [rateLimitMiddlewareUnderTest]), so the count is
// of the middleware and not of the SDK around it; [BenchmarkAttachRateLimitFunc]
// weighs it against a whole round trip. Each method is first asked over a
// session too, which keeps the benchmark's session half answering. A refusal
// allocates its answer and is pinned by what it says, by the tests above,
// rather than by what it costs.
//
// It does not run in parallel: the count a measurement reads is the whole
// process's, so a test allocating beside it would be counted against the
// middleware.
func TestAttachRateLimitFunc_AllocationsPerMethod(t *testing.T) {
	limiter := neverDryLimiter()
	handler, served := rateLimitMiddlewareUnderTest(t, limiter)
	session := meteredSession(t, limiter)
	ctx := t.Context()

	for _, tc := range meteredMethodCases() {
		t.Run(tc.method, func(t *testing.T) {
			if err := tc.call(ctx, session); err != nil {
				t.Fatalf("%s over a session: %v", tc.method, err)
			}
			result, err := handler(ctx, tc.method, tc.request)
			if err != nil || result != served {
				t.Fatalf("%s was answered by the middleware (%v, %v); the measurement below would be of a refusal", tc.method, result, err)
			}
			got := testing.AllocsPerRun(1000, func() { _, _ = handler(ctx, tc.method, tc.request) })
			if got != tc.allocs {
				t.Errorf("the middleware allocates %v times per %s, want %v", got, tc.method, tc.allocs)
			}
		})
	}
}

// BenchmarkAttachRateLimitFunc measures one admitted request of each method
// [TestAttachRateLimitFunc_AllocationsPerMethod] pins, at two depths: the
// middleware called on its own, where a change to how it charges a method
// shows, and a whole round trip over an in-memory session, the way the tests
// above drive it, where that change is weighed against what a request costs
// anyway. It is what benchstat compared before and after the method switch
// became tenancy.MeterFor, and what it compares any later change to it.
func BenchmarkAttachRateLimitFunc(b *testing.B) {
	limiter := neverDryLimiter()
	handler, _ := rateLimitMiddlewareUnderTest(b, limiter)
	session := meteredSession(b, limiter)
	ctx := b.Context()

	for _, tc := range meteredMethodCases() {
		b.Run("middleware/"+tc.method, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_, _ = handler(ctx, tc.method, tc.request)
			}
		})
		b.Run("session/"+tc.method, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if err := tc.call(ctx, session); err != nil {
					b.Fatalf("%s: %v", tc.method, err)
				}
			}
		})
	}
}

// testProcessBucket is a process listing bucket of burst tools that refills a
// token a million seconds, so what a test spends from it stays spent for as
// long as the test runs.
func testProcessBucket(burst int) *RateLimiter {
	return &RateLimiter{
		limiter:        rate.NewLimiter(1e-6, burst),
		scope:          scopeProcess,
		throttleWindow: defaultThrottleWindow,
	}
}

// heldTools is how many whole tokens a bucket holds now. Floored, because the
// glacial refill of [testProcessBucket] adds a few millionths of a token while
// a test runs, and what the tests below count is tools.
func heldTools(r *RateLimiter) int {
	return int(math.Floor(r.limiter.Tokens()))
}

// listingOf is a tools/list result carrying n tools.
func listingOf(n int) *mcp.ListToolsResult {
	listed := &mcp.ListToolsResult{Tools: make([]*mcp.Tool, 0, n)}
	for i := range n {
		listed.Tools = append(listed.Tools, &mcp.Tool{Name: "tool_" + strconv.Itoa(i)})
	}
	return listed
}

// TestProcessCatalog_Values_AreTheRegistersRTC007 holds the bucket the whole
// process shares to the register's row RTC-007: its refill and burst are the
// values the row names, counted in tools, and it names itself as the process's
// in the line a refusal writes.
func TestProcessCatalog_Values_AreTheRegistersRTC007(t *testing.T) {
	t.Parallel()
	if got, want := processCatalog.limiter.Limit(), rate.Limit(tenancy.CatalogProcessRate); got != want {
		t.Errorf("refill = %v tools a second, want %v", got, want)
	}
	if got, want := processCatalog.limiter.Burst(), tenancy.CatalogProcessBurst; got != want {
		t.Errorf("burst = %d tools, want %d", got, want)
	}
	if processCatalog.scope != scopeProcess {
		t.Errorf("scope = %q, want %q", processCatalog.scope, scopeProcess)
	}
}

// TestRateLimiterTake_ChargeOverWhatIsHeld_IsRefusedAndSpendsNothing pins the
// charge a listing makes before it is answered: granted when the bucket holds
// the whole of it, refused without spending anything when it does not, and cut
// to the burst when it asks for more than the bucket could ever hold.
func TestRateLimiterTake_ChargeOverWhatIsHeld_IsRefusedAndSpendsNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		spent   int
		charge  int
		granted bool
		left    int
	}{
		{name: "a full bucket grants all of itself", charge: 10, granted: true, left: 0},
		{name: "exactly what is left is granted", spent: 6, charge: 4, granted: true, left: 0},
		{name: "one more than is left is refused and spends nothing", spent: 6, charge: 5, granted: false, left: 4},
		{name: "a charge above the burst is cut to it", charge: 25, granted: true, left: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bucket := testProcessBucket(10)
			if tc.spent > 0 {
				if _, ok := bucket.take(tc.spent); !ok {
					t.Fatalf("take(%d) on a full bucket of 10 was refused", tc.spent)
				}
			}
			if _, granted := bucket.take(tc.charge); granted != tc.granted {
				t.Errorf("take(%d) granted = %v, want %v", tc.charge, granted, tc.granted)
			}
			if got := heldTools(bucket); got != tc.left {
				t.Errorf("%d tools left, want %d", got, tc.left)
			}
		})
	}
}

// TestCatalogChargeRefund_EntryRefusal_HandsTheToolsBack verifies that the
// tools a listing took are returned whole when the entry's own bucket refuses
// it, which is what makes that refusal cost the process bucket nothing
// (PAT-003).
func TestCatalogChargeRefund_EntryRefusal_HandsTheToolsBack(t *testing.T) {
	t.Parallel()
	bucket := testProcessBucket(10)
	charge, ok := bucket.take(7)
	if !ok || heldTools(bucket) != 3 {
		t.Fatalf("take(7) = %v with %d left, want it granted with 3 left", ok, heldTools(bucket))
	}
	charge.refund()
	if got := heldTools(bucket); got != 10 {
		t.Errorf("%d tools after the refund, want the 10 the bucket held before", got)
	}
}

// TestRateLimiterDebit_SettlementOverTheBucket_WaitsItOutUpToOneBurstOwed
// verifies the settlement of a listing that carried more tools than it was
// charged for: it is spent whether the bucket holds it or not, so the listings
// after it wait it out, a debit of nothing spends nothing, one above the burst
// is cut to it, and the debt stops at one burst however many settlements land,
// which is what keeps a wave of listings admitted before anyone knew their
// size from locking every listing in the process out for as long as their sum
// takes to refill.
func TestRateLimiterDebit_SettlementOverTheBucket_WaitsItOutUpToOneBurstOwed(t *testing.T) {
	t.Parallel()
	// Every case is a run of debits against a bucket of ten, the first of
	// four, so a debit cut to the burst is told apart from one spent whole.
	for _, tc := range []struct {
		name   string
		debits []int
		left   int
	}{
		{name: "nothing", debits: []int{4, 0}, left: 6},
		{name: "less than the bucket holds", debits: []int{4, 4}, left: 2},
		{name: "more than the bucket holds", debits: []int{4, 7}, left: -1},
		{name: "more than the bucket could ever hold", debits: []int{4, 40}, left: -4},
		{name: "a settlement stops at one burst owed", debits: []int{4, 10, 10}, left: -10},
		{name: "once a burst is owed nothing more is spent", debits: []int{4, 10, 10, 1}, left: -10},
		{name: "a wave of settlements owes one burst", debits: []int{10, 10, 10, 10, 10}, left: -10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bucket := testProcessBucket(10)
			for _, n := range tc.debits {
				bucket.debit(n)
			}
			if got := heldTools(bucket); got != tc.left {
				t.Errorf("%d tools left, want %d", got, tc.left)
			}
		})
	}
}

// listingRefusal is the refusal a listing meets from either bucket, as a client
// reads it: the words are the same, so the cases below tell the two apart by
// what each bucket holds afterwards and by the line the refusal writes.
const listingRefusal = RateLimitRefusalPrefix + methodToolsList + rateLimitRetrySuffix

// errListingFailed is what the handler behind a failed listing answers.
var errListingFailed = errors.New("the listing failed")

// serveCase is one listing put through [catalogListing.serve], with what it
// must leave behind.
type serveCase struct {
	name string
	// spent is what the process bucket of processBurst has already given out,
	// and remembered what the server's last listing carried.
	processBurst, spent int
	remembered          int64
	// entryDrained empties the entry's bucket of five before the listing.
	entryDrained bool
	answer       func() (mcp.Result, error)
	// wantErr is the refusal or the handler's error, empty when the listing is
	// answered.
	wantErr string
	// wantHeldWhenAnswered is what the process bucket held while the handler
	// answered, which is what the listing was charged up front.
	wantHeldWhenAnswered int
	wantLeft             int
	wantRemembered       int64
	wantEntryLeft        int
}

// serveOutcome is what one listing through serve left behind.
type serveOutcome struct {
	err                                      error
	answered                                 bool
	heldWhenAnswered, processLeft, entryLeft int
	remembered                               int64
}

// run puts the case's listing through serve, against a process bucket and an
// entry bucket of its own, and reads back what it left.
func (tc serveCase) run(t *testing.T) serveOutcome {
	t.Helper()
	process := testProcessBucket(tc.processBurst)
	process.debit(tc.spent)
	entry := &RateLimiter{limiter: rate.NewLimiter(1e-6, 5), scope: scopeCredential}
	if tc.entryDrained {
		entry.limiter.AllowN(time.Now(), 5)
	}
	listings := &catalogListing{process: process}
	listings.tools.Store(tc.remembered)

	var got serveOutcome
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		got.answered = true
		got.heldWhenAnswered = heldTools(process)
		return tc.answer()
	}
	request := &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}}
	_, got.err = listings.serve(t.Context(), methodToolsList, request, next, entry)
	got.processLeft, got.entryLeft, got.remembered = heldTools(process), heldTools(entry), listings.tools.Load()
	return got
}

// check holds what the listing left to what the case says it must.
func (tc serveCase) check(t *testing.T, got serveOutcome) {
	t.Helper()
	gotErr := ""
	if got.err != nil {
		gotErr = got.err.Error()
	}
	if gotErr != tc.wantErr {
		t.Fatalf("serve: %q, want %q", gotErr, tc.wantErr)
	}
	refused := tc.wantErr == listingRefusal
	if got.answered == refused {
		t.Errorf("the handler answered = %v, want %v", got.answered, !refused)
	}
	if got.answered && got.heldWhenAnswered != tc.wantHeldWhenAnswered {
		t.Errorf("the process bucket held %d tools while the listing was answered, want %d",
			got.heldWhenAnswered, tc.wantHeldWhenAnswered)
	}
	if got.processLeft != tc.wantLeft {
		t.Errorf("the process bucket holds %d tools afterwards, want %d", got.processLeft, tc.wantLeft)
	}
	if got.remembered != tc.wantRemembered {
		t.Errorf("the server is remembered to list %d tools, want %d", got.remembered, tc.wantRemembered)
	}
	if got.entryLeft != tc.wantEntryLeft {
		t.Errorf("the entry's bucket holds %d tokens afterwards, want %d", got.entryLeft, tc.wantEntryLeft)
	}
}

// TestCatalogListing_Serve_ChargesTheProcessFirstAndInTools pins how one
// server's listings are charged to the bucket the whole process shares and to
// the entry's own.
//
// The process bucket is charged first and in tools: before a listing is
// answered with what the server's last listing carried, and afterwards with
// whatever this one carried beyond that, which is how a server's first listing
// comes to cost what it is when nothing taught the bucket first. A listing the
// process bucket cannot cover is refused before the entry's bucket is touched,
// and one the entry's bucket refuses gets its tools back, so neither refusal
// costs the other bucket anything (PAT-003). An answer that is not a listing
// settles nothing.
func TestCatalogListing_Serve_ChargesTheProcessFirstAndInTools(t *testing.T) {
	t.Parallel()
	listing := func(n int) func() (mcp.Result, error) {
		return func() (mcp.Result, error) { return listingOf(n), nil }
	}
	for _, tc := range []serveCase{
		{
			name: "the first listing is charged one and settled to what it carried", processBurst: 100,
			answer: listing(30), wantHeldWhenAnswered: 99, wantLeft: 70, wantRemembered: 30, wantEntryLeft: 4,
		},
		{
			name: "a listing is charged what the last one carried before it is answered", processBurst: 100, remembered: 30,
			answer: listing(30), wantHeldWhenAnswered: 70, wantLeft: 70, wantRemembered: 30, wantEntryLeft: 4,
		},
		{
			name: "a listing carrying fewer tools than it was charged is not refunded the rest", processBurst: 100, remembered: 30,
			answer: listing(10), wantHeldWhenAnswered: 70, wantLeft: 70, wantRemembered: 10, wantEntryLeft: 4,
		},
		{
			name: "a listing the process bucket cannot cover is refused before the entry's bucket", processBurst: 10, spent: 8,
			remembered: 5, answer: listing(5), wantErr: listingRefusal, wantLeft: 2, wantRemembered: 5, wantEntryLeft: 5,
		},
		{
			name: "a listing the entry's bucket refuses gets its tools back", processBurst: 100, remembered: 7, entryDrained: true,
			answer: listing(7), wantErr: listingRefusal, wantLeft: 100, wantRemembered: 7, wantEntryLeft: 0,
		},
		{
			name: "a failed listing settles nothing and is not remembered", processBurst: 100,
			answer:               func() (mcp.Result, error) { return nil, errListingFailed },
			wantErr:              errListingFailed.Error(),
			wantHeldWhenAnswered: 99, wantLeft: 99, wantEntryLeft: 4,
		},
		{
			name: "a typed nil listing settles nothing", processBurst: 100,
			answer:               func() (mcp.Result, error) { return (*mcp.ListToolsResult)(nil), nil },
			wantHeldWhenAnswered: 99, wantLeft: 99, wantEntryLeft: 4,
		},
		{
			name: "an answer that is not a listing settles nothing", processBurst: 100,
			answer:               func() (mcp.Result, error) { return &mcp.CallToolResult{}, nil },
			wantHeldWhenAnswered: 99, wantLeft: 99, wantEntryLeft: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t, tc.run(t))
		})
	}
}

// TestCatalogListing_Learn_RemembersWhatTheServersOwnListingCarried pins what
// a listing the server makes of itself does to the process bucket's charge:
// the size of the listing is remembered, so a client's first listing is
// charged what it carries, the answer and its error are handed back as they
// were, and an answer that is not a listing teaches nothing. The bucket itself
// is never touched.
func TestCatalogListing_Learn_RemembersWhatTheServersOwnListingCarried(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		result         mcp.Result
		err            error
		wantRemembered int64
	}{
		{name: "a listing", result: listingOf(30), wantRemembered: 30},
		{name: "a failed listing", err: errListingFailed, wantRemembered: 7},
		{name: "a typed nil listing", result: (*mcp.ListToolsResult)(nil), wantRemembered: 7},
		{name: "an answer that is not a listing", result: &mcp.CallToolResult{}, wantRemembered: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			process := testProcessBucket(10)
			listings := &catalogListing{process: process}
			listings.tools.Store(7)
			result, err := listings.learn(tc.result, tc.err)
			if result != tc.result || !errors.Is(err, tc.err) {
				t.Errorf("learn handed back (%v, %v), want (%v, %v) unchanged", result, err, tc.result, tc.err)
			}
			if got := listings.tools.Load(); got != tc.wantRemembered {
				t.Errorf("the server is remembered to list %d tools, want %d", got, tc.wantRemembered)
			}
			if got := heldTools(process); got != 10 {
				t.Errorf("the process bucket holds %d tools, want the 10 it held: the server's own listing is charged nothing", got)
			}
		})
	}
}

// refusedListing is one listing refused by one of the two buckets, with what
// the client was answered and what the log said.
type refusedListing struct {
	err  error
	logs string
}

// refuseListing puts one listing through serve against a process bucket that
// is spent when processSpent and an entry bucket that is spent otherwise, and
// captures the log it writes. It swaps the default logger, so its callers do
// not run in parallel.
func refuseListing(t *testing.T, processSpent bool) refusedListing {
	t.Helper()
	logs := captureSlog(t)
	process := testProcessBucket(10)
	entry := &RateLimiter{limiter: rate.NewLimiter(1e-6, 1), scope: scopeCredential, throttleWindow: defaultThrottleWindow}
	if processSpent {
		process.debit(10)
	} else {
		entry.limiter.AllowN(time.Now(), 1)
	}
	listings := &catalogListing{process: process}
	listings.tools.Store(5)
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		t.Error("a listing both buckets were to refuse between them was answered")
		return listingOf(5), nil
	}
	_, err := listings.serve(t.Context(), methodToolsList, &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}}, next, entry)
	return refusedListing{err: err, logs: logs.String()}
}

// TestCatalogListing_EitherBucketRefusing_AnswersTheSameAndLogsWhichRefused
// verifies what each side of a refused listing is told. A client meets the
// same code and the same words from either bucket, since the next action is
// the same and a sentence naming the process would tell it that other callers
// are listing (INV-019). The operator is told which: the line the refusal
// writes names the bucket's scope, and the process's carries its own figures,
// counted in tools. That line is the one place the two refusals differ, so it
// is what this pins, through serve rather than by calling the reporter.
func TestCatalogListing_EitherBucketRefusing_AnswersTheSameAndLogsWhichRefused(t *testing.T) {
	for _, tc := range []struct {
		name         string
		processSpent bool
		wantLine     []string
		wantAbsent   []string
	}{
		{
			name: "the process's bucket", processSpent: true,
			wantLine: []string{
				`"msg":"listing refused: rate limit exceeded across the process"`,
				`"scope":"process"`, `"limit_tools_per_second":`, `"burst_tools":10`,
			},
			wantAbsent: []string{`"scope":"credential"`},
		},
		{
			name: "the entry's bucket", processSpent: false,
			wantLine:   []string{`"msg":"tool call refused: rate limit exceeded"`, `"scope":"credential"`, `"burst":1`},
			wantAbsent: []string{`"scope":"process"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := refuseListing(t, tc.processSpent)
			var rpcErr *jsonrpc.Error
			if !errors.As(got.err, &rpcErr) {
				t.Fatalf("serve = %v, want a JSON-RPC error", got.err)
			}
			if rpcErr.Code != tenancy.CodeTooManyRequests || rpcErr.Message != listingRefusal {
				t.Errorf("code %d, message %q; want %d, %q", rpcErr.Code, rpcErr.Message, tenancy.CodeTooManyRequests, listingRefusal)
			}
			if lines := strings.Count(got.logs, "rate limit exceeded"); lines != 1 {
				t.Errorf("%d refusal lines, want 1:\n%s", lines, got.logs)
			}
			for _, want := range tc.wantLine {
				assertContains(t, got.logs, want)
			}
			for _, unwanted := range tc.wantAbsent {
				assertNotContains(t, got.logs, unwanted)
			}
		})
	}
}

// TestAttachRateLimitFunc_ListingOverASession_IsChargedToTheProcessBucket drives
// the process's listing bucket through the middleware over a session: an empty
// one refuses a listing the entry's own bucket would allow, and it is consulted
// only where the entry's is, so a request no entry's bucket meters, and the
// server's own listings, are charged to neither. That is issue 951's decision
// that the process bucket follows RTC-003, and so is off when --rate-limit-rps
// is 0. The server's own listings are not charged, and they are what tells the
// bucket what a client's listing will cost before the first one arrives.
func TestAttachRateLimitFunc_ListingOverASession_IsChargedToTheProcessBucket(t *testing.T) {
	t.Parallel()
	emptied := func() *RateLimiter {
		process := testProcessBucket(1)
		process.debit(1)
		return process
	}

	t.Run("an empty process bucket refuses a listing the entry's would allow", func(t *testing.T) {
		t.Parallel()
		entry := NewRateLimiter(10, 40)
		session, ctx := connectClient(t, serverWith(emptied(), func() *RateLimiter { return entry }))
		_, err := session.ListTools(ctx, nil)
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != rateLimitedErrorCode || rpcErr.Message != listingRefusal {
			t.Fatalf("ListTools = %v, want the refusal %q", err, listingRefusal)
		}
		if got := heldTools(entry.forCatalog()); got != 40 {
			t.Errorf("the entry's listing bucket holds %d, want the 40 the process's refusal left it", got)
		}
	})

	t.Run("with no entry bucket the process bucket is not consulted", func(t *testing.T) {
		t.Parallel()
		session, ctx := connectClient(t, serverWith(emptied(), func() *RateLimiter { return nil }))
		if _, err := session.ListTools(ctx, nil); err != nil {
			t.Fatalf("a listing no entry's bucket meters was refused: %v", err)
		}
	})

	t.Run("the server's own listings are charged to neither", func(t *testing.T) {
		t.Parallel()
		server := serverWith(emptied(), func() *RateLimiter { return NewRateLimiter(10, 40) })
		if _, err := ListRegisteredTools(t.Context(), server, "inspection"); err != nil {
			t.Fatalf("the server's own listing was refused: %v", err)
		}
	})

	t.Run("a served listing costs the tools the server lists", func(t *testing.T) {
		t.Parallel()
		process := testProcessBucket(100)
		server := withTwoMore(serverWith(process, func() *RateLimiter { return NewRateLimiter(10, 40) }))
		session, ctx := connectClient(t, server)
		for range 2 {
			if _, err := session.ListTools(ctx, nil); err != nil {
				t.Fatalf("ListTools: %v", err)
			}
		}
		if got := heldTools(process); got != 94 {
			t.Errorf("the process bucket holds %d tools after two listings of three, want 94", got)
		}
	})
}

// serverWith is a server of one tool whose rate-limit middleware resolves every
// request to entry's bucket and charges listings to process.
func serverWith(process *RateLimiter, entry func() *RateLimiter) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	attachRateLimitFunc(server, func(context.Context) *RateLimiter { return entry() }, process)
	return server
}

// withTwoMore adds two tools to a server, so a listing of it carries three.
func withTwoMore(server *mcp.Server) *mcp.Server {
	for _, name := range []string{"second", "third"} {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: "Another tool to list."},
			func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{}, nil, nil
			})
	}
	return server
}

// TestAttachRateLimitFunc_ServersOwnListing_ChargesTheFirstClientListingItsSize
// pins what the server's own listing is for, beside being charged nothing: it
// tells the process's bucket how many tools this server lists before any
// client asks, so a client's first listing is charged what it carries up front
// rather than one tool settled afterwards. A bucket of five holding two cannot
// cover a listing of three and can cover the one tool a listing is charged
// when nothing taught it the size, so the first client listing is refused only
// if the server's own listing taught the bucket, and served into debt if it
// did not.
func TestAttachRateLimitFunc_ServersOwnListing_ChargesTheFirstClientListingItsSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		inspected bool
		wantErr   bool
		wantLeft  int
	}{
		{name: "after the server listed itself a client's first listing is charged what it carries", inspected: true, wantErr: true, wantLeft: 2},
		{name: "with nothing learned a client's first listing is charged one and settled", inspected: false, wantLeft: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			process := testProcessBucket(5)
			process.debit(3)
			server := withTwoMore(serverWith(process, func() *RateLimiter { return NewRateLimiter(10, 40) }))
			if tc.inspected {
				if _, err := ListRegisteredTools(t.Context(), server, "inspection"); err != nil {
					t.Fatalf("the server's own listing: %v", err)
				}
				if got := heldTools(process); got != 2 {
					t.Fatalf("the server's own listing left the bucket %d tools, want the 2 it held", got)
				}
			}
			session, ctx := connectClient(t, server)
			if _, err := session.ListTools(ctx, nil); (err != nil) != tc.wantErr {
				t.Errorf("the first client listing: %v, want refused %v", err, tc.wantErr)
			}
			if got := heldTools(process); got != tc.wantLeft {
				t.Errorf("the process bucket holds %d tools, want %d", got, tc.wantLeft)
			}
		})
	}
}
