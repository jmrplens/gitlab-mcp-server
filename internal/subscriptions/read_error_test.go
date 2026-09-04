// read_error_test.go validates the mapping from GitLab API failures onto
// the sentinels a watcher acts on.
//
// The split that matters here is between "stop watching" and "try again":
// getting it wrong in one direction keeps polling with a revoked token, and
// in the other silently kills a healthy subscription over a transient 500.
package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// statusErr builds the error shape client-go returns for an HTTP failure.
//
// The embedded Request is not decoration: ErrorResponse.Error() guards
// against a nil Response but then dereferences Response.Request.URL
// unconditionally, so an error built without one panics as soon as anything
// renders it — including the message-matching inside toolutil.IsNotFound.
// Real client-go errors always carry the request, so this mirrors
// production rather than working around a bug.
func statusErr(code int) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://gitlab.example.com/api/v4/projects/42/pipelines/99", nil)
	if err != nil {
		panic(err) // a constant URL: unreachable
	}
	return &gl.ErrorResponse{
		Response: &http.Response{StatusCode: code, Request: req},
		Message:  fmt.Sprintf("%d error", code),
	}
}

func TestTranslateReadError_MapsStatusesToSentinels(t *testing.T) {
	tests := []struct {
		name string
		code int
		want error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrInaccessible},
		{"forbidden", http.StatusForbidden, ErrInaccessible},
		{"not found", http.StatusNotFound, ErrInaccessible},
		{"too many requests", http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TranslateReadError(statusErr(tt.code))
			if !errors.Is(got, tt.want) {
				t.Errorf("TranslateReadError(HTTP %d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// TestTranslateReadError_ServerErrorsStayTransient verifies failures that
// are not the subscriber's fault keep the subscription alive.
//
// This is the direction with the worse failure mode: treating a 500 or a
// dropped connection as fatal would silently retire a healthy subscription
// during an outage, exactly when a client most wants to be told what
// changed once things recover.
func TestTranslateReadError_ServerErrorsStayTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"internal server error", statusErr(http.StatusInternalServerError)},
		{"bad gateway", statusErr(http.StatusBadGateway)},
		{"service unavailable", statusErr(http.StatusServiceUnavailable)},
		{"gateway timeout", statusErr(http.StatusGatewayTimeout)},
		{"bad request", statusErr(http.StatusBadRequest)},
		{"connection reset", errors.New("read tcp: connection reset by peer")},
		{"decode failure", errors.New("json: cannot unmarshal object")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TranslateReadError(tt.err)
			if errors.Is(got, ErrInaccessible) || errors.Is(got, ErrRateLimited) {
				t.Errorf("TranslateReadError(%v) = %v, want it left transient", tt.err, got)
			}
			if !errors.Is(got, tt.err) {
				t.Errorf("TranslateReadError(%v) = %v, want the original error preserved", tt.err, got)
			}
		})
	}
}

func TestTranslateReadError_Nil_IsNil(t *testing.T) {
	if got := TranslateReadError(nil); got != nil {
		t.Errorf("TranslateReadError(nil) = %v, want nil", got)
	}
}

// TestTranslateReadError_WrappedError_IsStillRecognized verifies the
// mapping survives wrapping, since handlers in this project wrap API errors
// with an operation label before they reach a watcher.
func TestTranslateReadError_WrappedError_IsStillRecognized(t *testing.T) {
	wrapped := fmt.Errorf("failed to get pipeline: %w", statusErr(http.StatusNotFound))
	if got := TranslateReadError(wrapped); !errors.Is(got, ErrInaccessible) {
		t.Errorf("TranslateReadError(wrapped 404) = %v, want ErrInaccessible", got)
	}
}

// TestTranslateReadError_SentinelNotFound_IsRecognized verifies client-go's
// own typed not-found sentinel maps too, not just a status-carrying
// response.
func TestTranslateReadError_SentinelNotFound_IsRecognized(t *testing.T) {
	if got := TranslateReadError(gl.ErrNotFound); !errors.Is(got, ErrInaccessible) {
		t.Errorf("TranslateReadError(gl.ErrNotFound) = %v, want ErrInaccessible", got)
	}
}

// TestTranslateReadError_MCPResourceNotFound_IsRecognized verifies the
// protocol's own not-found signal stops a watcher.
//
// Reads go through the registered resource handlers, and several of those
// answer with mcp.ResourceNotFoundError rather than passing GitLab's status
// through — a repository file that does not exist is the common case. That
// error carries no HTTP status and its message says only "Resource not
// found", so without an explicit branch a deleted file would look like a
// transient blip and keep being polled for a full day.
func TestTranslateReadError_MCPResourceNotFound_IsRecognized(t *testing.T) {
	err := mcp.ResourceNotFoundError("gitlab://project/42/file/main/gone.txt")
	if got := TranslateReadError(err); !errors.Is(got, ErrInaccessible) {
		t.Errorf("TranslateReadError(mcp.ResourceNotFoundError) = %v, want ErrInaccessible", got)
	}
	wrapped := fmt.Errorf("reading resource: %w", err)
	if got := TranslateReadError(wrapped); !errors.Is(got, ErrInaccessible) {
		t.Errorf("TranslateReadError(wrapped) = %v, want ErrInaccessible", got)
	}
}

// TestTranslateReadError_OtherJSONRPCError_StaysTransient verifies the
// branch is scoped to not-found rather than to every protocol error.
func TestTranslateReadError_OtherJSONRPCError_StaysTransient(t *testing.T) {
	err := &jsonrpc.Error{Code: -32603, Message: "internal error"}
	got := TranslateReadError(err)
	if errors.Is(got, ErrInaccessible) || errors.Is(got, ErrRateLimited) {
		t.Errorf("TranslateReadError(internal error) = %v, want it left transient", got)
	}
}
