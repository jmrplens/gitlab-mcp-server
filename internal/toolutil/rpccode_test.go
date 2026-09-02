// rpccode_test.go verifies the JSON-RPC classification wrapper: which code a
// failure travels out under, and that wrapping nothing produces nothing.
package toolutil

import (
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// TestCoded_NothingToWrap_StaysNil covers the guard both classifiers share.
//
// They are applied on the way out of a handler, where "there was no error" is
// the ordinary case: wrapping nil would produce a non-nil error value that
// every caller checks and none can read, turning every successful call into a
// JSON-RPC failure.
func TestCoded_NothingToWrap_StaysNil(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		wrap func(error) error
	}{
		{name: "invalid params", wrap: InvalidParams},
		{name: "internal error", wrap: InternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.wrap(nil); got != nil {
				t.Errorf("wrapping nil returned %#v, want nil", got)
			}
		})
	}
}

// TestCoded_CarriesBothTheCauseAndTheCode pins what a wrapped error still
// answers to.
//
// The cause has to stay reachable through errors.Is, because callers branch on
// sentinels the handler returned, and the code has to be reachable the same way,
// because that is how the SDK decides which JSON-RPC error to write. An
// implementation that carried only one of the two would look correct in
// whichever half its test checked.
func TestCoded_CarriesBothTheCauseAndTheCode(t *testing.T) {
	t.Parallel()

	cause := errors.New("the caller sent an empty project id")

	tests := []struct {
		name     string
		wrapped  error
		wantCode int64
	}{
		{name: "invalid params", wrapped: InvalidParams(cause), wantCode: jsonrpc.CodeInvalidParams},
		{name: "internal error", wrapped: InternalError(cause), wantCode: jsonrpc.CodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(tt.wrapped, cause) {
				t.Errorf("errors.Is lost the cause in %v", tt.wrapped)
			}
			var rpcErr *jsonrpc.Error
			if !errors.As(tt.wrapped, &rpcErr) {
				t.Fatalf("errors.As found no JSON-RPC error in %v", tt.wrapped)
			}
			if rpcErr.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rpcErr.Code, tt.wantCode)
			}
			if tt.wrapped.Error() != cause.Error() {
				t.Errorf("message = %q, want the cause's own %q", tt.wrapped, cause)
			}
		})
	}
}
