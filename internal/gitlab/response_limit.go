package gitlab

import (
	"bytes"
	"cmp"
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
	// reportsUnauthorized says whether a 401 answered through this transport
	// reaches the client's unauthorized hook ([Client.SetOnUnauthorized]).
	//
	// The SDK chain reports, since every call a tool, resource, prompt or
	// watcher makes passes there. The health client does not: each request it
	// makes is a probe whose caller acts on the answer itself, and one of them
	// is the probe that confirms a 401 nobody explained
	// ([Client.CheckCredential]). Reporting that probe's own 401 would hand the
	// pool a second refusal to confirm, raised by the request confirming the
	// first.
	reportsUnauthorized bool
}

// unauthorizedPeekBytes is how much of a 401's body the transport reads to
// classify it.
//
// GitLab's 401 bodies are all well under 300 bytes, rack-oauth2's included, so
// 4 KiB holds any of them whole. A longer body is cut at the bound and no
// longer decodes, which classifies it as naming no cause: the direction that
// leads to asking GitLab again rather than to acting on a guess.
const unauthorizedPeekBytes = 4 << 10

// RoundTrip delegates to the base transport and wraps the response body in a
// reader that refuses to deliver more than the client's ceiling.
func (t *responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if t.reportsUnauthorized && resp.StatusCode == http.StatusUnauthorized {
		// Innermost layer, so every call the SDK makes passes here: the first
		// data call GitLab refuses is the revocation signal, not the next
		// periodic re-check. Classified here and not by whoever registered the
		// hook, because this is the only layer that sees the request and the
		// body together before anyone has consumed the body.
		t.client.notifyUnauthorized(classifyUnauthorized(req, resp))
	}
	if resp.Body == nil {
		return resp, nil
	}
	limit := t.client.maxResponseBytes()
	if limit <= 0 {
		return resp, nil
	}
	resp.Body = &limitedBody{inner: resp.Body, remaining: limit}
	return resp, nil
}

// classifyUnauthorized says what a 401 names, reading a bounded prefix of its
// body and putting the prefix back so the body is delivered whole.
//
// The peek happens before the ceiling is applied, so the limit still counts
// every byte, the peeked ones included, and the SDK and the response capture
// (ADR-0021) read exactly what GitLab sent. The prefix is classified whatever
// the read returned: one cut short by a failed read does not decode, so it
// names nothing, and the failure itself is handed on to whoever reads the body
// next rather than swallowed here.
func classifyUnauthorized(req *http.Request, resp *http.Response) UnauthorizedAnswer {
	var prefix []byte
	if resp.Body != nil {
		var readErr error
		prefix, readErr = io.ReadAll(io.LimitReader(resp.Body, unauthorizedPeekBytes))
		resp.Body = &replayedBody{
			Reader: io.MultiReader(bytes.NewReader(prefix), remainderAfterPeek(resp.Body, readErr)),
			closer: resp.Body,
		}
	}
	if UnauthorizedNamesCredential(req, prefix) {
		return UnauthorizedCredential
	}
	return UnauthorizedUnexplained
}

// remainderAfterPeek is what follows the peeked prefix: the rest of the body,
// or, when the peek failed, the error it failed with.
//
// The error is replayed rather than the body read again because nothing
// promises a body returns the same error twice. net/http's own does, and an
// in-process transport need not, so reading on could hand the caller bytes that
// follow a gap instead of the failure that made it.
func remainderAfterPeek(body io.Reader, readErr error) io.Reader {
	if readErr != nil {
		return failingReader{err: readErr}
	}
	return body
}

// replayedBody is a response body whose first bytes were read to classify it
// and are delivered again ahead of the rest. Closing it closes the original.
type replayedBody struct {
	io.Reader
	closer io.Closer
}

// Close closes the original body.
func (b *replayedBody) Close() error { return b.closer.Close() }

// failingReader fails every read with the error a peek met.
type failingReader struct{ err error }

// Read returns the recorded error, and [io.ErrUnexpectedEOF] if none was
// recorded. A reader that answers nothing and no error is one its caller waits
// on forever, since io.ReadAll and the SDK's decoder both read until something
// ends the body, so a reader whose job is to fail must never be able to answer
// that, whoever built it.
func (r failingReader) Read([]byte) (int, error) { return 0, cmp.Or(r.err, io.ErrUnexpectedEOF) }

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
