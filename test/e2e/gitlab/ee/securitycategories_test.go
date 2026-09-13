//go:build e2e

// securitycategories_test.go covers the security category lifecycle under a
// top-level group: create, update, delete. The catalog offers no read of a
// category, so what a step changed is read off its own answer, and the
// delete is observable only by not failing.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securitycategories"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// newSecurityCategory creates a category in the group through the session
// and registers a second delete of it as the test's own cleanup, through
// the server, which is what an agent's session does with what it created
// and what the old suite recorded as the category's teardown.
func newSecurityCategory(e *harness.Env, s *harness.Session, group fixture.Group, multiple bool) securitycategories.Output {
	e.T.Helper()

	created := harness.Do[securitycategories.Output](s, actionSecurityCategoryCreate, map[string]any{
		"namespace_id": group.ID, "name": e.Name("category"), "description": "e2e security category", "multiple_selection": multiple,
	})
	if created.ID == 0 {
		e.T.Fatalf("create answered %+v, want a category with an ID", created)
	}
	e.T.Cleanup(func() {
		// A refusal of a category the body already deleted is worth a line
		// rather than a failure: the group goes with what is left.
		if _, err := harness.Try[securitycategories.Output](s, actionSecurityCategoryDelete,
			map[string]any{"category_id": created.ID}, harness.For(harness.PurposeCleanup)); err != nil {
			e.T.Logf("the cleanup delete of category %d answered: %v", created.ID, err)
		}
	})
	return created
}

// TestSecurityCategories_Lifecycle_CreatesUpdatesAndDeletes walks one
// category per surface under a shared group.
//
// Replaces: TestMeta_SecurityCategories, TestMeta_SecurityClassifications
func TestSecurityCategories_Lifecycle_CreatesUpdatesAndDeletes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("seccat"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)

		created := newSecurityCategory(e, s, group, true)
		if !created.MultipleSelection {
			e.T.Errorf("create answered multiple_selection=false after asking for true: %+v", created)
		}

		renamed := created.Name + "-updated"
		updated := harness.Do[securitycategories.Output](s, actionSecurityCategoryUpdate, map[string]any{
			"category_id": created.ID, "namespace_id": group.ID, "name": renamed, "description": "updated e2e security category",
		})
		if updated.ID != created.ID || updated.Name != renamed || updated.Description != "updated e2e security category" {
			e.T.Errorf("update answered %+v, want category %d renamed to %q with the new description", updated, created.ID, renamed)
		}

		harness.DoVoid(s, actionSecurityCategoryDelete, map[string]any{"category_id": created.ID})
	})
}
