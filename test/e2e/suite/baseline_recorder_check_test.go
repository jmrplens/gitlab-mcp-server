//go:build e2e

// baseline_recorder_check_test.go holds the recorder's own tests. They run
// inside the suite, against the same sessions, because that is the only
// process the recorder exists in: a baseline recorded by an attribution that
// names the wrong test is worse than none, and these are what say the names
// are right on the run that records it.

package suite

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestBaseline_Attribution_NamesTheRunningSubtest checks that the stack walk
// names the test exactly as the testing package does, through the shapes the
// suite uses: a top-level body, a subtest, a nested subtest, a repeated name,
// a helper closure declared outside the subtest that calls it, a goroutine,
// a table-driven loop whose names are not literals, and a cleanup.
//
// Every assertion is t.Name() against the walk, so the test is also what
// proves the source index reads this file the way the runtime runs it.
func TestBaseline_Attribution_NamesTheRunningSubtest(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, want string) {
		t.Helper()
		got := baselineOriginFromStack()
		if got.test != want {
			t.Errorf("attributed to %q, want %q", got.test, want)
		}
		if got.purpose != e2ecalls.PurposeTest {
			t.Errorf("purpose = %q, want %q", got.purpose, e2ecalls.PurposeTest)
		}
	}
	check(t, t.Name())

	t.Run("Sub", func(t *testing.T) {
		check(t, t.Name())
		t.Run("Nested", func(t *testing.T) { check(t, t.Name()) })
		t.Run("Nested", func(t *testing.T) { check(t, t.Name()) })
	})
	t.Run("Sub", func(t *testing.T) { check(t, t.Name()) })

	t.Run("Goroutine", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Go(func() { check(t, t.Name()) })
		wg.Wait()
	})

	t.Run("Cleanup", func(t *testing.T) {
		t.Cleanup(func() {
			got := baselineOriginFromStack()
			if got.test != t.Name() || got.purpose != e2ecalls.PurposeCleanup {
				t.Errorf("cleanup attributed to %q as %q, want %q as %q", got.test, got.purpose, t.Name(), e2ecalls.PurposeCleanup)
			}
		})
	})

	// Subtests a helper declares run on a goroutine with no Test frame at
	// all, which is the shape the pipeline lifecycles have: the walk has to
	// follow the goroutine's creator up to this function.
	runBaselineHelperSubtests(t, check)
	t.Run("Sub", func(t *testing.T) { runBaselineHelperSubtests(t, check) })

	// A name that is not a literal cannot be read off the source, so the
	// walk stops at the enclosing literal, which here is the Test function.
	cases := []string{"Table"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) { check(t, "TestBaseline_Attribution_NamesTheRunningSubtest") })
	}
}

// runBaselineHelperSubtests declares subtests from outside the Test function,
// nested two deep and with a goroutine inside, and checks each against its
// own name.
func runBaselineHelperSubtests(t *testing.T, check func(*testing.T, string)) {
	t.Helper()
	t.Run("HelperLeaf", func(t *testing.T) {
		check(t, t.Name())
		t.Run("Deeper", func(t *testing.T) {
			check(t, t.Name())
			var wg sync.WaitGroup
			wg.Go(func() { check(t, t.Name()) })
			wg.Wait()
		})
	})
}

// TestBaseline_Attribution_ReadsTheContextFirst checks that an origin set on
// the context wins over the stack, which is how the ledger's cleanups are
// attributed.
func TestBaseline_Attribution_ReadsTheContextFirst(t *testing.T) {
	t.Parallel()
	ctx := withBaselineOrigin(context.Background(), "TestElsewhere", e2ecalls.PurposeCleanup)
	got := baselineOriginFor(ctx)
	if got.test != "TestElsewhere" || got.purpose != e2ecalls.PurposeCleanup {
		t.Errorf("origin = %+v, want TestElsewhere as cleanup", got)
	}
	if bare := baselineOriginFor(context.Background()); bare.test != t.Name() {
		t.Errorf("a context with no origin fell back to %q, want %q", bare.test, t.Name())
	}
}

