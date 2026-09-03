// logging_test.go contains unit tests for the LogToolCall, LogToolCallAll,
// and logToolCallWithUser helpers. Tests capture slog output and assert that
// the correct log level, tool name, and structured fields are present.
package toolutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
)

// captureSlog redirects slog to a buffer for the duration of the test.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// assertContains fails if the output does not contain the expected substring.
func assertContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Errorf("output missing %q, got:\n%s", want, output)
	}
}

// assertNotContains fails if the output unexpectedly contains the substring.
func assertNotContains(t *testing.T, output, unwanted string) {
	t.Helper()
	if strings.Contains(output, unwanted) {
		t.Errorf("output should not contain %q, got:\n%s", unwanted, output)
	}
}

// TestLogToolCall_Success verifies that logToolCall logs an INFO message
// with the tool name and duration for a successful call (nil error).
func TestLogToolCall_Success(t *testing.T) {
	buf := captureSlog(t)
	logToolCall(context.Background(), "test_tool", time.Now(), false, nil)
	out := buf.String()
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"msg":"tool call completed"`)
	assertContains(t, out, `"tool":"test_tool"`)
	assertContains(t, out, `"duration"`)
	assertNotContains(t, out, `"error"`)
}

// TestLogToolCall_Error verifies that logToolCall logs an ERROR message
// with the tool name, duration, and error details for a failed call.
func TestLogToolCall_Error(t *testing.T) {
	buf := captureSlog(t)
	logToolCall(context.Background(), "test_tool", time.Now(), false, errors.New("something failed"))
	out := buf.String()
	assertContains(t, out, `"level":"ERROR"`)
	assertContains(t, out, `"msg":"tool call failed"`)
	assertContains(t, out, `"tool":"test_tool"`)
	assertContains(t, out, `"error":"something failed"`)
}

// TestLogToolCallAll_NilRequest verifies that LogToolCallAll handles a nil
// CallToolRequest for both success and error paths, logging the correct level.
func TestLogToolCallAll_NilRequest(t *testing.T) {
	buf := captureSlog(t)
	ctx := context.Background()

	LogToolCallAll(ctx, nil, "nil_req_tool", time.Now(), nil, nil)
	LogToolCallAll(ctx, nil, "nil_req_tool", time.Now(), nil, errors.New("err"))

	out := buf.String()
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"level":"ERROR"`)
	assertContains(t, out, `"tool":"nil_req_tool"`)
}

// TestLogToolCallAll_WithRequest verifies that LogToolCallAll handles
// a non-nil request without a session, logging the correct fields.
func TestLogToolCallAll_WithRequest(t *testing.T) {
	buf := captureSlog(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	LogToolCallAll(ctx, req, "req_tool", time.Now(), nil, nil)
	LogToolCallAll(ctx, req, "req_tool", time.Now(), nil, errors.New("err"))

	out := buf.String()
	assertContains(t, out, `"tool":"req_tool"`)
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"level":"ERROR"`)
}

// TestLogToolCallAll_WithAuthenticatedUser verifies that LogToolCallAll
// routes to logToolCallWithUser when an authenticated identity is present,
// including user and user_id fields in the log output.
func TestLogToolCallAll_WithAuthenticatedUser(t *testing.T) {
	buf := captureSlog(t)
	identity := UserIdentity{UserID: "123", Username: "testuser"}
	ctx := IdentityToContext(context.Background(), identity)

	LogToolCallAll(ctx, nil, "user_tool", time.Now(), nil, nil)
	LogToolCallAll(ctx, nil, "user_tool", time.Now(), nil, errors.New("test error"))

	out := buf.String()
	assertContains(t, out, `"tool":"user_tool"`)
	assertContains(t, out, `"user":"testuser"`)
	assertContains(t, out, `"user_id":"123"`)
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"level":"ERROR"`)
}

