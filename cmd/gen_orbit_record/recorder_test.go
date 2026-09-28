package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// roundTripFunc answers a forwarded request in-process. The recorder calls it
// on the goroutine that called ServeHTTP, which in these tests is the test's.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls the function.
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// answerWith is a transport answering every request with one response and
// remembering the request it forwarded.
func answerWith(status int, contentType, body string, forwarded **http.Request) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*forwarded = req
		header := http.Header{}
		if contentType != "" {
			header.Set("Content-Type", contentType)
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
}

// TestRecorder_AnOrbitRequest_IsForwardedAndKept verifies what the proxy does
// with an Orbit request: it forwards the route and query to the upstream with
// the credential and nothing that would change the answer's encoding, answers
// the handler with exactly what came back, and keeps the exchange by name.
func TestRecorder_AnOrbitRequest_IsForwardedAndKept(t *testing.T) {
	var forwarded *http.Request
	proxy := newRecorder("https://gitlab.com/", &http.Client{Transport: answerWith(201, "application/json; charset=utf-8", `{"a":1}`, &forwarded)})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v4/orbit/query?b=1&a=2&a=3", strings.NewReader(`{"response_format":"raw","query":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Private-Token", "glpat-x")
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if forwarded.URL.String() != "https://gitlab.com/api/v4/orbit/query?b=1&a=2&a=3" || forwarded.Method != http.MethodPost {
		t.Errorf("forwarded %s %s", forwarded.Method, forwarded.URL)
	}
	if forwarded.Header.Get("Private-Token") != "glpat-x" || forwarded.Header.Get("Content-Type") != "application/json" || forwarded.Header.Get("Accept-Encoding") != "" {
		t.Errorf("forwarded headers = %v", forwarded.Header)
	}
	if rec.Code != 201 || rec.Body.String() != `{"a":1}` || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("answer = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	kept := proxy.take()
	if len(kept) != 1 {
		t.Fatalf("kept %d exchanges, want 1", len(kept))
	}
	got := kept[0]
	if got.method != http.MethodPost || got.path != "/orbit/query" || got.status != 201 || got.contentType != "application/json" || string(got.payload) != `{"a":1}` {
		t.Errorf("exchange = %+v", got)
	}
	if !slices.Equal(got.query, []string{"a", "b"}) || !slices.Equal(got.body, []string{"query", "response_format"}) {
		t.Errorf("names = %q, %q", got.query, got.body)
	}
	if got.describe() != "POST /orbit/query answered 201" {
		t.Errorf("describe() = %q", got.describe())
	}
	if again := proxy.take(); again != nil {
		t.Errorf("take() after take() = %+v, want nothing", again)
	}
}

// TestRecorder_ARequestThatIsNotOrbit_IsForwardedAndNotKept verifies the
// proxy passes through what is not an Orbit route (a client's own health
// probe) without keeping it, and an answer with no media type gets none.
func TestRecorder_ARequestThatIsNotOrbit_IsForwardedAndNotKept(t *testing.T) {
	var forwarded *http.Request
	proxy := newRecorder("https://gitlab.com", &http.Client{Transport: answerWith(200, "", "ok", &forwarded)})
	for _, path := range []string{"/api/v4/version", "/orbit/status"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
			if rec.Code != 200 || rec.Body.String() != "ok" || rec.Header().Get("Content-Type") != "" {
				t.Errorf("answer = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
			}
			if kept := proxy.take(); kept != nil {
				t.Errorf("kept %+v for %s", kept, path)
			}
		})
	}
}

// failingReader fails every read, standing for a connection that broke.
type failingReader struct{}

// Read fails.
func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// TestRecorder_AFailureToForward_IsAnswered502 verifies each way forwarding can
// fail is answered 502 naming the step, which the handler that made the
// request reports as its own failure rather than as an answer to record.
func TestRecorder_AFailureToForward_IsAnswered502(t *testing.T) {
	failing := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("no route to host") })
	unreadable := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(failingReader{}), Request: req}, nil
	})
	cases := []struct {
		name     string
		upstream string
		client   *http.Client
		body     io.Reader
		want     string
	}{
		{name: "the request body", upstream: "https://gitlab.com", client: &http.Client{}, body: failingReader{}, want: "read the request"},
		{name: "a bad upstream", upstream: "https://gitlab.com\x7f", client: &http.Client{}, want: "build the forwarded request"},
		{name: "the upstream", upstream: "https://gitlab.com", client: &http.Client{Transport: failing}, want: "forward the request"},
		{name: "the answer", upstream: "https://gitlab.com", client: &http.Client{Transport: unreadable}, want: "read the answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := newRecorder(tc.upstream, tc.client)
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v4/orbit/status", tc.body))
			if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("answer = %d %q, want 502 and %q", rec.Code, rec.Body.String(), tc.want)
			}
			if kept := proxy.take(); kept != nil {
				t.Errorf("a failed forward was kept: %+v", kept)
			}
		})
	}
}

// TestNames_AreReadOnlyFromWhatCarriesThem verifies the parameter name readers:
// no query is nothing, a body is read only when it is a JSON object with
// something in it, and a media type is read without its parameters.
func TestNames_AreReadOnlyFromWhatCarriesThem(t *testing.T) {
	if got := queryNames(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/orbit/tools", nil)); got != nil {
		t.Errorf("queryNames() = %q, want nil", got)
	}
	bodies := []struct {
		name        string
		contentType string
		body        string
		want        []string
	}{
		{name: "a JSON object", contentType: "application/json", body: `{"b":1,"a":2}`, want: []string{"a", "b"}},
		{name: "a JSON suffix", contentType: "application/vnd.api+json", body: `{"a":1}`, want: []string{"a"}},
		{name: "a form", contentType: "application/x-www-form-urlencoded", body: "a=1"},
		{name: "not JSON", contentType: "application/json", body: "a=1"},
		{name: "an empty object", contentType: "application/json", body: "{}"},
		{name: "an array", contentType: "application/json", body: "[1]"},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			if got := bodyNames(tc.contentType, []byte(tc.body)); !slices.Equal(got, tc.want) {
				t.Errorf("bodyNames() = %q, want %q", got, tc.want)
			}
		})
	}
	media := map[string]string{
		"text/plain; charset=utf-8": "text/plain",
		"Application/JSON":          "application/json",
		"not a media type;;":        "not a media type;;",
	}
	for header, want := range media {
		t.Run(header, func(t *testing.T) {
			if got := mediaType(header); got != want {
				t.Errorf("mediaType(%q) = %q, want %q", header, got, want)
			}
		})
	}
}
