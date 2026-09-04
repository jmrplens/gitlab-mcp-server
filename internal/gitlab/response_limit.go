package gitlab

import (
	"errors"
	"io"
	"net/http"
)

// DefaultMaxResponseBytes is how much of one GitLab response body a client
// will read before giving up on it.
//
// The number bounds the *decompressed* stream, which is the only place it can
// do any good: net/http gunzips a Content-Encoding: gzip response inside the
// base transport, so a cap applied to wire bytes bounds the compressed size
// and misses the amplification entirely. A 528 KB gzip that inflates to 67 MB
// is an ordinary artifact of a hostile or compromised upstream, and the SDK
// decodes whatever it is handed with no ceiling of its own.
//
// 64 MiB is chosen to sit above every response this server asks for in
// practice — the largest are job traces, which the jobs package truncates to
// 100 KB of its own accord, and artifact and export downloads, which are
// base64-encoded into a JSON-RPC response and are already unusable long before
// this — while being far below the multi-gigabyte working set an unbounded
// decode reaches. Raise it with [Client.SetMaxResponseBytes] if a deployment
// genuinely streams more than that through a tool.
const DefaultMaxResponseBytes int64 = 64 << 20

// ErrResponseTooLarge reports that a GitLab response body exceeded the
// client's size ceiling and was abandoned rather than buffered.
//
// It surfaces from whatever was reading the body — the SDK's JSON decode, an
// io.Copy into a caller's writer — so a tool handler sees a failed call rather
// than a truncated success.
var ErrResponseTooLarge = errors.New("gitlab response exceeded the maximum size")

// SetMaxResponseBytes sets how much of a single response body this client will
// read. A value of zero or less removes the ceiling.
//
// The ceiling applies from the next request onwards; a body already being read
// keeps the limit it started with.
func (c *Client) SetMaxResponseBytes(n int64) { c.maxResponse.Store(n) }

// maxResponseBytes returns the client's current response ceiling.
func (c *Client) maxResponseBytes() int64 {
	if c == nil {
		return DefaultMaxResponseBytes
	}
	return c.maxResponse.Load()
}

// responseLimitTransport caps every response body the GitLab SDK receives.
//
// It sits closest to the base transport so that the body it wraps is the one
// net/http has already decompressed. Wrapping further out would work equally
// well for ordering reasons — the body is passed through untouched by the
// other layers — but placing it here makes the "after gunzip" property
// structural rather than incidental.
type responseLimitTransport struct {
	base   http.RoundTripper
	client *Client
}

// RoundTrip delegates to the base transport and wraps the response body in a
// reader that refuses to deliver more than the client's ceiling.
func (t *responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil && resp.StatusCode == http.StatusUnauthorized {
		// Innermost layer, so every call the SDK makes passes here: the first
		// data call GitLab refuses is the revocation signal, not the next
		// periodic re-check.
		t.client.notifyUnauthorized()
	}
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	limit := t.client.maxResponseBytes()
	if limit <= 0 {
		return resp, nil
	}
	resp.Body = &limitedBody{inner: resp.Body, remaining: limit}
	return resp, nil
}

// limitedBody is an [io.ReadCloser] that delivers at most a fixed number of
// bytes and then fails with [ErrResponseTooLarge].
//
// It is deliberately not [io.LimitReader]: a limit reader reports EOF at the
// ceiling, which turns an oversized body into a truncated one and hands the
// JSON decoder a syntax error that says nothing about what happened. Failing
// with a named error keeps the diagnosis, and keeps a truncated response from
// ever being mistaken for a complete one.
type limitedBody struct {
	inner     io.ReadCloser
	remaining int64
	exceeded  bool
}

// Read fills p from the underlying body, refusing the first byte past the
// ceiling.
func (b *limitedBody) Read(p []byte) (int, error) {
	if b.exceeded {
		return 0, ErrResponseTooLarge
	}
	// Read one byte past what is left, so the overflow is detected on this
	// call rather than only on the next one.
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.inner.Read(p)
	if int64(n) > b.remaining {
		b.exceeded = true
		b.remaining = 0
		return 0, ErrResponseTooLarge
	}
	b.remaining -= int64(n)
	return n, err
}

// Close closes the underlying body.
func (b *limitedBody) Close() error { return b.inner.Close() }
