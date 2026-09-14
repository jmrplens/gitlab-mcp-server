//go:build e2e

// mcp_subscription_delivery_test.go watches a resource this test owns and
// changes it, which is the half of the subscription capability the sweep
// deliberately cannot reach.
//
// mcp_subscriptions_test.go subscribes to every advertised template and stops
// there, because the shared World is read-only by contract and observing an
// update would mean changing something in it. What that leaves unproven is the
// part a client actually depends on: that the server polls GitLab, notices the
// content differ, and delivers notifications/resources/updated for the URI it
// was given. Nothing else in the suite asserts a server-initiated notification
// ever arrives.
//
// The resource is an issue created here, so changing it disturbs nobody, and
// the assertion is the notification rather than the read: a subscribe that is
// accepted and never fires is exactly the failure this covers.

package common

import (
	"fmt"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// subscriptionDeliveryTimeout bounds the wait for one update notification.
//
// The server polls rather than listening, so the notification cannot arrive
// before the next poll: the wait has to cover a full cadence with room for the
// GitLab call inside it, and generously, since a fixture instance under a
// suite's load is the slowest GitLab this will ever run against. A test that
// timed out here would report a delivery defect that was a busy server.
const subscriptionDeliveryTimeout = 90 * time.Second

// TestSubscriptionDelivery_ChangedIssue_NotifiesTheSubscriber subscribes to an
// issue of its own, changes its title, and waits for the update.
//
// One surface rather than three: subscriptions are a capability of the server
// and not of a tool surface, the watcher is per credential, and each extra
// surface would add a poll cadence of waiting to prove the same thing once
// more.
func TestSubscriptionDelivery_ChangedIssue_NotifiesTheSubscriber(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceDynamic)

	project := fixture.NewProject(e, fixture.WithNamePrefix("subdelivery"))
	issue := fixture.NewIssue(e, project, "subscription delivery fixture")
	// The template this binds is gitlab://project/{project_id}/issue/{issue_iid},
	// written out rather than expanded from the manifest because the resource
	// is this test's own and no World binding names it.
	uri := fmt.Sprintf("gitlab://project/%s/issue/%d", project.IDParam(), issue.IID)

	watch := s.Subscribe(uri)
	if watch.URI() != uri {
		t.Errorf("the subscription reports URI %q, want %q", watch.URI(), uri)
	}
	// Nothing has changed yet, so nothing may have been delivered. This is the
	// half that catches a watcher notifying on its own baseline read, which
	// would make every later assertion here pass for the wrong reason.
	if watch.TryNext(2 * time.Second) {
		t.Error("an update arrived before anything changed: the baseline read was reported as a change")
	}

	updated := harness.Do[issues.Output](s, actionIssueUpdate, map[string]any{
		"project_id": project.IDParam(),
		"issue_iid":  issue.IID,
		"title":      "subscription delivery fixture, changed",
	})
	if updated.IID != issue.IID {
		t.Fatalf("the update answered issue #%d, want #%d", updated.IID, issue.IID)
	}

	watch.Next(subscriptionDeliveryTimeout)
}