// TestBaseline_SubtestIndex_ParsesThisFile checks the source index against
// this file: the literals of the attribution test above, in order, with the
// repeated names numbered the way the testing package numbers them.
func TestBaseline_SubtestIndex_ParsesThisFile(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller gave no file")
	}
	parsed, err := parseBaselineSubtests(baselineSourcePath(file))
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	cases := []struct {
		function string
		want     []string
	}{
		{function: "TestBaseline_Attribution_NamesTheRunningSubtest", want: []string{"Sub", "Nested", "Nested#01", "Sub#01", "Goroutine", "Cleanup", "Sub#02"}},
		{function: "runBaselineHelperSubtests", want: []string{"HelperLeaf", "Deeper"}},
	}
	for _, tc := range cases {
		t.Run(tc.function, func(t *testing.T) {
			literals := parsed[tc.function]
			if len(literals) != len(tc.want) {
				t.Fatalf("found %d literals, want %d: %+v", len(literals), len(tc.want), literals)
			}
			for i, literal := range literals {
				if literal.name != tc.want[i] {
					t.Errorf("literal %d = %q, want %q", i, literal.name, tc.want[i])
				}
				if literal.start <= 0 || literal.end < literal.start {
					t.Errorf("literal %q spans lines %d to %d", literal.name, literal.start, literal.end)
				}
			}
		})
	}
}

