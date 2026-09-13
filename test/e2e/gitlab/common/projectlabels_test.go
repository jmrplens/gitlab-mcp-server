//go:build e2e

// projectlabels_test.go covers what a project label offers beyond its own
// lifecycle: the subscription a caller can take out on it and give up, and
// its promotion to the group the project sits in, which a personal project
// has no group for.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labeldata"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestProjectLabels_SubscribeAndPromote_ReadBack creates a label in a
// personal project of each surface's own, subscribes to it, reads the
// subscription back, ends it, and shows the promotion of a personal
// project's label refused; then promotes a label of a group project and
// finds it among the group's labels.
//
// Replaces: TestMeta_ProjectLabelsDeep
func TestProjectLabels_SubscribeAndPromote_ReadBack(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		personal := fixture.NewProject(e, fixture.WithNamePrefix("personallabels"))
		params := map[string]any{"project_id": personal.IDParam()}
		name := e.Name("label")

		created := harness.Do[labeldata.Output](s, actionProjectLabelCreate, withParams(params, map[string]any{"name": name, "color": labelColor}))
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("label_create answered %+v, want the label %q with an ID", created, name)
		}
		label := withParams(params, map[string]any{"label_id": name})

		got := harness.Do[labeldata.Output](s, actionProjectLabelGet, label)
		if got.ID != created.ID || got.Subscribed {
			e.T.Errorf("label_get answered %+v, want label %d with no subscription yet", got, created.ID)
		}
		subscribed := harness.Do[labeldata.Output](s, actionProjectLabelSubscribe, label)
		if subscribed.ID != created.ID || !subscribed.Subscribed {
			e.T.Errorf("label_subscribe answered %+v, want label %d subscribed", subscribed, created.ID)
		}
		// The unsubscribe answers no content, so the read afterwards is
		// what tells a subscription that ended from a call that returned.
		harness.DoVoid(s, actionProjectLabelUnsubscribe, label)
		unsubscribed := harness.Do[labeldata.Output](s, actionProjectLabelGet, label)
		if unsubscribed.Subscribed {
			e.T.Errorf("the label still reads as subscribed after the unsubscribe: %+v", unsubscribed)
		}

		refused := harness.ExpectToolError(s, actionProjectLabelPromote, label, "promote")
		e.T.Logf("the promotion of a personal project's label is refused: %s", firstLine(refused))

		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("promotion"))
		grouped := fixture.NewProject(e, fixture.WithNamePrefix("groupedlabels"), fixture.InGroup(group))
		promotable := e.Name("promotable")
		groupedLabel := harness.Do[labeldata.Output](s, actionProjectLabelCreate, map[string]any{"project_id": grouped.IDParam(), "name": promotable, "color": labelColor})
		if groupedLabel.ID == 0 || !groupedLabel.IsProjectLabel {
			e.T.Fatalf("label_create answered %+v, want the project label %q with an ID", groupedLabel, promotable)
		}
		harness.DoVoid(s, actionProjectLabelPromote, map[string]any{"project_id": grouped.IDParam(), "label_id": promotable})
		if !groupHasLabel(e, group, promotable) {
			e.T.Errorf("group %d does not hold the label %q after its promotion", group.ID, promotable)
		}
	})
}

// groupHasLabel reads a group's labels through client-go and reports
// whether one carries the name.
func groupHasLabel(e *harness.Env, group fixture.Group, name string) bool {
	e.T.Helper()
	labels, _, err := e.Client().GL().GroupLabels.ListGroupLabels(group.ID, &gl.ListGroupLabelsOptions{Search: new(name)}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("listing the labels of group %d through client-go: %v", group.ID, err)
	}
	for _, label := range labels {
		if label.Name == name {
			return true
		}
	}
	return false
}
