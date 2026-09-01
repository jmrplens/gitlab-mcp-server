//go:build collectore2e

package collectore2e

import (
	"strings"
	"testing"
	"time"
)

// TestRealCollector_AServerInitiatedNotificationIsAClientSpan covers the half of
// the convention's split that nothing here reached, and the instrument that
// went with it.
//
// The split is by initiator, not by role: this process is the receiver for a
// tools/call and the initiator for a notification, so it produces both kinds of
// span with different rules. Every case in this module had driven only the
// receiving half, so mcp.client.operation.duration was documented, implemented,
// and exercised by nothing.
//
// A notification is the cheap way in. An elicitation needs a client that
// answers, which this module's caller is not; a notification is sent and not
// waited on, so a subscription noticing a change produces one with no client
// cooperation at all.
func TestRealCollector_AServerInitiatedNotificationIsAClientSpan(t *testing.T) {
	c := startCollector(t)
	fake := startMutableFakeGitLab(t)
	srv := startServer(t, telemetryEnv(c),
		"--gitlab-url="+fake.URL(),
		// Subscriptions are the legacy path and need a session.
		"--stateless=false",
	)

	sess := srv.openSession(t)
	sess.call(t, 2, "resources/subscribe", `{"uri":"`+resourceURI+`"}`)

	// The watcher notices on its next poll, and the base cadence is 15 seconds,
	// so this is the one case here that is slow by design rather than by
	// accident.
	fake.change("changed after the subscription was established")

	_, span, ok := c.awaitSpan(t, 90*time.Second, func(_ otlpResourceSpans, s otlpSpan) bool {
		return s.Kind == spanKindClient && strings.HasPrefix(s.Name, "notifications/")
	})
	if !ok {
		t.Fatalf("no client span for a server-initiated notification.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	t.Run("the span is named for the method and is CLIENT kind", func(t *testing.T) {
		if got, _ := attr(span.Attributes, "mcp.method.name"); !strings.HasPrefix(got, "notifications/") {
			t.Errorf("mcp.method.name = %q, want a notifications/ method", got)
		}
		// SERVER would be the natural default and is wrong here. The convention
		// splits by who initiated, and this process initiated.
		if span.Kind != spanKindClient {
			t.Errorf("kind = %d, want %d for something this server initiated", span.Kind, spanKindClient)
		}
	})

	t.Run("the client instrument is recorded", func(t *testing.T) {
		_, duration, found := c.awaitMetric(t, exportDeadline, "mcp.client.operation.duration")
		if !found {
			t.Fatalf("mcp.client.operation.duration was never recorded, so the second half of the convention is unmeasured.\nCollector:\n%s",
				c.containerLogs(t))
		}
		if duration.Unit != "s" {
			t.Errorf("unit = %q, want %q", duration.Unit, "s")
		}
	})

	t.Run("the poll that noticed is its own rooted span", func(t *testing.T) {
		_, poll, found := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
			return s.Name == "subscription poll"
		})
		if !found {
			t.Fatal("no subscription poll span; the watcher is invisible")
		}
		// Rooted with a link rather than nested, because a poll outlives the
		// request that created the subscription and nesting it would attach
		// every poll to a span that ended minutes ago.
		if poll.ParentSpanID != "" {
			t.Errorf("the poll span has parent %s; it should be rooted and linked", poll.ParentSpanID)
		}
		if len(poll.Links) != 1 {
			t.Errorf("recorded %d links, want 1 back to the subscribe that created it", len(poll.Links))
		}
		if _, present := attr(poll.Attributes, "gitlab_mcp.subscription.kind"); !present {
			t.Errorf("the poll span does not say what kind of resource it polls; recorded %v", keys(poll.Attributes))
		}
	})

	sess.close(t)
}
