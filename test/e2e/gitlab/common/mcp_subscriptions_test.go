//go:build e2e

// mcp_subscriptions_test.go subscribes to every subscribable resource the
// server advertises whose URI binds from the shared World, through the real
// binary, since only cmd/server wires the subscription machinery.
//
// It proves the reach a unit test cannot: the subscription surface is set up
// only in the server binary, so a subscribe accepted here is one the released
// program accepts. Delivery is deliberately not exercised: observing an update
// would mean changing the watched resource, and the World is read-only by
// contract, so the sweep proves the subscribe is accepted and leaves proving a
// notification arrives to the scenario tests that create state of their own. A
// template whose URI the World cannot bind is named rather than subscribed.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestSubscriptions_Sweep subscribes to every advertised subscribable template
// whose URI binds from the World.
//
// Replaces: the subscription coverage the old suite had none of, since
// subscriptions are wired only in cmd/server, which the in-process suite never
// started.
func TestSubscriptions_Sweep(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceDynamic)

	manifest := readToolsManifest(e, s)
	if manifest.Subscriptions == nil || !manifest.Subscriptions.Supported {
		t.Fatal("the full-capability session advertises no subscriptions; the sweep would prove nothing")
	}
	templates := manifest.Subscriptions.SubscribableURITemplates
	if len(templates) == 0 {
		t.Fatal("the subscription manifest lists no template")
	}

	subscribed, skipped := 0, 0
	for _, template := range templates {
		uri, missing := expandTemplate(template, world)
		if missing != "" {
			t.Logf("subscribable %s not subscribed: %s", template, missing)
			skipped++
			continue
		}
		s.Subscribe(uri)
		subscribed++
	}
	t.Logf("subscribed to %d of %d advertised subscribable templates (%d the World cannot bind)",
		subscribed, len(templates), skipped)
}
