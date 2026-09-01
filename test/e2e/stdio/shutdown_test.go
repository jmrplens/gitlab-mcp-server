//go:build stdioe2e

package stdioe2e

import (
	"testing"
	"time"
)

// TestShutdown_IsNotBlockedByAnOpenSubscription pins that a server with a live
// subscription still stops when it is told to.
//
// "The server SHOULD send a completion result for any active
// subscriptions/listen requests before shutting down."
//
// A subscription at 2026-07-28 is a request the client leaves open, and the
// SDK's handler blocks on it until its context ends. Nothing ended those
// contexts on shutdown, so SIGTERM did not stop the process: it kept running
// with the stream held, and had to be killed. That is worse than the missing
// completion result the clause is about — a server that ignores SIGTERM is one
// a supervisor will eventually SIGKILL, mid-write.
//
// The control case is the point of comparison: without a listen open the same
// server stops promptly, so a failure here is attributable to the subscription
// rather than to shutdown in general.
func TestShutdown_IsNotBlockedByAnOpenSubscription(t *testing.T) {
	const grace = 20 * time.Second

	t.Run("with no subscription open", func(t *testing.T) {
		gitlab := startFakeGitLab(t)
		env := baseEnv(gitlab.URL)
		env["CAPABILITY_SURFACE"] = "full"
		s := startSession(t, env)

		if got := s.call(t, request(1, "tools/list", "")); got["error"] != nil {
			t.Fatalf("tools/list failed: %v", got["error"])
		}
		if !s.terminate(t, grace) {
			t.Fatalf("the server ignored SIGTERM with nothing open:\n%s", s.stderrText())
		}
	})

	t.Run("with a subscription open", func(t *testing.T) {
		gitlab := startFakeGitLab(t)
		env := baseEnv(gitlab.URL)
		env["CAPABILITY_SURFACE"] = "full"
		s := startSession(t, env)

		// The listen is a request that stays open; its acknowledgment arrives
		// as a notification rather than a response to it.
		s.send(t, request(7, "subscriptions/listen",
			`{"notifications":{"resourceSubscriptions":["gitlab://project/42"]}}`))

		deadline := time.Now().Add(30 * time.Second)
		acknowledged := false
		for time.Now().Before(deadline) && !acknowledged {
			got := s.readMessage(t, 30*time.Second)
			if method, _ := got["method"].(string); method == "notifications/subscriptions/acknowledged" {
				acknowledged = true
			}
			if got["error"] != nil {
				t.Fatalf("the subscription was refused, so this case is not testing what it says: %v", got["error"])
			}
		}
		if !acknowledged {
			t.Fatalf("the subscription was never acknowledged:\n%s", s.stderrText())
		}

		if !s.terminate(t, grace) {
			t.Fatalf("the server ignored SIGTERM with a subscription open; it had to be killed:\n%s", s.stderrText())
		}
	})
}
