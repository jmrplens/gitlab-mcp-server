//go:build e2e

// securityattributes_test.go covers the security attribute lifecycle under
// a category of a top-level group, and the two ways an attribute is put on
// a project: the project's own update, and the bulk update across projects.
// Like a category, an attribute has no read of its own, so each step is
// asserted on its answer.

package ee

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securityattributes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two colors an attribute is created with and changed to.
const (
	attributeColor        = "#FF0000"
	attributeUpdatedColor = "#00FF00"
)

// The wait for the bulk update's background job to assign the attribute.
//
// The drain comes first and is the longer of the two: the job waits for its
// turn behind whatever the suite left in Sidekiq, which the licensed complete
// run of the wave 1 stack measured past the minute the poll alone allowed,
// while the job itself, once it runs, lands well inside that minute.
const (
	bulkUpdateDrainWait = 3 * time.Minute
	bulkUpdateInterval  = 2 * time.Second
	bulkUpdateWait      = 60 * time.Second
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
		// Take the attribute off again before the bulk add, so that the bulk
		// add has something to do: on a project that already carries it a
		// bulk update answering success proves nothing, and the removal at
		// the end would pass on the direct add alone.
		cleared := harness.Do[securityattributes.ProjectUpdateOutput](s, actionSecurityAttributeProjectUpdate, map[string]any{
			"project_id": f.project.ID, "remove_attribute_ids": []int64{attribute.ID},
		})
		if cleared.RemovedCount < 1 {
			e.T.Errorf("the project update removed %d attributes before the bulk add, want at least the one", cleared.RemovedCount)
		}

		bulk := harness.Do[securityattributes.BulkUpdateOutput](s, actionSecurityAttributeBulkUpdate, map[string]any{
			"project_ids": []int64{f.project.ID}, "attribute_ids": []int64{attribute.ID}, "mode": securityattributes.BulkUpdateModeAdd,
		})
		if bulk.Status != "success" {
			e.T.Errorf("the bulk update answered status %q, want success: %+v", bulk.Status, bulk)
		}

		// The bulk update is a background job: GitLab's BulkUpdateService
		// enqueues a scheduler worker and answers "initiated", so the
		// assignment lands later. No action reads a project's attributes,
		// so the removal is what observes it: retried until it finds the
		// one the bulk add assigned, which is what proves the bulk add did.
		// Sidekiq is drained first, so the poll measures the job rather than
		// the queue in front of it, and the log says what the drain returned:
		// true when it saw the queues empty, and also when it could not read
		// them.
		drained := fixture.DrainSidekiqWithin(e.Ctx, e.Client(), bulkUpdateDrainWait)
		e.T.Logf("the Sidekiq drain before the wait for the bulk add returned %t (queues seen empty, or unreadable)", drained)
		removed := harness.Eventually(s, actionSecurityAttributeProjectUpdate, map[string]any{
			"project_id": f.project.ID, "remove_attribute_ids": []int64{attribute.ID},
		}, bulkUpdateInterval, bulkUpdateWait, func(out securityattributes.ProjectUpdateOutput) bool { return out.RemovedCount >= 1 })
		if removed.RemovedCount < 1 {
			e.T.Errorf("the project update removed %d attributes after the bulk add, want the one the bulk add assigned", removed.RemovedCount)
		}

		harness.DoVoid(s, actionSecurityAttributeDelete, map[string]any{"attribute_id": attribute.ID})
		harness.DoVoid(s, actionSecurityCategoryDelete, map[string]any{"category_id": category.ID})
	})
}

