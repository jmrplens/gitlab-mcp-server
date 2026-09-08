package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// ResponseCapture holds the body of the last response a request made under
// [WithResponseCapture] received, so that a handler can read from it a field
// GitLab sends and client-go's struct does not model, from the same bytes the
// SDK decoded, without a second request and without spelling the route
// itself.
//
// It exists because the 1:1 surface has no room for a field GitLab sends and
// this server drops, and client-go models most of the API but not all of it:
// a note's `imported_from`, a member's `membership_state`, a key's
// `expires_at`. Before this the answer was a request of the handler's own,
// which duplicates the route client-go already spells and, for a list, its
// pagination too. A capture leaves every request as it was and adds one
// decode of bytes already in memory.
//
// Safe for use from one goroutine at a time, which is what a handler is; the
// transport records under a lock because a retried request answers more than
// once, and the last answer is the one the SDK decoded.
type ResponseCapture struct {
	mu   sync.Mutex
	body []byte
	seen bool
}

// captureKey is the context key a capture travels under.
type captureKey struct{}

// ErrNoResponseCaptured is what [ResponseCapture.Decode] returns when no
// response passed through the transport under the capture's context: the
// request was never made, or was made with another context.
var ErrNoResponseCaptured = errors.New("no response was captured")

// WithResponseCapture returns a context under which the SDK client records
// the body of every response it receives, and the capture to read it from.
// Pass the context to the SDK call with gl.WithContext.
func WithResponseCapture(ctx context.Context) (context.Context, *ResponseCapture) {
	capture := &ResponseCapture{}
	return context.WithValue(ctx, captureKey{}, capture), capture
}

// CapturedBody returns a capture already holding body, as if a request made
// under it had been answered with it. It is for a test of a reader that
// decodes captures, which would otherwise need a transport to hand it one.
func CapturedBody(body []byte) *ResponseCapture {
	capture := &ResponseCapture{}
	capture.record(body)
	return capture
}

// record keeps a body, replacing whatever an earlier attempt of the same
// request recorded.
func (c *ResponseCapture) record(body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body, c.seen = body, true
}

// Decode unmarshals the captured body into v, the way the SDK unmarshalled it
// into its own struct. A body that does not decode is an error a handler
// reports rather than swallows: the SDK decoded the same bytes, so the fault
// is in the type v names.
func (c *ResponseCapture) Decode(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.seen {
		return ErrNoResponseCaptured
	}
	if err := json.Unmarshal(c.body, v); err != nil {
		return fmt.Errorf("decode the captured response: %w", err)
	}
	return nil
}

// captureTransport copies the body of every response whose request came
// under a [ResponseCapture], and hands the SDK the same bytes to decode.
//
// It sits just outside the response ceiling, so the body it copies is the
// bounded one: an oversized answer fails here with [ErrResponseTooLarge]
// exactly as it would have failed in the SDK's decoder.
type captureTransport struct {
	base http.RoundTripper
}

// RoundTrip delegates to the base transport and, for a request that asked,
// reads the body once, records it, and replaces it with a reader over the
// same bytes.
func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	capture, wanted := req.Context().Value(captureKey{}).(*ResponseCapture)
	if err != nil || resp == nil || resp.Body == nil || !wanted {
		return resp, err
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read the response to capture it: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close the captured response: %w", closeErr)
	}
	capture.record(body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}