// TestLogToolCallWithUser_Success verifies that logToolCallWithUser logs
// an INFO message with user and user_id fields for a successful call.
func TestLogToolCallWithUser_Success(t *testing.T) {
	buf := captureSlog(t)
	user := UserIdentity{UserID: "42", Username: "admin"}
	logToolCallWithUser(context.Background(), "user_success_tool", time.Now(), false, nil, user)

	out := buf.String()
	assertContains(t, out, `"level":"INFO"`)
	assertContains(t, out, `"msg":"tool call completed"`)
	assertContains(t, out, `"tool":"user_success_tool"`)
	assertContains(t, out, `"user":"admin"`)
	assertContains(t, out, `"user_id":"42"`)
	assertNotContains(t, out, `"error"`)
}

// TestLogToolCallWithUser_Error verifies that logToolCallWithUser logs
// an ERROR message with user, user_id, and error fields for a failed call.
func TestLogToolCallWithUser_Error(t *testing.T) {
	buf := captureSlog(t)
	user := UserIdentity{UserID: "42", Username: "admin"}
	logToolCallWithUser(context.Background(), "user_error_tool", time.Now(), false, errors.New("api failure"), user)

	out := buf.String()
	assertContains(t, out, `"level":"ERROR"`)
	assertContains(t, out, `"msg":"tool call failed"`)
	assertContains(t, out, `"tool":"user_error_tool"`)
	assertContains(t, out, `"user":"admin"`)
	assertContains(t, out, `"user_id":"42"`)
	assertContains(t, out, `"error":"api failure"`)
}

// TestCancellation_IsNotReportedAsAFailure pins how a call that ended because
// its caller went away is described.
//
// "Implementations SHOULD log cancellation reasons for debugging." A client
// canceling, or a deadline it set expiring, is the protocol working. Both used
// to be logged at ERROR as "tool call failed" and classified to the model as an
// "unexpected error", which is untrue twice over: nothing failed, and there is
// nothing for either the operator or the model to check.
//
// The reason string the client sent cannot be included, and that is not ours to
// fix: go-sdk reads CancelledParams.RequestID and discards Reason before any
// application code runs. What is left is the outcome and the elapsed time.
func TestCancellation_IsNotReportedAsAFailure(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantSemantic string
	}{
		{
			name:         "a client canceling",
			err:          context.Canceled,
			wantSemantic: "canceled by the client",
		},
		{
			name:         "a deadline expiring",
			err:          context.DeadlineExceeded,
			wantSemantic: "exceeded its deadline",
		},
		{
			name: "a cancellation wrapped by the layer that noticed it",
			// What a canceled call usually looks like by the time it is
			// classified: the transport failure is the symptom, and every
			// other branch would describe that instead.
			err:          fmt.Errorf("performing request: %w", context.Canceled),
			wantSemantic: "canceled by the client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if !strings.Contains(got, tt.wantSemantic) {
				t.Errorf("ClassifyError() = %q, want it to mention %q", got, tt.wantSemantic)
			}
			if strings.Contains(got, "unexpected error") {
				t.Error("a cancellation was described as an unexpected error")
			}
		})
	}

	t.Run("a real failure is still a failure", func(t *testing.T) {
		got := ClassifyError(errors.New("something actually broke"))
		if !strings.Contains(got, "unexpected error") {
			t.Errorf("ClassifyError() = %q, want the unexpected-error wording preserved", got)
		}
	})
}

// TestWasCancelled_SeparatesCallerDepartureFromFailure checks the predicate the
// log level turns on.
func TestWasCancelled_SeparatesCallerDepartureFromFailure(t *testing.T) {
	canceled := []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline_exceeded", err: context.DeadlineExceeded},
		{name: "wrapped_canceled", err: fmt.Errorf("listing issues: %w", context.Canceled)},
	}
	for _, tc := range canceled {
		t.Run(tc.name, func(t *testing.T) {
			if !wasCancelled(tc.err) {
				t.Errorf("wasCancelled(%v) = false, want true", tc.err)
			}
		})
	}

	failures := []struct {
		name string
		err  error
	}{
		{name: "nil_error", err: nil},
		{name: "plain_failure", err: errors.New("boom")},
		{name: "wrapped_failure", err: fmt.Errorf("listing issues: %w", errors.New("500 from GitLab"))},
	}
	for _, tc := range failures {
		t.Run(tc.name, func(t *testing.T) {
			if wasCancelled(tc.err) {
				t.Errorf("wasCancelled(%v) = true; a real failure would be logged at INFO and lost", tc.err)
			}
		})
	}
}

