// discovery_test.go verifies the wrapper around the SDK's protected-resource
// metadata handler: the HTTP rules that apply to any public document, which the
// SDK handler does not meet on its own.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMetadataDocument_BehavesLikeAnHTTPDocument covers each method the
// wrapper treats differently, and what it adds on top of the SDK handler.
//
// Two of these are MUSTs the SDK handler does not meet, on an endpoint where
// the difference shows: health checks, link checkers and CDN origin probes all
// reach for HEAD, which the SDK answers with 405, and a 405 must carry an Allow
// header, which it does not set. Access-Control-Allow-Methods is not a
// substitute — that header answers a CORS preflight, while a client asking what
// a resource supports reads Allow. The caching directive lives here too rather
// than in the security headers, which are right to default every response to
// no-store: this is the one document that wants the opposite.
func TestMetadataDocument_BehavesLikeAnHTTPDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// method is what the client sends.
		method string
		// wantSeen is the method the SDK handler below must be handed, or ""
		// when the wrapper answers without reaching it.
		wantSeen  string
		wantCache bool
		wantAllow bool
	}{
		{name: "GET is the ordinary fetch", method: http.MethodGet, wantSeen: http.MethodGet, wantCache: true},
		{name: "HEAD is served through the GET path", method: http.MethodHead, wantSeen: http.MethodGet, wantCache: true},
		{name: "OPTIONS is left to the SDK's preflight", method: http.MethodOptions, wantSeen: http.MethodOptions},
		{name: "anything else gets an Allow header", method: http.MethodPost, wantSeen: http.MethodPost, wantAllow: true},
		{name: "DELETE too", method: http.MethodDelete, wantSeen: http.MethodDelete, wantAllow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var seen string
			handler := metadataDocument(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, "/.well-known/oauth-protected-resource", http.NoBody))

			if seen != tt.wantSeen {
				t.Errorf("the handler below saw %q, want %q", seen, tt.wantSeen)
			}
			if got := rec.Header().Get("Cache-Control"); (got != "") != tt.wantCache {
				t.Errorf("Cache-Control = %q, want present=%v", got, tt.wantCache)
			}
			if tt.wantCache && rec.Header().Get("Cache-Control") != discoveryCacheControl {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), discoveryCacheControl)
			}
			if got := rec.Header().Get("Allow"); (got != "") != tt.wantAllow {
				t.Errorf("Allow = %q, want present=%v", got, tt.wantAllow)
			}
			if tt.wantAllow && rec.Header().Get("Allow") != "GET, HEAD, OPTIONS" {
				t.Errorf("Allow = %q, want the methods this document really serves", rec.Header().Get("Allow"))
			}
		})
	}
}

// TestMetadataDocument_HEAD_DoesNotMutateTheCallersRequest covers the clone the
// HEAD path makes.
//
// The SDK checks the method itself, so it has to see a GET; rewriting the
// request in place would hand every later middleware in the chain — and the
// server's own logging — a request that claims to be something the client never
// sent.
func TestMetadataDocument_HEAD_DoesNotMutateTheCallersRequest(t *testing.T) {
	t.Parallel()

	handler := metadataDocument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodHead, "/.well-known/oauth-protected-resource", http.NoBody)

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if req.Method != http.MethodHead {
		t.Errorf("the caller's request now says %q; the rewrite must happen on a clone", req.Method)
	}
}

// TestMetadataDocument_CarriesAValidatorAndAnswersAConditionalFetch covers
// what the wrapper adds on top of the lifetime it already published.
//
// max-age on its own tells a client how long it may reuse the document and
// gives it nothing to say when that runs out, so the next fetch is a full one
// however unchanged the document is. The tag is derived from the bytes the SDK
// handler actually wrote rather than from a second marshaling here, so it
// cannot describe a body nobody sent.
func TestMetadataDocument_CarriesAValidatorAndAnswersAConditionalFetch(t *testing.T) {
	t.Parallel()

	document := `{"resource":"https://mcp.example.test/gitlab"}` + "\n"
	handler := metadataDocument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeJSON)
		_, _ = w.Write([]byte(document))
	}))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/oauth-protected-resource", http.NoBody))

	tag := first.Header().Get(hdrETag)
	if tag == "" {
		t.Fatal("no ETag on the metadata document, so a client has nothing to revalidate with")
	}
	if tag != entityTagFor([]byte(document)) {
		t.Errorf("ETag = %q, want the tag of the bytes the handler below wrote", tag)
	}
	if first.Body.String() != document {
		t.Errorf("body = %q, want the document the handler below wrote", first.Body.String())
	}
	if got := first.Header().Get(hdrContentType); got != mimeJSON {
		t.Errorf("Content-Type = %q, want %q; the buffering must not let ServeContent sniff it", got, mimeJSON)
	}

	conditional := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/oauth-protected-resource", http.NoBody)
	conditional.Header.Set("If-None-Match", tag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, conditional)

	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d on a fetch carrying the tag we published, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried %d bytes of body", second.Body.Len())
	}
}

// TestMetadataDocument_AFailureIsForwardedUntagged covers the branch where the
// SDK handler answers with something that is not the document.
//
// It encodes into the response writer, so a marshaling failure is answered
// after the status is chosen. Tagging that answer would publish a validator
// for an error page under the document's own identity, and a client
// revalidating would be handed the failure back.
func TestMetadataDocument_AFailureIsForwardedUntagged(t *testing.T) {
	t.Parallel()

	handler := metadataDocument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Failed to encode metadata", http.StatusInternalServerError)
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/oauth-protected-resource", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the 500 the handler below chose", rec.Code)
	}
	if got := rec.Header().Get(hdrETag); got != "" {
		t.Errorf("ETag = %q on a failure, which a client could then revalidate into", got)
	}
	// The lifetime is set before the document is rendered, on the assumption
	// that a document is what comes back. Left in place on a 500, a shared
	// cache would serve the failure to everyone behind it for an hour.
	if got := rec.Header().Get(hdrCacheControl); got != cacheControlNoStore {
		t.Errorf("Cache-Control = %q on a failure, want %q", got, cacheControlNoStore)
	}
	if !strings.Contains(rec.Body.String(), "Failed to encode metadata") {
		t.Errorf("body = %q, want the handler's own message", rec.Body.String())
	}
}
