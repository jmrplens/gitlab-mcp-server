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
// rather than subscribed, and so is one whose first read stands on an extra
// the World does not hold although every variable binds: the latest pipeline
// template on a project GitLab created no pipeline in, whose read can only
// fail. Each subscribe is made for the sweep, so what it
// earns in the coverage record is sweep-only; an asserted subscription comes
// from a scenario that subscribes on purpose, such as the delivery test.
//
// Two facts about the server shape the sweep. A subscription counts as
// accepted only once the server acknowledges it, because on protocol
// 2026-07-28 the SDK discards the server's answer to a subscribe, and the
// acknowledgement is the one word a client gets. A refusal is therefore
// learned only by that acknowledgement timing out (the harness's
// subscribeAckTimeout, 30 seconds), so each declined template costs the sweep
// that long, and a run whose log shows a 30-second gap before each declined
// line is behaving as designed. And the server holds at most ten watchers per
// credential, evicting only one that has sat idle past its thirty-minute lease,
// so twenty-six subscriptions held at once would be sixteen refusals. The sweep
// therefore runs on a session of its own and closes each subscription before
// opening the next; a close reaches the server a moment after it returns, so a
// few watchers may overlap, far inside the cap.
//
// A decline is therefore a verdict and not only a log line. A template the
// suite expects the server to decline is declared in knownDeclines with the
// reason, and any other decline fails the sweep: it is the watcher cap, which
// is what a Close that stopped releasing the server's watcher would produce
// (ten acknowledged, sixteen declined at thirty seconds each), or a first read
// the World should have satisfied. The two read the same from here, which is
// why the sweep fails on either rather than guessing which it was. A declared
// template the server acknowledges fails too, so the table cannot outlive the
// defect that justified the entry, and so does one the server does not
// advertise as subscribable at all, renamed or withdrawn, since that entry
// excuses nothing. A declared template the World could not bind on this run
// is logged as not judged rather than failed: the extra it stands on is one an
// instance may legitimately lack, and the next run that binds it judges it.

package common

import (
	"maps"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// knownDeclines is every subscribable template the sweep expects the server to
// decline, with the reason. An entry goes when the defect it names is fixed:
// the sweep fails once the server acknowledges a template declared here, and
// once the server no longer advertises it as subscribable.
var knownDeclines = map[string]string{
	// The World binds its feature branch, slash and all, and the branch
	// resource hands the percent-encoded name to GitLab undecoded, so the
	// first read is a 404 and the subscribe is declined (issue 912).
	"gitlab://project/{project_id}/branch/{branch}": "issue 912: a branch name carrying a slash is escaped twice",
}

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

	acknowledged := 0
	var declined, unexpected []string
	unbound := map[string]string{}
	for _, template := range templates {
		uri, missing := expandTemplate(template, world)
		if missing != "" {
			t.Logf("subscribable %s not subscribed: %s", template, missing)
			unbound[template] = missing
			continue
		}
		subscription, err := s.TrySubscribe(uri, harness.For(harness.PurposeSweep))
		if err != nil {
			declined = append(declined, template)
			reason, known := knownDeclines[template]
			if known {
				t.Logf("subscribable %s declined for %s, as expected (%s): %v", template, uri, reason, err)
				continue
			}
			t.Logf("subscribable %s declined for %s: %v", template, uri, err)
			unexpected = append(unexpected, template)
			continue
		}
		subscription.Close()
		acknowledged++
		if reason, known := knownDeclines[template]; known {
			t.Errorf("subscribable %s was acknowledged, but knownDeclines expects it declined (%s): remove the entry", template, reason)
		}
	}
	t.Logf("subscribed to %d of %d advertised subscribable templates (%d declined, %d the World cannot bind)",
		acknowledged, len(templates), len(declined), len(unbound))
	for _, template := range slices.Sorted(maps.Keys(knownDeclines)) {
		switch missing, skipped := unbound[template]; {
		case !slices.Contains(templates, template):
			t.Errorf("knownDeclines names %s (%s), which the server does not advertise as subscribable: "+
				"the entry excuses nothing, so rename or remove it", template, knownDeclines[template])
		case skipped:
			t.Logf("knownDeclines entry %s was not judged on this run: the World could not bind it (%s)", template, missing)
		}
	}
	if acknowledged == 0 {
		t.Fatal("the server acknowledged no subscription the World could bind; the sweep proves nothing about subscribing")
	}
	if len(unexpected) > 0 {
		slices.Sort(unexpected)
		t.Fatalf("%d of %d declines are not in knownDeclines (%q): a decline past the known ones is the watcher cap, "+
			"which a subscription whose Close stopped releasing the server's watcher reaches after ten, "+
			"or a first read the World should have satisfied", len(unexpected), len(declined), unexpected)
	}
}