// TestLogToolCallAll_ErrorResultIsNotLoggedAsSuccess pins the distinction an
// operator needs and did not have.
//
// A handler can report failure two ways: by returning a Go error, which was
// logged at ERROR, or by returning a result with IsError set, which was logged
// as "tool call completed" with nothing to tell it apart from a call that
// worked. The second is not a rare path: NotFoundResult takes it at eighteen
// call sites, so every 404 this server turns into a helpful message was counted
// as a success. An error rate could not be computed from this stream, because
// the stream did not contain one.
func TestLogToolCallAll_ErrorResultIsNotLoggedAsSuccess(t *testing.T) {
	notFound := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "## Project Not Found"}}}

	t.Run("anonymous", func(t *testing.T) {
		buf := captureSlog(t)
		LogToolCallAll(context.Background(), nil, "gitlab_project_get", time.Now(), notFound, nil)

		out := buf.String()
		assertContains(t, out, `"msg":"tool call completed"`)
		assertContains(t, out, `"is_error":true`)
	})

	t.Run("with an identity", func(t *testing.T) {
		buf := captureSlog(t)
		ctx := IdentityToContext(context.Background(), UserIdentity{UserID: "7", Username: "someone"})
		LogToolCallAll(ctx, nil, "gitlab_project_get", time.Now(), notFound, nil)

		out := buf.String()
		assertContains(t, out, `"is_error":true`)
		assertContains(t, out, `"user":"someone"`)
	})

	t.Run("a call that worked says so", func(t *testing.T) {
		buf := captureSlog(t)
		LogToolCallAll(context.Background(), nil, "gitlab_project_get", time.Now(), &mcp.CallToolResult{}, nil)

		assertContains(t, buf.String(), `"is_error":false`)
	})
}

// TestLogToolRefusal_RecordsWhatWasDeclinedAndWhy pins the line the silent
// paths now write.
//
// Safe-mode previews, unknown actions, missing parameters and unconfirmed
// destructive actions all returned an error result to the model and wrote
// nothing at all: they return before reaching LogToolCallAll. A deployment
// refusing every third call because its clients have not learned the parameter
// shape looked identical to a healthy one. ADR-0011 IMP-008 accepted this
// ("observability for ... validation failure, policy block, and destructive
// confirmation events") and only the destructive half was delivered.
func TestLogToolRefusal_RecordsWhatWasDeclinedAndWhy(t *testing.T) {
	buf := captureSlog(t)
	ctx := IdentityToContext(context.Background(), UserIdentity{UserID: "7", Username: "someone"})

	LogToolRefusal(ctx, nil, "gitlab_project/delete", RefusalNeedsConfirmation)
	LogToolRefusal(context.Background(), nil, "gitlab_execute_action/nope.nope", RefusalUnknownAction)

	out := buf.String()
	assertContains(t, out, `"msg":"tool call refused"`)
	assertContains(t, out, `"reason":"needs_confirmation"`)
	assertContains(t, out, `"tool":"gitlab_project/delete"`)
	assertContains(t, out, `"user":"someone"`)
	assertContains(t, out, `"reason":"unknown_action"`)
	// A refusal is the protocol working, not a failure. Logging it at ERROR
	// would fill an operator's dashboard with entries nobody can act on.
	assertNotContains(t, out, `"level":"ERROR"`)
}

