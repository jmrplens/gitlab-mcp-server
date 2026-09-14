//go:build e2e

// b7_mr_subscription_test.go covers the two actions that toggle the caller's
// own subscription to a merge request.
//
// The unsubscribe runs first on purpose. GitLab counts the author of a request
// as subscribed to it without any subscription record of its own, so a
// subscribe asked for first would be answered 304 Not Modified with an empty
// body; the handler turns that into a plain read, which is correct and would
// make the assertion say nothing about the subscribe. Asking to unsubscribe
// first leaves the request in a state a subscribe has to change. Both
// assertions hold either way: a 304 means the caller is already in the state
// asked for, and the read the handler falls back to reports exactly that.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMergeRequestSubscription_UnsubscribeThenSubscribe unsubscribes the
// caller from a merge request of its own on every surface and reads that the
// answer no longer reports a subscription, then subscribes again and reads
// that it does.
func TestMergeRequestSubscription_UnsubscribeThenSubscribe(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrsubscribe"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "mrsubscribe")
		params := f.params()

		unsubscribed := harness.Do[mergerequests.Output](s, actionMergeRequestUnsubscribe, params)
		if unsubscribed.IID != f.mr.IID || unsubscribed.Subscribed {
			e.T.Fatalf("unsubscribe answered %+v, want request !%d with no subscription", unsubscribed, f.mr.IID)
		}

		subscribed := harness.Do[mergerequests.Output](s, actionMergeRequestSubscribe, params)
		if subscribed.IID != f.mr.IID || !subscribed.Subscribed {
			e.T.Errorf("subscribe answered %+v, want request !%d subscribed to", subscribed, f.mr.IID)
		}
	})
}
