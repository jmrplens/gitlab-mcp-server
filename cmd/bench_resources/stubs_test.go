// stubs_test.go covers the two stand-in servers a scenario runs against.
//
// They exist so a measurement needs no GitLab instance, no credentials and no
// network, which is what lets two people repeat a run and compare this server
// rather than their installations. That only holds if they answer what the
// server under measurement expects, so these tests pin the answers rather than
// the fact that a server started.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// getStub issues a GET against a stand-in server and returns what it answered,
// closing the body before returning so no caller has to.
func getStub(t *testing.T, url string) (status int, contentType string, body []byte) {
	t.Helper()
	//#nosec G107 -- the URL is a loopback stand-in this test just started
	resp, err := http.Get(url) //nolint:noctx // no deadline to carry in a loopback test
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the response from %s: %v", url, err)
	}
	return resp.StatusCode, resp.Header.Get(headerContentType), body
}

// TestStubGitLab_AnswersTheProbesTheServerMakesAtStartup verifies the stand-in
// instance answers version and user, and refuses everything else.
//
// Those two are what the server probes while it builds a catalog. A stand-in
// that answered everything would hide a scenario accidentally reaching for
// real data, and one that answered neither would measure a server retrying
// rather than a server working.
func TestStubGitLab_AnswersTheProbesTheServerMakesAtStartup(t *testing.T) {
	stub := startStubGitLab()
	defer stub.close()

	t.Run("version", func(t *testing.T) {
		status, contentType, body := getStub(t, stub.url+"/api/v4/version")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if !strings.HasPrefix(contentType, mediaJSON) {
			t.Errorf("Content-Type = %q, want JSON", contentType)
		}
		var decoded struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded.Version == "" {
			t.Error("the stand-in reported no version, so the record would carry none")
		}
	})

	t.Run("user", func(t *testing.T) {
		status, _, body := getStub(t, stub.url+"/api/v4/user")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		var decoded struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded.ID == 0 || decoded.Username == "" {
			t.Errorf("the stand-in identified nobody: %+v", decoded)
		}
	})

	t.Run("anything else is refused", func(t *testing.T) {
		// The scope and tier probes land here, and 404 is what "this instance
		// will not say" looks like to every caller that makes them.
		if status, _, _ := getStub(t, stub.url+"/api/v4/projects"); status != http.StatusNotFound {
			t.Errorf("status = %d, want 404", status)
		}
	})

	t.Run("it counts what it was asked", func(t *testing.T) {
		if got := stub.calls.Load(); got < 3 {
			t.Errorf("the stand-in counted %d calls, want at least the three just made", got)
		}
	})
}

// TestOTLPSink_AcceptsAnExportAndMeasuresIt verifies the sink answers what
// stops an exporter retrying, and reports how much it was sent.
//
// The telemetry scenarios measure what exporting costs the server. A sink that
// made the exporter retry would publish the cost of retrying instead, and the
// byte count is the figure that says how much telemetry the server actually
// produced.
func TestOTLPSink_AcceptsAnExportAndMeasuresIt(t *testing.T) {
	sink := startOTLPSink()
	defer sink.close()

	payload := strings.Repeat("x", 4096)
	resp, err := http.Post(sink.url+"/v1/traces", mediaProtobuf, strings.NewReader(payload)) //nolint:noctx // a loopback stand-in, in a test
	if err != nil {
		t.Fatalf("POST to the sink: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 so the exporter does not retry", resp.StatusCode)
	}
	if ct := resp.Header.Get(headerContentType); ct != mediaProtobuf {
		t.Errorf("Content-Type = %q, want %q", ct, mediaProtobuf)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the sink's response: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("the sink returned %d bytes, want the empty export response", len(body))
	}

	if got := sink.requests.Load(); got != 1 {
		t.Errorf("the sink counted %d requests, want 1", got)
	}
	if got := sink.bytes.Load(); got != int64(len(payload)) {
		t.Errorf("the sink measured %d bytes, want %d", got, len(payload))
	}
}

// TestDrain_EmptyAndAbsentBodies_MeasureZero verifies the byte counter handles
// a request with nothing in it, since an exporter's first call can carry an
// empty payload and a nil body is what a hand-built request has.
func TestDrain_EmptyAndAbsentBodies_MeasureZero(t *testing.T) {
	cases := []struct {
		name string
		req  *http.Request
	}{
		{"no body at all", &http.Request{}},
		{"an empty body", func() *http.Request {
			req, err := http.NewRequest(http.MethodPost, "http://sink.invalid/", strings.NewReader("")) //nolint:noctx // no request is sent
			if err != nil {
				panic(err)
			}
			return req
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drain(tc.req); got != 0 {
				t.Errorf("drain = %d, want 0", got)
			}
		})
	}
}
