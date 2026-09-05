//go:build httpe2e

// health_test.go drives /health on the real binary: what it reports about the
// build and the configuration, and how it answers while the process drains.
//
// The unit tests prove the handler and the digest in isolation. What they
// cannot see is the wiring a deployment relies on: that the digest served is
// computed from the configuration the flags resolved to, and that SIGTERM
// flips the endpoint before the listener closes, which only a process that
// receives a real signal can show.
package httpe2e

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
	"time"
)

// healthBody is the subset of the /health body these tests read.
type healthBody struct {
	Status       string `json:"status"`
	Build        string `json:"build"`
	ConfigDigest string `json:"config_digest"`
}

// getHealth fetches /health from a running server and decodes its body,
// returning the status code and the response headers alongside it.
func getHealth(t *testing.T, s *server) (int, http.Header, healthBody) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.baseURL+"/health", http.NoBody)
	if err != nil {
		t.Fatalf("building the health request: %v", err)
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	var body healthBody
	if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr != nil {
		t.Fatalf("decoding /health: %v", decodeErr)
	}
	return resp.StatusCode, resp.Header, body
}

var hexDigest = regexp.MustCompile(`^[0-9a-f]{12}$`)

// TestHealth_ReportsBuildAndConfigDigest pins that a serving instance answers
// 200 ok with a displayable build label and a twelve-character digest of the
// configuration that shapes what it serves.
func TestHealth_ReportsBuildAndConfigDigest(t *testing.T) {
	t.Parallel()

	s := startServer(t, nil)
	code, _, body := getHealth(t, s)
	if code != http.StatusOK || body.Status != "ok" {
		t.Fatalf("status %d body %q, want 200 ok", code, body.Status)
	}
	if body.Build == "" {
		t.Errorf("build is empty; a monitor has nothing to display for this instance")
	}
	if !hexDigest.MatchString(body.ConfigDigest) {
		t.Errorf("config_digest = %q, want twelve hex characters", body.ConfigDigest)
	}
}

// TestHealth_ConfigDigest_TellsDifferentlyConfiguredInstancesApart pins the
// use the digest exists for: two instances configured alike report the same
// digest, and one that serves a different catalog reports another, so a
// monitor comparing the nodes behind one balancer can spot the odd one out
// without any of them publishing its configuration.
func TestHealth_ConfigDigest_TellsDifferentlyConfiguredInstancesApart(t *testing.T) {
	t.Parallel()

	digestOf := func(flags ...string) string {
		_, _, body := getHealth(t, startServer(t, nil, flags...))
		return body.ConfigDigest
	}

	dynamic := digestOf("--tool-surface=dynamic")
	if again := digestOf("--tool-surface=dynamic"); again != dynamic {
		t.Errorf("two instances configured alike report %q and %q", dynamic, again)
	}
	if meta := digestOf("--tool-surface=meta"); meta == dynamic {
		t.Errorf("a meta-surface instance reports the dynamic surface's digest %q", meta)
	}
	if readOnly := digestOf("--tool-surface=dynamic", "--read-only"); readOnly == dynamic {
		t.Errorf("a read-only instance reports the writable instance's digest %q", readOnly)
	}
}

// TestHealth_Draining_AnswersServiceUnavailableBeforeTheListenerCloses pins
// the shutdown announcement: with --drain-delay set, SIGTERM makes /health
// answer 503 draining, uncacheable, while the listener stays open for the
// delay, and the process still exits cleanly afterwards.
//
// Without the delay the first thing a balancer notices is the closed
// listener, one probe later, and every request it sent in that window
// failed. The delay is what lets the probe see the flip first, and this test
// is the only place that observes the flip on a process rather than a
// handler.
func TestHealth_Draining_AnswersServiceUnavailableBeforeTheListenerCloses(t *testing.T) {
	t.Parallel()

	const delay = 3 * time.Second
	s := shutdownStartServer(t, "--allow-any-gitlab-url", "--drain-delay="+delay.String())

	signaled := time.Now()
	if err := signalTermination(s.cmd.Process); err != nil {
		t.Fatalf("sending %s: %v", terminationSignalName, err)
	}

	// The flip lands within milliseconds; the deadline is generous because a
	// loaded machine must not decide the outcome.
	var (
		code   int
		header http.Header
		body   healthBody
	)
	deadline := time.Now().Add(delay / 2)
	for {
		code, header, body = getHealth(t, s.probe)
		if code == http.StatusServiceUnavailable || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if code != http.StatusServiceUnavailable || body.Status != "draining" {
		t.Fatalf("after SIGTERM /health answered %d %q, want 503 draining:\n%s", code, body.Status, s.logs())
	}
	if header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q while draining, want no-store", header.Get("Cache-Control"))
	}

	reaped := make(chan struct{})
	go func() {
		_ = s.wait()
		close(reaped)
	}()
	select {
	case <-reaped:
	case <-time.After(delay + shutdownGrace):
		t.Fatalf("the server was still running %s after SIGTERM:\n%s", delay+shutdownGrace, s.logs())
	}
	if elapsed := time.Since(signaled); elapsed < delay {
		t.Errorf("the process exited %s after SIGTERM, before the %s drain delay had passed", elapsed, delay)
	}
	if exit := s.cmd.ProcessState.ExitCode(); exit != 0 {
		t.Errorf("exit status %d after a drained shutdown, want 0:\n%s", exit, s.logs())
	}
}
