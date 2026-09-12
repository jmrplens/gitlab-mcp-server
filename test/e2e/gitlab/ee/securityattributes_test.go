//go:build e2e

// securityattributes_test.go covers the security attribute lifecycle under
// a category of a top-level group, and the two ways an attribute is put on
// a project: the project's own update, and the bulk update across projects.
// Like a category, an attribute has no read of its own, so each step is
// asserted on its answer.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securityattributes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two colors an attribute is created with and changed to.
const (
	attributeColor        = "#FF0000"
	attributeUpdatedColor = "#00FF00"
)

// classificationFixture is a top-level group with a project in it, which is
// what an attribute is assigned to.
type classificationFixture struct {
	group   fixture.Group
	project fixture.Project
}

// buildClassificationFixture creates both.
func buildClassificationFixture(e *harness.Env) classificationFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("secattr"))
	return classificationFixture{group: group, project: fixture.NewProject(e, fixture.WithNamePrefix("secattr"), fixture.InGroup(group))}
}

// TestSecurityAttributes_Lifecycle_AssignsToAProjectAndDeletes creates a
// category and an attribute in it on every surface, renames and recolors
// the attribute, puts it on the project both ways, takes it off, and
// deletes the attribute and then the category.
//
// Replaces: TestMeta_SecurityAttributes, TestMeta_SecurityClassifications
func TestSecurityAttributes_Lifecycle_AssignsToAProjectAndDeletes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, buildClassificationFixture, func(e *harness.Env, surface harness.Surface, f classificationFixture) {
		s := e.On(surface)
		category := newSecurityCategory(e, s, f.group, true)

		name := e.Name("attribute")
		created := harness.Do[securityattributes.CreateOutput](s, actionSecurityAttributeCreate, map[string]any{
			"namespace_id": f.group.ID, "category_id": category.ID,
			"attributes": []map[string]any{{"name": name, "description": "e2e security attribute", "color": attributeColor}},
		})
		if len(created.Attributes) != 1 || created.Attributes[0].ID == 0 {
			e.T.Fatalf("create answered %+v, want exactly the one attribute with an ID", created.Attributes)
		}
		attribute := created.Attributes[0]
		e.T.Cleanup(func() {
			// The body deletes the attribute itself; this is the teardown an
			// agent's session runs regardless, and a refusal of an attribute
			// already gone is worth a line rather than a failure.
			if _, err := harness.Try[securityattributes.Output](s, actionSecurityAttributeDelete,
				map[string]any{"attribute_id": attribute.ID}, harness.For(harness.PurposeCleanup)); err != nil {
				e.T.Logf("the cleanup delete of attribute %d answered: %v", attribute.ID, err)
			}
		})

		updated := harness.Do[securityattributes.Output](s, actionSecurityAttributeUpdate, map[string]any{
			"attribute_id": attribute.ID, "name": name + "-updated", "color": attributeUpdatedColor,
		})
		if updated.ID != attribute.ID || updated.Name != name+"-updated" || updated.Color != attributeUpdatedColor {
			e.T.Errorf("update answered %+v, want attribute %d renamed to %q in %s", updated, attribute.ID, name+"-updated", attributeUpdatedColor)
		}

		added := harness.Do[securityattributes.ProjectUpdateOutput](s, actionSecurityAttributeProjectUpdate, map[string]any{
			"project_id": f.project.ID, "add_attribute_ids": []int64{attribute.ID},
		})
		if added.AddedCount < 1 {
			e.T.Errorf("the project update added %d attributes, want at least the one", added.AddedCount)
		}

		bulk := harness.Do[securityattributes.BulkUpdateOutput](s, actionSecurityAttributeBulkUpdate, map[string]any{
			"project_ids": []int64{f.project.ID}, "attribute_ids": []int64{attribute.ID}, "mode": securityattributes.BulkUpdateModeAdd,
		})
		if bulk.Status != "success" {
			e.T.Errorf("the bulk update answered status %q, want success: %+v", bulk.Status, bulk)
		}

		removed := harness.Do[securityattributes.ProjectUpdateOutput](s, actionSecurityAttributeProjectUpdate, map[string]any{
			"project_id": f.project.ID, "remove_attribute_ids": []int64{attribute.ID},
		})
		if removed.RemovedCount < 1 {
			e.T.Errorf("the project update removed %d attributes, want at least the one", removed.RemovedCount)
		}

		harness.DoVoid(s, actionSecurityAttributeDelete, map[string]any{"attribute_id": attribute.ID})
		harness.DoVoid(s, actionSecurityCategoryDelete, map[string]any{"category_id": category.ID})
	})
}
