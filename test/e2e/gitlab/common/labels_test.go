//go:build e2e

// labels_test.go covers a project's labels through the server: create one,
// find it in the listing, change its description and color, delete it and
// check the listing no longer holds it, once per surface on one shared
// project.

package common

import (
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two colors a label is given here, spelled as GitLab spells them.
const (
	labelColorBlue  = "#428BCA"
	labelColorGreen = "#00FF00"
)

// labelIDs lists the IDs of a label listing.
func labelIDs(listed []labels.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, label := range listed {
		ids = append(ids, label.ID)
	}
	return ids
}

// TestProjectLabels_Lifecycle_CreateListUpdateDelete creates a label on
// every surface, finds it in the project's listing, updates its description
// and color, deletes it and checks it is gone from the listing. The label is
// named by its ID spelled as a string, which is how the schema declares the
// parameter: it also accepts the label's name.
//
// Replaces: TestIndividual_Labels, TestMeta_Labels
func TestProjectLabels_Lifecycle_CreateListUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("labels"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		name := e.Name("label")

		created := harness.Do[labels.Output](s, actionProjectLabelCreate, withParams(params, map[string]any{"name": name, "color": labelColorBlue}))
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("label_create answered %+v, want the label %q with an ID", created, name)
		}
		labelParams := withParams(params, map[string]any{"label_id": strconv.FormatInt(created.ID, 10)})

		listed := harness.Do[labels.ListOutput](s, actionProjectLabelList, params)
		if !containsID(labelIDs(listed.Labels), created.ID) {
			e.T.Errorf("the project's labels do not hold %d: %v", created.ID, labelIDs(listed.Labels))
		}

		updated := harness.Do[labels.Output](s, actionProjectLabelUpdate, withParams(labelParams, map[string]any{
			"description": "updated by the e2e suite", "color": labelColorGreen,
		}))
		if updated.ID != created.ID || updated.Description != "updated by the e2e suite" || updated.Color != labelColorGreen {
			e.T.Errorf("label_update answered %+v, want label %d with the new description and color %s", updated, created.ID, labelColorGreen)
		}

		harness.DoVoid(s, actionProjectLabelDelete, labelParams)
		remaining := harness.Do[labels.ListOutput](s, actionProjectLabelList, params)
		if containsID(labelIDs(remaining.Labels), created.ID) {
			e.T.Errorf("label %d is still listed after its delete", created.ID)
		}
	})
}
