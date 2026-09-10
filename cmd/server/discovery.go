package main

import (
	"net/http"
)

// discoveryCacheControl is how long a client may reuse the protected-resource
// metadata document.
//
// RFC 9728 §7.10: "Implementations should utilize HTTP caching directives such
// as Cache-Control." The document is public by construction — it is served with
// Access-Control-Allow-Origin: * and contains nothing a client could not
// discover by asking — and it only changes when the operator restarts with
// different flags, while every client fetches it on every discovery attempt.
// An hour matches what the server card already uses.
const discoveryCacheControl = "public, max-age=3600"

// metadataDocument adapts the SDK's protected-resource metadata handler to the
// rules that apply to any HTTP document.
//
// Two of them are MUSTs that the SDK handler does not meet, and it is mounted
// on a public endpoint where the difference shows: health checks, link
// checkers and CDN origin probes all reach for HEAD.
//
//   - "All general-purpose servers MUST support the methods GET and HEAD."
//     The SDK answers anything that is not GET with 405, HEAD included. Go's
//     net/http discards the body of a HEAD response on its own, so serving it
//     through the GET path is both correct and complete.
//   - "The origin server MUST generate an Allow header field in a 405."
//     The SDK sets none. Access-Control-Allow-Methods is not a substitute:
//     that header answers a CORS preflight, and a client asking what a
//     resource supports reads Allow.
//
// The caching directive lives here too rather than in
// securityHeadersMiddleware, which is right to default every response to
// no-store; this is the one document that wants the opposite, and saying so at
// the mount keeps the default strict.
//
// The validator is added here for the same reason. A lifetime on its own tells
// a client how long it may reuse the document and gives it nothing to say when
// that lifetime runs out, so the next fetch is a full one however unchanged
// the document is; an entity tag turns that fetch into a conditional request
// the origin can answer with 304.
func metadataDocument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			w.Header().Set(hdrCacheControl, discoveryCacheControl)
			// So a cross-origin script can read the validator back, and only
			// when nothing published a list already: corsMiddleware sets a
			// longer one for a trusted origin, ETag included, and replacing it
			// here would cost that origin the headers it names.
			//
			// The request half is answered by whoever gets the preflight
			// first. A trusted origin reaches corsMiddleware, whose allow-list
			// names If-None-Match. Any other origin reaches the SDK handler,
			// which answers Access-Control-Allow-Headers: Content-Type and
			// nothing else, so a browser there will not send the header at
			// all. That half is upstream's to widen; the server-side clients
			// that fetch this document during discovery are unaffected either
			// way.
			if w.Header().Get(headerExposeHeaders) == "" {
				w.Header().Set(headerExposeHeaders, hdrETag)
			}
			serveMetadataDocument(w, r, next)
		case http.MethodOptions:
			// Left to the SDK, which answers the CORS preflight.
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			next.ServeHTTP(w, r)
		}
	})
}

// serveMetadataDocument renders the metadata through next and serves it with
// an entity tag derived from the bytes next produced.
//
// The document is small, so this hashes per request rather than caching a tag
// beside a copy of the body. That is deliberate: the handler this wraps is the
// SDK's and its output is not this package's to assume constant, and a tag
// computed from whatever it wrote this time cannot be stale. What it costs is
// a SHA-256 over a few hundred bytes on a route a client reaches once per
// discovery.
func serveMetadataDocument(w http.ResponseWriter, r *http.Request, next http.Handler) {
	// The SDK checks the method itself, so it has to see a GET. The response
	// body is dropped by net/http before it reaches the wire.
	rendered := r
	if r.Method == http.MethodHead {
		rendered = r.Clone(r.Context())
		rendered.Method = http.MethodGet
	}
	captured := &capturedResponse{ResponseWriter: w}
	next.ServeHTTP(captured, rendered)
	if !captured.ok() {
		// A failure the SDK answered with is forwarded exactly as it wrote
		// it, with one correction. It carries no validator, because a
		// validator identifies a representation of the resource and an error
		// page is not one; and the lifetime set above is withdrawn, because
		// the branch that set it assumed the document was about to be
		// rendered. Left in place, a 500 would go out marked cacheable for an
		// hour and a shared cache would serve the failure to everyone behind
		// it for that long. http.Error, which is how the SDK answers, sets a
		// content type and a status and clears nothing.
		w.Header().Set(hdrCacheControl, cacheControlNoStore)
		captured.replay(w)
		return
	}
	// r rather than rendered, so ServeContent knows it is answering a HEAD
	// and sends the length without the body.
	serveCachedDocument(w, r, entityTagFor(captured.body.Bytes()), captured.body.Bytes())
}
