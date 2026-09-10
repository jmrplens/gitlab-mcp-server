// etag.go gives the documents this server renders once and then serves
// unchanged a validator, so a client that already holds a copy is answered
// 304 rather than handed a second one.
//
// Three routes qualify: the SEP-2127 card, the enumerating card at the
// .well-known path, and the RFC 9728 protected-resource metadata. All three
// are public, all three are fetched far more often than they change (they
// change only when the process restarts with different flags), and one of
// them is 137 KB, which is the whole reason this file exists: a registry
// scanner polling it costs the fleet that much per poll, per replica, for a
// document that has not moved since startup.
//
// /health deliberately does not qualify. Its body carries uptime_seconds and
// so differs on every probe: that is a document with no validator, not one
// whose validator is merely expensive to compute.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

// hdrETag is the response header the validator is published under.
const hdrETag = "ETag"

// entityTagFor derives a strong entity tag from a document's own bytes.
//
// Deriving it from the bytes and from nothing else is the property that
// matters here, and it is what makes the tag worth having on a deployment
// running several replicas behind one balancer: two processes that rendered
// the same document publish the same tag, so a client revalidating against
// whichever replica the balancer happened to pick is answered 304. A tag
// minted per process — a start instant, a random string, even the build
// digest — would miss on every request that changed replica, which on a
// round-robin balancer is most of them, and the miss would be invisible
// because the response is still correct.
//
// SHA-256 truncated to 128 bits keeps the header short. The tag has to tell
// apart the handful of documents one deployment serves over its lifetime, not
// resist a collision search: anyone able to choose the body here could choose
// the response, so there is nothing further to protect.
func entityTagFor(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// serveCachedDocument writes body under the validator etag, answering a
// conditional request the way RFC 9110 section 13 says to.
//
// [http.ServeContent] does the conditional handling rather than a comparison
// written here, because that comparison has four ways to be wrong and every
// one of them fails silently, by serving a correct 200 nobody needed:
// If-None-Match is a list; "*" is a value of its own matching any current
// representation; the comparison a GET uses is the *weak* one; and a proxy
// that compresses this response rewrites the tag it forwards as W/"…", so a
// strong string equality would stop matching the moment the document crossed
// a CDN, which is precisely where it was supposed to help. ServeContent also
// answers HEAD and a Range request on the same terms and sets Content-Length,
// none of which a bare w.Write does.
//
// The caller sets Content-Type and Cache-Control before calling: ServeContent
// sniffs a type only when none is set, and this package chooses both per
// route.
func serveCachedDocument(w http.ResponseWriter, r *http.Request, etag string, body []byte) {
	w.Header().Set(hdrETag, etag)
	// The zero instant leaves Last-Modified unsent, which is the honest
	// answer: these documents are rendered at startup out of configuration
	// and have no modification time a client could reason about. Sending the
	// process start instant instead would invite a client to compare two
	// replicas by it and conclude the document had changed when only the
	// process had.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// capturedResponse keeps the body a handler writes so it can be given a
// validator, while letting that handler's headers through to the real
// response as it sets them.
//
// It exists for the one public document this package publishes but does not
// render: the RFC 9728 metadata, which the SDK handler marshals itself.
// Hashing what the SDK actually wrote is what keeps the tag honest —
// re-marshaling the same struct here to hash it would publish a validator
// for bytes nobody sent, and would drift apart from the response the first
// time the SDK changed a field's spelling or its encoder's settings.
type capturedResponse struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

// WriteHeader records the first status the handler chose. A later call is
// ignored, which is what net/http itself does with a superfluous WriteHeader.
func (c *capturedResponse) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

// Write buffers rather than sending, and records the implied 200 for a
// handler that wrote a body without stating a status.
func (c *capturedResponse) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(p)
}

// ok reports whether the handler produced a representation, rather than an
// error it answered with itself.
func (c *capturedResponse) ok() bool {
	return c.status == 0 || c.status == http.StatusOK
}

// replay forwards a response captured from a handler verbatim.
func (c *capturedResponse) replay(w http.ResponseWriter) {
	if c.status != 0 {
		w.WriteHeader(c.status)
	}
	_, _ = w.Write(c.body.Bytes())
}
