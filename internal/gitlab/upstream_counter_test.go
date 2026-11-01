// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
)

// The tests in this file read the process's counter, which every request any
// test of this package sends adds to, so none of them is parallel: a top-level
// test that is not parallel runs while no other test of the package does,
// which is what makes a difference between two readings this test's own. None
// of them installs a meter provider either, and nothing in this package does,
// so every one of them also shows the counter counting with telemetry off.

// upstreamTotal is every request the process's counter holds, whatever server
// address it was counted under.
func upstreamTotal() int64 {
	var total int64
	for _, count := range UpstreamRequests() {
		total += count
	}
	return total
}

// countingServer answers every request with status after counting it, and
// redirects the one path it is told to, so a test can compare what the
// instance received with what the process counted.
func countingServer(t *testing.T, status int, redirectFrom, redirectTo string) (url string, hits *atomic.Int64) {
	t.Helper()
	hits = &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if redirectFrom != "" && r.URL.Path == redirectFrom {
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":1,"username":"someone","version":"19.5.0"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, hits
}

// TestUpstreamRequests_TheAPIClient_CountsEveryRetry verifies that a GitLab
// request the SDK re-sends is counted once per attempt.
//
// The per-user and per-IP budgets GitLab keeps are spent by every attempt, so
// a count of the calls handlers make would read a third of what GitLab saw
// from an instance answering 500 to every read. The instance's own count is
// the oracle, because the attempts a read becomes are client-go's business and
// any figure written here would restate its policy rather than check the
// counter.
func TestUpstreamRequests_TheAPIClient_CountsEveryRetry(t *testing.T) {
	srvURL, hits := countingFailureServer(t, http.StatusInternalServerError, -1, false)
	client, err := newTokenClient(srvURL, testValidToken, false, false, testRetryCeiling)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	before := upstreamTotal()
	_, _, _ = client.GL().Projects.GetProject("group/proj", nil)
	counted := upstreamTotal() - before

	if hits.Load() < maxRetries+1 {
		t.Fatalf("the instance saw %d requests, want at least %d: the read was not retried, so this case proves nothing about retries",
			hits.Load(), maxRetries+1)
	}
	if counted != hits.Load() {
		t.Errorf("the process counted %d upstream requests, the instance received %d", counted, hits.Load())
	}
}

// TestUpstreamRequests_TheHealthClient_CountsItsProbes verifies that the
// probes the health client sends are counted, a redirect hop included.
//
// The health client does not go through the SDK's chain, which is why the
// duration histogram never saw it, and a credential probe or a version check
// spends the same budgets a tool call does. A redirect is followed by
// net/http as a request of its own, so it costs GitLab one more and is counted
// as one more.
func TestUpstreamRequests_TheHealthClient_CountsItsProbes(t *testing.T) {
	tests := []struct {
		name     string
		redirect bool
		wantHits int64
	}{
		{name: "one probe is one request", wantHits: 1},
		{name: "a redirect hop is a request of its own", redirect: true, wantHits: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := ""
			if tt.redirect {
				from = "/api/v4/user"
			}
			srvURL, hits := countingServer(t, http.StatusOK, from, "/api/v4/user/again")
			client, err := NewClientWithTokenRetries(srvURL, testValidToken, false, true)
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}

			before := upstreamTotal()
			if got := client.CheckCredential(t.Context()); got != CredentialAccepted {
				t.Fatalf("CheckCredential() = %v, want the stub's acceptance", got)
			}
			counted := upstreamTotal() - before

			if hits.Load() != tt.wantHits {
				t.Fatalf("the instance saw %d requests, want %d", hits.Load(), tt.wantHits)
			}
			if counted != tt.wantHits {
				t.Errorf("the process counted %d upstream requests, the instance received %d", counted, tt.wantHits)
			}
		})
	}
}

// TestHTTPTransport_CountsWhatItSends verifies that the transport lent to a
// caller that makes its own requests counts them too, with and without
// certificate verification.
//
// The OAuth verifier is that caller, and every token it has not seen before
// is a request from the deployment's address: a stream of invented bearer
// tokens is nothing but such requests, all spending the per-IP budget.
func TestHTTPTransport_CountsWhatItSends(t *testing.T) {
	for _, skipTLSVerify := range []bool{false, true} {
		t.Run(map[bool]string{false: "verifying", true: "skipping verification"}[skipTLSVerify], func(t *testing.T) {
			srvURL, hits := countingServer(t, http.StatusOK, "", "")
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srvURL+"/api/v4/user", http.NoBody)
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}

			before := upstreamTotal()
			resp, err := HTTPTransport(skipTLSVerify).RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}
			_ = resp.Body.Close()
			counted := upstreamTotal() - before

			if hits.Load() != 1 || counted != 1 {
				t.Errorf("the instance received %d requests and the process counted %d, want one of each", hits.Load(), counted)
			}
		})
	}
}

