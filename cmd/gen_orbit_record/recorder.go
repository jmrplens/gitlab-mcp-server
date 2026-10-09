package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"slices"
	"strings"
	"sync"
)

// apiPrefix is where GitLab's REST API is mounted, which the recorded paths
// are written below.
const apiPrefix = "/api/v4"

// contentTypeHeader is read on both legs of an exchange: the request's says
// how to name the body's fields, the answer's is recorded and passed back.
const contentTypeHeader = "Content-Type"

// forwardedHeaders are the request headers the proxy passes on. The list is
// short on purpose: a copied Accept-Encoding would make the proxy's own
// transport hand back the compressed bytes it asked for, and the recording
// could not read them.
var forwardedHeaders = []string{"Accept", "Authorization", contentTypeHeader, "Private-Token", "User-Agent"}

// exchange is one request the proxy forwarded and what came back, as the
// record needs it: names, never values.
type exchange struct {
	method      string
	path        string
	query       []string
	body        []string
	status      int
	contentType string
	payload     []byte
}

// recorder is a reverse proxy that keeps every Orbit exchange it forwards.
// The handlers are pointed at it rather than at GitLab.com, so what it keeps
// is exactly what they sent and what they were answered.
type recorder struct {
	upstream string
	client   *http.Client

	mu        sync.Mutex
	exchanges []exchange
}

// newRecorder forwards to upstream through a copy of client that answers a
// redirect rather than following it.
//
// The proxy forwards Private-Token, which net/http does not know to be a
// credential: a client following a redirect copies it onto every hop, to
// whatever host the Location names, so a redirect from GitLab.com would hand
// GITLAB_COM_TOKEN to that host. Answered as it came, the redirect reaches the
// handler as a 3xx and the run fails on a call not answered 200, which is
// right: a recording is of GitLab.com's own answers, so there is nothing to
// follow.
func newRecorder(upstream string, client *http.Client) *recorder {
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &recorder{upstream: strings.TrimRight(upstream, "/"), client: &noFollow}
}

// ServeHTTP forwards one request and answers with what the upstream said,
// keeping the exchange when it was an Orbit route. A failure to forward is
// answered 502, which the handler that made the request reports as its own
// failure.
func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "read the request: "+err.Error(), http.StatusBadGateway)
		return
	}
	// #nosec G704 -- the host is the recorder's own upstream, GitLab.com in
	// every real run, and only the route below it comes from the request,
	// which the handlers of this repository built on the loopback interface.
	forward, err := http.NewRequestWithContext(req.Context(), req.Method, r.upstream+req.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build the forwarded request: "+err.Error(), http.StatusBadGateway)
		return
	}
	for _, name := range forwardedHeaders {
		if value := req.Header.Get(name); value != "" {
			forward.Header.Set(name, value)
		}
	}
	// #nosec G704 -- the request built above, to the recorder's own upstream.
	resp, err := r.client.Do(forward)
	if err != nil {
		http.Error(w, "forward the request: "+err.Error(), http.StatusBadGateway)
		return
	}
	payload, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		http.Error(w, "read the answer: "+err.Error(), http.StatusBadGateway)
		return
	}
	if route, isAPI := strings.CutPrefix(req.URL.Path, apiPrefix); isAPI && strings.HasPrefix(route, "/orbit/") {
		r.keep(exchange{
			method:      req.Method,
			path:        route,
			query:       queryNames(req),
			body:        bodyNames(req.Header.Get(contentTypeHeader), body),
			status:      resp.StatusCode,
			contentType: mediaType(resp.Header.Get(contentTypeHeader)),
			payload:     payload,
		})
	}
	if contentType := resp.Header.Get(contentTypeHeader); contentType != "" {
		w.Header().Set(contentTypeHeader, contentType)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(payload)
}

// keep records one exchange.
func (r *recorder) keep(ex exchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exchanges = append(r.exchanges, ex)
}

// take returns the exchanges kept since the last take and forgets them, so
// each handler call is read on its own.
func (r *recorder) take() []exchange {
	r.mu.Lock()
	defer r.mu.Unlock()
	taken := r.exchanges
	r.exchanges = nil
	return taken
}

// queryNames is the sorted set of query parameter names a request carried.
func queryNames(req *http.Request) []string {
	return sortedNames(req.URL.Query())
}

// bodyNames is the sorted set of top-level names a JSON object body carried,
// or nothing for a body that is not one.
func bodyNames(contentType string, body []byte) []string {
	if !isJSON(mediaType(contentType)) {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil
	}
	return sortedNames(fields)
}

// sortedNames is the sorted names of set, nil when it has none, which is what
// keeps an empty query or object out of the record.
func sortedNames[V any](set map[string]V) []string {
	return slices.Sorted(maps.Keys(set))
}

// mediaType is a Content-Type header without its parameters, lowercased, or
// the header as written when it does not parse.
func mediaType(header string) string {
	parsed, _, err := mime.ParseMediaType(header)
	if err != nil {
		return header
	}
	return parsed
}

// isJSON reports whether a media type is JSON: application/json itself, or a
// structured suffix that is.
func isJSON(media string) bool {
	return media == "application/json" || strings.HasSuffix(media, "+json")
}

// describe renders an exchange for an error message.
func (ex exchange) describe() string {
	return fmt.Sprintf("%s %s answered %d", ex.method, ex.path, ex.status)
}
