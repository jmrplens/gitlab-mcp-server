//go:build e2e

// grouplabels_test.go covers the archived flag of a group label: created
// archived, filtered by it in the listing, unarchived by an update, and
// deleted. The flag is a Free feature the old suite believed licensed and
// so never drove on the Community runtime.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// labelColor is the color every fixture label carries.
const labelColor = "#428BCA"

// labelNames lists the names of a label listing.
func labelNames(labels []grouplabels.Output) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return names
}

// TestGroupLabels_Archived_RoundTripsThroughCreateListAndUpdate creates an
// archived label and a plain one in a group of the surface's own, shows
// the archived filter tells them apart, unarchives the first by an update,
// deletes it, and leaves the plain one for the test's own cleanup to
// delete through the server, as an agent undoing its work would.
//
// Replaces: TestMeta_GroupLabelArchive
func TestGroupLabels_Archived_RoundTripsThroughCreateListAndUpdate(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("labels"))
		scope := map[string]any{"group_id": group.IDParam()}
		archivedName, plainName := e.Name("archived"), e.Name("plain")

		archived := harness.Do[grouplabels.Output](s, actionGroupLabelCreate, withParams(scope, map[string]any{
			"name": archivedName, "color": labelColor, "archived": true,
		}))
		if archived.Name != archivedName || !archived.Archived {
			e.T.Errorf("create answered %+v, want the label %q archived", archived, archivedName)
		}
		plain := harness.Do[grouplabels.Output](s, actionGroupLabelCreate, withParams(scope, map[string]any{"name": plainName, "color": labelColor}))
		if plain.Name != plainName || plain.Archived {
			e.T.Errorf("create answered %+v, want the label %q active", plain, plainName)
		}
		e.T.Cleanup(func() {
			// The group goes with its labels, so a delete that failed here
			// leaves nothing behind and is worth a line rather than a failure.
			if _, err := harness.Try[grouplabels.Output](s, actionGroupLabelDelete, withParams(scope, map[string]any{"label_id": plainName}), harness.For(harness.PurposeCleanup)); err != nil {
				e.T.Logf("the cleanup delete of label %q answered: %v", plainName, err)
			}
		})

		onlyArchived := harness.Do[grouplabels.ListOutput](s, actionGroupLabelList, withParams(scope, map[string]any{"archived": true}))
		if names := labelNames(onlyArchived.Labels); len(names) != 1 || names[0] != archivedName {
			e.T.Errorf("the listing filtered to archived labels holds %v, want only %q", names, archivedName)
		}
		everything := harness.Do[grouplabels.ListOutput](s, actionGroupLabelList, scope)
		if names := labelNames(everything.Labels); len(names) != 2 {
			e.T.Errorf("the unfiltered listing holds %v, want both labels", names)
		}

		unarchived := harness.Do[grouplabels.Output](s, actionGroupLabelUpdate, withParams(scope, map[string]any{"label_id": archivedName, "archived": false}))
		if unarchived.Archived {
			e.T.Errorf("update answered %+v after unarchiving, want archived=false", unarchived)
		}

		harness.DoVoid(s, actionGroupLabelDelete, withParams(scope, map[string]any{"label_id": archivedName}))
		remaining := harness.Do[grouplabels.ListOutput](s, actionGroupLabelList, scope)
		if names := labelNames(remaining.Labels); len(names) != 1 || names[0] != plainName {
			e.T.Errorf("the listing after the delete holds %v, want only %q", names, plainName)
		}
	})
}