// TestUpstreamRequests_ARequestTheGuardRefused_IsNotCounted verifies that a
// request the destination guard refuses is left out of the count, whichever
// transport carried it.
//
// The guard refuses before a connection exists, so the request never left the
// process and spent nothing GitLab keeps. Counting it would let anyone who can
// name a metadata address raise a figure an operator reads as GitLab traffic.
func TestUpstreamRequests_ARequestTheGuardRefused_IsNotCounted(t *testing.T) {
	const metadata = "http://169.254.169.254"

	client, err := NewClientWithTokenRetries(metadata, testValidToken, false, true)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	tests := []struct {
		name string
		send func(*http.Request) error
	}{
		{name: "the client's own chain", send: func(*http.Request) error {
			return client.CheckCredentialDetail(t.Context()).Err
		}},
		{name: "the lent transport", send: func(req *http.Request) error {
			resp, sendErr := HTTPTransport(false).RoundTrip(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			return sendErr
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, metadata+"/latest/meta-data/", http.NoBody)
			if reqErr != nil {
				t.Fatalf("building the request: %v", reqErr)
			}

			before := upstreamTotal()
			sendErr := tt.send(req)
			counted := upstreamTotal() - before

			if !errors.Is(sendErr, ErrDestinationRefused) {
				t.Fatalf("the request was not refused by the destination guard: %v", sendErr)
			}
			if counted != 0 {
				t.Errorf("the process counted %d upstream requests for one that never left it", counted)
			}
		})
	}
}

// TestUpstreamRequests_NamesOnlyTheServerAddress verifies what the counter is
// keyed on: the server address the metric may name, and nothing about who
// sent the request or what it asked for.
//
// The count is the process's (INV-018), so a credential, a user or a path on
// it would split one budget into series nobody keeps, and would put on a
// metric the identifiers this server declines to record anywhere else.
func TestUpstreamRequests_NamesOnlyTheServerAddress(t *testing.T) {
	const (
		token = "glpat-a-credential-of-its-own"
		path  = "/api/v4/projects/secret-group%2Fsecret-project"
	)
	t.Cleanup(func() { mcpotel.SetMetricServerAddresses(nil) })
	mcpotel.SetMetricServerAddresses([]string{"127.0.0.1"})

	srvURL, hits := countingServer(t, http.StatusOK, "", "")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srvURL+path, http.NoBody)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	before := UpstreamRequests()["127.0.0.1"]
	resp, err := HTTPTransport(false).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}
	_ = resp.Body.Close()

	reading := UpstreamRequests()
	if got := reading["127.0.0.1"] - before; got != hits.Load() {
		t.Errorf("the declared host counted %d requests, the instance received %d", got, hits.Load())
	}
	for address := range reading {
		if address != "127.0.0.1" && address != mcpotel.OtherServerAddress {
			t.Errorf("the counter is keyed on %q, which is neither the declared host nor the other bucket", address)
		}
		if strings.Contains(address, token) || strings.Contains(address, "secret") || strings.Contains(address, "someone") {
			t.Errorf("the counter's key %q carries the credential, the path or the user", address)
		}
	}
}

// TestUpstreamCounter_AnUndeclaredHost_AddsNoKeyOfItsOwn verifies that the
// counter's keys are bounded however many hosts are named.
//
// Under --allow-any-gitlab-url a caller names the instance, so a key per host
// would be a table a caller grows without bound, one entry per name it
// invents. Every undeclared host is counted under the other bucket instead.
func TestUpstreamCounter_AnUndeclaredHost_AddsNoKeyOfItsOwn(t *testing.T) {
	t.Cleanup(func() { mcpotel.SetMetricServerAddresses(nil) })
	mcpotel.SetMetricServerAddresses([]string{"gitlab.example.com"})

	counter := newUpstreamCounter()
	for _, host := range []string{"gitlab.example.com", "one.invalid", "two.invalid", "three.invalid"} {
		counter.add(host)
	}

	reading := counter.reading()
	if want := map[string]int64{"gitlab.example.com": 1, mcpotel.OtherServerAddress: 3}; !maps.Equal(reading, want) {
		t.Errorf("the counter holds %v, want %v", reading, want)
	}

	reading[mcpotel.OtherServerAddress] = 0
	if again := counter.reading(); again[mcpotel.OtherServerAddress] != 3 {
		t.Error("changing a reading changed the counter, so a caller could rewrite what the process sent")
	}
}