// TestLogToolCall_InstanceNamesWhichGitLabAnsweredIt pins the field that makes
// an audit line unambiguous, and pins that it is absent when it would say
// nothing.
//
// A GitLab user id is unique within an instance and means nothing across them,
// so a deployment publishing several through --gitlab-url logs two different
// people as user_id 7 with nothing to tell them apart. ADR-0008 fixes the
// identity as {UserID, Username} and motivates it expressly for audit logging
// without recording that limit, which is the one place an ambiguous subject
// matters.
//
// The absent case is the other half of the claim: stdio resolves one identity
// against one instance, and a pinned deployment has only the one, so emitting
// the field there would repeat a constant on every line instead of giving
// something to group by.
func TestLogToolCall_InstanceNamesWhichGitLabAnsweredIt(t *testing.T) {
	tests := []struct {
		name     string
		identity UserIdentity
		want     bool
	}{
		{
			name:     "several instances published, so the id needs qualifying",
			identity: UserIdentity{UserID: "7", Username: "someone", Instance: "https://gitlab.example.com"},
			want:     true,
		},
		{
			name:     "one instance, so there is nothing to distinguish",
			identity: UserIdentity{UserID: "7", Username: "someone"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureSlog(t)
			ctx := IdentityToContext(context.Background(), tt.identity)

			LogToolCallAll(ctx, nil, "gitlab_project_get", time.Now(), nil, nil)
			LogToolRefusal(ctx, nil, "gitlab_project_delete", RefusalNeedsConfirmation)

			out := buf.String()
			if got := strings.Contains(out, `"instance":"https://gitlab.example.com"`); got != tt.want {
				t.Errorf("instance present = %v, want %v:\n%s", got, tt.want, out)
			}
			// The fields that were always there must still be, on both records.
			if strings.Count(out, `"user_id":"7"`) != 2 {
				t.Errorf("the identity is missing from one of the two records:\n%s", out)
			}
		})
	}
}

// capturingExporter keeps the log records the SDK exports so the test can read
// what a collector would have received.
type capturingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *capturingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, records...)
	return nil
}

func (e *capturingExporter) Shutdown(context.Context) error   { return nil }
func (e *capturingExporter) ForceFlush(context.Context) error { return nil }

func (e *capturingExporter) all() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// TestLogToolCallAll_InsideASpan_ExportsACorrelatedRecord is the regression that
// a handler-level test could not be.
//
// The fan-out handler was always correct: hand it a context carrying a span and
// it exports a correlated record. What was wrong was every caller, because the
// server logged through the context-free slog.Info family, and
// LogToolCallAll in particular accepted a context and then dropped it on the
// floor by delegating to helpers that took none.
//
// Nothing in the package's own tests could see that. The bridge passed, the
// records were exported, the log stream on stderr was unchanged, and a
// collector receiving real traffic showed 235 records with not one trace ID
// among them. So the assertion here deliberately goes through the exported
// entry point a tool handler actually calls, rather than through the handler
// the previous test covers.
func TestLogToolCallAll_InsideASpan_ExportsACorrelatedRecord(t *testing.T) {
	exp := &capturingExporter{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	previous := global.GetLoggerProvider()
	global.SetLoggerProvider(lp)
	t.Cleanup(func() { global.SetLoggerProvider(previous) })

	handler := telemetry.NewSlogHandler(slog.NewJSONHandler(io.Discard, nil), slog.LevelInfo, nil)
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previousDefault) })

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "tools/call gitlab_execute_action")
	LogToolCallAll(ctx, nil, "gitlab_execute_action", time.Now(), nil, nil)
	span.End()

	records := exp.all()
	if len(records) != 1 {
		t.Fatalf("exported %d records, want 1", len(records))
	}

	want := span.SpanContext().TraceID()
	if got := records[0].TraceID(); got != want {
		t.Errorf("record trace ID = %v, want %v: a tool call log cannot be joined to the span it happened inside", got, want)
	}
}

// TestRefusalReasons_EveryOneIsDocumented ties the closed set to the guide that
// calls it closed.
//
// The guide tells an operator these five values are the whole set and that
// grouping a dashboard by them is safe. That claim is true today and nothing
// kept it true: a sixth constant added next year would silently give the metric
// a value no dashboard filters on and no page mentions.
//
// The constants are read out of the source rather than listed here, because a
// list here would be the same drift one file further along.
func TestRefusalReasons_EveryOneIsDocumented(t *testing.T) {
	t.Parallel()

	reasons := refusalConstants(t)
	if len(reasons) < 5 {
		t.Fatalf("found %d refusal constants, want at least the five this package declares; the parse is probably wrong",
			len(reasons))
	}

	guide := readTelemetryGuide(t)
	for name, value := range reasons {
		if !strings.Contains(guide, "`"+value+"`") {
			t.Errorf("%s = %q is a refusal this server can record and the telemetry guide never names it, so an operator cannot filter on it",
				name, value)
		}
	}
}

