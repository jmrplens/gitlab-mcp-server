// retry_test.go contains unit tests for the bounded retry and backoff policy
// this package imposes on client-go.
package gitlab

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// unixIn renders a Unix timestamp d from now, as the RateLimit-Reset header
// carries it.
func unixIn(d time.Duration) string {
	return strconv.FormatInt(time.Now().Add(d).Unix(), 10)
}

// writeProbeName is the project name the write half of the retry table sends,
// held in a variable because the SDK option takes a pointer.
var writeProbeName = "probe"

// TestClampedBackoff_NeverWaitsPastTheCeiling verifies the wait between
// attempts is bounded whatever the upstream says.
//
// client-go sets the wait to time.Until(RateLimit-Reset) whenever that is
// longer than its minimum and clamps it with nothing, so a 429 naming a reset
// an hour ahead parks the calling goroutine for an hour — with no
// http.Client.Timeout, no handler deadline and nothing else in the chain to
// stop it. The table walks the resets an upstream can send, honest and
// otherwise, and the invariant is the same for all of them.
func TestClampedBackoff_NeverWaitsPastTheCeiling(t *testing.T) {
	tests := []struct {
		name    string
		status  int // zero means no response at all
		reset   string
		attempt int
		wantMin time.Duration
	}{
		{name: "no response at all", wantMin: retryBackoffStep},
		{name: "server error", status: http.StatusInternalServerError, wantMin: retryBackoffStep},
		{name: "server error on a later attempt", status: http.StatusInternalServerError, attempt: 3, wantMin: 4 * retryBackoffStep},
		{name: "rate limited with no reset header", status: http.StatusTooManyRequests, wantMin: retryBackoffStep},
		{name: "rate limited resetting in a second", status: http.StatusTooManyRequests, reset: unixIn(time.Second), wantMin: retryBackoffStep},
		{name: "rate limited resetting in an hour", status: http.StatusTooManyRequests, reset: unixIn(time.Hour), wantMin: maxRetryBackoff},
		{name: "rate limited resetting in a year", status: http.StatusTooManyRequests, reset: unixIn(365 * 24 * time.Hour), wantMin: maxRetryBackoff},
		{name: "rate limited with a reset already past", status: http.StatusTooManyRequests, reset: "1", wantMin: retryBackoffStep},
		{name: "rate limited with an unparseable reset", status: http.StatusTooManyRequests, reset: "soon please", wantMin: retryBackoffStep},
		{name: "rate limited with a negative reset", status: http.StatusTooManyRequests, reset: "-9", wantMin: retryBackoffStep},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.status != 0 {
				resp = &http.Response{StatusCode: tt.status, Header: http.Header{}}
				if tt.reset != "" {
					resp.Header.Set(rateLimitResetHeader, tt.reset)
				}
			}

			got := clampedBackoff(100*time.Millisecond, 400*time.Millisecond, tt.attempt, resp)

			if got > maxRetryBackoff {
				t.Errorf("clampedBackoff() = %v, want no more than %v", got, maxRetryBackoff)
			}
			if got < tt.wantMin {
				t.Errorf("clampedBackoff() = %v, want at least %v", got, tt.wantMin)
			}
		})
	}
}

// TestClampedBackoff_RespectsTheCallersMinimum verifies a caller asking for a
// longer minimum wait than the policy's own step still gets it, up to the
// ceiling, so the policy narrows client-go's behavior without contradicting
// the arguments it is handed.
func TestClampedBackoff_RespectsTheCallersMinimum(t *testing.T) {
	tests := []struct {
		name    string
		minWait time.Duration
		maxWait time.Duration
		wantMin time.Duration
	}{
		{name: "minimum below the step", minWait: 10 * time.Millisecond, maxWait: 20 * time.Millisecond, wantMin: retryBackoffStep},
		{name: "minimum above the step", minWait: 2 * time.Second, maxWait: 2 * time.Second, wantMin: 2 * time.Second},
		{name: "minimum above the ceiling", minWait: time.Hour, maxWait: time.Hour, wantMin: maxRetryBackoff},
		{name: "no jitter window", minWait: time.Second, maxWait: time.Second, wantMin: time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampedBackoff(tt.minWait, tt.maxWait, 0, nil)

			if got > maxRetryBackoff {
				t.Errorf("clampedBackoff() = %v, want no more than %v", got, maxRetryBackoff)
			}
			if got < tt.wantMin {
				t.Errorf("clampedBackoff() = %v, want at least %v", got, tt.wantMin)
			}
		})
	}
}

