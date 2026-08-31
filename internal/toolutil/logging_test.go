// logging_test.go contains unit tests for the LogToolCall, LogToolCallAll,
// and logToolCallWithUser helpers. Tests capture slog output and assert that
// the correct log level, tool name, and structured fields are present.
package toolutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	canceled := []error{
		context.Canceled,
		context.DeadlineExceeded,
		fmt.Errorf("listing issues: %w", context.Canceled),
	}
	for _, err := range canceled {
		if !wasCancelled(err) {
			t.Errorf("wasCancelled(%v) = false, want true", err)
		}
	}

	failures := []error{
		nil,
		errors.New("boom"),
		fmt.Errorf("listing issues: %w", errors.New("500 from GitLab")),
	}
	for _, err := range failures {
		if wasCancelled(err) {
			t.Errorf("wasCancelled(%v) = true; a real failure would be logged at INFO and lost", err)
		}
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
