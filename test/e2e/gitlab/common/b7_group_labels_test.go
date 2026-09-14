//go:build e2e

// b7_group_labels_test.go covers the single-label read of a group and the
// subscription pair that hangs off it.
//
// grouplabels_test.go drives the archived flag through the create, list,
// update and delete; the read of one label by name and the two calls that
// turn a caller's subscription on and off were reached here only by a read
// sweep asking for a label that is not there, which asserts the refusal and
// nothing about the label.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestGroupLabels_Subscription_GetSubscribeAndUnsubscribe reads a group label
// of each surface's own by its name, subscribes the calling user to it, reads
// the flag back, unsubscribes and reads it back again.
//
// It runs on the dynamic, meta and individual surfaces. It asserts that the
// get answers the label the fixture created and reports it unsubscribed,
// that the subscribe answers the same label with the flag set, that a
// re-read agrees, and that after the unsubscribe a re-read reports it clear
// again. The re-reads are what make the pair an assertion about the group's
// state rather than about the two answers alone.
func TestGroupLabels_Subscription_GetSubscribeAndUnsubscribe(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("labelsub"))
		label := fixture.NewGroupLabel(e, group, "sub")
		params := map[string]any{"group_id": group.IDParam(), "label_id": label.Name}

		got := harness.Do[grouplabels.Output](s, actionGroupLabelGet, params)
		if got.ID != label.ID || got.Name != label.Name {
			e.T.Fatalf("group_label_get answered %+v, want the label %q with id %d", got, label.Name, label.ID)
		}
		if got.Subscribed {
			e.T.Errorf("group_label_get answered subscribed=true for %q, and nothing subscribed to it", label.Name)
		}

		subscribed := harness.Do[grouplabels.Output](s, actionGroupLabelSubscribe, params)
		if subscribed.Name != label.Name || !subscribed.Subscribed {
			e.T.Errorf("group_label_subscribe answered %+v, want the label %q subscribed", subscribed, label.Name)
		}
		if reread := harness.Do[grouplabels.Output](s, actionGroupLabelGet, params); !reread.Subscribed {
			e.T.Errorf("group_label_get answered subscribed=false for %q right after subscribing to it", label.Name)
		}

		harness.DoVoid(s, actionGroupLabelUnsubscribe, params)
		if reread := harness.Do[grouplabels.Output](s, actionGroupLabelGet, params); reread.Subscribed {
			e.T.Errorf("group_label_get answered subscribed=true for %q after unsubscribing from it", label.Name)
		}
	})
}
