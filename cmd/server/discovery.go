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
func metadataDocument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Cache-Control", discoveryCacheControl)
		case http.MethodHead:
			w.Header().Set("Cache-Control", discoveryCacheControl)
			// The SDK checks the method itself, so it has to see a GET. The
			// response body is dropped by net/http before it reaches the wire.
			r = r.Clone(r.Context())
			r.Method = http.MethodGet
		case http.MethodOptions:
			// Left to the SDK, which answers the CORS preflight.
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		}
		next.ServeHTTP(w, r)
	})
}