// TestSecurityAttributes_Find_UpdateExampleIsACallTheInstanceAccepts asks
// the dynamic surface's find for the attribute update, which requires the
// attribute and at least one of a name, a description or a color, and then
// sends the example call find offers, with the real attribute bound in.
//
// The update is the action issue 1175 was found on: find listed all four as
// required and filled the color with "value", which its hex pattern refuses,
// so the call find offered was one the server turned away before GitLab saw
// it. Now required_params names the attribute alone, the other three are
// alternatives of their own, and the example fills the first of them; the
// instance accepting it and renaming the attribute is what proves the example
// is a call, not only a shape.
func TestSecurityAttributes_Find_UpdateExampleIsACallTheInstanceAccepts(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))
	f := buildClassificationFixture(e)
	s := e.On(harness.SurfaceDynamic)
	category := newSecurityCategory(e, s, f.group, true)

	created := harness.Do[securityattributes.CreateOutput](s, actionSecurityAttributeCreate, map[string]any{
		"namespace_id": f.group.ID, "category_id": category.ID,
		"attributes": []map[string]any{{"name": e.Name("attribute"), "description": "e2e security attribute", "color": attributeColor}},
	})
	if len(created.Attributes) != 1 || created.Attributes[0].ID == 0 {
		e.T.Fatalf("create answered %+v, want exactly the one attribute with an ID", created.Attributes)
	}
	attribute := created.Attributes[0]
	e.T.Cleanup(func() {
		if _, err := harness.Try[securityattributes.Output](s, actionSecurityAttributeDelete,
			map[string]any{"attribute_id": attribute.ID}, harness.For(harness.PurposeCleanup)); err != nil {
			e.T.Logf("the cleanup delete of attribute %d answered: %v", attribute.ID, err)
		}
	})

	result := findUpdateResult(e, s)
	if !slices.Equal(result.RequiredParams, []string{"attribute_id"}) {
		e.T.Errorf("find required_params = %v, want attribute_id alone", result.RequiredParams)
	}
	if want := [][]string{{"name"}, {"description"}, {"color"}}; !reflect.DeepEqual(result.RequiredParamsAnyOf, want) {
		e.T.Errorf("find required_params_any_of = %v, want %v", result.RequiredParamsAnyOf, want)
	}
	params, ok := result.Example.Arguments["params"].(map[string]any)
	if !ok {
		e.T.Fatalf("find example carries no params object: %#v", result.Example.Arguments)
	}
	if got := slices.Sorted(maps.Keys(params)); !slices.Equal(got, []string{"attribute_id", "name"}) {
		e.T.Fatalf("find example fills %v, want attribute_id and the first alternative, name", got)
	}

	params["attribute_id"] = attribute.ID
	updated := harness.Do[securityattributes.Output](s, actionSecurityAttributeUpdate, params)
	if updated.ID != attribute.ID || updated.Name != params["name"] {
		e.T.Errorf("the example update answered %+v, want attribute %d renamed to %v", updated, attribute.ID, params["name"])
	}
	if updated.Color != attributeColor {
		e.T.Errorf("the example update answered color %q, want %q left as it was", updated.Color, attributeColor)
	}
}

// findUpdateResult runs gitlab_find_action for the attribute update and
// returns its result, failing the test when find does not return it. It goes
// through Raw because find is discovery rather than an action the coverage
// report credits.
func findUpdateResult(e *harness.Env, s *harness.Session) dynamictools.FindResult {
	e.T.Helper()

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      dynamictools.FindActionToolName,
		Arguments: dynamictools.FindInput{Query: "update a security attribute name description or color", Limit: 10},
	})
	if err != nil {
		e.T.Fatalf("%s: %v", dynamictools.FindActionToolName, err)
	}
	if result == nil || result.IsError {
		e.T.Fatalf("%s answered an error: %+v", dynamictools.FindActionToolName, result)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		e.T.Fatalf("encoding the %s answer: %v", dynamictools.FindActionToolName, err)
	}
	var out dynamictools.FindOutput
	if err = json.Unmarshal(encoded, &out); err != nil {
		e.T.Fatalf("decoding the %s answer: %v", dynamictools.FindActionToolName, err)
	}
	for _, found := range out.Results {
		if found.ID == string(actionSecurityAttributeUpdate) {
			return found
		}
	}
	e.T.Fatalf("%s did not return %s", dynamictools.FindActionToolName, actionSecurityAttributeUpdate)
	return dynamictools.FindResult{}
}
