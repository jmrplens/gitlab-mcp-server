// stubs_test.go covers the two stand-in servers a scenario runs against.
//
// They exist so a measurement needs no GitLab instance, no credentials and no
// network, which is what lets two people repeat a run and compare this server
// rather than their installations. That only holds if they answer what the
// server under measurement expects, so these tests pin the answers rather than
// the fact that a server started.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// getStub issues a GET against a stand-in server and returns what it answered,
// closing the body before returning so no caller has to.
func getStub(t *testing.T, url string) (status int, contentType string, body []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
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

// getStubWithin is getStub for a goroutine that is not the test's: it touches
// no *testing.T and gives up after limit, reporting a read it could not make
// as status 0.
func getStubWithin(url string, limit time.Duration) (status int, contentType string, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, "", nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil
	}
	return resp.StatusCode, resp.Header.Get(headerContentType), body
}

// TestDrain_ABodyLongerThanOneRead_IsCountedWhole verifies a body several
// reads long is read to its end and counted whole, and that the read ends.
//
// An export batch runs to hundreds of kilobytes, so the sink's byte count is
// the sum of many reads; the watchdog is there because a read loop that can
// stop advancing is one that holds the sink's handler, and with it the
// exporter, for the rest of the run. First in the file so that a loop that
// stopped advancing is reported here, by name, rather than as a hang in the
// sink's own test below.
func TestDrain_ABodyLongerThanOneRead_IsCountedWhole(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "a body of many reads", body: strings.Repeat("z", 100_000)},
		{name: "an empty body", body: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://sink.invalid/", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("build the request: %v", err)
			}
			var got int64
			finishWithin(t, 10*time.Second, "draining the body", func() { got = drain(req) })
			if got != int64(len(tc.body)) {
				t.Errorf("drain = %d, want the body's %d bytes", got, len(tc.body))
			}
		})
	}
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

// TestStubGitLab_ProjectRead_WaitsOnTheHoldAfterItsHeaders verifies the one
// read the held-request mode makes the stand-in keep waiting: answered at once
// while no hold is on, held with its headers already sent while one is, let
// through when the hold is released, and given up on when its caller leaves.
//
// The headers going first is the property the mode depends on. The server
// gives GitLab a minute to answer with headers and no bound on the body, so a
// read held before its headers would be abandoned and retried after a minute,
// and a step would measure the retry rather than the hold.
func TestStubGitLab_ProjectRead_WaitsOnTheHoldAfterItsHeaders(t *testing.T) {
	stub := startStubGitLab()
	defer stub.close()

	t.Run("no hold answers at once", func(t *testing.T) {
		// Bounded, because a stub that held a read with no hold on would keep
		// it until its caller left, and the test would hang rather than fail.
		var status int
		var contentType string
		var body []byte
		finishWithin(t, 10*time.Second, "a project read with no hold on", func() {
			status, contentType, body = getStubWithin(stub.url+"/api/v4/projects/1", 5*time.Second)
		})
		if status != http.StatusOK || !strings.HasPrefix(contentType, mediaJSON) {
			t.Fatalf("status = %d, Content-Type = %q, want a 200 in JSON", status, contentType)
		}
		var decoded struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil || decoded.ID != 1 {
			t.Errorf("body = %s (%v), want project 1", body, err)
		}
		if stub.holding() != 0 {
			t.Errorf("holding = %d with no hold on, want 0", stub.holding())
		}
	})

	t.Run("a hold keeps the body until it is released", func(t *testing.T) {
		release := stub.hold()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, stub.url+"/api/v4/projects/1", http.NoBody)
		if err != nil {
			t.Fatalf("build the request: %v", err)
		}
		// The stub flushes its headers before it waits, so this returns while
		// the hold is still on; a stub that held them would hang it here.
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			release()
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d before the release, want the 200 sent ahead of the hold", resp.StatusCode)
		}
		waitFor(t, func() bool { return stub.holding() == 1 }, "the read to be counted as held")

		read := make(chan []byte, 1)
		go func() {
			body, _ := io.ReadAll(resp.Body)
			read <- body
		}()
		select {
		case body := <-read:
			t.Errorf("the body arrived while the hold was on: %s", body)
		case <-time.After(50 * time.Millisecond):
		}
		release()
		select {
		case body := <-read:
			if !strings.Contains(string(body), `"id":1`) {
				t.Errorf("body = %s, want project 1", body)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the body never arrived after the release")
		}
		waitFor(t, func() bool { return stub.holding() == 0 }, "the released read to stop being counted")
	})

	t.Run("a caller that leaves is not held", func(t *testing.T) {
		release := stub.hold()
		defer release()
		ctx, cancel := context.WithCancel(t.Context())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, stub.url+"/api/v4/projects/1", http.NoBody)
		if err != nil {
			cancel()
			t.Fatalf("build the request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			t.Fatalf("GET: %v", err)
		}
		waitFor(t, func() bool { return stub.holding() == 1 }, "the read to be counted as held")
		cancel()
		_ = resp.Body.Close()
		waitFor(t, func() bool { return stub.holding() == 0 }, "the abandoned read to stop being counted")
	})
}

// waitFor polls cond until it holds, failing the test after ten seconds with
// what it was waiting for.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, sink.url+"/v1/traces", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build the export request: %v", err)
	}
	req.Header.Set(headerContentType, mediaProtobuf)
	resp, err := http.DefaultClient.Do(req)
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
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://sink.invalid/", strings.NewReader(""))
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
