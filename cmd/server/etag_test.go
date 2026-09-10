// etag_test.go verifies the validator this server publishes for the documents
// it renders once and then serves unchanged, and the conditional request that
// validator exists to make possible.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEntityTagFor_DependsOnTheBytesAndNothingElse covers the one property
// that decides whether the tag is worth publishing.
//
// The public deployment runs several replicas behind one balancer, so a client
// revalidating is very unlikely to reach the process that served it the
// document. A tag derived from the bytes is identical across those replicas
// and the revalidation is answered 304; a tag carrying anything about the
// process — a start instant, a random string, the build digest — would miss on
// every request that changed replica, and would do it invisibly, because the
// 200 it produces is still a correct response.
func TestEntityTagFor_DependsOnTheBytesAndNothingElse(t *testing.T) {
	t.Parallel()

	// Built separately on purpose: same content, no shared backing array, the
	// way two replicas would have rendered it.
	first := []byte(`{"name":"gitlab","version":"3.0.0"}`)
	second := append([]byte(`{"name":"gitlab",`), []byte(`"version":"3.0.0"}`)...)

	if entityTagFor(first) != entityTagFor(second) {
		t.Errorf("two renderings of the same document tagged differently: %s vs %s",
			entityTagFor(first), entityTagFor(second))
	}
	if entityTagFor(first) == entityTagFor([]byte(`{"name":"gitlab","version":"3.1.0"}`)) {
		t.Error("two different documents share a tag, so a client would never be told the document moved")
	}
}

// TestEntityTagFor_IsSyntacticallyAnEntityTag covers the framing.
//
// RFC 9110 section 8.8.3 defines an entity tag as an opaque quoted string,
// optionally weak. A bare hex string is not one, and every parser on the path
// — net/http's own scanETag included — discards it silently rather than
// reporting it, so an unquoted tag would disable the conditional request
// without any request failing.
func TestEntityTagFor_IsSyntacticallyAnEntityTag(t *testing.T) {
	t.Parallel()

	tag := entityTagFor([]byte("body"))

	if !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) {
		t.Errorf("tag %s is not a quoted string", tag)
	}
	if strings.HasPrefix(tag, `W/`) {
		t.Errorf("tag %s is weak; these documents are served byte-for-byte and the tag should say so", tag)
	}
	if strings.Count(tag, `"`) != 2 {
		t.Errorf("tag %s quotes something inside itself, which no parser will read back whole", tag)
	}
}

// TestServeCachedDocument_ConditionalRequestIsAnsweredFromTheValidator covers
// the four shapes of If-None-Match a real client sends.
//
// The weak and list forms are the ones a hand-written comparison gets wrong:
// a proxy that compresses this response rewrites the tag it forwards as
// W/"…", so a strong equality would stop matching exactly where the caching
// was meant to help, and "*" is a value of its own that matches any current
// representation rather than a tag to compare against.
func TestServeCachedDocument_ConditionalRequestIsAnsweredFromTheValidator(t *testing.T) {
	t.Parallel()

	body := []byte(`{"schema":"https://example.test/card"}`)
	tag := entityTagFor(body)

	tests := []struct {
		name        string
		ifNoneMatch string
		wantStatus  int
	}{
		{name: "no condition is a plain fetch", ifNoneMatch: "", wantStatus: http.StatusOK},
		{name: "the tag we published", ifNoneMatch: tag, wantStatus: http.StatusNotModified},
		{name: "the same tag weakened by a compressing proxy", ifNoneMatch: "W/" + tag, wantStatus: http.StatusNotModified},
		{name: "a list holding it", ifNoneMatch: `"stale", ` + tag, wantStatus: http.StatusNotModified},
		{name: "the wildcard, which matches any representation", ifNoneMatch: "*", wantStatus: http.StatusNotModified},
		{name: "a tag from another document", ifNoneMatch: `"0123456789abcdef0123456789abcdef"`, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/server-card", http.NoBody)
			if tt.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tt.ifNoneMatch)
			}
			rec := httptest.NewRecorder()
			rec.Header().Set(hdrContentType, mimeServerCard)
			rec.Header().Set(hdrCacheControl, cacheControlPublic1h)

			serveCachedDocument(rec, req, tag, body)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get(hdrETag); got != tag {
				t.Errorf("ETag = %q, want %q on every answer, so the next request can be conditional too", got, tag)
			}
			// RFC 9110 section 15.4.5: a 304 carries the caching directives a
			// 200 would have, or the client has to guess how long the copy it
			// was just told to keep stays fresh.
			if got := rec.Header().Get(hdrCacheControl); got != cacheControlPublic1h {
				t.Errorf("Cache-Control = %q, want %q", got, cacheControlPublic1h)
			}
			wantBody := tt.wantStatus == http.StatusOK
			if gotBody := rec.Body.Len() > 0; gotBody != wantBody {
				t.Errorf("body present = %v, want %v", gotBody, wantBody)
			}
		})
	}
}

