//go:build collectore2e

package collectore2e

import (
	"testing"
)

// TestRealCollector_StatefulSessionIsNamedAndMeasured covers the two things
// that only exist when a deployment has sessions, and that every other case in
// this module asserts the absence of.
//
// The absence is the easy half and it is what was covered: under the default
// stateless transport each POST is its own session with no id, so the
// convention's condition is not met and neither the attribute nor the
// instrument should appear. A rule with only its negative tested passes just as
// well when the positive never happens either.
func TestRealCollector_StatefulSessionIsNamedAndMeasured(t *testing.T) {
	c := startCollector(t)
	// --stateless=false, and the session is driven over revision 2025-11-25:
	// 2026-07-28 is stateless only, so a deployment with sessions does not
	// advertise it and refuses a client that insists. That the transports
	// diverge this way is asserted in test/e2e/http, which is where transport
	// behavior belongs; here it is only the reason the handshake looks
	// different from every other call in this module.
	srv := startServer(t, telemetryEnv(c),
		"--gitlab-url="+startFakeGitLab(t),
		"--stateless=false",
	)

	sess := srv.openSession(t)
	for i := range 3 {
		sess.call(t, i+2, "tools/call",
			`{"name":"gitlab_execute_action","arguments":{"action":"issue.list",`+
				`"params":{"project_id":"some-group/some-project"}}}`)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return s.Name == "tools/call gitlab_execute_action"
	})
	if !ok {
		t.Fatalf("no tools/call span on a stateful deployment.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	t.Run("the span names the session", func(t *testing.T) {
		id, present := attr(span.Attributes, "mcp.session.id")
		if !present {
			t.Fatalf("mcp.session.id is absent while a session exists; recorded %v", keys(span.Attributes))
		}
		if id != sess.id {
			t.Errorf("mcp.session.id = %q, want the id the server issued (%q)", id, sess.id)
		}
	})

	t.Run("the protocol version is the one negotiated", func(t *testing.T) {
		// Not the newest this build supports. The value is the revision this
		// session actually speaks, and reporting the build's own would make the
		// attribute a constant.
		if got, _ := attr(span.Attributes, "mcp.protocol.version"); got != legacyProtocolVersion {
			t.Errorf("mcp.protocol.version = %q, want %q", got, legacyProtocolVersion)
		}
	})

	// Ending the session is what produces the measurement: the instrument is
	// recorded from a goroutine parked on the session's own lifetime, so an
	// abandoned session would only be measured when the idle timeout fired.
	sess.close(t)

	t.Run("the session duration is recorded", func(t *testing.T) {
		_, duration, found := c.awaitMetric(t, exportDeadline, "mcp.server.session.duration")
		if !found {
			t.Fatalf("mcp.server.session.duration was never recorded.\nCollector:\n%s\nServer:\n%s",
				c.containerLogs(t), srv.logs())
		}
		if duration.Unit != "s" {
			t.Errorf("unit = %q, want %q", duration.Unit, "s")
		}

		points := dataPointAttributes(t, duration)
		if len(points) == 0 {
			t.Fatal("the instrument arrived with no data points")
		}
		for _, point := range points {
			// The convention's own instrument table omits the session id, and
			// it has to: one value per connected client is a series count that
			// grows with the number of people using the deployment.
			if value, present := attr(point, "mcp.session.id"); present {
				t.Errorf("mcp.session.id = %q is a dimension of the session instrument", value)
			}
		}
	})
}
