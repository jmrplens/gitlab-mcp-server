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

// TestShutdown_ExitStatusSaysItWasClean pins that an ordinary stop is reported
// as one.
//
// "Servers SHOULD exit promptly when their standard input is closed or reads
// return end-of-file. This is the primary graceful-shutdown signal and the only
// portable one."
//
// The binding says nothing about exit status, so this is not a conformance
// case. It is a truthfulness one: the two shutdowns the binding prescribes are
// the two a supervisor sees most, and announcing either as a failure is a
// report nobody can act on. systemd's Restart=on-failure restarts on it, a CI
// wrapper fails the job on it, and a launcher that propagates its child's status
// passes it outward. The repository already states the intent against itself:
// the comment on TestRunWithContext_PingFailure_StartsInDegradedMode says "the
// transport now treats EOF as the ordinary end of a session (a client closing
// its pipe is not a failure)", which held only while nothing was in flight.
//
// The idle-EOF case is the control: it passed before this test existed, so a
// failure in the others is attributable to what they add rather than to
// shutdown in general.
func TestShutdown_ExitStatusSaysItWasClean(t *testing.T) {
	const grace = 20 * time.Second

	tests := []struct {
		name string
		// subscriptions turns on the capability surface the listen case needs.
		subscriptions bool
		// occupy leaves the server with something open, or is nil for an idle
		// one. It must not return until that thing is genuinely in flight.
		occupy func(t *testing.T, s *session, gitlab *fakeGitLab)
		// stop performs the shutdown under test and returns its exit status.
		stop func(t *testing.T, s *session) (int, bool)
	}{
		{
			// The control: this passed before the fix, so a failure in any
			// other case is attributable to what that case adds rather than
			// to shutdown in general.
			name: "stdin closed with nothing in flight",
			stop: func(t *testing.T, s *session) (int, bool) {
				t.Helper()
				return s.closeStdinAndWait(t, grace)
			},
		},
		{
			name: "stdin closed with a tool call in flight",
			occupy: func(t *testing.T, s *session, gitlab *fakeGitLab) {
				t.Helper()
				// Project 99 never answers, so the call is still open when the
				// pipe goes. A client closing stdin while a call is running is
				// the everyday case, not an exotic one: it is what happens when
				// the client exits.
				s.send(t, request(2, "tools/call",
					`{"name":"gitlab_execute_action","arguments":{"action":"project.get","params":{"project_id":"99"}}}`))
				// Waiting for the request to arrive rather than sleeping. A
				// fixed delay asserts on the scheduler: if the call had not
				// reached GitLab yet, this would be the idle case wearing the
				// name of the interesting one, and would pass either way.
				gitlab.awaitInFlightCall(t, 30*time.Second)
			},
			stop: func(t *testing.T, s *session) (int, bool) {
				t.Helper()
				return s.closeStdinAndWait(t, grace)
			},
		},
		{
			name:          "stdin closed with a subscription open",
			subscriptions: true,
			occupy: func(t *testing.T, s *session, _ *fakeGitLab) {
				t.Helper()
				s.send(t, request(7, "subscriptions/listen",
					`{"notifications":{"resourceSubscriptions":["gitlab://project/42"]}}`))
				awaitAcknowledgement(t, s)
			},
			stop: func(t *testing.T, s *session) (int, bool) {
				t.Helper()
				return s.closeStdinAndWait(t, grace)
			},
		},
		{
			name: "SIGTERM with nothing in flight",
			stop: func(t *testing.T, s *session) (int, bool) {
				t.Helper()
				return s.terminateAndWait(t, grace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitlab := startFakeGitLab(t)
			env := baseEnv(gitlab.URL)
			if tt.subscriptions {
				env["CAPABILITY_SURFACE"] = "full"
			}
			s := startSession(t, env)

			// A completed call first, so what follows is stopping a server that
			// is actually serving.
			if got := s.call(t, request(1, "tools/list", "")); got["error"] != nil {
				t.Fatalf("tools/list failed: %v", got["error"])
			}
			if tt.occupy != nil {
				tt.occupy(t, s, gitlab)
			}

			code, exited := tt.stop(t, s)
			if !exited {
				t.Fatalf("the server did not exit:\n%s", s.stderrText())
			}
			if code != 0 {
				t.Errorf("exit status %d, want 0:\n%s", code, s.stderrText())
			}
		})
	}
}

// awaitAcknowledgement blocks until the server confirms a subscription.
func awaitAcknowledgement(t *testing.T, s *session) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		got := s.readMessage(t, 30*time.Second)
		if got["error"] != nil {
			t.Fatalf("the subscription was refused, so this case is not testing what it says: %v", got["error"])
		}
		if method, _ := got["method"].(string); method == "notifications/subscriptions/acknowledged" {
			return
		}
	}
	t.Fatalf("the subscription was never acknowledged:\n%s", s.stderrText())
}