// TestServeCachedDocument_HEAD_SendsTheLengthWithoutTheBody covers the method
// a CDN origin probe and a link checker reach for.
//
// Answering HEAD is not optional (RFC 9110 section 9.1: a general-purpose
// server MUST support GET and HEAD), and answering it with a body would be
// wrong in a way the client cannot correct.
func TestServeCachedDocument_HEAD_SendsTheLengthWithoutTheBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{"schema":"https://example.test/card"}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodHead, "/server-card", http.NoBody)
	rec := httptest.NewRecorder()
	rec.Header().Set(hdrContentType, mimeServerCard)

	serveCachedDocument(rec, req, entityTagFor(body), body)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD answered with %d bytes of body", rec.Body.Len())
	}
	if got := rec.Header().Get("Content-Length"); got != "38" {
		t.Errorf("Content-Length = %q, want the length of the body a GET would return", got)
	}
}

// TestCapturedResponse_ReplaysAFailureVerbatim covers what happens when the
// wrapped handler answered with something that is not a representation.
//
// A validator identifies a representation of the resource. Tagging an error
// page would let a client cache the failure under the document's own identity
// and revalidate into it, which is worse than not caching at all.
func TestCapturedResponse_ReplaysAFailureVerbatim(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	captured := &capturedResponse{ResponseWriter: rec}
	captured.WriteHeader(http.StatusInternalServerError)
	_, _ = captured.Write([]byte("failed to encode metadata\n"))
	// A second WriteHeader is what a handler does by mistake; net/http ignores
	// it and so must this.
	captured.WriteHeader(http.StatusOK)

	if captured.ok() {
		t.Error("a 500 was classified as a representation worth tagging")
	}
	captured.replay(rec)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the 500 the handler chose", rec.Code)
	}
	if rec.Body.String() != "failed to encode metadata\n" {
		t.Errorf("body = %q, want the handler's own", rec.Body.String())
	}
	if got := rec.Header().Get(hdrETag); got != "" {
		t.Errorf("ETag = %q on a failure", got)
	}
}

// TestCapturedResponse_TreatsAWrittenBodyAsA200 covers the handler that writes
// a body without stating a status, which is the ordinary shape: net/http
// implies 200 there, and a wrapper that read the status as 0 would classify
// every successful document as a failure and forward it untagged.
func TestCapturedResponse_TreatsAWrittenBodyAsA200(t *testing.T) {
	t.Parallel()

	captured := &capturedResponse{ResponseWriter: httptest.NewRecorder()}
	_, _ = captured.Write([]byte(`{"resource":"https://example.test"}`))

	if !captured.ok() {
		t.Error("a body written with no explicit status was not treated as a 200")
	}
	if captured.status != http.StatusOK {
		t.Errorf("status = %d, want %d", captured.status, http.StatusOK)
	}
}
