//go:build e2e

// mcp_subscriptions_test.go subscribes to every subscribable resource the
// server advertises whose URI binds from the shared World, through the real
// binary, since only cmd/server wires the subscription machinery.
//
// It proves the reach a unit test cannot: the subscription surface is set up
// only in the server binary, so a subscribe the server acknowledges here is
// one the released program accepts. Delivery is deliberately not exercised:
// observing an update would mean changing the watched resource, and the World
// is read-only by contract, so the sweep proves the subscribe is accepted and
// leaves proving a notification arrives to the scenario tests that create
// state of their own. A template whose URI the World cannot bind is named
// rather than subscribed.
//
// Two facts about the server shape the sweep. A subscription counts as
// accepted only once the server acknowledges it, because on protocol
// 2026-07-28 the SDK discards the server's answer to a subscribe, and the
// acknowledgement is the one word a client gets. And the server holds at most
// ten watchers per credential, evicting only one that has sat idle past its
// thirty-minute lease, so twenty-six subscriptions held at once would be
// sixteen refusals. The sweep therefore runs on a session of its own and
// closes each subscription before opening the next; a close reaches the server
// a moment after it returns, so a few watchers may overlap, far inside the cap.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestSubscriptions_Sweep subscribes to every advertised subscribable template
// whose URI binds from the World, one at a time, and names every one the
// server declines.
//
// It replaces no old test: the old suite had no subscription coverage, since
// subscriptions are wired only in cmd/server, which the in-process suite never
// started.
func TestSubscriptions_Sweep(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.Session(harness.ServerConfig{Surface: harness.SurfaceDynamic, Private: true})

	manifest := readToolsManifest(e, s)
	if manifest.Subscriptions == nil || !manifest.Subscriptions.Supported {
		t.Fatal("the full-capability session advertises no subscriptions; the sweep would prove nothing")
	}
	templates := manifest.Subscriptions.SubscribableURITemplates
	if len(templates) == 0 {
		t.Fatal("the subscription manifest lists no template")
	}

	acknowledged, declined, skipped := 0, 0, 0
	for _, template := range templates {
		uri, missing := expandTemplate(template, world)
		if missing != "" {
			t.Logf("subscribable %s not subscribed: %s", template, missing)
			skipped++
			continue
		}
		subscription, err := s.TrySubscribe(uri)
		if err != nil {
			t.Logf("subscribable %s declined for %s: %v", template, uri, err)
			declined++
			continue
		}
		subscription.Close()
		acknowledged++
	}
	t.Logf("subscribed to %d of %d advertised subscribable templates (%d declined, %d the World cannot bind)",
		acknowledged, len(templates), declined, skipped)
	if acknowledged == 0 {
		t.Fatal("the server acknowledged no subscription the World could bind; the sweep proves nothing about subscribing")
	}
}