// TestBaseline_DeclaredFunctionName_StripsWhatTheRuntimeAppends checks the
// runtime's closure, wrapper, range and instantiation suffixes come off a
// frame's function name and nothing the source declared does.
func TestBaseline_DeclaredFunctionName_StripsWhatTheRuntimeAppends(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "helper", want: "helper"},
		{name: "closure", in: "TestX.func3", want: "TestX"},
		{name: "nested closure", in: "TestX.func3.1", want: "TestX"},
		{name: "defer wrapper", in: "helper.deferwrap1", want: "helper"},
		{name: "go wrapper", in: "helper.func1.gowrap2", want: "helper"},
		{name: "range body", in: "helper-range1", want: "helper"},
		{name: "generic", in: "callToolOn[...]", want: "callToolOn"},
		{name: "method", in: "(*E2EContext).Meta", want: "(*E2EContext).Meta"},
		{name: "method closure", in: "(*E2EContext).Meta.func1", want: "(*E2EContext).Meta"},
		{name: "value method", in: "ResourceRecord.redactedLabel", want: "ResourceRecord.redactedLabel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaredFunctionName(tc.in); got != tc.want {
				t.Errorf("declaredFunctionName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBaseline_ParseGoroutineDump_ReadsFramesAndCreators checks the dump
// parser on the shape runtime.Stack prints: the first block is the caller,
// frames pair a function line with a location line, and "created by" names
// the creating goroutine.
func TestBaseline_ParseGoroutineDump_ReadsFramesAndCreators(t *testing.T) {
	t.Parallel()
	dump := "goroutine 18 [running]:\n" +
		"github.com/x/suite.helper.func1(0xc000, {0x1, 0x2})\n" +
		"\t/src/suite/a_test.go:42 +0x1c\n" +
		"testing.tRunner(0xc000, 0xc001)\n" +
		"\t/usr/local/go/src/testing/testing.go:1700 +0x3e5\n" +
		"created by testing.(*T).Run in goroutine 7\n" +
		"\t/usr/local/go/src/testing/testing.go:1750 +0x3e5\n" +
		"\n" +
		"goroutine 7 [chan receive]:\n" +
		"github.com/x/suite.TestX(0xc000)\n" +
		"\t/src/suite/a_test.go:10 +0x1c\n" +
		"created by testing.(*T).Run in goroutine 1\n" +
		"\t/usr/local/go/src/testing/testing.go:1750 +0x3e5\n"
	stacks, current := parseGoroutineDump([]byte(dump))
	if current != 18 {
		t.Errorf("current goroutine = %d, want 18", current)
	}
	child, known := stacks[18]
	if !known || child.createdBy != 7 || len(child.frames) != 2 {
		t.Fatalf("goroutine 18 = %+v, want two frames created by 7", child)
	}
	if child.frames[0].function != "github.com/x/suite.helper.func1" || child.frames[0].file != "/src/suite/a_test.go" || child.frames[0].line != 42 {
		t.Errorf("first frame = %+v", child.frames[0])
	}
	parent, known := stacks[7]
	if !known || parent.createdBy != 1 || len(parent.frames) != 1 || parent.frames[0].function != "github.com/x/suite.TestX" {
		t.Errorf("goroutine 7 = %+v", parent)
	}
}

// TestBaseline_MergeNameParts_DropsTheSharedLiteral checks that a literal
// both a creator and its goroutine sit inside is spelled once.
func TestBaseline_MergeNameParts_DropsTheSharedLiteral(t *testing.T) {
	t.Parallel()
	root := baselineNamePart{name: "TestX"}
	sub := baselineNamePart{file: "a_test.go", start: 10, name: "Sub"}
	leaf := baselineNamePart{file: "a_test.go", start: 20, name: "Leaf"}
	merged := mergeNameParts([]baselineNamePart{root, sub}, []baselineNamePart{sub, leaf})
	if got := joinNameParts(merged); got != "TestX/Sub/Leaf" {
		t.Errorf("merged = %q, want TestX/Sub/Leaf", got)
	}
}

// TestBaseline_RewriteSubtestName_SpellsNamesLikeTheTestingPackage checks
// the name rewrite on the two substitutions the testing package makes.
func TestBaseline_RewriteSubtestName_SpellsNamesLikeTheTestingPackage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "ApprovalState", want: "ApprovalState"},
		{name: "space", in: "approval state", want: "approval_state"},
		{name: "tab is a space", in: "tab\there", want: "tab_here"},
		{name: "unprintable", in: "bell\x07here", want: `bell\ahere`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewriteSubtestName(tc.in); got != tc.want {
				t.Errorf("rewriteSubtestName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBaseline_Outcome_ClassifiesLikeTheHarness checks the outcome
// classification on one answer of each class, since the baseline is compared
// with the rebuilt suite's record credit by credit.
func TestBaseline_Outcome_ClassifiesLikeTheHarness(t *testing.T) {
	t.Parallel()
	toolResult := func(isError bool, text string) *mcp.CallToolResult {
		return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
	}
	cases := []struct {
		name   string
		method string
		result mcp.Result
		err    error
		want   string
	}{
		{name: "ok", method: baselineMethodCallTool, result: toolResult(false, "{}"), want: e2ecalls.OutcomeOK},
		{name: "tool error", method: baselineMethodCallTool, result: toolResult(true, "gitlab said no"), want: e2ecalls.OutcomeToolError},
		{name: "needs confirmation", method: baselineMethodCallTool, result: toolResult(true, "Re-send with confirm=true"), want: e2ecalls.RefusedOutcome(toolutil.RefusalNeedsConfirmation)},
		{name: "unknown action", method: baselineMethodCallTool, result: toolResult(true, "unknown action 'x'"), want: e2ecalls.RefusedOutcome(toolutil.RefusalUnknownAction)},
		{name: "invalid params", method: baselineMethodCallTool, result: toolResult(true, "project_id is required for this action"), want: e2ecalls.RefusedOutcome(toolutil.RefusalInvalidParams)},
		{name: "not found", method: baselineMethodCallTool, result: toolResult(true, "404 Not Found"), want: e2ecalls.RefusedOutcome("not_found")},
		{name: "forbidden", method: baselineMethodCallTool, result: toolResult(true, "403 Forbidden"), want: e2ecalls.RefusedOutcome("forbidden")},
		{name: "protocol error", method: baselineMethodCallTool, err: errors.New("invalid params"), want: e2ecalls.OutcomeProtocolError},
		{name: "transport error", method: baselineMethodCallTool, err: errors.New("read: connection reset by peer"), want: e2ecalls.OutcomeTransportError},
		{name: "no result", method: baselineMethodCallTool, want: e2ecalls.OutcomeProtocolError},
		{name: "resource read", method: baselineMethodReadResource, result: &mcp.ReadResourceResult{}, want: e2ecalls.OutcomeOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := baselineOutcome(tc.method, tc.result, tc.err); got != tc.want {
				t.Errorf("baselineOutcome = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBaseline_Arguments_NamesTypedAndUntypedInputs checks that argument
// names come out the same whether a test passed a map or a typed input.
func TestBaseline_Arguments_NamesTypedAndUntypedInputs(t *testing.T) {
	t.Parallel()
	type typed struct {
		ProjectID string `json:"project_id"`
		Title     string `json:"title,omitempty"`
	}
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{name: "map", in: map[string]any{"title": "x", "project_id": "1"}, want: []string{"project_id", "title"}},
		{name: "typed", in: typed{ProjectID: "1"}, want: []string{"project_id"}},
		{name: "nil", in: nil, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, names := baselineArguments(tc.in)
			if tc.in != nil && len(raw) == 0 {
				t.Error("no JSON came back for a non-nil input")
			}
			if len(names) != len(tc.want) {
				t.Fatalf("names = %v, want %v", names, tc.want)
			}
			for i := range names {
				if names[i] != tc.want[i] {
					t.Errorf("names = %v, want %v", names, tc.want)
				}
			}
		})
	}
}

// TestBaseline_Recorder_JoinsTheDispatchedAction drives one read through the
// meta session and reads the line the recorder built for it: the test is
// this one, the action is the one the call named, and the dispatched action
// is what the server's own span said, joined on the trace the client stamped.
//
// This is the end-to-end check of the record on the run that produces it.
// The observer sees the line whether or not a shard is being written, so the
// test holds in an ordinary run too.
func TestBaseline_Recorder_JoinsTheDispatchedAction(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	var (
		mu    sync.Mutex
		lines []*e2ecalls.Call
	)
	remove := baseline.observe(func(call *e2ecalls.Call) {
		if call.Test != t.Name() {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, call)
	})
	defer remove()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	requireNoError(t, callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{"action": "current"}), "user current")

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 1 {
		t.Fatalf("recorded %d lines for this test, want 1", len(lines))
	}
	line := lines[0]
	type recorded struct {
		method, tool, action, dispatched, outcome, purpose, session, surface, mode string
	}
	want := recorded{
		method: baselineMethodCallTool, tool: "gitlab_user", action: "user.current", dispatched: "user.current",
		outcome: e2ecalls.OutcomeOK, purpose: e2ecalls.PurposeTest,
		session: shapeMeta.label, surface: shapeMeta.surface, mode: shapeMeta.mode,
	}
	got := recorded{
		method: line.Method, tool: line.Tool, action: line.Action, dispatched: line.Dispatched,
		outcome: line.Outcome, purpose: line.Purpose,
		session: line.Session, surface: line.Surface, mode: line.Mode,
	}
	if got != want {
		t.Errorf("recorded line = %+v, want %+v", got, want)
	}
	if line.TraceID == "" {
		t.Error("the call carries no trace id, so nothing could have joined its span")
	}
	if len(line.Arguments) != 1 || line.Arguments[0] != "action" {
		t.Errorf("arguments = %v, want [action]", line.Arguments)
	}
	if _, seen := baseline.spans.lookup(line.TraceID); !seen {
		t.Error("the server span for this call never reached the processor")
	}
}