// TestRateLimitResetWait_ReadsTheHeader verifies how the RateLimit-Reset
// header is turned into a wait, including every shape that must yield none.
func TestRateLimitResetWait_ReadsTheHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		wantZero bool
	}{
		{name: "absent", header: "", wantZero: true},
		{name: "not a number", header: "later", wantZero: true},
		{name: "zero", header: "0", wantZero: true},
		{name: "negative", header: "-1", wantZero: true},
		{name: "in the past", header: "1000", wantZero: true},
		{name: "in the future", header: unixIn(time.Hour)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set(rateLimitResetHeader, tt.header)
			}

			got := rateLimitResetWait(resp)

			if (got == 0) != tt.wantZero {
				t.Errorf("rateLimitResetWait(%q) = %v, want zero = %v", tt.header, got, tt.wantZero)
			}
			if got < 0 {
				t.Errorf("rateLimitResetWait(%q) = %v, want no negative wait", tt.header, got)
			}
		})
	}
}

// TestRetryOptions_AreAppliedToTheSDKClient verifies the policy is accepted by
// client-go's option chain, so a change to the SDK that rejects one of these
// options fails here rather than silently leaving the defaults in place.
func TestRetryOptions_AreAppliedToTheSDKClient(t *testing.T) {
	tests := []struct {
		name  string
		build func(opts []gl.ClientOptionFunc) (*gl.Client, error)
	}{
		{name: "token client", build: func(opts []gl.ClientOptionFunc) (*gl.Client, error) {
			return gl.NewClient(testValidToken, opts...)
		}},
		{name: "auth source client", build: func(opts []gl.ClientOptionFunc) (*gl.Client, error) {
			return gl.NewAuthSourceClient(gl.AccessTokenAuthSource{Token: testValidToken}, opts...)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]gl.ClientOptionFunc{gl.WithBaseURL("https://gitlab.example.com")}, retryOptions()...)
			client, err := tt.build(opts)
			if err != nil {
				t.Fatalf("building a client with the retry options: %v", err)
			}
			if client == nil {
				t.Fatal("client is nil")
			}
		})
	}
}

// countingFailureServer answers with failStatus (and optionally a far-future
// RateLimit-Reset) for the first failures requests and 200 afterwards, and
// counts every request it receives.
func countingFailureServer(t *testing.T, failStatus, failures int, rateLimited bool) (url string, hits *atomic.Int64) {
	t.Helper()
	hits = &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if failures < 0 || n <= int64(failures) {
			if rateLimited {
				w.Header().Set(rateLimitResetHeader, strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
			}
			w.WriteHeader(failStatus)
			fmt.Fprint(w, `{"message":"nope"}`)
			return
		}
		fmt.Fprint(w, `{"id":1,"path_with_namespace":"group/proj"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, hits
}

// TestClient_RetryBudget_IsBounded verifies that one tool call cannot become
// an unbounded number of upstream requests, nor park its goroutine for hours.
//
// Three things are pinned. A failing read is re-sent [maxRetries] times and no
// more, so client-go's default of five — one call, six upstream requests, a
// latency floor around twelve seconds — cannot come back. A failing write is
// not re-sent at all, because the server may already have applied it. And a
// 429 naming a reset an hour ahead costs a bounded wait rather than an hour,
// which is the case with no upper bound at all before the clamp.
//
// The elapsed-time assertions are deliberately loose: they are an order of
// magnitude away from both the fixed and the unfixed behavior, so they
// discriminate without turning CI load into a failure.
func TestClient_RetryBudget_IsBounded(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		failures    int
		rateLimited bool
		write       bool
		wantHits    int64
		maxElapsed  time.Duration
	}{
		{name: "server error on a read is retried a bounded number of times", status: http.StatusInternalServerError, failures: -1, wantHits: maxRetries + 1, maxElapsed: 8 * time.Second},
		{name: "server error on a write is not retried", status: http.StatusInternalServerError, failures: -1, write: true, wantHits: 1, maxElapsed: 8 * time.Second},
		{name: "rate limit reset an hour ahead does not park the call", status: http.StatusTooManyRequests, failures: 1, rateLimited: true, wantHits: 2, maxElapsed: 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvURL, hits := countingFailureServer(t, tt.status, tt.failures, tt.rateLimited)

			client, err := NewClientWithTokenRetries(srvURL, testValidToken, false, false)
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}

			// context.Background() on purpose: retryablehttp's sleep selects
			// on the request context, so a test that supplies a deadline
			// measures its own deadline and passes on unfixed code.
			start := time.Now()
			if tt.write {
				_, _, _ = client.GL().Projects.CreateProject(&gl.CreateProjectOptions{Name: &writeProbeName})
			} else {
				_, _, _ = client.GL().Projects.GetProject("group/proj", nil)
			}
			elapsed := time.Since(start)

			if got := hits.Load(); got != tt.wantHits {
				t.Errorf("upstream saw %d requests, want %d", got, tt.wantHits)
			}
			if elapsed > tt.maxElapsed {
				t.Errorf("call took %v, want no more than %v", elapsed, tt.maxElapsed)
			}
		})
	}
}
