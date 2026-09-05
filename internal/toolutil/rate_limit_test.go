// rate_limit_test.go verifies the token-bucket rate limiter and the MCP
// receiving middleware that converts over-budget tools/call requests into
// structured tool error results.
package toolutil

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/time/rate"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
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

// TestAttachRateLimit_ListNotGated verifies that tools/list bypasses the
// limiter so clients can always discover available tools regardless of
// burst exhaustion.
func TestAttachRateLimit_ListNotGated(t *testing.T) {
	t.Parallel()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerEchoTool(server)
	AttachRateLimit(server, NewRateLimiter(1, 1))

	session, ctx := connectClient(t, server)
	for i := range 5 {
		if _, err := session.ListTools(ctx, nil); err != nil {
			t.Fatalf("ListTools #%d: %v", i+1, err)
		}
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
				`"limit_rps":1`,
				`"burst":1`,
			},
		},
		{
			name:  "a flood inside the window is counted, not logged",
			build: func() *RateLimiter { return NewRateLimiter(10, 40) },
			refuse: func(r *RateLimiter) {
				for range 102 {
					r.reportRefusal(context.Background(), "gitlab_execute_action")
				}
			},
			wantLines: 1,
		},
		{
			name: "the next window reports what the last one absorbed",
			build: func() *RateLimiter {
				r := NewRateLimiter(10, 40)
				r.throttleWindow = time.Millisecond
				return r
			},
			refuse: func(r *RateLimiter) {
				r.reportRefusal(context.Background(), "gitlab_execute_action")
				for range 41 {
					r.reportRefusal(context.Background(), "gitlab_execute_action")
				}
				time.Sleep(5 * time.Millisecond)
				r.reportRefusal(context.Background(), "gitlab_execute_action")
			},
			wantLines: 2,
			// Without the count, an operator reading one line per ten seconds
			// has no idea whether it stands for one refusal or a thousand.
			wantContains: []string{`"also_refused_since_last_report":41`},
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
			// A limiter built without going through NewRateLimiter, which the
			// scaled() completion limiter is, must not divide by an unset
			// window.
			name:  "a zero window falls back to the default",
			build: func() *RateLimiter { return &RateLimiter{limiter: rate.NewLimiter(1, 1)} },
			refuse: func(r *RateLimiter) {
				r.reportRefusal(context.Background(), "gitlab_execute_action")
				r.reportRefusal(context.Background(), "gitlab_execute_action")
			},
			wantLines: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureSlog(t)
			tt.refuse(tt.build())

			out := buf.String()
			if got := strings.Count(out, "rate limit exceeded"); got != tt.wantLines {
				t.Fatalf("%d refusal lines, want %d:\n%s", got, tt.wantLines, out)
			}
			for _, want := range tt.wantContains {
				assertContains(t, out, want)
			}
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
	AttachRateLimit(server, NewRateLimiter(1, 1))

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
	}{
		{"shallow", 3, 1, false},
		{"at_the_limit", 8, 1, false},
		{"one_over_the_limit", 9, 0, true},
		{"far_over_the_limit", 4000, 0, true},
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
				if !strings.Contains(err.Error(), "nest") {
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
	buckets := map[string]*RateLimiter{
		"noisy": NewRateLimiter(1, 1),
		"quiet": NewRateLimiter(1, 1),
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
func TestAttachRateLimitFunc_NothingToInstall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		server  *mcp.Server
		resolve func(context.Context) *RateLimiter
	}{
		{name: "no server", resolve: func(context.Context) *RateLimiter { return nil }},
		{name: "no resolver", server: mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			AttachRateLimitFunc(tc.server, tc.resolve)
		})
	}
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