// refusalConstants returns every Refusal* constant declared in this package,
// by name and value.
func refusalConstants(t *testing.T) map[string]string {
	t.Helper()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "logging.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing logging.go: %v", err)
	}

	out := map[string]string{}
	ast.Inspect(parsed, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "Refusal") || i >= len(spec.Values) {
				continue
			}
			literal, isLiteral := spec.Values[i].(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				continue
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil {
				t.Fatalf("unquoting %s: %v", name.Name, unquoteErr)
			}
			out[name.Name] = value
		}
		return true
	})
	return out
}

// readTelemetryGuide returns the operator guide, located from this package
// rather than from a working directory a test runner chooses.
func readTelemetryGuide(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", "docs", "guides", "telemetry.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the telemetry guide: %v", err)
	}
	return string(content)
}

// TestLogToolCallAll_FailureLineCarriesNoUpstreamBodyOrTerminalEscapes pins the
// "tool call failed" line directly rather than by inference.
//
// The line logs the wrapped error value, so it inherits whatever sanitize left
// on the way through the wrapping helpers, and until now the only proof of that
// was TestWrapErrWithMessage_DoesNotReflectUpstreamResponseBody asserting on the
// error's own text one layer down. That is the property this line depends on,
// not a property this line has: a change that rendered the cause a second way
// here, or logged the unwrapped error alongside it, would restore the
// reflection with that test still green.
//
// Both halves matter and they fail differently. The upstream body is whatever
// answered between this server and GitLab, so on a pinned deployment it is a
// proxy or WAF page carrying internal hostnames. The escape sequence stops
// being text the moment the record reaches a terminal, which stderr on stdio
// always is.
//
// The assertion reads the decoded JSON field rather than the raw buffer: slog's
// JSON handler escapes a control byte to a six-character sequence on its own,
// so searching the buffer would pass whether or not anything had been stripped.
func TestLogToolCallAll_FailureLineCarriesNoUpstreamBodyOrTerminalEscapes(t *testing.T) {
	const (
		sentinel = "internal-host-secret-4c1d"
		tail     = "tail-marker-8ae2"
		escape   = "\x1b[2J\x1b]0;window-title\a"
	)
	body := "<html><body>" + escape + sentinel + strings.Repeat(" filler", 60) + tail + "</body></html>"
	upstream := &gl.ErrorResponse{
		StatusCode: http.StatusBadGateway,
		Response: &http.Response{
			StatusCode: http.StatusBadGateway,
			Request:    &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "gitlab.example.com", Path: "/api/v4/projects/1"}},
		},
		Body:    []byte(body),
		Message: "failed to parse unknown error format: " + body,
	}

	buf := captureSlog(t)
	LogToolCallAll(context.Background(), nil, "gitlab_project_update", time.Now(),
		nil, WrapErrWithMessage("projectUpdate", upstream))

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("decoding the log record: %v (raw: %s)", err, buf.String())
	}
	if got, want := record["msg"], "tool call failed"; got != want {
		t.Fatalf("msg = %v, want %q", got, want)
	}
	logged, ok := record["error"].(string)
	if !ok {
		t.Fatalf("error field = %#v, want a string", record["error"])
	}
	unwanted := []struct{ name, value string }{
		{"upstream body sentinel", sentinel},
		{"upstream body tail", tail},
		{"upstream markup", "<html>"},
		{"escape byte", "\x1b"},
		{"bell byte", "\a"},
	}
	for _, u := range unwanted {
		t.Run(u.name, func(t *testing.T) {
			if strings.Contains(logged, u.value) {
				t.Errorf("%s reached the failure log line: %q", u.name, logged)
			}
		})
	}
}
